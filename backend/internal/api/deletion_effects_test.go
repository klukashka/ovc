package api_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/operators"
)

const (
	tutorDeletedSubject   = "A tutor closed their OVC account"
	deletionApplied       = `"msg":"tutor.deletion_applied"`
	endedByDeletion       = `"by":"deletion"`
	seededCardPrefix      = "Seeded card "
	tutorRelationships    = "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid"
	tutorInvitations      = "SELECT count(invitations.id) FROM invitations JOIN relationships ON relationships.id = invitations.relationship_id WHERE relationships.tutor_id = $1::uuid"
	tutorOpenInvitations  = tutorInvitations + " AND invitations.used_at IS NULL AND invitations.revoked_at IS NULL"
	tutorJoinRequests     = "SELECT count(id) FROM join_requests WHERE tutor_id = $1::uuid"
	tutorBlockedStudents  = "SELECT count(student_id) FROM blocked_requesters WHERE tutor_id = $1::uuid"
	tutorDecidedRequests  = "SELECT count(id) FROM join_requests WHERE tutor_id = $1::uuid AND state IN ('accepted', 'declined')"
	tutorGroups           = "SELECT count(id) FROM groups WHERE tutor_id = $1::uuid"
	tutorGroupMembers     = "SELECT count(group_id) FROM group_members WHERE tutor_id = $1::uuid"
	tutorChannels         = "SELECT count(id) FROM channels WHERE tutor_id = $1::uuid"
	tutorChannelMembers   = "SELECT count(channel_members.user_id) FROM channel_members JOIN channels ON channels.id = channel_members.channel_id WHERE channels.tutor_id = $1::uuid"
	tutorProfiles         = "SELECT count(user_id) FROM tutor_profiles WHERE user_id = $1::uuid"
	tutorNotifications    = "SELECT count(id) FROM notifications WHERE user_id = $1::uuid OR tutor_id = $1::uuid OR student_id = $1::uuid"
	tutorDeletedQueued    = "SELECT count(id) FROM outbox WHERE kind = $1"
	studentRelationships  = "SELECT count(id) FROM relationships WHERE student_id = $1::uuid"
	requestsOfEmail       = "SELECT count(join_requests.id) FROM join_requests JOIN users ON users.id = join_requests.student_id WHERE users.email = $1"
	unarchivedGroupsQuery = "SELECT count(id) FROM groups WHERE tutor_id = $1::uuid AND archived_at IS NULL"
)

type purgedRow struct {
	State         string
	StudentID     *string
	Email         *string
	DisplayName   *string
	Notes         string
	DiscardReason *string
	EndedBy       *string
	EndedAt       *time.Time
}

func (h *authHarness) purgedRow(t *testing.T, id string) purgedRow {
	t.Helper()
	var row purgedRow
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT state, student_id::text, email::text, display_name, notes, discard_reason, ended_by, ended_at FROM relationships WHERE id = $1::uuid", id,
	).Scan(&row.State, &row.StudentID, &row.Email, &row.DisplayName, &row.Notes, &row.DiscardReason, &row.EndedBy, &row.EndedAt)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *authHarness) operators() *operators.Service {
	return operators.New(h.store, h.clock, slog.New(slog.NewJSONHandler(h.logs, nil)), h.effects)
}

func (h *authHarness) deleteInBackground(token string, password string) <-chan pendingAnswer {
	return h.sendInBackground(http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, password),
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"})
}

func (h *authHarness) regeneratedLink(t *testing.T, token string, card string) string {
	t.Helper()
	regenerated := h.cardCall(t, token, http.MethodPost, card, "/invitation", "")
	var issued struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
		t.Fatalf("regenerating the link answered %d %s", regenerated.status, regenerated.body)
	}
	return linkToken(t, &issued.Invitation)
}

func (h *authHarness) seedInvitedCards(t *testing.T, tutor domain.UserID, count int) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO relationships (id, tutor_id, state, display_name, email, email_set_at, notes, created_at, updated_at)
SELECT gen_random_uuid(), $1, 'invited', $2 || n, 'seeded-' || n || '@example.org', $3, 'Seeded notes', $3, $3
FROM generate_series(1, $4::int) AS n`, tutor, seededCardPrefix, authStart, count)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.database.Pool.Exec(context.Background(), `
INSERT INTO invitations (id, relationship_id, token_hash, expires_at, created_at)
SELECT gen_random_uuid(), id, sha256(id::text::bytea), $2::timestamptz + interval '7 days', $2
FROM relationships WHERE tutor_id = $1 AND display_name LIKE $3`, tutor, authStart, seededCardPrefix+"%")
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) tutorRows(t *testing.T, tutor domain.UserID) map[string]int64 {
	t.Helper()
	queries := map[string]string{
		"relationships": tutorRelationships, "invitations": tutorInvitations, "join requests": tutorJoinRequests,
		"blocked students": tutorBlockedStudents, "groups": tutorGroups, "group members": tutorGroupMembers, "profile": tutorProfiles,
		"notifications": tutorNotifications, "channels": tutorChannels, "channel members": tutorChannelMembers,
	}
	left := map[string]int64{}
	for table, query := range queries {
		if count := h.countRows(t, query, tutor.String()); count > 0 {
			left[table] = count
		}
	}
	return left
}

func hasSubject(messages []string, subject string) bool {
	for _, message := range messages {
		if message == subject {
			return true
		}
	}
	return false
}

func TestTutorDeletionEndsRelationshipsDiscardsCardsAndProvisionalLinksArchivesGroupsAndEmailsStudents(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.setAccount(t, parties.tutor, markupTutorName, domain.LanguageEnglish)
	invited := h.newCard(t, parties.tutorToken, "An invited student", cardEmail)
	invitedLink := linkToken(t, invited.Invitation)
	provisional, _ := h.linkedStudent(t, parties.tutorToken, newcomerEmail)
	_, requester := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	request := h.enteredRequest(t, requester, h.storedCode(t, parties.tutor).Display())
	deletedAt := h.now(t)

	deleted := h.deleteMe(t, parties.tutorToken, accountPassword)
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s", deleted.status, deleted.body)
	}
	active := h.purgedRow(t, parties.card.ID)
	if active.State != "ended" || active.EndedBy == nil || *active.EndedBy != "deletion" || !sameInstant(active.EndedAt, deletedAt) {
		t.Fatalf("the active relationship is %+v", active)
	}
	if h.membershipsOf(t, parties.card.ID) != 0 || h.countRows(t, unarchivedGroupsQuery, parties.tutor.String()) != 0 {
		t.Fatal("the deletion kept a membership or an unarchived group")
	}
	for name, card := range map[string]string{"the invited card": invited.ID, "the provisional link": provisional.ID} {
		if stored := h.storedRelationship(t, card); stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "tutor_deleted" {
			t.Fatalf("%s is %+v", name, stored)
		}
	}
	if fresh := h.cardsOf(t, parties.tutor, domain.RelationshipInvited); len(fresh) != 0 {
		t.Fatalf("the deleted tutor kept or got invited cards %+v", fresh)
	}
	if h.revokeReason(t, invitedLink) != "tutor_deleted" || h.ciphertextKept(t, invitedLink) || h.invitationPage(t, invitedLink).status != http.StatusNotFound {
		t.Fatal("the invited card's link was not revoked for the tutor's deletion")
	}
	if h.countRows(t, tutorOpenInvitations, parties.tutor.String()) != 0 || h.joinRequestState(t, request) != "expired" {
		t.Fatal("the deletion left an open invitation or an open join request")
	}
	_, tutors := h.myTutors(t, parties.studentToken)
	if len(tutors) != 1 || tutors[0].State != "ended" || tutors[0].EndedBy == nil || *tutors[0].EndedBy != "deletion" ||
		tutors[0].AccountClosedUntil == nil || !tutors[0].AccountClosedUntil.Equal(deletedAt.Add(domain.DeletedAccountRetention)) {
		t.Fatalf("the student lists %+v", tutors)
	}
	message := onlyMessage(t, h.mailTo(t, studentEmail), tutorDeletedSubject)
	if !strings.Contains(message.Text, markupTutorName) || !strings.Contains(message.HTML, `Anna &lt;a href=&#34;https://evil.example/&#34;&gt;Click&lt;/a&gt;`) {
		t.Fatalf("the email does not name the tutor escaped: %s", message.HTML)
	}
	if strings.Count(message.HTML, "<a ") != 1 || !strings.Contains(message.HTML, `<a href="`+publicOrigin+`/tutors"`) || strings.Contains(message.Text+message.HTML, cardName) {
		t.Fatalf("the email links elsewhere or carries the card name: %s", message.HTML)
	}
	if hasSubject(subjects(h.mailTo(t, newcomerEmail)), tutorDeletedSubject) || len(h.mailTo(t, requestingEmail)) != 0 || h.endedEmailsQueued(t) != 0 {
		t.Fatal("a provisional student or a requester was emailed, or an ending email was queued")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, relationshipEndedMsg) || !strings.Contains(logs, endedByDeletion) || !strings.Contains(logs, deletionApplied) ||
		!strings.Contains(logs, `"cause":"tutor_deleted"`) || !strings.Contains(logs, groupArchived) {
		t.Fatal("the deletion's effects were not recorded")
	}
}

func TestNoTutorDeletedEmailToUnconfirmedStudents(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unconfirmed := h.seedStudentAccount(t, 2, domain.AccountUnverified)
	card := h.seedCard(t, tutor, 1, domain.RelationshipActive, &unconfirmed)
	h.linkedStudent(t, token, newcomerEmail)

	deleted := h.deleteMe(t, token, accountPassword)
	h.tick(t)

	if deleted.status != http.StatusNoContent || h.storedRelationship(t, card).State != "ended" {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	if h.countRows(t, tutorDeletedQueued, string(domain.EventTutorDeleted)) != 0 {
		t.Fatal("a tutor.deleted email was queued for an unconfirmed student")
	}
	if len(h.mailTo(t, "member-2@example.org")) != 0 || hasSubject(subjects(h.mailTo(t, newcomerEmail)), tutorDeletedSubject) {
		t.Fatal("an unconfirmed student was emailed")
	}
}

type queuedTutorDeleted struct {
	IdempotencyKey string
	Payload        domain.TutorDeletedPayload
	Lane           string
	Priority       int16
	Status         string
	NextAttemptAt  time.Time
	CreatedAt      time.Time
}

func (h *authHarness) queuedTutorDeleted(t *testing.T) []queuedTutorDeleted {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT idempotency_key, payload::text, lane, priority, status, next_attempt_at, created_at FROM outbox WHERE kind = $1 ORDER BY idempotency_key",
		string(domain.EventTutorDeleted))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var queued []queuedTutorDeleted
	for rows.Next() {
		var row queuedTutorDeleted
		var payload string
		if err := rows.Scan(&row.IdempotencyKey, &payload, &row.Lane, &row.Priority, &row.Status, &row.NextAttemptAt, &row.CreatedAt); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(payload), &row.Payload); err != nil {
			t.Fatal(err)
		}
		queued = append(queued, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return queued
}

func expectedTutorDeleted(tutor domain.UserID, deletedAt time.Time, students ...domain.UserID) []queuedTutorDeleted {
	encodedTutor := hex.EncodeToString(tutor[:])
	expected := make([]queuedTutorDeleted, 0, len(students))
	for _, student := range students {
		recipient := hex.EncodeToString(student[:])
		expected = append(expected, queuedTutorDeleted{
			IdempotencyKey: fmt.Sprintf("tutor.deleted:%s:%d:%s", encodedTutor, deletedAt.UnixMicro(), recipient),
			Payload:        domain.TutorDeletedPayload{Tutor: encodedTutor, DeletedAtMicros: deletedAt.UnixMicro(), Recipient: recipient},
			Lane:           "email", Priority: 1, Status: "pending", NextAttemptAt: deletedAt, CreatedAt: deletedAt,
		})
	}
	return expected
}

func sameQueuedRows(got []queuedTutorDeleted, want []queuedTutorDeleted) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index].IdempotencyKey != want[index].IdempotencyKey || got[index].Payload != want[index].Payload || got[index].Lane != want[index].Lane ||
			got[index].Priority != want[index].Priority || got[index].Status != want[index].Status ||
			!got[index].NextAttemptAt.Equal(want[index].NextAttemptAt) || !got[index].CreatedAt.Equal(want[index].CreatedAt) {
			return false
		}
	}
	return true
}

func TestTutorDeletionQueuesOneTutorDeletedRowPerConfirmedStudentOfAnEndedRelationship(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 9, "other-tutor@example.org")
	first := h.seedStudentAccount(t, 2, domain.AccountActive)
	second := h.seedStudentAccount(t, 3, domain.AccountActive)
	unconfirmed := h.seedStudentAccount(t, 4, domain.AccountUnverified)
	formerlyEnded := h.seedStudentAccount(t, 5, domain.AccountActive)
	provisional := h.seedStudentAccount(t, 6, domain.AccountUnverified)
	h.seedCard(t, tutor, 1, domain.RelationshipActive, &first)
	h.seedCard(t, tutor, 2, domain.RelationshipActive, &second)
	h.seedCard(t, tutor, 3, domain.RelationshipActive, &unconfirmed)
	h.seedCard(t, tutor, 4, domain.RelationshipEnded, &formerlyEnded)
	h.seedCard(t, tutor, 5, domain.RelationshipProvisional, &provisional)
	h.seedCard(t, tutor, 6, domain.RelationshipInvited, nil)
	h.seedCard(t, otherTutor, 7, domain.RelationshipActive, &first)
	deletedAt := h.now(t)

	deleted := h.deleteMe(t, token, accountPassword)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s", deleted.status, deleted.body)
	}
	if queued, want := h.queuedTutorDeleted(t), expectedTutorDeleted(tutor, deletedAt, first, second); !sameQueuedRows(queued, want) {
		t.Fatalf("the queued tutor.deleted rows are %+v, want %+v", queued, want)
	}
	if h.countRows(t, "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid AND state = 'ended' AND ended_by = 'deletion'", tutor.String()) != 3 {
		t.Fatal("the deletion did not end exactly the three active relationships")
	}
	if h.countRows(t, "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid AND state = 'active'", otherTutor.String()) != 1 {
		t.Fatal("the deletion touched another tutor's relationship")
	}
	logs := h.logs.String()
	if strings.Count(logs, relationshipEndedMsg) != 3 || !strings.Contains(logs, `"ended":3`) || !strings.Contains(logs, `"emails_queued":2`) {
		t.Fatalf("the deletion did not record its three endings and two queued emails: %s", logs)
	}
}

func TestStudentDeletionEndsRelationshipsWithoutEmail(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	deletedAt := h.now(t)

	deleted := h.deleteMe(t, parties.studentToken, accountPassword)
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the student's deletion answered %d %s", deleted.status, deleted.body)
	}
	stored := h.purgedRow(t, parties.card.ID)
	if stored.State != "ended" || stored.EndedBy == nil || *stored.EndedBy != "deletion" || !sameInstant(stored.EndedAt, deletedAt) {
		t.Fatalf("the relationship is %+v", stored)
	}
	card := h.cardOf(t, parties.tutorToken, parties.card.ID)
	if card.State != "ended" || len(card.Groups) != 0 || h.membershipsOf(t, parties.card.ID) != 0 {
		t.Fatalf("the tutor's card shows %+v", card)
	}
	if h.endedEmailsQueued(t) != 0 || len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatal("the student's deletion emailed the tutor")
	}
	if logs := h.logs.String(); !strings.Contains(logs, relationshipEndedMsg) || !strings.Contains(logs, endedByDeletion) {
		t.Fatal("no relationship.ended record for the deletion")
	}
}

func TestPurgeOfStudentKeepsTheTutorsHistoryUnderTheFormerStudentLabel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.withNotes(t, parties.tutorToken, parties.card.ID, cardNotes)
	h.deleteMe(t, parties.studentToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	stored := h.purgedRow(t, parties.card.ID)
	if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "student_purged" || stored.StudentID != nil || stored.Email != nil {
		t.Fatalf("the purged student's relationship is %+v", stored)
	}
	if stored.DisplayName != nil || stored.Notes != "" || stored.EndedBy == nil || *stored.EndedBy != "deletion" || stored.EndedAt == nil {
		t.Fatalf("the tutor's history kept the card name or notes or lost its ending: %+v", stored)
	}
	if h.userExists(t, studentEmail) {
		t.Fatal("the student account was not purged")
	}
	listed := h.send(t, http.MethodGet, studentsPath, "", jsonCall(parties.tutorToken))
	for _, private := range []string{endingStudentName, studentEmail, cardName, cardNotes} {
		if strings.Contains(listed.body, private) {
			t.Fatalf("the tutor still sees %q: %s", private, listed.body)
		}
	}
}

func TestPurgeOfTutorDeletesCardsNotesInvitationsRequestsAndGroupsInBatches(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.withNotes(t, parties.tutorToken, parties.card.ID, cardNotes)
	h.seedInvitedCards(t, parties.tutor, 2*500+1)
	_, requester := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	h.enteredRequest(t, requester, h.storedCode(t, parties.tutor).Display())
	h.blockRequester(t, parties.tutor, parties.student)
	before := h.tutorRows(t, parties.tutor)
	h.deleteMe(t, parties.tutorToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention-time.Hour))
	h.tick(t)
	kept := h.tutorRows(t, parties.tutor)
	h.at(t, h.now(t).Add(time.Hour))
	h.tick(t)

	if before["relationships"] != 2*500+2 || before["invitations"] < 2*500+1 || before["groups"] != 1 || before["join requests"] != 1 || before["blocked students"] != 1 {
		t.Fatalf("the seed is %v", before)
	}
	if kept["relationships"] != before["relationships"] || kept["profile"] != 1 {
		t.Fatalf("the tutor was purged before 30 days: %v", kept)
	}
	if left := h.tutorRows(t, parties.tutor); len(left) != 0 || h.userExists(t, tutorEmail) {
		t.Fatalf("rows of the purged tutor remain: %v", left)
	}
	if !h.userExists(t, studentEmail) || !h.userExists(t, requestingEmail) {
		t.Fatal("the tutor's purge removed a student account")
	}
	if before["notifications"] == 0 {
		t.Fatalf("the seed holds no notification naming the tutor: %v", before)
	}
	if !strings.Contains(h.logs.String(), `"batches":12`) {
		t.Fatal("the tutor's purge did not run in batches of 500 rows")
	}
}

func TestTutorPurgeStopsAtItsBatchBudgetAndResumes(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.seedInvitedCards(t, tutor, 60*500)
	h.deleteMe(t, token, accountPassword)
	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))

	h.tick(t)
	afterFirstRun := h.tutorRows(t, tutor)
	h.advance(t, time.Hour)
	h.tick(t)

	if afterFirstRun["profile"] != 1 || afterFirstRun["invitations"] != 0 || afterFirstRun["relationships"] != 20*500 {
		t.Fatalf("the first run did not stop after its budget of 100 batches: %v", afterFirstRun)
	}
	if left := h.tutorRows(t, tutor); len(left) != 0 || h.userExists(t, tutorEmail) {
		t.Fatalf("the next run did not finish the purge: %v", left)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"job":"accounts.purge","result":"ok","batch_size":0`) || !strings.Contains(logs, `"msg":"account.purge_paused"`) {
		t.Fatal("the first run did not record the paused purge")
	}
	if !strings.Contains(logs, `"batches":100`) || !strings.Contains(logs, `"batches":20`) {
		t.Fatal("the runs did not record the batch transactions each of them ran")
	}
}

func TestRestoredTutorKeepsRelationshipsEnded(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.deleteMe(t, parties.tutorToken, accountPassword)

	if _, err := h.operators().Restore(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	tutorToken := h.relogin(t, tutorEmail)

	card := h.cardOf(t, tutorToken, parties.card.ID)
	if card.State != "ended" || len(card.Groups) != 0 {
		t.Fatalf("the restored tutor's card is %+v", card)
	}
	groups := h.listGroups(t, tutorToken)
	if len(groups) != 1 || groups[0].ArchivedAt == nil || h.membershipsOf(t, parties.card.ID) != 0 {
		t.Fatalf("the restored tutor's groups are %+v", groups)
	}
	_, tutors := h.myTutors(t, parties.studentToken)
	if len(tutors) != 1 || tutors[0].State != "ended" || tutors[0].AccountClosedUntil != nil {
		t.Fatalf("the student lists %+v", tutors)
	}
}

func TestTutorDeletedEmailIsObsoleteAfterRestore(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.deleteMe(t, parties.tutorToken, accountPassword)
	if _, err := h.operators().Restore(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	h.tick(t)

	if len(h.mailTo(t, studentEmail)) != 0 || h.obsoleteRows(t, domain.EventTutorDeleted) != 1 {
		t.Fatal("the tutor.deleted email was sent after the restore")
	}
}

func TestTutorDeletionRevokesAnEndedCardsRegeneratedLink(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	link := h.regeneratedLink(t, parties.tutorToken, parties.card.ID)

	deleted := h.deleteMe(t, parties.tutorToken, accountPassword)

	if deleted.status != http.StatusNoContent || h.storedRelationship(t, parties.card.ID).State != "ended" {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	if h.revokeReason(t, link) != "tutor_deleted" || h.ciphertextKept(t, link) || h.openInvitationsOf(t, parties.card.ID) != 0 {
		t.Fatal("the ended card's regenerated link was not revoked")
	}
	if page := h.invitationPage(t, link); page.status != http.StatusNotFound {
		t.Fatalf("the revoked link answered %d %s", page.status, page.body)
	}
}

func TestStudentPurgeClearsCardEmailsAlsoOnDiscardedRowsAndKeepsEndingFields(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutorA, tokenA := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, tokenB := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, secondTutorEmail)
	_, tokenC := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 4, "third-tutor@example.org")
	discarded, student := h.squat(t, tokenC, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	endedByTutor := h.linkedSignedIn(t, tokenA, owner, cardName, newcomerEmail)
	endedByDeletionCard := h.linkedSignedIn(t, tokenB, owner, cardName, newcomerEmail)
	h.deleteMe(t, tokenC, accountPassword)
	if _, err := h.operators().Restore(context.Background(), "third-tutor@example.org"); err != nil {
		t.Fatal(err)
	}
	h.endAsTutor(t, tokenA, endedByTutor.ID)
	h.deleteMe(t, owner, ownerPassword)
	before := h.purgedRow(t, discarded.ID)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if before.StudentID == nil || before.Email == nil || before.DiscardReason == nil || *before.DiscardReason != "tutor_deleted" {
		t.Fatalf("before the purge the discarded row is %+v", before)
	}
	if h.countRows(t, studentRelationships, student.String()) != 0 {
		t.Fatal("a relationship still names the purged student")
	}
	want := map[string]struct {
		reason  string
		endedBy *string
	}{
		discarded.ID:           {reason: "tutor_deleted"},
		endedByTutor.ID:        {reason: "student_purged", endedBy: ptr("tutor")},
		endedByDeletionCard.ID: {reason: "student_purged", endedBy: ptr("deletion")},
	}
	for id, expected := range want {
		row := h.purgedRow(t, id)
		if row.State != "discarded" || row.DiscardReason == nil || *row.DiscardReason != expected.reason || row.StudentID != nil || row.Email != nil {
			t.Errorf("the row %s is %+v", id, row)
		}
		if (expected.endedBy == nil) != (row.EndedBy == nil) || (row.EndedBy != nil && *row.EndedBy != *expected.endedBy) || (row.EndedBy != nil) != (row.EndedAt != nil) {
			t.Errorf("the row %s kept the wrong ending fields: %+v", id, row)
		}
	}
	if fresh := h.cardsOf(t, tutorA, domain.RelationshipInvited); len(fresh) != 0 {
		t.Fatalf("the purge gave tutor A new cards %+v", fresh)
	}
}

func ptr(text string) *string {
	return &text
}

func TestStudentPurgeGivesABlockedTutorANewCardWhoseLinkWorksOnlyWhileTheTutorIsActive(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.linkedStudent(t, token, newcomerEmail)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	h.at(t, authStart.Add(7*24*time.Hour))
	h.tick(t)
	if _, err := h.operators().Unblock(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	fresh := h.cardsOf(t, tutor, domain.RelationshipInvited)
	if len(fresh) != 1 {
		t.Fatalf("the blocked tutor's new cards are %+v", fresh)
	}
	link := linkToken(t, h.cardOf(t, h.relogin(t, tutorEmail), fresh[0].ID).Invitation)
	whileActive := h.invitationPage(t, link)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	whileBlocked := h.invitationPage(t, link)

	if h.userExists(t, newcomerEmail) || h.countRows(t, studentRelationships, student.String()) != 0 {
		t.Fatal("the unconfirmed account or a relationship naming it remains")
	}
	if removed := h.purgedRow(t, card.ID); removed.State != "discarded" || removed.DiscardReason == nil || *removed.DiscardReason != "removed" {
		t.Fatalf("the link of the blocked tutor is %+v", removed)
	}
	if fresh[0].Email == nil || *fresh[0].Email != newcomerEmail || fresh[0].DisplayName != cardName {
		t.Fatalf("the new card is %+v", fresh[0])
	}
	if whileActive.status != http.StatusOK || whileBlocked.status != http.StatusNotFound {
		t.Fatalf("the new link answered %d while the tutor was active and %d while blocked", whileActive.status, whileBlocked.status)
	}
}

func TestPurgeEndsAStillActiveRelationshipBeforeKeepingItAsHistory(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unconfirmed := h.seedStudentAccount(t, 2, domain.AccountUnverified)
	card := h.seedCard(t, tutor, 1, domain.RelationshipActive, &unconfirmed)
	h.newGroup(t, token, groupName, []string{card})
	purgedAt := authStart.Add(7 * 24 * time.Hour)

	h.at(t, purgedAt)
	h.tick(t)

	stored := h.purgedRow(t, card)
	if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "student_purged" || stored.StudentID != nil {
		t.Fatalf("the purged relationship is %+v", stored)
	}
	if stored.EndedBy == nil || *stored.EndedBy != "deletion" || !sameInstant(stored.EndedAt, purgedAt) {
		t.Fatalf("the purge did not end the relationship first: %+v", stored)
	}
	if h.membershipsOf(t, card) != 0 || h.endedEmailsQueued(t) != 0 {
		t.Fatal("the purge kept a membership or queued an ending email")
	}
	if logs := h.logs.String(); !strings.Contains(logs, relationshipEndedMsg) || !strings.Contains(logs, endedByDeletion) {
		t.Fatal("no relationship.ended record for the purge")
	}
}

func TestRestoreIsRefusedOnceThePurgeIsDue(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.deleteMe(t, token, accountPassword)
	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))

	_, err := h.operators().Restore(context.Background(), tutorEmail)

	if !errors.Is(err, operators.ErrImpossibleState) {
		t.Fatalf("a restore after the retention answered %v", err)
	}
	if user, err := h.store.Repositories().Users.ByEmail(context.Background(), tutorEmail); err != nil || user.State != domain.AccountDeleted {
		t.Fatalf("the refused restore changed the account: %+v %v", user, err)
	}
}

func TestOutboxRowOfAPurgedSubjectIsObsolete(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, secondStudent := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	h.linkedSignedIn(t, parties.tutorToken, secondStudent, "A second student", requestingEmail)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	h.deleteMe(t, parties.tutorToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.userExists(t, tutorEmail) {
		t.Fatal("the tutor was not purged")
	}
	if len(h.mailTo(t, studentEmail)) != 0 || len(h.mailTo(t, requestingEmail)) != 0 {
		t.Fatal("an email about a purged subject was sent")
	}
	if h.obsoleteRows(t, domain.EventRelationshipEnded) != 1 || h.obsoleteRows(t, domain.EventTutorDeleted) != 1 {
		t.Fatal("the rows of the purged subject were not marked obsolete")
	}
}

func TestUnblockIntoUnverifiedRestartsTheClockOfALinkedStudent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.linkedStudent(t, token, newcomerEmail)
	h.at(t, authStart.Add(5*24*time.Hour))
	if _, err := h.operators().Block(context.Background(), newcomerEmail); err != nil {
		t.Fatal(err)
	}
	unblockedAt := authStart.Add(6 * 24 * time.Hour)
	h.at(t, unblockedAt)
	if _, err := h.operators().Unblock(context.Background(), newcomerEmail); err != nil {
		t.Fatal(err)
	}

	h.at(t, authStart.Add(7*24*time.Hour+time.Hour))
	h.tick(t)
	keptAfterSevenDays := h.storedRelationship(t, card.ID)
	h.at(t, unblockedAt.Add(7*24*time.Hour))
	h.tick(t)

	if keptAfterSevenDays.State != "provisional" {
		t.Fatalf("seven days after sign-up the restarted link is %+v", keptAfterSevenDays)
	}
	if h.userExists(t, newcomerEmail) {
		t.Fatal("the account was not removed seven days after the unblock")
	}
	removed := h.storedRelationship(t, card.ID)
	if removed.State != "discarded" || removed.DiscardReason == nil || *removed.DiscardReason != "removed" {
		t.Fatalf("after the restarted clock the link is %+v", removed)
	}
	if fresh := h.cardsOf(t, tutor, domain.RelationshipInvited); len(fresh) != 1 || fresh[0].Email == nil || *fresh[0].Email != newcomerEmail {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
}

type preparedLink struct {
	send         func() <-chan pendingAnswer
	linked       func(t *testing.T, h *authHarness) bool
	reference    *response
	verifyEmails int64
}

const verifyEmailsQueued = "SELECT count(id) FROM outbox WHERE kind = $1"

func (h *authHarness) verifyEmails(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, verifyEmailsQueued, string(domain.EmailPurposeVerify.Event()))
}

func (h *authHarness) assertAnsweredLikeATakenEmail(t *testing.T, answer response, prepared preparedLink, hashed int) {
	t.Helper()
	if !sameAnswer(answer, *prepared.reference) {
		t.Fatalf("the refused sign-up answered %d %s %v, a taken email %d %s %v", answer.status, answer.body, answer.headers,
			prepared.reference.status, prepared.reference.body, prepared.reference.headers)
	}
	if hashed != 1 {
		t.Fatalf("the refused sign-up hashed %d passwords instead of one", hashed)
	}
	if queued := h.verifyEmails(t); queued != prepared.verifyEmails {
		t.Fatalf("%d confirmation emails are queued instead of %d", queued, prepared.verifyEmails)
	}
}

type racingLink struct {
	prepare func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string) preparedLink
	status  int
	code    string
}

func racingLinks() map[string]racingLink {
	return map[string]racingLink{
		"invitation sign-up": {
			prepare: func(t *testing.T, h *authHarness, _ domain.UserID, tutorToken string) preparedLink {
				t.Helper()
				link := linkToken(t, h.newCard(t, tutorToken, cardName, "").Invitation)
				h.seedUserAs(t, domain.RoleStudent, 5, "taken@example.org", accountPassword, currentHash)
				hashesBefore := h.hasher.Hashes()
				reference := h.send(t, http.MethodPost, invitationPath+link+"/accept", studentSignUp{Email: "taken@example.org"}.body(), signUpHeaders())
				if hashed := h.hasher.Hashes() - hashesBefore; hashed != 1 {
					t.Fatalf("the taken-email sign-up hashed %d passwords", hashed)
				}
				return preparedLink{
					send: func() <-chan pendingAnswer {
						return h.sendInBackground(http.MethodPost, invitationPath+link+"/accept", studentSignUp{Email: newcomerEmail}.body(), signUpHeaders())
					},
					linked: func(t *testing.T, h *authHarness) bool {
						t.Helper()
						return h.userExists(t, newcomerEmail)
					},
					reference: &reference,
				}
			},
			status: http.StatusAccepted,
		},
		"signed-in acceptance": {
			prepare: func(t *testing.T, h *authHarness, _ domain.UserID, tutorToken string) preparedLink {
				t.Helper()
				student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
				link := linkToken(t, h.newCard(t, tutorToken, cardName, studentEmail).Invitation)
				return preparedLink{
					send: func() <-chan pendingAnswer { return h.trySignedInAcceptance(token, link) },
					linked: func(t *testing.T, h *authHarness) bool {
						t.Helper()
						return h.linksOf(t, student) > 0
					},
				}
			},
			status: http.StatusConflict, code: "invitation_invalid",
		},
		"sign-up with a code": {
			prepare: func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) preparedLink {
				t.Helper()
				code := h.storedCode(t, tutor).Display()
				h.seedUserAs(t, domain.RoleStudent, 5, "taken@example.org", accountPassword, currentHash)
				hashesBefore := h.hasher.Hashes()
				reference := h.register(t, studentCodeSignUp("taken@example.org", code))
				if hashed := h.hasher.Hashes() - hashesBefore; hashed != 1 {
					t.Fatalf("the taken-email sign-up hashed %d passwords", hashed)
				}
				body := studentCodeSignUp(entrantEmail, code).body()
				return preparedLink{
					send: func() <-chan pendingAnswer {
						return h.sendInBackground(http.MethodPost, "/api/v1/auth/register", body, signUpHeaders())
					},
					linked: func(t *testing.T, h *authHarness) bool {
						t.Helper()
						return h.countRows(t, requestsOfEmail, entrantEmail) > 0
					},
					reference:    &reference,
					verifyEmails: 1,
				}
			},
			status: http.StatusAccepted,
		},
		"code entry": {
			prepare: func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) preparedLink {
				t.Helper()
				student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
				code := h.storedCode(t, tutor).Display()
				return preparedLink{
					send: func() <-chan pendingAnswer { return h.tryEnterCode(token, code) },
					linked: func(t *testing.T, h *authHarness) bool {
						t.Helper()
						return h.countRows(t, requestsOfStudent, student.String()) > 0
					},
				}
			},
			status: http.StatusUnprocessableEntity, code: "tutor_code_invalid",
		},
	}
}

type racingStateChange struct {
	start func(t *testing.T, h *authHarness, tutorToken string) <-chan error
}

func racingStateChanges() map[string]racingStateChange {
	return map[string]racingStateChange{
		"the operator's block": {start: func(_ *testing.T, h *authHarness, _ string) <-chan error {
			done := make(chan error, 1)
			go func() {
				_, err := h.operators().Block(context.Background(), tutorEmail)
				done <- err
			}()
			return done
		}},
		"the tutor's deletion": {start: func(_ *testing.T, h *authHarness, tutorToken string) <-chan error {
			done := make(chan error, 1)
			answer := h.deleteInBackground(tutorToken, accountPassword)
			go func() {
				got := <-answer
				if got.err == nil && got.result.status != http.StatusNoContent {
					got.err = fmt.Errorf("the deletion answered %d %s", got.result.status, got.result.body)
				}
				done <- got.err
			}()
			return done
		}},
	}
}

func awaitChange(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(pauseDeadline):
		t.Fatal("the state change did not finish")
	}
}

func TestTutorStateChangeRacingALinkHasOneOutcome(t *testing.T) {
	t.Parallel()
	for linkName, link := range racingLinks() {
		for changeName, change := range racingStateChanges() {
			t.Run(changeName+" before "+linkName, func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				prepared := link.prepare(t, h, tutor, tutorToken)
				release := h.holdTutorProfile(t, tutor)
				changed := change.start(t, h, tutorToken)
				h.waitForLockWaiters(t, 1)
				hashesBefore := h.hasher.Hashes()
				answer := prepared.send()
				h.waitForLockWaiters(t, 2)

				release()
				awaitChange(t, changed)
				refused := awaitAnswer(t, answer)
				hashed := h.hasher.Hashes() - hashesBefore

				if refused.status != link.status || (link.code != "" && code(t, refused) != link.code) {
					t.Fatalf("the link answered %d %s", refused.status, refused.body)
				}
				if prepared.reference != nil {
					h.assertAnsweredLikeATakenEmail(t, refused, prepared, hashed)
				}
				if prepared.linked(t, h) {
					t.Fatal("the link reached a tutor who was no longer active")
				}
			})
		}
	}
	t.Run("invitation sign-up before the operator's block", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		prepared := racingLinks()["invitation sign-up"].prepare(t, h, tutor, tutorToken)
		release := h.holdTutorProfile(t, tutor)
		answer := prepared.send()
		h.waitForLockWaiters(t, 1)
		changed := racingStateChanges()["the operator's block"].start(t, h, tutorToken)
		h.waitForLockWaiters(t, 2)

		release()
		accepted := awaitAnswer(t, answer)
		awaitChange(t, changed)

		if accepted.status != http.StatusAccepted || !prepared.linked(t, h) {
			t.Fatalf("the sign-up that took the lock first answered %d %s", accepted.status, accepted.body)
		}
		if user, err := h.store.Repositories().Users.ByID(context.Background(), tutor); err != nil || user.State != domain.AccountBlocked {
			t.Fatalf("the tutor is %+v %v after the block", user, err)
		}
	})
}

func TestTutorDeletionRacingTheOwnersReclaimChoiceHasOneOutcome(t *testing.T) {
	t.Parallel()
	for _, choiceFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("choice first %t", choiceFirst), func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			owner := h.reclaimed(t, newcomerEmail)
			choose := func() <-chan pendingAnswer {
				return h.sendInBackground(http.MethodPost, reclaimPath, reclaimChoice{DropLinks: []string{card.ID}}.body(t), jsonCall(owner))
			}
			release := h.holdTutorProfile(t, tutor)

			var choice <-chan pendingAnswer
			var deletion <-chan pendingAnswer
			if choiceFirst {
				choice = choose()
				h.waitForLockWaiters(t, 1)
				deletion = h.deleteInBackground(tutorToken, accountPassword)
			} else {
				deletion = h.deleteInBackground(tutorToken, accountPassword)
				h.waitForLockWaiters(t, 1)
				choice = choose()
			}
			h.waitForLockWaiters(t, 2)
			release()
			chosen, deleted := awaitAnswer(t, choice), awaitAnswer(t, deletion)

			if deleted.status != http.StatusNoContent || chosen.status != http.StatusOK || outcomeOf(t, chosen) != "" {
				t.Fatalf("the deletion answered %d %s and the choice %d %s", deleted.status, deleted.body, chosen.status, chosen.body)
			}
			stored := h.storedRelationship(t, card.ID)
			wantReason := "tutor_deleted"
			if choiceFirst {
				wantReason = "removed"
			}
			if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != wantReason {
				t.Fatalf("the link is %+v", stored)
			}
			if fresh := h.cardsOf(t, tutor, domain.RelationshipInvited); len(fresh) != 0 {
				t.Fatalf("an invited card of the deleted tutor remains: %+v", fresh)
			}
			if h.countRows(t, tutorOpenInvitations, tutor.String()) != 0 || h.linksOf(t, student) != 0 {
				t.Fatal("an open invitation or a link of the deleted tutor remains")
			}
			if choiceFirst && len(h.cardsOf(t, tutor, domain.RelationshipDiscarded)) != 2 {
				t.Fatal("the card the owner's drop created was not discarded by the deletion")
			}
		})
	}
}

type tutorWrite struct {
	method string
	path   func(cards tutorWriteCards) string
	body   func(cards tutorWriteCards) string
}

type tutorWriteCards struct {
	invited     string
	ended       string
	active      string
	open        string
	archived    string
	request     string
	provisional string
	blocked     domain.UserID
}

func tutorWrites() map[string]tutorWrite {
	return map[string]tutorWrite{
		"creating a card": {method: http.MethodPost, path: func(tutorWriteCards) string { return studentsPath },
			body: func(tutorWriteCards) string { return cardBody("A late card", "late@example.org") }},
		"changing a card's email": {method: http.MethodPatch, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.invited },
			body: func(tutorWriteCards) string { return `{"email":"changed@example.org"}` }},
		"regenerating an ended card's link": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.ended + "/invitation" },
			body: func(tutorWriteCards) string { return "" }},
		"sending a link": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.invited + "/invitation/send" },
			body: func(tutorWriteCards) string { return "" }},
		"creating a group": {method: http.MethodPost, path: func(tutorWriteCards) string { return groupsRoute },
			body: func(cards tutorWriteCards) string { return newGroupBody(groupName, []string{cards.active}) }},
		"renaming a group": {method: http.MethodPatch, path: func(cards tutorWriteCards) string { return groupsRoute + "/" + cards.open },
			body: func(tutorWriteCards) string { return `{"name":"Renamed group"}` }},
		"archiving a group": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return groupsRoute + "/" + cards.open + "/archive" },
			body: func(tutorWriteCards) string { return "" }},
		"unarchiving a group": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return groupsRoute + "/" + cards.archived + "/unarchive" },
			body: func(tutorWriteCards) string { return "" }},
		"regenerating the code": {method: http.MethodPost, path: func(tutorWriteCards) string { return "/api/v1/tutor/code/regenerate" },
			body: func(tutorWriteCards) string { return "" }},
		"accepting a join request": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return joinRequestsPath + "/" + cards.request + "/accept" },
			body: func(tutorWriteCards) string { return "" }},
		"declining a join request": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return joinRequestsPath + "/" + cards.request + "/decline" },
			body: func(tutorWriteCards) string { return `{"block":false}` }},
		"declining and blocking": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return joinRequestsPath + "/" + cards.request + "/decline" },
			body: func(tutorWriteCards) string { return `{"block":true}` }},
		"withdrawing a card": {method: http.MethodDelete, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.invited },
			body: func(tutorWriteCards) string { return "" }},
		"removing a provisional student": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.provisional + "/remove" },
			body: func(tutorWriteCards) string { return "" }},
		"ending a relationship": {method: http.MethodPost, path: func(cards tutorWriteCards) string { return studentsPath + "/" + cards.active + "/end" },
			body: func(tutorWriteCards) string { return "" }},
		"unblocking a student": {method: http.MethodDelete, path: func(cards tutorWriteCards) string { return blockedPath + "/" + cards.blocked.String() },
			body: func(tutorWriteCards) string { return "" }},
	}
}

const (
	tutorEndedByTutor       = "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid AND ended_by = 'tutor'"
	tutorRemovedOrWithdrawn = "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid AND discard_reason IN ('removed', 'withdrawn')"
)

type tutorWriteTrace struct {
	counts [8]int64
	code   domain.TutorCode
}

func (h *authHarness) tutorWriteTrace(t *testing.T, tutor domain.UserID) tutorWriteTrace {
	t.Helper()
	return tutorWriteTrace{
		counts: [8]int64{
			h.countRows(t, tutorRelationships, tutor.String()),
			h.countRows(t, tutorInvitations, tutor.String()),
			h.countRows(t, tutorGroups, tutor.String()),
			h.countRows(t, tutorDeletedQueued, string(domain.EventInvitationCreated)),
			h.countRows(t, tutorDecidedRequests, tutor.String()),
			h.countRows(t, tutorBlockedStudents, tutor.String()),
			h.countRows(t, tutorEndedByTutor, tutor.String()),
			h.countRows(t, tutorRemovedOrWithdrawn, tutor.String()),
		},
		code: h.storedCode(t, tutor),
	}
}

func TestTutorWriteAuthenticatedBeforeTheTutorStoppedBeingActiveIsRefused(t *testing.T) {
	t.Parallel()
	for writeName, write := range tutorWrites() {
		for changeName, change := range racingStateChanges() {
			t.Run(changeName+" before "+writeName, func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
				ended := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
				h.endAsTutor(t, tutorToken, ended.ID)
				_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
				active := h.linkedSignedIn(t, tutorToken, otherToken, "An active student", requestingEmail)
				cards := tutorWriteCards{invited: h.newCard(t, tutorToken, "An invited student", cardEmail).ID, ended: ended.ID, active: active.ID}
				cards.open = h.newGroup(t, tutorToken, "An open group", []string{active.ID}).ID
				cards.archived = h.newGroup(t, tutorToken, "An archived group", nil).ID
				_, requesterToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 4, entrantEmail)
				cards.request = requestText(h.enteredRequest(t, requesterToken, h.storedCode(t, tutor).Display()))
				provisional, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
				cards.provisional = provisional.ID
				blocked, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, "blocked@example.org")
				h.blockRequester(t, tutor, blocked)
				cards.blocked = blocked
				if archived := h.groupCall(t, tutorToken, http.MethodPost, cards.archived, "/archive", ""); archived.status != http.StatusOK {
					t.Fatalf("archiving answered %d %s", archived.status, archived.body)
				}
				unarchivedAfter := h.countRows(t, unarchivedGroupsQuery, tutor.String())
				if changeName == "the tutor's deletion" {
					unarchivedAfter = 0
				}
				release := h.holdTutorProfile(t, tutor)
				changed := change.start(t, h, tutorToken)
				h.waitForLockWaiters(t, 1)
				before := h.tutorWriteTrace(t, tutor)
				answer := h.sendInBackground(write.method, write.path(cards), write.body(cards), jsonCall(tutorToken))
				h.waitForLockWaiters(t, 2)

				release()
				awaitChange(t, changed)
				refused := awaitAnswer(t, answer)

				if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
					t.Fatalf("the write answered %d %s", refused.status, refused.body)
				}
				if after := h.tutorWriteTrace(t, tutor); after != before {
					t.Fatalf("the refused write changed cards, links, groups, emails, requests, blocks, endings, removals or the code: %+v then %+v", before, after)
				}
				if h.countRows(t, unarchivedGroupsQuery, tutor.String()) != unarchivedAfter || h.countRows(t, "SELECT count(id) FROM groups WHERE name = $1", "Renamed group") != 0 {
					t.Fatal("the refused write archived, unarchived or renamed a group")
				}
			})
		}
	}
}

type operatorTutorChange struct {
	prepare func(t *testing.T, h *authHarness, tutor domain.UserID)
	apply   func(service *operators.Service, ctx context.Context, email string) (domain.UserID, error)
	want    domain.AccountState
}

func operatorTutorChanges() map[string]operatorTutorChange {
	return map[string]operatorTutorChange{
		"block": {
			prepare: func(*testing.T, *authHarness, domain.UserID) {},
			apply:   (*operators.Service).Block,
			want:    domain.AccountBlocked,
		},
		"unblock": {
			prepare: func(t *testing.T, h *authHarness, _ domain.UserID) {
				t.Helper()
				if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
					t.Fatal(err)
				}
			},
			apply: (*operators.Service).Unblock,
			want:  domain.AccountActive,
		},
		"restore": {
			prepare: func(t *testing.T, h *authHarness, tutor domain.UserID) {
				t.Helper()
				if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), tutor, authStart); err != nil {
					t.Fatal(err)
				}
			},
			apply: (*operators.Service).Restore,
			want:  domain.AccountActive,
		},
	}
}

func TestOperatorStateChangesOfATutorTakeTheProfileLock(t *testing.T) {
	t.Parallel()
	for name, change := range operatorTutorChanges() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			change.prepare(t, h, tutor)
			release := h.holdTutorProfile(t, tutor)
			changed := make(chan error, 1)
			go func() {
				_, err := change.apply(h.operators(), context.Background(), tutorEmail)
				changed <- err
			}()
			h.waitForLockWaiters(t, 1)

			release()
			awaitChange(t, changed)

			if user, err := h.store.Repositories().Users.ByID(context.Background(), tutor); err != nil || user.State != change.want {
				t.Fatalf("the tutor is %+v %v after the change", user, err)
			}
		})
	}
}

func (h *authHarness) assertNothingOpenOfDeletedTutor(t *testing.T, tutor domain.UserID, student domain.UserID) {
	t.Helper()
	if invited := h.cardsOf(t, tutor, domain.RelationshipInvited); len(invited) != 0 {
		t.Fatalf("an invited card of the deleted tutor remains: %+v", invited)
	}
	if h.countRows(t, tutorOpenInvitations, tutor.String()) != 0 || h.linksOf(t, student) != 0 {
		t.Fatal("an open invitation or a link of the deleted tutor remains")
	}
}

func (h *authHarness) assertDiscarded(t *testing.T, tutor domain.UserID, card string, reason string, discarded int) {
	t.Helper()
	if stored := h.storedRelationship(t, card); stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != reason {
		t.Fatalf("the link is %+v instead of discarded as %s", stored, reason)
	}
	if cards := h.cardsOf(t, tutor, domain.RelationshipDiscarded); len(cards) != discarded {
		t.Fatalf("the tutor has %d discarded cards instead of %d: %+v", len(cards), discarded, cards)
	}
}

func TestTutorRemovalRacingTheTutorsDeletionHasOneOutcome(t *testing.T) {
	t.Parallel()
	for _, removalFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("removal first %t", removalFirst), func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			remove := func() <-chan pendingAnswer {
				return h.sendInBackground(http.MethodPost, studentsPath+"/"+card.ID+"/remove", "", jsonCall(tutorToken))
			}
			release := h.holdTutorProfile(t, tutor)
			var removal, deletion <-chan pendingAnswer
			if removalFirst {
				removal = remove()
				h.waitForLockWaiters(t, 1)
				deletion = h.deleteInBackground(tutorToken, accountPassword)
			} else {
				deletion = h.deleteInBackground(tutorToken, accountPassword)
				h.waitForLockWaiters(t, 1)
				removal = remove()
			}
			h.waitForLockWaiters(t, 2)

			release()
			removed, deleted := awaitAnswer(t, removal), awaitAnswer(t, deletion)

			if deleted.status != http.StatusNoContent {
				t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
			}
			if removalFirst {
				if removed.status != http.StatusOK {
					t.Fatalf("the removal that took the lock first answered %d %s", removed.status, removed.body)
				}
				h.assertDiscarded(t, tutor, card.ID, "removed", 2)
			} else {
				if removed.status != http.StatusUnauthorized || code(t, removed) != "unauthenticated" {
					t.Fatalf("the removal after the deletion answered %d %s", removed.status, removed.body)
				}
				h.assertDiscarded(t, tutor, card.ID, "tutor_deleted", 1)
			}
			h.assertNothingOpenOfDeletedTutor(t, tutor, student)
		})
	}
}

func TestStopStudyingRacingTheTutorsDeletionHasOneOutcome(t *testing.T) {
	t.Parallel()
	stop := func(h *authHarness, studentToken string, card string) <-chan pendingAnswer {
		return h.sendInBackground(http.MethodPost, tutorsPath+card+"/end", "", jsonCall(studentToken))
	}
	t.Run("committed while the deletion waits", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		studentToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		release := h.holdTutorProfile(t, tutor)
		deletion := h.deleteInBackground(tutorToken, accountPassword)
		h.waitForLockWaiters(t, 1)
		stopped := awaitAnswer(t, stop(h, studentToken, card.ID))

		release()
		deleted := awaitAnswer(t, deletion)

		if stopped.status != http.StatusNoContent || deleted.status != http.StatusNoContent {
			t.Fatalf("Stop studying answered %d %s and the deletion %d %s", stopped.status, stopped.body, deleted.status, deleted.body)
		}
		h.assertDiscarded(t, tutor, card.ID, "removed", 2)
		h.assertNothingOpenOfDeletedTutor(t, tutor, student)
	})
	t.Run("sent before the deletion and locked after it", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		studentToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		release := h.holdUserRow(t, student)
		pending := stop(h, studentToken, card.ID)
		h.waitForLockWaiters(t, 1)
		deleted := awaitAnswer(t, h.deleteInBackground(tutorToken, accountPassword))

		release()
		refused := awaitAnswer(t, pending)

		if deleted.status != http.StatusNoContent || refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Fatalf("the deletion answered %d %s and Stop studying %d %s", deleted.status, deleted.body, refused.status, refused.body)
		}
		h.assertDiscarded(t, tutor, card.ID, "tutor_deleted", 1)
		h.assertNothingOpenOfDeletedTutor(t, tutor, student)
	})
}

func TestTutorDeletionRecordsWhenEachOpenRequestEnded(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	closedStudent, closedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	overdueStudent, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	closed := h.closedByUnblock(t, tutor, tutorToken, closedStudent, closedToken)
	overdue := h.seedJoinRequest(t, tutor, overdueStudent, 1, domain.JoinRequestPending)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE join_requests SET expires_at = $1 WHERE id = $2", authStart, overdue); err != nil {
		t.Fatal(err)
	}
	h.advance(t, time.Hour)
	deletedAt := h.now(t)
	signedInAfterTheAdvance := h.relogin(t, tutorEmail)
	if closedUntil := h.requestDecidedAt(t, closed); closedUntil == nil || !closedUntil.After(deletedAt) {
		t.Fatalf("the request the unblock closed is decided at %v, not after the deletion", closedUntil)
	}

	deleted := awaitAnswer(t, h.deleteInBackground(signedInAfterTheAdvance, accountPassword))

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	if got, decidedAt := h.storedRequest(t, closed), h.requestDecidedAt(t, closed); got.State != "expired" || !sameInstant(decidedAt, deletedAt) {
		t.Fatalf("the request the unblock closed is %+v, decided at %v instead of the deletion time %v", got, decidedAt, deletedAt)
	}
	if got, decidedAt := h.storedRequest(t, overdue), h.requestDecidedAt(t, overdue); got.State != "expired" || !sameInstant(decidedAt, authStart) {
		t.Fatalf("the overdue request is %+v, decided at %v instead of its expiry %v", got, decidedAt, authStart)
	}
}

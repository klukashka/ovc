package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	cardRetention       = 90 * 24 * time.Hour
	emailErasedRecord   = `"msg":"card.email_erased"`
	erasureRefused      = `"msg":"invitation.email_erased_refused"`
	erasureFailed       = `"msg":"cards.erase_emails_failed"`
	neverIssuedToken    = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	removedPathCardName = "A seeded student"
	removedPathEmail    = "removed-path@example.org"
	removedPathNotes    = "Moves to the new card"
)

type storedRetention struct {
	State       string
	DisplayName *string
	Email       *string
	EmailSetAt  *time.Time
	ErasedAt    *time.Time
	Notes       string
}

func (h *authHarness) storedRetention(t *testing.T, card string) storedRetention {
	t.Helper()
	var row storedRetention
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT state, display_name, email::text, email_set_at, email_erased_at, notes FROM relationships WHERE id = $1::uuid", card,
	).Scan(&row.State, &row.DisplayName, &row.Email, &row.EmailSetAt, &row.ErasedAt, &row.Notes)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *authHarness) erasureRevocations(t *testing.T, card string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(*) FROM invitations WHERE relationship_id = $1::uuid AND revoke_reason = 'email_erased' AND token_encrypted IS NULL", card)
}

func (h *authHarness) retentionTutor(t *testing.T) (domain.UserID, string) {
	t.Helper()
	return h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
}

func (h *authHarness) erasedCard(t *testing.T, token string, name string, email string) (studentCard, string) {
	t.Helper()
	card := h.newCard(t, token, name, email)
	link := linkToken(t, card.Invitation)
	h.tickAt(t, h.now(t).Add(cardRetention))
	return card, link
}

func (h *authHarness) patchCard(t *testing.T, token string, card string, body string) studentCard {
	t.Helper()
	result := h.cardCall(t, token, http.MethodPatch, card, "", body)
	if result.status != http.StatusOK {
		t.Fatalf("changing the card answered %d %s", result.status, result.body)
	}
	return decodeCard(t, result)
}

func assertNotLogged(t *testing.T, logs string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(logs, value) {
			t.Fatalf("the logs contain %q", value)
		}
	}
}

func TestCardEmailErasedAfterNinetyDays(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	card := h.newCard(t, token, cardName, cardEmail)
	created := h.now(t)

	h.tickAt(t, created.Add(cardRetention-time.Hour))
	before := h.cardOf(t, h.relogin(t, tutorEmail), card.ID)
	h.tickAt(t, created.Add(cardRetention))
	after := h.cardOf(t, h.relogin(t, tutorEmail), card.ID)
	stored := h.storedRetention(t, card.ID)

	if before.Email == nil || *before.Email != cardEmail || before.EmailErased || before.Invitation == nil {
		t.Fatalf("one hour before the period ended the card was %+v", before)
	}
	if after.Email != nil || !after.EmailErased || after.Invitation != nil || after.DisplayName != cardName {
		t.Fatalf("after the period the card is %+v", after)
	}
	if stored.Email != nil || stored.EmailSetAt != nil || stored.ErasedAt == nil || !stored.ErasedAt.Equal(created.Add(cardRetention)) {
		t.Fatalf("the stored card is %+v", stored)
	}
	if h.erasureRevocations(t, card.ID) != 1 {
		t.Fatal("the open invitation was not revoked with its ciphertext cleared")
	}
	if strings.Count(h.logs.String(), emailErasedRecord) != 1 {
		t.Fatal("the erasure was not recorded once")
	}
	assertNotLogged(t, h.logs.String(), cardEmail, cardName)
}

func TestErasedLinkAnswersLikeRevoked(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	_, link := h.erasedCard(t, token, cardName, cardEmail)
	_, holderToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, cardEmail)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, "other-student@example.org")

	erasedPage := h.invitationPage(t, link)
	unknownPage := h.invitationPage(t, neverIssuedToken)
	signUps := []response{h.acceptInvitation(t, link, cardEmail), h.acceptInvitation(t, link, "someone-else@example.org")}
	signedIn := []response{h.acceptSignedIn(t, holderToken, link), h.acceptSignedIn(t, otherToken, link)}

	if erasedPage.status != http.StatusNotFound || refusal(t, erasedPage) != refusal(t, unknownPage) {
		t.Fatalf("the erased link's page answered %s instead of %s", refusal(t, erasedPage), refusal(t, unknownPage))
	}
	for index, answer := range append(signUps, signedIn...) {
		assertRefused(t, fmt.Sprintf("acceptance %d", index), answer, http.StatusConflict, "invitation_invalid")
	}
	if h.userExists(t, "someone-else@example.org") {
		t.Fatal("a sign-up through the erased link created an account")
	}
}

func TestErasedCardRefusesRegenerateAndSend(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "other-tutor@example.org")
	card, _ := h.erasedCard(t, token, cardName, cardEmail)
	token, otherToken := h.relogin(t, tutorEmail), h.relogin(t, "other-tutor@example.org")
	invitations, regenerations := h.invitationsOf(t, card.ID), h.regenerationsToday(t)

	regenerated := h.cardCall(t, token, http.MethodPost, card.ID, "/invitation", "")
	sent := h.sendInvitation(t, token, card.ID)
	outOfScope := h.cardCall(t, otherToken, http.MethodPost, card.ID, "/invitation", "")
	unchanged := h.invitationsOf(t, card.ID) == invitations && h.regenerationsToday(t) == regenerations
	entered := h.patchCard(t, token, card.ID, `{"email":"new-address@example.org"}`)
	enteredAt := h.now(t)
	regeneratedAfter := h.cardCall(t, token, http.MethodPost, card.ID, "/invitation", "")
	sentAfter := h.sendInvitation(t, token, card.ID)
	h.tickAt(t, enteredAt.Add(cardRetention-time.Hour))
	keptBeforeNewPeriod := h.storedRetention(t, card.ID)
	h.tickAt(t, enteredAt.Add(cardRetention))
	erasedAgain := h.storedRetention(t, card.ID)

	assertRefused(t, "regenerating", regenerated, http.StatusConflict, "email_erased")
	assertRefused(t, "sending", sent, http.StatusConflict, "email_erased")
	assertRefused(t, "another tutor's regeneration", outOfScope, http.StatusNotFound, "not_found")
	if !unchanged {
		t.Fatal("a refused regeneration or send changed the invitations or the counters")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, erasureRefused) || !strings.Contains(logs, `"action":"regenerate"`) || !strings.Contains(logs, `"action":"send"`) {
		t.Fatal("the refusals were not recorded with their action")
	}
	if entered.EmailErased || entered.Invitation == nil || entered.Invitation.URL == nil || entered.Email == nil {
		t.Fatalf("after entering an email the card is %+v", entered)
	}
	if regeneratedAfter.status != http.StatusOK || sentAfter.status != http.StatusAccepted {
		t.Fatalf("after entering an email regenerating answered %d and sending %d", regeneratedAfter.status, sentAfter.status)
	}
	if keptBeforeNewPeriod.Email == nil || keptBeforeNewPeriod.ErasedAt != nil || erasedAgain.Email != nil || erasedAgain.ErasedAt == nil {
		t.Fatalf("the new period kept %+v and then left %+v", keptBeforeNewPeriod, erasedAgain)
	}
	assertNotLogged(t, logs, cardEmail, "new-address@example.org")
}

func TestCardEmailErasureSkipsChangedCards(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	_, acceptedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, "accepted@example.org")
	accepted := h.linkedSignedIn(t, token, acceptedToken, "Accepted", "accepted@example.org")
	withdrawn := h.newCard(t, token, "Withdrawn", "withdrawn@example.org")
	renewed := h.newCard(t, token, "Renewed", "renewed@example.org")
	notesOnly := h.newCard(t, token, "Notes only", "notes-only@example.org")
	renamed := h.newCard(t, token, "Renamed", "renamed@example.org")
	sameEmail := h.newCard(t, token, "Same email", "same-email@example.org")
	withoutEmail := h.newCard(t, token, "Without email", "")
	created := h.now(t)
	h.mustAnswer(t, h.cardCall(t, token, http.MethodDelete, withdrawn.ID, "", ""), http.StatusNoContent)
	h.at(t, created.Add(cardRetention-24*time.Hour))
	token = h.relogin(t, tutorEmail)
	h.patchCard(t, token, renewed.ID, `{"email":"renewed-again@example.org"}`)
	h.patchCard(t, token, notesOnly.ID, `{"notes":"New notes"}`)
	h.patchCard(t, token, renamed.ID, `{"display_name":"Renamed again"}`)
	h.patchCard(t, token, sameEmail.ID, `{"email":"Same-Email@example.org"}`)

	h.tickAt(t, created.Add(cardRetention))

	if row := h.storedRetention(t, accepted.ID); row.State != "active" || row.Email == nil || row.ErasedAt != nil {
		t.Fatalf("the accepted card is %+v", row)
	}
	if row := h.storedRetention(t, withdrawn.ID); row.State != "discarded" || row.ErasedAt != nil {
		t.Fatalf("the withdrawn card is %+v", row)
	}
	if row := h.storedRetention(t, renewed.ID); row.Email == nil || *row.Email != "renewed-again@example.org" || row.ErasedAt != nil {
		t.Fatalf("the card with a newly entered email is %+v", row)
	}
	for _, card := range []studentCard{notesOnly, renamed, sameEmail} {
		if row := h.storedRetention(t, card.ID); row.Email != nil || row.ErasedAt == nil {
			t.Fatalf("a change that did not enter a new email restarted the period: %+v", row)
		}
	}
	if row := h.storedRetention(t, withoutEmail.ID); row.ErasedAt != nil || h.erasureRevocations(t, withoutEmail.ID) != 0 {
		t.Fatalf("the card without an email is %+v", row)
	}
}

func TestCardEmailErasureKeepsConcurrentEmail(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.retentionTutor(t)
	card := h.newCard(t, token, cardName, cardEmail)
	h.at(t, h.now(t).Add(cardRetention))
	token = h.relogin(t, tutorEmail)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id = $1::uuid FOR UPDATE", tutor.String())
	change := h.sendInBackground(http.MethodPatch, "/api/v1/students/"+card.ID, `{"email":"entered-meanwhile@example.org"}`, jsonCall(token))
	h.waitForLockWaiters(t, 1)
	ticked := make(chan error, 1)
	now := h.now(t)
	go func() { ticked <- h.worker.Tick(context.Background(), now) }()
	h.waitForLockWaiters(t, 2)

	release()
	changed := awaitAnswer(t, change)
	tickErr := <-ticked

	if tickErr != nil || changed.status != http.StatusOK {
		t.Fatalf("the change answered %d %s and the tick %v", changed.status, changed.body, tickErr)
	}
	row := h.storedRetention(t, card.ID)
	current := h.cardOf(t, token, card.ID)
	if row.Email == nil || *row.Email != "entered-meanwhile@example.org" || row.ErasedAt != nil || h.erasureRevocations(t, card.ID) != 0 {
		t.Fatalf("the job erased an email entered while it waited: %+v", row)
	}
	if current.Invitation == nil || current.Invitation.URL == nil || current.Invitation.Status != "open" {
		t.Fatalf("the link the change issued is %+v", current.Invitation)
	}
}

func TestQueuedInvitationEmailObsoleteAfterErasure(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	card := h.newCard(t, token, cardName, cardEmail)
	created := h.now(t)
	h.at(t, created.Add(cardRetention-30*time.Minute))
	h.mustAnswer(t, h.sendInvitation(t, h.relogin(t, tutorEmail), card.ID), http.StatusAccepted)

	h.tickAt(t, created.Add(cardRetention))

	if got := h.mailTo(t, cardEmail); len(got) != 0 {
		t.Fatalf("the erased address received %v", subjects(got))
	}
	if h.obsoleteRows(t, domain.EventInvitationCreated) != 1 || h.erasureRevocations(t, card.ID) != 1 {
		t.Fatal("the queued invitation email was not dropped as obsolete after the erasure")
	}
}

func TestCardEmailErasureIsolatesFailures(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	failing := h.newCard(t, token, "Failing card", "failing@example.org")
	erased := h.newCard(t, token, "Erased card", "erased@example.org")
	statement := fmt.Sprintf(`CREATE FUNCTION fail_card_erasure() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF NEW.id = '%s'::uuid AND NEW.email IS NULL THEN RAISE EXCEPTION 'fixture failure %%', OLD.email; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_card_erasure BEFORE UPDATE ON relationships FOR EACH ROW EXECUTE FUNCTION fail_card_erasure()`, failing.ID)
	if _, err := h.database.Pool.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}

	h.tickAt(t, h.now(t).Add(cardRetention))

	if row := h.storedRetention(t, failing.ID); row.Email == nil || row.ErasedAt != nil {
		t.Fatalf("the failing card is %+v", row)
	}
	if row := h.storedRetention(t, erased.ID); row.Email != nil || row.ErasedAt == nil {
		t.Fatalf("the other due card is %+v", row)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, erasureFailed) || !strings.Contains(logs, `"relationship_id":"`+failing.ID+`"`) {
		t.Fatal("the failed card was not reported")
	}
	if !strings.Contains(logs, `"job":"cards.erase_emails","result":"failed"`) {
		t.Fatal("the run was not recorded as failed")
	}
	assertNotLogged(t, logs, "failing@example.org", "erased@example.org", "Failing card", "fixture failure")
}

func TestWithdrawAndMergeEraseCardData(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, "merge-student@example.org")
	withdrawn := h.newCard(t, token, "Withdrawn card", "withdrawn@example.org")
	h.patchCard(t, token, withdrawn.ID, `{"notes":"Withdrawn notes"}`)
	kept := h.linkedSignedIn(t, token, studentToken, "Kept card", "merge-student@example.org")
	h.patchCard(t, token, kept.ID, `{"notes":"Kept notes"}`)
	merged := h.newCard(t, token, "Merged card", "")
	h.patchCard(t, token, merged.ID, `{"notes":"Merged notes"}`)

	h.mustAnswer(t, h.cardCall(t, token, http.MethodDelete, withdrawn.ID, "", ""), http.StatusNoContent)
	repeated := h.cardCall(t, token, http.MethodDelete, withdrawn.ID, "", "")
	h.mustAnswer(t, h.acceptSignedIn(t, studentToken, linkToken(t, merged.Invitation)), http.StatusOK)

	for label, card := range map[string]string{"withdrawn": withdrawn.ID, "merged": merged.ID} {
		row := h.storedRetention(t, card)
		if row.State != "discarded" || row.DisplayName != nil || row.Email != nil || row.EmailSetAt != nil || row.ErasedAt != nil || row.Notes != "" {
			t.Fatalf("the %s card keeps %+v", label, row)
		}
	}
	if repeated.status != http.StatusNoContent {
		t.Fatalf("repeating the withdrawal answered %d", repeated.status)
	}
	if row := h.storedRetention(t, kept.ID); row.Notes != "Kept notes" || row.DisplayName == nil || *row.DisplayName != "Kept card" {
		t.Fatalf("the surviving card is %+v", row)
	}
	labels := map[string]bool{}
	for _, item := range h.notificationsOf(t, token) {
		if item.Event == "invitation.accepted" && item.Label != nil {
			labels[*item.Label] = true
		}
	}
	if !labels["Kept card"] || len(labels) != 1 {
		t.Fatalf("the acceptance rows are labelled %v instead of the surviving card's name", labels)
	}
	assertNotLogged(t, h.logs.String(), "Withdrawn notes", "Merged notes", "Withdrawn card", "Merged card")
}

type removalFixture struct {
	h            *authHarness
	tutor        domain.UserID
	tutorToken   string
	student      domain.UserID
	studentToken string
	card         string
}

func newRemovalFixture(t *testing.T, path string) removalFixture {
	t.Helper()
	h := newAuthHarness(t)
	tutor, token := h.retentionTutor(t)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.exec2(t, "UPDATE relationships SET email = $2, email_set_at = created_at, notes = 'Moves to the new card' WHERE id = $1::uuid", card, removedPathEmail)
	f := removalFixture{h: h, tutor: tutor, tutorToken: token, student: domain.UserID{10}, studentToken: h.studentToken(t, 10), card: card}
	h.setRelationshipState(t, card, domain.RelationshipProvisional)
	if strings.Contains(path, "reclaim") || path == "keep treated as drop" {
		h.setRelationshipState(t, card, domain.RelationshipReclaimPending)
	}
	if strings.Contains(path, "purge") {
		h.exec2(t, "UPDATE users SET state = 'unverified', email_verified_at = NULL, unverified_since = $2 WHERE id = $1::uuid",
			f.student.String(), h.now(t).Add(-7*24*time.Hour).Format(time.RFC3339Nano))
		f.studentToken = h.studentToken(t, 10)
	}
	return f
}

func (f removalFixture) remove(t *testing.T, path string) {
	t.Helper()
	h := f.h
	switch path {
	case "tutor removal":
		h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	case "stop studying":
		h.mustAnswer(t, h.stopStudying(t, f.studentToken, f.card), http.StatusNoContent)
	case "reclaim drop":
		h.mustAnswer(t, h.decideReclaim(t, f.studentToken, reclaimChoice{DropLinks: []string{f.card}}), http.StatusOK)
	case "keep treated as drop":
		if _, err := h.store.Repositories().Users.Block(t.Context(), f.tutor, h.now(t)); err != nil {
			t.Fatal(err)
		}
		h.mustAnswer(t, h.decideReclaim(t, f.studentToken, reclaimChoice{KeepLinks: []string{f.card}}), http.StatusOK)
	case "reclaim expiry":
		h.exec2(t, "UPDATE users SET reclaim_until = $2 WHERE id = $1::uuid", f.student.String(), h.now(t).Add(-time.Second).Format(time.RFC3339Nano))
		if _, err := h.tutoring.ExpireReclaims(t.Context(), h.now(t)); err != nil {
			t.Fatal(err)
		}
	case "seven day purge":
		if _, err := h.purger.CleanupUnverified(t.Context(), h.now(t)); err != nil {
			t.Fatal(err)
		}
	case "immediate purge", "student deletion provisional", "student deletion reclaim":
		h.mustAnswer(t, h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
	}
}

func (h *authHarness) onlyNewCard(t *testing.T, tutor domain.UserID) string {
	t.Helper()
	fresh := h.cardsOf(t, tutor, domain.RelationshipInvited)
	if len(fresh) != 1 {
		t.Fatalf("the removal left %d new cards", len(fresh))
	}
	return fresh[0].ID
}

func TestRemovalErasesDisplayName(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"tutor removal", "stop studying", "reclaim drop", "keep treated as drop", "reclaim expiry", "seven day purge", "immediate purge", "student deletion provisional", "student deletion reclaim"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			f := newRemovalFixture(t, path)
			h := f.h
			removedAt := h.now(t)

			f.remove(t, path)
			fresh := h.onlyNewCard(t, f.tutor)
			discarded := h.storedRetention(t, f.card)
			replacement := h.storedRetention(t, fresh)
			h.tickAt(t, removedAt.Add(cardRetention-time.Hour))
			keptBeforePeriod := h.storedRetention(t, fresh)
			h.tickAt(t, removedAt.Add(cardRetention))
			afterPeriod := h.storedRetention(t, fresh)

			if discarded.State != "discarded" || discarded.DisplayName != nil || discarded.Email != nil || discarded.EmailSetAt != nil || discarded.Notes != "" {
				t.Fatalf("the removed row keeps %+v", discarded)
			}
			if replacement.DisplayName == nil || *replacement.DisplayName != removedPathCardName || replacement.Email == nil || *replacement.Email != removedPathEmail ||
				replacement.Notes != removedPathNotes || replacement.EmailSetAt == nil || !replacement.EmailSetAt.Equal(removedAt) {
				t.Fatalf("the new card is %+v", replacement)
			}
			if keptBeforePeriod.Email == nil || afterPeriod.Email != nil || afterPeriod.ErasedAt == nil {
				t.Fatalf("the new card's period kept %+v and then left %+v", keptBeforePeriod, afterPeriod)
			}
			assertNotLogged(t, h.logs.String(), removedPathEmail, removedPathCardName, removedPathNotes)
		})
	}
}

type tutorLessonAudience struct {
	Audience struct {
		Label     *string `json:"label"`
		LabelKind *string `json:"label_kind"`
	} `json:"audience"`
}

type cardChannel struct {
	ChannelID *string `json:"channel_id"`
}

func TestErasedNameLabels(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	card, _ := h.squat(t, token, cardEmail, cardEmail)
	studentToken := accessToken(t, h.login(t, cardEmail, newcomerPassword))
	lesson := h.lessonWith(t, token, card.ID)
	var channel cardChannel
	if err := json.Unmarshal([]byte(h.cardCall(t, token, http.MethodGet, card.ID, "", "").body), &channel); err != nil || channel.ChannelID == nil {
		t.Fatal("the provisional card has no direct channel")
	}
	h.mustAnswer(t, h.sendMessage(t, studentToken, *channel.ChannelID, "Hello from the provisional account", nil, ""), http.StatusCreated)
	h.tickAt(t, time.Date(2030, time.March, 13, 9, 0, 0, 0, time.UTC))
	token = h.relogin(t, tutorEmail)
	mailBeforeRemoval := len(h.mailTo(t, tutorEmail))

	h.mustAnswer(t, h.removeStudent(t, token, card.ID), http.StatusOK)
	h.tickAt(t, h.now(t).Add(24*time.Hour))
	token = h.relogin(t, tutorEmail)
	notifications := h.send(t, http.MethodGet, notificationsPath+"?limit=100", "", jsonCall(token))
	lessonAnswer := h.send(t, http.MethodGet, lessonsRoute+"/"+lesson, "", jsonCall(token))
	channels, page := h.channelList(t, token, "")
	searched, found := h.channelList(t, token, "?q=Maria")
	detail, _ := h.channelCall(t, token, *channel.ChannelID)

	var accepted []notificationItem
	for _, item := range h.notificationsOf(t, token) {
		if item.Event == "invitation.accepted" {
			accepted = append(accepted, item)
		}
	}
	if len(accepted) != 1 || accepted[0].Label != nil || accepted[0].LabelKind == nil || *accepted[0].LabelKind != "removed_student" {
		t.Fatalf("the acceptance rows after the removal are %+v", accepted)
	}
	var audience tutorLessonAudience
	if err := json.Unmarshal([]byte(lessonAnswer.body), &audience); err != nil || lessonAnswer.status != http.StatusOK {
		t.Fatalf("the past lesson answered %d %s", lessonAnswer.status, lessonAnswer.body)
	}
	if audience.Audience.Label != nil || audience.Audience.LabelKind == nil || *audience.Audience.LabelKind != "removed_student" {
		t.Fatalf("the past lesson's audience is %+v", audience.Audience)
	}
	if channels.status != http.StatusOK || len(page.Items) != 0 || searched.status != http.StatusOK || len(found.Items) != 0 || detail.status != http.StatusNotFound {
		t.Fatalf("the removed link's channel is still reachable: list %d, search %d, detail %d", len(page.Items), len(found.Items), detail.status)
	}
	bodies := []string{notifications.body, lessonAnswer.body, channels.body, searched.body}
	for _, message := range h.mailTo(t, tutorEmail)[mailBeforeRemoval:] {
		bodies = append(bodies, message.Subject, message.Text, message.HTML)
	}
	for _, body := range bodies {
		if strings.Contains(body, studentAccountName) || strings.Contains(body, cardName) {
			t.Fatalf("a tutor-facing answer or email names the removed student: %s", body)
		}
	}
}

func TestTutorDeletionClearsErasureMarker(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	card, _ := h.erasedCard(t, token, cardName, cardEmail)

	h.mustAnswer(t, h.deleteMe(t, h.relogin(t, tutorEmail), accountPassword), http.StatusNoContent)

	if row := h.storedRetention(t, card.ID); row.State != "discarded" || row.ErasedAt != nil {
		t.Fatalf("the erased card after the tutor's deletion is %+v", row)
	}
}

func TestPurgeAfterRetentionMigration(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.retentionTutor(t)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, "purged-student@example.org")
	card := h.linkedSignedIn(t, token, studentToken, "Purged student", "purged-student@example.org")
	h.mustAnswer(t, h.deleteMe(t, studentToken, accountPassword), http.StatusNoContent)

	h.tickAt(t, h.now(t).Add(31*24*time.Hour))

	if h.userExists(t, "purged-student@example.org") || h.countRows(t, "SELECT count(*) FROM users WHERE id = $1::uuid", student.String()) != 0 {
		t.Fatal("the purge did not run")
	}
	if row := h.storedRetention(t, card.ID); row.Email != nil || row.EmailSetAt != nil {
		t.Fatalf("the purged student's card keeps %+v", row)
	}
}

package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	studentAccountName  = "Maria Account Name"
	invitationAccepted  = `"msg":"invitation.accepted"`
	relationshipRemoved = `"msg":"relationship.removed"`
	otherEmail          = "someone-else@example.org"
)

type studentSignUp struct {
	Email string
}

func (form studentSignUp) body() string {
	return fmt.Sprintf(`{"name":%s,"email":%s,"password":%s,"language":"en","timezone":"Europe/Moscow","consent":true}`,
		jsonText(studentAccountName), jsonText(form.Email), jsonText(newcomerPassword))
}

func (h *authHarness) acceptInvitation(t *testing.T, link string, email string) response {
	t.Helper()
	return h.postJSON(t, invitationPath+link+"/accept", studentSignUp{Email: email}.body(), nil)
}

type storedLink struct {
	State         string
	StudentID     *string
	Email         *string
	Notes         string
	DiscardReason *string
}

func (h *authHarness) storedRelationship(t *testing.T, id string) storedLink {
	t.Helper()
	var row storedLink
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT state, student_id::text, email::text, notes, discard_reason FROM relationships WHERE id = $1::uuid", id,
	).Scan(&row.State, &row.StudentID, &row.Email, &row.Notes, &row.DiscardReason)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *authHarness) cardsOf(t *testing.T, tutor domain.UserID, state domain.RelationshipState) []studentCard {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT id::text, state, display_name, email::text, notes FROM relationships WHERE tutor_id = $1 AND state = $2 ORDER BY created_at, id",
		tutor, string(state))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cards []studentCard
	for rows.Next() {
		var card studentCard
		if err := rows.Scan(&card.ID, &card.State, &card.DisplayName, &card.Email, &card.Notes); err != nil {
			t.Fatal(err)
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return cards
}

func (h *authHarness) linksOf(t *testing.T, student domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM relationships WHERE student_id = $1::uuid AND state <> 'discarded'", student.String())
}

func (h *authHarness) mismatchCount(t *testing.T, link string) int16 {
	t.Helper()
	var count int16
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT mismatch_count FROM invitations WHERE token_hash = $1", tokenHash(t, link)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) userExists(t *testing.T, email string) bool {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM users WHERE email = $1", email) > 0
}

func (h *authHarness) withNotes(t *testing.T, token string, card string, notes string) {
	t.Helper()
	if updated := h.cardCall(t, token, http.MethodPatch, card, "", fmt.Sprintf(`{"notes":%s}`, jsonText(notes))); updated.status != http.StatusOK {
		t.Fatalf("adding notes answered %d %s", updated.status, updated.body)
	}
}

func (h *authHarness) linkedStudent(t *testing.T, tutorToken string, email string) (studentCard, domain.UserID) {
	t.Helper()
	card := h.newCard(t, tutorToken, cardName, email)
	if accepted := h.acceptInvitation(t, linkToken(t, card.Invitation), newcomerEmail); accepted.status != http.StatusAccepted {
		t.Fatalf("the sign-up answered %d %s", accepted.status, accepted.body)
	}
	return card, h.userID(t, newcomerEmail)
}

func (h *authHarness) removeStudent(t *testing.T, token string, card string) response {
	t.Helper()
	return h.cardCall(t, token, http.MethodPost, card, "/remove", "")
}

type removal struct {
	NewCard studentCard `json:"new_card"`
}

type pendingAnswer struct {
	result response
	err    error
}

func (h *authHarness) sendInBackground(method string, path string, body string, headers map[string]string) <-chan pendingAnswer {
	answer := make(chan pendingAnswer, 1)
	go func() {
		result, err := h.trySend(method, path, body, headers)
		answer <- pendingAnswer{result: result, err: err}
	}()
	return answer
}

func awaitAnswer(t *testing.T, answer <-chan pendingAnswer) response {
	t.Helper()
	select {
	case got := <-answer:
		if got.err != nil {
			t.Fatal(got.err)
		}
		return got.result
	case <-time.After(pauseDeadline):
		t.Fatal("the request did not answer")
	}
	return response{}
}

type contestedRow struct {
	holder   int32
	relation uint32
	page     int32
	tuple    int16
}

const lockedRowPosition = "tableoid::oid, (ctid::text::point)[0]::int4, (ctid::text::point)[1]::int2"

const contestedRowWaiters = `
WITH RECURSIVE waiting(pid) AS (
    SELECT direct.pid FROM pg_locks AS direct
    WHERE NOT direct.granted AND (
        (direct.locktype = 'transactionid' AND direct.transactionid IN (
            SELECT held.transactionid FROM pg_locks AS held WHERE held.pid = $1 AND held.locktype = 'transactionid' AND held.granted))
        OR (direct.locktype = 'tuple' AND direct.database = (SELECT oid FROM pg_database WHERE datname = current_database())
            AND direct.relation = $2 AND direct.page = $3 AND direct.tuple = $4))
    UNION
    SELECT queued.pid FROM pg_locks AS queued JOIN waiting ON waiting.pid = ANY (pg_blocking_pids(queued.pid))
    WHERE NOT queued.granted
)
SELECT count(pid) FROM waiting`

func (h *authHarness) holdRow(t *testing.T, lockQuery string, key string) func() {
	t.Helper()
	holder, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if released {
			return
		}
		if err := holder.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var row contestedRow
	if err := holder.QueryRow(context.Background(), lockQuery, key).Scan(&row.relation, &row.page, &row.tuple); err != nil {
		t.Fatal(err)
	}
	if err := holder.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&row.holder); err != nil {
		t.Fatal(err)
	}
	h.contested = &row
	return func() {
		released = true
		if err := holder.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *authHarness) holdUserRow(t *testing.T, user domain.UserID) func() {
	t.Helper()
	return h.holdRow(t, "SELECT "+lockedRowPosition+" FROM users WHERE id = $1::uuid FOR UPDATE", user.String())
}

func (h *authHarness) waitForLockWaiters(t *testing.T, expected int64) {
	t.Helper()
	if h.contested == nil {
		t.Fatal("no row is held, so no waiter can be counted")
	}
	row := *h.contested
	deadline := time.NewTimer(pauseDeadline)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var waiting int64
		err := h.database.Pool.QueryRow(context.Background(), contestedRowWaiters, row.holder, row.relation, row.page, row.tuple).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= expected {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("%d requests wait on the held row, directly or behind another waiter, instead of %d", waiting, expected)
		case <-poll.C:
		}
	}
}

func decodeRemoval(t *testing.T, result response) removal {
	t.Helper()
	var decoded removal
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.NewCard.ID == "" {
		t.Fatalf("not a removal: %d %s", result.status, result.body)
	}
	return decoded
}

func signUpHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json", "Origin": publicOrigin, "X-Forwarded-For": homeAddress}
}

func TestInvitationSignUpAnswersIdenticallyForNewAndExistingEmail(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.seedUserAs(t, domain.RoleStudent, 2, "taken@example.org", "correct horse battery", currentHash)
	freshCard := h.newCard(t, tutor, "A new student", "")
	takenCard := h.newCard(t, tutor, "A known student", "")
	fresh, taken := linkToken(t, freshCard.Invitation), linkToken(t, takenCard.Invitation)
	before := h.hasher.Hashes()

	newAnswer := h.acceptInvitation(t, fresh, newcomerEmail)
	afterNew := h.hasher.Hashes()
	takenAnswer := h.acceptInvitation(t, taken, "taken@example.org")
	afterTaken := h.hasher.Hashes()
	h.tick(t)

	if newAnswer.status != http.StatusAccepted || strings.TrimSpace(newAnswer.body) != "{}" || !sameAnswer(newAnswer, takenAnswer) {
		t.Fatalf("new %d %q %v, taken %d %q %v", newAnswer.status, newAnswer.body, newAnswer.headers, takenAnswer.status, takenAnswer.body, takenAnswer.headers)
	}
	if newAnswer.headers.Get("Cache-Control") != "no-store" || newAnswer.headers.Get("Set-Cookie") != "" {
		t.Fatalf("the answer carries %v", newAnswer.headers)
	}
	if afterNew-before != 1 || afterTaken-afterNew != 1 {
		t.Fatalf("hashes: %d for the new email and %d for the taken one", afterNew-before, afterTaken-afterNew)
	}
	if got := subjects(h.mailTo(t, newcomerEmail)); !slices.Equal(got, []string{confirmSubject}) {
		t.Fatalf("the newcomer received %v", got)
	}
	if got := subjects(h.mailTo(t, "taken@example.org")); !slices.Equal(got, []string{duplicateSubject}) {
		t.Fatalf("the existing account received %v", got)
	}
	if page := h.invitationPage(t, taken); page.status != http.StatusOK {
		t.Fatalf("the link tried with a taken email answers %d", page.status)
	}
	if page := h.invitationPage(t, fresh); page.status != http.StatusNotFound {
		t.Fatalf("the used link answers %d", page.status)
	}
	linked := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, freshCard.ID, "", ""))
	if linked.State != "provisional" || linked.Account == nil || linked.Account.Name != studentAccountName || linked.Account.EmailConfirmed || linked.Invitation != nil {
		t.Fatalf("the linked card is %+v", linked)
	}
	if untouched := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, takenCard.ID, "", "")); untouched.State != "invited" || untouched.Account != nil {
		t.Fatalf("the card tried with a taken email is %+v", untouched)
	}
}

func TestCopiedAndEmailedLinksBehaveIdentically(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	copiedCard := h.newCard(t, tutor, "Copied", "copied@example.org")
	emailedCard := h.newCard(t, tutor, "Emailed", "emailed@example.org")
	copied := linkToken(t, copiedCard.Invitation)
	if sent := h.sendInvitation(t, tutor, emailedCard.ID); sent.status != http.StatusAccepted {
		t.Fatalf("sending answered %d %s", sent.status, sent.body)
	}
	h.tick(t)
	links := invitationLinks(t, h.mailTo(t, "emailed@example.org"))
	if len(links) != 1 {
		t.Fatalf("the emailed links are %v", links)
	}
	emailed := strings.TrimPrefix(links[0], publicOrigin+"/invite/")

	copiedAnswer := h.postJSON(t, invitationPath+copied+"/accept", studentSignUp{Email: "copied@example.org"}.body(), nil)
	emailedAnswer := h.postJSON(t, invitationPath+emailed+"/accept", studentSignUp{Email: "emailed@example.org"}.body(), nil)

	if !sameAnswer(copiedAnswer, emailedAnswer) || copiedAnswer.status != http.StatusAccepted {
		t.Fatalf("copied %d %s, emailed %d %s", copiedAnswer.status, copiedAnswer.body, emailedAnswer.status, emailedAnswer.body)
	}
	for _, card := range []studentCard{copiedCard, emailedCard} {
		if linked := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", "")); linked.State != "provisional" {
			t.Fatalf("the card %s is %s", card.DisplayName, linked.State)
		}
	}
	for _, link := range []string{copied, emailed} {
		if page := h.invitationPage(t, link); page.status != http.StatusNotFound || h.ciphertextKept(t, link) {
			t.Fatalf("a used link answers %d", page.status)
		}
	}
}

func TestEmailBoundCardRefusesAnotherEmail(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, cardEmail)
	link := linkToken(t, card.Invitation)

	refused := h.acceptInvitation(t, link, otherEmail)
	afterRefusal := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))
	accepted := h.acceptInvitation(t, link, " Pupil@Example.org ")

	if refused.status != http.StatusConflict || code(t, refused) != "invitation_email_mismatch" || refused.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("another email answered %d %s", refused.status, refused.body)
	}
	if h.userExists(t, otherEmail) || afterRefusal.State != "invited" || afterRefusal.Invitation == nil || afterRefusal.Invitation.Status != "open" {
		t.Fatalf("the refusal changed the card: %+v", afterRefusal)
	}
	if accepted.status != http.StatusAccepted {
		t.Fatalf("the card's email answered %d %s", accepted.status, accepted.body)
	}
	if linked := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", "")); linked.State != "provisional" || linked.Account == nil || linked.Account.Email != cardEmail {
		t.Fatalf("the card is %+v", linked)
	}
}

func TestFiveMismatchesRevokeTheLink(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, cardEmail)
	link := linkToken(t, card.Invitation)

	var mismatches []response
	for attempt := range 5 {
		mismatches = append(mismatches, h.acceptInvitation(t, link, fmt.Sprintf("wrong-%d@example.org", attempt)))
	}
	sixth := h.acceptInvitation(t, link, cardEmail)
	revoked := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))

	for index, refused := range mismatches {
		if refused.status != http.StatusConflict || code(t, refused) != "invitation_email_mismatch" {
			t.Fatalf("mismatch %d answered %d %s", index+1, refused.status, refused.body)
		}
	}
	if sixth.status != http.StatusConflict || code(t, sixth) != "invitation_invalid" || h.userExists(t, cardEmail) {
		t.Fatalf("the attempt after five mismatches answered %d %s", sixth.status, sixth.body)
	}
	if revoked.State != "invited" || revoked.Invitation == nil || revoked.Invitation.Status != "revoked_after_mismatches" || revoked.Invitation.URL != nil {
		t.Fatalf("the card shows %+v", revoked.Invitation)
	}
	if h.revokeReason(t, link) != "mismatch_limit" || h.ciphertextKept(t, link) || h.mismatchCount(t, link) != 5 {
		t.Fatal("the link was not revoked for the mismatch limit")
	}
	logs := h.logs.String()
	if strings.Count(logs, `"level":"WARN","msg":"invitation.revoked_after_mismatches"`) != 1 || strings.Count(logs, `"reason":"email_mismatch"`) != 5 {
		t.Fatalf("expected one revocation warning and five mismatch refusals")
	}
}

func TestMismatchIsCountedBeforeHashing(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := linkToken(t, h.newCard(t, tutor, cardName, cardEmail).Invitation)
	before := h.hasher.Hashes()

	refused := h.acceptInvitation(t, link, otherEmail)

	if refused.status != http.StatusConflict || code(t, refused) != "invitation_email_mismatch" {
		t.Fatalf("the mismatch answered %d %s", refused.status, refused.body)
	}
	if hashed := h.hasher.Hashes() - before; hashed != 0 || h.mismatchCount(t, link) != 1 {
		t.Fatalf("%d hashes and %d counted mismatches", hashed, h.mismatchCount(t, link))
	}
}

func TestForwardedLinkUsedOnceCannotBeUsedAgain(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, "")
	link := linkToken(t, card.Invitation)

	first := h.acceptInvitation(t, link, newcomerEmail)
	before := h.hasher.Hashes()
	second := h.acceptInvitation(t, link, otherEmail)
	page := h.invitationPage(t, link)

	if first.status != http.StatusAccepted {
		t.Fatalf("the first sign-up answered %d %s", first.status, first.body)
	}
	if second.status != http.StatusConflict || code(t, second) != "invitation_invalid" || h.userExists(t, otherEmail) || h.hasher.Hashes() != before {
		t.Fatalf("the second sign-up answered %d %s", second.status, second.body)
	}
	if page.status != http.StatusNotFound {
		t.Fatalf("the used link's page answered %d", page.status)
	}
	if linked := h.storedRelationship(t, card.ID); linked.StudentID == nil || *linked.StudentID != h.userID(t, newcomerEmail).String() {
		t.Fatal("the card is not linked to the first account")
	}
}

func TestConcurrentAcceptancesLinkOnce(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, "")
	link := linkToken(t, card.Invitation)
	const attempts = 6
	start := make(chan struct{})
	answers := make([]<-chan pendingAnswer, 0, attempts)
	for attempt := range attempts {
		headers := signUpHeaders()
		headers["X-Forwarded-For"] = fmt.Sprintf("198.51.100.%d", 30+attempt)
		body := studentSignUp{Email: fmt.Sprintf("racer-%d@example.org", attempt)}.body()
		answer := make(chan pendingAnswer, 1)
		go func() {
			<-start
			result, err := h.trySend(http.MethodPost, invitationPath+link+"/accept", body, headers)
			answer <- pendingAnswer{result: result, err: err}
		}()
		answers = append(answers, answer)
	}

	close(start)
	statuses := map[string]int{}
	for _, answer := range answers {
		result := awaitAnswer(t, answer)
		key := fmt.Sprint(result.status)
		if result.status != http.StatusAccepted {
			key += " " + code(t, result)
		}
		statuses[key]++
	}

	if statuses["202"] == 0 || statuses["202"]+statuses["409 invitation_invalid"] != attempts {
		t.Fatalf("statuses %v", statuses)
	}
	if students := h.countRows(t, "SELECT count(id) FROM users WHERE role = $1", "student"); students != 1 {
		t.Fatalf("%d student accounts exist", students)
	}
	if linked := h.cardsOf(t, tutorID, domain.RelationshipProvisional); len(linked) != 1 || linked[0].ID != card.ID {
		t.Fatalf("provisional cards %v", linked)
	}
}

func TestBlockedTutorsLinkIsInvalid(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := linkToken(t, h.newCard(t, tutor, cardName, "").Invitation)
	if _, err := h.store.Repositories().Users.Block(context.Background(), tutorID, authStart); err != nil {
		t.Fatal(err)
	}
	before := h.hasher.Hashes()

	refused := h.acceptInvitation(t, link, newcomerEmail)

	if refused.status != http.StatusConflict || code(t, refused) != "invitation_invalid" {
		t.Fatalf("a blocked tutor's link answered %d %s", refused.status, refused.body)
	}
	if h.userExists(t, newcomerEmail) || h.hasher.Hashes() != before {
		t.Fatal("a blocked tutor's link created an account or hashed a password")
	}
}

func TestConfirmationInTheOwnBrowserActivatesTheLink(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.linkedStudent(t, tutor, newcomerEmail)
	signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)

	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q}`, token), signedIn)
	active := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))

	if confirmed.status != http.StatusOK || strings.TrimSpace(confirmed.body) != `{"outcome":"linked"}` {
		t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
	}
	if active.State != "active" || active.Account == nil || !active.Account.EmailConfirmed {
		t.Fatalf("the card is %+v", active)
	}
	if !strings.Contains(h.logs.String(), `"msg":"relationship.confirmed"`) {
		t.Fatal("no relationship.confirmed record")
	}
}

func TestAcceptWithBearerAndSignUpFieldsIsRefused(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, cardEmail)
	link := linkToken(t, card.Invitation)
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	before := h.hasher.Hashes()

	refused := h.postJSON(t, invitationPath+link+"/accept", studentSignUp{Email: otherEmail}.body(), map[string]string{"Authorization": "Bearer " + student})

	want := []string{
		"body.name:not_allowed", "body.email:not_allowed", "body.password:not_allowed",
		"body.language:not_allowed", "body.timezone:not_allowed", "body.consent:not_allowed",
	}
	if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "validation_failed" || !slices.Equal(fields(t, refused), want) {
		t.Fatalf("sign-up fields with a bearer answered %d %s", refused.status, refused.body)
	}
	if h.mismatchCount(t, link) != 0 || h.hasher.Hashes() != before || h.userExists(t, otherEmail) {
		t.Fatal("the refusal counted, hashed or created something")
	}
	if unchanged := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", "")); unchanged.State != "invited" || h.invitationPage(t, link).status != http.StatusOK {
		t.Fatalf("the card changed: %+v", unchanged)
	}
}

func TestRemovalRacingConfirmationHasOneOutcome(t *testing.T) {
	for _, confirmationFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("confirmation first %t", confirmationFirst), func(t *testing.T) {
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.linkedStudent(t, tutor, newcomerEmail)
			signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			h.tick(t)
			token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
			confirmation := func() <-chan pendingAnswer {
				headers := signUpHeaders()
				headers["Authorization"] = "Bearer " + signedIn
				return h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email", fmt.Sprintf(`{"token":%q}`, token), headers)
			}
			removal := func() <-chan pendingAnswer {
				return h.sendInBackground(http.MethodPost, "/api/v1/students/"+card.ID+"/remove", "", jsonCall(tutor))
			}
			first, second := confirmation, removal
			if !confirmationFirst {
				first, second = removal, confirmation
			}
			release := h.holdUserRow(t, student)

			firstAnswer := first()
			h.waitForLockWaiters(t, 1)
			secondAnswer := second()
			h.waitForLockWaiters(t, 2)
			release()
			earlier, later := awaitAnswer(t, firstAnswer), awaitAnswer(t, secondAnswer)

			confirmed, removed := earlier, later
			if !confirmationFirst {
				confirmed, removed = later, earlier
			}
			stored := h.storedRelationship(t, card.ID)
			if confirmationFirst {
				if strings.TrimSpace(confirmed.body) != `{"outcome":"linked"}` || removed.status != http.StatusConflict || code(t, removed) != "relationship_not_provisional" {
					t.Fatalf("confirmation %d %s, removal %d %s", confirmed.status, confirmed.body, removed.status, removed.body)
				}
				if stored.State != "active" {
					t.Fatalf("the relationship is %s", stored.State)
				}
				return
			}
			if removed.status != http.StatusOK || confirmed.status != http.StatusOK || strings.TrimSpace(confirmed.body) != `{"outcome":null}` {
				t.Fatalf("removal %d %s, confirmation %d %s", removed.status, removed.body, confirmed.status, confirmed.body)
			}
			if stored.State != "discarded" || h.linksOf(t, student) != 0 {
				t.Fatalf("the relationship is %s", stored.State)
			}
		})
	}
}

func TestRemovedProvisionalStudentLosesTheTutorAndTheTutorGetsANewCard(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	invited := h.newCard(t, tutor, "Still invited", "invited@example.org")
	card := h.newCard(t, tutor, cardName, newcomerEmail)
	h.withNotes(t, tutor, card.ID, cardNotes)
	oldLink := linkToken(t, card.Invitation)
	if accepted := h.acceptInvitation(t, oldLink, newcomerEmail); accepted.status != http.StatusAccepted {
		t.Fatalf("the sign-up answered %d %s", accepted.status, accepted.body)
	}
	student := h.userID(t, newcomerEmail)

	removed := h.removeStudent(t, tutor, card.ID)
	repeated := h.removeStudent(t, tutor, card.ID)
	invitedRefusal := h.removeStudent(t, tutor, invited.ID)
	oldCard := h.cardCall(t, tutor, http.MethodGet, card.ID, "", "")
	h.tick(t)

	fresh := decodeRemoval(t, removed).NewCard
	if removed.status != http.StatusOK || removed.headers.Get("Cache-Control") != "no-store" || fresh.ID == card.ID || fresh.State != "invited" {
		t.Fatalf("the removal answered %d %s", removed.status, removed.body)
	}
	if fresh.DisplayName != cardName || fresh.Email == nil || *fresh.Email != newcomerEmail || fresh.Notes == nil || *fresh.Notes != cardNotes || fresh.Account != nil {
		t.Fatalf("the new card is %+v", fresh)
	}
	newLink := linkToken(t, fresh.Invitation)
	if newLink == oldLink || h.invitationPage(t, newLink).status != http.StatusOK || h.invitationPage(t, oldLink).status != http.StatusNotFound {
		t.Fatal("the new card has no fresh link of its own")
	}
	if repeated.status != http.StatusNotFound || oldCard.status != http.StatusNotFound || refusal(t, repeated) != refusal(t, h.removeStudent(t, tutor, randomCardID)) {
		t.Fatalf("the removed card answers %d and %d", repeated.status, oldCard.status)
	}
	if invitedRefusal.status != http.StatusConflict || code(t, invitedRefusal) != "relationship_not_provisional" {
		t.Fatalf("removing an invited card answered %d %s", invitedRefusal.status, invitedRefusal.body)
	}
	stored := h.storedRelationship(t, card.ID)
	if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "removed" || stored.Notes != "" || stored.Email != nil {
		t.Fatalf("the removed row is %+v", stored)
	}
	if h.linksOf(t, student) != 0 || len(h.cardsOf(t, tutorID, domain.RelationshipInvited)) != 2 {
		t.Fatal("the removed account still has a link or the tutor has no new card")
	}
	if got := h.mailTo(t, tutorEmail); len(got) != 0 {
		t.Fatalf("the tutor received %v", subjects(got))
	}
	if got := subjects(h.mailTo(t, newcomerEmail)); !slices.Equal(got, []string{confirmSubject}) {
		t.Fatalf("the removed student received %v", got)
	}
	if !strings.Contains(h.logs.String(), relationshipRemoved) || !strings.Contains(h.logs.String(), `"cause":"tutor"`) {
		t.Fatal("no relationship.removed record with the tutor as cause")
	}
}

func TestRemovingAnActiveCardIsRefused(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first, second := h.seedStudent(t, 2), h.seedStudent(t, 3)
	active := h.seedCard(t, tutorID, 1, domain.RelationshipActive, &first)
	ended := h.seedCard(t, tutorID, 2, domain.RelationshipEnded, &second)

	refusals := []response{h.removeStudent(t, tutor, active), h.removeStudent(t, tutor, ended)}

	for _, refused := range refusals {
		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_provisional" {
			t.Fatalf("removal answered %d %s", refused.status, refused.body)
		}
	}
	if h.storedRelationship(t, active).State != "active" || h.storedRelationship(t, ended).State != "ended" {
		t.Fatal("a refused removal changed a card")
	}
}

func TestUnverifiedLinkedAccountCanStillBeDeletedAndCleanedUp(t *testing.T) {
	removals := map[string]func(t *testing.T, h *authHarness){
		"its own deletion": func(t *testing.T, h *authHarness) {
			t.Helper()
			signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			if deleted := h.deleteMe(t, signedIn, newcomerPassword); deleted.status != http.StatusNoContent {
				t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
			}
		},
		"the 7-day cleanup": func(t *testing.T, h *authHarness) {
			t.Helper()
			h.at(t, authStart.Add(7*24*time.Hour))
			h.tick(t)
		},
	}
	for name, remove := range removals {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.linkedStudent(t, tutor, newcomerEmail)

			remove(t, h)

			if h.userExists(t, newcomerEmail) || len(h.rowsOwnedBy(t, student)) != 0 {
				t.Fatalf("the account was not purged: %v", h.rowsOwnedBy(t, student))
			}
			if left := h.countRows(t, "SELECT count(id) FROM relationships WHERE student_id = $1::uuid", student.String()); left != 0 {
				t.Fatalf("%d relationship rows still name the purged account", left)
			}
			stored := h.storedRelationship(t, card.ID)
			if stored.State != "discarded" || *stored.DiscardReason != "removed" || stored.StudentID != nil || stored.Email != nil {
				t.Fatalf("the old card is %+v", stored)
			}
			fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited)
			if len(fresh) != 1 || fresh[0].DisplayName != cardName || fresh[0].Email == nil || *fresh[0].Email != newcomerEmail {
				t.Fatalf("the tutor's new cards are %+v", fresh)
			}
			shown := decodeCard(t, h.cardCall(t, h.relogin(t, tutorEmail), http.MethodGet, fresh[0].ID, "", ""))
			if h.invitationPage(t, linkToken(t, shown.Invitation)).status != http.StatusOK {
				t.Fatal("the new card's link does not work")
			}
		})
	}
}

func TestUnconfirmedLinkedStudentIsRemovedAfter7DaysSilently(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.linkedStudent(t, tutor, "")
	h.tick(t)

	h.at(t, authStart.Add(7*24*time.Hour-time.Hour))
	h.tick(t)
	stillLinked := h.storedRelationship(t, card.ID)
	h.at(t, authStart.Add(7*24*time.Hour))
	h.tick(t)

	if stillLinked.State != "provisional" {
		t.Fatalf("before 7 days the card is %s", stillLinked.State)
	}
	if h.userExists(t, newcomerEmail) || h.storedRelationship(t, card.ID).State != "discarded" {
		t.Fatal("the unconfirmed account was not removed after 7 days")
	}
	fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited)
	if len(fresh) != 1 || fresh[0].DisplayName != cardName || fresh[0].Email != nil {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
	if got := h.mailTo(t, tutorEmail); len(got) != 0 {
		t.Fatalf("the tutor received %v", subjects(got))
	}
	logs := h.logs.String()
	if !strings.Contains(logs, relationshipRemoved) || !strings.Contains(logs, `"cause":"cleanup"`) || !strings.Contains(logs, `"user_id":"`+student.String()+`"`) {
		t.Fatal("no relationship.removed record with the cleanup as cause")
	}
}

func TestSameBrowserConfirmationRechecksTheSessionUnderTheLock(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.linkedStudent(t, tutor, newcomerEmail)
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.at(t, authStart.Add(61*time.Second))
	h.forgot(t, newcomerEmail)
	h.tick(t)
	messages := h.mailTo(t, newcomerEmail)
	confirmation, reset := latestToken(t, messages, confirmSubject), latestToken(t, messages, resetSubject)
	confirmHeaders := signUpHeaders()
	confirmHeaders["Authorization"] = "Bearer " + squatter
	release := h.holdUserRow(t, student)

	resetAnswer := h.sendInBackground(http.MethodPost, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, reset), signUpHeaders())
	h.waitForLockWaiters(t, 1)
	confirmAnswer := h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email", fmt.Sprintf(`{"token":%q}`, confirmation), confirmHeaders)
	h.waitForLockWaiters(t, 2)
	release()
	resetDone, confirmDone := awaitAnswer(t, resetAnswer), awaitAnswer(t, confirmAnswer)

	if resetDone.status != http.StatusOK {
		t.Fatalf("the reset answered %d %s", resetDone.status, resetDone.body)
	}
	if confirmDone.status != http.StatusUnauthorized || code(t, confirmDone) != "unauthenticated" {
		t.Fatalf("the confirmation authenticated before the reset answered %d %s", confirmDone.status, confirmDone.body)
	}
	if stored := h.storedRelationship(t, card.ID); stored.State != "reclaim_pending" {
		t.Fatalf("the squatter's confirmation decided the link: %s", stored.State)
	}
}

func TestResetAnswersNullOutcomeForAnActiveAccount(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student := h.seedUserAs(t, domain.RoleStudent, 2, newcomerEmail, newcomerPassword, currentHash)
	card := h.seedCard(t, tutorID, 1, domain.RelationshipActive, &student)
	h.forgot(t, newcomerEmail)
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), resetSubject)

	reset := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"a brand new password"}`, token), nil)

	if reset.status != http.StatusOK || strings.TrimSpace(reset.body) != `{"outcome":null}` || reset.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the reset answered %d %s %v", reset.status, reset.body, reset.headers)
	}
	if h.storedRelationship(t, card).State != "active" {
		t.Fatal("the reset of an active account changed its relationship")
	}
}

func TestTutoringFlowsLogNoEmailsNamesNotesOrTokens(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, newcomerEmail)
	h.withNotes(t, tutor, card.ID, cardNotes)
	link := linkToken(t, card.Invitation)
	malformed := "sentinel-malformed-token"

	h.acceptInvitation(t, link, otherEmail)
	h.acceptInvitation(t, malformed, newcomerEmail)
	h.acceptInvitation(t, link, newcomerEmail)
	removed := h.removeStudent(t, tutor, card.ID)

	answer := decodeRemoval(t, removed)
	logs := h.logs.String()
	for _, event := range []string{`"reason":"email_mismatch"`, `"reason":"invalid"`, invitationAccepted, relationshipRemoved} {
		if !strings.Contains(logs, event) {
			t.Fatalf("the logs lack %s", event)
		}
	}
	secrets := []string{newcomerEmail, otherEmail, tutorEmail, cardName, cardNotes, studentAccountName, newcomerPassword, link, link[:16], malformed}
	secrets = append(secrets, linkToken(t, answer.NewCard.Invitation))
	for _, secret := range secrets {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain %q", secret)
		}
	}
	if records := accessRecordsFor(t, logs, "POST /api/v1/invitations/{token}/accept"); len(records) != 3 {
		t.Fatalf("expected three access records with the route template, got %d", len(records))
	}
}

func TestInvitationSignUpReportsMissingFieldsAsRequired(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, tutor, cardName, cardEmail)
	link := linkToken(t, card.Invitation)
	complete := studentSignUp{Email: newcomerEmail}.body()
	withoutTimezone := strings.Replace(complete, `,"timezone":"Europe/Moscow"`, "", 1)
	withoutConsent := strings.Replace(complete, `"consent":true`, `"consent":false`, 1)
	randomToken := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	before := h.hasher.Hashes()

	empty := h.postJSON(t, invitationPath+link+"/accept", "{}", nil)
	noTimezone := h.postJSON(t, invitationPath+link+"/accept", withoutTimezone, nil)
	noConsent := h.postJSON(t, invitationPath+link+"/accept", withoutConsent, nil)
	unknown := h.postJSON(t, invitationPath+randomToken+"/accept", "{}", nil)

	want := []string{
		"body.name:required", "body.email:required", "body.password:required",
		"body.language:required", "body.timezone:required", "body.consent:required",
	}
	if empty.status != http.StatusUnprocessableEntity || code(t, empty) != "validation_failed" || !slices.Equal(fields(t, empty), want) {
		t.Fatalf("an empty sign-up answered %d %s", empty.status, empty.body)
	}
	if noTimezone.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, noTimezone), []string{"body.timezone:required"}) {
		t.Fatalf("a sign-up without a timezone answered %d %s", noTimezone.status, noTimezone.body)
	}
	if noConsent.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, noConsent), []string{"body.consent:required"}) {
		t.Fatalf("a sign-up without consent answered %d %s", noConsent.status, noConsent.body)
	}
	if unknown.status != http.StatusConflict || code(t, unknown) != "invitation_invalid" {
		t.Fatalf("an empty sign-up with an unknown token answered %d %s", unknown.status, unknown.body)
	}
	if h.hasher.Hashes() != before || h.mismatchCount(t, link) != 0 || h.userExists(t, newcomerEmail) || h.invitationPage(t, link).status != http.StatusOK {
		t.Fatal("a refused sign-up hashed, counted a mismatch, created an account or used the link")
	}
}

func TestRemovalRacingThePurgeOfTheAccountAnswersNotFound(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.linkedStudent(t, tutor, newcomerEmail)
	signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	deleteHeaders := map[string]string{"Authorization": "Bearer " + signedIn, "Content-Type": "application/json"}
	release := h.holdUserRow(t, student)

	deletion := h.sendInBackground(http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, newcomerPassword), deleteHeaders)
	h.waitForLockWaiters(t, 1)
	removalAnswer := h.sendInBackground(http.MethodPost, "/api/v1/students/"+card.ID+"/remove", "", jsonCall(tutor))
	h.waitForLockWaiters(t, 2)
	release()
	deleted, removed := awaitAnswer(t, deletion), awaitAnswer(t, removalAnswer)

	if deleted.status != http.StatusNoContent || h.userExists(t, newcomerEmail) {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	if removed.status != http.StatusNotFound || refusal(t, removed) != refusal(t, h.removeStudent(t, tutor, randomCardID)) {
		t.Fatalf("the removal after the purge answered %d %s", removed.status, removed.body)
	}
	if fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited); len(fresh) != 1 || fresh[0].DisplayName != cardName {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
}

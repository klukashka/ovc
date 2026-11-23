package api_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

const (
	englishInvitationSubject = "You are invited to study on OVC"
	russianInvitationSubject = "Приглашение на занятия в OVC"
	sendQueued               = `"msg":"invitation.send_queued"`
)

func (h *authHarness) sendInvitation(t *testing.T, token string, card string) response {
	t.Helper()
	return h.cardCall(t, token, http.MethodPost, card, "/invitation/send", "")
}

func (h *authHarness) sendTimes(t *testing.T, token string, card string, times int) {
	t.Helper()
	for index := range times {
		if sent := h.sendInvitation(t, token, card); sent.status != http.StatusAccepted {
			t.Fatalf("send %d answered %d %s", index+1, sent.status, sent.body)
		}
	}
}

func (h *authHarness) setAccount(t *testing.T, user domain.UserID, name string, language domain.Language) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET name = $1, language = $2 WHERE id = $3", name, string(language), user)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) relogin(t *testing.T, email string) string {
	t.Helper()
	return accessToken(t, h.login(t, email, accountPassword))
}

func invitationLinks(t *testing.T, messages []mail.SinkMessage) []string {
	t.Helper()
	var links []string
	for _, message := range messages {
		for _, link := range message.Links {
			if strings.HasPrefix(link, publicOrigin+"/invite/") {
				links = append(links, link)
			}
		}
	}
	return links
}

func sameAnswer(first response, second response) bool {
	return first.status == second.status && first.body == second.body &&
		fmt.Sprint(comparableHeaders(first.headers)) == fmt.Sprint(comparableHeaders(second.headers))
}

func TestInvitationEmailRespectsTheTutorsBudget(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	h.sendTimes(t, token, card.ID, 10)

	eleventh := h.sendInvitation(t, token, card.ID)
	h.at(t, authStart.Add(8*24*time.Hour))
	token = h.relogin(t, tutorEmail)
	for index := range 50 {
		if sent := h.sendInvitation(t, token, card.ID); sent.status != http.StatusAccepted {
			t.Fatalf("send %d after the first week answered %d %s", index+1, sent.status, sent.body)
		}
	}
	fiftyFirst := h.sendInvitation(t, token, card.ID)

	for name, limited := range map[string]response{"the 11th send of the first week": eleventh, "the 51st send of a later day": fiftyFirst} {
		if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Cache-Control") != "no-store" {
			t.Errorf("%s answered %d %v %s", name, limited.status, limited.headers, limited.body)
		}
	}
	if eleventh.headers.Get("Retry-After") != strconv.Itoa(int((15 * time.Hour).Seconds())) {
		t.Fatalf("the refused send says Retry-After %q", eleventh.headers.Get("Retry-After"))
	}
	logs := h.logs.String()
	if strings.Count(logs, `"msg":"invitation.send_limited"`) != 2 || !strings.Contains(logs, `"msg":"invitation.send_limited","tutor_id"`) {
		t.Fatalf("expected two invitation.send_limited records: %s", logs)
	}
	if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated)); queued != 6 {
		t.Fatalf("the budget queued %d emails instead of three per day for the one recipient", queued)
	}
}

func TestRecipientCapIsSilent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, first := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, second := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	firstCard := h.newCard(t, first, cardName, cardEmail)
	secondCard := h.newCard(t, second, "Maria from the other school", cardEmail)
	accepted := h.sendInvitation(t, first, firstCard.ID)
	h.tick(t)
	h.sendTimes(t, first, firstCard.ID, 1)
	h.tick(t)
	h.sendTimes(t, second, secondCard.ID, 1)
	before := h.cardCall(t, second, http.MethodGet, secondCard.ID, "", "")

	dropped := h.sendInvitation(t, second, secondCard.ID)
	after := h.cardCall(t, second, http.MethodGet, secondCard.ID, "", "")
	h.tick(t)

	if !sameAnswer(accepted, dropped) || dropped.status != http.StatusAccepted {
		t.Fatalf("the dropped send answered %d %v %q unlike a queued one %d %v %q",
			dropped.status, comparableHeaders(dropped.headers), dropped.body, accepted.status, comparableHeaders(accepted.headers), accepted.body)
	}
	if before.body != after.body {
		t.Fatalf("the dropped send changed the card:\n%s\n%s", before.body, after.body)
	}
	if got := h.mailTo(t, cardEmail); len(got) != 3 {
		t.Fatalf("the recipient got %d emails instead of 3", len(got))
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"msg":"invitation.send_dropped","tutor_id":"`) || !strings.Contains(logs, `"reason":"recipient_cap"`) || strings.Count(logs, sendQueued) != 3 {
		t.Fatalf("the dropped send was not recorded: %s", logs)
	}
}

func TestRecipientCapResetsOnTheNextUTCDay(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	for range 4 {
		h.sendTimes(t, token, card.ID, 1)
		h.tick(t)
	}
	h.at(t, authStart.Add(15*time.Hour))
	token = h.relogin(t, tutorEmail)

	nextDay := h.sendInvitation(t, token, card.ID)
	h.tick(t)

	if nextDay.status != http.StatusAccepted || len(h.mailTo(t, cardEmail)) != 4 {
		t.Fatalf("the next day's send answered %d and the recipient holds %d emails instead of 4", nextDay.status, len(h.mailTo(t, cardEmail)))
	}
}

func (h *authHarness) regenerationsToday(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", "invitation_regeneration_day")
}

func (h *authHarness) revokeAfterMismatches(t *testing.T, link string) {
	t.Helper()
	for range domain.InvitationMismatchLimit {
		if _, err := h.store.Repositories().Invitations.CountMismatch(context.Background(), tokenHash(t, link), authStart); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *authHarness) invitationsOf(t *testing.T, card string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM invitations WHERE relationship_id = $1::uuid", card)
}

func TestSendingAMismatchRevokedLinkIssuesAFreshOneCountedAsARegeneration(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	revoked := linkToken(t, card.Invitation)
	h.revokeAfterMismatches(t, revoked)

	sent := h.sendInvitation(t, token, card.ID)
	h.tick(t)

	current := decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", ""))
	fresh := linkToken(t, current.Invitation)
	links := invitationLinks(t, h.mailTo(t, cardEmail))
	if sent.status != http.StatusAccepted || current.Invitation.Status != "open" || fresh == revoked || h.regenerationsToday(t) != 1 {
		t.Fatalf("the send answered %d, the card shows %+v, %d regenerations counted", sent.status, current.Invitation, h.regenerationsToday(t))
	}
	if len(links) != 1 || links[0] != *current.Invitation.URL || h.revokeReason(t, revoked) != "mismatch_limit" {
		t.Fatalf("the email carries %v", links)
	}
}

func TestSendNeedingAFreshLinkIsRefusedWhenRegenerationsAreUsedUp(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other := h.newCard(t, token, "Another pupil", "")
	card := h.newCard(t, token, cardName, cardEmail)
	h.revokeAfterMismatches(t, linkToken(t, card.Invitation))
	for index := range 50 {
		if regenerated := h.cardCall(t, token, http.MethodPost, other.ID, "/invitation", ""); regenerated.status != http.StatusOK {
			t.Fatalf("regeneration %d answered %d", index+1, regenerated.status)
		}
	}
	invitationsBefore := h.invitationsOf(t, card.ID)

	refused := h.sendInvitation(t, token, card.ID)

	if refused.status != http.StatusTooManyRequests || code(t, refused) != "rate_limited" || refused.headers.Get("Retry-After") != strconv.Itoa(int((15*time.Hour).Seconds())) {
		t.Fatalf("the send answered %d %v %s", refused.status, refused.headers, refused.body)
	}
	if h.invitationsOf(t, card.ID) != invitationsBefore || h.regenerationsToday(t) != 50 {
		t.Fatal("the refused send issued a link or counted a regeneration")
	}
	queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated))
	budget := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", "invitation_email_day")
	if queued != 0 || budget != 0 {
		t.Fatalf("the refused send queued %d rows and counted %d sends", queued, budget)
	}
}

func TestSendingAnOpenOrExpiredLinkCountsNoRegeneration(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	open := h.newCard(t, token, cardName, cardEmail)
	expiring := h.newCard(t, token, "Expiring", "expiring@example.org")
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE invitations SET expires_at = $1 WHERE relationship_id = $2::uuid", authStart.Add(-time.Hour), expiring.ID); err != nil {
		t.Fatal(err)
	}

	openSend := h.sendInvitation(t, token, open.ID)
	expiredSend := h.sendInvitation(t, token, expiring.ID)

	if openSend.status != http.StatusAccepted || expiredSend.status != http.StatusAccepted || h.regenerationsToday(t) != 0 {
		t.Fatalf("the sends answered %d and %d and counted %d regenerations", openSend.status, expiredSend.status, h.regenerationsToday(t))
	}
	if h.revokeReason(t, linkToken(t, expiring.Invitation)) != "expired_on_send" {
		t.Fatal("the expired link was not replaced")
	}
}

func TestMissingCardEmailIsRefusedBeforeTheExhaustedBudget(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	withoutEmail := h.newCard(t, token, "No email", "")
	h.sendTimes(t, token, card.ID, 10)

	refused := h.sendInvitation(t, token, withoutEmail.ID)

	if refused.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, refused), []string{"card.email:required"}) {
		t.Fatalf("a card without an email over the budget answered %d %s", refused.status, refused.body)
	}
}

func TestBudgetRefusalLeavesAnExpiredLinkUnrevoked(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	expiring := h.newCard(t, token, "Expiring", "expiring@example.org")
	h.sendTimes(t, token, card.ID, 10)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE invitations SET expires_at = $1 WHERE relationship_id = $2::uuid", authStart.Add(-time.Hour), expiring.ID); err != nil {
		t.Fatal(err)
	}
	link := linkToken(t, expiring.Invitation)

	refused := h.sendInvitation(t, token, expiring.ID)

	if refused.status != http.StatusTooManyRequests || code(t, refused) != "rate_limited" {
		t.Fatalf("the send over the budget answered %d %s", refused.status, refused.body)
	}
	if h.revokeReason(t, link) != "" || !h.ciphertextKept(t, link) || h.invitationsOf(t, expiring.ID) != 1 {
		t.Fatal("the refused send changed the expired link")
	}
}

func TestUnreadableInvitationCiphertextMakesTheEmailDead(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	h.sendTimes(t, token, card.ID, 1)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE invitations SET token_encrypted = $1 WHERE relationship_id = $2::uuid", make([]byte, 60), card.ID); err != nil {
		t.Fatal(err)
	}

	h.tick(t)

	var status, errorCode string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT status, coalesce(last_error_code, '') FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated)).Scan(&status, &errorCode)
	if err != nil {
		t.Fatal(err)
	}
	if status != "dead" || errorCode != "permanent" || len(h.mailTo(t, cardEmail)) != 0 {
		t.Fatalf("the row ended %s with %q", status, errorCode)
	}
}

func TestNoInvitationEmailToABlockedOrDeletedAccount(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blocked := h.seedStudent(t, 2)
	deleted := h.seedStudent(t, 3)
	blockedLater := h.seedStudent(t, 4)
	open := h.newCard(t, token, "Open address", "open@example.org")
	cards := map[string]studentCard{
		"blocked":       h.newCard(t, token, "Blocked", "student-2@example.org"),
		"deleted":       h.newCard(t, token, "Deleted", "student-3@example.org"),
		"blocked later": h.newCard(t, token, "Blocked later", "student-4@example.org"),
	}
	if _, err := h.store.Repositories().Users.Block(context.Background(), blocked, authStart); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil {
		t.Fatal(err)
	}
	reference := h.sendInvitation(t, token, open.ID)

	answers := map[string]response{}
	for name, card := range cards {
		answers[name] = h.sendInvitation(t, token, card.ID)
	}
	if _, err := h.store.Repositories().Users.Block(context.Background(), blockedLater, authStart); err != nil {
		t.Fatal(err)
	}
	h.tick(t)

	for name, answer := range answers {
		if !sameAnswer(reference, answer) {
			t.Errorf("the send to a %s account answered %d %q unlike any other", name, answer.status, answer.body)
		}
	}
	for _, email := range []string{"student-2@example.org", "student-3@example.org", "student-4@example.org"} {
		if got := h.mailTo(t, email); len(got) != 0 {
			t.Errorf("%s received %v", email, subjects(got))
		}
	}
	if len(h.mailTo(t, "open@example.org")) != 1 || h.obsoleteRows(t, domain.EventInvitationCreated) != 1 {
		t.Fatal("expected one sent email and one dropped at dispatch")
	}
	if strings.Count(h.logs.String(), `"reason":"recipient_unavailable"`) != 2 {
		t.Fatalf("expected two recipient_unavailable drops: %s", h.logs.String())
	}
}

func TestInvitationSendRefusesCardsThatCannotTakeIt(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unverified := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 2, "unconfirmed-tutor@example.org")
	students := []domain.UserID{h.seedStudent(t, 3), h.seedStudent(t, 4), h.seedStudent(t, 5)}
	withoutEmail := h.newCard(t, token, "No email", "")
	linked := map[string]string{
		"provisional":     h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &students[0]),
		"reclaim pending": h.seedCard(t, tutor, 2, domain.RelationshipReclaimPending, &students[1]),
		"active":          h.seedCard(t, tutor, 3, domain.RelationshipActive, &students[2]),
	}

	missing := h.sendInvitation(t, token, withoutEmail.ID)
	refusals := map[string]response{}
	for state, id := range linked {
		refusals[state] = h.sendInvitation(t, token, id)
	}
	notConfirmed := h.sendInvitation(t, unverified, randomCardID)

	if missing.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, missing), []string{"card.email:required"}) {
		t.Fatalf("a card without an email answered %d %s", missing.status, missing.body)
	}
	for state, refused := range refusals {
		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_active" {
			t.Errorf("sending a %s card's link answered %d %s", state, refused.status, refused.body)
		}
	}
	if notConfirmed.status != http.StatusForbidden || code(t, notConfirmed) != "email_not_verified" {
		t.Fatalf("an unverified tutor's send answered %d %s", notConfirmed.status, notConfirmed.body)
	}
	if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated)); queued != 0 {
		t.Fatalf("refused sends queued %d emails", queued)
	}
}

func TestSendingAnExpiredLinkSendsAFreshOne(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	expired := linkToken(t, card.Invitation)
	h.at(t, authStart.Add(15*24*time.Hour))
	token = h.relogin(t, tutorEmail)

	sent := h.sendInvitation(t, token, card.ID)
	h.tick(t)

	current := decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", ""))
	fresh := linkToken(t, current.Invitation)
	links := invitationLinks(t, h.mailTo(t, cardEmail))
	if sent.status != http.StatusAccepted || fresh == expired || h.revokeReason(t, expired) != "expired_on_send" || h.ciphertextKept(t, expired) {
		t.Fatalf("sending the expired link answered %d and left the card %+v", sent.status, current.Invitation)
	}
	if len(links) != 1 || links[0] != *current.Invitation.URL || h.invitationPage(t, fresh).status != http.StatusOK {
		t.Fatalf("the email carries %v instead of the card's fresh link", links)
	}
}

func TestSendingACardWithoutAnOpenLinkIssuesOne(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student := h.seedStudent(t, 2)
	ended := h.seedCard(t, tutor, 1, domain.RelationshipEnded, &student)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE relationships SET email = $1, email_set_at = updated_at WHERE tutor_id = $2", "student-2@example.org", tutor); err != nil {
		t.Fatal(err)
	}

	sent := h.sendInvitation(t, token, ended)
	h.tick(t)

	card := decodeCard(t, h.cardCall(t, token, http.MethodGet, ended, "", ""))
	links := invitationLinks(t, h.mailTo(t, "student-2@example.org"))
	if sent.status != http.StatusAccepted || card.Invitation == nil || len(links) != 1 || links[0] != *card.Invitation.URL {
		t.Fatalf("sending an ended card without a link answered %d, card %+v, links %v", sent.status, card.Invitation, links)
	}
	var bound *domain.UserID
	err := h.database.Pool.QueryRow(context.Background(), "SELECT bound_student_id FROM invitations WHERE token_hash = $1", tokenHash(t, linkToken(t, card.Invitation))).Scan(&bound)
	if err != nil || bound == nil || *bound != student {
		t.Fatalf("the issued link is bound to %v (%v)", bound, err)
	}
}

func TestInvitationEmailIsObsoleteOnceTheLinkIsUsedRevokedOrExpired(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blockedTutor, blockedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "blocked-tutor@example.org")
	student := h.seedStudent(t, 3)
	revoked := h.newCard(t, token, "Revoked", "revoked@example.org")
	used := h.newCard(t, token, "Used", "used@example.org")
	resent := h.newCard(t, token, "Resent", "resent@example.org")
	withdrawn := h.newCard(t, token, "Withdrawn", "withdrawn@example.org")
	ofBlockedTutor := h.newCard(t, blockedToken, "Blocked tutor's", "blocked-tutors@example.org")
	for _, card := range []studentCard{revoked, used, resent, resent, withdrawn} {
		h.sendTimes(t, token, card.ID, 1)
	}
	h.sendTimes(t, blockedToken, ofBlockedTutor.ID, 1)
	h.cardCall(t, token, http.MethodPost, revoked.ID, "/invitation", "")
	if _, err := h.store.Repositories().Invitations.Consume(context.Background(), tokenHash(t, linkToken(t, used.Invitation)), student, authStart); err != nil {
		t.Fatal(err)
	}
	h.cardCall(t, token, http.MethodDelete, withdrawn.ID, "", "")
	if _, err := h.store.Repositories().Users.Block(context.Background(), blockedTutor, authStart); err != nil {
		t.Fatal(err)
	}

	h.tick(t)
	expiring := h.newCard(t, token, "Expiring", "expiring@example.org")
	h.sendTimes(t, token, expiring.ID, 1)
	h.at(t, authStart.Add(14*24*time.Hour))
	h.tick(t)

	for _, email := range []string{"revoked@example.org", "used@example.org", "withdrawn@example.org", "blocked-tutors@example.org", "expiring@example.org"} {
		if got := h.mailTo(t, email); len(got) != 0 {
			t.Errorf("%s received %v", email, subjects(got))
		}
	}
	if got := h.mailTo(t, "resent@example.org"); len(got) != 1 {
		t.Fatalf("the resent card's address received %d emails instead of the latest send only", len(got))
	}
	if dropped := h.obsoleteRows(t, domain.EventInvitationCreated); dropped != 6 {
		t.Fatalf("%d invitation emails were dropped as obsolete instead of 6", dropped)
	}
}

func TestInvitationTokenNeverInOutboxPayloadOrLogs(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setAccount(t, tutor, "Anna Tutorova", domain.LanguageEnglish)
	card := h.newCard(t, token, cardName, cardEmail)
	link := linkToken(t, card.Invitation)

	sent := h.sendInvitation(t, token, card.ID)
	h.tick(t)

	var payload, key string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT payload::text, idempotency_key FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated)).Scan(&payload, &key)
	if err != nil {
		t.Fatal(err)
	}
	var invitation []byte
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT id FROM invitations WHERE token_hash = $1", tokenHash(t, link)).Scan(&invitation); err != nil {
		t.Fatal(err)
	}
	identifier := hex.EncodeToString(invitation)
	if sent.status != http.StatusAccepted || payload != fmt.Sprintf(`{"send": 1, "invitation": %q}`, identifier) || key != "invitation.created:"+identifier+":1" {
		t.Fatalf("the outbox row holds %s under %s", payload, key)
	}
	logs := h.logs.String()
	for _, private := range []string{link, hex.EncodeToString(tokenHash(t, link)), cardEmail, cardName, "Anna Tutorova"} {
		if strings.Contains(payload+key+logs, private) {
			t.Fatalf("the outbox row or the logs contain %q", private)
		}
	}
	if !strings.Contains(logs, sendQueued) || !strings.Contains(logs, `"kind":"invitation.created"`) {
		t.Fatalf("the send and its delivery were not recorded: %s", logs)
	}
	if links := invitationLinks(t, h.mailTo(t, cardEmail)); len(links) != 1 || links[0] != *card.Invitation.URL {
		t.Fatalf("the email carries %v", links)
	}
}

func TestInvitationEmailEscapesTheTutorNameAndKeepsItOutOfTheSubject(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setAccount(t, tutor, `Anna <a href="https://evil.example/">Click</a> & "Co"`, domain.LanguageEnglish)
	card := h.newCard(t, token, cardName, cardEmail)

	h.sendTimes(t, token, card.ID, 1)
	h.tick(t)

	messages := h.mailTo(t, cardEmail)
	if len(messages) != 1 {
		t.Fatalf("expected one email, got %d", len(messages))
	}
	message := messages[0]
	if message.Subject != englishInvitationSubject || strings.Contains(message.Subject, "Anna") {
		t.Fatalf("the subject is %q", message.Subject)
	}
	if strings.Contains(message.HTML, `<a href="https://evil`) || !strings.Contains(message.HTML, `Anna &lt;a href=&#34;https://evil.example/&#34;&gt;Click&lt;/a&gt; &amp; &#34;Co&#34;`) {
		t.Fatalf("the HTML body does not escape the tutor's name: %s", message.HTML)
	}
	if strings.Count(message.HTML, "<a ") != 1 || !strings.Contains(message.HTML, `<a href="`+*card.Invitation.URL+`"`) {
		t.Fatalf("the HTML body links to anything but the invitation: %s", message.HTML)
	}
	if !strings.Contains(message.Text, `Anna <a href="https://evil.example/">Click</a> & "Co"`) || !strings.Contains(message.Text, ": "+*card.Invitation.URL) {
		t.Fatalf("the text body is %q", message.Text)
	}
	if strings.Contains(message.Text+message.HTML, cardName) {
		t.Fatal("the email carries the tutor's private card name")
	}
}

func TestInvitationEmailFollowsTheRecipientsLanguageOrTheTutors(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	englishTutor, englishToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	russianTutor, russianToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "russian-tutor@example.org")
	h.setAccount(t, englishTutor, "Anna", domain.LanguageEnglish)
	h.setAccount(t, russianTutor, "Ирина", domain.LanguageRussian)
	russianStudent, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, "unconfirmed@example.org")
	h.setAccount(t, russianStudent, "Борис", domain.LanguageRussian)
	cards := map[string]string{
		"no account, English tutor":           h.newCard(t, englishToken, "New pupil", "new@example.org").ID,
		"no account, Russian tutor":           h.newCard(t, russianToken, "Новый ученик", "novy@example.org").ID,
		"unverified Russian account, English": h.newCard(t, englishToken, "Boris", "unconfirmed@example.org").ID,
	}
	tokens := map[string]string{
		"no account, English tutor": englishToken, "no account, Russian tutor": russianToken, "unverified Russian account, English": englishToken,
	}

	for name, card := range cards {
		h.sendTimes(t, tokens[name], card, 1)
	}
	h.tick(t)

	expected := map[string]struct {
		email   string
		subject string
	}{
		"no account, English tutor":           {"new@example.org", englishInvitationSubject},
		"no account, Russian tutor":           {"novy@example.org", russianInvitationSubject},
		"unverified Russian account, English": {"unconfirmed@example.org", russianInvitationSubject},
	}
	for name, want := range expected {
		got := h.mailTo(t, want.email)
		if len(got) != 1 || got[0].Subject != want.subject || len(invitationLinks(t, got)) != 1 {
			t.Errorf("%s: got %v", name, subjects(got))
		}
	}
	russian := h.mailTo(t, "novy@example.org")[0]
	if !strings.Contains(russian.HTML, `lang="ru"`) || !strings.Contains(russian.Text, "Ирина") || !strings.Contains(russian.Text, "Откройте приглашение") {
		t.Fatalf("the Russian rendering is %q", russian.Text)
	}
	english := h.mailTo(t, "new@example.org")[0]
	if !strings.Contains(english.HTML, `lang="en"`) || !strings.Contains(english.Text, "Anna") || !strings.Contains(english.Text, "Open the invitation") {
		t.Fatalf("the English rendering is %q", english.Text)
	}
}

type freshLinkSend struct {
	answer        response
	invitation    *cardInvitation
	regenerations int64
	invitations   int64
	budget        int64
}

func (h *authHarness) sendNeedingAFreshLink(t *testing.T, recipient string, prepare func(t *testing.T, h *authHarness)) freshLinkSend {
	t.Helper()
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	prepare(t, h)
	card := h.newCard(t, token, cardName, recipient)
	h.revokeAfterMismatches(t, linkToken(t, card.Invitation))
	answer := h.sendInvitation(t, token, card.ID)
	return freshLinkSend{
		answer:        answer,
		invitation:    decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", "")).Invitation,
		regenerations: h.regenerationsToday(t),
		invitations:   h.invitationsOf(t, card.ID),
		budget:        h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", "invitation_email_day"),
	}
}

func TestDroppedSendIssuesTheFreshLinkLikeAQueuedOne(t *testing.T) {
	t.Parallel()
	nothing := func(*testing.T, *authHarness) {}
	capped := func(t *testing.T, h *authHarness) {
		t.Helper()
		_, other := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
		earlier := h.newCard(t, other, "Elsewhere", cardEmail)
		h.sendTimes(t, other, earlier.ID, 3)
	}
	unavailable := func(state domain.AccountState) func(*testing.T, *authHarness) {
		return func(t *testing.T, h *authHarness) {
			t.Helper()
			student := h.seedStudent(t, 3)
			users := h.store.Repositories().Users
			change := users.Block
			if state == domain.AccountDeleted {
				change = users.MarkDeleted
			}
			if _, err := change(context.Background(), student, authStart); err != nil {
				t.Fatal(err)
			}
		}
	}
	queued := newAuthHarness(t).sendNeedingAFreshLink(t, cardEmail, nothing)
	drops := map[string]struct {
		recipient string
		prepare   func(*testing.T, *authHarness)
		reason    string
	}{
		"over the recipient cap": {cardEmail, capped, "recipient_cap"},
		"a blocked account":      {"student-3@example.org", unavailable(domain.AccountBlocked), "recipient_unavailable"},
		"a deleted account":      {"student-3@example.org", unavailable(domain.AccountDeleted), "recipient_unavailable"},
	}

	for name, drop := range drops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)

			dropped := h.sendNeedingAFreshLink(t, drop.recipient, drop.prepare)

			if !sameAnswer(queued.answer, dropped.answer) || !strings.Contains(h.logs.String(), `"reason":"`+drop.reason+`"`) {
				t.Fatalf("the dropped send answered %d %q unlike the queued %d %q", dropped.answer.status, dropped.answer.body, queued.answer.status, queued.answer.body)
			}
			if dropped.invitation == nil || dropped.invitation.Status != "open" || dropped.invitation.URL == nil || queued.invitation.Status != "open" {
				t.Fatalf("the dropped send left the card with %+v", dropped.invitation)
			}
			if dropped.regenerations != queued.regenerations || dropped.invitations != queued.invitations || dropped.regenerations != 1 {
				t.Fatalf("regenerations %d and invitations %d after the drop, %d and %d after the queued send",
					dropped.regenerations, dropped.invitations, queued.regenerations, queued.invitations)
			}
			if !dropped.invitation.ExpiresAt.Equal(queued.invitation.ExpiresAt) {
				t.Fatalf("the fresh links expire at %v and %v", dropped.invitation.ExpiresAt, queued.invitation.ExpiresAt)
			}
		})
	}
	if queued.budget != 1 {
		t.Fatalf("the queued send counted %d sends", queued.budget)
	}
}

package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	randomCardID   = "0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a"
	cardEmail      = "pupil@example.org"
	cardName       = "Maria the reader"
	cardNotes      = "Prefers mornings; working on articles"
	invitationPath = "/api/v1/invitations/"
	cardCreated    = `"msg":"card.created"`
)

type cardInvitation struct {
	URL       *string   `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
}

type cardAccount struct {
	Name           string  `json:"name"`
	Email          string  `json:"email"`
	AvatarID       *string `json:"avatar_id"`
	EmailConfirmed bool    `json:"email_confirmed"`
}

type studentCard struct {
	ID          string            `json:"id"`
	State       string            `json:"state"`
	DisplayName string            `json:"display_name"`
	Email       *string           `json:"email"`
	Account     *cardAccount      `json:"account"`
	Groups      []json.RawMessage `json:"groups"`
	Notes       *string           `json:"notes"`
	Invitation  *cardInvitation   `json:"invitation"`
	CreatedAt   time.Time         `json:"created_at"`
}

type cardPage struct {
	Items      []studentCard `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

func cardBody(name string, email string) string {
	if email == "" {
		return fmt.Sprintf(`{"display_name":%s}`, jsonText(name))
	}
	return fmt.Sprintf(`{"display_name":%s,"email":%s}`, jsonText(name), jsonText(email))
}

func (h *authHarness) createCard(t *testing.T, token string, body string, key string) response {
	t.Helper()
	headers := jsonCall(token)
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return h.send(t, http.MethodPost, "/api/v1/students", body, headers)
}

func (h *authHarness) newCard(t *testing.T, token string, name string, email string) studentCard {
	t.Helper()
	result := h.createCard(t, token, cardBody(name, email), "")
	if result.status != http.StatusCreated {
		t.Fatalf("creating a card answered %d %s", result.status, result.body)
	}
	return decodeCard(t, result)
}

func (h *authHarness) cardCall(t *testing.T, token string, method string, id string, suffix string, body string) response {
	t.Helper()
	return h.send(t, method, "/api/v1/students/"+id+suffix, body, jsonCall(token))
}

func (h *authHarness) invitationPage(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodGet, invitationPath+token, "", nil)
}

func decodeCard(t *testing.T, result response) studentCard {
	t.Helper()
	var card studentCard
	if err := json.Unmarshal([]byte(result.body), &card); err != nil || card.ID == "" {
		t.Fatalf("not a student card: %d %s", result.status, result.body)
	}
	return card
}

func linkToken(t *testing.T, invitation *cardInvitation) string {
	t.Helper()
	if invitation == nil || invitation.URL == nil {
		t.Fatal("the card shows no link")
	}
	parsed, err := url.Parse(*invitation.URL)
	if err != nil {
		t.Fatal(err)
	}
	token, found := strings.CutPrefix(parsed.Path, "/invite/")
	if !found || parsed.Scheme+"://"+parsed.Host != publicOrigin || len(token) != 43 {
		t.Fatalf("the link %q is not an invitation link of the public origin", *invitation.URL)
	}
	return token
}

func (h *authHarness) seedCard(t *testing.T, tutor domain.UserID, id byte, state domain.RelationshipState, student *domain.UserID) string {
	t.Helper()
	card := repositories.NewRelationship{
		ID: domain.RelationshipID{0xc0, id}, TutorID: tutor, State: state, DisplayName: "A seeded student", CreatedAt: authStart,
	}
	if student != nil {
		colour := domain.ColourClay
		card.StudentID, card.Colour, card.LinkedAt = student, &colour, &authStart
	}
	switch state {
	case domain.RelationshipEnded:
		by := domain.EndedByTutor
		card.EndedAt, card.EndedBy = &authStart, &by
	case domain.RelationshipDiscarded:
		reason := domain.DiscardMerged
		if student != nil {
			reason = domain.DiscardRemoved
		}
		card.DiscardReason, card.DiscardedAt = &reason, &authStart
	case domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive:
	}
	if err := h.store.Repositories().Relationships.Create(context.Background(), card); err != nil {
		t.Fatal(err)
	}
	return card.ID.String()
}

func (h *authHarness) seedStudent(t *testing.T, id byte) domain.UserID {
	t.Helper()
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, id, fmt.Sprintf("student-%d@example.org", id))
	return student
}

func (h *authHarness) cardCount(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	var count int64
	err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM relationships WHERE tutor_id = $1", tutor).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) ciphertextKept(t *testing.T, token string) bool {
	t.Helper()
	var kept bool
	err := h.database.Pool.QueryRow(context.Background(), "SELECT token_encrypted IS NOT NULL FROM invitations WHERE token_hash = $1", tokenHash(t, token)).Scan(&kept)
	if err != nil {
		t.Fatal(err)
	}
	return kept
}

func (h *authHarness) revokeReason(t *testing.T, token string) string {
	t.Helper()
	var reason *string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT revoke_reason FROM invitations WHERE token_hash = $1", tokenHash(t, token)).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	if reason == nil {
		return ""
	}
	return *reason
}

func tokenHash(t *testing.T, token string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	return hash[:]
}

func errorDetails(t *testing.T, result response) map[string]string {
	t.Helper()
	var decoded struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Error.Details
}

func TestTutorCreatesReadsEditsAndRegeneratesACardWithoutLoggingPrivateText(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	created := h.createCard(t, token, cardBody(" "+cardName+" ", strings.ToUpper(cardEmail)), "")
	card := decodeCard(t, created)
	firstToken := linkToken(t, card.Invitation)
	read := decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", ""))
	edited := h.cardCall(t, token, http.MethodPatch, card.ID, "", fmt.Sprintf(`{"display_name":"Maria R.","notes":%s}`, jsonText(cardNotes)))
	regenerated := h.cardCall(t, token, http.MethodPost, card.ID, "/invitation", "")
	var fresh struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(regenerated.body), &fresh); err != nil {
		t.Fatal(err)
	}
	secondToken := linkToken(t, &fresh.Invitation)
	page := h.invitationPage(t, secondToken)
	withdrawn := h.cardCall(t, token, http.MethodDelete, card.ID, "", "")
	afterWithdrawal := h.cardCall(t, token, http.MethodGet, card.ID, "", "")

	if created.status != http.StatusCreated || card.State != "invited" || card.DisplayName != cardName || card.Email == nil || *card.Email != cardEmail {
		t.Fatalf("created %d %s", created.status, created.body)
	}
	if card.Account != nil || card.Groups == nil || len(card.Groups) != 0 || card.Notes == nil || *card.Notes != "" || card.Invitation.Status != "open" {
		t.Fatalf("unexpected new card %s", created.body)
	}
	if !card.Invitation.ExpiresAt.Equal(authStart.Add(14*24*time.Hour)) || read.Invitation == nil || *read.Invitation.URL != *card.Invitation.URL {
		t.Fatalf("the link is not shown again with its expiry: %s", created.body)
	}
	editedCard := decodeCard(t, edited)
	if edited.status != http.StatusOK || editedCard.DisplayName != "Maria R." || *editedCard.Notes != cardNotes || *editedCard.Invitation.URL != *card.Invitation.URL {
		t.Fatalf("edited %d %s", edited.status, edited.body)
	}
	if regenerated.status != http.StatusOK || secondToken == firstToken || h.revokeReason(t, firstToken) != "regenerated" || h.ciphertextKept(t, firstToken) {
		t.Fatalf("regenerated %d %s", regenerated.status, regenerated.body)
	}
	if page.status != http.StatusOK || !strings.Contains(page.body, `"name":"A language tutor"`) || strings.Contains(page.body, cardEmail) {
		t.Fatalf("the invitation page answered %d %s", page.status, page.body)
	}
	if withdrawn.status != http.StatusNoContent || afterWithdrawal.status != http.StatusNotFound || h.revokeReason(t, secondToken) != "withdrawn" {
		t.Fatalf("withdrawn %d, read afterwards %d", withdrawn.status, afterWithdrawal.status)
	}
	logs := h.logs.String()
	for _, private := range []string{cardName, "Maria R.", cardEmail, strings.ToUpper(cardEmail), cardNotes, firstToken, secondToken} {
		if strings.Contains(logs, private) {
			t.Fatalf("the logs contain %q", private)
		}
	}
	for _, event := range []string{cardCreated, `"msg":"card.updated"`, `"msg":"invitation.regenerated"`, `"msg":"card.withdrawn"`} {
		if !strings.Contains(logs, event) {
			t.Fatalf("no %s record", event)
		}
	}
	if !strings.Contains(logs, `"fields":"display_name,notes"`) || !strings.Contains(logs, `"relationship_id":"`+card.ID+`"`) ||
		!strings.Contains(logs, `"tutor_id":"`+tutor.String()+`"`) {
		t.Fatalf("the card records lack their fields: %s", logs)
	}
}

func TestTutorCannotReachAnotherTutorsStudentCard(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	keyed := h.createCard(t, ownerToken, cardBody(cardName, cardEmail), "shared-key")
	invited := decodeCard(t, keyed)
	withdrawn := h.newCard(t, ownerToken, "Withdrawn", "")
	h.cardCall(t, ownerToken, http.MethodDelete, withdrawn.ID, "", "")
	active, ended, provisional, pending, removed := h.seedStudent(t, 3), h.seedStudent(t, 4), h.seedStudent(t, 5), h.seedStudent(t, 6), h.seedStudent(t, 7)
	cards := map[string]string{
		"invited":         invited.ID,
		"withdrawn":       withdrawn.ID,
		"active":          h.seedCard(t, owner, 1, domain.RelationshipActive, &active),
		"ended":           h.seedCard(t, owner, 2, domain.RelationshipEnded, &ended),
		"provisional":     h.seedCard(t, owner, 3, domain.RelationshipProvisional, &provisional),
		"reclaim pending": h.seedCard(t, owner, 4, domain.RelationshipReclaimPending, &pending),
		"removed":         h.seedCard(t, owner, 5, domain.RelationshipDiscarded, &removed),
		"merged":          h.seedCard(t, owner, 6, domain.RelationshipDiscarded, nil),
	}
	routes := []struct {
		method string
		suffix string
		body   string
	}{
		{http.MethodGet, "", ""},
		{http.MethodPatch, "", `{"display_name":"Renamed"}`},
		{http.MethodPatch, "", `{"email":"changed@example.org"}`},
		{http.MethodDelete, "", ""},
		{http.MethodPost, "/invitation", ""},
		{http.MethodPost, "/invitation/send", ""},
	}

	for _, route := range routes {
		random := refusal(t, h.cardCall(t, intruder, route.method, randomCardID, route.suffix, route.body))
		for state, id := range cards {
			result := h.cardCall(t, intruder, route.method, id, route.suffix, route.body)

			if got := refusal(t, result); got != random {
				t.Errorf("%s %s on another tutor's %s card answered\n%s\ninstead of\n%s", route.method, route.suffix, state, got, random)
			}
		}
	}
	sameKey := h.createCard(t, intruder, cardBody("The intruder's pupil", ""), "shared-key")
	ownerList := h.send(t, http.MethodGet, "/api/v1/students", "", jsonCall(ownerToken))

	intruderCard := decodeCard(t, sameKey)
	if sameKey.status != http.StatusCreated || intruderCard.ID == invited.ID || intruderCard.DisplayName != "The intruder's pupil" {
		t.Fatalf("another tutor's request with the same key answered %d %s", sameKey.status, sameKey.body)
	}
	if strings.Contains(ownerList.body, intruderCard.ID) || strings.Contains(ownerList.body, "intruder") {
		t.Fatal("the owner sees the other tutor's card")
	}
	if seen := h.cardCall(t, ownerToken, http.MethodGet, intruderCard.ID, "", ""); seen.status != http.StatusNotFound {
		t.Fatalf("the owner reading the other tutor's card answered %d", seen.status)
	}
	if owned := h.cardCall(t, ownerToken, http.MethodDelete, cards["active"], "", ""); owned.status != http.StatusConflict {
		t.Fatalf("the owner withdrawing an active card answered %d", owned.status)
	}
	unchanged := decodeCard(t, h.cardCall(t, ownerToken, http.MethodGet, invited.ID, "", ""))
	if unchanged.DisplayName != cardName || *unchanged.Email != cardEmail || *unchanged.Invitation.URL != *invited.Invitation.URL {
		t.Fatalf("another tutor changed the card: %+v", unchanged)
	}
	if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventInvitationCreated)); queued != 0 {
		t.Fatalf("another tutor queued %d invitation emails", queued)
	}
}

func TestUnverifiedTutorCannotCreateACard(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)

	refused := h.createCard(t, token, cardBody(cardName, cardEmail), "first")
	listed := h.send(t, http.MethodGet, "/api/v1/students", "", jsonCall(token))

	if refused.status != http.StatusForbidden || code(t, refused) != "email_not_verified" {
		t.Fatalf("an unverified tutor creating a card got %d %s", refused.status, refused.body)
	}
	if listed.status != http.StatusForbidden || h.cardCount(t, tutor) != 0 || strings.Contains(h.logs.String(), cardCreated) {
		t.Fatal("an unverified tutor reached the cards")
	}
}

func TestDuplicateCardEmailNamesTheExistingCard(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	existing := h.newCard(t, token, cardName, cardEmail)
	second := h.newCard(t, token, "Another pupil", "another@example.org")

	duplicate := h.createCard(t, token, cardBody("Someone else", " "+strings.ToUpper(cardEmail)), "")
	invalidAndDuplicate := h.createCard(t, token, cardBody("   ", cardEmail), "")
	renamedOnto := h.cardCall(t, token, http.MethodPatch, second.ID, "", fmt.Sprintf(`{"email":%s}`, jsonText(cardEmail)))
	otherTutor := h.createCard(t, other, cardBody(cardName, cardEmail), "")

	for name, refused := range map[string]response{"a new card": duplicate, "an invalid new card": invalidAndDuplicate, "an email change": renamedOnto} {
		if refused.status != http.StatusConflict || code(t, refused) != "student_exists" || errorDetails(t, refused)["student_id"] != existing.ID {
			t.Errorf("%s with a taken email answered %d %s", name, refused.status, refused.body)
		}
	}
	if otherTutor.status != http.StatusCreated || h.cardCount(t, tutor) != 2 {
		t.Fatalf("another tutor's card with the same email answered %d %s", otherTutor.status, otherTutor.body)
	}
}

func TestCardLimitRefusesThe51stCardOfTheDay(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for index := range 50 {
		h.newCard(t, token, fmt.Sprintf("Student %d", index), "")
	}

	fiftyFirst := h.createCard(t, token, cardBody("One too many", ""), "")
	invalid := h.createCard(t, token, cardBody("\u0007", ""), "")
	h.advance(t, 15*time.Hour)
	nextDay := h.createCard(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), cardBody("Tomorrow", ""), "")

	if fiftyFirst.status != http.StatusTooManyRequests || code(t, fiftyFirst) != "rate_limited" || fiftyFirst.headers.Get("Retry-After") != strconv.Itoa(int((15*time.Hour).Seconds())) {
		t.Fatalf("the 51st card answered %d %v %s", fiftyFirst.status, fiftyFirst.headers, fiftyFirst.body)
	}
	if invalid.status != http.StatusUnprocessableEntity || fields(t, invalid)[0] != "body.display_name:invalid" {
		t.Fatalf("an invalid card over the limit answered %d %s", invalid.status, invalid.body)
	}
	if nextDay.status != http.StatusCreated || h.cardCount(t, tutor) != 51 {
		t.Fatalf("the first card of the next day answered %d %s", nextDay.status, nextDay.body)
	}
}

func TestCardEmailChangesAreLimitedPerDay(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	for index := range 50 {
		changed := h.cardCall(t, token, http.MethodPatch, card.ID, "", fmt.Sprintf(`{"email":"change-%d@example.org"}`, index))
		if changed.status != http.StatusOK {
			t.Fatalf("email change %d answered %d %s", index+1, changed.status, changed.body)
		}
	}

	fiftyFirst := h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"email":"one-too-many@example.org"}`)
	renamed := h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"display_name":"Still editable"}`)

	if fiftyFirst.status != http.StatusTooManyRequests || code(t, fiftyFirst) != "rate_limited" {
		t.Fatalf("the 51st email change answered %d %s", fiftyFirst.status, fiftyFirst.body)
	}
	if renamed.status != http.StatusOK || *decodeCard(t, renamed).Email != "change-49@example.org" {
		t.Fatalf("a rename over the email limit answered %d %s", renamed.status, renamed.body)
	}
}

func TestRepeatedIdempotencyKeyReturnsTheFirstCard(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")

	first := h.createCard(t, token, cardBody(cardName, cardEmail), "create-maria-1")
	repeated := h.createCard(t, token, cardBody("A different body", "different@example.org"), "create-maria-1")
	fresh := h.createCard(t, token, cardBody("Another pupil", ""), "create-maria-2")
	otherTutor := h.createCard(t, other, cardBody(cardName, cardEmail), "create-maria-1")
	malformedKey := h.createCard(t, token, cardBody("Spaced key", ""), "has a space")

	original, again := decodeCard(t, first), decodeCard(t, repeated)
	if first.status != http.StatusCreated || repeated.status != http.StatusCreated || again.ID != original.ID || again.DisplayName != cardName {
		t.Fatalf("the repeated key answered %d %s after %s", repeated.status, repeated.body, first.body)
	}
	if *again.Invitation.URL != *original.Invitation.URL || fresh.status != http.StatusCreated || decodeCard(t, fresh).ID == original.ID {
		t.Fatalf("a new key answered %d %s", fresh.status, fresh.body)
	}
	if otherTutor.status != http.StatusCreated || decodeCard(t, otherTutor).ID == original.ID || h.cardCount(t, tutor) != 2 {
		t.Fatalf("another tutor's key answered %d %s", otherTutor.status, otherTutor.body)
	}
	if malformedKey.status != http.StatusUnprocessableEntity || strings.Count(h.logs.String(), cardCreated) != 3 {
		t.Fatalf("a key with a space answered %d %s", malformedKey.status, malformedKey.body)
	}
}

func TestRepeatedWithdrawAnswers204(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	link := linkToken(t, card.Invitation)

	first := h.cardCall(t, token, http.MethodDelete, card.ID, "", "")
	second := h.cardCall(t, token, http.MethodDelete, card.ID, "", "")
	recreated := h.createCard(t, token, cardBody(cardName, cardEmail), "")

	if first.status != http.StatusNoContent || second.status != http.StatusNoContent || first.body != "" || second.body != "" {
		t.Fatalf("withdrawals answered %d and %d", first.status, second.status)
	}
	if h.invitationPage(t, link).status != http.StatusNotFound || h.ciphertextKept(t, link) {
		t.Fatal("the withdrawn card's link still works")
	}
	if recreated.status != http.StatusCreated || strings.Count(h.logs.String(), `"msg":"card.withdrawn"`) != 1 {
		t.Fatalf("a new card with the withdrawn card's email answered %d %s", recreated.status, recreated.body)
	}
}

func TestWithdrawingALinkedCardIsRefused(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	students := []domain.UserID{h.seedStudent(t, 2), h.seedStudent(t, 3), h.seedStudent(t, 4), h.seedStudent(t, 5), h.seedStudent(t, 6)}
	linked := map[string]string{
		"provisional":     h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &students[0]),
		"reclaim pending": h.seedCard(t, tutor, 2, domain.RelationshipReclaimPending, &students[1]),
		"active":          h.seedCard(t, tutor, 3, domain.RelationshipActive, &students[2]),
		"ended":           h.seedCard(t, tutor, 4, domain.RelationshipEnded, &students[3]),
	}
	discarded := map[string]string{
		"merged":  h.seedCard(t, tutor, 5, domain.RelationshipDiscarded, nil),
		"removed": h.seedCard(t, tutor, 6, domain.RelationshipDiscarded, &students[4]),
	}
	random := refusal(t, h.cardCall(t, token, http.MethodDelete, randomCardID, "", ""))

	for state, id := range linked {
		refused := h.cardCall(t, token, http.MethodDelete, id, "", "")

		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_active" {
			t.Errorf("withdrawing a %s card answered %d %s", state, refused.status, refused.body)
		}
	}
	for reason, id := range discarded {
		refused := h.cardCall(t, token, http.MethodDelete, id, "", "")

		if got := refusal(t, refused); got != random {
			t.Errorf("withdrawing a %s card answered\n%s\ninstead of\n%s", reason, got, random)
		}
	}
	if strings.Contains(h.logs.String(), `"msg":"card.withdrawn"`) {
		t.Fatal("a refused withdrawal was recorded")
	}
}

func TestLinkedCardsRefuseEmailChangesAndOnlyEndedCardsRegenerate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	students := []domain.UserID{h.seedStudent(t, 2), h.seedStudent(t, 3), h.seedStudent(t, 4), h.seedStudent(t, 5)}
	provisional := h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &students[0])
	pending := h.seedCard(t, tutor, 2, domain.RelationshipReclaimPending, &students[1])
	active := h.seedCard(t, tutor, 3, domain.RelationshipActive, &students[2])
	ended := h.seedCard(t, tutor, 4, domain.RelationshipEnded, &students[3])

	emailChange := h.cardCall(t, token, http.MethodPatch, active, "", `{"email":"new@example.org"}`)
	rename := h.cardCall(t, token, http.MethodPatch, active, "", `{"display_name":"Renamed","email":null}`)
	regenerations := map[string]response{
		"provisional":     h.cardCall(t, token, http.MethodPost, provisional, "/invitation", ""),
		"reclaim pending": h.cardCall(t, token, http.MethodPost, pending, "/invitation", ""),
		"active":          h.cardCall(t, token, http.MethodPost, active, "/invitation", ""),
	}
	endedLink := h.cardCall(t, token, http.MethodPost, ended, "/invitation", "")
	endedCard := decodeCard(t, h.cardCall(t, token, http.MethodGet, ended, "", ""))
	pendingCard := decodeCard(t, h.cardCall(t, token, http.MethodGet, pending, "", ""))
	activeCard := decodeCard(t, h.cardCall(t, token, http.MethodGet, active, "", ""))

	if emailChange.status != http.StatusConflict || code(t, emailChange) != "not_editable" {
		t.Fatalf("changing a linked card's email answered %d %s", emailChange.status, emailChange.body)
	}
	if rename.status != http.StatusOK || decodeCard(t, rename).DisplayName != "Renamed" {
		t.Fatalf("renaming a linked card answered %d %s", rename.status, rename.body)
	}
	for state, refused := range regenerations {
		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_active" {
			t.Errorf("regenerating a %s card's link answered %d %s", state, refused.status, refused.body)
		}
	}
	var bound *domain.UserID
	link := linkToken(t, endedCard.Invitation)
	err := h.database.Pool.QueryRow(context.Background(), "SELECT bound_student_id FROM invitations WHERE token_hash = $1", tokenHash(t, link)).Scan(&bound)
	if err != nil {
		t.Fatal(err)
	}
	if endedLink.status != http.StatusOK || bound == nil || *bound != students[3] {
		t.Fatalf("the ended card's link answered %d and is bound to %v", endedLink.status, bound)
	}
	if pendingCard.Account != nil || activeCard.Account == nil || activeCard.Account.Email != "student-4@example.org" || !activeCard.Account.EmailConfirmed {
		t.Fatalf("the cards show accounts %+v and %+v", pendingCard.Account, activeCard.Account)
	}
}

func TestInvalidInvitationTokensAnswerIdentically(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blockedTutor, blockedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "blocked-tutor@example.org")
	deletedTutor, deletedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "deleted-tutor@example.org")
	student := h.seedStudent(t, 4)
	valid := linkToken(t, h.newCard(t, token, "Valid", "").Invitation)
	expiring := linkToken(t, h.newCard(t, token, "Expiring", "").Invitation)
	used := linkToken(t, h.newCard(t, token, "Used", "").Invitation)
	if _, err := h.store.Repositories().Invitations.Consume(context.Background(), tokenHash(t, used), student, authStart); err != nil {
		t.Fatal(err)
	}
	regenerated := h.newCard(t, token, "Regenerated", "")
	h.cardCall(t, token, http.MethodPost, regenerated.ID, "/invitation", "")
	withdrawn := h.newCard(t, token, "Withdrawn", "")
	h.cardCall(t, token, http.MethodDelete, withdrawn.ID, "", "")
	blocked := linkToken(t, h.newCard(t, blockedToken, "Blocked tutor's", "").Invitation)
	deleted := linkToken(t, h.newCard(t, deletedToken, "Deleted tutor's", "").Invitation)
	if _, err := h.store.Repositories().Users.Block(context.Background(), blockedTutor, authStart); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deletedTutor, authStart); err != nil {
		t.Fatal(err)
	}
	unknown := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	answers := map[string]response{
		"unknown":       h.invitationPage(t, unknown),
		"used":          h.invitationPage(t, used),
		"revoked":       h.invitationPage(t, linkToken(t, regenerated.Invitation)),
		"withdrawn":     h.invitationPage(t, linkToken(t, withdrawn.Invitation)),
		"blocked tutor": h.invitationPage(t, blocked),
		"deleted tutor": h.invitationPage(t, deleted),
		"malformed":     h.invitationPage(t, "not-a-token"),
		"short":         h.invitationPage(t, base64.RawURLEncoding.EncodeToString(make([]byte, 31))),
		"padded":        h.invitationPage(t, valid+"="),
	}
	stillValid := h.invitationPage(t, expiring)
	h.advance(t, 14*24*time.Hour)
	answers["expired"] = h.invitationPage(t, expiring)

	expected := refusal(t, answers["unknown"])
	for cause, answer := range answers {
		if answer.status != http.StatusNotFound || refusal(t, answer) != expected {
			t.Errorf("the %s token answered\n%s\ninstead of\n%s", cause, refusal(t, answer), expected)
		}
	}
	if stillValid.status != http.StatusOK {
		t.Fatalf("the link answered %d before it expired", stillValid.status)
	}
}

func TestCardEmailChangeIssuesAFreshLink(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	original := linkToken(t, card.Invitation)
	h.advance(t, time.Hour)
	token = accessToken(t, h.login(t, tutorEmail, accountPassword))

	changed := h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"email":"Corrected@Example.org"}`)
	changedCard := decodeCard(t, changed)
	fresh := linkToken(t, changedCard.Invitation)
	freshPage := h.invitationPage(t, fresh)
	cleared := h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"email":null}`)
	unchanged := h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"email":null}`)

	clearedCard := decodeCard(t, cleared)
	last := linkToken(t, clearedCard.Invitation)
	if changed.status != http.StatusOK || *changedCard.Email != "corrected@example.org" || fresh == original {
		t.Fatalf("changing the email answered %d %s", changed.status, changed.body)
	}
	if h.revokeReason(t, original) != "email_changed" || h.ciphertextKept(t, original) || h.invitationPage(t, original).status != http.StatusNotFound {
		t.Fatal("the link of the old email still works")
	}
	if !changedCard.Invitation.ExpiresAt.Equal(authStart.Add(time.Hour+14*24*time.Hour)) || freshPage.status != http.StatusOK {
		t.Fatalf("the fresh link does not work: %s", changed.body)
	}
	if cleared.status != http.StatusOK || clearedCard.Email != nil || last == fresh || h.revokeReason(t, fresh) != "email_changed" {
		t.Fatalf("clearing the email answered %d %s", cleared.status, cleared.body)
	}
	if unchanged.status != http.StatusOK || linkToken(t, decodeCard(t, unchanged).Invitation) != last {
		t.Fatalf("repeating the same email answered %d %s", unchanged.status, unchanged.body)
	}
}

func TestResponsesCarryingALinkAreNotStored(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	created := h.createCard(t, token, cardBody(cardName, cardEmail), "key")
	card := decodeCard(t, created)

	answers := map[string]response{
		"create":          created,
		"repeated create": h.createCard(t, token, cardBody(cardName, cardEmail), "key"),
		"read":            h.cardCall(t, token, http.MethodGet, card.ID, "", ""),
		"edit":            h.cardCall(t, token, http.MethodPatch, card.ID, "", `{"notes":"x"}`),
		"regenerate":      h.cardCall(t, token, http.MethodPost, card.ID, "/invitation", ""),
		"list":            h.send(t, http.MethodGet, "/api/v1/students", "", jsonCall(token)),
	}
	link := linkToken(t, decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", "")).Invitation)
	answers["invitation page"] = h.invitationPage(t, link)

	for name, answer := range answers {
		if answer.status >= http.StatusBadRequest || answer.headers.Get("Cache-Control") != "no-store" {
			t.Errorf("the %s response answered %d with Cache-Control %q", name, answer.status, answer.headers.Get("Cache-Control"))
		}
	}
	page := answers["invitation page"]
	if page.headers.Get("Referrer-Policy") != "no-referrer" || page.status != http.StatusOK {
		t.Fatalf("the invitation page answered %d with Referrer-Policy %q", page.status, page.headers.Get("Referrer-Policy"))
	}
}

func TestExpiredInvitationsLoseTheirCiphertext(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	old := h.newCard(t, token, "Invited first", "")
	h.advance(t, 2*24*time.Hour)
	recent := h.newCard(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), "Invited later", "")
	oldLink, recentLink := linkToken(t, old.Invitation), linkToken(t, recent.Invitation)
	h.at(t, authStart.Add(14*24*time.Hour+time.Hour))

	h.tick(t)

	if h.ciphertextKept(t, oldLink) || !h.ciphertextKept(t, recentLink) {
		t.Fatal("the job did not clear exactly the expired invitation's ciphertext")
	}
	expired := decodeCard(t, h.cardCall(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), http.MethodGet, old.ID, "", ""))
	if expired.Invitation == nil || expired.Invitation.Status != "expired" || expired.Invitation.URL != nil {
		t.Fatalf("the expired card shows %+v", expired.Invitation)
	}
	if !strings.Contains(h.logs.String(), `"job":"invitations.expire","result":"ok","batch_size":1`) {
		t.Fatalf("no job.run record for invitations.expire: %s", h.logs.String())
	}
}

func TestInvitationTokensNeverReachAccessRecords(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := linkToken(t, h.newCard(t, token, cardName, "").Invitation)
	malformed := "sentinel-malformed-token"

	valid := h.invitationPage(t, link)
	refused := h.invitationPage(t, malformed)

	logs := h.logs.String()
	records := accessRecordsFor(t, logs, "GET /api/v1/invitations/{token}")
	if valid.status != http.StatusOK || refused.status != http.StatusNotFound || len(records) != 2 {
		t.Fatalf("expected two access records with the route template, got %d", len(records))
	}
	for _, secret := range []string{link, malformed, link[:16]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain an invitation token")
		}
	}
}

func TestStudentListFiltersSearchesAndPages(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	students := []domain.UserID{h.seedStudent(t, 2), h.seedStudent(t, 3), h.seedStudent(t, 4)}
	h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &students[0])
	h.seedCard(t, tutor, 2, domain.RelationshipReclaimPending, &students[1])
	h.seedCard(t, tutor, 3, domain.RelationshipEnded, &students[2])
	h.seedCard(t, tutor, 4, domain.RelationshipDiscarded, nil)
	var invited []string
	for index, name := range []string{"Anna 100%", "Boris_B", "Clara", "Denis", "Elena"} {
		h.advance(t, time.Minute)
		invited = append(invited, h.newCard(t, token, name, fmt.Sprintf("card-%d@example.org", index)).ID)
	}
	list := func(query string) response {
		return h.send(t, http.MethodGet, "/api/v1/students"+query, "", jsonCall(token))
	}
	decodePage := func(result response) cardPage {
		var page cardPage
		if err := json.Unmarshal([]byte(result.body), &page); err != nil || result.status != http.StatusOK {
			t.Fatalf("not a page: %d %s", result.status, result.body)
		}
		return page
	}

	var walked []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page := decodePage(list("?state=invited&limit=2" + cursor))
		for _, item := range page.Items {
			walked = append(walked, item.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	all := decodePage(list(""))
	active := decodePage(list("?state=active"))
	ended := decodePage(list("?state=ended"))
	percent := decodePage(list("?q=" + url.QueryEscape("0%")))
	underscore := decodePage(list("?q=" + url.QueryEscape("s_b")))
	byEmail := decodePage(list("?q=CARD-3"))
	badRequests := []response{list("?cursor=not-a-cursor"), list("?state=discarded"), list("?group=nope"), list("?limit=101"), list("?limit=0")}

	if strings.Join(walked, ",") != strings.Join([]string{invited[4], invited[3], invited[2], invited[1], invited[0]}, ",") {
		t.Fatalf("the pages walked %v instead of the invited cards newest first", walked)
	}
	if len(all.Items) != 8 || all.NextCursor != nil || all.Items[0].Notes != nil || len(active.Items) != 2 || len(ended.Items) != 1 {
		t.Fatalf("the filters listed %d, %d active and %d ended cards", len(all.Items), len(active.Items), len(ended.Items))
	}
	if len(percent.Items) != 1 || percent.Items[0].DisplayName != "Anna 100%" || len(underscore.Items) != 1 || underscore.Items[0].DisplayName != "Boris_B" {
		t.Fatalf("the search did not treat %% and _ literally: %+v %+v", percent.Items, underscore.Items)
	}
	if len(byEmail.Items) != 1 || byEmail.Items[0].ID != invited[3] {
		t.Fatalf("the search by card email found %+v", byEmail.Items)
	}
	for _, refused := range badRequests {
		if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "validation_failed" {
			t.Errorf("a bad filter answered %d %s", refused.status, refused.body)
		}
	}
}

func TestInvitationRegenerationLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)
	other := h.newCard(t, token, "Another pupil", "")
	for index := range 50 {
		if regenerated := h.cardCall(t, token, http.MethodPost, card.ID, "/invitation", ""); regenerated.status != http.StatusOK {
			t.Fatalf("regeneration %d answered %d %s", index+1, regenerated.status, regenerated.body)
		}
	}
	current := linkToken(t, decodeCard(t, h.cardCall(t, token, http.MethodGet, card.ID, "", "")).Invitation)

	fiftyFirst := h.cardCall(t, token, http.MethodPost, other.ID, "/invitation", "")
	otherLinkKept := h.ciphertextKept(t, linkToken(t, other.Invitation))
	h.advance(t, 15*time.Hour)
	nextDay := h.cardCall(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), http.MethodPost, other.ID, "/invitation", "")

	if fiftyFirst.status != http.StatusTooManyRequests || code(t, fiftyFirst) != "rate_limited" || fiftyFirst.headers.Get("Retry-After") != strconv.Itoa(int((15*time.Hour).Seconds())) {
		t.Fatalf("the 51st regeneration answered %d %v %s", fiftyFirst.status, fiftyFirst.headers, fiftyFirst.body)
	}
	if fiftyFirst.headers.Get("Cache-Control") != "no-store" || !otherLinkKept {
		t.Fatal("the refused regeneration changed the other card's link")
	}
	if h.revokeReason(t, current) != "" || strings.Count(h.logs.String(), `"msg":"invitation.regenerated"`) != 51 {
		t.Fatal("expected 51 regenerations and the last link still open")
	}
	if nextDay.status != http.StatusOK {
		t.Fatalf("the first regeneration of the next day answered %d %s", nextDay.status, nextDay.body)
	}
}

func TestCreatedCardReadsBackUnchanged(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.at(t, authStart.Add(1234567*time.Nanosecond))
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))

	created := h.createCard(t, token, cardBody(cardName, cardEmail), "")
	read := h.cardCall(t, token, http.MethodGet, decodeCard(t, created).ID, "", "")

	createdFields, readFields := rawFields(t, created), rawFields(t, read)
	nextLessons, carried := readFields["next_lessons"]
	delete(readFields, "next_lessons")
	if !carried || string(nextLessons) != "[]" || !maps.EqualFunc(createdFields, readFields, slices.Equal) {
		t.Fatalf("the created card\n%s\nreads back as\n%s", created.body, read.body)
	}
}

func rawFields(t *testing.T, result response) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &fields); err != nil {
		t.Fatalf("not a JSON object: %d %s", result.status, result.body)
	}
	return fields
}

type concurrentAnswers struct {
	statuses map[int]int
	ids      map[string]bool
}

func (h *authHarness) createConcurrently(t *testing.T, token string, attempts int, body func(int) string, key string) concurrentAnswers {
	t.Helper()
	start := make(chan struct{})
	results := make(chan response, attempts)
	failures := make(chan error, attempts)
	for index := range attempts {
		go func() {
			<-start
			headers := jsonCall(token)
			if key != "" {
				headers["Idempotency-Key"] = key
			}
			result, err := h.trySend(http.MethodPost, "/api/v1/students", body(index), headers)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	answers := concurrentAnswers{statuses: map[int]int{}, ids: map[string]bool{}}
	for range attempts {
		select {
		case err := <-failures:
			t.Fatal(err)
		case result := <-results:
			answers.statuses[result.status]++
			if result.status == http.StatusCreated {
				answers.ids[decodeCard(t, result).ID] = true
			}
		case <-time.After(pauseDeadline):
			t.Fatal("the concurrent creations did not finish")
		}
	}
	return answers
}

func TestConcurrentCardCreationsStopAtTheLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	_, third := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "third-tutor@example.org")

	limited := h.createConcurrently(t, token, 60, func(index int) string { return cardBody(fmt.Sprintf("Student %d", index), "") }, "")
	sameEmail := h.createConcurrently(t, other, 10, func(int) string { return cardBody(cardName, cardEmail) }, "")
	sameKey := h.createConcurrently(t, third, 10, func(int) string { return cardBody(cardName, cardEmail) }, "one-key")

	if limited.statuses[http.StatusCreated] != 50 || limited.statuses[http.StatusTooManyRequests] != 10 || len(limited.statuses) != 2 || h.cardCount(t, tutor) != 50 {
		t.Fatalf("60 concurrent creations answered %v", limited.statuses)
	}
	if sameEmail.statuses[http.StatusCreated] != 1 || sameEmail.statuses[http.StatusConflict] != 9 || len(sameEmail.statuses) != 2 {
		t.Fatalf("10 concurrent creations with one email answered %v", sameEmail.statuses)
	}
	if sameKey.statuses[http.StatusCreated] != 10 || len(sameKey.ids) != 1 {
		t.Fatalf("10 concurrent creations with one key answered %v with %d cards", sameKey.statuses, len(sameKey.ids))
	}
}

func TestCardNotesRefuseControlCharactersOtherThanLineFeedAndTab(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.newCard(t, token, cardName, cardEmail)

	refused := map[string]response{}
	for name, notes := range map[string]string{"a NUL byte": "before\u0000after", "a bell": "ring\u0007", "a carriage return": "one\r\ntwo", "a C1 control": "next\u0085line"} {
		refused[name] = h.cardCall(t, token, http.MethodPatch, card.ID, "", fmt.Sprintf(`{"notes":%s}`, jsonText(notes)))
	}
	kept := h.cardCall(t, token, http.MethodPatch, card.ID, "", fmt.Sprintf(`{"notes":%s}`, jsonText("first line\n\tindented")))

	for name, result := range refused {
		if result.status != http.StatusUnprocessableEntity || code(t, result) != "validation_failed" || !slices.Equal(fields(t, result), []string{"body.notes:invalid"}) {
			t.Fatalf("notes with %s answered %d %s", name, result.status, result.body)
		}
	}
	if notes := decodeCard(t, kept).Notes; kept.status != http.StatusOK || notes == nil || *notes != "first line\n\tindented" {
		t.Fatalf("notes with a line feed and a tab answered %d %s", kept.status, kept.body)
	}
	if strings.Contains(h.logs.String(), `"level":"ERROR"`) {
		t.Fatalf("a refused note was logged as an error:\n%s", h.logs.String())
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
)

const (
	ownerPassword        = "mailbox owner password"
	reclaimPath          = "/api/v1/users/me/reclaim"
	reclaimDecided       = `"msg":"relationship.reclaim_decided"`
	reclaimPendingRecord = `"msg":"relationship.reclaim_pending"`
	squatterEmail        = "squatted@example.org"
)

type reclaimTutor struct {
	Name     string   `json:"name"`
	AvatarID *string  `json:"avatar_id"`
	Subjects []string `json:"subjects"`
}

type reclaimLink struct {
	RelationshipID string       `json:"relationship_id"`
	Tutor          reclaimTutor `json:"tutor"`
}

type reclaimRequest struct {
	RequestID string       `json:"request_id"`
	Tutor     reclaimTutor `json:"tutor"`
}

type reclaimList struct {
	Links    []reclaimLink    `json:"links"`
	Requests []reclaimRequest `json:"requests"`
	ChooseBy *time.Time       `json:"choose_by"`
}

type reclaimChoice struct {
	KeepLinks    []string `json:"keep_links,omitempty"`
	DropLinks    []string `json:"drop_links,omitempty"`
	KeepRequests []string `json:"keep_requests,omitempty"`
	DropRequests []string `json:"drop_requests,omitempty"`
}

func (c reclaimChoice) body(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

type storedRequest struct {
	State       string
	Suppressed  string
	OwnerChoice bool
}

func (h *authHarness) squat(t *testing.T, tutorToken string, cardEmail string, email string) (studentCard, domain.UserID) {
	t.Helper()
	card := h.newCard(t, tutorToken, cardName, cardEmail)
	if accepted := h.acceptInvitation(t, linkToken(t, card.Invitation), email); accepted.status != http.StatusAccepted {
		t.Fatalf("the sign-up answered %d %s", accepted.status, accepted.body)
	}
	return card, h.userID(t, email)
}

func (h *authHarness) confirmFromAnotherBrowser(t *testing.T, email string, bearer string) response {
	t.Helper()
	h.tick(t)
	token := latestToken(t, h.mailTo(t, email), confirmSubject)
	return h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, token, ownerPassword), bearer)
}

func (h *authHarness) reclaimed(t *testing.T, email string) string {
	t.Helper()
	confirmed := h.confirmFromAnotherBrowser(t, email, "")
	if confirmed.status != http.StatusOK || outcomeOf(t, confirmed) != "choice_required" {
		t.Fatalf("the reclaim answered %d %s", confirmed.status, confirmed.body)
	}
	return accessToken(t, confirmed)
}

func (h *authHarness) resetToken(t *testing.T, email string) string {
	t.Helper()
	h.advance(t, 61*time.Second)
	h.forgot(t, email)
	h.tick(t)
	return latestToken(t, h.mailTo(t, email), resetSubject)
}

func (h *authHarness) reset(t *testing.T, token string) response {
	t.Helper()
	return h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":%q}`, token, ownerPassword), nil)
}

func (h *authHarness) sendReset(token string) <-chan pendingAnswer {
	return h.sendInBackground(http.MethodPost, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":%q}`, token, ownerPassword), signUpHeaders())
}

func (h *authHarness) ownerSignIn(t *testing.T, email string) string {
	t.Helper()
	return accessToken(t, h.login(t, email, ownerPassword))
}

func outcomeOf(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		Outcome *string `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("not an outcome: %d %s", result.status, result.body)
	}
	if decoded.Outcome == nil {
		return ""
	}
	return *decoded.Outcome
}

func (h *authHarness) reclaimList(t *testing.T, token string) (response, reclaimList) {
	t.Helper()
	result := h.send(t, http.MethodGet, reclaimPath, "", map[string]string{"Authorization": "Bearer " + token})
	var list reclaimList
	if err := json.Unmarshal([]byte(result.body), &list); err != nil || result.status != http.StatusOK || list.Links == nil || list.Requests == nil {
		t.Fatalf("not a reclaim list: %d %s", result.status, result.body)
	}
	return result, list
}

func (h *authHarness) decideReclaim(t *testing.T, token string, choice reclaimChoice) response {
	t.Helper()
	return h.send(t, http.MethodPost, reclaimPath, choice.body(t), jsonCall(token))
}

func (h *authHarness) reclaimDeadline(t *testing.T, user domain.UserID) *time.Time {
	t.Helper()
	var deadline *time.Time
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT reclaim_until FROM users WHERE id = $1", user).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	return deadline
}

func (h *authHarness) meReclaimPending(t *testing.T, token string) bool {
	t.Helper()
	result := h.me(t, token)
	var decoded struct {
		ReclaimPending *bool `json:"reclaim_pending"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.ReclaimPending == nil {
		t.Fatalf("not an account: %d %s", result.status, result.body)
	}
	return *decoded.ReclaimPending
}

func (h *authHarness) storedRequest(t *testing.T, id domain.JoinRequestID) storedRequest {
	t.Helper()
	var row storedRequest
	err := h.database.Pool.QueryRow(context.Background(), "SELECT state, suppressed, owner_choice_pending FROM join_requests WHERE id = $1", id).
		Scan(&row.State, &row.Suppressed, &row.OwnerChoice)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *authHarness) now(t *testing.T) time.Time {
	t.Helper()
	now, err := h.clock.Instant(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return now
}

func requestText(id domain.JoinRequestID) string {
	return id.String()
}

func sameInstant(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want)
}

func TestReclaimDoesNotActivateALinkWithoutAcceptance(t *testing.T) {
	reclaims := map[string]func(t *testing.T, h *authHarness) response{
		"confirmation from another browser": func(t *testing.T, h *authHarness) response {
			t.Helper()
			return h.confirmFromAnotherBrowser(t, newcomerEmail, "")
		},
		"a reset of the unverified account": func(t *testing.T, h *authHarness) response {
			t.Helper()
			return h.reset(t, h.resetToken(t, newcomerEmail))
		},
	}
	for name, reclaim := range reclaims {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)

			reclaimed := reclaim(t, h)
			reclaimedAt := h.now(t)
			h.at(t, reclaimedAt.Add(domain.ReclaimWindow-time.Hour))
			h.tick(t)

			if reclaimed.status != http.StatusOK || outcomeOf(t, reclaimed) != "choice_required" {
				t.Fatalf("the reclaim answered %d %s", reclaimed.status, reclaimed.body)
			}
			if stored := h.storedRelationship(t, card.ID); stored.State != "reclaim_pending" {
				t.Fatalf("after the reclaim the link is %s", stored.State)
			}
			if deadline := h.reclaimDeadline(t, student); !sameInstant(deadline, reclaimedAt.Add(domain.ReclaimWindow)) {
				t.Fatalf("the reclaim deadline is %v", deadline)
			}
			tutor = h.relogin(t, tutorEmail)
			shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))
			if shown.State != "reclaim_pending" || shown.Account != nil {
				t.Fatalf("after the reclaim the card is %+v", shown)
			}
			_, tutors := h.myTutors(t, h.ownerSignIn(t, newcomerEmail))
			if len(tutors) != 0 {
				t.Fatalf("My tutors lists %+v", tutors)
			}
			if !strings.Contains(h.logs.String(), reclaimPendingRecord) {
				t.Fatal("no relationship.reclaim_pending record")
			}
		})
	}
}

func TestReclaimKeepActivatesAndDropRemoves(t *testing.T) {
	t.Run("keep", func(t *testing.T) {
		h := newFlowHarness(t)
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
		owner := h.reclaimed(t, newcomerEmail)

		kept := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}})

		if kept.status != http.StatusOK || strings.TrimSpace(kept.body) != `{"outcome":"linked"}` || kept.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("keeping answered %d %s", kept.status, kept.body)
		}
		shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))
		if shown.State != "active" || shown.Account == nil || !shown.Account.EmailConfirmed {
			t.Fatalf("the kept card is %+v", shown)
		}
		_, tutors := h.myTutors(t, owner)
		if len(tutors) != 1 || tutors[0].RelationshipID != card.ID || tutors[0].State != "active" {
			t.Fatalf("My tutors lists %+v", tutors)
		}
		if h.reclaimDeadline(t, student) != nil || h.meReclaimPending(t, owner) {
			t.Fatal("the deadline or the pending flag outlived the choice")
		}
		logs := h.logs.String()
		if !strings.Contains(logs, reclaimDecided) || !strings.Contains(logs, `"kept":1,"dropped":0,"cause":"owner"`) {
			t.Fatal("no relationship.reclaim_decided record for the keep")
		}
	})
	t.Run("drop an email-less link", func(t *testing.T) {
		h := newFlowHarness(t)
		tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutor, "", newcomerEmail)
		h.withNotes(t, tutor, card.ID, cardNotes)
		owner := h.reclaimed(t, newcomerEmail)

		dropped := h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}})
		h.tick(t)

		if dropped.status != http.StatusOK || strings.TrimSpace(dropped.body) != `{"outcome":null}` {
			t.Fatalf("dropping answered %d %s", dropped.status, dropped.body)
		}
		stored := h.storedRelationship(t, card.ID)
		if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "removed" || stored.Notes != "" {
			t.Fatalf("the dropped link is %+v", stored)
		}
		fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited)
		if len(fresh) != 1 || fresh[0].DisplayName != cardName || fresh[0].Email != nil || fresh[0].Notes == nil || *fresh[0].Notes != cardNotes {
			t.Fatalf("the tutor's new cards are %+v", fresh)
		}
		shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, fresh[0].ID, "", ""))
		if shown.Email != nil || h.invitationPage(t, linkToken(t, shown.Invitation)).status != http.StatusOK {
			t.Fatalf("the new card is %+v", shown)
		}
		if _, tutors := h.myTutors(t, owner); len(tutors) != 0 || h.linksOf(t, student) != 0 || h.reclaimDeadline(t, student) != nil {
			t.Fatalf("the owner still lists %+v", tutors)
		}
		if got := h.mailTo(t, tutorEmail); len(got) != 0 {
			t.Fatalf("the tutor received %v", subjects(got))
		}
		if !strings.Contains(h.logs.String(), relationshipRemoved) || !strings.Contains(h.logs.String(), `"cause":"reclaim"`) {
			t.Fatal("no relationship.removed record with the reclaim as cause")
		}
	})
}

func TestReclaimChoiceMustNameEveryPendingItem(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	request := h.seedJoinRequest(t, otherTutor, student, 1, domain.JoinRequestHeld)
	owner := h.reclaimed(t, newcomerEmail)
	link, held := []string{card.ID}, []string{requestText(request)}

	choices := map[string]struct {
		choice reclaimChoice
		want   []string
	}{
		"nothing named":           {reclaimChoice{}, []string{"body.keep_links:required", "body.keep_requests:required"}},
		"the request left out":    {reclaimChoice{KeepLinks: link}, []string{"body.keep_requests:required"}},
		"a link kept and dropped": {reclaimChoice{KeepLinks: link, DropLinks: link, DropRequests: held}, []string{"body.drop_links:invalid"}},
		"a request kept twice":    {reclaimChoice{DropLinks: link, KeepRequests: append(append([]string{}, held...), held...)}, []string{"body.keep_requests:invalid"}},
	}
	for name, testCase := range choices {
		refused := h.decideReclaim(t, owner, testCase.choice)

		if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "validation_failed" || !slices.Equal(fields(t, refused), testCase.want) {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	if stored := h.storedRelationship(t, card.ID); stored.State != "reclaim_pending" {
		t.Fatalf("a refused choice changed the link: %s", stored.State)
	}
	if stored := h.storedRequest(t, request); stored.State != "held" || !stored.OwnerChoice || !h.meReclaimPending(t, owner) {
		t.Fatalf("a refused choice changed the request: %+v", stored)
	}
}

func TestRepeatedReclaimChoiceAnswersTheCurrentState(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	choice := reclaimChoice{KeepLinks: []string{card.ID}}

	first := h.decideReclaim(t, owner, choice)
	repeated := h.decideReclaim(t, owner, choice)
	reversed := h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}})
	random := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{randomCardID}})
	_, list := h.reclaimList(t, owner)

	if outcomeOf(t, first) != "linked" {
		t.Fatalf("the first choice answered %d %s", first.status, first.body)
	}
	for name, answer := range map[string]response{"the repeat": repeated, "a reversed repeat": reversed, "a random identifier": random} {
		if answer.status != http.StatusOK || strings.TrimSpace(answer.body) != `{"outcome":null}` {
			t.Fatalf("%s answered %d %s", name, answer.status, answer.body)
		}
	}
	if h.storedRelationship(t, card.ID).State != "active" || len(list.Links) != 0 || len(list.Requests) != 0 || list.ChooseBy != nil {
		t.Fatalf("a repeat changed the decided link or left items: %+v", list)
	}
	if strings.Count(h.logs.String(), reclaimDecided) != 1 {
		t.Fatal("a repeat recorded another decision")
	}
}

func TestReclaimChoiceRefusesItemsOfAnotherAccount(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	own, _ := h.squat(t, tutor, "", newcomerEmail)
	foreign, stranger := h.squat(t, tutor, "", squatterEmail)
	foreignRequest := h.seedJoinRequest(t, otherTutor, stranger, 1, domain.JoinRequestHeld)
	owner := h.reclaimed(t, newcomerEmail)
	strangerToken := h.reclaimed(t, squatterEmail)

	answers := map[string]response{
		"another account's link":                      h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{own.ID, foreign.ID}}),
		"another account's request":                   h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{own.ID}, KeepRequests: []string{requestText(foreignRequest)}}),
		"another account's link with an item missing": h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{foreign.ID}}),
		"a malformed identifier":                      h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{own.ID, "not-an-identifier"}}),
	}
	random := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{own.ID, randomCardID}})

	for name, answer := range answers {
		if answer.status != http.StatusNotFound || refusal(t, answer) != refusal(t, random) {
			t.Errorf("%s answered %d %s instead of %s", name, answer.status, answer.body, random.body)
		}
	}
	for _, card := range []studentCard{own, foreign} {
		if stored := h.storedRelationship(t, card.ID); stored.State != "reclaim_pending" {
			t.Fatalf("a refused choice changed a link: %s", stored.State)
		}
	}
	if h.storedRequest(t, foreignRequest).State != "held" || !h.meReclaimPending(t, owner) || !h.meReclaimPending(t, strangerToken) {
		t.Fatal("a refused choice decided an item")
	}
}

func TestKeepingALinkOfAnInactiveTutorDropsIt(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	if _, err := h.store.Repositories().Users.Block(context.Background(), tutorID, authStart); err != nil {
		t.Fatal(err)
	}

	kept := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}})

	if kept.status != http.StatusOK || strings.TrimSpace(kept.body) != `{"outcome":null}` {
		t.Fatalf("keeping a blocked tutor's link answered %d %s", kept.status, kept.body)
	}
	stored := h.storedRelationship(t, card.ID)
	if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "removed" || h.linksOf(t, student) != 0 {
		t.Fatalf("the link of the blocked tutor is %+v", stored)
	}
	if fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited); len(fresh) != 1 {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
	if !strings.Contains(h.logs.String(), `"kept":0,"dropped":1,"cause":"owner"`) {
		t.Fatal("the keep of an inactive tutor's link was not recorded as a drop")
	}
}

func TestSignedInAcceptanceOfTheSameTutorCountsAsAKeep(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first, student := h.linkedStudent(t, tutor, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	_, before := h.myTutors(t, owner)
	pendingBefore := h.meReclaimPending(t, owner)
	second := h.newCard(t, tutor, "A second card", "")

	kept := h.acceptSignedIn(t, owner, linkToken(t, second.Invitation))
	_, after := h.myTutors(t, owner)

	if len(before) != 0 || !pendingBefore {
		t.Fatalf("before the keep My tutors lists %+v and the choice is pending: %t", before, pendingBefore)
	}
	keep := acceptedTutor(t, kept)
	if keep.RelationshipID != first.ID || keep.State != "active" || len(after) != 1 || !sameTutor(t, after[0], keep) {
		t.Fatalf("the keep answered %+v and My tutors lists %+v", keep, after)
	}
	shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, first.ID, "", ""))
	if shown.State != "active" || shown.Account == nil || !shown.Account.EmailConfirmed {
		t.Fatalf("the kept card is %+v", shown)
	}
	if h.storedRelationship(t, second.ID).State != "discarded" || h.linksOf(t, student) != 1 {
		t.Fatal("the second card was not merged")
	}
	if h.reclaimDeadline(t, student) != nil || h.meReclaimPending(t, owner) || !strings.Contains(h.logs.String(), reclaimDecided) {
		t.Fatal("the keep by acceptance left the choice pending or was not recorded")
	}
}

func TestTutorCanRemoveAReclaimPendingLink(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	deadline := h.reclaimDeadline(t, student)

	removed := h.removeStudent(t, tutor, card.ID)
	_, list := h.reclaimList(t, owner)
	repeat := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}})

	fresh := decodeRemoval(t, removed).NewCard
	if removed.status != http.StatusOK || fresh.State != "invited" || fresh.Email == nil || *fresh.Email != newcomerEmail {
		t.Fatalf("the removal answered %d %s", removed.status, removed.body)
	}
	if stored := h.storedRelationship(t, card.ID); stored.State != "discarded" || *stored.DiscardReason != "removed" {
		t.Fatalf("the removed link is %+v", stored)
	}
	if len(list.Links) != 0 || h.meReclaimPending(t, owner) || len(h.cardsOf(t, tutorID, domain.RelationshipInvited)) != 1 {
		t.Fatalf("after the removal the owner still sees %+v", list)
	}
	if outcomeOf(t, repeat) != "" || repeat.status != http.StatusOK {
		t.Fatalf("the owner's choice after the removal answered %d %s", repeat.status, repeat.body)
	}
	if after := h.reclaimDeadline(t, student); deadline == nil || after == nil || !after.Equal(*deadline) {
		t.Fatalf("the tutor's removal wrote the account's deadline: %v then %v", deadline, after)
	}
}

func TestReclaimPendingCardShowsNoAccount(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, "", newcomerEmail)
	h.reclaimed(t, newcomerEmail)

	shown := h.cardCall(t, tutor, http.MethodGet, card.ID, "", "")
	listed := h.send(t, http.MethodGet, studentsPath+"?state=active", "", jsonCall(tutor))

	if got := decodeCard(t, shown); got.State != "reclaim_pending" || got.Account != nil || got.Invitation != nil {
		t.Fatalf("the card is %+v", got)
	}
	var page cardPage
	if err := json.Unmarshal([]byte(listed.body), &page); err != nil || len(page.Items) != 1 || page.Items[0].Account != nil || page.Items[0].State != "reclaim_pending" {
		t.Fatalf("the list is %d %s", listed.status, listed.body)
	}
	for _, answer := range []response{shown, listed} {
		for _, private := range []string{newcomerEmail, studentAccountName} {
			if strings.Contains(answer.body, private) {
				t.Fatalf("the tutor sees %q before the owner kept the link: %s", private, answer.body)
			}
		}
	}
}

func TestReclaimPendingLinkIsHiddenFromMyTutorsAndCannotBeEnded(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, "", newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)

	listed, tutors := h.myTutors(t, owner)
	withdrawn := h.cardCall(t, tutor, http.MethodDelete, card.ID, "", "")
	regenerated := h.cardCall(t, tutor, http.MethodPost, card.ID, "/invitation", "")

	if len(tutors) != 0 || strings.Contains(listed.body, card.ID) {
		t.Fatalf("My tutors lists %s", listed.body)
	}
	for name, refused := range map[string]response{"withdrawing": withdrawn, "regenerating its link": regenerated} {
		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_active" {
			t.Fatalf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	if stored := h.storedRelationship(t, card.ID); stored.State != "reclaim_pending" {
		t.Fatalf("the link is %s", stored.State)
	}
}

func TestTutorDeletionDiscardsReclaimPendingLinksWithoutANewCard(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	request := h.seedJoinRequest(t, tutorID, student, 1, domain.JoinRequestHeld)
	owner := h.reclaimed(t, newcomerEmail)
	deadline := h.reclaimDeadline(t, student)

	deleted := h.deleteMe(t, tutor, accountPassword)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s", deleted.status, deleted.body)
	}
	stored := h.storedRelationship(t, card.ID)
	if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "tutor_deleted" {
		t.Fatalf("the reclaim-pending link is %+v", stored)
	}
	if fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited); len(fresh) != 0 {
		t.Fatalf("the deleted tutor got new cards %+v", fresh)
	}
	if h.storedRequest(t, request).State != "expired" || h.meReclaimPending(t, owner) {
		t.Fatal("the deleted tutor's request still waits for the owner's choice")
	}
	if after := h.reclaimDeadline(t, student); after == nil || !after.Equal(*deadline) {
		t.Fatalf("the tutor's deletion wrote the student's deadline: %v", after)
	}
	if !strings.Contains(h.logs.String(), `"cause":"tutor_deleted"`) {
		t.Fatal("no relationship.removed record with the tutor's deletion as cause")
	}
}

func TestStudentDeletionDropsReclaimPendingLinks(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	request := h.seedJoinRequest(t, otherTutor, student, 1, domain.JoinRequestHeld)
	owner := h.reclaimed(t, newcomerEmail)

	deleted := h.deleteMe(t, owner, ownerPassword)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	stored := h.storedRelationship(t, card.ID)
	if stored.State != "discarded" || *stored.DiscardReason != "removed" || h.linksOf(t, student) != 0 {
		t.Fatalf("the reclaim-pending link is %+v", stored)
	}
	fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited)
	if len(fresh) != 1 || fresh[0].Email == nil || *fresh[0].Email != newcomerEmail {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
	if h.storedRequest(t, request).State != "expired" || h.reclaimDeadline(t, student) != nil {
		t.Fatal("the deletion left the request or the deadline")
	}
	if !strings.Contains(h.logs.String(), relationshipRemoved) || !strings.Contains(h.logs.String(), `"cause":"student"`) {
		t.Fatal("no relationship.removed record with the student as cause")
	}
}

func TestReclaimItemsAreDroppedWhenTheDeadlinePasses(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	request := h.seedJoinRequest(t, otherTutor, student, 1, domain.JoinRequestHeld)
	h.reclaimed(t, newcomerEmail)
	deadline := *h.reclaimDeadline(t, student)

	h.at(t, deadline.Add(-time.Hour))
	h.tick(t)
	beforeDeadline := h.storedRelationship(t, card.ID)
	h.at(t, deadline)
	h.tick(t)

	if beforeDeadline.State != "reclaim_pending" {
		t.Fatalf("before the deadline the link is %s", beforeDeadline.State)
	}
	stored := h.storedRelationship(t, card.ID)
	if stored.State != "discarded" || *stored.DiscardReason != "removed" || h.storedRequest(t, request).State != "expired" {
		t.Fatalf("after the deadline the link is %+v", stored)
	}
	if fresh := h.cardsOf(t, tutorID, domain.RelationshipInvited); len(fresh) != 1 || h.reclaimDeadline(t, student) != nil {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
	if got := h.mailTo(t, tutorEmail); len(got) != 0 {
		t.Fatalf("the tutor received %v", subjects(got))
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"job":"reclaim.expire","result":"ok","batch_size":1`) || !strings.Contains(logs, `"kept":0,"dropped":2,"cause":"timeout"`) {
		t.Fatal("no job.run or relationship.reclaim_decided record for the expiry")
	}
}

func (h *authHarness) tickInBackground(t *testing.T) <-chan error {
	t.Helper()
	now := h.now(t)
	done := make(chan error, 1)
	go func() { done <- h.worker.Tick(context.Background(), now) }()
	return done
}

func awaitTick(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(pauseDeadline):
		t.Fatal("the tick did not finish")
	}
}

func TestReclaimExpiryRacingTheOwnersChoiceHasOneOutcome(t *testing.T) {
	for _, choiceFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("choice first %t", choiceFirst), func(t *testing.T) {
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
			h.reclaimed(t, newcomerEmail)
			h.at(t, *h.reclaimDeadline(t, student))
			owner := h.ownerSignIn(t, newcomerEmail)
			choose := func() <-chan pendingAnswer {
				return h.sendInBackground(http.MethodPost, reclaimPath, reclaimChoice{KeepLinks: []string{card.ID}}.body(t), jsonCall(owner))
			}
			release := h.holdUserRow(t, student)

			var choice <-chan pendingAnswer
			var expiry <-chan error
			if choiceFirst {
				choice = choose()
				h.waitForLockWaiters(t, 1)
				expiry = h.tickInBackground(t)
			} else {
				expiry = h.tickInBackground(t)
				h.waitForLockWaiters(t, 1)
				choice = choose()
			}
			h.waitForLockWaiters(t, 2)
			release()
			chosen := awaitAnswer(t, choice)
			awaitTick(t, expiry)

			stored := h.storedRelationship(t, card.ID)
			if choiceFirst {
				if outcomeOf(t, chosen) != "linked" || stored.State != "active" {
					t.Fatalf("the choice answered %d %s and the link is %s", chosen.status, chosen.body, stored.State)
				}
			} else if chosen.status != http.StatusOK || outcomeOf(t, chosen) != "" || stored.State != "discarded" {
				t.Fatalf("the choice answered %d %s and the link is %s", chosen.status, chosen.body, stored.State)
			}
			if h.reclaimDeadline(t, student) != nil || h.linksOf(t, student) > 1 || strings.Count(h.logs.String(), reclaimDecided) != 1 {
				t.Fatal("the race left a deadline, two links or two decisions")
			}
		})
	}
}

func TestReclaimByResetListsItemsAfterSignIn(t *testing.T) {
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.describeTutor(t, tutorID, "Grammar", []string{"English", "German"})
	avatar := uploadedID(t, h.upload(t, tutor, "me.png", pngBytes(t, 2, 2)))
	if bound := h.patchMe(t, tutor, fmt.Sprintf(`{"avatar_id":%q}`, avatar)); bound.status != http.StatusOK {
		t.Fatalf("binding the tutor's avatar answered %d %s", bound.status, bound.body)
	}
	card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	request := h.seedJoinRequest(t, otherTutor, student, 1, domain.JoinRequestHeld)

	reset := h.reset(t, h.resetToken(t, newcomerEmail))
	resetAt := h.now(t)
	owner := h.ownerSignIn(t, newcomerEmail)
	listed, list := h.reclaimList(t, owner)

	if reset.status != http.StatusOK || strings.TrimSpace(reset.body) != `{"outcome":"choice_required"}` {
		t.Fatalf("the reset answered %d %s", reset.status, reset.body)
	}
	if !h.meReclaimPending(t, owner) || listed.headers.Get("Cache-Control") != "no-store" {
		t.Fatal("the signed-in owner is not told that a choice is due")
	}
	if len(list.Links) != 1 || list.Links[0].RelationshipID != card.ID || list.Links[0].Tutor.Name != "A language tutor" ||
		strings.Join(list.Links[0].Tutor.Subjects, ",") != "English,German" || list.Links[0].Tutor.AvatarID == nil || *list.Links[0].Tutor.AvatarID != avatar {
		t.Fatalf("the waiting links are %+v", list.Links)
	}
	if len(list.Requests) != 1 || list.Requests[0].RequestID != requestText(request) || list.Requests[0].Tutor.AvatarID != nil || list.Requests[0].Tutor.Subjects == nil {
		t.Fatalf("the waiting requests are %+v", list.Requests)
	}
	if !sameInstant(list.ChooseBy, resetAt.Add(domain.ReclaimWindow)) {
		t.Fatalf("choose by %v", list.ChooseBy)
	}
	for _, private := range []string{cardName, newcomerEmail, `"notes"`, `"display_name"`} {
		if strings.Contains(listed.body, private) {
			t.Fatalf("the list contains %q: %s", private, listed.body)
		}
	}
}

func TestResetAnswersChoiceRequiredForAnUnverifiedAccountWithLinks(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.squat(t, tutor, "", newcomerEmail)
	h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)

	withLink := h.reset(t, h.resetToken(t, newcomerEmail))
	withoutLink := h.reset(t, h.resetToken(t, studentEmail))

	if withLink.status != http.StatusOK || strings.TrimSpace(withLink.body) != `{"outcome":"choice_required"}` {
		t.Fatalf("the reset with a link answered %d %s", withLink.status, withLink.body)
	}
	if withoutLink.status != http.StatusOK || strings.TrimSpace(withoutLink.body) != `{"outcome":null}` {
		t.Fatalf("the reset without a link answered %d %s", withoutLink.status, withoutLink.body)
	}
}

func TestVerifyEmailFromAnotherBrowserAnswersChoiceRequired(t *testing.T) {
	cases := map[string]func(t *testing.T, h *authHarness) string{
		"a browser signed out": func(*testing.T, *authHarness) string { return "" },
		"a browser signed in to another account": func(t *testing.T, h *authHarness) string {
			t.Helper()
			_, other := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
			return other
		},
	}
	for name, bearer := range cases {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)

			confirmed := h.confirmFromAnotherBrowser(t, newcomerEmail, bearer(t, h))

			if confirmed.status != http.StatusOK || outcomeOf(t, confirmed) != "choice_required" || accessToken(t, confirmed) == "" {
				t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
			}
			if h.storedRelationship(t, card.ID).State != "reclaim_pending" {
				t.Fatal("the confirmation from another browser did not wait for the owner's choice")
			}
		})
	}
}

func TestMeReportsAPendingReclaim(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	requestOnly, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, studentEmail)
	request := h.seedJoinRequest(t, otherTutor, requestOnly, 1, domain.JoinRequestHeld)

	beforeReclaim := h.meReclaimPending(t, squatter)
	owner := h.reclaimed(t, newcomerEmail)
	afterReclaim := h.meReclaimPending(t, owner)
	h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}})
	afterChoice := h.meReclaimPending(t, owner)
	requestReset := h.reset(t, h.resetToken(t, studentEmail))
	requestOwner := h.ownerSignIn(t, studentEmail)

	if beforeReclaim || !afterReclaim || afterChoice {
		t.Fatalf("reclaim_pending was %t before, %t after the reclaim and %t after the choice", beforeReclaim, afterReclaim, afterChoice)
	}
	if outcomeOf(t, requestReset) != "choice_required" || !h.meReclaimPending(t, requestOwner) || !h.storedRequest(t, request).OwnerChoice {
		t.Fatalf("a reclaim with only a held request answered %s", requestReset.body)
	}
}

func TestReclaimPageDataShowsTheCurrentNameAndAvatar(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.squat(t, tutor, newcomerEmail, newcomerEmail)
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	avatar := uploadedID(t, h.upload(t, squatter, "me.png", pngBytes(t, 2, 2)))
	if edited := h.patchMe(t, squatter, fmt.Sprintf(`{"name":"A chosen name","avatar_id":%q}`, avatar)); edited.status != http.StatusOK {
		t.Fatalf("the squatter's edit answered %d %s", edited.status, edited.body)
	}

	owner := h.reclaimed(t, newcomerEmail)
	shown := decodeMe(t, h.me(t, owner))
	corrected := h.patchMe(t, owner, `{"name":"The mailbox owner","avatar_id":null}`)
	_, list := h.reclaimList(t, owner)

	if shown.Name != "A chosen name" || shown.AvatarID == nil || *shown.AvatarID != avatar || !shown.ReclaimPending {
		t.Fatalf("the owner sees %+v", shown)
	}
	after := decodeMe(t, corrected)
	if corrected.status != http.StatusOK || after.Name != "The mailbox owner" || after.AvatarID != nil || !after.ReclaimPending || len(list.Links) != 1 {
		t.Fatalf("the owner's correction answered %d %s", corrected.status, corrected.body)
	}
}

type meView struct {
	Name           string  `json:"name"`
	AvatarID       *string `json:"avatar_id"`
	ReclaimPending bool    `json:"reclaim_pending"`
}

func decodeMe(t *testing.T, result response) meView {
	t.Helper()
	var decoded meView
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK {
		t.Fatalf("not an account: %d %s", result.status, result.body)
	}
	return decoded
}

func (h *authHarness) rowVersion(t *testing.T, user domain.UserID) string {
	t.Helper()
	var version string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT xmin::text FROM users WHERE id = $1", user).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func TestTutorRemovalAndDeletionNeverWriteTheStudentsUserRow(t *testing.T) {
	t.Run("removal", func(t *testing.T) {
		h := newFlowHarness(t)
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
		h.reclaimed(t, newcomerEmail)
		before := h.rowVersion(t, student)

		removed := h.removeStudent(t, tutor, card.ID)

		if removed.status != http.StatusOK || h.rowVersion(t, student) != before {
			t.Fatalf("the removal answered %d %s and wrote the student's row", removed.status, removed.body)
		}
	})
	t.Run("deletion", func(t *testing.T) {
		h := newFlowHarness(t)
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
		h.reclaimed(t, newcomerEmail)
		before := h.rowVersion(t, student)
		release := h.holdUserRow(t, student)

		deleted := awaitAnswer(t, h.sendInBackground(http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, accountPassword),
			map[string]string{"Authorization": "Bearer " + tutor, "Content-Type": "application/json"}))
		release()

		if deleted.status != http.StatusNoContent || h.rowVersion(t, student) != before {
			t.Fatalf("the deletion answered %d %s and wrote the student's row", deleted.status, deleted.body)
		}
		if h.storedRelationship(t, card.ID).State != "discarded" {
			t.Fatal("the deletion did not discard the reclaim-pending link while the student's row was held")
		}
	})
}

func TestReclaimExpiresHeldRequestsUnlessKept(t *testing.T) {
	h := newFlowHarness(t)
	visibleTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blockingTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	fullTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "full-tutor@example.org")
	closedTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 4, "closed-tutor@example.org")
	droppedTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "dropped-tutor@example.org")
	student, squatter := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 6, newcomerEmail)
	requests := map[string]domain.JoinRequestID{
		"visible": h.enteredRequest(t, squatter, h.storedCode(t, visibleTutor).Display()),
		"blocked": h.enteredRequest(t, squatter, h.storedCode(t, blockingTutor).Display()),
		"limit":   h.enteredRequest(t, squatter, string(h.storedCode(t, fullTutor))),
		"closed":  h.enteredRequest(t, squatter, strings.ToLower(h.storedCode(t, closedTutor).Display())),
		"dropped": h.enteredRequest(t, squatter, h.storedCode(t, droppedTutor).Display()),
	}
	h.blockRequester(t, blockingTutor, student)
	h.fillPendingRequests(t, fullTutor)
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), closedTutor, authStart); err != nil {
		t.Fatal(err)
	}
	reset := h.reset(t, h.resetToken(t, newcomerEmail))
	owner := h.ownerSignIn(t, newcomerEmail)

	decided := h.decideReclaim(t, owner, reclaimChoice{
		KeepRequests: []string{requestText(requests["visible"]), requestText(requests["blocked"]), requestText(requests["limit"]), requestText(requests["closed"])},
		DropRequests: []string{requestText(requests["dropped"])},
	})

	if outcomeOf(t, reset) != "choice_required" || decided.status != http.StatusOK || outcomeOf(t, decided) != "request_sent" {
		t.Fatalf("the reset answered %s and the choice %d %s", reset.body, decided.status, decided.body)
	}
	want := map[string]storedRequest{
		"visible": {State: "pending", Suppressed: "none"},
		"blocked": {State: "pending", Suppressed: "blocked"},
		"limit":   {State: "pending", Suppressed: "limit"},
		"closed":  {State: "expired", Suppressed: "none"},
		"dropped": {State: "expired", Suppressed: "none"},
	}
	for name, id := range requests {
		if got := h.storedRequest(t, id); got != want[name] {
			t.Errorf("the %s request is %+v", name, got)
		}
	}
	if h.meReclaimPending(t, owner) || h.reclaimDeadline(t, student) != nil || strings.Count(h.logs.String(), `"msg":"join_request.released"`) != 3 {
		t.Fatal("the choice left the reclaim pending or did not record the released requests")
	}
}

func (h *authHarness) blockRequester(t *testing.T, tutor domain.UserID, student domain.UserID) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "INSERT INTO blocked_requesters (tutor_id, student_id, created_at) VALUES ($1, $2, $3)", tutor, student, authStart); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) fillPendingRequests(t *testing.T, tutor domain.UserID) {
	t.Helper()
	h.fillPendingRequestsWith(t, tutor, "filler-", domain.VisiblePendingRequestCap)
}

func (h *authHarness) fillPendingRequestsWith(t *testing.T, tutor domain.UserID, prefix string, count int) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at,
    unverified_since, created_at, updated_at)
SELECT gen_random_uuid(), 'student', 'active', $3 || n || '@example.org', $1, '$argon2id$unused', 'A filler', 'en', 'UTC', 'system', '2026-10', $1, $1, $1, $1
FROM generate_series(1, $2::int) AS n`, authStart, count, prefix)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.database.Pool.Exec(context.Background(), `
INSERT INTO join_requests (id, tutor_id, student_id, state, suppressed, owner_choice_pending, expires_at, created_at)
SELECT gen_random_uuid(), $1, users.id, 'pending', 'none', false, $2, $3 FROM users WHERE users.email LIKE $4`,
		tutor, authStart.Add(14*24*time.Hour), authStart, prefix+"%")
	if err != nil {
		t.Fatal(err)
	}
}

func TestReclaimChoiceBodyIsRefusedBeforeAnyItemIsDecided(t *testing.T) {
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	decide := api.OperationAudience{Method: http.MethodPost, Path: reclaimPath, Audience: api.AudienceUser}
	schema, takesBody := loadPublishedDocument(t).bodySchema(t, decide)
	probes := operationProbes(schema)
	probes["a number for keep_links"] = bodyProbe{body: `{"keep_links":1}`, contentType: "application/json"}
	probes["a number inside keep_links"] = bodyProbe{body: `{"keep_links":[1]}`, contentType: "application/json"}

	h.assertProbesAreRefusedBeforeScope(t, decide, reclaimPath, probes, owner)

	if !takesBody {
		t.Fatal("the reclaim choice publishes no request body")
	}
	if h.storedRelationship(t, card.ID).State != "reclaim_pending" || !h.meReclaimPending(t, owner) {
		t.Fatal("a refused body decided the link")
	}
}

func TestKeepingALinkHonoursTheChoiceForARequestToTheSameTutor(t *testing.T) {
	decisions := map[string]struct {
		choice func(link string, request string) reclaimChoice
		want   string
	}{
		"the request kept": {func(link string, request string) reclaimChoice {
			return reclaimChoice{KeepLinks: []string{link}, KeepRequests: []string{request}}
		}, "accepted"},
		"the request dropped": {func(link string, request string) reclaimChoice {
			return reclaimChoice{KeepLinks: []string{link}, DropRequests: []string{request}}
		}, "expired"},
	}
	for name, decision := range decisions {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
			request := h.seedJoinRequest(t, tutorID, student, 1, domain.JoinRequestHeld)
			owner := h.reclaimed(t, newcomerEmail)

			decided := h.decideReclaim(t, owner, decision.choice(card.ID, requestText(request)))

			if decided.status != http.StatusOK || outcomeOf(t, decided) != "linked" {
				t.Fatalf("the choice answered %d %s", decided.status, decided.body)
			}
			if got := h.storedRequest(t, request); got.State != decision.want || got.OwnerChoice {
				t.Fatalf("the request is %+v instead of %s", got, decision.want)
			}
			if h.storedRelationship(t, card.ID).State != "active" || h.meReclaimPending(t, owner) {
				t.Fatal("the link was not kept or the choice stays pending")
			}
		})
	}
}

func TestKeepingAnExpiredHeldRequestAnswersCodeInvalid(t *testing.T) {
	h := newFlowHarness(t)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
	request := h.seedJoinRequest(t, otherTutor, student, 1, domain.JoinRequestHeld)
	h.reset(t, h.resetToken(t, newcomerEmail))
	owner := h.ownerSignIn(t, newcomerEmail)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE join_requests SET expires_at = $1 WHERE id = $2", authStart, request); err != nil {
		t.Fatal(err)
	}

	kept := h.decideReclaim(t, owner, reclaimChoice{KeepRequests: []string{requestText(request)}})

	if kept.status != http.StatusOK || strings.TrimSpace(kept.body) != `{"outcome":"code_invalid"}` {
		t.Fatalf("keeping an expired request answered %d %s", kept.status, kept.body)
	}
	if got := h.storedRequest(t, request); got.State != "expired" {
		t.Fatalf("the request is %+v", got)
	}
}

func (h *authHarness) seedDueReclaims(t *testing.T, prefix string, count int, deadline time.Time) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at,
    unverified_since, created_at, updated_at, reclaim_until)
SELECT gen_random_uuid(), 'student', 'active', $1 || n || '@example.org', $2, '$argon2id$unused', 'A reclaimed account', 'en', 'UTC', 'system',
    '2026-10', $2, $2, $2, $2, $3::timestamptz + n * interval '1 microsecond'
FROM generate_series(1, $4::int) AS n`, prefix, authStart, deadline, count)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) failDeadlineClearsOf(t *testing.T, prefix string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
CREATE FUNCTION refuse_reclaim_clear() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'the test refuses to clear this deadline';
END
$$`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.database.Pool.Exec(context.Background(), fmt.Sprintf(`
CREATE TRIGGER refuse_reclaim_clear BEFORE UPDATE OF reclaim_until ON users FOR EACH ROW
WHEN (NEW.reclaim_until IS NULL AND OLD.email::text LIKE '%s%%') EXECUTE FUNCTION refuse_reclaim_clear()`, prefix))
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) deadlinesLeft(t *testing.T, pattern string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM users WHERE reclaim_until IS NOT NULL AND email::text LIKE $1", pattern)
}

func TestReclaimExpirySkipsAFailingAccountAndRunsTheRest(t *testing.T) {
	h := newFlowHarness(t)
	h.seedDueReclaims(t, "before-", 1, authStart.Add(-3*time.Hour))
	h.seedDueReclaims(t, "failing-", 1, authStart.Add(-2*time.Hour))
	h.seedDueReclaims(t, "after-", 1, authStart.Add(-time.Hour))
	h.failDeadlineClearsOf(t, "failing-")

	h.tick(t)

	if h.deadlinesLeft(t, "failing-%") != 1 || h.deadlinesLeft(t, "before-%")+h.deadlinesLeft(t, "after-%") != 0 {
		t.Fatal("the expiry did not skip only the failing account")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"msg":"reclaim.expire_failed"`) || !strings.Contains(logs, `"job":"reclaim.expire","result":"failed","batch_size":2`) {
		t.Fatal("the failing account or the failed run was not recorded")
	}
}

func TestReclaimExpiryPagesPastFailuresUpToItsBound(t *testing.T) {
	h := newFlowHarness(t)
	h.seedDueReclaims(t, "failing-", 100, authStart.Add(-3*time.Hour))
	h.seedDueReclaims(t, "due-", 901, authStart.Add(-2*time.Hour))
	h.failDeadlineClearsOf(t, "failing-")

	h.tick(t)
	afterFirstRun := h.deadlinesLeft(t, "due-%")
	h.advance(t, time.Hour)
	h.tick(t)

	if afterFirstRun != 1 {
		t.Fatalf("the first run left %d of 901 due accounts instead of the one past its bound", afterFirstRun)
	}
	if h.deadlinesLeft(t, "due-%") != 0 || h.deadlinesLeft(t, "failing-%") != 100 {
		t.Fatal("the next run did not clear the account left over")
	}
	if !strings.Contains(h.logs.String(), `"job":"reclaim.expire","result":"failed","batch_size":900`) {
		t.Fatal("the first run did not record 900 cleared accounts")
	}
}

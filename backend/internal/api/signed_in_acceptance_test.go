package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	relationshipReactivated = `"msg":"relationship.reactivated"`
	secondTutorEmail        = "second-tutor@example.org"
	studentsPath            = "/api/v1/students"
)

type myTutorProfile struct {
	Name     string   `json:"name"`
	AvatarID *string  `json:"avatar_id"`
	Subjects []string `json:"subjects"`
	Bio      string   `json:"bio"`
}

type myTutor struct {
	RelationshipID     string         `json:"relationship_id"`
	State              string         `json:"state"`
	Colour             string         `json:"colour"`
	Tutor              myTutorProfile `json:"tutor"`
	EndedAt            *time.Time     `json:"ended_at"`
	EndedBy            *string        `json:"ended_by"`
	AccountClosedUntil *time.Time     `json:"account_closed_until"`
}

func (h *authHarness) acceptSignedIn(t *testing.T, bearer string, link string) response {
	t.Helper()
	return h.postJSON(t, invitationPath+link+"/accept", "{}", map[string]string{"Authorization": "Bearer " + bearer})
}

func (h *authHarness) trySignedInAcceptance(bearer string, link string) <-chan pendingAnswer {
	headers := signUpHeaders()
	headers["Authorization"] = "Bearer " + bearer
	return h.sendInBackground(http.MethodPost, invitationPath+link+"/accept", "{}", headers)
}

func acceptedTutor(t *testing.T, result response) myTutor {
	t.Helper()
	var decoded struct {
		Tutor *myTutor `json:"tutor"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.Tutor == nil || result.status != http.StatusOK {
		t.Fatalf("not a signed-in acceptance: %d %s", result.status, result.body)
	}
	return *decoded.Tutor
}

func (h *authHarness) myTutors(t *testing.T, bearer string) (response, []myTutor) {
	t.Helper()
	result := h.send(t, http.MethodGet, "/api/v1/tutors", "", map[string]string{"Authorization": "Bearer " + bearer})
	var decoded struct {
		Tutors []myTutor `json:"tutors"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Tutors == nil {
		t.Fatalf("not a list of tutors: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("My tutors has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return result, decoded.Tutors
}

func (h *authHarness) linkedSignedIn(t *testing.T, tutorToken string, studentToken string, name string, email string) studentCard {
	t.Helper()
	card := h.newCard(t, tutorToken, name, email)
	if accepted := h.acceptSignedIn(t, studentToken, linkToken(t, card.Invitation)); accepted.status != http.StatusOK {
		t.Fatalf("the signed-in acceptance answered %d %s", accepted.status, accepted.body)
	}
	return card
}

func (h *authHarness) seedJoinRequest(t *testing.T, tutor domain.UserID, student domain.UserID, id byte, state domain.JoinRequestState) domain.JoinRequestID {
	t.Helper()
	request := repositories.NewJoinRequest{
		ID: domain.JoinRequestID{0xd0, id}, TutorID: tutor, StudentID: student, State: state, Suppressed: domain.SuppressionNone,
		ExpiresAt: authStart.Add(14 * 24 * time.Hour), CreatedAt: authStart,
	}
	if err := h.store.Repositories().JoinRequests.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	return request.ID
}

func (h *authHarness) joinRequestState(t *testing.T, id domain.JoinRequestID) string {
	t.Helper()
	var state string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT state FROM join_requests WHERE id = $1", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (h *authHarness) describeTutor(t *testing.T, tutor domain.UserID, bio string, subjects []string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE tutor_profiles SET bio = $1, subjects = $2 WHERE user_id = $3", bio, subjects, tutor); err != nil {
		t.Fatal(err)
	}
}

func sameTutor(t *testing.T, first myTutor, second myTutor) bool {
	t.Helper()
	encodedFirst, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	encodedSecond, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	return string(encodedFirst) == string(encodedSecond)
}

func randomInvitationToken() string {
	return base64.RawURLEncoding.EncodeToString(make([]byte, 32))
}

func TestTutorAcceptingAnInvitationGetsRoleMismatch(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, owner := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, activeTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	_, unverifiedTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 3, "unconfirmed-tutor@example.org")
	card := h.newCard(t, owner, cardName, secondTutorEmail)
	link := linkToken(t, card.Invitation)

	answers := []response{
		h.acceptSignedIn(t, activeTutor, link),
		h.acceptSignedIn(t, unverifiedTutor, link),
		h.acceptSignedIn(t, activeTutor, randomInvitationToken()),
	}

	for index, refused := range answers {
		if refused.status != http.StatusConflict || code(t, refused) != "role_mismatch" || refusal(t, refused) != refusal(t, answers[0]) {
			t.Fatalf("tutor acceptance %d answered %d %s", index, refused.status, refused.body)
		}
	}
	if h.mismatchCount(t, link) != 0 || h.invitationPage(t, link).status != http.StatusOK {
		t.Fatal("a tutor's acceptance counted a mismatch or used the link")
	}
	if unchanged := decodeCard(t, h.cardCall(t, owner, http.MethodGet, card.ID, "", "")); unchanged.State != "invited" {
		t.Fatalf("the card changed: %+v", unchanged)
	}
	if strings.Count(h.logs.String(), `"reason":"role_mismatch"`) != len(answers) {
		t.Fatal("expected one role_mismatch refusal record per attempt")
	}
}

func TestUnconfirmedStudentCannotAcceptASecondTutorsInvitation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, firstTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, secondTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	first, student := h.linkedStudent(t, firstTutor, newcomerEmail)
	signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	second := h.newCard(t, secondTutor, cardName, newcomerEmail)
	link := linkToken(t, second.Invitation)

	refused := h.acceptSignedIn(t, signedIn, link)
	unknown := h.acceptSignedIn(t, signedIn, randomInvitationToken())

	if refused.status != http.StatusForbidden || code(t, refused) != "email_not_verified" || refused.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the second tutor's link answered %d %s", refused.status, refused.body)
	}
	if refusal(t, unknown) != refusal(t, refused) {
		t.Fatalf("an unknown token answered %d %s", unknown.status, unknown.body)
	}
	if h.mismatchCount(t, link) != 0 || h.invitationPage(t, link).status != http.StatusOK || h.linksOf(t, student) != 1 {
		t.Fatal("the refusal counted, used the link or linked the account")
	}
	if kept := h.storedRelationship(t, first.ID); kept.State != "provisional" {
		t.Fatalf("the first link is %s", kept.State)
	}
	if !strings.Contains(h.logs.String(), `"reason":"not_verified"`) {
		t.Fatal("no not_verified refusal record")
	}
}

func TestUnconfirmedStudentWithoutALinkMayAcceptAFirstInvitation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, secondTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.describeTutor(t, tutorID, "Grammar and conversation", []string{"English", "German"})
	student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, studentEmail)
	card := h.newCard(t, tutor, cardName, "")
	link := linkToken(t, card.Invitation)
	secondLink := linkToken(t, h.newCard(t, secondTutor, cardName, "").Invitation)
	before := h.hasher.Hashes()

	accepted := h.acceptSignedIn(t, signedIn, link)
	_, listed := h.myTutors(t, signedIn)
	second := h.acceptSignedIn(t, signedIn, secondLink)

	if hashed := h.hasher.Hashes() - before; hashed != 0 {
		t.Fatalf("signed-in acceptance hashed %d passwords", hashed)
	}
	linked := acceptedTutor(t, accepted)
	if linked.RelationshipID != card.ID || linked.State != "provisional" || linked.Colour != "clay" || accepted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the acceptance answered %s", accepted.body)
	}
	if linked.Tutor.Name != "A language tutor" || linked.Tutor.Bio != "Grammar and conversation" || strings.Join(linked.Tutor.Subjects, ",") != "English,German" {
		t.Fatalf("the tutor is %+v", linked.Tutor)
	}
	if len(listed) != 1 || !sameTutor(t, listed[0], linked) {
		t.Fatalf("My tutors lists %+v", listed)
	}
	shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))
	if shown.State != "provisional" || shown.Account == nil || shown.Account.EmailConfirmed || shown.Email != nil || shown.Invitation != nil {
		t.Fatalf("the card is %+v", shown)
	}
	if h.invitationPage(t, link).status != http.StatusNotFound || h.linksOf(t, student) != 1 {
		t.Fatal("the link was not consumed")
	}
	if second.status != http.StatusForbidden || code(t, second) != "email_not_verified" {
		t.Fatalf("a second tutor's link answered %d %s", second.status, second.body)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"path":"signed_in"`) || !strings.Contains(logs, `"effect":"provisional"`) {
		t.Fatal("no invitation.accepted record for the signed-in path")
	}
}

func TestSignedInAcceptanceOfAnEmailBoundCardCountsTheMismatch(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, otherEmail)
	_, owner := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, cardEmail)
	card := h.newCard(t, tutor, cardName, " Pupil@Example.org ")
	link := linkToken(t, card.Invitation)
	before := h.hasher.Hashes()

	refused := h.acceptSignedIn(t, other, link)
	accepted := h.acceptSignedIn(t, owner, link)

	if hashed := h.hasher.Hashes() - before; hashed != 0 {
		t.Fatalf("signed-in acceptance hashed %d passwords", hashed)
	}
	if refused.status != http.StatusConflict || code(t, refused) != "invitation_email_mismatch" || h.mismatchCount(t, link) != 1 {
		t.Fatalf("another account answered %d %s", refused.status, refused.body)
	}
	if linked := acceptedTutor(t, accepted); linked.State != "active" || linked.RelationshipID != card.ID {
		t.Fatalf("the card's account got %+v", linked)
	}
	shown := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, card.ID, "", ""))
	if shown.State != "active" || shown.Account == nil || !shown.Account.EmailConfirmed || shown.Account.Email != cardEmail {
		t.Fatalf("the card is %+v", shown)
	}
	if !strings.Contains(h.logs.String(), `"effect":"linked"`) {
		t.Fatal("no linked effect recorded")
	}
}

func TestSecondCardOfTheSameStudentIsMerged(t *testing.T) {
	t.Parallel()
	t.Run("into an active relationship", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		first := h.linkedSignedIn(t, tutor, signedIn, cardName, "")
		waiting := h.seedJoinRequest(t, tutorID, student, 1, domain.JoinRequestPending)
		second := h.newCard(t, tutor, "A second card", studentEmail)
		link := linkToken(t, second.Invitation)

		merged := h.acceptSignedIn(t, signedIn, link)

		kept := acceptedTutor(t, merged)
		if kept.RelationshipID != first.ID || kept.State != "active" {
			t.Fatalf("the merge answered %+v", kept)
		}
		stored := h.storedRelationship(t, second.ID)
		if stored.State != "discarded" || stored.DiscardReason == nil || *stored.DiscardReason != "merged" || stored.StudentID != nil {
			t.Fatalf("the second card is %+v", stored)
		}
		if h.linksOf(t, student) != 1 || h.invitationPage(t, link).status != http.StatusNotFound || h.joinRequestState(t, waiting) != "accepted" {
			t.Fatal("the merge left a second link, an open token or a waiting request")
		}
		if gone := h.cardCall(t, tutor, http.MethodGet, second.ID, "", ""); gone.status != http.StatusNotFound {
			t.Fatalf("the merged card answers %d", gone.status)
		}
		if !strings.Contains(h.logs.String(), `"effect":"merged"`) {
			t.Fatal("no merged effect recorded")
		}
	})
	t.Run("into an ended relationship with an open link", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		ended := h.seedCard(t, tutorID, 1, domain.RelationshipEnded, &student)
		regenerated := h.cardCall(t, tutor, http.MethodPost, ended, "/invitation", "")
		var issued struct {
			Invitation cardInvitation `json:"invitation"`
		}
		if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
			t.Fatalf("regenerating answered %d %s", regenerated.status, regenerated.body)
		}
		endedLink := linkToken(t, &issued.Invitation)
		second := h.newCard(t, tutor, "A second card", studentEmail)

		merged := h.acceptSignedIn(t, signedIn, linkToken(t, second.Invitation))

		kept := acceptedTutor(t, merged)
		if kept.RelationshipID != ended || kept.State != "active" || kept.EndedAt != nil || kept.EndedBy != nil {
			t.Fatalf("the merge answered %+v", kept)
		}
		if h.revokeReason(t, endedLink) != "linked" || h.ciphertextKept(t, endedLink) || h.invitationPage(t, endedLink).status != http.StatusNotFound {
			t.Fatal("the ended card's own link stays open")
		}
		if h.storedRelationship(t, second.ID).State != "discarded" || !strings.Contains(h.logs.String(), relationshipReactivated) {
			t.Fatal("the second card was not merged or the reactivation was not recorded")
		}
	})
}

func TestEndedCardInvitationAcceptsOnlyTheBoundAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, stranger := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, otherEmail)
	ended := h.seedCard(t, tutorID, 1, domain.RelationshipEnded, &student)
	regenerated := decodeCard(t, h.cardCall(t, tutor, http.MethodGet, ended, "", ""))
	if regenerated.Invitation != nil {
		t.Fatalf("the seeded ended card has a link: %+v", regenerated.Invitation)
	}
	issued := h.cardCall(t, tutor, http.MethodPost, ended, "/invitation", "")
	var body struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(issued.body), &body); err != nil || issued.status != http.StatusOK {
		t.Fatalf("regenerating answered %d %s", issued.status, issued.body)
	}
	link := linkToken(t, &body.Invitation)

	newAccount := h.acceptInvitation(t, link, studentEmail)
	another := h.acceptSignedIn(t, stranger, link)
	bound := h.acceptSignedIn(t, signedIn, link)

	for name, refused := range map[string]response{"a new account": newAccount, "another account": another} {
		if refused.status != http.StatusConflict || code(t, refused) != "invitation_email_mismatch" {
			t.Fatalf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	reactivated := acceptedTutor(t, bound)
	if reactivated.RelationshipID != ended || reactivated.State != "active" || reactivated.EndedAt != nil || reactivated.Colour != "clay" {
		t.Fatalf("the bound account got %+v", reactivated)
	}
	if h.mismatchCount(t, link) != 2 || h.invitationPage(t, link).status != http.StatusNotFound {
		t.Fatal("the mismatches were not counted or the link stays open")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, relationshipReactivated) || !strings.Contains(logs, `"effect":"reactivated"`) {
		t.Fatal("the reactivation was not recorded")
	}
}

func TestSharedStudentIsIsolatedBetweenTutors(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, ivan := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, pavel := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	_, maria := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, cardEmail)
	ivansCard := h.newCard(t, ivan, cardName, cardEmail)
	h.withNotes(t, ivan, ivansCard.ID, cardNotes)
	if accepted := h.acceptSignedIn(t, maria, linkToken(t, ivansCard.Invitation)); accepted.status != http.StatusOK {
		t.Fatalf("accepting Ivan's link answered %d %s", accepted.status, accepted.body)
	}
	pavelsCard := h.linkedSignedIn(t, pavel, maria, "Pavel's pupil", "")

	responses := []response{
		h.cardCall(t, pavel, http.MethodGet, pavelsCard.ID, "", ""),
		h.send(t, http.MethodGet, studentsPath, "", jsonCall(pavel)),
		h.send(t, http.MethodGet, studentsPath+"?q=Maria", "", jsonCall(pavel)),
		h.cardCall(t, pavel, http.MethodGet, ivansCard.ID, "", ""),
	}
	_, marias := h.myTutors(t, maria)

	for index, answer := range responses {
		for _, private := range []string{cardName, cardNotes, ivansCard.ID} {
			if strings.Contains(answer.body, private) {
				t.Fatalf("Pavel's response %d contains %q: %s", index, private, answer.body)
			}
		}
	}
	if shown := decodeCard(t, responses[0]); shown.Email != nil || shown.DisplayName != "Pavel's pupil" || shown.Notes == nil || *shown.Notes != "" {
		t.Fatalf("Pavel's card is %+v", shown)
	}
	if responses[3].status != http.StatusNotFound {
		t.Fatalf("Pavel reading Ivan's card answered %d", responses[3].status)
	}
	if len(marias) != 2 || marias[0].Colour == marias[1].Colour {
		t.Fatalf("Maria's tutors are %+v", marias)
	}
}

func TestNotesNeverReachAStudentResponse(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, cardEmail)
	card := h.newCard(t, tutor, cardName, cardEmail)
	h.withNotes(t, tutor, card.ID, cardNotes)
	link := linkToken(t, card.Invitation)

	accepted := h.acceptSignedIn(t, student, link)
	listed, _ := h.myTutors(t, student)

	for name, answer := range map[string]response{"the acceptance": accepted, "My tutors": listed} {
		if answer.status != http.StatusOK {
			t.Fatalf("%s answered %d %s", name, answer.status, answer.body)
		}
		for _, private := range []string{cardNotes, cardName, `"notes"`, `"display_name"`, `"email"`} {
			if strings.Contains(answer.body, private) {
				t.Fatalf("%s contains %q: %s", name, private, answer.body)
			}
		}
	}
	logs := h.logs.String()
	for _, secret := range []string{cardNotes, cardName, cardEmail, link, link[:16]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain %q", secret)
		}
	}
}

func TestStudentSeesOnlyTheirOwnTutors(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	firstTutorID, firstTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, secondTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	closedTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "closed-tutor@example.org")
	first, firstToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 4, studentEmail)
	_, secondToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, otherEmail)
	firstLink := h.linkedSignedIn(t, firstTutor, firstToken, cardName, "")
	earlier := h.linkedSignedIn(t, firstTutor, secondToken, "Another student", "")
	h.at(t, authStart.Add(time.Minute))
	secondLinks := []studentCard{earlier, h.linkedSignedIn(t, secondTutor, secondToken, "Another student", "")}
	h.newCard(t, firstTutor, "Still invited", "")
	h.seedCard(t, firstTutorID, 9, domain.RelationshipDiscarded, &first)
	ended := h.seedCard(t, closedTutor, 1, domain.RelationshipEnded, &first)
	deletedAt := authStart.Add(time.Hour)
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), closedTutor, deletedAt); err != nil {
		t.Fatal(err)
	}

	_, firsts := h.myTutors(t, firstToken)
	_, seconds := h.myTutors(t, secondToken)

	if len(firsts) != 2 || firsts[0].RelationshipID != firstLink.ID || firsts[0].State != "active" || firsts[0].AccountClosedUntil != nil {
		t.Fatalf("the first student's tutors are %+v", firsts)
	}
	past := firsts[1]
	if past.RelationshipID != ended || past.State != "ended" || past.EndedBy == nil || *past.EndedBy != "tutor" || past.EndedAt == nil {
		t.Fatalf("the past tutor is %+v", past)
	}
	if past.AccountClosedUntil == nil || !past.AccountClosedUntil.Equal(deletedAt.Add(30*24*time.Hour)) {
		t.Fatalf("the closed tutor's account is shown until %v", past.AccountClosedUntil)
	}
	if len(seconds) != 2 || seconds[0].RelationshipID != secondLinks[0].ID || seconds[1].RelationshipID != secondLinks[1].ID {
		t.Fatalf("the second student's tutors are %+v", seconds)
	}
	if seconds[0].Colour != "clay" || seconds[1].Colour != "slate" {
		t.Fatalf("the second student's colours are %s and %s", seconds[0].Colour, seconds[1].Colour)
	}
}

func TestConcurrentAcceptancesByOneAccountNeverAnswer500(t *testing.T) {
	t.Parallel()
	t.Run("an unconfirmed account and two tutors", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, firstTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		_, secondTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
		student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, studentEmail)
		links := []string{
			linkToken(t, h.newCard(t, firstTutor, cardName, "").Invitation),
			linkToken(t, h.newCard(t, secondTutor, cardName, "").Invitation),
		}
		release := h.holdUserRow(t, student)
		var answers []<-chan pendingAnswer
		for index, link := range links {
			answers = append(answers, h.trySignedInAcceptance(signedIn, link))
			h.waitForLockWaiters(t, int64(index+1))
		}

		release()
		statuses := map[string]int{}
		for _, answer := range answers {
			result := awaitAnswer(t, answer)
			statuses[fmt.Sprint(result.status, " ", code(t, result))]++
		}

		if statuses["200 "] != 1 || statuses["403 email_not_verified"] != 1 || len(statuses) != 2 {
			t.Fatalf("statuses %v", statuses)
		}
		if h.linksOf(t, student) != 1 {
			t.Fatalf("the account holds %d links", h.linksOf(t, student))
		}
	})
	t.Run("a confirmed account and two cards of one tutor", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		student, signedIn := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		links := []string{
			linkToken(t, h.newCard(t, tutor, cardName, "").Invitation),
			linkToken(t, h.newCard(t, tutor, "A second card", studentEmail).Invitation),
		}
		release := h.holdUserRow(t, student)
		var answers []<-chan pendingAnswer
		for index, link := range links {
			answers = append(answers, h.trySignedInAcceptance(signedIn, link))
			h.waitForLockWaiters(t, int64(index+1))
		}

		release()
		kept := map[string]bool{}
		for _, answer := range answers {
			kept[acceptedTutor(t, awaitAnswer(t, answer)).RelationshipID] = true
		}

		if len(kept) != 1 || h.linksOf(t, student) != 1 {
			t.Fatalf("the acceptances kept %v and the account holds %d links", kept, h.linksOf(t, student))
		}
		logs := h.logs.String()
		if !strings.Contains(logs, `"effect":"linked"`) || !strings.Contains(logs, `"effect":"merged"`) {
			t.Fatal("expected one link and one merge")
		}
	})
}

func TestSignedInAcceptanceRechecksTheSessionUnderTheLock(t *testing.T) {
	t.Parallel()
	cases := map[string]domain.AccountState{
		"a reset reclaims the unconfirmed account":                             domain.AccountUnverified,
		"a reset revokes the confirmed account's sessions and keeps its state": domain.AccountActive,
	}
	for name, state := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			student, signedIn := h.signedInAs(t, domain.RoleStudent, state, 2, studentEmail)
			card := h.newCard(t, tutor, cardName, "")
			link := linkToken(t, card.Invitation)
			h.forgot(t, studentEmail)
			h.tick(t)
			reset := latestToken(t, h.mailTo(t, studentEmail), resetSubject)
			release := h.holdUserRow(t, student)

			resetAnswer := h.sendInBackground(http.MethodPost, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, reset), signUpHeaders())
			h.waitForLockWaiters(t, 1)
			acceptance := h.trySignedInAcceptance(signedIn, link)
			h.waitForLockWaiters(t, 2)
			release()
			resetDone, accepted := awaitAnswer(t, resetAnswer), awaitAnswer(t, acceptance)

			if resetDone.status != http.StatusOK {
				t.Fatalf("the reset answered %d %s", resetDone.status, resetDone.body)
			}
			if accepted.status != http.StatusUnauthorized || code(t, accepted) != "unauthenticated" {
				t.Fatalf("the acceptance authenticated before the reset answered %d %s", accepted.status, accepted.body)
			}
			if stored := h.storedRelationship(t, card.ID); stored.State != "invited" || h.invitationPage(t, link).status != http.StatusOK || h.linksOf(t, student) != 0 {
				t.Fatalf("the acceptance with a revoked session changed the card: %+v", stored)
			}
			if after, err := h.store.Repositories().Users.ByID(context.Background(), student); err != nil || after.State != domain.AccountActive {
				t.Fatalf("after the reset the account is %+v (%v)", after.State, err)
			}
		})
	}
}

func TestSignedInAcceptanceOfAnUnusableTokenAnswersIdentically(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutorID, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blockedTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	expired := linkToken(t, h.newCard(t, tutor, "Expired", "").Invitation)
	h.at(t, authStart.Add(domain.InvitationLifetime+time.Minute))
	tutor = h.relogin(t, tutorEmail)
	used := linkToken(t, h.newCard(t, tutor, "Used", "").Invitation)
	if accepted := h.acceptInvitation(t, used, newcomerEmail); accepted.status != http.StatusAccepted {
		t.Fatalf("using the link answered %d %s", accepted.status, accepted.body)
	}
	regeneratedCard := h.newCard(t, tutor, "Regenerated", "")
	revoked := linkToken(t, regeneratedCard.Invitation)
	if regenerated := h.cardCall(t, tutor, http.MethodPost, regeneratedCard.ID, "/invitation", ""); regenerated.status != http.StatusOK {
		t.Fatalf("regenerating answered %d %s", regenerated.status, regenerated.body)
	}
	blocked := linkToken(t, h.newCard(t, h.relogin(t, secondTutorEmail), "Blocked", "").Invitation)
	if _, err := h.store.Repositories().Users.Block(context.Background(), blockedTutor, authStart); err != nil {
		t.Fatal(err)
	}
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	tokens := map[string]string{
		"random": randomInvitationToken(), "malformed": "not-a-token", "used": used, "expired": expired, "revoked": revoked, "blocked tutor": blocked,
	}
	before := h.hasher.Hashes()

	answers := map[string]response{}
	for name, token := range tokens {
		answers[name] = h.acceptSignedIn(t, student, token)
	}

	expected := refusal(t, answers["random"])
	for name, answer := range answers {
		if answer.status != http.StatusConflict || code(t, answer) != "invitation_invalid" || refusal(t, answer) != expected {
			t.Errorf("the %s token answered %d %s", name, answer.status, answer.body)
		}
	}
	for _, name := range []string{"used", "expired", "revoked", "blocked tutor"} {
		if count := h.mismatchCount(t, tokens[name]); count != 0 {
			t.Errorf("the %s token counted %d mismatches", name, count)
		}
	}
	if h.hasher.Hashes() != before || h.cardCount(t, tutorID) != 3 {
		t.Fatal("a refused acceptance hashed a password or changed the tutor's cards")
	}
}

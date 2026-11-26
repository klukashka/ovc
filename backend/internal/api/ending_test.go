package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

const (
	tutorsPath             = "/api/v1/tutors/"
	relationshipEndedMsg   = `"msg":"relationship.ended"`
	endedToStudentSubject  = "Your lessons with a tutor on OVC have ended"
	endedToTutorSubject    = "A student stopped studying with you on OVC"
	endingStudentName      = "A student account name"
	endingTutorAccountName = "Anna the tutor"
	markupTutorName        = `Anna <a href="https://evil.example/">Click</a> & "Co"`
	malformedRelationship  = "not-a-relationship"
)

func (h *authHarness) endAsTutor(t *testing.T, token string, card string) response {
	t.Helper()
	return h.cardCall(t, token, http.MethodPost, card, "/end", "")
}

func (h *authHarness) stopStudying(t *testing.T, token string, relationship string) response {
	t.Helper()
	return h.send(t, http.MethodPost, tutorsPath+relationship+"/end", "", jsonCall(token))
}

func endedTutor(t *testing.T, result response) myTutor {
	t.Helper()
	var tutor myTutor
	if err := json.Unmarshal([]byte(result.body), &tutor); err != nil || result.status != http.StatusOK || tutor.RelationshipID == "" {
		t.Fatalf("not a tutor of the student: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" || result.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("the ending answered with Cache-Control %q and Content-Type %q", result.headers.Get("Cache-Control"), result.headers.Get("Content-Type"))
	}
	return tutor
}

type endingParties struct {
	tutor        domain.UserID
	tutorToken   string
	student      domain.UserID
	studentToken string
	card         studentCard
	group        studentGroup
}

func (h *authHarness) activeStudentInAGroup(t *testing.T) endingParties {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	h.setAccount(t, student, endingStudentName, domain.LanguageEnglish)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	group := h.newGroup(t, tutorToken, groupName, []string{card.ID})
	return endingParties{tutor: tutor, tutorToken: tutorToken, student: student, studentToken: studentToken, card: card, group: group}
}

func (h *authHarness) membershipsOf(t *testing.T, card string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(group_id) FROM group_members WHERE relationship_id = $1::uuid", card)
}

func (h *authHarness) endedEmailsQueued(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventRelationshipEnded))
}

func (h *authHarness) openInvitationsOf(t *testing.T, card string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM invitations WHERE relationship_id = $1::uuid AND used_at IS NULL AND revoked_at IS NULL", card)
}

func onlyMessage(t *testing.T, messages []mail.SinkMessage, subject string) mail.SinkMessage {
	t.Helper()
	if len(messages) != 1 || messages[0].Subject != subject {
		t.Fatalf("expected one %q email, got %v", subject, subjects(messages))
	}
	return messages[0]
}

func TestTutorEndingRemovesFromGroupsAndEmailsTheStudent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.setAccount(t, parties.tutor, markupTutorName, domain.LanguageEnglish)

	ended := h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	h.tick(t)

	card := decodeCard(t, ended)
	if ended.status != http.StatusOK || card.State != "ended" || len(card.Groups) != 0 || card.Invitation != nil || ended.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}
	stored := h.cardOf(t, parties.tutorToken, parties.card.ID)
	if stored.State != "ended" || stored.Account == nil || stored.Account.Email != studentEmail {
		t.Fatalf("the ended card shows %+v", stored)
	}
	if h.membershipsOf(t, parties.card.ID) != 0 || h.openInvitationsOf(t, parties.card.ID) != 0 {
		t.Fatal("the ended relationship keeps a membership or an open link")
	}
	if group := decodeGroup(t, h.groupCall(t, parties.tutorToken, http.MethodGet, parties.group.ID, "", "")); len(group.Members) != 0 {
		t.Fatalf("the group still lists %+v", group.Members)
	}
	_, tutors := h.myTutors(t, parties.studentToken)
	if len(tutors) != 1 || tutors[0].State != "ended" || tutors[0].EndedBy == nil || *tutors[0].EndedBy != "tutor" || tutors[0].EndedAt == nil {
		t.Fatalf("the student lists %+v", tutors)
	}
	message := onlyMessage(t, h.mailTo(t, studentEmail), endedToStudentSubject)
	if !strings.Contains(message.Text, markupTutorName) || strings.Contains(message.Text+message.HTML, cardName) {
		t.Fatalf("the student's email is %q", message.Text)
	}
	if strings.Contains(message.HTML, `<a href="https://evil`) || !strings.Contains(message.HTML, `Anna &lt;a href=&#34;https://evil.example/&#34;&gt;Click&lt;/a&gt; &amp; &#34;Co&#34;`) {
		t.Fatalf("the HTML body does not escape the tutor's name: %s", message.HTML)
	}
	if strings.Contains(message.Subject, "Anna") || strings.Count(message.HTML, "<a ") != 1 || !strings.Contains(message.HTML, `<a href="`+publicOrigin+`/tutors"`) {
		t.Fatalf("the student's email links elsewhere or names the tutor in the subject: %s", message.HTML)
	}
	if len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatal("the tutor was emailed about their own ending")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, relationshipEndedMsg) || !strings.Contains(logs, `"by":"tutor"`) || !strings.Contains(logs, `"memberships_removed":1`) {
		t.Fatalf("no relationship.ended record: %s", logs)
	}
}

func TestStudentEndingRemovesFromGroupsAndEmailsTheTutor(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)

	ended := h.stopStudying(t, parties.studentToken, parties.card.ID)
	h.tick(t)

	tutor := endedTutor(t, ended)
	if tutor.State != "ended" || tutor.EndedBy == nil || *tutor.EndedBy != "student" || tutor.EndedAt == nil || tutor.Tutor.Name != endingTutorAccountName {
		t.Fatalf("stopping answered %s", ended.body)
	}
	card := h.cardOf(t, parties.tutorToken, parties.card.ID)
	if card.State != "ended" || len(card.Groups) != 0 || h.membershipsOf(t, parties.card.ID) != 0 {
		t.Fatalf("the tutor's card shows %+v", card)
	}
	message := onlyMessage(t, h.mailTo(t, tutorEmail), endedToTutorSubject)
	if !strings.Contains(message.Text, cardName) || strings.Contains(message.Text+message.HTML, endingStudentName) {
		t.Fatalf("the tutor's email is %q", message.Text)
	}
	if !strings.Contains(message.Text, publicOrigin+"/students/"+parties.card.ID) {
		t.Fatalf("the tutor's email does not link to the card: %q", message.Text)
	}
	if len(h.mailTo(t, studentEmail)) != 0 {
		t.Fatal("the student was emailed about their own ending")
	}
	if logs := h.logs.String(); !strings.Contains(logs, relationshipEndedMsg) || !strings.Contains(logs, `"by":"student"`) {
		t.Fatalf("no relationship.ended record: %s", logs)
	}
}

func TestStudentCannotEndAnotherStudentsRelationship(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, intruder := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, "another-student@example.org")

	outOfScope := h.stopStudying(t, intruder, parties.card.ID)
	malformed := h.stopStudying(t, intruder, malformedRelationship)
	random := h.stopStudying(t, intruder, randomCardID)

	if outOfScope.status != http.StatusNotFound || refusal(t, outOfScope) != refusal(t, random) {
		t.Fatalf("another student's ending answered %d %s", outOfScope.status, outOfScope.body)
	}
	if refusal(t, malformed) != refusal(t, random) {
		t.Fatalf("a malformed identifier answered %d %s", malformed.status, malformed.body)
	}
	if h.storedRelationship(t, parties.card.ID).State != "active" || h.membershipsOf(t, parties.card.ID) != 1 || h.endedEmailsQueued(t) != 0 {
		t.Fatal("another student's request changed the relationship")
	}
}

func TestStudentEndAnswersNotFoundForOwnInvitedDiscardedOrReclaimPendingIds(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	secondTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, secondTutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	ids := map[string]string{
		"an invited card with the student's email": h.newCard(t, tutorToken, cardName, studentEmail).ID,
		"a removed link":         h.seedCard(t, tutor, 1, domain.RelationshipDiscarded, &student),
		"a reclaim-pending link": h.seedCard(t, secondTutor, 2, domain.RelationshipReclaimPending, &student),
	}
	random := h.stopStudying(t, token, randomCardID)

	for name, id := range ids {
		before := h.storedRelationship(t, id)

		refused := h.stopStudying(t, token, id)

		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
		if after := h.storedRelationship(t, id); after.State != before.State {
			t.Errorf("%s moved from %s to %s", name, before.State, after.State)
		}
	}
	if h.endedEmailsQueued(t) != 0 || h.cardCount(t, secondTutor) != 1 {
		t.Fatal("a refused ending queued an email or created a card")
	}
}

func TestEndedEmailToTheTutorUsesTheCardNameEscaped(t *testing.T) {
	t.Parallel()
	t.Run("a card the tutor named", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		card := h.linkedSignedIn(t, tutorToken, studentToken, `Maria <b>"the reader"</b> & co`, studentEmail)
		h.setAccount(t, student, "Squatter <a href=\"https://evil.example/\">Click</a>\r\nBcc: victim@example.org", domain.LanguageEnglish)

		h.stopStudying(t, studentToken, card.ID)
		h.tick(t)

		message := onlyMessage(t, h.mailTo(t, tutorEmail), endedToTutorSubject)
		if !strings.Contains(message.HTML, `Maria &lt;b&gt;&#34;the reader&#34;&lt;/b&gt; &amp; co`) || strings.Contains(message.HTML, "<b>") {
			t.Fatalf("the HTML body does not escape the card name: %s", message.HTML)
		}
		if !strings.Contains(message.Text, `Maria <b>"the reader"</b> & co`) {
			t.Fatalf("the text body is %q", message.Text)
		}
		for _, leaked := range []string{"Squatter", "evil.example", "victim@example.org"} {
			if strings.Contains(message.Subject+message.Text+message.HTML, leaked) {
				t.Fatalf("the email carries the account name: %q", leaked)
			}
		}
		if strings.Count(message.HTML, "<a ") != 1 {
			t.Fatalf("the HTML body carries more than the card link: %s", message.HTML)
		}
	})
	t.Run("a card created by accepting a join request", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		h.setAccount(t, student, `Boris <i>at accept</i>`, domain.LanguageEnglish)
		request := h.enteredRequest(t, studentToken, h.storedCode(t, tutor).Display())
		card := decodeCard(t, h.acceptRequest(t, tutorToken, requestText(request)))
		h.setAccount(t, student, "Renamed <script>later</script>\nafter accept", domain.LanguageEnglish)

		h.stopStudying(t, studentToken, card.ID)
		h.tick(t)

		message := onlyMessage(t, h.mailTo(t, tutorEmail), endedToTutorSubject)
		if !strings.Contains(message.HTML, `Boris &lt;i&gt;at accept&lt;/i&gt;`) || !strings.Contains(message.Text, `Boris <i>at accept</i>`) {
			t.Fatalf("the email does not carry the copy taken at accept: %s", message.HTML)
		}
		if strings.Contains(message.Subject+message.Text+message.HTML, "Renamed") || strings.Contains(message.HTML, "<script>") {
			t.Fatal("the email carries the account name after the rename")
		}
	})
}

func TestEndedEmailIsNotSentToAnUnverifiedRecipient(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)
	card := h.seedCard(t, tutor, 1, domain.RelationshipActive, &student)

	ended := h.endAsTutor(t, tutorToken, card)
	h.tick(t)

	if ended.status != http.StatusOK || decodeCard(t, ended).State != "ended" {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}
	if len(h.mailTo(t, studentEmail)) != 0 || h.obsoleteRows(t, domain.EventRelationshipEnded) != 1 {
		t.Fatal("an unverified recipient was emailed about the ending")
	}
}

func TestEndingIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)

	first := h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	again := h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	byStudent := h.stopStudying(t, parties.studentToken, parties.card.ID)
	h.tick(t)

	firstCard, againCard := decodeCard(t, first), decodeCard(t, again)
	if again.status != http.StatusOK || againCard.State != "ended" || againCard.ID != firstCard.ID {
		t.Fatalf("repeating the ending answered %d %s", again.status, again.body)
	}
	tutor := endedTutor(t, byStudent)
	if tutor.State != "ended" || tutor.EndedBy == nil || *tutor.EndedBy != "tutor" {
		t.Fatalf("the student's ending of an ended relationship answered %s", byStudent.body)
	}
	if h.endedEmailsQueued(t) != 1 || len(h.mailTo(t, studentEmail)) != 1 || len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatal("a repeated ending queued another email")
	}
	if strings.Count(h.logs.String(), relationshipEndedMsg) != 1 {
		t.Fatal("a repeated ending was recorded again")
	}
}

func TestEndingAnInvitedCardIsRefusedAndADiscardedCardIsNotFound(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unverified, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, "unconfirmed@example.org")
	reclaimed, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, "reclaimed@example.org")
	removed, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 4, "removed@example.org")
	refusedCards := map[string]string{
		"an invited card":        h.newCard(t, token, cardName, cardEmail).ID,
		"a provisional link":     h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &unverified),
		"a reclaim-pending link": h.seedCard(t, tutor, 2, domain.RelationshipReclaimPending, &reclaimed),
	}
	discarded := h.seedCard(t, tutor, 3, domain.RelationshipDiscarded, &removed)
	random := h.endAsTutor(t, token, randomCardID)

	for name, card := range refusedCards {
		before := h.storedRelationship(t, card).State

		refused := h.endAsTutor(t, token, card)

		if refused.status != http.StatusConflict || code(t, refused) != "relationship_not_active" {
			t.Errorf("ending %s answered %d %s", name, refused.status, refused.body)
		}
		if after := h.storedRelationship(t, card).State; after != before {
			t.Errorf("%s moved from %s to %s", name, before, after)
		}
	}
	gone := h.endAsTutor(t, token, discarded)
	malformed := h.endAsTutor(t, token, malformedRelationship)
	if gone.status != http.StatusNotFound || refusal(t, gone) != refusal(t, random) {
		t.Fatalf("ending a discarded card answered %d %s", gone.status, gone.body)
	}
	if refusal(t, malformed) != refusal(t, random) {
		t.Fatalf("a malformed identifier answered %d %s", malformed.status, malformed.body)
	}
	if h.endedEmailsQueued(t) != 0 {
		t.Fatal("a refused ending queued an email")
	}
}

func TestReactivationByInvitationKeepsHistoryAndRestoresNoGroups(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.withNotes(t, parties.tutorToken, parties.card.ID, "Prefers mornings")
	before := h.cardOf(t, parties.tutorToken, parties.card.ID)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)

	regenerated := h.cardCall(t, parties.tutorToken, http.MethodPost, parties.card.ID, "/invitation", "")
	var issued struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
		t.Fatalf("inviting again answered %d %s", regenerated.status, regenerated.body)
	}
	accepted := h.acceptSignedIn(t, parties.studentToken, linkToken(t, &issued.Invitation))

	tutor := acceptedTutor(t, accepted)
	if tutor.RelationshipID != parties.card.ID || tutor.State != "active" || tutor.EndedAt != nil || tutor.EndedBy != nil {
		t.Fatalf("the acceptance answered %s", accepted.body)
	}
	after := h.cardOf(t, parties.tutorToken, parties.card.ID)
	if after.State != "active" || after.CreatedAt != before.CreatedAt || after.Notes == nil || *after.Notes != "Prefers mornings" || len(after.Groups) != 0 {
		t.Fatalf("the reactivated card is %+v", after)
	}
	if h.membershipsOf(t, parties.card.ID) != 0 || h.linksOf(t, parties.student) != 1 {
		t.Fatal("the reactivation restored a membership or created a second relationship")
	}
	if !strings.Contains(h.logs.String(), relationshipReactivated) {
		t.Fatal("no relationship.reactivated record")
	}
}

func TestReactivationByJoinRequestAfterAnEndingRestoresNoGroups(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.stopStudying(t, parties.studentToken, parties.card.ID)
	request := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())

	accepted := h.acceptRequest(t, parties.tutorToken, requestText(request))

	card := decodeCard(t, accepted)
	if accepted.status != http.StatusOK || card.ID != parties.card.ID || card.State != "active" || len(card.Groups) != 0 {
		t.Fatalf("accepting answered %d %s", accepted.status, accepted.body)
	}
	_, tutors := h.myTutors(t, parties.studentToken)
	if len(tutors) != 1 || tutors[0].State != "active" || tutors[0].EndedBy != nil || h.membershipsOf(t, parties.card.ID) != 0 {
		t.Fatalf("the student lists %+v", tutors)
	}
}

func TestReactivationByInvitationAcceptsARequestAnUnblockClosed(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	code := h.storedCode(t, parties.tutor).Display()
	declined := h.enteredRequest(t, parties.studentToken, code)
	h.declineRequest(t, parties.tutorToken, requestText(declined), true)
	suppressed := h.enteredRequest(t, parties.studentToken, code)
	h.unblock(t, parties.tutorToken, parties.student)
	regenerated := h.cardCall(t, parties.tutorToken, http.MethodPost, parties.card.ID, "/invitation", "")
	var issued struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
		t.Fatalf("inviting again answered %d %s", regenerated.status, regenerated.body)
	}

	accepted := h.acceptSignedIn(t, parties.studentToken, linkToken(t, &issued.Invitation))

	if acceptedTutor(t, accepted).State != "active" {
		t.Fatalf("the acceptance answered %s", accepted.body)
	}
	if state := h.joinRequestState(t, suppressed); state != "accepted" {
		t.Fatalf("the request the unblock closed is %s after the reactivation", state)
	}
	if state := h.joinRequestState(t, declined); state != "declined" {
		t.Fatalf("the declined request is %s after the reactivation", state)
	}
}

func TestRelationshipEndedEmailIsObsoleteAfterReactivation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.stopStudying(t, parties.studentToken, parties.card.ID)
	request := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())
	h.acceptRequest(t, parties.tutorToken, requestText(request))

	h.tick(t)

	if len(h.mailTo(t, tutorEmail)) != 0 || h.obsoleteRows(t, domain.EventRelationshipEnded) != 1 {
		t.Fatal("the ending email was sent after the reactivation")
	}
}

func TestEndedEmailOfAnEarlierEndingIsObsoleteAfterTheRelationshipEndsAgain(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.stopStudying(t, parties.studentToken, parties.card.ID)
	request := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())
	h.acceptRequest(t, parties.tutorToken, requestText(request))
	h.advance(t, time.Minute)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)

	h.tick(t)

	onlyMessage(t, h.mailTo(t, studentEmail), endedToStudentSubject)
	if len(h.mailTo(t, tutorEmail)) != 0 || h.obsoleteRows(t, domain.EventRelationshipEnded) != 1 || h.endedEmailsQueued(t) != 2 {
		t.Fatal("the earlier ending's email was not dropped, or the later one was not sent")
	}
}

func TestEndedAgainAfterAJoinRequestReactivationHoldsNoOpenInvitation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	if regenerated := h.cardCall(t, parties.tutorToken, http.MethodPost, parties.card.ID, "/invitation", ""); regenerated.status != http.StatusOK {
		t.Fatalf("inviting again answered %d %s", regenerated.status, regenerated.body)
	}
	request := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())
	if accepted := h.acceptRequest(t, parties.tutorToken, requestText(request)); decodeCard(t, accepted).State != "active" {
		t.Fatalf("the reactivation answered %d %s", accepted.status, accepted.body)
	}
	h.advance(t, time.Minute)

	ended := h.endAsTutor(t, parties.tutorToken, parties.card.ID)

	if card := decodeCard(t, ended); card.State != "ended" || card.Invitation != nil {
		t.Fatalf("the second ending answered %s", ended.body)
	}
	if h.openInvitationsOf(t, parties.card.ID) != 0 {
		t.Fatal("the card ended again still holds an open invitation")
	}
}

func TestStopStudyingOnAProvisionalLinkRemovesIt(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.linkedStudent(t, tutorToken, newcomerEmail)
	h.withNotes(t, tutorToken, card.ID, "Met at the open day")
	token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))

	stopped := h.stopStudying(t, token, card.ID)
	again := h.stopStudying(t, token, card.ID)
	h.tick(t)

	if stopped.status != http.StatusNoContent || stopped.body != "" || stopped.headers.Get("Cache-Control") != "no-store" || stopped.headers.Get("Content-Type") != "" {
		t.Fatalf("stopping a provisional link answered %d %v %s", stopped.status, stopped.headers, stopped.body)
	}
	if again.status != http.StatusNotFound {
		t.Fatalf("repeating it answered %d %s", again.status, again.body)
	}
	old := h.storedRelationship(t, card.ID)
	if old.State != "discarded" || old.DiscardReason == nil || *old.DiscardReason != "removed" || old.Email != nil || old.Notes != "" {
		t.Fatalf("the old link is %+v", old)
	}
	fresh := h.cardsOf(t, tutor, domain.RelationshipInvited)
	if len(fresh) != 1 || fresh[0].DisplayName != cardName || fresh[0].Email == nil || *fresh[0].Email != newcomerEmail ||
		fresh[0].Notes == nil || *fresh[0].Notes != "Met at the open day" {
		t.Fatalf("the tutor's new cards are %+v", fresh)
	}
	if _, tutors := h.myTutors(t, token); len(tutors) != 0 || h.linksOf(t, student) != 0 {
		t.Fatalf("the student still lists %+v", tutors)
	}
	if h.endedEmailsQueued(t) != 0 || len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatal("the silent removal emailed someone")
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"msg":"relationship.removed"`) || !strings.Contains(logs, `"cause":"student"`) || strings.Contains(logs, relationshipEndedMsg) {
		t.Fatalf("the removal was not recorded as one by the student: %s", logs)
	}
}

func TestEndingRacingAGroupChangeLeavesNoMembership(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, 2)
	group := h.newGroup(t, token, groupName, cards[:1])
	release := h.holdGroup(t, group.ID)
	addition := h.sendInBackground(http.MethodPatch, groupsRoute+"/"+group.ID, `{"add":`+idList(cards[1:])+`}`, jsonCall(token))
	h.waitForLockWaiters(t, 1)
	ending := h.sendInBackground(http.MethodPost, studentsPath+"/"+cards[1]+"/end", "", jsonCall(token))
	h.waitForLockWaiters(t, 2)

	release()
	added, ended := awaitAnswer(t, addition), awaitAnswer(t, ending)

	if added.status != http.StatusOK {
		t.Fatalf("the group change answered %d %s", added.status, added.body)
	}
	if ended.status != http.StatusOK || decodeCard(t, ended).State != "ended" {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}
	if h.membershipsOf(t, cards[1]) != 0 {
		t.Fatal("the ended relationship kept the membership the group change added")
	}
	if after := decodeGroup(t, h.groupCall(t, token, http.MethodGet, group.ID, "", "")); len(after.Members) != 1 || after.Members[0].RelationshipID != cards[0] {
		t.Fatalf("the group lists %+v", after.Members)
	}
}

func anotherTutorsActiveStudent(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owner, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruderID, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card := h.linkedCard(t, owner, 10, domain.RelationshipActive)
	h.shareStudentWith(t, intruderID, domain.UserID{10})
	outOfScope := h.endAsTutor(t, intruder, card)
	random := h.endAsTutor(t, intruder, randomCardID)
	if h.storedRelationship(t, card).State != string(domain.RelationshipActive) {
		t.Fatal("another tutor ended the relationship")
	}
	return outOfScope, random
}

func anotherStudentsRelationship(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	_, intruder := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	outOfScope := h.stopStudying(t, intruder, card)
	random := h.stopStudying(t, intruder, randomCardID)
	if h.storedRelationship(t, card).State != string(domain.RelationshipActive) {
		t.Fatal("another student ended the relationship")
	}
	return outOfScope, random
}

func TestEndedEmailFollowsTheRecipientsLanguage(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.setAccount(t, parties.tutor, "Ирина", domain.LanguageRussian)
	h.setAccount(t, parties.student, "Борис", domain.LanguageRussian)

	h.stopStudying(t, parties.studentToken, parties.card.ID)
	h.tick(t)
	h.advance(t, time.Minute)
	reactivation := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())
	h.acceptRequest(t, parties.tutorToken, requestText(reactivation))
	h.endAsTutor(t, parties.tutorToken, parties.card.ID)
	h.tick(t)

	toTutor := onlyMessage(t, h.mailTo(t, tutorEmail), "Ученик прекратил занятия в OVC")
	toStudent := onlyMessage(t, h.mailTo(t, studentEmail), "Занятия с преподавателем в OVC завершены")
	if !strings.Contains(toTutor.Text, cardName) || !strings.Contains(toStudent.Text, "Ирина") || !strings.Contains(toStudent.HTML, `lang="ru"`) {
		t.Fatalf("the Russian emails are %q and %q", toTutor.Text, toStudent.Text)
	}
	for _, message := range []mail.SinkMessage{toTutor, toStudent} {
		if hasOperatorLine(message.Text + message.HTML) {
			t.Fatalf("the ended email carries the operator line: %s", message.Text)
		}
	}
}

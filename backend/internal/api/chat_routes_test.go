package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const randomChannelID = "0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a"

type sharedChatWorld struct {
	parties     endingParties
	direct      storedChannel
	group       storedChannel
	second      domain.UserID
	secondToken string
	secondCard  studentCard
	secondChat  storedChannel
	squatter    domain.UserID
	squatCard   studentCard
	squatChat   storedChannel
}

func (h *authHarness) sharedChatWorld(t *testing.T) sharedChatWorld {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "second-tutor@example.org")
	h.setAccount(t, second, "Boris the second tutor", domain.LanguageEnglish)
	secondCard := h.linkedSignedIn(t, secondToken, parties.studentToken, "Second tutor's card", studentEmail)
	squatCard, squatter := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	return sharedChatWorld{
		parties: parties, direct: h.directChannel(t, parties.card.ID), group: h.groupChannel(t, parties.group.ID),
		second: second, secondToken: secondToken, secondCard: secondCard, secondChat: h.directChannel(t, secondCard.ID),
		squatter: squatter, squatCard: squatCard, squatChat: h.directChannel(t, squatCard.ID),
	}
}

func itemOf(page channelPage, id string) (channelItem, bool) {
	for _, item := range page.Items {
		if item.ID == id {
			return item, true
		}
	}
	return channelItem{}, false
}

func TestAProvisionalLinkReachesOnlyItsDirectChannel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))

	listed, page := h.channelList(t, squatterToken, "")
	own, item := h.channelCall(t, squatterToken, world.squatChat.ID)
	group, _ := h.channelCall(t, squatterToken, world.group.ID)
	other, _ := h.channelCall(t, squatterToken, world.direct.ID)
	_, forTutor := h.channelCall(t, world.parties.tutorToken, world.squatChat.ID)

	if listed.status != http.StatusOK || !slices.Equal(listedChannelIDs(page), []string{world.squatChat.ID}) {
		t.Fatalf("the unconfirmed account lists %d %v", listed.status, listedChannelIDs(page))
	}
	if own.status != http.StatusOK || item.Label == nil || *item.Label != endingTutorAccountName || item.Unconfirmed || !item.Writable {
		t.Fatalf("the unconfirmed account reads its private chat as %d %+v", own.status, item)
	}
	if group.status != http.StatusNotFound || other.status != http.StatusNotFound {
		t.Fatalf("the unconfirmed account reached the group chat %d or another student's chat %d", group.status, other.status)
	}
	if forTutor.Label == nil || *forTutor.Label != cardName || !forTutor.Unconfirmed || forTutor.UserID == nil || *forTutor.UserID != world.squatter.String() {
		t.Fatalf("the tutor reads the provisional chat as %+v", forTutor)
	}
}

func TestChannelListIsScopedOrderedAndLabelledByRole(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	h.setChannelActivity(t, world.group.ID, authStart.Add(3*time.Hour))
	h.setChannelActivity(t, world.direct.ID, authStart.Add(2*time.Hour))
	h.setChannelActivity(t, world.squatChat.ID, authStart.Add(time.Hour))
	_, tutors := h.myTutors(t, world.parties.studentToken)

	_, tutorPage := h.channelList(t, world.parties.tutorToken, "")
	_, firstPage := h.channelList(t, world.parties.tutorToken, "?limit=1")
	if firstPage.NextCursor == nil {
		t.Fatal("the first page of one channel names no next page")
	}
	_, restPage := h.channelList(t, world.parties.tutorToken, "?limit=2&cursor="+*firstPage.NextCursor)
	_, searched := h.channelList(t, world.parties.tutorToken, "?q=CONVERSATION")
	_, studentPage := h.channelList(t, world.parties.studentToken, "")
	_, secondPage := h.channelList(t, world.secondToken, "")
	badCursor, _ := h.channelList(t, world.parties.tutorToken, "?cursor=not-a-cursor")

	order := []string{world.group.ID, world.direct.ID, world.squatChat.ID}
	if !slices.Equal(listedChannelIDs(tutorPage), order) || tutorPage.NextCursor != nil || tutorPage.UnreadChannels != 0 {
		t.Fatalf("the tutor lists %v, want %v by last activity", listedChannelIDs(tutorPage), order)
	}
	if !slices.Equal(append(listedChannelIDs(firstPage), listedChannelIDs(restPage)...), order) || restPage.NextCursor != nil {
		t.Fatalf("the pages list %v and %v", listedChannelIDs(firstPage), listedChannelIDs(restPage))
	}
	if !slices.Equal(listedChannelIDs(searched), []string{world.group.ID}) {
		t.Fatalf("the search lists %v", listedChannelIDs(searched))
	}
	direct, _ := itemOf(tutorPage, world.direct.ID)
	group, _ := itemOf(tutorPage, world.group.ID)
	squat, _ := itemOf(tutorPage, world.squatChat.ID)
	if *direct.Label != cardName || direct.Tutor != nil || *direct.UserID != world.parties.student.String() || direct.Online == nil || *direct.Online || direct.Unconfirmed {
		t.Fatalf("the tutor's direct item is %+v", direct)
	}
	if *group.Label != groupName || group.Tutor != nil || group.UserID != nil || group.Online != nil || *group.Colour != world.parties.group.Colour {
		t.Fatalf("the tutor's group item is %+v", group)
	}
	if *squat.Label != cardName || !squat.Unconfirmed || string(squat.LastMessage) != "null" {
		t.Fatalf("the tutor's provisional item is %+v", squat)
	}
	if !slices.Equal(listedChannelIDs(secondPage), []string{world.secondChat.ID}) {
		t.Fatalf("the second tutor lists %v", listedChannelIDs(secondPage))
	}
	studentDirect, _ := itemOf(studentPage, world.direct.ID)
	studentGroup, _ := itemOf(studentPage, world.group.ID)
	studentSecond, _ := itemOf(studentPage, world.secondChat.ID)
	if len(studentPage.Items) != 3 || *studentDirect.Label != endingTutorAccountName || *studentSecond.Label != "Boris the second tutor" {
		t.Fatalf("the student lists %+v", studentPage.Items)
	}
	colour := tutors[slices.IndexFunc(tutors, func(tutor myTutor) bool { return tutor.RelationshipID == world.parties.card.ID })].Colour
	if *studentDirect.Colour != colour || studentDirect.Tutor != nil || *studentDirect.UserID != world.parties.tutor.String() {
		t.Fatalf("the student's direct item is %+v, the relationship colour %s", studentDirect, colour)
	}
	if *studentGroup.Label != groupName || studentGroup.Tutor == nil || studentGroup.Tutor.Name != endingTutorAccountName ||
		*studentGroup.Tutor.Colour != colour || *studentGroup.Colour != colour || studentGroup.UserID != nil || studentGroup.Online != nil {
		t.Fatalf("the student's group item is %+v", studentGroup)
	}
	if badCursor.status != http.StatusUnprocessableEntity || code(t, badCursor) != "validation_failed" {
		t.Fatalf("a malformed cursor answered %d %s", badCursor.status, badCursor.body)
	}
}

func TestUnreadChannelsAreCountedForTheCallerOnly(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	h.setChannelSeq(t, world.group.ID, 120)
	h.setChannelSeq(t, world.direct.ID, 2)

	tutorUnread := h.send(t, http.MethodGet, channelsRoute+"/unread", "", jsonCall(world.parties.tutorToken))
	secondUnread := h.send(t, http.MethodGet, channelsRoute+"/unread", "", jsonCall(world.secondToken))
	_, page := h.channelList(t, world.parties.studentToken, "")

	if unreadCount(t, tutorUnread) != 2 || unreadCount(t, secondUnread) != 0 || tutorUnread.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the unread counts are %s for the tutor and %s for the second tutor", tutorUnread.body, secondUnread.body)
	}
	group, _ := itemOf(page, world.group.ID)
	direct, _ := itemOf(page, world.direct.ID)
	if page.UnreadChannels != 2 || group.UnreadCount != domain.UnreadCountCap || direct.UnreadCount != 2 {
		t.Fatalf("the student's list counts %d channels, %d and %d messages", page.UnreadChannels, group.UnreadCount, direct.UnreadCount)
	}
}

func TestOneChannelIsReadWithItsListFieldsAndHeaderLink(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	_, tutorPage := h.channelList(t, world.parties.tutorToken, "")
	_, studentPage := h.channelList(t, world.parties.studentToken, "")

	tutorDirect, direct := h.channelCall(t, world.parties.tutorToken, world.direct.ID)
	_, group := h.channelCall(t, world.parties.tutorToken, world.group.ID)
	studentGroup, _ := h.channelCall(t, world.parties.studentToken, world.group.ID)
	nonMember, _ := h.channelCall(t, world.secondToken, world.direct.ID)
	random, _ := h.channelCall(t, world.secondToken, randomChannelID)
	h.mustAnswer(t, h.removeStudent(t, world.parties.tutorToken, world.squatCard.ID), http.StatusOK)
	pendingErasure, _ := h.channelCall(t, world.parties.tutorToken, world.squatChat.ID)
	h.mustAnswer(t, h.groupCall(t, world.parties.tutorToken, http.MethodPatch, world.parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.parties.card.ID)), http.StatusOK)
	removed, _ := h.channelCall(t, world.parties.studentToken, world.group.ID)

	listed, _ := itemOf(tutorPage, world.direct.ID)
	listedForStudent, _ := itemOf(studentPage, world.group.ID)
	var studentFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(studentGroup.body), &studentFields); err != nil {
		t.Fatal(err)
	}
	if tutorDirect.headers.Get("Cache-Control") != "no-store" || direct.RelationshipID == nil || *direct.RelationshipID != world.parties.card.ID || direct.GroupID != nil {
		t.Fatalf("the tutor's direct header is %+v", direct)
	}
	if len(direct.Members) != 1 || direct.Members[0].Label != cardName || direct.Members[0].UserID != world.parties.student.String() {
		t.Fatalf("the tutor's direct header lists %+v", direct.Members)
	}
	if group.GroupID == nil || *group.GroupID != world.parties.group.ID || len(group.Members) != 1 || group.Members[0].Online == nil {
		t.Fatalf("the tutor's group header is %+v", group)
	}
	direct.RelationshipID, direct.Members = nil, nil
	if listFields(t, direct) != listFields(t, listed) {
		t.Fatalf("the tutor reads %+v, the list shows %+v", direct, listed)
	}
	for _, field := range []string{"members", "relationship_id", "group_id"} {
		if _, carried := studentFields[field]; carried {
			t.Fatalf("the student's group header carries %s: %s", field, studentGroup.body)
		}
	}
	var forStudent channelItem
	if err := json.Unmarshal([]byte(studentGroup.body), &forStudent); err != nil || listFields(t, forStudent) != listFields(t, listedForStudent) {
		t.Fatalf("the student reads %+v, the list shows %+v", forStudent, listedForStudent)
	}
	for name, refused := range map[string]response{"a non-member": nonMember, "a channel pending erasure": pendingErasure, "a removed member": removed} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Fatalf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
}

func listFields(t *testing.T, item channelItem) string {
	t.Helper()
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestOtherRoutesCarryTheCallersChannel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)

	card := h.cardCall(t, world.parties.tutorToken, http.MethodGet, world.parties.card.ID, "", "")
	invited := h.cardCall(t, world.parties.tutorToken, http.MethodGet, h.newCard(t, world.parties.tutorToken, "An invited card", "").ID, "", "")
	group := h.groupCall(t, world.parties.tutorToken, http.MethodGet, world.parties.group.ID, "", "")
	tutors := h.send(t, http.MethodGet, "/api/v1/tutors", "", jsonCall(world.parties.studentToken))

	if got := channelField(t, card.body); got != world.direct.ID {
		t.Fatalf("the student card carries the channel %q", got)
	}
	if got := channelField(t, invited.body); got != "" {
		t.Fatalf("an invited card carries the channel %q", got)
	}
	if got := channelField(t, group.body); got != world.group.ID {
		t.Fatalf("the group page carries the channel %q", got)
	}
	var listed struct {
		Tutors []struct {
			RelationshipID string  `json:"relationship_id"`
			ChannelID      *string `json:"channel_id"`
		} `json:"tutors"`
	}
	if err := json.Unmarshal([]byte(tutors.body), &listed); err != nil {
		t.Fatal(err)
	}
	for _, tutor := range listed.Tutors {
		want := map[string]string{world.parties.card.ID: world.direct.ID, world.secondCard.ID: world.secondChat.ID}[tutor.RelationshipID]
		if tutor.ChannelID == nil || *tutor.ChannelID != want {
			t.Fatalf("My tutors lists %s with the channel %v, want %s", tutor.RelationshipID, tutor.ChannelID, want)
		}
	}
}

func channelField(t *testing.T, body string) string {
	t.Helper()
	var decoded struct {
		ChannelID *string `json:"channel_id"`
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatal(err)
	}
	if _, carried := fields["channel_id"]; !carried {
		t.Fatalf("the body carries no channel_id: %s", body)
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ChannelID == nil {
		return ""
	}
	return *decoded.ChannelID
}

func (h *authHarness) lessonChannel(t *testing.T, token string, lesson string) (int, string) {
	t.Helper()
	result := h.lessonPage(t, token, lesson)
	if result.status != http.StatusOK {
		return result.status, ""
	}
	return result.status, channelField(t, result.body)
}

func TestLessonChannelIsNullWithoutMembership(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	oneToOne := h.mondays(t, world.parties.tutorToken, world.parties.card.ID, 1).Lessons[0].ID
	groupLesson := h.groupMondays(t, world.parties.tutorToken, world.parties.group.ID, 1).Lessons[0].ID
	frozen := h.groupMondays(t, world.parties.tutorToken, world.parties.group.ID, 1).Lessons[0].ID
	provisional := h.mondays(t, world.parties.tutorToken, world.squatCard.ID, 1).Lessons[0].ID

	_, tutorOneToOne := h.lessonChannel(t, world.parties.tutorToken, oneToOne)
	_, studentOneToOne := h.lessonChannel(t, world.parties.studentToken, oneToOne)
	_, studentGroup := h.lessonChannel(t, world.parties.studentToken, groupLesson)
	h.cancelWithRoster(t, frozen, world.parties.student)
	h.mustAnswer(t, h.groupCall(t, world.parties.tutorToken, http.MethodPatch, world.parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.parties.card.ID)), http.StatusOK)
	frozenStatus, studentFrozen := h.lessonChannel(t, world.parties.studentToken, frozen)
	h.reclaimed(t, newcomerEmail)
	_, tutorReclaimPending := h.lessonChannel(t, world.parties.tutorToken, provisional)
	ownerStatus, _ := h.lessonChannel(t, h.ownerSignIn(t, newcomerEmail), provisional)
	h.mustAnswer(t, h.removeStudent(t, world.parties.tutorToken, world.squatCard.ID), http.StatusOK)
	_, tutorDiscarded := h.lessonChannel(t, world.parties.tutorToken, provisional)

	if tutorOneToOne != world.direct.ID || studentOneToOne != world.direct.ID || studentGroup != world.group.ID {
		t.Fatalf("the lessons carry %q, %q and %q", tutorOneToOne, studentOneToOne, studentGroup)
	}
	if frozenStatus != http.StatusOK || studentFrozen != "" {
		t.Fatalf("a lesson held only by its frozen roster answered %d with the channel %q", frozenStatus, studentFrozen)
	}
	if tutorReclaimPending != world.squatChat.ID || ownerStatus != http.StatusNotFound {
		t.Fatalf("while reclaim-pending the tutor reads the channel %q and the owner the lesson %d", tutorReclaimPending, ownerStatus)
	}
	if tutorDiscarded != "" {
		t.Fatalf("a discarded link's lesson carries the channel %q", tutorDiscarded)
	}
}

func TestChatRoutesDenyEveryOutOfScopeCaller(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, requestingEmail)
	otherCard := h.linkedSignedIn(t, world.parties.tutorToken, otherToken, "Another card", requestingEmail)
	otherGroup := h.newGroup(t, world.parties.tutorToken, "Another group", []string{otherCard.ID})
	removedSquatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.mustAnswer(t, h.removeStudent(t, world.parties.tutorToken, world.squatCard.ID), http.StatusOK)
	h.mustAnswer(t, h.groupCall(t, world.parties.tutorToken, http.MethodPatch, world.parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	h.mustAnswer(t, h.groupCall(t, world.parties.tutorToken, http.MethodPatch, world.parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, otherCard.ID)), http.StatusOK)
	_, pendingToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 7, "third-tutor@example.org")
	_, reclaimCard := h.squatAs(t, pendingToken, "reclaimed@example.org")
	ownerToken := h.reclaimed(t, "reclaimed@example.org")
	reclaimChat := h.directChannel(t, reclaimCard)
	h.mustAnswer(t, h.endAsTutor(t, world.parties.tutorToken, world.parties.card.ID), http.StatusOK)

	random, _ := h.channelCall(t, world.secondToken, randomChannelID)
	cases := map[string]struct {
		token   string
		channel string
	}{
		"a second tutor sharing the student":                    {world.secondToken, world.direct.ID},
		"a removed group member":                                {otherToken, world.group.ID},
		"a student of another group":                            {world.parties.studentToken, h.groupChannel(t, otherGroup.ID).ID},
		"a reclaim-pending account":                             {ownerToken, reclaimChat.ID},
		"a removed provisional account":                         {removedSquatter, world.squatChat.ID},
		"an ended relationship's student, on the group it left": {world.parties.studentToken, world.group.ID},
	}
	randomUpload := h.chatUpload(t, world.secondToken, randomChannelID, "notes.txt", plainText(16))
	randomAddress, _ := h.addressOf(t, world.secondToken, fixtureID(10, 99), "")
	index := byte(0)
	for name, denied := range cases {
		index++
		attachment := fixtureID(10, index)
		h.boundFixture(t, attachment, denied.channel, messageID(index))
		refused, _ := h.channelCall(t, denied.token, denied.channel)
		_, page := h.channelList(t, denied.token, "")
		upload := h.chatUpload(t, denied.token, denied.channel, "notes.txt", plainText(16))
		address, _ := h.addressOf(t, denied.token, attachment, "")

		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) || slices.Contains(listedChannelIDs(page), denied.channel) {
			t.Fatalf("%s answered %d %s and lists %v", name, refused.status, refused.body, listedChannelIDs(page))
		}
		if name == "a removed provisional account" {
			if upload.status != http.StatusForbidden || code(t, upload) != "email_not_verified" {
				t.Fatalf("%s uploaded with %s", name, describe(upload))
			}
		} else if upload.status != http.StatusNotFound || refusal(t, upload) != refusal(t, randomUpload) {
			t.Fatalf("%s uploaded with %s", name, describe(upload))
		}
		if address.status != http.StatusNotFound || refusal(t, address) != refusal(t, randomAddress) {
			t.Fatalf("%s got a download address: %s", name, describe(address))
		}
	}
	endedRead, ended := h.channelCall(t, world.parties.studentToken, world.direct.ID)
	endedUpload := h.chatUpload(t, world.parties.studentToken, world.direct.ID, "notes.txt", plainText(16))
	if endedRead.status != http.StatusOK || ended.Writable {
		t.Fatalf("the ended relationship's student reads the private chat as %d %+v", endedRead.status, ended)
	}
	if endedUpload.status != http.StatusConflict || code(t, endedUpload) != "channel_read_only" {
		t.Fatalf("the ended relationship's student uploads to the private chat with %s", describe(endedUpload))
	}
}

func (h *authHarness) squatAs(t *testing.T, tutorToken string, email string) (domain.UserID, string) {
	t.Helper()
	card := h.newCard(t, tutorToken, cardName, email)
	if accepted := h.acceptInvitation(t, linkToken(t, card.Invitation), email); accepted.status != http.StatusAccepted {
		t.Fatalf("the sign-up answered %d %s", accepted.status, accepted.body)
	}
	return h.userID(t, email), card.ID
}

func anotherTutorsChannel(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	h.shareStudentWith(t, intruder, student)
	outOfScope, _ := h.channelCall(t, intruderToken, h.directChannel(t, card.ID).ID)
	random, _ := h.channelCall(t, intruderToken, randomChannelID)
	return outOfScope, random
}

func TestEntityChangedChannelReachesMembersUnderTheChannelLock(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	group := h.groupChannel(t, parties.group.ID)
	log := h.recordFrames(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", group.ID)

	removal := h.sendInBackground(http.MethodPatch, groupsRoute+"/"+parties.group.ID, fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID), jsonCall(parties.tutorToken))
	h.waitForLockWaiters(t, 1)
	whileHeld := log.settle(t, h)
	release()
	h.mustAnswer(t, awaitAnswer(t, removal), http.StatusOK)
	removed := log.settle(t, h)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", `{"name":"A renamed group"}`), http.StatusOK)
	renamed := log.settle(t, h)

	channelFrame := []sentFrame{{kind: string(domain.EntityChannel), id: group.ID}}
	if len(whileHeld) != 0 {
		t.Fatalf("frames were sent before the channel lock was granted: %v", whileHeld)
	}
	if !slices.Equal(framesTo(removed, parties.student), channelFrame) || !hasFrame(removed, parties.tutor, domain.EntityChannel, group.ID) {
		t.Fatalf("the removal sent the student %v and the tutor %v", framesTo(removed, parties.student), framesTo(removed, parties.tutor))
	}
	if got := framesTo(removed, other); len(got) != 0 {
		t.Fatalf("another member learned of the removal: %v", got)
	}
	if !slices.Equal(framesTo(renamed, other), channelFrame) || !hasFrame(renamed, parties.tutor, domain.EntityChannel, group.ID) {
		t.Fatalf("the rename sent the member %v and the tutor %v", framesTo(renamed, other), framesTo(renamed, parties.tutor))
	}
	if got := framesTo(renamed, parties.student); len(got) != 0 {
		t.Fatalf("the member who left was sent %v", got)
	}
}

type racingWrites struct {
	hold   func() func()
	first  func() <-chan pendingAnswer
	second func() <-chan pendingAnswer
	after  func(t *testing.T, first response, second response)
}

func answersOK(what string) func(t *testing.T, first response, second response) {
	return func(t *testing.T, first response, second response) {
		t.Helper()
		if first.status >= http.StatusMultipleChoices || second.status >= http.StatusMultipleChoices {
			t.Fatalf("%s answered %d %s and %d %s", what, first.status, first.body, second.status, second.body)
		}
	}
}

func (h *authHarness) tickAsAnswer(t *testing.T) <-chan pendingAnswer {
	t.Helper()
	ticked := h.tickInBackground(t)
	answer := make(chan pendingAnswer, 1)
	go func() {
		err := <-ticked
		answer <- pendingAnswer{result: response{status: http.StatusOK}, err: err}
	}()
	return answer
}

func endingAndMembershipChange(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	return racingWrites{
		hold: func() func() {
			return h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE group_id = $1::uuid FOR UPDATE", parties.group.ID)
		},
		first: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, tutorsPath+parties.card.ID+"/end", "", jsonCall(parties.studentToken))
		},
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, groupsRoute+"/"+parties.group.ID, fmt.Sprintf(`{"add":[%q]}`, otherCard.ID), jsonCall(parties.tutorToken))
		},
		after: answersOK("the ending and the membership change"),
	}
}

func tutorRenameAndErasure(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
	h.advance(t, time.Minute)
	tutorToken = h.relogin(t, tutorEmail)
	return racingWrites{
		hold: func() func() { return h.holdTutorProfile(t, tutor) },
		first: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, "/api/v1/users/me", `{"name":"A renamed tutor","bio":"A new bio"}`, jsonCall(tutorToken))
		},
		second: func() <-chan pendingAnswer { return h.tickAsAnswer(t) },
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the tutor's rename and the erasure job")(t, first, second)
			if erasing := h.directChannel(t, card.ID); erasing.ErasePending {
				t.Fatalf("the erasure job left the channel %s pending", erasing.ID)
			}
		},
	}
}

func TestMembershipChangesAndEndingsNeverDeadlock(t *testing.T) {
	t.Parallel()
	pairs := map[string]func(t *testing.T, h *authHarness) racingWrites{
		"an ending and a group membership change": endingAndMembershipChange,
		"a tutor's rename and the erasure job":    tutorRenameAndErasure,
	}
	for name, arrange := range pairs {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, reversed %t", name, reversed), func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				writes := arrange(t, h)
				first, second := writes.first, writes.second
				if reversed {
					first, second = second, first
				}
				release := writes.hold()

				firstAnswer := first()
				h.waitForLockWaiters(t, 1)
				secondAnswer := second()
				h.waitForLockWaiters(t, 2)
				release()
				one, other := awaitAnswer(t, firstAnswer), awaitAnswer(t, secondAnswer)
				if reversed {
					one, other = other, one
				}

				writes.after(t, one, other)
				h.assertMirrored(t)
			})
		}
	}
}

func TestDroppingAReclaimedLinkLocksJoinRequestsThenChannelsThenLessons(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	h.mondays(t, tutorToken, card.ID, 2)
	h.enterCode(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), h.storedCode(t, other).Display())
	owner := h.reclaimed(t, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	_, list := h.reclaimList(t, owner)
	requests := make([]string, 0, len(list.Requests))
	for _, request := range list.Requests {
		requests = append(requests, request.RequestID)
	}
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel.ID)

	decision := h.sendInBackground(http.MethodPost, reclaimPath, reclaimChoice{DropLinks: []string{card.ID}, DropRequests: requests}.body(t), jsonCall(owner))
	h.waitForLockWaiters(t, 1)
	requestsHeld := h.lockedElsewhere(t, "join_requests", "student_id", squatter.String())
	lessonsHeld := h.lockedElsewhere(t, "lessons", "relationship_id", card.ID)
	seriesHeld := h.lockedElsewhere(t, "lesson_series", "relationship_id", card.ID)
	release()
	decided := awaitAnswer(t, decision)

	if !requestsHeld || lessonsHeld || seriesHeld {
		t.Fatalf("while waiting for the channel the drop held join requests %t, lessons %t and series %t", requestsHeld, lessonsHeld, seriesHeld)
	}
	h.mustAnswer(t, decided, http.StatusOK)
	if len(requests) != 1 || h.lessonState(t, h.storedLessonOf(t, card.ID)) != "cancelled" {
		t.Fatalf("the owner's choice listed %+v and left the link's lesson uncancelled", list)
	}
	h.assertMirrored(t)
}

func (h *authHarness) storedLessonOf(t *testing.T, relationship string) string {
	t.Helper()
	var lesson string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT id::text FROM lessons WHERE relationship_id = $1::uuid ORDER BY starts_at DESC LIMIT 1", relationship).
		Scan(&lesson)
	if err != nil {
		t.Fatal(err)
	}
	return lesson
}

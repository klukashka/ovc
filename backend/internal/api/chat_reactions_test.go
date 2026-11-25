package api_test

import (
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
	secondStudentEmail = "second-student@example.org"
	secondCardName     = "Boris the writer"
	reactionsOfUser    = "SELECT count(*) FROM message_reactions WHERE user_id = $1::uuid"
	reactionsOfMessage = "SELECT count(*) FROM message_reactions WHERE message_id = $1::uuid"
)

type reactionMember struct {
	Label     *string `json:"label"`
	LabelKind *string `json:"label_kind"`
}

type messageReaction struct {
	Reaction string            `json:"reaction"`
	Count    int64             `json:"count"`
	Mine     bool              `json:"mine"`
	Members  *[]reactionMember `json:"members"`
}

type reactedMessage struct {
	ID        string            `json:"id"`
	Reactions []messageReaction `json:"reactions"`
}

func reactionRequest(reaction string) string {
	return fmt.Sprintf(`{"reaction":%q}`, reaction)
}

func (h *authHarness) react(t *testing.T, token string, message string, reaction string) response {
	t.Helper()
	return h.send(t, http.MethodPut, messagesRoute+message+"/reaction", reactionRequest(reaction), jsonCall(token))
}

func (h *authHarness) unreact(t *testing.T, token string, message string) response {
	t.Helper()
	return h.send(t, http.MethodDelete, messagesRoute+message+"/reaction", "", jsonCall(token))
}

func (h *authHarness) reactionsIn(t *testing.T, token string, channel string, message string) []messageReaction {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+"/"+channel+"/messages", "", jsonCall(token))
	h.mustAnswer(t, result, http.StatusOK)
	var page struct {
		Items []reactedMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.body), &page); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == message {
			return item.Reactions
		}
	}
	t.Fatalf("the history holds no message %s", message)
	return nil
}

type reactionGroup struct {
	parties     endingParties
	second      domain.UserID
	secondToken string
	secondCard  studentCard
	channel     storedChannel
}

func (h *authHarness) reactionGroupOfTwo(t *testing.T) reactionGroup {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	second, secondToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, secondStudentEmail)
	secondCard := h.linkedSignedIn(t, parties.tutorToken, secondToken, secondCardName, secondStudentEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, secondCard.ID)), http.StatusOK)
	return reactionGroup{parties: parties, second: second, secondToken: secondToken, secondCard: secondCard, channel: h.groupChannel(t, parties.group.ID)}
}

func reactionNamed(reactions []messageReaction, name string) (messageReaction, bool) {
	for _, reaction := range reactions {
		if reaction.Reaction == name {
			return reaction, true
		}
	}
	return messageReaction{}, false
}

func memberLabels(reaction messageReaction) []string {
	if reaction.Members == nil {
		return nil
	}
	labels := make([]string, 0, len(*reaction.Members))
	for _, member := range *reaction.Members {
		if member.Label == nil {
			labels = append(labels, "kind:"+*member.LabelKind)
			continue
		}
		labels = append(labels, *member.Label)
	}
	return labels
}

func TestStudentsSeeReactionCountsAndOnlyTheTutorSeesWhoReacted(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	group := h.reactionGroupOfTwo(t)
	parties := group.parties
	asked := h.sent(t, parties.tutorToken, group.channel.ID, "Who finished the reading")
	own := h.sent(t, parties.studentToken, group.channel.ID, "My own note about the reading")
	for token, reaction := range map[string]string{parties.studentToken: "thumbs_up", group.secondToken: "thumbs_up", parties.tutorToken: "heart"} {
		h.mustAnswer(t, h.react(t, token, asked.ID, reaction), http.StatusNoContent)
	}
	h.mustAnswer(t, h.react(t, group.secondToken, own.ID, "check"), http.StatusNoContent)
	base := channelsRoute + "/" + group.channel.ID + "/messages"

	forStudent := map[string]response{
		"history":  h.send(t, http.MethodGet, base, "", jsonCall(parties.studentToken)),
		"around":   h.send(t, http.MethodGet, fmt.Sprintf("%s?around_seq=%d", base, asked.Seq), "", jsonCall(parties.studentToken)),
		"catch-up": h.send(t, http.MethodGet, base+"?after_seq=0", "", jsonCall(parties.studentToken)),
		"search":   h.searchCall(t, parties.studentToken, group.channel.ID, searchBody(t, "reading", nil)),
		"edit":     h.editMessage(t, parties.studentToken, own.ID, "My own note about the reading, corrected", 1),
		"send":     h.sendMessage(t, parties.studentToken, group.channel.ID, "Another note about the reading", nil, ""),
	}
	forTutorPaths := map[string]response{
		"history":  h.send(t, http.MethodGet, base, "", jsonCall(parties.tutorToken)),
		"around":   h.send(t, http.MethodGet, fmt.Sprintf("%s?around_seq=%d", base, asked.Seq), "", jsonCall(parties.tutorToken)),
		"catch-up": h.send(t, http.MethodGet, base+"?after_seq=0", "", jsonCall(parties.tutorToken)),
		"search":   h.searchCall(t, parties.tutorToken, group.channel.ID, searchBody(t, "reading", nil)),
	}
	forTutor := h.reactionsIn(t, parties.tutorToken, group.channel.ID, asked.ID)
	forFirst := h.reactionsIn(t, parties.studentToken, group.channel.ID, asked.ID)

	for name, result := range forStudent {
		if result.status != http.StatusOK && result.status != http.StatusCreated {
			t.Fatalf("the student's %s answered %s", name, describe(result))
		}
		for _, revealing := range []string{`"members"`, group.second.String(), secondCardName} {
			if strings.Contains(result.body, revealing) {
				t.Fatalf("the student's %s reveals %q: %s", name, revealing, result.body)
			}
		}
	}
	if !strings.Contains(forStudent["edit"].body, `"reaction":"check","count":1`) {
		t.Fatalf("the edit response lost the reaction: %s", forStudent["edit"].body)
	}
	thumbs, _ := reactionNamed(forFirst, "thumbs_up")
	heart, _ := reactionNamed(forFirst, "heart")
	if len(forFirst) != 2 || forFirst[0].Reaction != "thumbs_up" || thumbs.Count != 2 || !thumbs.Mine || heart.Count != 1 || heart.Mine {
		t.Fatalf("the student sees %+v", forFirst)
	}
	tutorThumbs, _ := reactionNamed(forTutor, "thumbs_up")
	tutorHeart, _ := reactionNamed(forTutor, "heart")
	for name, result := range forTutorPaths {
		if result.status != http.StatusOK || !strings.Contains(result.body, `"members"`) || !strings.Contains(result.body, secondCardName) {
			t.Fatalf("the tutor's %s lacks the reactors: %s", name, describe(result))
		}
	}
	if !slices.Equal(memberLabels(tutorThumbs), []string{secondCardName, cardName}) {
		t.Fatalf("the tutor sees the reactors %v", memberLabels(tutorThumbs))
	}
	if !tutorHeart.Mine || tutorHeart.Members == nil || len(*tutorHeart.Members) != 0 {
		t.Fatalf("the tutor's own reaction is shown as %+v", tutorHeart)
	}
}

func TestAReactionChangeReachesOnlyActiveMembersAsAMessageUpdate(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	group := h.reactionGroupOfTwo(t)
	parties := group.parties
	message := h.sent(t, parties.tutorToken, group.channel.ID, "React to this")
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, group.secondCard.ID)), http.StatusOK)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 7, "squat-tutor@example.org")
	card, account := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	squatted := h.directChannel(t, card.ID)
	provisional := h.sent(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), squatted.ID, "Provisional words")
	h.reclaimed(t, newcomerEmail)
	log := h.recordFrames(t)

	h.mustAnswer(t, h.react(t, parties.studentToken, message.ID, "joy"), http.StatusNoContent)
	h.mustAnswer(t, h.react(t, tutorToken, provisional.ID, "wow"), http.StatusNoContent)
	frames := log.settle(t, h)

	updated := []sentFrame{{kind: string(domain.FrameMessageUpdated), id: message.ID}}
	if !slices.Equal(chatFramesTo(frames, parties.tutor), updated) || !slices.Equal(chatFramesTo(frames, parties.student), updated) {
		t.Fatalf("the group was sent %v", frames)
	}
	if len(chatFramesTo(frames, group.second)) != 0 || len(chatFramesTo(frames, account)) != 0 {
		t.Fatalf("a removed or suspended member was sent %v and %v", chatFramesTo(frames, group.second), chatFramesTo(frames, account))
	}
}

func TestReactionsReplaceRemoveAndRefuseUnknownValues(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	message := h.sent(t, parties.tutorToken, direct.ID, "Pick one")
	h.mustAnswer(t, h.react(t, parties.studentToken, message.ID, "heart"), http.StatusNoContent)
	log := h.recordFrames(t)

	replaced := h.react(t, parties.studentToken, message.ID, "sad")
	afterReplace := h.reactionsIn(t, parties.studentToken, direct.ID, message.ID)
	unchanged := h.react(t, parties.studentToken, message.ID, "sad")
	quiet := log.settle(t, h)
	removed := h.unreact(t, parties.studentToken, message.ID)
	again := h.unreact(t, parties.studentToken, message.ID)
	afterRemove := h.reactionsIn(t, parties.studentToken, direct.ID, message.ID)
	invalid := map[string]response{
		"a seventh value": h.react(t, parties.studentToken, message.ID, "rocket"),
		"an empty value":  h.react(t, parties.studentToken, message.ID, ""),
		"a wrong case":    h.react(t, parties.studentToken, message.ID, "Heart"),
	}

	for _, answer := range []response{replaced, unchanged, removed, again} {
		h.mustAnswer(t, answer, http.StatusNoContent)
	}
	if len(afterReplace) != 1 || afterReplace[0].Reaction != "sad" || !afterReplace[0].Mine || len(afterRemove) != 0 {
		t.Fatalf("after the replacement %+v, after the removal %+v", afterReplace, afterRemove)
	}
	updates := 0
	for _, frame := range chatFramesTo(quiet, parties.tutor) {
		if frame.kind == string(domain.FrameMessageUpdated) {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("the replacement and the unchanged set sent %d updates", updates)
	}
	for name, answer := range invalid {
		if answer.status != http.StatusUnprocessableEntity || !strings.Contains(answer.body, `"path":"body.reaction"`) {
			t.Fatalf("%s answered %s", name, describe(answer))
		}
	}
}

func TestAReadOnlyChatRefusesNewReactionsButAllowsRemoval(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	message := h.sent(t, parties.tutorToken, direct.ID, "Before the ending")
	h.mustAnswer(t, h.react(t, parties.studentToken, message.ID, "heart"), http.StatusNoContent)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	set := h.react(t, parties.studentToken, message.ID, "joy")
	removed := h.unreact(t, parties.studentToken, message.ID)

	if set.status != http.StatusConflict || !strings.Contains(set.body, fmt.Sprintf(`"code":%q`, domain.ErrorChannelReadOnly)) {
		t.Fatalf("a reaction in a read-only chat answered %s", describe(set))
	}
	h.mustAnswer(t, removed, http.StatusNoContent)
}

func TestReactionsReachOnlyLiveMessagesOfTheCallersChannels(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	group := h.reactionGroupOfTwo(t)
	parties := group.parties
	kept := h.sent(t, parties.tutorToken, group.channel.ID, "Still here")
	gone := h.sent(t, parties.tutorToken, group.channel.ID, "Soon deleted")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, gone.ID), http.StatusNoContent)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, group.secondCard.ID)), http.StatusOK)
	elsewhere := h.sent(t, parties.tutorToken, h.directChannel(t, group.secondCard.ID).ID, "For the second student only")
	_, squatTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 7, "squat-tutor@example.org")
	squatCard, _ := h.squat(t, squatTutorToken, newcomerEmail, newcomerEmail)
	provisional := h.sent(t, squatTutorToken, h.directChannel(t, squatCard.ID).ID, "Before the reclaim")
	h.mustAnswer(t, h.react(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), provisional.ID, "wow"), http.StatusNoContent)
	reclaiming := h.reclaimed(t, newcomerEmail)

	random := h.react(t, parties.studentToken, messageID(99), "heart")
	refusals := map[string]response{
		"a deleted message":              h.react(t, parties.studentToken, gone.ID, "heart"),
		"a removed member":               h.react(t, group.secondToken, kept.ID, "heart"),
		"another channel's message":      h.react(t, parties.studentToken, elsewhere.ID, "heart"),
		"a reclaim_pending account":      h.react(t, reclaiming, provisional.ID, "heart"),
		"removal of a random message":    h.unreact(t, parties.studentToken, messageID(99)),
		"removal on a deleted message":   h.unreact(t, parties.studentToken, gone.ID),
		"removal by a removed member":    h.unreact(t, group.secondToken, kept.ID),
		"removal in another channel":     h.unreact(t, parties.studentToken, elsewhere.ID),
		"removal by a reclaiming caller": h.unreact(t, reclaiming, provisional.ID),
	}

	for name, refused := range refusals {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
}

func TestReactionsCountAsMessageWritesAndRefusalsCountNothing(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	ended := h.sent(t, parties.tutorToken, direct.ID, "Before the ending")
	message := h.sent(t, parties.tutorToken, group.ID, "React here")
	plain := h.sent(t, parties.tutorToken, group.ID, "Nobody reacts here")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.advance(t, domain.MessageWriteWindow)

	refusals := []response{
		h.react(t, parties.tutorToken, message.ID, "rocket"),
		h.react(t, parties.tutorToken, messageID(98), "heart"),
		h.react(t, parties.tutorToken, ended.ID, "heart"),
	}
	counted := []response{
		h.react(t, parties.tutorToken, message.ID, "heart"),
		h.react(t, parties.tutorToken, message.ID, "heart"),
		h.unreact(t, parties.tutorToken, plain.ID),
	}
	for index := range domain.MessageWritesPerMinute - len(counted) {
		counted = append(counted, h.sendMessage(t, parties.tutorToken, group.ID, fmt.Sprintf("Write %d", index), nil, ""))
	}
	limited := h.react(t, parties.tutorToken, message.ID, "joy")

	wantRefusals := []int{http.StatusUnprocessableEntity, http.StatusNotFound, http.StatusConflict}
	for index, refused := range refusals {
		if refused.status != wantRefusals[index] {
			t.Fatalf("refusal %d answered %s", index, describe(refused))
		}
	}
	for index, answer := range counted {
		if answer.status != http.StatusNoContent && answer.status != http.StatusCreated {
			t.Fatalf("write %d answered %s", index, describe(answer))
		}
	}
	if limited.status != http.StatusTooManyRequests || limited.headers.Get("Retry-After") == "" {
		t.Fatalf("the write past the limit answered %s", describe(limited))
	}
}

func TestDeletingAMessageOrPurgingAnAccountRemovesItsReactions(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	group := h.reactionGroupOfTwo(t)
	parties := group.parties
	deleted := h.sent(t, parties.tutorToken, group.channel.ID, "Soon deleted")
	fromSecond := h.sent(t, group.secondToken, group.channel.ID, "From the second student")
	kept := h.sent(t, parties.tutorToken, group.channel.ID, "Kept")
	for _, message := range []string{deleted.ID, fromSecond.ID, kept.ID} {
		h.mustAnswer(t, h.react(t, parties.studentToken, message, "heart"), http.StatusNoContent)
	}
	h.mustAnswer(t, h.react(t, group.secondToken, kept.ID, "joy"), http.StatusNoContent)

	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, deleted.ID), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMe(t, group.secondToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	h.tick(t)
	afterPurge := h.reactionsIn(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), group.channel.ID, kept.ID)

	if h.countRows(t, reactionsOfMessage, deleted.ID) != 0 || h.countRows(t, reactionsOfMessage, fromSecond.ID) != 0 {
		t.Fatal("a deleted or erased message kept its reactions")
	}
	if h.countRows(t, reactionsOfUser, group.second.String()) != 0 || len(afterPurge) != 1 || afterPurge[0].Reaction != "heart" {
		t.Fatalf("after the purge the kept message shows %+v", afterPurge)
	}
}

func TestAPurgedStudentWithoutAGroupMemberRowLosesTheReactionsLeftInTheGroup(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	message := h.sent(t, parties.tutorToken, group.ID, "React before leaving")
	h.mustAnswer(t, h.react(t, parties.studentToken, message.ID, "check"), http.StatusNoContent)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
	memberRows := h.countRows(t, "SELECT count(*) FROM channel_members JOIN channels ON channels.id = channel_members.channel_id WHERE channels.kind = 'group' AND channel_members.user_id = $1::uuid", parties.student.String())
	labelled := h.reactionsIn(t, parties.tutorToken, group.ID, message.ID)

	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	h.tick(t)

	if memberRows != 0 {
		t.Fatalf("the removed student still has %d group member rows", memberRows)
	}
	check, _ := reactionNamed(labelled, "check")
	if !slices.Equal(memberLabels(check), []string{cardName}) {
		t.Fatalf("the tutor saw the removed student's reaction as %v", memberLabels(check))
	}
	if h.countRows(t, reactionsOfUser, parties.student.String()) != 0 {
		t.Fatal("the purged student's reaction stayed")
	}
}

func TestAKeptReclaimRemovesOnlyTheReclaimingAccountsReactions(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	message := h.sent(t, tutorToken, channel.ID, "From the tutor")
	h.mustAnswer(t, h.react(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), message.ID, "heart"), http.StatusNoContent)
	h.mustAnswer(t, h.react(t, tutorToken, message.ID, "check"), http.StatusNoContent)

	owner := h.reclaimed(t, newcomerEmail)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	forOwner := h.reactionsIn(t, owner, channel.ID, message.ID)

	if len(forOwner) != 1 || forOwner[0].Reaction != "check" || forOwner[0].Mine || forOwner[0].Count != 1 {
		t.Fatalf("after the kept reclaim the owner sees %+v", forOwner)
	}
}

func TestEndingsAndMembershipSyncsKeepReactions(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	inDirect := h.sent(t, parties.tutorToken, direct.ID, "In private")
	inGroup := h.sent(t, parties.tutorToken, group.ID, "In the group")
	h.mustAnswer(t, h.react(t, parties.studentToken, inDirect.ID, "heart"), http.StatusNoContent)
	h.mustAnswer(t, h.react(t, parties.tutorToken, inDirect.ID, "check"), http.StatusNoContent)
	h.mustAnswer(t, h.react(t, parties.studentToken, inGroup.ID, "joy"), http.StatusNoContent)
	_, secondToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, secondStudentEmail)
	secondCard := h.linkedSignedIn(t, parties.tutorToken, secondToken, secondCardName, secondStudentEmail)

	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, secondCard.ID)), http.StatusOK)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	if h.countRows(t, reactionsOfMessage, inDirect.ID) != 2 || h.countRows(t, reactionsOfMessage, inGroup.ID) != 1 {
		t.Fatal("an ending or a group addition removed reactions")
	}
}

func TestTheTutorSeesAGroupLeaversReactionUnderTheirCardName(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	message := h.sent(t, parties.tutorToken, group.ID, "React and leave")
	h.mustAnswer(t, h.react(t, parties.studentToken, message.ID, "wow"), http.StatusNoContent)

	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
	forTutor := h.reactionsIn(t, parties.tutorToken, group.ID, message.ID)

	wow, _ := reactionNamed(forTutor, "wow")
	if !slices.Equal(memberLabels(wow), []string{cardName}) || wow.Mine {
		t.Fatalf("the tutor sees the leaver's reaction as %+v", wow)
	}
	if h.countLogs(`"msg":"chat.reaction.set"`, message.ID, `"reaction":"wow"`) != 1 {
		t.Fatal("the reaction was not recorded")
	}
}

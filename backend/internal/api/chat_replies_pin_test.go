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

	"ovc/backend/internal/domain"
)

const replyField = "body.reply_to_message_id:not_found"

type messageQuote struct {
	MessageID         string  `json:"message_id"`
	Seq               int64   `json:"seq"`
	SenderLabel       *string `json:"sender_label"`
	SenderLabelKind   *string `json:"sender_label_kind"`
	Own               bool    `json:"own"`
	Preview           *string `json:"preview"`
	State             string  `json:"state"`
	SentBeforeReclaim bool    `json:"sent_before_reclaim"`
}

type quotingMessage struct {
	ID      string        `json:"id"`
	Seq     int64         `json:"seq"`
	ReplyTo *messageQuote `json:"reply_to"`
}

type quotingPage struct {
	Items      []quotingMessage `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	HasNewer   bool             `json:"has_newer"`
}

func replyRequest(body string, replyTo string) string {
	encoded, err := json.Marshal(struct {
		Body          string   `json:"body"`
		AttachmentIDs []string `json:"attachment_ids"`
		ReplyTo       string   `json:"reply_to_message_id"`
	}{Body: body, AttachmentIDs: []string{}, ReplyTo: replyTo})
	if err != nil {
		panic("a reply request could not be encoded")
	}
	return string(encoded)
}

func (h *authHarness) reply(t *testing.T, token string, channel string, body string, replyTo string, replay string) response {
	t.Helper()
	return h.send(t, http.MethodPost, channelsRoute+"/"+channel+"/messages", replyRequest(body, replyTo), withReplayKey(jsonCall(token), replay))
}

func (h *authHarness) quotingHistory(t *testing.T, token string, channel string, query string) (response, quotingPage) {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+"/"+channel+"/messages"+query, "", jsonCall(token))
	var page quotingPage
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &page); err != nil {
			t.Fatal(err)
		}
	}
	return result, page
}

func (h *authHarness) quoteOf(t *testing.T, token string, channel string, message string) *messageQuote {
	t.Helper()
	result, page := h.quotingHistory(t, token, channel, "?limit=100")
	h.mustAnswer(t, result, http.StatusOK)
	for _, item := range page.Items {
		if item.ID == message {
			return item.ReplyTo
		}
	}
	t.Fatalf("the message %s is not in the history", message)
	return nil
}

func (h *authHarness) pin(t *testing.T, token string, channel string, message string) response {
	t.Helper()
	return h.send(t, http.MethodPut, channelsRoute+"/"+channel+"/pin", fmt.Sprintf(`{"message_id":%q}`, message), jsonCall(token))
}

func (h *authHarness) unpin(t *testing.T, token string, channel string) response {
	t.Helper()
	return h.send(t, http.MethodDelete, channelsRoute+"/"+channel+"/pin", "", jsonCall(token))
}

func (h *authHarness) pinnedIn(t *testing.T, token string, channel string) *messageQuote {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+"/"+channel, "", jsonCall(token))
	h.mustAnswer(t, result, http.StatusOK)
	var detail struct {
		Pinned *messageQuote `json:"pinned"`
	}
	if err := json.Unmarshal([]byte(result.body), &detail); err != nil {
		t.Fatal(err)
	}
	return detail.Pinned
}

func (h *authHarness) accountName(t *testing.T, user domain.UserID) string {
	t.Helper()
	var name string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT name FROM users WHERE id = $1::uuid", user.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func quoteLabel(quote *messageQuote) string {
	switch {
	case quote == nil:
		return "no quote"
	case quote.SenderLabelKind != nil:
		return *quote.SenderLabelKind
	case quote.Own:
		return "own"
	case quote.SenderLabel != nil:
		return *quote.SenderLabel
	}
	return "unlabelled"
}

func previewOfQuote(quote *messageQuote) string {
	if quote == nil || quote.Preview == nil {
		return "no preview"
	}
	return *quote.Preview
}

func TestARepliedMessageIsResolvedOnlyWithinItsChannelBeforeTheReadOnlyRefusal(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	inGroup := h.sent(t, parties.studentToken, group.ID, "In the group")
	quoted := h.sent(t, parties.tutorToken, direct.ID, "The quoted question")
	gone := h.sent(t, parties.tutorToken, direct.ID, "Soon deleted")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, gone.ID), http.StatusNoContent)

	otherChannel := h.reply(t, parties.studentToken, direct.ID, "Answer", inGroup.ID, "")
	random := h.reply(t, parties.studentToken, direct.ID, "Answer", messageID(99), "")
	malformed := h.reply(t, parties.studentToken, direct.ID, "Answer", "not-an-identifier", "")
	deleted := h.reply(t, parties.studentToken, direct.ID, "Answer", gone.ID, "")
	valid := h.reply(t, parties.studentToken, direct.ID, "The answer", quoted.ID, "")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	readOnlyOther := h.reply(t, parties.studentToken, direct.ID, "Answer", inGroup.ID, "")
	readOnlyValid := h.reply(t, parties.studentToken, direct.ID, "Answer", quoted.ID, "")

	for name, refused := range map[string]response{"another channel's message": otherChannel, "a malformed identifier": malformed,
		"a deleted message": deleted, "another channel's message in a read-only chat": readOnlyOther} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Fatalf("a reply to %s answered %s, a random identifier %s", name, describe(refused), describe(random))
		}
	}
	if fields := fieldsOf(t, random); !slices.Equal(fields, []string{replyField}) {
		t.Fatalf("the refusal names %v", fields)
	}
	h.mustAnswer(t, valid, http.StatusCreated)
	if quote := quotingMessageOf(t, valid).ReplyTo; quote == nil || quote.MessageID != quoted.ID || quote.Seq != quoted.Seq || quote.Own ||
		quoteLabel(quote) != h.accountName(t, parties.tutor) || previewOfQuote(quote) != "The quoted question" || quote.State != "available" {
		t.Fatalf("the reply quotes %+v", quote)
	}
	if readOnlyValid.status != http.StatusConflict || code(t, readOnlyValid) != "channel_read_only" {
		t.Fatalf("a valid reply in a read-only chat answered %s", describe(readOnlyValid))
	}
	if h.countLogs(`"msg":"chat.message.persisted"`, `"has_reply":true`) != 1 || h.countLogs(`"has_reply":false`) != 3 {
		t.Fatal("the persisted messages do not record whether they reply")
	}
}

func quotingMessageOf(t *testing.T, result response) quotingMessage {
	t.Helper()
	var message quotingMessage
	if err := json.Unmarshal([]byte(result.body), &message); err != nil {
		t.Fatalf("not a message: %d %s", result.status, result.body)
	}
	return message
}

func TestAQuoteFollowsItsMessageAndEachReadersLabel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	group := h.groupChannel(t, parties.group.ID)
	byOther := h.sent(t, otherToken, group.ID, "First version")
	reply := quotingMessageOf(t, h.reply(t, parties.studentToken, group.ID, "About that", byOther.ID, ""))

	forStudent := h.quoteOf(t, parties.studentToken, group.ID, reply.ID)
	forTutor := h.quoteOf(t, parties.tutorToken, group.ID, reply.ID)
	forAuthor := h.quoteOf(t, otherToken, group.ID, reply.ID)
	h.mustAnswer(t, h.editMessage(t, otherToken, byOther.ID, "Second version", 1), http.StatusOK)
	afterEdit := h.quoteOf(t, parties.studentToken, group.ID, reply.ID)
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, byOther.ID), http.StatusNoContent)
	afterDelete := h.quoteOf(t, parties.studentToken, group.ID, reply.ID)

	if quoteLabel(forStudent) != h.accountName(t, other) || quoteLabel(forTutor) != "Another card" || quoteLabel(forAuthor) != "own" {
		t.Fatalf("the quote's sender is %q for the student, %q for the tutor and %q for its author", quoteLabel(forStudent), quoteLabel(forTutor), quoteLabel(forAuthor))
	}
	if previewOfQuote(forStudent) != "First version" || previewOfQuote(afterEdit) != "Second version" || afterEdit.State != "available" {
		t.Fatalf("the quote shows %q, then %q", previewOfQuote(forStudent), previewOfQuote(afterEdit))
	}
	if afterDelete == nil || afterDelete.State != "deleted" || afterDelete.Preview != nil {
		t.Fatalf("after the delete the quote is %+v", afterDelete)
	}
}

func TestAQuoteOfAPurgedSendersMessageShowsItRemoved(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	fromStudent := h.sent(t, parties.studentToken, direct.ID, "The student's question")
	reply := quotingMessageOf(t, h.reply(t, parties.tutorToken, direct.ID, "The answer", fromStudent.ID, ""))
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	beforePurge := h.quoteOf(t, parties.tutorToken, direct.ID, reply.ID)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)

	h.tick(t)
	afterPurge := h.quoteOf(t, h.relogin(t, tutorEmail), direct.ID, reply.ID)

	if quoteLabel(beforePurge) != string(domain.LabelDeletedUser) || previewOfQuote(beforePurge) != "The student's question" {
		t.Fatalf("before the purge the quote is %+v", beforePurge)
	}
	if afterPurge == nil || afterPurge.State != "removed" || afterPurge.Preview != nil || quoteLabel(afterPurge) != string(domain.LabelDeletedUser) {
		t.Fatalf("after the purge the quote is %+v", afterPurge)
	}
}

func TestAReplayedReplyAnswersTheFirstMessage(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	first := h.sent(t, parties.tutorToken, direct.ID, "First")
	second := h.sent(t, parties.tutorToken, direct.ID, "Second")

	sent := h.reply(t, parties.studentToken, direct.ID, "Answer", first.ID, firstReplay)
	replayed := h.reply(t, parties.studentToken, direct.ID, "Answer", second.ID, firstReplay)

	h.mustAnswer(t, sent, http.StatusCreated)
	h.mustAnswer(t, replayed, http.StatusCreated)
	if original, again := quotingMessageOf(t, sent), quotingMessageOf(t, replayed); original.ID != again.ID || again.ReplyTo == nil || again.ReplyTo.MessageID != first.ID {
		t.Fatalf("the replay answered %+v for %+v", again, original)
	}
	if h.messageCount(t, direct.ID) != 3 {
		t.Fatal("the replay created a message")
	}
}

func TestHistoryAroundASequenceHoldsItAndHalfALimitOnEachSide(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 10)

	middle, window := h.quotingHistory(t, parties.studentToken, direct.ID, "?around_seq=5&limit=4")
	end, last := h.quotingHistory(t, parties.studentToken, direct.ID, "?around_seq=100&limit=4")
	withCursor, _ := h.quotingHistory(t, parties.studentToken, direct.ID, "?around_seq=5&cursor=Mw")
	withAfter, _ := h.quotingHistory(t, parties.studentToken, direct.ID, "?around_seq=5&after_seq=1")

	seqs := func(page quotingPage) []int64 {
		listed := make([]int64, 0, len(page.Items))
		for _, item := range page.Items {
			listed = append(listed, item.Seq)
		}
		return listed
	}
	h.mustAnswer(t, middle, http.StatusOK)
	h.mustAnswer(t, end, http.StatusOK)
	if !slices.Equal(seqs(window), []int64{4, 5, 6, 7}) || !window.HasNewer || window.NextCursor == nil {
		t.Fatalf("around 5 the page is %v, newer %v, cursor %v", seqs(window), window.HasNewer, window.NextCursor)
	}
	if !slices.Equal(seqs(last), []int64{9, 10}) || last.HasNewer || last.NextCursor == nil {
		t.Fatalf("around 100 the page is %v, newer %v", seqs(last), last.HasNewer)
	}
	for name, refused := range map[string]response{"a cursor": withCursor, "an after sequence": withAfter} {
		if refused.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, refused), []string{"query.around_seq:invalid"}) {
			t.Fatalf("around with %s answered %s", name, describe(refused))
		}
	}
}

func TestOnlyTheChannelsTutorPinsAndActiveMembersLearnOfIt(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	removed, removedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	removedCard := h.linkedSignedIn(t, parties.tutorToken, removedToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, removedCard.ID)), http.StatusOK)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, removedCard.ID)), http.StatusOK)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	fromStudent := h.sent(t, parties.studentToken, group.ID, "Pin this one")
	inDirect := h.sent(t, parties.tutorToken, direct.ID, "In private")
	gone := h.sent(t, parties.tutorToken, group.ID, "Soon deleted")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, gone.ID), http.StatusNoContent)
	log := h.recordFrames(t)

	byStudent := h.pin(t, parties.studentToken, group.ID, fromStudent.ID)
	random := h.pin(t, parties.tutorToken, group.ID, messageID(99))
	otherChannel := h.pin(t, parties.tutorToken, group.ID, inDirect.ID)
	malformed := h.pin(t, parties.tutorToken, group.ID, "not-an-identifier")
	deleted := h.pin(t, parties.tutorToken, group.ID, gone.ID)
	pinned := h.pin(t, parties.tutorToken, group.ID, fromStudent.ID)
	frames := log.settle(t, h)
	forStudent := h.pinnedIn(t, parties.studentToken, group.ID)
	forTutor := h.pinnedIn(t, parties.tutorToken, group.ID)

	if byStudent.status != http.StatusNotFound {
		t.Fatalf("a student pinned with %s", describe(byStudent))
	}
	if fields := fieldsOf(t, random); random.status != http.StatusNotFound || !slices.Equal(fields, []string{"body.message_id:not_found"}) {
		t.Fatalf("pinning a random message answered %s", describe(random))
	}
	for name, refused := range map[string]response{"another channel's message": otherChannel, "a malformed identifier": malformed, "a deleted message": deleted} {
		if refusal(t, refused) != refusal(t, random) {
			t.Fatalf("pinning %s answered %s", name, describe(refused))
		}
	}
	h.mustAnswer(t, pinned, http.StatusNoContent)
	pinFrame := []sentFrame{{kind: string(domain.EntityChannelPin), id: group.ID}}
	if !slices.Equal(framesTo(frames, parties.student), pinFrame) || !hasFrame(frames, parties.tutor, domain.EntityChannelPin, group.ID) ||
		hasFrame(frames, parties.tutor, domain.EntityChannel, group.ID) || len(framesTo(frames, removed)) != 0 {
		t.Fatalf("the pin sent the student %v, the tutor %v and the removed member %v", framesTo(frames, parties.student),
			framesTo(frames, parties.tutor), framesTo(frames, removed))
	}
	if forStudent == nil || forStudent.MessageID != fromStudent.ID || !forStudent.Own || previewOfQuote(forStudent) != "Pin this one" {
		t.Fatalf("the student sees the pin %+v", forStudent)
	}
	if quoteLabel(forTutor) != cardName {
		t.Fatalf("the tutor sees the pin from %q", quoteLabel(forTutor))
	}
	if refused, _ := h.channelCall(t, removedToken, group.ID); refused.status != http.StatusNotFound {
		t.Fatalf("the removed member read the channel: %s", describe(refused))
	}
	if h.countLogs(`"msg":"chat.message.pinned"`, group.ID, fromStudent.ID) != 1 {
		t.Fatal("the pin was not recorded")
	}
}

func TestDeletingThePinnedMessageUnpinsItAndAReadOnlyChatKeepsOnlyUnpinning(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	first := h.sent(t, parties.studentToken, direct.ID, "First")
	second := h.sent(t, parties.studentToken, direct.ID, "Second")
	h.mustAnswer(t, h.pin(t, parties.tutorToken, direct.ID, first.ID), http.StatusNoContent)
	log := h.recordFrames(t)

	h.mustAnswer(t, h.deleteMessage(t, parties.studentToken, first.ID), http.StatusNoContent)
	deleteFrames := log.settle(t, h)
	afterDelete := h.pinnedIn(t, parties.tutorToken, direct.ID)
	h.mustAnswer(t, h.pin(t, parties.tutorToken, direct.ID, second.ID), http.StatusNoContent)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	readOnlyPin := h.pin(t, parties.tutorToken, direct.ID, second.ID)
	keptPin := h.pinnedIn(t, parties.studentToken, direct.ID)
	log.settle(t, h)
	unpinned := h.unpin(t, parties.tutorToken, direct.ID)
	unpinFrames := log.settle(t, h)
	again := h.unpin(t, parties.tutorToken, direct.ID)
	againFrames := log.settle(t, h)

	pinFrame := []sentFrame{{kind: string(domain.EntityChannelPin), id: direct.ID}}
	if afterDelete != nil || !slices.Equal(channelKindsOnly(framesTo(deleteFrames, parties.tutor)), pinFrame) ||
		!slices.Equal(channelKindsOnly(framesTo(deleteFrames, parties.student)), pinFrame) {
		t.Fatalf("after the delete the pin is %+v and the frames were %v", afterDelete, deleteFrames)
	}
	if h.countLogs(`"msg":"chat.message.unpinned"`, first.ID, `"cause":"deleted"`) != 1 {
		t.Fatal("the delete's unpin was not recorded")
	}
	if readOnlyPin.status != http.StatusConflict || code(t, readOnlyPin) != "channel_read_only" || keptPin == nil || keptPin.MessageID != second.ID {
		t.Fatalf("in the read-only chat pinning answered %s and the pin is %+v", describe(readOnlyPin), keptPin)
	}
	h.mustAnswer(t, unpinned, http.StatusNoContent)
	h.mustAnswer(t, again, http.StatusNoContent)
	if !slices.Equal(channelKindsOnly(framesTo(unpinFrames, parties.student)), pinFrame) ||
		!slices.Equal(channelKindsOnly(framesTo(unpinFrames, parties.tutor)), pinFrame) || len(againFrames) != 0 {
		t.Fatalf("the unpin sent %v and the repeated unpin %v", unpinFrames, againFrames)
	}
	if h.countLogs(`"msg":"chat.message.unpinned"`, second.ID) != 1 || h.pinnedIn(t, parties.studentToken, direct.ID) != nil {
		t.Fatal("the unpin was not recorded exactly once or the pin stayed")
	}
}

func channelKindsOnly(frames []sentFrame) []sentFrame {
	kept := make([]sentFrame, 0, len(frames))
	for _, frame := range frames {
		if frame.kind == string(domain.EntityChannel) || frame.kind == string(domain.EntityChannelPin) {
			kept = append(kept, frame)
		}
	}
	return kept
}

func TestAPinChangeSkipsASuspendedReclaimPendingMember(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	message := h.sent(t, squatterToken, channel.ID, "Pinned before the reclaim")
	h.mustAnswer(t, h.pin(t, tutorToken, channel.ID, message.ID), http.StatusNoContent)
	h.reclaimed(t, newcomerEmail)
	tutorAgain := h.relogin(t, tutorEmail)
	log := h.recordFrames(t)

	unpinned := h.unpin(t, tutorAgain, channel.ID)
	frames := log.settle(t, h)

	h.mustAnswer(t, unpinned, http.StatusNoContent)
	if !slices.Equal(channelKindsOnly(framesTo(frames, tutor)), []sentFrame{{kind: string(domain.EntityChannelPin), id: channel.ID}}) {
		t.Fatalf("the unpin sent the tutor %v", framesTo(frames, tutor))
	}
	if got := framesTo(frames, squatter); len(got) != 0 {
		t.Fatalf("the unpin sent the suspended reclaim_pending member %v", got)
	}
}

func TestPinsCountAsMessageWrites(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	message := h.sent(t, parties.tutorToken, direct.ID, "Pinned and unpinned")

	answers := make([]int, 0, 30)
	for index := range 29 {
		if index%2 == 0 {
			answers = append(answers, h.pin(t, parties.tutorToken, direct.ID, message.ID).status)
		} else {
			answers = append(answers, h.unpin(t, parties.tutorToken, direct.ID).status)
		}
	}
	limited := h.pin(t, parties.tutorToken, direct.ID, message.ID)

	if slices.ContainsFunc(answers, func(status int) bool { return status != http.StatusNoContent }) {
		t.Fatalf("the writes within the limit answered %v", answers)
	}
	if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Retry-After") == "" {
		t.Fatalf("the thirty-first write answered %s", describe(limited))
	}
}

func TestPurgesAndTheErasureJobClearThePin(t *testing.T) {
	t.Parallel()
	t.Run("a tutor purge with a pin and no files", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.activeStudentInAGroup(t)
		direct := h.directChannel(t, parties.card.ID)
		message := h.sent(t, parties.studentToken, direct.ID, "Pinned before the purge")
		h.mustAnswer(t, h.pin(t, parties.tutorToken, direct.ID, message.ID), http.StatusNoContent)
		h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
		h.advance(t, domain.DeletedAccountRetention+time.Hour)

		h.tick(t)
		h.tick(t)

		if h.userExists(t, tutorEmail) || h.countLogs(`"msg":"account.purge_failed"`) != 0 || h.messageCount(t, direct.ID) != 0 {
			t.Fatalf("the tutor's purge did not finish:\n%s", h.failures())
		}
	})
	t.Run("a student purge", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.activeStudentInAGroup(t)
		direct := h.directChannel(t, parties.card.ID)
		message := h.sent(t, parties.studentToken, direct.ID, "Pinned before the purge")
		h.mustAnswer(t, h.pin(t, parties.tutorToken, direct.ID, message.ID), http.StatusNoContent)
		h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
		h.advance(t, domain.DeletedAccountRetention+time.Hour)
		log := h.recordFrames(t)

		h.tick(t)
		frames := log.settle(t, h)
		after := h.pinnedIn(t, h.relogin(t, tutorEmail), direct.ID)

		if h.userExists(t, studentEmail) || after != nil || h.countRows(t, "SELECT count(id) FROM channels WHERE id = $1::uuid AND pinned_message_id IS NOT NULL", direct.ID) != 0 {
			t.Fatalf("after the student's purge the pin is %+v", after)
		}
		if h.countLogs(`"msg":"chat.message.unpinned"`, message.ID, `"cause":"purged"`) != 1 {
			t.Fatal("the purge's unpin was not recorded")
		}
		if got := framesOfKind(framesTo(frames, parties.tutor), domain.EntityChannelPin); !slices.Equal(got, []sentFrame{{kind: string(domain.EntityChannelPin), id: direct.ID}}) {
			t.Fatalf("the purge's unpin sent the tutor the pin frames %v", got)
		}
	})
	t.Run("the erasure job", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		channel := h.directChannel(t, card.ID)
		squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		message := h.sent(t, squatterToken, channel.ID, "Pinned before the erasure")
		h.mustAnswer(t, h.pin(t, tutorToken, channel.ID, message.ID), http.StatusNoContent)
		h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
		h.advance(t, time.Minute)
		log := h.recordFrames(t)

		h.tick(t)
		frames := log.settle(t, h)

		if h.messageCount(t, channel.ID) != 0 || h.directChannel(t, card.ID).ErasePending {
			t.Fatalf("the erasure left %d messages:\n%s", h.messageCount(t, channel.ID), h.failures())
		}
		if h.countLogs(`"msg":"chat.message.unpinned"`, message.ID, `"cause":"erased"`) != 1 {
			t.Fatal("the erasure's unpin was not recorded")
		}
		if got := framesOfKind(frames, domain.EntityChannelPin); len(got) != 0 {
			t.Fatalf("the erasure job sent the pin frames %v", got)
		}
	})
}

func TestQuotesAndThePinCarryTheReclaimCueForBothSides(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	before := h.sent(t, squatterToken, channel.ID, "Sent by whoever held the account")
	reply := quotingMessageOf(t, h.reply(t, tutorToken, channel.ID, "Answering it", before.ID, ""))
	h.mustAnswer(t, h.pin(t, tutorToken, channel.ID, before.ID), http.StatusNoContent)
	owner := h.reclaimed(t, newcomerEmail)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	tutorAgain := h.relogin(t, tutorEmail)

	quoteForTutor := h.quoteOf(t, tutorAgain, channel.ID, reply.ID)
	quoteForOwner := h.quoteOf(t, owner, channel.ID, reply.ID)
	pinForTutor := h.pinnedIn(t, tutorAgain, channel.ID)
	pinForOwner := h.pinnedIn(t, owner, channel.ID)

	for name, quote := range map[string]*messageQuote{"the tutor's quote": quoteForTutor, "the owner's quote": quoteForOwner,
		"the tutor's pin": pinForTutor, "the owner's pin": pinForOwner} {
		if quote == nil || !quote.SentBeforeReclaim || quote.MessageID != before.ID {
			t.Fatalf("%s is %+v", name, quote)
		}
	}
	if !strings.Contains(previewOfQuote(pinForOwner), "whoever held the account") || !pinForOwner.Own {
		t.Fatalf("the owner sees the pin %+v", pinForOwner)
	}
}

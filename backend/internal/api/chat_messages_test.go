package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	firstReplay  = "send-1.first"
	secondReplay = "send-2.second"
)

func TestSendingStoresAMessageAndSignalsItsMembers(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	log := h.recordFrames(t)

	sent := h.sendMessage(t, parties.studentToken, group.ID, "Hello, everyone", nil, "")
	frames := log.settle(t, h)
	_, tutorPage := h.channelList(t, parties.tutorToken, "")

	h.mustAnswer(t, sent, http.StatusCreated)
	message := messageOf(t, sent)
	if message.Seq != 1 || !message.Own || message.Body == nil || *message.Body != "Hello, everyone" || message.Version != 1 ||
		message.Deleted || message.Erased || message.SentBeforeReclaim || len(message.Attachments) != 0 || sent.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the sender sees %+v", message)
	}
	stored := h.storedMessage(t, message.ID)
	if stored.Body == nil || *stored.Body != "Hello, everyone" || stored.Sender == nil || *stored.Sender != parties.student.String() ||
		h.groupChannel(t, parties.group.ID).LastSeq != 1 {
		t.Fatalf("the stored message is %+v", stored)
	}
	want := []sentFrame{{kind: string(domain.FrameMessageNew), id: message.ID}}
	if !slices.Equal(chatFramesTo(frames, parties.tutor), want) || !slices.Equal(chatFramesTo(frames, parties.student), want) {
		t.Fatalf("the members were sent %v and %v", chatFramesTo(frames, parties.tutor), chatFramesTo(frames, parties.student))
	}
	item, _ := itemOf(tutorPage, group.ID)
	last := lastMessageOf(t, item)
	if last.Seq != 1 || last.Own || last.SenderLabel == nil || *last.SenderLabel != cardName || last.Preview == nil || *last.Preview != "Hello, everyone" ||
		item.UnreadCount != 1 || h.channelMembers(t, group.ID)[parties.student.String()].LastRead != 1 {
		t.Fatalf("the tutor lists the channel as %+v with %+v", item, last)
	}
	t.Run("the chat signal is parsed strictly", func(t *testing.T) {
		t.Parallel()
		channel, recipient := group.ID, parties.tutor.String()
		refused := []string{
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"message.new","c":%q,"i":%q,"s":null}`, recipient, channel, message.ID),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"message.updated","c":%q,"i":%q,"s":1}`, recipient, channel, message.ID),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"read.updated","c":%q,"i":%q,"s":null}`, recipient, channel, message.ID),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"message.send","c":%q,"i":%q,"s":1}`, recipient, channel, message.ID),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"message.new","c":%q,"i":%q,"s":0}`, recipient, channel, message.ID),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"read.updated","c":%q,"i":null}`, recipient, channel),
			fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"read.updated","c":%q,"i":null,"s":null,"x":1}`, recipient, channel),
		}

		accepted, acceptedErr := domain.ParseSignal(fmt.Sprintf(`{"v":1,"k":"chat","u":[%q],"e":"read.updated","c":%q,"i":null,"s":null}`, recipient, channel))

		if acceptedErr != nil || accepted.Kind() != domain.SignalChat {
			t.Fatalf("a read signal was refused: %v", acceptedErr)
		}
		for _, payload := range refused {
			if _, err := domain.ParseSignal(payload); err == nil {
				t.Fatalf("the listener accepted %s", payload)
			}
		}
	})
}

func TestARepeatedIdempotencyKeyIsReauthorizedAndReturnsTheCurrentMessage(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	direct := h.directChannel(t, parties.card.ID)
	first := h.sendMessage(t, parties.studentToken, group.ID, "First words", nil, firstReplay)
	h.mustAnswer(t, first, http.StatusCreated)
	original := messageOf(t, first)
	h.mustAnswer(t, h.editMessage(t, parties.studentToken, original.ID, "Edited words", 1), http.StatusOK)

	repeated := h.sendMessage(t, parties.studentToken, group.ID, "Different words", nil, firstReplay)
	elsewhere := h.sendMessage(t, parties.studentToken, direct.ID, "First words", nil, firstReplay)
	malformed := h.sendMessage(t, parties.studentToken, group.ID, "Words", nil, "not a valid key!")
	tooLong := h.sendMessage(t, parties.studentToken, group.ID, "Words", nil, strings.Repeat("k", 65))
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
	afterLeaving := h.sendMessage(t, parties.studentToken, group.ID, "First words", nil, firstReplay)
	random := h.sendMessage(t, parties.studentToken, randomChannelID, "First words", nil, secondReplay)

	h.mustAnswer(t, repeated, http.StatusCreated)
	replayed := messageOf(t, repeated)
	if replayed.ID != original.ID || replayed.Body == nil || *replayed.Body != "Edited words" || replayed.Version != 2 || h.messageCount(t, group.ID) != 1 {
		t.Fatalf("the repeated key answered %+v and the channel holds %d messages", replayed, h.messageCount(t, group.ID))
	}
	h.mustAnswer(t, elsewhere, http.StatusCreated)
	if messageOf(t, elsewhere).ID == original.ID || h.messageCount(t, direct.ID) != 1 {
		t.Fatal("the key of another channel replayed the first message")
	}
	if malformed.status != http.StatusUnprocessableEntity || tooLong.status != http.StatusUnprocessableEntity || code(t, malformed) != "validation_failed" {
		t.Fatalf("malformed keys answered %s and %s", describe(malformed), describe(tooLong))
	}
	if afterLeaving.status != http.StatusNotFound || refusal(t, afterLeaving) != refusal(t, random) {
		t.Fatalf("a member who left replayed the key: %s", describe(afterLeaving))
	}
}

func TestBodiesArePlainTextWithinTheirBounds(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	longest := strings.Repeat("ё", domain.MessageBodyMaxCodePoints)
	markup := "  <b>bold</b>\n\tand <script>x()</script>  "

	accepted := h.sendMessage(t, parties.studentToken, direct.ID, longest, nil, "")
	kept := h.sendMessage(t, parties.studentToken, direct.ID, markup, nil, "")
	cases := map[string]struct {
		body string
		code string
	}{
		"over the limit":          {longest + "ё", "too_long"},
		"a NUL":                   {"before\x00after", "invalid"},
		"a carriage return":       {"before\rafter", "invalid"},
		"an escape":               {"before\x1bafter", "invalid"},
		"a C1 control":            {"before\u0085after", "invalid"},
		"an empty body":           {"", "required"},
		"a whitespace-only body":  {" \n\t ", "required"},
		"over the limit, escaped": {strings.Repeat("a", domain.MessageBodyMaxCodePoints+1), "too_long"},
	}
	refusals := map[string]response{}
	for name, refused := range cases {
		refusals[name] = h.sendMessage(t, parties.studentToken, direct.ID, refused.body, nil, "")
	}

	outOfScope := h.sendMessage(t, parties.studentToken, randomChannelID, "", nil, "")
	group := h.groupChannel(t, parties.group.ID)
	h.mustAnswer(t, h.archive(t, parties.tutorToken, parties.group.ID), http.StatusOK)
	readOnly := h.sendMessage(t, parties.studentToken, group.ID, "before\x00after", nil, "")

	h.mustAnswer(t, accepted, http.StatusCreated)
	h.mustAnswer(t, kept, http.StatusCreated)
	if outOfScope.status != http.StatusNotFound || readOnly.status != http.StatusConflict || code(t, readOnly) != "channel_read_only" {
		t.Fatalf("an invalid body answered %s outside the caller's channels and %s in a read-only one", describe(outOfScope), describe(readOnly))
	}
	if body := messageOf(t, kept).Body; body == nil || *body != markup {
		t.Fatalf("the body was not stored as entered: %v", body)
	}
	for name, refused := range cases {
		result := refusals[name]
		if result.status != http.StatusUnprocessableEntity || fieldList(t, result) != "body.body:"+refused.code {
			t.Fatalf("%s answered %s", name, describe(result))
		}
	}
	if h.messageCount(t, direct.ID) != 2 {
		t.Fatalf("refused bodies left %d messages", h.messageCount(t, direct.ID))
	}
}

func TestAnAttachmentOnlyMessageIsAccepted(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "notes.txt", 64)

	sent := h.sendMessage(t, parties.studentToken, direct.ID, " ", []string{upload}, "")
	_, page := h.channelList(t, parties.tutorToken, "")

	h.mustAnswer(t, sent, http.StatusCreated)
	message := messageOf(t, sent)
	if len(message.Attachments) != 1 || message.Attachments[0].ID != upload || message.Attachments[0].Filename != "notes.txt" ||
		message.Attachments[0].SizeBytes != 64 || !strings.HasPrefix(message.Attachments[0].URL, "/api/v1/files/"+upload+"?t=") {
		t.Fatalf("the attachment-only message is %+v", message)
	}
	item, _ := itemOf(page, direct.ID)
	if last := lastMessageOf(t, item); last.Preview != nil || last.AttachmentCount != 1 {
		t.Fatalf("the list previews the attachment-only message as %+v", last)
	}
	if downloaded := h.fetch(t, message.Attachments[0].URL, nil); downloaded.status != http.StatusOK || downloaded.body != string(plainText(64)) {
		t.Fatalf("the attachment's address answered %s", describe(downloaded))
	}
}

func TestSendsBindOnlyTheCallersPendingUploadsOfTheChannel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	others := h.pendingUpload(t, parties.tutorToken, direct.ID, "tutor.txt", 16)
	otherChannel := h.pendingUpload(t, parties.studentToken, group.ID, "group.txt", 16)
	bound := h.pendingUpload(t, parties.studentToken, direct.ID, "bound.txt", 16)
	h.sent(t, parties.studentToken, direct.ID, "Bound already", bound)
	expiring := h.pendingUpload(t, parties.studentToken, direct.ID, "late.txt", 16)
	h.exec(t, "UPDATE attachments SET expires_at = now() - interval '1 second' WHERE id = $1::uuid", expiring)
	own := make([]string, 0, domain.MessageAttachmentsMax+1)
	for index := range domain.MessageAttachmentsMax + 1 {
		own = append(own, h.pendingUpload(t, parties.studentToken, direct.ID, fmt.Sprintf("own-%d.txt", index), 16))
	}
	cases := map[string][]string{
		"another member's upload":       {others},
		"an upload to another channel":  {otherChannel},
		"an upload bound already":       {bound},
		"an expired upload":             {expiring},
		"a random identifier":           {fixtureID(12, 1)},
		"a random identifier among own": {own[0], fixtureID(12, 2)},
	}
	answers := map[string]response{}
	for name, attachments := range cases {
		answers[name] = h.sendMessage(t, parties.studentToken, direct.ID, "With files", attachments, "")
	}
	eleven := h.sendMessage(t, parties.studentToken, direct.ID, "With files", own, "")
	elevenRandom := h.sendMessage(t, parties.studentToken, direct.ID, "With files", append(slices.Clone(own[:domain.MessageAttachmentsMax]), fixtureID(12, 3)), "")

	ten := h.sendMessage(t, parties.studentToken, direct.ID, "With files", own[:domain.MessageAttachmentsMax], "")

	for name := range cases {
		if answers[name].status != http.StatusNotFound {
			t.Fatalf("%s answered %s", name, describe(answers[name]))
		}
	}
	if eleven.status != http.StatusUnprocessableEntity || fieldList(t, eleven) != "body.attachment_ids:invalid" || refusal(t, eleven) != refusal(t, elevenRandom) {
		t.Fatalf("eleven attachments answered %s and eleven with an unknown one %s", describe(eleven), describe(elevenRandom))
	}
	h.mustAnswer(t, ten, http.StatusCreated)
	if got := len(messageOf(t, ten).Attachments); got != domain.MessageAttachmentsMax || h.uploadState(t, own[domain.MessageAttachmentsMax]) != "pending" {
		t.Fatalf("ten attachments bound %d", got)
	}
	for _, untouched := range []string{others, otherChannel, expiring} {
		if h.referencesOf(t, untouched) != 0 {
			t.Fatalf("the refused upload %s was bound", untouched)
		}
	}
	if h.messageCount(t, direct.ID) != 2 {
		t.Fatalf("the refused sends left %d messages", h.messageCount(t, direct.ID))
	}
}

func TestBindingChargesTheTutorOnceAndRefusesOverTheQuota(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	first := h.pendingUpload(t, parties.studentToken, direct.ID, "first.txt", 100)
	second := h.pendingUpload(t, parties.tutorToken, direct.ID, "second.txt", 50)
	over := h.pendingUpload(t, parties.studentToken, direct.ID, "over.txt", 200)

	h.sent(t, parties.studentToken, direct.ID, "One", first)
	h.sent(t, parties.tutorToken, direct.ID, "Two", second)
	charged := h.usage(t, parties.tutor)
	h.setQuota(t, parties.tutor, charged+199, charged)
	refused := h.sendMessage(t, parties.studentToken, direct.ID, "Too much", []string{over}, "")

	if charged != 150 || h.chargesOf(t, first) != 1 || h.chargesOf(t, second) != 1 {
		t.Fatalf("binding charged %d bytes with charges %d and %d", charged, h.chargesOf(t, first), h.chargesOf(t, second))
	}
	if refused.status != http.StatusInsufficientStorage || code(t, refused) != "quota_exceeded" {
		t.Fatalf("binding over the quota answered %s", describe(refused))
	}
	if h.usage(t, parties.tutor) != charged || h.uploadState(t, over) != "pending" || h.chargesOf(t, over) != 0 || h.messageCount(t, direct.ID) != 2 ||
		h.directChannel(t, parties.card.ID).LastSeq != 2 {
		t.Fatal("the refused binding changed the usage, the upload or the channel")
	}
}

func TestEditsWithinTwentyFourHoursAndEditWindowPassedAfter(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	message := h.sent(t, parties.studentToken, direct.ID, "Original")
	other := h.sent(t, parties.tutorToken, direct.ID, "The tutor's")
	log := h.recordFrames(t)

	edited := h.editMessage(t, parties.studentToken, message.ID, "Corrected", 1)
	frames := log.settle(t, h)
	stale := h.editMessage(t, parties.studentToken, message.ID, "Again", 1)
	notAuthor := h.editMessage(t, parties.studentToken, other.ID, "Mine now", 1)
	empty := h.editMessage(t, parties.studentToken, message.ID, " ", 2)
	h.advance(t, domain.MessageEditWindow-time.Minute)
	studentToken := h.relogin(t, studentEmail)
	lastMinute := h.editMessage(t, studentToken, message.ID, "Just in time", 2)
	h.advance(t, time.Minute)
	late := h.editMessage(t, studentToken, message.ID, "Too late", 3)
	lateAndEmpty := h.editMessage(t, studentToken, message.ID, " ", 3)

	h.mustAnswer(t, edited, http.StatusOK)
	if got := messageOf(t, edited); got.Version != 2 || got.EditedAt == nil || got.Body == nil || *got.Body != "Corrected" {
		t.Fatalf("the edit answered %+v", got)
	}
	if !slices.Equal(chatFramesTo(frames, parties.tutor), []sentFrame{{kind: string(domain.FrameMessageUpdated), id: message.ID}}) {
		t.Fatalf("the tutor was sent %v", chatFramesTo(frames, parties.tutor))
	}
	if stale.status != http.StatusConflict || code(t, stale) != "conflict" {
		t.Fatalf("a stale version answered %s", describe(stale))
	}
	if notAuthor.status != http.StatusNotFound || empty.status != http.StatusUnprocessableEntity {
		t.Fatalf("another author's message answered %s and an empty body %s", describe(notAuthor), describe(empty))
	}
	h.mustAnswer(t, lastMinute, http.StatusOK)
	if late.status != http.StatusConflict || code(t, late) != "edit_window_passed" || code(t, lateAndEmpty) != "edit_window_passed" {
		t.Fatalf("an edit after 24 hours answered %s", describe(late))
	}
	if stored := h.storedMessage(t, message.ID); stored.Version != 3 || *stored.Body != "Just in time" {
		t.Fatalf("the stored message is %+v", stored)
	}
}

func TestDeletingErasesTheBodyAndReleasesTheFiles(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "notes.txt", 80)
	message := h.sent(t, parties.studentToken, direct.ID, "With a file", upload)
	address := message.Attachments[0].URL
	log := h.recordFrames(t)

	deleted := h.deleteMessage(t, parties.studentToken, message.ID)
	frames := log.settle(t, h)
	repeated := h.deleteMessage(t, parties.studentToken, message.ID)
	listed := h.messagesOf(t, parties.tutorToken, direct.ID)
	addressAfter, _ := h.addressOf(t, parties.tutorToken, upload, "")

	h.mustAnswer(t, deleted, http.StatusNoContent)
	h.mustAnswer(t, repeated, http.StatusNoContent)
	stored := h.storedMessage(t, message.ID)
	if stored.Body != nil || !stored.Deleted || h.referencesOf(t, upload) != 0 || h.chargesOf(t, upload) != 0 || h.usage(t, parties.tutor) != 0 {
		t.Fatalf("the deleted message is %+v with %d references and %d bytes charged", stored, h.referencesOf(t, upload), h.usage(t, parties.tutor))
	}
	if h.uploadState(t, upload) != "deleted" || !h.deletionQueued(t, upload) {
		t.Fatalf("the file of the deleted message is %s", h.uploadState(t, upload))
	}
	if len(listed) != 1 || !listed[0].Deleted || listed[0].Body != nil || len(listed[0].Attachments) != 0 {
		t.Fatalf("the history shows %+v", listed)
	}
	if !slices.Equal(chatFramesTo(frames, parties.tutor), []sentFrame{{kind: string(domain.FrameMessageDeleted), id: message.ID}}) {
		t.Fatalf("the tutor was sent %v", chatFramesTo(frames, parties.tutor))
	}
	if fetched := h.fetch(t, address, nil); fetched.status != http.StatusNotFound || addressAfter.status != http.StatusNotFound {
		t.Fatalf("the deleted message's file still downloads: %s, %s", describe(fetched), describe(addressAfter))
	}
}

func TestTheTutorDeletesAnyMessageInTheirChannelsAlsoWhenReadOnly(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	upload := h.pendingUpload(t, parties.studentToken, group.ID, "notes.txt", 40)
	inGroup := h.sent(t, parties.studentToken, group.ID, "In the group", upload)
	inDirect := h.sent(t, parties.studentToken, direct.ID, "In private")
	h.mustAnswer(t, h.archive(t, parties.tutorToken, parties.group.ID), http.StatusOK)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	byOtherStudent := h.deleteMessage(t, otherToken, inGroup.ID)
	groupDelete := h.deleteMessage(t, parties.tutorToken, inGroup.ID)
	directDelete := h.deleteMessage(t, parties.tutorToken, inDirect.ID)

	if byOtherStudent.status != http.StatusNotFound {
		t.Fatalf("another student deleted the message: %s", describe(byOtherStudent))
	}
	h.mustAnswer(t, groupDelete, http.StatusNoContent)
	h.mustAnswer(t, directDelete, http.StatusNoContent)
	if !h.storedMessage(t, inGroup.ID).Deleted || !h.storedMessage(t, inDirect.ID).Deleted || h.chargesOf(t, upload) != 0 || h.usage(t, parties.tutor) != 0 {
		t.Fatal("the tutor's deletes in read-only channels left the messages or their charges")
	}
	if h.countLogs(`"msg":"chat.message.deleted"`, `"by":"tutor"`) != 2 {
		t.Fatal("the tutor's deletes were not recorded as the tutor's")
	}
}

func TestAuthorsDeleteButDoNotEditInReadOnlyChannels(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	message := h.sent(t, parties.studentToken, direct.ID, "Before the ending")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	edited := h.editMessage(t, parties.studentToken, message.ID, "After the ending", 1)
	sent := h.sendMessage(t, parties.studentToken, direct.ID, "Another", nil, "")
	read, _ := h.readChannel(t, parties.studentToken, direct.ID, 1)
	deleted := h.deleteMessage(t, parties.studentToken, message.ID)

	if edited.status != http.StatusConflict || code(t, edited) != "channel_read_only" || sent.status != http.StatusConflict || code(t, sent) != "channel_read_only" {
		t.Fatalf("the read-only channel answered the edit %s and the send %s", describe(edited), describe(sent))
	}
	h.mustAnswer(t, read, http.StatusOK)
	h.mustAnswer(t, deleted, http.StatusNoContent)
	if stored := h.storedMessage(t, message.ID); !stored.Deleted || stored.Version != 1 {
		t.Fatalf("the author's message is %+v", stored)
	}
}

func TestMessageWritesAreLimitedToThirtyPerMinute(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	h.at(t, h.now(t).Truncate(time.Minute).Add(time.Minute+time.Second))
	for range 3 {
		if refused := h.sendMessage(t, parties.studentToken, randomChannelID, "Nowhere", nil, ""); refused.status != http.StatusNotFound {
			t.Fatalf("a send to a random channel answered %s", describe(refused))
		}
	}
	keyed := h.sendMessage(t, parties.studentToken, group.ID, "With a key", nil, firstReplay)
	h.mustAnswer(t, keyed, http.StatusCreated)
	for index := range domain.MessageWritesPerMinute - 2 {
		h.sent(t, parties.studentToken, direct.ID, fmt.Sprintf("Message %d", index))
	}
	h.mustAnswer(t, h.editMessage(t, parties.studentToken, messageOf(t, keyed).ID, "Edited", 1), http.StatusOK)

	limited := h.sendMessage(t, parties.studentToken, direct.ID, "One too many", nil, "")
	replayed := h.sendMessage(t, parties.studentToken, group.ID, "With a key", nil, firstReplay)
	limitedDelete := h.deleteMessage(t, parties.studentToken, messageOf(t, keyed).ID)
	tutorWrite := h.sendMessage(t, parties.tutorToken, direct.ID, "The tutor's own limit", nil, "")
	h.advance(t, time.Minute)
	afterWindow := h.sendMessage(t, parties.studentToken, direct.ID, "A new minute", nil, "")

	if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Retry-After") != "59" {
		t.Fatalf("the 31st write answered %s with Retry-After %q", describe(limited), limited.headers.Get("Retry-After"))
	}
	if limitedDelete.status != http.StatusTooManyRequests {
		t.Fatalf("a delete beyond the limit answered %s", describe(limitedDelete))
	}
	h.mustAnswer(t, replayed, http.StatusCreated)
	h.mustAnswer(t, tutorWrite, http.StatusCreated)
	h.mustAnswer(t, afterWindow, http.StatusCreated)
	if h.countLogs(`"msg":"chat.rate_limited"`, `"limit":"writes"`) != 2 {
		t.Fatal("the refused writes were not recorded as rate limited")
	}
}

func TestHistoryPagesBySeqWithoutGapsOrRepeats(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 120)

	var pages [][]int64
	query := ""
	for {
		result, page := h.history(t, parties.studentToken, direct.ID, query)
		h.mustAnswer(t, result, http.StatusOK)
		pages = append(pages, sequences(page.Items))
		if page.NextCursor == nil {
			break
		}
		query = "?cursor=" + *page.NextCursor
	}
	_, largest := h.history(t, parties.studentToken, direct.ID, "?limit=100")
	tooLarge, _ := h.history(t, parties.studentToken, direct.ID, "?limit=101")
	badCursor, _ := h.history(t, parties.studentToken, direct.ID, "?cursor=bm90LWEtc2Vx")

	if len(pages) != 3 || len(pages[0]) != domain.MessagePageDefault || len(pages[1]) != domain.MessagePageDefault || len(pages[2]) != 20 {
		t.Fatalf("the history pages hold %v", pages)
	}
	var all []int64
	for _, page := range pages {
		all = append(all, page...)
	}
	for index, seq := range all {
		if seq != int64(120-index) {
			t.Fatalf("the history has a gap or repeat at %d: %v", index, all)
		}
	}
	if len(largest.Items) != 100 || largest.Items[0].Seq != 120 || largest.NextCursor == nil {
		t.Fatalf("the largest page holds %d messages", len(largest.Items))
	}
	if tooLarge.status != http.StatusUnprocessableEntity || badCursor.status != http.StatusUnprocessableEntity || fieldList(t, badCursor) != "query.cursor:invalid" {
		t.Fatalf("a bad limit answered %s and a bad cursor %s", describe(tooLarge), describe(badCursor))
	}
}

func TestNewerMessagesAfterASeq(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 120)

	_, newer := h.history(t, parties.studentToken, direct.ID, "?after_seq=100")
	_, limited := h.history(t, parties.studentToken, direct.ID, "?after_seq=100&limit=5")
	_, none := h.history(t, parties.studentToken, direct.ID, "?after_seq=120")
	both, _ := h.history(t, parties.studentToken, direct.ID, "?after_seq=1&cursor=MTA")
	negative, _ := h.history(t, parties.studentToken, direct.ID, "?after_seq=-1")

	if !slices.Equal(sequences(newer.Items), []int64{101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120}) ||
		newer.NextCursor != nil {
		t.Fatalf("the newer messages are %v", sequences(newer.Items))
	}
	if !slices.Equal(sequences(limited.Items), []int64{101, 102, 103, 104, 105}) || len(none.Items) != 0 {
		t.Fatalf("the limited catch-up is %v and the empty one %v", sequences(limited.Items), sequences(none.Items))
	}
	if both.status != http.StatusUnprocessableEntity || negative.status != http.StatusUnprocessableEntity {
		t.Fatalf("a cursor with after_seq answered %s and a negative after_seq %s", describe(both), describe(negative))
	}
}

func TestReadMarkersMoveForwardOnlyAndReachOwnSockets(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 5)
	log := h.recordFrames(t)

	_, forward := h.readChannel(t, parties.studentToken, direct.ID, 3)
	_, backward := h.readChannel(t, parties.studentToken, direct.ID, 2)
	_, clamped := h.readChannel(t, parties.studentToken, direct.ID, 99)
	frames := log.settle(t, h)
	release := h.holdTutorProfile(t, parties.tutor)
	tutorRead, tutorState := h.readChannel(t, parties.tutorToken, direct.ID, 5)
	release()
	notMember, _ := h.readChannel(t, parties.studentToken, randomChannelID, 1)

	if forward.UnreadCount != 2 || forward.UnreadChannels != 1 || backward.UnreadCount != 2 || clamped.UnreadCount != 0 || clamped.UnreadChannels != 0 {
		t.Fatalf("the read states are %+v, %+v and %+v", forward, backward, clamped)
	}
	if member := h.channelMembers(t, direct.ID)[parties.student.String()]; member.LastRead != 5 {
		t.Fatalf("the student's marker is %d", member.LastRead)
	}
	readFrame := sentFrame{kind: string(domain.FrameReadUpdated), id: direct.ID}
	if !slices.Equal(chatFramesTo(frames, parties.student), []sentFrame{readFrame, readFrame}) || len(chatFramesTo(frames, parties.tutor)) != 0 {
		t.Fatalf("the read frames went to the student %v and the tutor %v", chatFramesTo(frames, parties.student), chatFramesTo(frames, parties.tutor))
	}
	h.mustAnswer(t, tutorRead, http.StatusOK)
	if tutorState.UnreadCount != 0 || notMember.status != http.StatusNotFound {
		t.Fatalf("the tutor's read answered %+v and a random channel %s", tutorState, describe(notMember))
	}
}

func TestUnreadCountsFollowTheSequenceCappedAt99(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 120)
	deleted := h.sent(t, parties.tutorToken, group.ID, "Soon deleted")
	h.sent(t, parties.tutorToken, group.ID, "Kept")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, deleted.ID), http.StatusNoContent)

	_, before := h.channelList(t, parties.studentToken, "")
	h.sent(t, parties.studentToken, direct.ID, "A reply")
	_, after := h.channelList(t, parties.studentToken, "")
	unread := h.send(t, http.MethodGet, channelsRoute+"/unread", "", jsonCall(parties.studentToken))

	directBefore, _ := itemOf(before, direct.ID)
	groupBefore, _ := itemOf(before, group.ID)
	if directBefore.UnreadCount != domain.UnreadCountCap || groupBefore.UnreadCount != 2 || before.UnreadChannels != 2 {
		t.Fatalf("before the reply the counts are %d and %d of %d channels", directBefore.UnreadCount, groupBefore.UnreadCount, before.UnreadChannels)
	}
	directAfter, _ := itemOf(after, direct.ID)
	if directAfter.UnreadCount != 0 || after.UnreadChannels != 1 || unreadCount(t, unread) != 1 {
		t.Fatalf("after the reply the count is %d of %d channels", directAfter.UnreadCount, after.UnreadChannels)
	}
}

func TestRacingSendsWithOneIdempotencyKeyStoreOneMessage(t *testing.T) {
	t.Parallel()
	for _, withFile := range []bool{true, false} {
		t.Run(fmt.Sprintf("with a file %t", withFile), func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			parties := h.activeStudentInAGroup(t)
			direct := h.directChannel(t, parties.card.ID)
			var attachments []string
			if withFile {
				attachments = []string{h.pendingUpload(t, parties.studentToken, direct.ID, "notes.txt", 32)}
			}
			headers := withReplayKey(jsonCall(parties.studentToken), firstReplay)
			path := channelsRoute + "/" + direct.ID + "/messages"
			release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", direct.ID)

			first := h.sendInBackground(http.MethodPost, path, messageRequest("Once", attachments), headers)
			h.waitForLockWaiters(t, 1)
			second := h.sendInBackground(http.MethodPost, path, messageRequest("Once", attachments), headers)
			h.waitForLockWaiters(t, 2)
			release()
			one, other := awaitAnswer(t, first), awaitAnswer(t, second)

			h.mustAnswer(t, one, http.StatusCreated)
			h.mustAnswer(t, other, http.StatusCreated)
			if messageOf(t, one).ID != messageOf(t, other).ID || h.messageCount(t, direct.ID) != 1 {
				t.Fatalf("the racing sends stored %d messages", h.messageCount(t, direct.ID))
			}
			if withFile && (h.chargesOf(t, attachments[0]) != 1 || h.usage(t, parties.tutor) != 32 || len(messageOf(t, other).Attachments) != 1) {
				t.Fatalf("the racing sends charged %d bytes", h.usage(t, parties.tutor))
			}
		})
	}
}

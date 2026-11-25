package api_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

const (
	bodySentinel     = "body-sentinel-words"
	editSentinel     = "edit-sentinel-words"
	fileSentinel     = "file-sentinel-name.txt"
	replaySentinel   = "replay-sentinel-value"
	chosenNameMarker = "A chosen squatter name"
)

func payloadFields(t *testing.T, frame serverFrame) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &fields); err != nil {
		t.Fatalf("the payload %s is not an object", frame.Payload)
	}
	return fields
}

func fieldNames(fields map[string]json.RawMessage) []string {
	return slices.Sorted(maps.Keys(fields))
}

func TestMessageFramesCarryIdentifiersOnly(t *testing.T) {
	t.Parallel()
	live := newLiveHarness(t, liveTuning{})
	h := live.authHarness
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	tutorSocket := h.connect(t, parties.tutorToken)
	studentSocket := h.connect(t, parties.studentToken)

	sent := h.sent(t, parties.tutorToken, direct.ID, bodySentinel)
	tutorNew, studentNew := tutorSocket.expect(t, domain.FrameMessageNew), studentSocket.expect(t, domain.FrameMessageNew)
	h.mustAnswer(t, h.editMessage(t, parties.tutorToken, sent.ID, editSentinel, 1), http.StatusOK)
	updated := studentSocket.expect(t, domain.FrameMessageUpdated)
	tutorSocket.expect(t, domain.FrameMessageUpdated)
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, sent.ID), http.StatusNoContent)
	deleted := studentSocket.expect(t, domain.FrameMessageDeleted)
	tutorSocket.expect(t, domain.FrameMessageDeleted)
	readResult, _ := h.readChannel(t, parties.studentToken, direct.ID, 1)
	read := studentSocket.expect(t, domain.FrameReadUpdated)
	following := h.sent(t, parties.tutorToken, direct.ID, "After the read")
	tutorFollowing := tutorSocket.expect(t, domain.FrameMessageNew)

	h.mustAnswer(t, readResult, http.StatusOK)
	newFields := payloadFields(t, studentNew)
	if !slices.Equal(fieldNames(newFields), []string{"channel_id", "message_id", "seq"}) || string(tutorNew.Payload) != string(studentNew.Payload) ||
		string(newFields["message_id"]) != `"`+sent.ID+`"` || string(newFields["channel_id"]) != `"`+direct.ID+`"` || string(newFields["seq"]) != "1" {
		t.Fatalf("message.new carries %s", studentNew.Payload)
	}
	for _, frame := range []serverFrame{updated, deleted} {
		if fields := payloadFields(t, frame); !slices.Equal(fieldNames(fields), []string{"channel_id", "message_id"}) || frame.Ref != nil {
			t.Fatalf("%s carries %s", frame.Type, frame.Payload)
		}
	}
	if fields := payloadFields(t, read); !slices.Equal(fieldNames(fields), []string{"channel_id"}) {
		t.Fatalf("read.updated carries %s", read.Payload)
	}
	if string(payloadFields(t, tutorFollowing)["message_id"]) != `"`+following.ID+`"` {
		t.Fatalf("the tutor's next frame after the student's read is %s", tutorFollowing.Payload)
	}
	for _, frame := range []serverFrame{tutorNew, studentNew, updated, deleted, read} {
		if strings.Contains(string(frame.Payload), bodySentinel) || strings.Contains(string(frame.Payload), editSentinel) {
			t.Fatalf("a frame carries message text: %s", frame.Payload)
		}
	}
}

func TestChatRecordsCarryNoBodiesNamesOrAddresses(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	file := h.pendingUpload(t, parties.studentToken, direct.ID, fileSentinel, 24)

	sent := h.sendMessage(t, parties.studentToken, direct.ID, bodySentinel, []string{file}, replaySentinel)
	h.mustAnswer(t, sent, http.StatusCreated)
	message := messageOf(t, sent)
	h.mustAnswer(t, h.sendMessage(t, parties.studentToken, direct.ID, bodySentinel, []string{file}, replaySentinel), http.StatusCreated)
	h.mustAnswer(t, h.editMessage(t, parties.studentToken, message.ID, editSentinel, 1), http.StatusOK)
	groupMessage := h.sent(t, parties.tutorToken, group.ID, bodySentinel)
	quoting := h.reply(t, parties.studentToken, group.ID, editSentinel, groupMessage.ID, "")
	h.mustAnswer(t, quoting, http.StatusCreated)
	h.mustAnswer(t, h.pin(t, parties.tutorToken, group.ID, groupMessage.ID), http.StatusNoContent)
	quoted := h.quoteOf(t, parties.studentToken, group.ID, quotingMessageOf(t, quoting).ID)
	h.pinnedIn(t, parties.studentToken, group.ID)
	readResult, _ := h.readChannel(t, parties.tutorToken, direct.ID, message.Seq)
	h.mustAnswer(t, readResult, http.StatusOK)
	listed := h.messagesOf(t, parties.tutorToken, direct.ID)
	h.channelList(t, parties.tutorToken, "")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, groupMessage.ID), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMessage(t, parties.studentToken, message.ID), http.StatusNoContent)
	records := h.logs.String()

	forbidden := []string{bodySentinel, editSentinel, fileSentinel, replaySentinel, cardName, groupName, endingStudentName, endingTutorAccountName,
		"/api/v1/files/", "?t="}
	if len(listed) != 1 || len(listed[0].Attachments) != 1 {
		t.Fatalf("the tutor reads %+v", listed)
	}
	if previewOfQuote(quoted) != bodySentinel {
		t.Fatalf("the quote shows %q", previewOfQuote(quoted))
	}
	forbidden = append(forbidden, strings.SplitN(listed[0].Attachments[0].URL, "?t=", 2)[1])
	for _, value := range forbidden {
		if strings.Contains(records, value) {
			t.Fatalf("a record carries %q", value)
		}
	}
	for _, event := range []string{"chat.message.persisted", "chat.message.edited", "chat.message.deleted", "chat.read_moved", "chat.message.pinned",
		"chat.message.unpinned"} {
		if h.countLogs(`"msg":"`+event+`"`, `"channel_id"`) == 0 {
			t.Fatalf("no %s record with its channel", event)
		}
	}
	if h.countLogs(`"msg":"chat.message.persisted"`, message.ID, `"attachment_count":1`) != 1 {
		t.Fatal("the send's record does not count its attachment once")
	}
	if h.countLogs(`"msg":"file.reference.added"`, file, `"ref_kind":"message"`) != 1 || h.countLogs(`"msg":"file.reference.removed"`, file, `"ref_kind":"message"`) != 1 {
		t.Fatal("the binding and the release of the file were not recorded once each")
	}
}

func TestTheReclaimCueRevealsNoSquatterIdentity(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.mustAnswer(t, h.patchMe(t, squatterToken, `{"name":"`+chosenNameMarker+`"}`), http.StatusOK)
	before := h.sent(t, squatterToken, channel.ID, "Hello from the provisional period")
	owner := h.reclaimed(t, newcomerEmail)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	log := h.recordFrames(t)

	reply := h.sent(t, tutorToken, channel.ID, "The tutor writes again")
	frames := log.settle(t, h)
	tutorHistory := h.send(t, http.MethodGet, channelsRoute+"/"+channel.ID+"/messages", "", jsonCall(tutorToken))
	ownerHistory := h.send(t, http.MethodGet, channelsRoute+"/"+channel.ID+"/messages", "", jsonCall(owner))
	tutorList := h.send(t, http.MethodGet, channelsRoute, "", jsonCall(tutorToken))
	tutorChannel := h.send(t, http.MethodGet, channelsRoute+"/"+channel.ID, "", jsonCall(tutorToken))

	for name, result := range map[string]response{"the tutor's history": tutorHistory, "the owner's history": ownerHistory, "the tutor's list": tutorList,
		"the tutor's channel": tutorChannel} {
		h.mustAnswer(t, result, http.StatusOK)
		for _, revealing := range []string{chosenNameMarker, newcomerEmail} {
			if strings.Contains(result.body, revealing) {
				t.Fatalf("%s reveals %q", name, revealing)
			}
		}
	}
	var page struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(tutorHistory.body), &page); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"attachments", "body", "created_at", "deleted", "edited_at", "erased", "id", "own", "reply_to", "sender_label", "sender_label_kind", "sent_before_reclaim",
		"seq", "version"}
	for _, item := range page.Items {
		if !slices.Equal(fieldNames(item), wantFields) {
			t.Fatalf("a message carries the fields %v", fieldNames(item))
		}
	}
	tutorView, _ := messageWithID(h.messagesOf(t, tutorToken, channel.ID), before.ID)
	if !tutorView.SentBeforeReclaim || tutorView.SenderLabel == nil || *tutorView.SenderLabel != cardName || tutorView.SenderLabelKind != nil {
		t.Fatalf("the tutor sees the account's provisional message as %+v", tutorView)
	}
	replyFrame := []sentFrame{{kind: string(domain.FrameMessageNew), id: reply.ID}}
	if len(frames) != 2 || !slices.Equal(framesTo(frames, squatter), replyFrame) {
		t.Fatalf("the reply was announced as %v", frames)
	}
	if strings.Contains(h.logs.String(), chosenNameMarker) {
		t.Fatal("a record carries the account's chosen name")
	}
}

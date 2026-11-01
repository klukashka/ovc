package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAttachmentCaptionsCanBeEditedToEmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "caption.txt", 64)
	original := h.sent(t, parties.studentToken, direct.ID, "A caption", upload)

	empty := h.editMessage(t, parties.studentToken, original.ID, "", 1)
	whitespace := h.editMessage(t, parties.studentToken, original.ID, " \t\n", 2)
	stored := h.storedMessage(t, original.ID)
	downloaded := h.fetch(t, original.Attachments[0].URL, nil)

	h.mustAnswer(t, empty, http.StatusOK)
	h.mustAnswer(t, whitespace, http.StatusOK)
	for index, response := range []response{empty, whitespace} {
		message := messageOf(t, response)
		body := []string{"", " \t\n"}[index]
		if message.Version != int32(index+2) || message.EditedAt == nil || message.Body == nil || *message.Body != body ||
			len(message.Attachments) != 1 || message.Attachments[0].ID != upload || message.Attachments[0].SizeBytes != 64 {
			t.Fatal("caption edit changed attachment ownership or lost its body")
		}
	}
	if stored.Version != 3 || stored.Body == nil || *stored.Body != " \t\n" ||
		h.referencesOf(t, upload) != 1 || h.chargesOf(t, upload) != 1 || h.usage(t, parties.tutor) != 64 {
		t.Fatal("caption edits changed reference, charge or stored message state")
	}
	if downloaded.status != http.StatusOK || downloaded.body != string(plainText(64)) {
		t.Fatalf("caption edit changed downloadable bytes: %s", describe(downloaded))
	}
}

func TestAttachmentCaptionEditsKeepBodyValidation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "caption.txt", 64)
	attached := h.sent(t, parties.studentToken, direct.ID, "A caption", upload)
	plain := h.sent(t, parties.studentToken, direct.ID, "Text only")
	cases := []struct {
		id   string
		body string
		code string
	}{
		{plain.ID, "", "required"},
		{plain.ID, " \t\n", "required"},
		{attached.ID, strings.Repeat("界", 4001), "too_long"},
		{attached.ID, "bad\x00caption", "invalid"},
		{attached.ID, "bad\rcaption", "invalid"},
	}
	responses := make([]response, 0, len(cases))

	for _, example := range cases {
		responses = append(responses, h.editMessage(t, parties.studentToken, example.id, example.body, 1))
	}

	for index, response := range responses {
		if response.status != http.StatusUnprocessableEntity || fieldList(t, response) != "body.body:"+cases[index].code {
			t.Fatalf("body rule %s answered %s", cases[index].code, describe(response))
		}
	}
	if actual := h.storedMessage(t, attached.ID); actual.Version != 1 || actual.Body == nil || *actual.Body != "A caption" {
		t.Fatal("refused caption edits changed the message")
	}
	if actual := h.storedMessage(t, plain.ID); actual.Version != 1 || actual.Body == nil || *actual.Body != "Text only" {
		t.Fatal("an empty text-only edit changed the message")
	}
	if h.referencesOf(t, upload) != 1 || h.chargesOf(t, upload) != 1 || h.usage(t, parties.tutor) != 64 {
		t.Fatal("refused edits changed attachment ownership")
	}
}

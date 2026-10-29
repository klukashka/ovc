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

const (
	messagesRoute     = "/api/v1/messages/"
	insertTextMessage = `WITH advanced AS (
    UPDATE channels SET last_seq = last_seq + 1, last_activity_at = $3::timestamptz WHERE id = $1::uuid RETURNING id, last_seq
)
INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT gen_random_uuid(), advanced.id, advanced.last_seq, $2::uuid, 'message ' || advanced.last_seq, 1, $3::timestamptz, $3::timestamptz
FROM advanced`
)

type messageFile struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	Width     *int32    `json:"width"`
	Height    *int32    `json:"height"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type chatMessage struct {
	ID                string        `json:"id"`
	Seq               int64         `json:"seq"`
	SenderLabel       *string       `json:"sender_label"`
	SenderLabelKind   *string       `json:"sender_label_kind"`
	Own               bool          `json:"own"`
	Body              *string       `json:"body"`
	CreatedAt         time.Time     `json:"created_at"`
	EditedAt          *time.Time    `json:"edited_at"`
	Deleted           bool          `json:"deleted"`
	Erased            bool          `json:"erased"`
	Version           int32         `json:"version"`
	SentBeforeReclaim bool          `json:"sent_before_reclaim"`
	Attachments       []messageFile `json:"attachments"`
}

type messagePage struct {
	Items      []chatMessage `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type lastMessage struct {
	Seq             int64   `json:"seq"`
	Own             bool    `json:"own"`
	SenderLabel     *string `json:"sender_label"`
	SenderLabelKind *string `json:"sender_label_kind"`
	Preview         *string `json:"preview"`
	AttachmentCount int64   `json:"attachment_count"`
}

type readState struct {
	UnreadCount    int64 `json:"unread_count"`
	UnreadChannels int64 `json:"unread_channels"`
}

type storedMessage struct {
	Body    *string
	Sender  *string
	Deleted bool
	Erased  bool
	Version int32
}

func messageRequest(body string, attachments []string) string {
	if attachments == nil {
		attachments = []string{}
	}
	encoded, err := json.Marshal(struct {
		Body          string   `json:"body"`
		AttachmentIDs []string `json:"attachment_ids"`
	}{Body: body, AttachmentIDs: attachments})
	if err != nil {
		panic("a message request could not be encoded")
	}
	return string(encoded)
}

func withReplayKey(headers map[string]string, replay string) map[string]string {
	if replay != "" {
		headers["Idempotency-Key"] = replay
	}
	return headers
}

func (h *authHarness) sendMessage(t *testing.T, bearerToken string, channel string, body string, attachments []string, replay string) response {
	t.Helper()
	return h.send(t, http.MethodPost, channelsRoute+"/"+channel+"/messages", messageRequest(body, attachments),
		withReplayKey(jsonCall(bearerToken), replay))
}

func (h *authHarness) sendMessageInBackground(bearerToken string, channel string, body string, attachments []string) <-chan pendingAnswer {
	return h.sendInBackground(http.MethodPost, channelsRoute+"/"+channel+"/messages", messageRequest(body, attachments), jsonCall(bearerToken))
}

func (h *authHarness) sent(t *testing.T, bearerToken string, channel string, body string, attachments ...string) chatMessage {
	t.Helper()
	result := h.sendMessage(t, bearerToken, channel, body, attachments, "")
	h.mustAnswer(t, result, http.StatusCreated)
	return messageOf(t, result)
}

func messageOf(t *testing.T, result response) chatMessage {
	t.Helper()
	var message chatMessage
	if err := json.Unmarshal([]byte(result.body), &message); err != nil {
		t.Fatalf("not a message: %d %s", result.status, result.body)
	}
	return message
}

func (h *authHarness) history(t *testing.T, bearerToken string, channel string, query string) (response, messagePage) {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+"/"+channel+"/messages"+query, "", jsonCall(bearerToken))
	var page messagePage
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &page); err != nil {
			t.Fatal(err)
		}
	}
	return result, page
}

func (h *authHarness) messagesOf(t *testing.T, bearerToken string, channel string) []chatMessage {
	t.Helper()
	result, page := h.history(t, bearerToken, channel, "?limit=100")
	h.mustAnswer(t, result, http.StatusOK)
	return page.Items
}

func editRequest(body string, version int32) string {
	return fmt.Sprintf(`{"body":%q,"version":%d}`, body, version)
}

func (h *authHarness) editMessage(t *testing.T, bearerToken string, message string, body string, version int32) response {
	t.Helper()
	return h.send(t, http.MethodPatch, messagesRoute+message, editRequest(body, version), jsonCall(bearerToken))
}

func (h *authHarness) deleteMessage(t *testing.T, bearerToken string, message string) response {
	t.Helper()
	return h.send(t, http.MethodDelete, messagesRoute+message, "", jsonCall(bearerToken))
}

func (h *authHarness) readChannel(t *testing.T, bearerToken string, channel string, seq int64) (response, readState) {
	t.Helper()
	result := h.send(t, http.MethodPost, channelsRoute+"/"+channel+"/read", fmt.Sprintf(`{"seq":%d}`, seq), jsonCall(bearerToken))
	var state readState
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &state); err != nil {
			t.Fatal(err)
		}
	}
	return result, state
}

func (h *authHarness) storedMessage(t *testing.T, id string) storedMessage {
	t.Helper()
	var message storedMessage
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT body, sender_id::text, deleted_at IS NOT NULL, erased_at IS NOT NULL, version FROM messages WHERE id = $1::uuid", id,
	).Scan(&message.Body, &message.Sender, &message.Deleted, &message.Erased, &message.Version)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func (h *authHarness) seedMessages(t *testing.T, channel string, sender domain.UserID, count int) {
	t.Helper()
	for range count {
		if _, err := h.database.Pool.Exec(context.Background(), insertTextMessage, channel, sender.String(), authStart); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *authHarness) usage(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	var used int64
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT file_usage_bytes FROM tutor_profiles WHERE user_id = $1", tutor.String()).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used
}

func (h *authHarness) chargesOf(t *testing.T, attachment string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(*) FROM quota_charges WHERE attachment_id = $1::uuid", attachment)
}

func (h *authHarness) referencesOf(t *testing.T, attachment string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(*) FROM attachment_references WHERE attachment_id = $1::uuid", attachment)
}

func (h *authHarness) messageCount(t *testing.T, channel string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM messages WHERE channel_id = $1::uuid", channel)
}

func (h *authHarness) digestPending(t *testing.T, user domain.UserID) bool {
	t.Helper()
	return h.countRows(t, "SELECT count(user_id) FROM digest_pending WHERE user_id = $1::uuid", user.String()) == 1
}

func (h *authHarness) pendingUpload(t *testing.T, bearerToken string, channel string, filename string, size int) string {
	t.Helper()
	uploaded := h.chatUpload(t, bearerToken, channel, filename, plainText(size))
	h.mustAnswer(t, uploaded, http.StatusCreated)
	return uploadedID(t, uploaded)
}

func sequences(messages []chatMessage) []int64 {
	seqs := make([]int64, 0, len(messages))
	for _, message := range messages {
		seqs = append(seqs, message.Seq)
	}
	return seqs
}

func messageWithID(messages []chatMessage, id string) (chatMessage, bool) {
	for _, message := range messages {
		if message.ID == id {
			return message, true
		}
	}
	return chatMessage{}, false
}

func lastMessageOf(t *testing.T, item channelItem) lastMessage {
	t.Helper()
	var last lastMessage
	if err := json.Unmarshal(item.LastMessage, &last); err != nil || strings.TrimSpace(string(item.LastMessage)) == "null" {
		t.Fatalf("the channel has no last message: %s", item.LastMessage)
	}
	return last
}

func chatFramesTo(frames []sentFrame, user domain.UserID) []sentFrame {
	var chat []sentFrame
	kinds := []domain.FrameType{domain.FrameMessageNew, domain.FrameMessageUpdated, domain.FrameMessageDeleted, domain.FrameReadUpdated}
	for _, frame := range framesTo(frames, user) {
		if slices.Contains(kinds, domain.FrameType(frame.kind)) {
			chat = append(chat, frame)
		}
	}
	return chat
}

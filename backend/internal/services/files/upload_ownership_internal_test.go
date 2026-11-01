package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
)

func (h uploadProtocolFixture) tutor(t *testing.T) (accounts.Principal, domain.ChannelID) {
	t.Helper()
	user, channel := domain.UserID{20}, domain.ChannelID{20}
	_, err := h.database.Pool.Exec(context.Background(), `WITH account AS (
 INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at, unverified_since, created_at, updated_at)
 VALUES ($1, 'tutor', 'active', 'tutor@example.org', $3, '$argon2id$fixture', 'Tutor', 'en', 'UTC', 'system', '1', $3, $3, $3, $3) RETURNING id
), profile AS (
 INSERT INTO tutor_profiles (user_id, file_quota_bytes, recording_quota_bytes, code, created_at, updated_at)
 SELECT id, 1000000000, 1000000000, 'ABCDEFGH', $3, $3 FROM account RETURNING user_id
), audience AS (
 INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at) SELECT $2, user_id, 'Group', 'clay', $3, $3 FROM profile RETURNING id, tutor_id
), channel AS (
 INSERT INTO channels (id, kind, tutor_id, group_id, last_activity_at, created_at, updated_at) SELECT id, 'group', tutor_id, id, $3, $3, $3 FROM audience RETURNING id, tutor_id
)
INSERT INTO channel_members (channel_id, user_id, created_at, updated_at) SELECT id, tutor_id, $3, $3 FROM channel`, user, channel, h.now)
	if err != nil {
		t.Fatal(err)
	}
	return accounts.Principal{UserID: user, Role: domain.RoleTutor, State: domain.AccountActive}, channel
}

func TestUploadReconciliationPreservesTransferredOwnership(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	principal, channel := h.tutor(t)
	content := []byte("A real uploaded message attachment.")
	uploaded, err := h.service.UploadChat(context.Background(), ChatUpload{Principal: principal, Channel: channel, Filename: "notes.txt", Declared: int64(len(content)), Body: bytes.NewReader(content), Finish: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	row, err := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	messaging := chat.New(chat.Dependencies{Store: h.service.deps.Store, Clock: h.service.deps.Clock, Random: core.CryptoRandom(), Addresses: h.service, Logger: h.service.deps.Logger})
	message, err := messaging.Send(context.Background(), chat.Author{UserID: principal.UserID, Role: principal.Role}, channel, chat.Draft{Attachments: []domain.AttachmentID{uploaded.ID}})
	if err != nil {
		t.Fatal(err)
	}
	opened := openUpload{id: uploaded.ID, key: row.StorageKey}
	generation := domain.UploadInitialCleanup
	payload, err := json.Marshal(domain.AttachmentPayload{Attachment: uploaded.ID.String(), Key: row.StorageKey, Generation: &generation})
	if err != nil {
		t.Fatal(err)
	}

	reason, staleErr := h.service.DeleteObject(context.Background(), repositories.ClaimedOutboxRow{Payload: payload}, h.now)
	reconciled := h.service.discardChatUpload(context.Background(), opened, true)
	after, lookupErr := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	object, openErr := h.objects.Open(context.Background(), row.StorageKey)
	if openErr != nil {
		t.Fatal(openErr)
	}
	data, readErr := io.ReadAll(object)
	if err := errors.Join(readErr, object.Close()); err != nil {
		t.Fatal(err)
	}
	var references int
	var usage int64
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM attachment_references WHERE attachment_id = $1 AND ref_id = $2", uploaded.ID, message.ID).Scan(&references); err != nil {
		t.Fatal(err)
	}
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT file_usage_bytes FROM tutor_profiles WHERE user_id = $1", principal.UserID).Scan(&usage); err != nil {
		t.Fatal(err)
	}

	if staleErr != nil || reason != domain.DoneObsolete || reconciled != nil || lookupErr != nil || after.State != domain.AttachmentBound || references != 1 || usage != int64(len(content)) || !bytes.Equal(data, content) || h.capacity(t) != 0 {
		t.Fatalf("reconciliation destroyed transferred ownership: %v %v state %s refs %d usage %d", reconciled, lookupErr, after.State, references, usage)
	}
}

func TestUploadCompleteAuthorizationWaitCannotGrantPermission(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	h.service.deps.Uploads.Settlement = 100 * time.Millisecond
	principal, channel := h.tutor(t)
	admission, err := h.service.BeginChatUpload(context.Background(), principal, channel, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Close(context.Background())
	upload, err := h.objects.CreateMultipartUpload(context.Background(), admission.opened.key, objectContentType)
	if err != nil {
		t.Fatal(err)
	}
	admission.opened.upload, admission.createAttempted = upload, true
	if allowed, err := h.service.recordUploadID(context.Background(), admission.opened, h.now); err != nil || !allowed {
		t.Fatalf("record: %t %v", allowed, err)
	}
	release := h.blockSettlement(t, "intent", admission.opened.id)
	ctx, cancel := h.service.settlementContext(context.Background(), admission.opened.id)
	defer cancel()

	refused := h.service.authorizeComplete(ctx, admission.opened)
	release()
	intent := h.read(t, admission.opened.id)
	_, objectErr := h.objects.Open(context.Background(), admission.opened.key)

	if refused == nil || intent.CompletionStarted || intent.Phase != domain.UploadStreaming || !errors.Is(objectErr, storage.ErrObjectNotFound) {
		t.Fatalf("a timed-out transition authorized Complete: %v phase %s started %t object %v", refused, intent.Phase, intent.CompletionStarted, objectErr)
	}
}

func TestUploadReconciliationDeletesUnpublishedObject(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	principal, channel := h.tutor(t)
	opened, err := h.service.openChatUpload(context.Background(), ChatUpload{Principal: principal, Channel: channel, Declared: 10}, "unpublished.txt")
	if err != nil {
		t.Fatal(err)
	}
	upload, err := h.objects.CreateMultipartUpload(context.Background(), opened.key, objectContentType)
	if err != nil {
		t.Fatal(err)
	}
	opened.upload = upload
	if allowed, err := h.service.recordUploadID(context.Background(), opened, h.now); err != nil || !allowed {
		t.Fatalf("record: %t %v", allowed, err)
	}
	part, err := h.objects.UploadPart(context.Background(), opened.key, upload, 1, []byte("unpublished"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.authorizeComplete(context.Background(), opened); err != nil {
		t.Fatal(err)
	}
	if err := h.objects.CompleteMultipartUpload(context.Background(), opened.key, upload, []storage.CompletedPart{part}); err != nil {
		t.Fatal(err)
	}

	reconciled := h.service.discardChatUpload(context.Background(), opened, true)
	h.tick(t, h.now)
	_, objectErr := h.objects.Open(context.Background(), opened.key)
	_, attachmentErr := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), opened.id)

	if reconciled != nil || !errors.Is(objectErr, storage.ErrObjectNotFound) || !errors.Is(attachmentErr, repositories.ErrNotFound) || h.capacity(t) != 0 {
		t.Fatalf("unpublished upload escaped cleanup: %v object %v attachment %v", reconciled, objectErr, attachmentErr)
	}
}

func TestUploadStaleIntentCleanupPreservesPendingOwnership(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	principal, channel := h.tutor(t)
	uploaded, err := h.service.UploadChat(context.Background(), ChatUpload{Principal: principal, Channel: channel, Filename: "pending.txt", Declared: 5, Body: bytes.NewBufferString("hello"), Finish: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	row, err := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	generation := domain.UploadInitialCleanup
	payload, err := json.Marshal(domain.AttachmentPayload{Attachment: uploaded.ID.String(), Key: row.StorageKey, Generation: &generation})
	if err != nil {
		t.Fatal(err)
	}

	reconciled := h.service.discardChatUpload(context.Background(), openUpload{id: uploaded.ID, key: row.StorageKey}, true)
	reason, staleErr := h.service.DeleteObject(context.Background(), repositories.ClaimedOutboxRow{Payload: payload}, h.now)
	after, lookupErr := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	object, openErr := h.objects.Open(context.Background(), row.StorageKey)
	if openErr != nil {
		t.Fatal(openErr)
	}
	body, readErr := io.ReadAll(object)
	if err := errors.Join(readErr, object.Close()); err != nil {
		t.Fatal(err)
	}

	if reconciled != nil || staleErr != nil || reason != domain.DoneObsolete || lookupErr != nil || after.State != domain.AttachmentPending || string(body) != "hello" || h.capacity(t) != 0 {
		t.Fatalf("pending ownership lost: reconcile %v stale %v reason %s lookup %v state %s", reconciled, staleErr, reason, lookupErr, after.State)
	}
}

package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/testdb"
)

func TestStudentUploadsFailClosedWithoutAccountingMigration(t *testing.T) {
	t.Parallel()
	h := uploadProtocolFixtureWithDatabase(t, testdb.NewMigratedTo(t, 20))
	var logs bytes.Buffer
	deps := h.service.deps
	deps.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	h.service = New(deps)
	student, _, channel := h.student(t)

	admission, err := h.service.BeginChatUpload(context.Background(), student, channel, 5)
	if admission != nil {
		admission.Close(context.Background())
	}
	var attachments int
	readErr := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM attachments").Scan(&attachments)
	var unavailable *AccountingUnavailableError

	assertUnavailableAccountingLogs(t, logs.String())
	if !errors.As(err, &unavailable) || admission != nil || readErr != nil || attachments != 0 || h.capacity(t) != 0 {
		t.Fatalf("missing activation allowed streaming: %v admission %t read %v rows %d", err, admission != nil, readErr, attachments)
	}
}

func (h uploadProtocolFixture) student(t *testing.T) (accounts.Principal, domain.UserID, domain.ChannelID) {
	t.Helper()
	tutor, channel := h.tutor(t)
	student, session := domain.UserID{21}, domain.SessionID{21}
	_, err := h.database.Pool.Exec(context.Background(), `WITH account AS (
 INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at, unverified_since, created_at, updated_at)
 VALUES ($1, 'student', 'active', 'student@example.org', $3, '$argon2id$fixture', 'Student', 'en', 'UTC', 'system', '1', $3, $3, $3, $3) RETURNING id
)
INSERT INTO channel_members (channel_id, user_id, created_at, updated_at) SELECT $2, id, $3, $3 FROM account`, student, channel, h.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.deps.Store.Repositories().Sessions.Create(context.Background(), repositories.NewSession{ID: session, UserID: student, RefreshHash: make([]byte, 32), ExpiresAt: h.now.Add(48 * time.Hour), CreatedAt: h.now}); err != nil {
		t.Fatal(err)
	}
	return accounts.Principal{UserID: student, SessionID: session, Role: domain.RoleStudent, State: domain.AccountActive}, tutor.UserID, channel
}

func (h uploadProtocolFixture) dailyTotal(t *testing.T, student domain.UserID, tutor domain.UserID, day time.Time) int64 {
	t.Helper()
	total, err := h.service.deps.Store.Repositories().DailyUploads.Bytes(context.Background(), student, tutor, utcDay(day))
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func TestDuplicateStudentSettlementPreservesOwnershipAndChargesOnce(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	student, tutor, channel := h.student(t)
	uploaded, err := h.service.UploadChat(context.Background(), ChatUpload{Principal: student, Channel: channel, Filename: "once.txt", Declared: 5, Body: bytes.NewBufferString("hello"), Finish: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	row, err := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	opened := openUpload{id: row.ID, key: row.StorageKey, uploader: student, tutor: tutor, channel: channel, created: row.CreatedAt}

	repeated := h.service.settleChatUpload(context.Background(), opened, storedContent{kind: kindPlainText, size: 5})
	reconciled := h.service.discardChatUpload(context.Background(), opened, true)
	after, lookupErr := h.service.deps.Store.Repositories().Attachments.ByID(context.Background(), uploaded.ID)
	total := h.dailyTotal(t, student.UserID, tutor, h.now)

	if repeated == nil || reconciled != nil || lookupErr != nil || after.State != domain.AttachmentPending || total != 5 || h.capacity(t) != 0 {
		t.Fatalf("duplicate settlement changed ownership/accounting: repeated %v reconcile %v lookup %v state %s total %d", repeated, reconciled, lookupErr, after.State, total)
	}
}

func TestPreCutoverStudentIntentsCannotCompleteOrPublishAfterActivation(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	student, tutor, channel := h.student(t)
	activation, err := h.service.deps.Store.Repositories().DailyUploads.Activation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	opened := h.intent(t, domain.AttachmentID{0x71}, false)
	opened.uploader, opened.tutor, opened.channel, opened.created = student, tutor, channel, activation.Add(-time.Minute)
	scope, scopeID := domain.ScopeChannel, domain.EntityID(channel)
	if err := h.service.deps.Store.Repositories().Attachments.CreateUploading(context.Background(), repositories.NewUploading{ID: opened.id, UploaderID: &student.UserID, Context: domain.ContextChat, ScopeKind: &scope, ScopeID: &scopeID, QuotaTutorID: &tutor, StorageKey: opened.key, Filename: "old.txt", ReservedBytes: 5, CreatedAt: opened.created}); err != nil {
		t.Fatal(err)
	}
	if allowed, err := h.service.recordUploadID(context.Background(), opened, h.now); err != nil || !allowed {
		t.Fatalf("legacy multipart fixture: %t %v", allowed, err)
	}

	if err := h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if _, err := repos.UploadIntents.Lock(context.Background(), opened.id); err != nil {
			return err
		}
		return repos.UploadIntents.AuthorizeComplete(context.Background(), opened.id, opened.key, string(opened.upload), opened.created)
	}); err != nil {
		t.Fatal(err)
	}
	part, err := h.objects.UploadPart(context.Background(), opened.key, opened.upload, 1, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.objects.CompleteMultipartUpload(context.Background(), opened.key, opened.upload, []storage.CompletedPart{part}); err != nil {
		t.Fatal(err)
	}

	completeErr := h.service.authorizeComplete(context.Background(), opened)
	started := h.read(t, opened.id).CompletionStarted
	settleErr := h.service.settleChatUpload(context.Background(), opened, storedContent{kind: kindPlainText, size: 5})
	h.tick(t, h.now)
	total := h.dailyTotal(t, student.UserID, tutor, opened.created)
	_, objectErr := h.objects.Open(context.Background(), opened.key)
	var completeUnavailable, settleUnavailable *AccountingUnavailableError

	if !errors.As(completeErr, &completeUnavailable) || !errors.As(settleErr, &settleUnavailable) || !started || total != 0 || !errors.Is(objectErr, storage.ErrObjectNotFound) {
		t.Fatalf("old intent escaped cutover: complete %v settle %v started %t bytes %d object %v", completeErr, settleErr, started, total, objectErr)
	}
}

func TestDailyAccountingRollbackDoesNotChargeACompletion(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	student, tutor, channel := h.student(t)
	opened, err := h.service.openChatUpload(context.Background(), ChatUpload{Principal: student, Channel: channel, Declared: 5}, "rollback.txt")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	err = h.service.settleChatUpload(ctx, opened, storedContent{kind: kindPlainText, size: 5})
	row, rowErr := h.service.deps.Store.Repositories().Attachments.ByID(ctx, opened.id)
	total := h.dailyTotal(t, student.UserID, tutor, opened.created)

	if !errors.Is(err, ErrOutOfScope) || rowErr != nil || row.State != domain.AttachmentUploading || total != 0 {
		t.Fatalf("rollback retained completion/accounting: %v row %v state %s bytes %d", err, rowErr, row.State, total)
	}
}

func TestDailyUploadCleanupCancellationAndOwnDeadlineBoundPoolAcquisition(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	release := h.database.HoldAllConnectionsBut(t, 0)
	defer release()
	wallClock := clock.System{}

	shortContext, cancelShort := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelShort()
	shortStarted := wallClock.Now()
	shortDeleted, shortErr := h.service.CleanupDailyUploads(shortContext, h.now)
	shortElapsed := wallClock.Now().Sub(shortStarted)
	longContext, cancelLong := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancelLong()
	longStarted := wallClock.Now()
	longDeleted, longErr := h.service.CleanupDailyUploads(longContext, h.now)
	longElapsed := wallClock.Now().Sub(longStarted)

	if shortDeleted != 0 || !errors.Is(shortErr, context.DeadlineExceeded) {
		t.Errorf("cleanup outlived its pool wait context: deleted %d error %v elapsed %s", shortDeleted, shortErr, shortElapsed)
	}
	if longDeleted != 0 || !errors.Is(longErr, context.DeadlineExceeded) || longElapsed > 21*time.Second {
		t.Errorf("cleanup exceeded its own 20-second deadline: deleted %d error %v elapsed %s", longDeleted, longErr, longElapsed)
	}
}

func assertUnavailableAccountingLogs(t *testing.T, records string) {
	t.Helper()
	var event struct {
		Time    time.Time `json:"time"`
		Level   string    `json:"level"`
		Message string    `json:"msg"`
		Kind    string    `json:"kind"`
	}
	decoder := json.NewDecoder(strings.NewReader(records))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		t.Fatalf("unavailable accounting diagnostic contains unexpected fields: %v", err)
	}
	if event.Level != "ERROR" || event.Message != "upload.accounting.unavailable" || event.Kind != "sqlstate_42P01" || event.Time.IsZero() {
		t.Errorf("unexpected unavailable accounting event %+v", event)
	}
	if err := decoder.Decode(&event); !errors.Is(err, io.EOF) {
		t.Errorf("unavailable accounting emitted another outcome: %v", err)
	}
	for _, forbidden := range []string{"upload.accepted", "chat_upload_accounting_activation", "student@example.org", "$argon2id$fixture", "storage_key", "filename", "relation", "does not exist"} {
		if strings.Contains(records, forbidden) {
			t.Errorf("unavailable accounting diagnostic exposed forbidden content %q", forbidden)
		}
	}
}

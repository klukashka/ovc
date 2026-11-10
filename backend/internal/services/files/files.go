package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	pendingLifetime       = 24 * time.Hour
	pendingAvatarsPerUser = 3
	uploadWindow          = time.Hour
	uploadsPerWindow      = 20
	concurrentPerUser     = 4
	concurrentPerInstance = 32
	decodeBudgetBytes     = 256 << 20
	imageBytesPerPixel    = 8
	jpegBytesPerPixel     = 16
	webpTreeAllowance     = 132 << 20
	webpBytesPerPixel     = 5
	jpegQuality           = 90
	storageKeyBytes       = 32
	expireBatch           = 200
	expireRounds          = 20

	purposeAvatarUploads domain.RateLimitPurpose = "avatar_upload_hour"
)

var (
	ErrUploadCapacity     = repositories.ErrUploadCapacity
	ErrTooLarge           = errors.New("the upload is larger than its limit")
	ErrUnsupported        = errors.New("the upload is not an allowed kind of file")
	ErrBusy               = errors.New("too many uploads are in progress")
	ErrDecodeBusy         = errors.New("the instance is decoding as many images as its memory budget allows")
	ErrAvatarReadsBusy    = errors.New("the instance is serving as many avatars as it allows at once")
	ErrPurgedDuringUpload = errors.New("the account was purged while its upload was stored")
	ErrNotFound           = errors.New("the file does not exist")

	errMalformedPayload = errors.New("the storage deletion payload is malformed")
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "too many uploads"
}

type Upload struct {
	ID        domain.AttachmentID
	Filename  string
	MediaType string
	SizeBytes int64
}

const avatarReadSlots = 32

type Avatar struct {
	MediaType string
	SizeBytes int64
	Body      ObjectBody
}

type objectBody struct {
	io.Reader
	object  io.ReadCloser
	release func()
}

func (b *objectBody) Close() error {
	b.release()
	return b.object.Close()
}

func (b *objectBody) Overruns() (bool, error) {
	extra, err := io.ReadFull(b.object, make([]byte, 1))
	if extra > 0 {
		return true, nil
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	return false, err
}

type ObjectBody interface {
	io.ReadCloser
	Overruns() (bool, error)
}

type Dependencies struct {
	Downloads DownloadTiming
	Uploads   UploadTiming
	Lifetime  context.Context
	Store     *repositories.Store
	Objects   storage.Store
	Clock     clock.Source
	Random    core.RandomSource
	Keys      core.Keys
	Logger    *slog.Logger
}

type Service struct {
	deps           Dependencies
	gate           *gate
	decodes        *decodeBudget
	avatarReads    chan struct{}
	downloads      chan struct{}
	hourly         *hourlyUploads
	libraryHourly  *hourlyUploads
	tokens         downloadTokens
	references     []referenceAuthorization
	recoveryMutex  sync.Mutex
	recoveryCursor domain.AttachmentID
}

func New(dependencies Dependencies) *Service {
	dependencies.Downloads = dependencies.Downloads.withDefaults()
	dependencies.Uploads = dependencies.Uploads.withDefaults()
	if dependencies.Lifetime == nil {
		dependencies.Lifetime = context.Background()
	}
	return &Service{
		deps: dependencies, gate: newGate(), decodes: newDecodeBudget(decodeBudgetBytes),
		avatarReads: make(chan struct{}, avatarReadSlots), downloads: make(chan struct{}, downloadSlots),
		hourly: newHourlyUploads(domain.ChatUploadsPerHour), libraryHourly: newHourlyUploads(domain.LibraryUploadsPerHour), tokens: downloadTokens{keys: dependencies.Keys},
		references: []referenceAuthorization{materialAuthorization{}, templateAuthorization{}},
	}
}

type imageKind struct {
	format     string
	extensions []string
}

var avatarKinds = []imageKind{
	{format: "jpeg", extensions: []string{".jpg", ".jpeg"}},
	{format: "png", extensions: []string{".png"}},
	{format: "webp", extensions: []string{".webp"}},
}

func (s *Service) UploadAvatar(ctx context.Context, principal accounts.Principal, filename string, read func() ([]byte, error)) (Upload, error) {
	uploader := principal.UserID
	release, err := s.gate.enter(uploader)
	if err != nil {
		s.rejected(ctx, uploader, domain.ContextAvatar, "busy")
		return Upload{}, err
	}
	defer release()
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Upload{}, err
	}
	if err := s.admit(ctx, uploader, now); err != nil {
		return Upload{}, err
	}
	data, err := read()
	if err != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.aborted", userAttribute(uploader), slog.String("kind", core.ErrorKind(err)))
		return Upload{}, err
	}
	if len(data) > domain.AvatarMaxBytes {
		s.rejected(ctx, uploader, domain.ContextAvatar, "too_large")
		return Upload{}, ErrTooLarge
	}
	encoded, mediaType, storedName, err := s.reencode(ctx, uploader, data, filename)
	if reason, rejected := rejectionReason(err); rejected {
		s.rejected(ctx, uploader, domain.ContextAvatar, reason)
		return Upload{}, err
	}
	if err != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.aborted", userAttribute(uploader), slog.String("kind", core.ErrorKind(err)))
		return Upload{}, err
	}
	upload, err := s.store(ctx, principal, encoded, mediaType, storedName, now)
	if errors.Is(err, ErrBusy) {
		s.rejected(ctx, uploader, domain.ContextAvatar, "pending")
		return Upload{}, err
	}
	if err != nil {
		return Upload{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.accepted", userAttribute(uploader),
		slog.String("context", string(domain.ContextAvatar)), slog.String("media_type", mediaType), slog.Int64("size_bytes", upload.SizeBytes))
	return upload, nil
}

func (s *Service) admit(ctx context.Context, uploader domain.UserID, now time.Time) error {
	repos := s.deps.Store.Repositories()
	pending, err := repos.Attachments.CountPending(ctx, uploader, domain.ContextAvatar, now)
	if err != nil {
		return err
	}
	if pending >= pendingAvatarsPerUser {
		s.rejected(ctx, uploader, domain.ContextAvatar, "pending")
		return ErrBusy
	}
	window, retryAfter := domain.FixedWindow(now, uploadWindow)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: purposeAvatarUploads, Subject: s.deps.Keys.ThrottleSubject(string(purposeAvatarUploads), uploader.String()), WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > uploadsPerWindow {
		s.rejected(ctx, uploader, domain.ContextAvatar, "hourly")
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (s *Service) reencode(ctx context.Context, uploader domain.UserID, data []byte, filename string) ([]byte, string, string, error) {
	kind, found := detect(data)
	if !found || !extensionMatches(kind, filename) {
		return nil, "", "", ErrUnsupported
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != kind.format || config.Width < 1 || config.Height < 1 || config.Width > domain.AvatarMaxSide || config.Height > domain.AvatarMaxSide {
		return nil, "", "", ErrUnsupported
	}
	cost, decodable := decodeCost(config, format)
	if !decodable {
		return nil, "", "", ErrUnsupported
	}
	release, admitted := s.decodes.take(uploader, cost)
	if !admitted {
		return nil, "", "", ErrDecodeBusy
	}
	defer release()
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", "", ErrUnsupported
	}
	encoded := &cappedBuffer{limit: domain.AvatarMaxStoredBytes}
	mediaType, storedName := "image/png", "avatar.png"
	if kind.format == "jpeg" {
		mediaType, storedName = "image/jpeg", "avatar.jpg"
		err = jpeg.Encode(encoded, decoded, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(encoded, decoded)
	}
	if errors.Is(err, ErrTooLarge) {
		return nil, "", "", ErrTooLarge
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("encode the avatar: %w", err)
	}
	return encoded.Bytes(), mediaType, storedName, nil
}

func decodeCost(config image.Config, format string) (int64, bool) {
	switch config.ColorModel {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model, color.CMYKModel, color.Alpha16Model:
		return 0, false
	}
	pixels := int64(config.Width) * int64(config.Height)
	switch format {
	case "jpeg":
		return pixels * jpegBytesPerPixel, true
	case "webp":
		return max(pixels*imageBytesPerPixel, pixels*webpBytesPerPixel+webpTreeAllowance), true
	}
	return pixels * imageBytesPerPixel, true
}

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(data []byte) (int, error) {
	if c.Len()+len(data) > c.limit {
		return 0, ErrTooLarge
	}
	written, err := c.Buffer.Write(data)
	if err != nil {
		return written, fmt.Errorf("buffer the encoded avatar: %w", err)
	}
	return written, nil
}

func detect(data []byte) (imageKind, bool) {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return avatarKinds[0], true
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return avatarKinds[1], true
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return avatarKinds[2], true
	}
	return imageKind{}, false
}

func extensionMatches(kind imageKind, filename string) bool {
	extension := strings.ToLower(path.Ext(filename))
	for _, allowed := range kind.extensions {
		if extension == allowed {
			return true
		}
	}
	return false
}

func rejectionReason(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrTooLarge):
		return "too_large", true
	case errors.Is(err, ErrUnsupported):
		return "unsupported", true
	case errors.Is(err, ErrDecodeBusy):
		return "decode_busy", true
	}
	return "", false
}

func (s *Service) store(ctx context.Context, principal accounts.Principal, encoded []byte, mediaType string, storedName string, now time.Time) (Upload, error) {
	uploader := principal.UserID
	id, err := core.NewID[domain.AttachmentID](s.deps.Random)
	if err != nil {
		return Upload{}, err
	}
	_, keyText, err := core.RandomToken(s.deps.Random, storageKeyBytes)
	if err != nil {
		return Upload{}, err
	}
	key := domain.AvatarKeyPrefix + keyText
	sum := sha256.Sum256(encoded)
	expires := now.Add(pendingLifetime)
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		pending, err := repos.Attachments.CountPending(ctx, uploader, domain.ContextAvatar, now)
		if err != nil {
			return err
		}
		if pending >= pendingAvatarsPerUser {
			return ErrBusy
		}
		return repos.Attachments.Create(ctx, repositories.NewAttachment{
			ID: id, UploaderID: uploader, Context: domain.ContextAvatar, StorageKey: key, Filename: storedName, MediaType: mediaType,
			SizeBytes: int64(len(encoded)), Sha256: sum[:], ExpiresAt: &expires, CreatedAt: now,
		})
	})
	if err != nil {
		return Upload{}, err
	}
	if err := s.deps.Objects.Put(ctx, key, encoded, mediaType); err != nil {
		return Upload{}, errors.Join(err, s.settle(context.WithoutCancel(ctx), principal, id, key, false, now))
	}
	if err := s.settle(context.WithoutCancel(ctx), principal, id, key, true, now); err != nil {
		return Upload{}, err
	}
	return Upload{ID: id, Filename: storedName, MediaType: mediaType, SizeBytes: int64(len(encoded))}, nil
}

func (s *Service) settle(ctx context.Context, principal accounts.Principal, id domain.AttachmentID, key string, stored bool, now time.Time) error {
	purged := false
	var refused error
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		refused = accounts.LockActingAccount(ctx, repos, principal, now, principal.State)
		if refused != nil && !errors.Is(refused, accounts.ErrUnauthenticated) {
			return refused
		}
		_, err := repos.Attachments.Lock(ctx, id)
		if err == nil && (!stored || refused != nil) {
			return s.discard(ctx, repos, id, now)
		}
		if errors.Is(err, repositories.ErrNotFound) {
			purged = true
			return s.queueDeletionAs(ctx, repos, objectDeletion{id: id, key: key}, string(domain.KindStorageDelete)+":"+id.String()+":after-purge", now)
		}
		return err
	})
	switch {
	case err != nil:
		return err
	case purged:
		return ErrPurgedDuringUpload
	case refused != nil && stored:
		s.rejected(ctx, principal.UserID, domain.ContextAvatar, "session_revoked")
		return refused
	}
	return nil
}

func (s *Service) BindAvatar(ctx context.Context, repos repositories.Repositories, user domain.UserID, target *domain.AttachmentID, now time.Time) error {
	current, err := repos.Attachments.AvatarOf(ctx, user)
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return err
	}
	if target != nil && hasCurrent && current == *target {
		return nil
	}
	if target != nil {
		attachment, err := repos.Attachments.Lock(ctx, *target)
		if errors.Is(err, repositories.ErrNotFound) {
			return domain.ErrInvalidAvatar
		}
		if err != nil {
			return err
		}
		expired := attachment.ExpiresAt == nil || !attachment.ExpiresAt.After(now)
		if attachment.UploaderID != user || attachment.Context != domain.ContextAvatar || attachment.State != domain.AttachmentPending || expired {
			return domain.ErrInvalidAvatar
		}
	}
	removed, err := repos.Attachments.RemoveAvatarOf(ctx, user)
	if err != nil {
		return err
	}
	for _, old := range removed {
		if err := s.discard(ctx, repos, old, now); err != nil {
			return err
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.removed", attachmentAttribute(old), slog.String("ref_kind", string(domain.ReferenceAvatar)))
	}
	if target == nil {
		return nil
	}
	if _, err := repos.Attachments.Bind(ctx, *target, now); err != nil {
		return err
	}
	err = repos.Attachments.Reference(ctx, repositories.NewAttachmentRef{
		AttachmentID: *target, RefKind: domain.ReferenceAvatar, RefID: domain.EntityID(user), CreatedAt: now,
	})
	if err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", attachmentAttribute(*target), slog.String("ref_kind", string(domain.ReferenceAvatar)))
	return nil
}

func (s *Service) Avatar(ctx context.Context, id domain.AttachmentID) (Avatar, error) {
	servable, err := s.deps.Store.Repositories().Attachments.ServableAvatar(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Avatar{}, ErrNotFound
	}
	if err != nil {
		return Avatar{}, err
	}
	select {
	case s.avatarReads <- struct{}{}:
	default:
		return Avatar{}, ErrAvatarReadsBusy
	}
	release := sync.OnceFunc(func() { <-s.avatarReads })
	body, err := s.deps.Objects.Open(ctx, servable.StorageKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		release()
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "avatars.object_missing", attachmentAttribute(id))
		return Avatar{}, ErrNotFound
	}
	if err != nil {
		release()
		return Avatar{}, err
	}
	return Avatar{
		MediaType: servable.MediaType,
		SizeBytes: servable.SizeBytes,
		Body:      &objectBody{Reader: io.LimitReader(body, servable.SizeBytes), object: body, release: release},
	}, nil
}

func (s *Service) discard(ctx context.Context, repos repositories.Repositories, id domain.AttachmentID, now time.Time) error {
	attachment, err := repos.Attachments.ByID(ctx, id)
	if err != nil {
		return err
	}
	marked, err := repos.Attachments.MarkDeleted(ctx, id, now)
	if err != nil || !marked {
		return err
	}
	return s.queueDeletion(ctx, repos, objectDeletion{id: id, key: attachment.StorageKey, upload: attachment.MultipartUploadID}, now)
}

type objectDeletion struct {
	id     domain.AttachmentID
	key    string
	upload *string
}

func (d objectDeletion) payload() domain.AttachmentPayload {
	payload := domain.AttachmentPayload{Attachment: d.id.String(), Key: d.key}
	if d.upload != nil {
		payload.Upload = *d.upload
	}
	return payload
}

func (s *Service) queueDeletion(ctx context.Context, repos repositories.Repositories, deletion objectDeletion, now time.Time) error {
	return s.queueDeletionWithKey(ctx, repos, deletion, string(domain.KindStorageDelete)+":"+deletion.id.String(), now)
}

func (s *Service) queueDeletionWithKey(ctx context.Context, repos repositories.Repositories, deletion objectDeletion, idempotencyKey string, now time.Time) error {
	intent, err := repos.UploadIntents.Lock(ctx, deletion.id)
	if err == nil {
		if intent.StorageKey != deletion.key {
			return repositories.ErrUploadTransition
		}
		if err := repos.UploadIntents.Revoke(ctx, deletion.id, now); err != nil {
			return err
		}
		intent.CleanupRequested = true
		return s.queueIntentCleanup(ctx, repos, intent, now)
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return err
	}
	return s.queueDeletionAs(ctx, repos, deletion, idempotencyKey, now)
}

func (s *Service) queueDeletionAs(ctx context.Context, repos repositories.Repositories, deletion objectDeletion, idempotencyKey string, now time.Time) error {
	payload, err := json.Marshal(deletion.payload())
	if err != nil {
		return fmt.Errorf("encode the storage deletion payload: %w", err)
	}
	row, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: row, Lane: domain.LaneStorageDelete, Priority: domain.PriorityNormal, Kind: domain.KindStorageDelete, Payload: payload,
		IdempotencyKey: idempotencyKey, NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

func (s *Service) PurgeUploadsOf(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	uploads, err := repos.Attachments.UploadsOf(ctx, user)
	if err != nil {
		return err
	}
	for _, upload := range uploads {
		if _, err := repos.Attachments.Lock(ctx, upload.ID); err != nil {
			return err
		}
	}
	for _, upload := range uploads {
		if _, err := repos.UploadIntents.Lock(ctx, upload.ID); err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
	}
	for _, upload := range uploads {
		deletion := objectDeletion{id: upload.ID, key: upload.StorageKey, upload: upload.MultipartUploadID}
		if err := s.queueDeletionWithKey(ctx, repos, deletion, string(domain.KindStorageDelete)+":"+upload.ID.String()+":purge", now); err != nil {
			return err
		}
	}
	_, err = repos.Attachments.DeleteUploadsOf(ctx, user)
	return err
}

var deletableKeyPrefixes = []string{domain.AvatarKeyPrefix, domain.ChatKeyPrefix, domain.LibraryKeyPrefix, domain.PromptImageKeyPrefix}

func (s *Service) DeleteObject(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (domain.OutboxDoneReason, error) {
	var payload domain.AttachmentPayload
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return "", domain.PermanentEffect(fmt.Errorf("%w: %w", errMalformedPayload, err))
	}
	id, ok := domain.ParseUUID[domain.AttachmentID](payload.Attachment)
	if !ok || !slices.ContainsFunc(deletableKeyPrefixes, func(prefix string) bool { return strings.HasPrefix(payload.Key, prefix) }) {
		return "", domain.PermanentEffect(errMalformedPayload)
	}
	if payload.Generation != nil || payload.Attempt != nil {
		return s.deleteUploadIntent(ctx, row, payload, id, now)
	}
	repos := s.deps.Store.Repositories()
	if _, err := repos.UploadIntents.ByID(ctx, id); err == nil {
		return domain.DoneObsolete, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return "", err
	}
	attachment, err := repos.Attachments.ByID(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return "", err
	}
	if found && (attachment.State != domain.AttachmentDeleted || attachment.StorageKey != payload.Key) {
		return domain.DoneObsolete, nil
	}
	if payload.Upload != "" {
		if err := s.deps.Objects.AbortMultipartUpload(ctx, payload.Key, storage.MultipartUploadID(payload.Upload)); err != nil {
			return "", err
		}
	}
	if err := s.deps.Objects.Delete(ctx, payload.Key); err != nil {
		return "", err
	}
	if found {
		if _, err := repos.Attachments.DeleteRow(ctx, id); err != nil {
			return "", err
		}
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.deleted", attachmentAttribute(id))
	return domain.DoneSent, nil
}

func (s *Service) ExpirePending(ctx context.Context, now time.Time) (int64, error) {
	var expired int64
	sweeps := []func(context.Context, repositories.Repositories, time.Time) (int, error){
		s.expireBatch, s.expireAgedUploadingBatch, s.expireUnbindableBatch, s.expireUnbindableLibraryBatch,
	}
	for _, sweep := range sweeps {
		for range expireRounds {
			var claimed int
			err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
				var err error
				claimed, err = sweep(ctx, repos, now)
				return err
			})
			if err != nil {
				return expired, err
			}
			expired += int64(claimed)
			if claimed < expireBatch {
				break
			}
		}
	}
	return expired, nil
}

func (s *Service) expireBatch(ctx context.Context, repos repositories.Repositories, now time.Time) (int, error) {
	ids, err := repos.Attachments.ClaimExpired(ctx, now, expireBatch)
	if err != nil {
		return 0, err
	}
	return len(ids), s.discardAll(ctx, repos, ids, now)
}

func (s *Service) expireUnbindableBatch(ctx context.Context, repos repositories.Repositories, now time.Time) (int, error) {
	ids, err := repos.Attachments.ClaimUnbindableChat(ctx, expireBatch)
	if err != nil {
		return 0, err
	}
	return len(ids), s.discardAll(ctx, repos, ids, now)
}

func (s *Service) expireUnbindableLibraryBatch(ctx context.Context, repos repositories.Repositories, now time.Time) (int, error) {
	ids, err := repos.Attachments.ClaimUnbindableLibrary(ctx, expireBatch)
	if err != nil {
		return 0, err
	}
	return len(ids), s.discardAll(ctx, repos, ids, now)
}

func (s *Service) expireAgedUploadingBatch(ctx context.Context, repos repositories.Repositories, now time.Time) (int, error) {
	aged, err := repos.Attachments.ClaimAged(ctx, uploadingSince(now), expireBatch)
	if err != nil {
		return 0, err
	}
	ids := make([]domain.AttachmentID, 0, len(aged))
	for _, upload := range aged {
		ids = append(ids, upload.ID)
	}
	return len(aged), s.discardAll(ctx, repos, ids, now)
}

func (s *Service) discardAll(ctx context.Context, repos repositories.Repositories, ids []domain.AttachmentID, now time.Time) error {
	slices.SortFunc(ids, func(a, b domain.AttachmentID) int { return bytes.Compare(a[:], b[:]) })
	for _, id := range ids {
		if _, err := repos.Attachments.Lock(ctx, id); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if _, err := repos.UploadIntents.Lock(ctx, id); err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
	}
	for _, id := range ids {
		if err := s.discard(ctx, repos, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) rejected(ctx context.Context, uploader domain.UserID, uploaded domain.AttachmentContext, reason string) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.rejected", userAttribute(uploader), slog.String("context", string(uploaded)), slog.String("reason", reason))
}

func userAttribute(user domain.UserID) slog.Attr {
	return slog.String("user_id", user.String())
}

func attachmentAttribute(id domain.AttachmentID) slog.Attr {
	return slog.String("attachment_id", id.String())
}

type decodeBudget struct {
	mutex  sync.Mutex
	limit  int64
	used   int64
	active map[domain.UserID]bool
}

func newDecodeBudget(limit int64) *decodeBudget {
	return &decodeBudget{limit: limit, active: map[domain.UserID]bool{}}
}

func (b *decodeBudget) take(user domain.UserID, cost int64) (func(), bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.active[user] || b.used+cost > b.limit {
		return nil, false
	}
	b.used += cost
	b.active[user] = true
	return func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.used -= cost
		delete(b.active, user)
	}, true
}

type gate struct {
	mutex   sync.Mutex
	total   int
	perUser map[domain.UserID]int
}

func newGate() *gate {
	return &gate{perUser: map[domain.UserID]int{}}
}

func (g *gate) enter(user domain.UserID) (func(), error) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.total >= concurrentPerInstance || g.perUser[user] >= concurrentPerUser {
		return nil, ErrBusy
	}
	g.total++
	g.perUser[user]++
	return func() {
		g.mutex.Lock()
		defer g.mutex.Unlock()
		g.total--
		g.perUser[user]--
		if g.perUser[user] == 0 {
			delete(g.perUser, user)
		}
	}, nil
}

package files

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"sync"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	detectionBytes        = 64 << 10
	objectContentType     = "application/octet-stream"
	completedDeletionMark = ":completed"
	inspectionDeadline    = 30 * time.Second
)

var (
	ErrOutOfScope     = errors.New("the upload scope is outside the caller's channels")
	ErrReadOnly       = errors.New("the channel is read-only")
	ErrQuotaExceeded  = errors.New("the tutor's file quota has no room")
	ErrInvalidName    = errors.New("the file name is refused")
	ErrWebPTooLarge   = errors.New("the WebP picture is larger than one upload part")
	ErrStalled        = errors.New("the upload body stalled")
	ErrUnexpectedPart = errors.New("the upload body holds another part")
	ErrBodyRead       = errors.New("the upload body failed to arrive")
)

type ChatUpload struct {
	Principal accounts.Principal
	Channel   domain.ChannelID
	Filename  string
	Declared  int64
	Body      io.Reader
	Finish    func() error
}

type openUpload struct {
	id       domain.AttachmentID
	key      string
	channel  domain.ChannelID
	tutor    domain.UserID
	upload   storage.MultipartUploadID
	created  time.Time
	uploader accounts.Principal
}

type storedContent struct {
	kind   fileKind
	size   int64
	sum    []byte
	pixels pictureSize
}

func (s *Service) UploadChat(ctx context.Context, request ChatUpload) (Upload, error) {
	uploader := request.Principal.UserID
	name, valid := cleanFileName(request.Filename)
	if !valid {
		s.rejectedFor(ctx, uploader, domain.RejectedFilename)
		return Upload{}, ErrInvalidName
	}
	release, err := s.gate.enter(uploader)
	if err != nil {
		s.rejectedFor(ctx, uploader, domain.RejectedBusy)
		return Upload{}, err
	}
	defer release()
	opened, err := s.openChatUpload(ctx, request, name)
	if err != nil {
		return Upload{}, err
	}
	content, completed, err := s.streamChatUpload(ctx, opened, request, name)
	if err != nil {
		s.reportStreamFailure(ctx, opened, err)
		s.discardOrReport(context.WithoutCancel(ctx), opened, completed)
		return Upload{}, err
	}
	if err := s.settleChatUpload(context.WithoutCancel(ctx), opened, content); err != nil {
		return Upload{}, err
	}
	mediaType := content.kind.mediaType()
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.accepted", userAttribute(uploader), attachmentAttribute(opened.id),
		channelAttribute(opened.channel), slog.String("context", string(domain.ContextChat)), slog.String("media_type", mediaType),
		slog.Int64("size_bytes", content.size))
	return Upload{ID: opened.id, Filename: name, MediaType: mediaType, SizeBytes: content.size}, nil
}

func uploadingSince(now time.Time) time.Time {
	return now.Add(-(domain.UploadDeadlineCeiling + domain.UploadAgeAfterDeadline))
}

func reservedBytes(declared int64) int64 {
	if declared <= 0 || declared > domain.ChatMaxBytes {
		return domain.ChatMaxBytes
	}
	return declared
}

func utcDay(instant time.Time) time.Time {
	return instant.UTC().Truncate(24 * time.Hour)
}

func (s *Service) lockUploader(ctx context.Context, repos repositories.Repositories, principal accounts.Principal, now time.Time) error {
	if principal.Role == domain.RoleStudent {
		return accounts.LockActingAccount(ctx, repos, principal, now, domain.AccountActive)
	}
	err := repos.TutorProfiles.Lock(ctx, principal.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return accounts.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	account, err := repos.Users.ByID(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if account.State != domain.AccountActive {
		return accounts.ErrUnauthenticated
	}
	return nil
}

func (s *Service) openChatUpload(ctx context.Context, request ChatUpload, name string) (openUpload, error) {
	uploader := request.Principal.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return openUpload{}, err
	}
	id, err := core.NewID[domain.AttachmentID](s.deps.Random)
	if err != nil {
		return openUpload{}, err
	}
	_, keyText, err := core.RandomToken(s.deps.Random, storageKeyBytes)
	if err != nil {
		return openUpload{}, err
	}
	opened := openUpload{id: id, key: domain.ChatKeyPrefix + keyText, channel: request.Channel, created: now, uploader: request.Principal}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockUploader(ctx, repos, request.Principal, now); err != nil {
			return err
		}
		channel, err := repos.Channels.LockOfMemberForShare(ctx, request.Channel, uploader)
		if errors.Is(err, repositories.ErrNotFound) {
			return s.refuse(ctx, opened, domain.RejectedScope, ErrOutOfScope)
		}
		if err != nil {
			return err
		}
		opened.tutor = channel.TutorID
		if channel.ReadOnlySince != nil {
			return s.refuse(ctx, opened, domain.RejectedReadOnly, ErrReadOnly)
		}
		if err := s.admitChatUpload(ctx, repos, opened, now); err != nil {
			return err
		}
		scope := domain.ScopeChannel
		scopeID := domain.EntityID(request.Channel)
		return repos.Attachments.CreateUploading(ctx, repositories.NewUploading{
			ID: id, UploaderID: uploader, Context: domain.ContextChat, ScopeKind: &scope, ScopeID: &scopeID, QuotaTutorID: &opened.tutor,
			StorageKey: opened.key, Filename: name, ReservedBytes: reservedBytes(request.Declared), CreatedAt: now,
		})
	})
	if errors.Is(err, accounts.ErrUnauthenticated) {
		s.rejectedFor(ctx, uploader, domain.RejectedSessionRevoked)
	}
	if err != nil {
		return openUpload{}, err
	}
	upload, err := s.deps.Objects.CreateMultipartUpload(ctx, opened.key, objectContentType)
	if err != nil {
		s.discardOrReport(context.WithoutCancel(ctx), opened, false)
		return openUpload{}, err
	}
	opened.upload = upload
	recorded := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		recorded, err = repos.Attachments.RecordMultipartUpload(ctx, id, string(upload), now)
		return err
	})
	if err != nil {
		s.discardOrReport(context.WithoutCancel(ctx), opened, false)
		return openUpload{}, err
	}
	if !recorded {
		s.discardOrReport(context.WithoutCancel(ctx), opened, false)
		return openUpload{}, ErrOutOfScope
	}
	return opened, nil
}

func (s *Service) admitChatUpload(ctx context.Context, repos repositories.Repositories, opened openUpload, now time.Time) error {
	uploader := opened.uploader.UserID
	open, err := repos.Attachments.CountOpenChatUploads(ctx, uploader, now, uploadingSince(now))
	if err != nil {
		return err
	}
	if open >= domain.ChatOpenUploadsPerUser {
		s.rateLimited(ctx, opened, domain.RejectedPendingCap, domain.LimitPendingUploads)
		return &ThrottledError{RetryAfter: time.Hour}
	}
	if retryAfter, admitted := s.hourly.take(uploader, now); !admitted {
		s.rateLimited(ctx, opened, domain.RejectedHourlyCap, domain.LimitHourlyUploads)
		return &ThrottledError{RetryAfter: retryAfter}
	}
	if opened.uploader.Role == domain.RoleStudent {
		day := utcDay(now)
		total, err := repos.Attachments.DailyChatBytes(ctx, uploader, opened.tutor, day)
		if err != nil {
			return err
		}
		if total >= domain.ChatDailyBytesPerTutor {
			s.rateLimited(ctx, opened, domain.RejectedDailyCap, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: day.Add(24 * time.Hour).Sub(now)}
		}
	}
	use, err := repos.Attachments.ChatQuotaUse(ctx, opened.tutor, nil, now, uploadingSince(now))
	if err != nil {
		return err
	}
	if use.FileUsageBytes+use.PendingBytes >= use.FileQuotaBytes {
		return s.quotaRefusal(ctx, opened)
	}
	return nil
}

func refusalOfChecks(err error) bool {
	var throttled *ThrottledError
	return errors.Is(err, ErrQuotaExceeded) || errors.As(err, &throttled)
}

func (s *Service) quotaRefusal(ctx context.Context, opened openUpload) error {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", slog.String("tutor_id", opened.tutor.String()),
		userAttribute(opened.uploader.UserID), channelAttribute(opened.channel))
	return s.refuse(ctx, opened, domain.RejectedQuota, ErrQuotaExceeded)
}

func (s *Service) refuse(ctx context.Context, opened openUpload, reason domain.UploadRejection, err error) error {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.rejected", userAttribute(opened.uploader.UserID),
		channelAttribute(opened.channel), slog.String("reason", string(reason)))
	return err
}

func (s *Service) rateLimited(ctx context.Context, opened openUpload, reason domain.UploadRejection, limit domain.ChatLimit) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.rate_limited", userAttribute(opened.uploader.UserID),
		channelAttribute(opened.channel), slog.String("limit", string(limit)))
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.rejected", userAttribute(opened.uploader.UserID),
		channelAttribute(opened.channel), slog.String("reason", string(reason)))
}

func (s *Service) RecordRejection(ctx context.Context, uploader domain.UserID, reason domain.UploadRejection) {
	s.rejectedFor(ctx, uploader, reason)
}

func (s *Service) rejectedFor(ctx context.Context, uploader domain.UserID, reason domain.UploadRejection) {
	s.rejected(ctx, uploader, string(reason))
}

type sizeLimit struct {
	source    io.Reader
	remaining int64
}

func (l *sizeLimit) Read(buffer []byte) (int, error) {
	read, err := l.source.Read(buffer)
	l.remaining -= int64(read)
	if l.remaining < 0 {
		return read, ErrTooLarge
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return read, fmt.Errorf("%w: %w", ErrBodyRead, err)
	}
	return read, err
}

type partSink struct {
	ctx     context.Context
	objects storage.Store
	key     string
	upload  storage.MultipartUploadID
	buffer  []byte
	parts   []storage.CompletedPart
	next    storage.PartNumber
	sum     hash.Hash
	size    int64
}

func newPartSink(ctx context.Context, objects storage.Store, opened openUpload) *partSink {
	return &partSink{
		ctx: ctx, objects: objects, key: opened.key, upload: opened.upload, buffer: make([]byte, 0, domain.UploadPartBytes), sum: sha256.New(),
	}
}

func (p *partSink) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		step := min(cap(p.buffer)-len(p.buffer), len(data))
		p.buffer = append(p.buffer, data[:step]...)
		p.sum.Write(data[:step])
		p.size += int64(step)
		data = data[step:]
		written += step
		if len(p.buffer) == cap(p.buffer) {
			if err := p.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (p *partSink) flush() error {
	if len(p.buffer) == 0 {
		return nil
	}
	p.next++
	part, err := p.objects.UploadPart(p.ctx, p.key, p.upload, p.next, p.buffer)
	if err != nil {
		return err
	}
	p.parts = append(p.parts, part)
	p.buffer = p.buffer[:0]
	return nil
}

func (p *partSink) complete() error {
	if err := p.flush(); err != nil {
		return err
	}
	return p.objects.CompleteMultipartUpload(p.ctx, p.key, p.upload, p.parts)
}

func (s *Service) streamChatUpload(ctx context.Context, opened openUpload, request ChatUpload, name string) (storedContent, bool, error) {
	limited := &sizeLimit{source: request.Body, remaining: domain.ChatMaxBytes}
	input := bufio.NewReaderSize(limited, detectionBytes)
	head, err := input.Peek(detectionBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return storedContent{}, false, err
	}
	kind, detected := detectKind(head, name)
	if !detected || len(head) == 0 {
		return storedContent{}, false, ErrUnsupported
	}
	sink := newPartSink(ctx, s.deps.Objects, opened)
	content := storedContent{kind: kind}
	switch {
	case kind == kindWebP:
		content.pixels, err = sink.holdWebP(input)
	case kind.picture():
		content.pixels, err = filterPicture(kind, input, sink)
		if err == nil {
			_, err = io.Copy(io.Discard, input)
		}
	case kind == kindPlainText:
		check := newPlainTextCheck()
		if _, err = io.Copy(io.MultiWriter(sink, check), input); err == nil {
			err = check.finish()
		}
	default:
		_, err = io.Copy(sink, input)
	}
	if err != nil {
		return storedContent{}, false, err
	}
	if err := request.Finish(); err != nil {
		return storedContent{}, false, err
	}
	if sink.size == 0 {
		return storedContent{}, false, ErrUnsupported
	}
	if err := sink.complete(); err != nil {
		return storedContent{}, false, err
	}
	content.size, content.sum = sink.size, sink.sum.Sum(nil)
	if kind.office() {
		inspecting, cancel := context.WithTimeout(ctx, inspectionDeadline)
		err := inspectContainer(storage.NewRangeReader(inspecting, s.deps.Objects, opened.key), content.size, kind)
		cancel()
		if err != nil {
			return storedContent{}, true, err
		}
	}
	return content, true, nil
}

func (p *partSink) holdWebP(input *bufio.Reader) (pictureSize, error) {
	held := p.buffer[:cap(p.buffer)]
	read, err := io.ReadFull(input, held)
	switch {
	case err == nil:
		if _, extra := input.ReadByte(); extra == nil {
			return pictureSize{}, ErrWebPTooLarge
		} else if !errors.Is(extra, io.EOF) {
			return pictureSize{}, extra
		}
	case !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF):
		return pictureSize{}, err
	}
	filtered, size, err := filterWebP(held[:read])
	if err != nil {
		return pictureSize{}, err
	}
	p.buffer = filtered
	p.sum.Write(filtered)
	p.size = int64(len(filtered))
	return size, nil
}

func (s *Service) reportStreamFailure(ctx context.Context, opened openUpload, err error) {
	uploader := opened.uploader.UserID
	var reason domain.UploadRejection
	switch {
	case errors.Is(err, ErrStalled):
		reason = domain.RejectedStalled
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrWebPTooLarge):
		reason = domain.RejectedTooLarge
	case errors.Is(err, ErrContainer):
		reason = domain.RejectedContainer
	case errors.Is(err, ErrUnsupported):
		reason = domain.RejectedUnsupported
	case errors.Is(err, ErrUnexpectedPart):
		reason = domain.RejectedParts
	default:
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.aborted", userAttribute(uploader), attachmentAttribute(opened.id),
			slog.String("kind", core.ErrorKind(err)))
		return
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.rejected", userAttribute(uploader), attachmentAttribute(opened.id),
		channelAttribute(opened.channel), slog.String("reason", string(reason)))
}

func (s *Service) discardOrReport(ctx context.Context, opened openUpload, completed bool) {
	if err := s.discardChatUpload(ctx, opened, completed); err != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelError, "upload.discard_failed", userAttribute(opened.uploader.UserID),
			attachmentAttribute(opened.id), slog.String("kind", core.ErrorKind(err)))
	}
}

func (s *Service) discardChatUpload(ctx context.Context, opened openUpload, completed bool) error {
	var aborted error
	if opened.upload != "" {
		aborted = s.deps.Objects.AbortMultipartUpload(ctx, opened.key, opened.upload)
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return errors.Join(aborted, err)
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		return s.discardLocked(ctx, repos, opened, completed, now)
	})
	return errors.Join(aborted, err)
}

func (s *Service) discardLocked(ctx context.Context, repos repositories.Repositories, opened openUpload, completed bool, now time.Time) error {
	row, err := repos.Attachments.Lock(ctx, opened.id)
	found := err == nil
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return err
	}
	deletion := objectDeletion{id: opened.id, key: opened.key}
	if opened.upload != "" {
		upload := string(opened.upload)
		deletion.upload = &upload
	}
	if found && row.State != domain.AttachmentDeleted {
		if _, err := repos.Attachments.MarkDeleted(ctx, opened.id, now); err != nil {
			return err
		}
		return s.queueDeletion(ctx, repos, deletion, now)
	}
	if !completed {
		return nil
	}
	return s.queueDeletionAs(ctx, repos, deletion, string(domain.KindStorageDelete)+":"+opened.id.String()+completedDeletionMark, now)
}

func (s *Service) settleChatUpload(ctx context.Context, opened openUpload, content storedContent) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		s.discardOrReport(ctx, opened, true)
		return err
	}
	var refusal error
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		refusal = nil
		if err := s.lockUploader(ctx, repos, opened.uploader, now); errors.Is(err, accounts.ErrUnauthenticated) {
			refusal = s.refuse(ctx, opened, domain.RejectedSessionRevoked, err)
			return s.discardLocked(ctx, repos, opened, true, now)
		} else if err != nil {
			return err
		}
		channel, err := repos.Channels.LockOfMemberForShare(ctx, opened.channel, opened.uploader.UserID)
		if errors.Is(err, repositories.ErrNotFound) {
			refusal = s.refuse(ctx, opened, domain.RejectedScope, ErrOutOfScope)
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if err != nil {
			return err
		}
		row, err := repos.Attachments.Lock(ctx, opened.id)
		if errors.Is(err, repositories.ErrNotFound) {
			refusal = ErrPurgedDuringUpload
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if err != nil {
			return err
		}
		if row.State != domain.AttachmentUploading {
			refusal = s.refuse(ctx, opened, domain.RejectedScope, ErrOutOfScope)
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if channel.ReadOnlySince != nil {
			refusal = s.refuse(ctx, opened, domain.RejectedReadOnly, ErrReadOnly)
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if err := s.exactChecks(ctx, repos, opened, content.size, now); err != nil {
			if !refusalOfChecks(err) {
				return err
			}
			refusal = err
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		completed := repositories.CompletedUpload{
			ID: opened.id, MediaType: content.kind.mediaType(), SizeBytes: content.size, Sha256: content.sum,
			ExpiresAt: now.Add(domain.PendingUploadLifetime), Now: now,
		}
		width, widthFits := int32Of(content.pixels.width)
		height, heightFits := int32Of(content.pixels.height)
		if content.pixels.previewable() && widthFits && heightFits {
			completed.Width, completed.Height = &width, &height
		}
		_, err = repos.Attachments.CompleteUpload(ctx, completed)
		return err
	})
	if err != nil {
		s.discardOrReport(ctx, opened, true)
		return err
	}
	return refusal
}

func (s *Service) exactChecks(ctx context.Context, repos repositories.Repositories, opened openUpload, size int64, now time.Time) error {
	if opened.uploader.Role == domain.RoleStudent {
		day := utcDay(opened.created)
		total, err := repos.Attachments.DailyChatBytes(ctx, opened.uploader.UserID, opened.tutor, day)
		if err != nil {
			return err
		}
		if total+size > domain.ChatDailyBytesPerTutor {
			s.rateLimited(ctx, opened, domain.RejectedDailyCap, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: max(day.Add(24*time.Hour).Sub(now), time.Second)}
		}
	}
	excluded := opened.id
	use, err := repos.Attachments.ChatQuotaUse(ctx, opened.tutor, &excluded, now, uploadingSince(now))
	if err != nil {
		return err
	}
	if use.FileUsageBytes+use.PendingBytes+size > use.FileQuotaBytes {
		return s.quotaRefusal(ctx, opened)
	}
	return nil
}

type hourlyUploads struct {
	mutex  sync.Mutex
	window time.Time
	counts map[domain.UserID]int
}

func newHourlyUploads() *hourlyUploads {
	return &hourlyUploads{counts: map[domain.UserID]int{}}
}

func (h *hourlyUploads) take(user domain.UserID, now time.Time) (time.Duration, bool) {
	window, retryAfter := domain.FixedWindow(now, uploadWindow)
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if !window.Equal(h.window) {
		h.window = window
		clear(h.counts)
	}
	if h.counts[user] >= domain.ChatUploadsPerHour {
		return retryAfter, false
	}
	h.counts[user]++
	return 0, true
}

func channelAttribute(channel domain.ChannelID) slog.Attr {
	return slog.String("channel_id", channel.String())
}

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
	detectionBytes     = 64 << 10
	objectContentType  = "application/octet-stream"
	inspectionDeadline = 30 * time.Second
)

var (
	ErrOutOfScope     = errors.New("the upload scope is outside the caller's channels")
	ErrReadOnly       = errors.New("the channel is read-only")
	ErrNotEditable    = errors.New("the submission can no longer be edited by its student")
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
	id         domain.AttachmentID
	key        string
	context    domain.AttachmentContext
	channel    domain.ChannelID
	submission domain.SubmissionID
	tutor      domain.UserID
	upload     storage.MultipartUploadID
	created    time.Time
	uploader   accounts.Principal
	kinds      *kindPolicy
}

type storedContent struct {
	kind   fileKind
	size   int64
	sum    []byte
	pixels pictureSize
}

func (s *Service) UploadChat(ctx context.Context, request ChatUpload) (Upload, error) {
	admission, err := s.BeginChatUpload(ctx, request.Principal, request.Channel, request.Declared)
	if err != nil {
		return Upload{}, err
	}
	defer admission.Close(ctx)
	return admission.Upload(ctx, request.Filename, request.Body, request.Finish)
}

func uploadingSince(now time.Time) time.Time {
	return now.Add(-(domain.UploadDeadlineCeiling + domain.UploadAgeAfterDeadline))
}

func reservedBytes(declared int64, limit int64) int64 {
	if declared <= 0 || declared > limit {
		return limit
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

	checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return err
	}
	session, err := repos.Sessions.ByID(ctx, principal.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return accounts.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if session.UserID != principal.UserID || session.RevokedAt != nil || !checkedAt.Before(session.ExpiresAt) {
		return accounts.ErrUnauthenticated
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
	return s.startUpload(ctx, s.chatTarget(request.Channel), request.Principal, request.Declared, name)
}

func (s *Service) startUpload(ctx context.Context, target uploadTarget, principal accounts.Principal, declared int64, name string) (openUpload, error) {
	uploader := principal.UserID
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
	opened := openUpload{id: id, key: target.policy.keyPrefix + keyText, context: target.policy.context, created: now, uploader: principal}
	target.scope.describe(&opened)
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockUploader(ctx, repos, principal, now); err != nil {
			return err
		}
		checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
		if err != nil {
			return err
		}
		now, opened.created = checkedAt, checkedAt
		locked, err := target.scope.lock(ctx, repos, principal, now)
		if errors.Is(err, repositories.ErrNotFound) {
			return s.refuse(ctx, opened, domain.RejectedScope, ErrOutOfScope)
		}
		if err != nil {
			return err
		}
		opened.tutor, opened.kinds = locked.tutor, locked.kinds
		if locked.closure != nil {
			return s.refuse(ctx, opened, locked.closure.reason, locked.closure.err)
		}
		reserved := reservedBytes(declared, target.policy.maxBytes)
		if err := s.admitUpload(ctx, repos, target, opened, reserved, now); err != nil {
			return err
		}
		scope := target.scope.row()
		if err := repos.Attachments.CreateUploading(ctx, repositories.NewUploading{
			ID: id, UploaderID: &uploader, Context: target.policy.context, ScopeKind: scope.kind, ScopeID: scope.id,
			ScopeQuestionID: scope.question, QuotaTutorID: &opened.tutor, StorageKey: opened.key, Filename: name, ReservedBytes: reserved,
			CreatedAt: now,
		}); err != nil {
			return err
		}
		return repos.UploadIntents.Create(ctx, id, opened.key, target.policy.class, now)
	})
	if errors.Is(err, accounts.ErrUnauthenticated) {
		s.rejectedFor(ctx, uploader, target.policy.context, domain.RejectedSessionRevoked)
	}
	if err != nil {
		return openUpload{}, err
	}

	return opened, nil
}

func (s *Service) admitUpload(
	ctx context.Context, repos repositories.Repositories, target uploadTarget, opened openUpload, reserved int64, now time.Time,
) error {
	uploader := opened.uploader.UserID
	if err := s.checkDailyActivation(ctx, repos, target.policy, opened, now); err != nil {
		return err
	}
	open, err := repos.Attachments.CountOpenUploads(ctx, uploader, target.policy.counted, now, uploadingSince(now))
	if err != nil {
		return err
	}
	if open >= target.policy.openLimit {
		s.rateLimited(ctx, opened, domain.RejectedPendingCap, domain.LimitPendingUploads)
		return &ThrottledError{RetryAfter: time.Hour}
	}
	if retryAfter, admitted := target.hourly.take(uploader, now); !admitted {
		s.rateLimited(ctx, opened, domain.RejectedHourlyCap, domain.LimitHourlyUploads)
		return &ThrottledError{RetryAfter: retryAfter}
	}
	if daily, counted := target.policy.dailyCap(opened); counted {
		day := utcDay(now)
		total, err := dailyBytes(ctx, repos, target.policy.daily, opened, day)
		if err != nil {
			return err
		}
		if total >= daily {
			s.rateLimited(ctx, opened, domain.RejectedDailyCap, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: day.Add(24 * time.Hour).Sub(now)}
		}
	}
	if target.policy.answerBytes {
		open, err := repos.Attachments.OpenAnswerBytes(ctx, uploader, opened.tutor, nil, now, uploadingSince(now))
		if err != nil {
			return err
		}
		if open+reserved > domain.AnswerPendingBytesPerTutor {
			s.rateLimited(ctx, opened, domain.RejectedPendingBytes, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: time.Hour}
		}
	}
	use, err := repos.Attachments.QuotaUse(ctx, opened.tutor, nil, now, uploadingSince(now))
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

func (o openUpload) attributes(extra ...slog.Attr) []slog.Attr {
	attributes := []slog.Attr{userAttribute(o.uploader.UserID), slog.String("context", string(o.context))}
	if o.channel != (domain.ChannelID{}) {
		attributes = append(attributes, channelAttribute(o.channel))
	}
	if o.submission != (domain.SubmissionID{}) {
		attributes = append(attributes, slog.String("submission_id", o.submission.String()))
	}
	return append(attributes, extra...)
}

func (s *Service) uploadEvent(ctx context.Context, level slog.Level, event string, opened openUpload, extra ...slog.Attr) {
	s.deps.Logger.LogAttrs(ctx, level, event, opened.attributes(extra...)...)
}

func (s *Service) quotaRefusal(ctx context.Context, opened openUpload) error {
	s.uploadEvent(ctx, slog.LevelInfo, "quota.exceeded", opened, slog.String("tutor_id", opened.tutor.String()))
	return s.refuse(ctx, opened, domain.RejectedQuota, ErrQuotaExceeded)
}

func (s *Service) refuse(ctx context.Context, opened openUpload, reason domain.UploadRejection, err error) error {
	s.uploadEvent(ctx, slog.LevelInfo, "upload.rejected", opened, slog.String("reason", string(reason)))
	return err
}

func (s *Service) rateLimited(ctx context.Context, opened openUpload, reason domain.UploadRejection, limit domain.ChatLimit) {
	if opened.context == domain.ContextChat {
		s.uploadEvent(ctx, slog.LevelInfo, "chat.rate_limited", opened, slog.String("limit", string(limit)))
	}
	s.uploadEvent(ctx, slog.LevelInfo, "upload.rejected", opened, slog.String("reason", string(reason)))
}

func (s *Service) RecordRejection(ctx context.Context, uploader domain.UserID, uploaded domain.AttachmentContext, reason domain.UploadRejection) {
	s.rejectedFor(ctx, uploader, uploaded, reason)
}

func (s *Service) rejectedFor(ctx context.Context, uploader domain.UserID, uploaded domain.AttachmentContext, reason domain.UploadRejection) {
	s.rejected(ctx, uploader, uploaded, string(reason))
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

func (s *Service) streamUpload(ctx context.Context, policy uploadPolicy, opened openUpload, body io.Reader, finish func() error, name string) (storedContent, bool, error) {
	limited := &sizeLimit{source: body, remaining: policy.maxBytes}
	input := bufio.NewReaderSize(limited, detectionBytes)
	head, err := input.Peek(detectionBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return storedContent{}, false, err
	}
	kinds := policy.kinds
	if opened.kinds != nil {
		kinds = *opened.kinds
	}
	kind, detected := kinds.detect(head, name)
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
	if err := finish(); err != nil {
		return storedContent{}, false, err
	}
	if sink.size == 0 {
		return storedContent{}, false, ErrUnsupported
	}
	if err := sink.flush(); err != nil {
		return storedContent{}, false, err
	}
	if err := s.authorizeComplete(ctx, opened); err != nil {
		return storedContent{}, false, err
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
		s.uploadEvent(ctx, slog.LevelInfo, "upload.aborted", opened, attachmentAttribute(opened.id), slog.String("kind", core.ErrorKind(err)))
		return
	}
	s.uploadEvent(ctx, slog.LevelInfo, "upload.rejected", opened, attachmentAttribute(opened.id), slog.String("reason", string(reason)))
}

func (s *Service) discardOrReport(ctx context.Context, opened openUpload, completed bool) {
	if err := s.discardChatUpload(ctx, opened, completed); err != nil {
		s.uploadEvent(ctx, slog.LevelError, "upload.discard_failed", opened, attachmentAttribute(opened.id), slog.String("kind", core.ErrorKind(err)))
	}
}

func (s *Service) discardChatUpload(ctx context.Context, opened openUpload, completed bool) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		return s.discardLocked(ctx, repos, opened, completed, now)
	})
}

func (s *Service) discardLocked(ctx context.Context, repos repositories.Repositories, opened openUpload, completed bool, now time.Time) error {
	return s.reconcileUpload(ctx, repos, opened, completed, now)
}

func (s *Service) settleChatUpload(ctx context.Context, opened openUpload, content storedContent) error {
	return s.settleUpload(ctx, s.chatTarget(opened.channel), opened, content)
}

func (s *Service) settleUpload(ctx context.Context, target uploadTarget, opened openUpload, content storedContent) error {
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
		locked, err := target.scope.lock(ctx, repos, opened.uploader, now)
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
		opened.created = row.CreatedAt
		if row.State != domain.AttachmentUploading {
			refusal = s.refuse(ctx, opened, domain.RejectedScope, ErrOutOfScope)
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if locked.closure != nil {
			refusal = s.refuse(ctx, opened, locked.closure.reason, locked.closure.err)
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
		if err != nil {
			return err
		}
		now = checkedAt
		if err := s.checkDailyActivation(ctx, repos, target.policy, opened, now); err != nil {
			refusal = err
			return s.discardLocked(ctx, repos, opened, true, now)
		}
		if err := s.exactChecks(ctx, repos, target.policy, opened, content.size, now); err != nil {
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
		if _, counted := target.policy.dailyCap(opened); counted {
			if err := addDailyBytes(ctx, repos, target.policy.daily, opened, utcDay(opened.created), content.size); err != nil {
				return err
			}
		}
		intent, err := repos.UploadIntents.Lock(ctx, opened.id)
		if err != nil {
			return err
		}
		if intent.StorageKey != opened.key || intent.CleanupRequested || !intent.CompletionStarted {
			return ErrOutOfScope
		}
		if err := repos.UploadIntents.ConfirmComplete(ctx, opened.id, opened.key, now); err != nil {
			return err
		}
		changed, err := repos.Attachments.CompleteUpload(ctx, completed)
		if err != nil {
			return err
		}
		if !changed {
			return repositories.ErrUploadTransition
		}
		return repos.UploadIntents.Remove(ctx, opened.id, opened.key, now)
	})
	if err != nil {
		s.discardOrReport(ctx, opened, true)
		return err
	}
	return refusal
}

func (s *Service) exactChecks(ctx context.Context, repos repositories.Repositories, policy uploadPolicy, opened openUpload, size int64, now time.Time) error {
	excluded := opened.id
	if daily, counted := policy.dailyCap(opened); counted {
		day := utcDay(opened.created)
		total, err := dailyBytes(ctx, repos, policy.daily, opened, day)
		if err != nil {
			return err
		}
		if total+size > daily {
			s.rateLimited(ctx, opened, domain.RejectedDailyCap, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: max(day.Add(24*time.Hour).Sub(now), time.Second)}
		}
	}
	if policy.answerBytes {
		open, err := repos.Attachments.OpenAnswerBytes(ctx, opened.uploader.UserID, opened.tutor, &excluded, now, uploadingSince(now))
		if err != nil {
			return err
		}
		if open+size > domain.AnswerPendingBytesPerTutor {
			s.rateLimited(ctx, opened, domain.RejectedPendingBytes, domain.LimitAttachmentBytes)
			return &ThrottledError{RetryAfter: time.Hour}
		}
	}
	use, err := repos.Attachments.QuotaUse(ctx, opened.tutor, &excluded, now, uploadingSince(now))
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
	limit  int
	window time.Time
	counts map[domain.UserID]int
}

func newHourlyUploads(limit int) *hourlyUploads {
	return &hourlyUploads{limit: limit, counts: map[domain.UserID]int{}}
}

func (h *hourlyUploads) take(user domain.UserID, now time.Time) (time.Duration, bool) {
	window, retryAfter := domain.FixedWindow(now, uploadWindow)
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if !window.Equal(h.window) {
		h.window = window
		clear(h.counts)
	}
	if h.counts[user] >= h.limit {
		return retryAfter, false
	}
	h.counts[user]++
	return 0, true
}

func channelAttribute(channel domain.ChannelID) slog.Attr {
	return slog.String("channel_id", channel.String())
}

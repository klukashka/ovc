package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/files"
)

const (
	chatWriteDeadline       = 35 * time.Second
	defaultUploadFloor      = 60 * time.Second
	defaultUploadRate       = 128 << 10
	defaultStallWindow      = 30 * time.Second
	defaultStallBytes       = 16 << 10
	multipartPeekBytes      = 4096
	channelScopePrefix      = "channel:"
	fieldScope              = "query.scope"
	fieldFile               = "body.file"
	partHeaderLineOverhead  = 4
	retryAfterBusyUploads   = "60"
	headerSectionAllowance  = domain.UploadPartHeaderBytes + 3*multipartPeekBytes
	contentDispositionField = "Content-Disposition"
	stallChecksPerWindow    = 4
	minimumStallCheck       = 10 * time.Millisecond
)

var errPartHeadersTooLarge = errors.New("the upload part headers are too large")

type UploadTiming struct {
	Floor          time.Duration
	BytesPerSecond int64
	StallWindow    time.Duration
	StallBytes     int64
}

func (t UploadTiming) withDefaults() UploadTiming {
	if t.Floor <= 0 {
		t.Floor = defaultUploadFloor
	}
	if t.BytesPerSecond <= 0 {
		t.BytesPerSecond = defaultUploadRate
	}
	if t.StallWindow <= 0 {
		t.StallWindow = defaultStallWindow
	}
	if t.StallBytes <= 0 {
		t.StallBytes = defaultStallBytes
	}
	return t
}

func (t UploadTiming) deadline(bytes int64) time.Duration {
	transfer := time.Duration(bytes / t.BytesPerSecond * int64(time.Second))
	return min(max(transfer, t.Floor), domain.UploadDeadlineCeiling)
}

type deliverySample struct {
	active    time.Duration
	delivered int64
}

type stallGuard struct {
	body         io.Reader
	mutex        sync.Mutex
	timer        *time.Timer
	stopped      bool
	stalled      bool
	reading      bool
	readingSince time.Time
	active       time.Duration
	delivered    int64
	samples      []deliverySample
	controller   *http.ResponseController
	timing       UploadTiming
	ctx          context.Context
	logger       *slog.Logger
	now          func() time.Time
}

func (t UploadTiming) checkEvery() time.Duration {
	return max(t.StallWindow/stallChecksPerWindow, minimumStallCheck)
}

func (r *router) guardAgainstStalls(ctx context.Context, body io.Reader, controller *http.ResponseController) *stallGuard {
	guard := &stallGuard{body: body, controller: controller, timing: r.uploads, ctx: ctx, logger: r.logger, now: r.now}
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	guard.timer = time.AfterFunc(r.uploads.checkEvery(), guard.check)
	return guard
}

func (g *stallGuard) deliveredBy(active time.Duration) int64 {
	var delivered int64
	for _, sample := range g.samples {
		if sample.active > active {
			break
		}
		delivered = sample.delivered
	}
	return delivered
}

func (g *stallGuard) prune(active time.Duration) {
	keep := 0
	for keep+1 < len(g.samples) && g.samples[keep+1].active <= active-g.timing.StallWindow {
		keep++
	}
	g.samples = g.samples[keep:]
}

func (g *stallGuard) check() {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(g.ctx, g.logger, "upload.stall_guard", fmt.Sprintf("%T", recovered))
		}
	}()
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.stopped {
		return
	}
	if g.reading {
		active := g.active + g.now().Sub(g.readingSince)
		if active >= g.timing.StallWindow && g.delivered-g.deliveredBy(active-g.timing.StallWindow) < g.timing.StallBytes {
			g.stalled = true
			if err := g.controller.SetReadDeadline(g.now()); err != nil && !errors.Is(err, http.ErrNotSupported) {
				g.logger.LogAttrs(g.ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "read"))
			}
			return
		}
	}
	g.timer.Reset(g.timing.checkEvery())
}

func (g *stallGuard) stop() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.stopped = true
	g.timer.Stop()
}

func (g *stallGuard) Read(buffer []byte) (int, error) {
	g.mutex.Lock()
	g.reading, g.readingSince = true, g.now()
	g.mutex.Unlock()
	read, err := g.body.Read(buffer)
	g.mutex.Lock()
	g.active += g.now().Sub(g.readingSince)
	g.reading = false
	g.delivered += int64(read)
	sample := deliverySample{active: g.active, delivered: g.delivered}
	bucket := g.timing.checkEvery()
	if last := len(g.samples) - 1; last >= 0 && g.samples[last].active/bucket == sample.active/bucket {
		g.samples[last] = sample
	} else {
		g.samples = append(g.samples, sample)
	}
	g.prune(g.active)
	stalled := g.stalled
	g.mutex.Unlock()
	if err != nil && stalled {
		return read, fmt.Errorf("%w: %w", files.ErrStalled, err)
	}
	return read, err
}

type headerSection struct {
	body      io.Reader
	remaining int64
	open      bool
}

func (h *headerSection) Read(buffer []byte) (int, error) {
	if h.open {
		return h.body.Read(buffer)
	}
	if h.remaining <= 0 {
		return 0, errPartHeadersTooLarge
	}
	read, err := h.body.Read(buffer[:min(int64(len(buffer)), h.remaining)])
	h.remaining -= int64(read)
	return read, err
}

func partHeaderBytes(header map[string][]string) int {
	total := 0
	for name, values := range header {
		for _, value := range values {
			total += len(name) + len(value) + partHeaderLineOverhead
		}
	}
	return total
}

func parseChannelScope(scope string) (domain.ChannelID, bool) {
	text, found := strings.CutPrefix(scope, channelScopePrefix)
	if !found {
		return domain.ChannelID{}, false
	}
	return domain.ParseUUID[domain.ChannelID](text)
}

func rawFileName(part *multipart.Part) string {
	_, parameters, err := mime.ParseMediaType(part.Header.Get(contentDispositionField))
	if err != nil {
		return ""
	}
	return parameters["filename"]
}

func streamedUploadLimit(uploaded domain.AttachmentContext) (int64, bool) {
	switch uploaded {
	case domain.ContextChat:
		return domain.ChatMaxBytes, true
	case domain.ContextLibrary:
		return domain.LibraryMaxBytes, true
	case domain.ContextPromptImage:
		return domain.PromptImageMaxBytes, true
	case domain.ContextAvatar, domain.ContextAnswer, domain.ContextCorrected:
		return 0, false
	}
	return 0, false
}

func earlyUploadRefusal(uploaded domain.AttachmentContext, principal accounts.Principal, declared int64, limit int64) (domain.ErrorCode, bool) {
	library := uploaded == domain.ContextLibrary || uploaded == domain.ContextPromptImage
	switch {
	case library && principal.Role != domain.RoleTutor:
		return domain.ErrorNotFound, true
	case principal.State != domain.AccountActive:
		return domain.ErrorEmailNotVerified, true
	case declared > limit+domain.UploadOverheadBytes:
		return domain.ErrorPayloadTooLarge, true
	}
	return "", false
}

type admissionStart func(declared int64) (*files.UploadAdmission, error)

func (r *router) uploadChat(ctx context.Context, input *uploadInput, request *http.Request) (*uploadOutput, error) {
	channel, valid := parseChannelScope(input.Scope)
	if !valid {
		return nil, fieldError(ctx, fieldScope, domain.FieldInvalid)
	}
	return r.uploadStreamed(ctx, domain.ContextChat, request, func(declared int64) (*files.UploadAdmission, error) {
		return r.files.BeginChatUpload(ctx, principalFrom(ctx), channel, declared)
	})
}

func (r *router) uploadLibrary(ctx context.Context, uploaded domain.AttachmentContext, request *http.Request) (*uploadOutput, error) {
	if request.URL.Query().Has("scope") {
		return nil, fieldError(ctx, fieldScope, domain.FieldNotAllowed)
	}
	return r.uploadStreamed(ctx, uploaded, request, func(declared int64) (*files.UploadAdmission, error) {
		return r.files.BeginLibraryUpload(ctx, principalFrom(ctx), uploaded, declared)
	})
}

func (r *router) uploadStreamed(ctx context.Context, uploaded domain.AttachmentContext, request *http.Request, begin admissionStart) (*uploadOutput, error) {
	principal := principalFrom(ctx)
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || parameters["boundary"] == "" {
		return nil, NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	controller, found := ctx.Value(rawControllerKey{}).(*http.ResponseController)
	if !found {
		panic("a streamed upload ran without its response controller")
	}
	admission, err := begin(request.ContentLength)
	if err != nil {
		return nil, r.streamedUploadError(ctx, err)
	}
	defer admission.Close(ctx)
	guard := r.guardAgainstStalls(ctx, request.Body, controller)
	defer guard.stop()
	headers := &headerSection{body: guard, remaining: int64(headerSectionAllowance + len(parameters["boundary"]))}
	reader := multipart.NewReader(headers, parameters["boundary"])
	part, err := reader.NextPart()
	switch {
	case errors.Is(err, io.EOF):
		return nil, uploadFieldError(ctx)
	case errors.Is(err, errPartHeadersTooLarge):
		r.files.RecordRejection(ctx, principal.UserID, uploaded, domain.RejectedParts)
		return nil, uploadFieldError(ctx)
	case err != nil:
		return nil, r.streamedUploadError(ctx, errors.Join(errMalformedUpload, err))
	}
	headers.open = true
	if part.FormName() != "file" || rawFileName(part) == "" || partHeaderBytes(part.Header) > domain.UploadPartHeaderBytes {
		r.files.RecordRejection(ctx, principal.UserID, uploaded, domain.RejectedParts)
		return nil, uploadFieldError(ctx)
	}
	finish := func() error {
		headers.open, headers.remaining = false, int64(headerSectionAllowance+len(parameters["boundary"]))
		_, err := reader.NextPart()
		guard.stop()
		switch {
		case err == nil, errors.Is(err, errPartHeadersTooLarge):
			return files.ErrUnexpectedPart
		case errors.Is(err, io.EOF):
			return nil
		}
		return fmt.Errorf("%w: %w", files.ErrBodyRead, errors.Join(errMalformedUpload, err))
	}
	upload, err := admission.Upload(ctx, rawFileName(part), part, finish)
	if deadlineErr := controller.SetWriteDeadline(r.now().Add(chatWriteDeadline)); deadlineErr != nil && !errors.Is(deadlineErr, http.ErrNotSupported) {
		r.logger.LogAttrs(ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
	}
	if err != nil {
		return nil, r.streamedUploadError(ctx, err)
	}
	return &uploadOutput{CacheControl: noStore, Body: schemas.UploadedFile{
		ID: upload.ID.String(), Filename: upload.Filename, MediaType: upload.MediaType, SizeBytes: upload.SizeBytes,
	}}, nil
}

func fieldError(ctx context.Context, path string, code domain.FieldCode) error {
	return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(
		domain.ErrorValidationFailed, core.RequestID(ctx), []schemas.FieldError{{Path: path, Code: string(code)}},
	)}
}

func (r *router) streamedUploadError(ctx context.Context, err error) error {
	var tooLarge *http.MaxBytesError
	var throttled *files.ThrottledError
	var accountingUnavailable *files.AccountingUnavailableError
	switch {
	case errors.Is(err, files.ErrInvalidName), errors.Is(err, files.ErrUnexpectedPart):
		return fieldError(ctx, fieldFile, domain.FieldInvalid)
	case errors.Is(err, files.ErrOutOfScope):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, files.ErrReadOnly):
		return NewError(ctx, domain.ErrorChannelReadOnly)
	case errors.Is(err, files.ErrQuotaExceeded):
		return NewError(ctx, domain.ErrorQuotaExceeded)
	case errors.Is(err, files.ErrWebPTooLarge):
		return &Error{status: http.StatusRequestEntityTooLarge, ErrorEnvelope: schemas.NewErrorEnvelope(
			domain.ErrorPayloadTooLarge, core.RequestID(ctx), []schemas.FieldError{{Path: fieldFile, Code: string(domain.FieldWebPTooLarge)}},
		)}
	case errors.Is(err, files.ErrTooLarge), errors.As(err, &tooLarge):
		return NewError(ctx, domain.ErrorPayloadTooLarge)
	case errors.Is(err, files.ErrUnsupported), errors.Is(err, files.ErrContainer):
		return NewError(ctx, domain.ErrorUnsupportedMediaType)
	case errors.Is(err, files.ErrPurgedDuringUpload), errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.As(err, &accountingUnavailable):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Connection", "close").withHeader("Retry-After", strconv.Itoa(max(1, int(math.Ceil(accountingUnavailable.RetryAfter.Seconds())))))
	case errors.Is(err, files.ErrUploadCapacity):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Connection", "close")
	case errors.Is(err, files.ErrBusy):
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", retryAfterBusyUploads)
	case errors.As(err, &throttled):
		return r.flowError(ctx, "uploads.create.failed", &accounts.ThrottledError{RetryAfter: throttled.RetryAfter})
	case errors.Is(err, files.ErrStalled), errors.Is(err, files.ErrBodyRead), errors.Is(err, errMalformedUpload),
		errors.Is(err, multipart.ErrMessageTooLarge), errors.Is(err, io.ErrUnexpectedEOF):
		return NewError(ctx, domain.ErrorBadRequest)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorBadRequest)
	}
	return r.internal(ctx, "uploads.create.failed", err)
}

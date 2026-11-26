package files

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	downloadSlots = 64
	downloadRoute = "/api/v1/files/"
	downloadQuery = "?t="

	downloadOpenTimeout = 30 * time.Second
)

var (
	ErrInvalidDisposition = errors.New("the disposition is not one the attachment accepts")
	ErrInvalidReference   = errors.New("the referencing item is not a known kind and identifier")
	ErrDownloadsBusy      = errors.New("the instance is serving as many downloads as it allows at once")
)

type DownloadAddress struct {
	URL       string
	ExpiresAt time.Time
}

type AddressRequest struct {
	Dispositions []string
	References   []string
}

func (r AddressRequest) disposition(width *int32) (domain.Disposition, bool) {
	disposition, valid := r.parseDisposition()
	return disposition, valid && (disposition != domain.DispositionInline || width != nil)
}

func (r AddressRequest) parseDisposition() (domain.Disposition, bool) {
	switch {
	case len(r.Dispositions) == 0:
		return domain.DispositionAttachment, true
	case len(r.Dispositions) > 1:
		return "", false
	}
	switch domain.Disposition(r.Dispositions[0]) {
	case domain.DispositionAttachment:
		return domain.DispositionAttachment, true
	case domain.DispositionInline:
		return domain.DispositionInline, true
	}
	return "", false
}

type visibleReference struct {
	reference DownloadReference
	width     *int32
}

type referenceAuthorization interface {
	kind() domain.ReferenceKind
	visible(ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID, now time.Time) ([]visibleReference, error)
	servable(ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID, now time.Time) (repositories.ServableFile, error)
}

func (s *Service) authorizationOf(kind domain.ReferenceKind) (referenceAuthorization, bool) {
	for _, authorization := range s.references {
		if authorization.kind() == kind {
			return authorization, true
		}
	}
	return nil, false
}

func (s *Service) DownloadAddress(ctx context.Context, viewer domain.UserID, id domain.AttachmentID, request AddressRequest) (DownloadAddress, error) {
	file, err := s.deps.Store.Repositories().Attachments.DownloadableChat(ctx, viewer, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return s.referencedAddress(ctx, viewer, id, request)
	}
	if err != nil {
		return DownloadAddress{}, err
	}
	if len(request.References) > 0 {
		return DownloadAddress{}, ErrNotFound
	}
	disposition, valid := request.disposition(file.Width)
	if !valid {
		return DownloadAddress{}, ErrInvalidDisposition
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return DownloadAddress{}, err
	}
	return s.MintAddress(id, file.MessageID, disposition, now), nil
}

func (s *Service) referencedAddress(ctx context.Context, viewer domain.UserID, id domain.AttachmentID, request AddressRequest) (DownloadAddress, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return DownloadAddress{}, err
	}
	repos := s.deps.Store.Repositories()
	var visible []visibleReference
	for _, authorization := range s.references {
		found, err := authorization.visible(ctx, repos, viewer, id, now)
		if err != nil {
			return DownloadAddress{}, err
		}
		visible = append(visible, found...)
	}
	if len(visible) == 0 || len(request.References) == 0 {
		return DownloadAddress{}, ErrNotFound
	}
	reference, valid := parseReference(request.References)
	if !valid {
		return DownloadAddress{}, ErrInvalidReference
	}
	index := slices.IndexFunc(visible, func(candidate visibleReference) bool { return candidate.reference == reference })
	if index < 0 {
		return DownloadAddress{}, ErrNotFound
	}
	disposition, valid := request.disposition(visible[index].width)
	if !valid {
		return DownloadAddress{}, ErrInvalidDisposition
	}
	return s.ReferenceAddress(id, reference, disposition, now)
}

func (s *Service) MintAddress(id domain.AttachmentID, message domain.EntityID, disposition domain.Disposition, now time.Time) DownloadAddress {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceMessage, Item: message}, disposition, now)
	if err != nil {
		panic("a message download token could not be minted")
	}
	return address
}

func (s *Service) ReferenceAddress(id domain.AttachmentID, reference DownloadReference, disposition domain.Disposition, now time.Time) (DownloadAddress, error) {
	if reference.Kind != domain.ReferenceMessage && !slices.Contains(addressableKinds, reference.Kind) {
		return DownloadAddress{}, errUnmintableReference
	}
	expires := downloadExpiry(now)
	token, err := s.tokens.mint(downloadClaims{Attachment: id, Reference: reference, ExpiresAt: expires, Disposition: disposition})
	if err != nil {
		return DownloadAddress{}, err
	}
	return DownloadAddress{URL: downloadRoute + id.String() + downloadQuery + token, ExpiresAt: expires}, nil
}

func (s *Service) ChatAddress(id domain.AttachmentID, message domain.MessageID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address := s.MintAddress(id, domain.EntityID(message), disposition, now)
	return address.URL, address.ExpiresAt
}

type Download struct {
	Deadline           time.Time
	MediaType          string
	SizeBytes          int64
	ContentDisposition string
	Body               ObjectBody
}

func (s *Service) Download(ctx context.Context, id domain.AttachmentID, token string) (Download, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Download{}, err
	}
	claims, err := s.tokens.verify(token, now)
	if errors.Is(err, errDownloadTokenExpired) {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedExpired)
	}
	if err != nil {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken)
	}
	if claims.Attachment != id {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken, referenceKindAttribute(claims.Reference.Kind))
	}
	if _, authorized := s.authorizationOf(claims.Reference.Kind); claims.Reference.Kind != domain.ReferenceMessage && !authorized {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken, referenceKindAttribute(claims.Reference.Kind))
	}
	opening, cancelCause := context.WithCancelCause(ctx)
	cancel := func() { cancelCause(context.Canceled) }
	deadline := time.AfterFunc(s.deps.Downloads.Open, func() { cancelCause(context.DeadlineExceeded) })
	download, err := s.openDownload(opening, id, claims, cancel, now)
	if err != nil {
		deadline.Stop()
		cause := context.Cause(opening)
		cancel()
		if errors.Is(cause, context.DeadlineExceeded) {
			return Download{}, errors.Join(err, cause)
		}
		return Download{}, err
	}
	if !deadline.Stop() {
		return Download{}, errors.Join(context.DeadlineExceeded, download.Body.Close())
	}
	download.Deadline = clock.System{}.Now().Add(s.deps.Downloads.duration(download.SizeBytes))
	stream := time.AfterFunc(time.Until(download.Deadline), func() { cancelCause(context.DeadlineExceeded) })
	body := download.Body
	download.Body = &timedDownloadBody{
		ObjectBody: body, ctx: opening,
		close: sync.OnceValue(func() error {
			stream.Stop()
			return body.Close()
		}),
	}
	return download, nil
}

type DownloadTiming struct {
	Open           time.Duration
	Floor          time.Duration
	BytesPerSecond int64
}

func (t DownloadTiming) withDefaults() DownloadTiming {
	if t.Open <= 0 {
		t.Open = downloadOpenTimeout
	}
	if t.Floor <= 0 {
		t.Floor = 60 * time.Second
	}
	if t.BytesPerSecond <= 0 {
		t.BytesPerSecond = 128 << 10
	}
	return t
}

func (t DownloadTiming) duration(size int64) time.Duration {
	return min(max(time.Duration(size/t.BytesPerSecond)*time.Second, t.Floor), domain.UploadDeadlineCeiling)
}

type timedDownloadBody struct {
	ObjectBody
	ctx   context.Context
	close func() error
}

func (b *timedDownloadBody) Read(data []byte) (int, error) {
	if b.ctx.Err() != nil {
		return 0, context.Cause(b.ctx)
	}
	n, err := b.ObjectBody.Read(data)
	if err != nil && b.ctx.Err() != nil {
		return n, context.Cause(b.ctx)
	}
	return n, err
}

func (b *timedDownloadBody) Overruns() (bool, error) {
	if b.ctx.Err() != nil {
		return false, context.Cause(b.ctx)
	}
	overruns, err := b.ObjectBody.Overruns()
	if err != nil && b.ctx.Err() != nil {
		return overruns, context.Cause(b.ctx)
	}
	return overruns, err
}

func (b *timedDownloadBody) Close() error {
	return b.close()
}

func (s *Service) openDownload(
	ctx context.Context, id domain.AttachmentID, claims downloadClaims, cancel context.CancelFunc, now time.Time,
) (Download, error) {
	kind := referenceKindAttribute(claims.Reference.Kind)
	file, err := s.servableFile(ctx, s.deps.Store.Repositories(), id, claims.Reference, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedUnreferenced, kind)
	}
	if err != nil {
		return Download{}, err
	}
	stored := slog.String("context", string(file.Context))
	if claims.Disposition == domain.DispositionInline && file.Width == nil {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken, kind, stored)
	}
	select {
	case s.downloads <- struct{}{}:
	default:
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "download.busy", attachmentAttribute(id), kind, stored)
		return Download{}, ErrDownloadsBusy
	}
	release := sync.OnceFunc(func() {
		<-s.downloads
		cancel()
	})
	body, err := s.deps.Objects.Open(ctx, file.StorageKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		release()
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "download.object_missing", attachmentAttribute(id), kind, stored)
		return Download{}, ErrNotFound
	}
	if err != nil {
		release()
		return Download{}, err
	}
	mediaType := file.MediaType
	if mediaType == kindPlainText.mediaType() {
		mediaType += "; charset=utf-8"
	}
	return Download{
		MediaType: mediaType, SizeBytes: file.SizeBytes, ContentDisposition: ContentDisposition(claims.Disposition, file.Filename),
		Body: &objectBody{Reader: io.LimitReader(body, file.SizeBytes), object: body, release: release},
	}, nil
}

func (s *Service) servableFile(
	ctx context.Context, repos repositories.Repositories, id domain.AttachmentID, reference DownloadReference, now time.Time,
) (repositories.ServableFile, error) {
	if reference.Kind == domain.ReferenceMessage {
		return repos.Attachments.ServableChat(ctx, id, reference.Item)
	}
	authorization, authorized := s.authorizationOf(reference.Kind)
	if !authorized {
		return repositories.ServableFile{}, repositories.ErrNotFound
	}
	return authorization.servable(ctx, repos, id, reference.Item, now)
}

func (s *Service) ExportableFile(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, id domain.AttachmentID, reference DownloadReference, now time.Time,
) (repositories.ServableFile, error) {
	if reference.Kind == domain.ReferenceMessage {
		file, err := repos.Attachments.DownloadableChat(ctx, viewer, id)
		if err != nil {
			return repositories.ServableFile{}, err
		}
		if file.MessageID != reference.Item {
			return repositories.ServableFile{}, repositories.ErrNotFound
		}
		return s.servableFile(ctx, repos, id, reference, now)
	}
	authorization, authorized := s.authorizationOf(reference.Kind)
	if !authorized {
		return repositories.ServableFile{}, repositories.ErrNotFound
	}
	visible, err := authorization.visible(ctx, repos, viewer, id, now)
	if err != nil {
		return repositories.ServableFile{}, err
	}
	if !slices.ContainsFunc(visible, func(candidate visibleReference) bool { return candidate.reference == reference }) {
		return repositories.ServableFile{}, repositories.ErrNotFound
	}
	return s.servableFile(ctx, repos, id, reference, now)
}

func (s *Service) refusedDownload(ctx context.Context, id domain.AttachmentID, reason domain.DownloadRefusal, known ...slog.Attr) error {
	attributes := append([]slog.Attr{attachmentAttribute(id), slog.String("reason", string(reason))}, known...)
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "download.refused", attributes...)
	return ErrNotFound
}

func referenceKindAttribute(kind domain.ReferenceKind) slog.Attr {
	return slog.String("ref_kind", string(kind))
}

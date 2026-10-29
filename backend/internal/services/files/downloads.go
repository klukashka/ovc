package files

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

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
	ErrDownloadsBusy      = errors.New("the instance is serving as many downloads as it allows at once")
)

type DownloadAddress struct {
	URL       string
	ExpiresAt time.Time
}

type DispositionRequest struct {
	Values []string
}

func (r DispositionRequest) parse() (domain.Disposition, bool) {
	switch {
	case len(r.Values) == 0:
		return domain.DispositionAttachment, true
	case len(r.Values) > 1:
		return "", false
	}
	switch domain.Disposition(r.Values[0]) {
	case domain.DispositionAttachment:
		return domain.DispositionAttachment, true
	case domain.DispositionInline:
		return domain.DispositionInline, true
	}
	return "", false
}

func (s *Service) DownloadAddress(ctx context.Context, viewer domain.UserID, id domain.AttachmentID, request DispositionRequest) (DownloadAddress, error) {
	file, err := s.deps.Store.Repositories().Attachments.DownloadableChat(ctx, viewer, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return DownloadAddress{}, ErrNotFound
	}
	if err != nil {
		return DownloadAddress{}, err
	}
	disposition, valid := request.parse()
	if !valid || (disposition == domain.DispositionInline && file.Width == nil) {
		return DownloadAddress{}, ErrInvalidDisposition
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return DownloadAddress{}, err
	}
	return s.MintAddress(id, file.MessageID, disposition, now), nil
}

func (s *Service) MintAddress(id domain.AttachmentID, message domain.EntityID, disposition domain.Disposition, now time.Time) DownloadAddress {
	expires := downloadExpiry(now)
	token := s.tokens.mint(downloadClaims{Attachment: id, Message: message, ExpiresAt: expires, Disposition: disposition})
	return DownloadAddress{URL: downloadRoute + id.String() + downloadQuery + token, ExpiresAt: expires}
}

func (s *Service) ChatAddress(id domain.AttachmentID, message domain.MessageID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address := s.MintAddress(id, domain.EntityID(message), disposition, now)
	return address.URL, address.ExpiresAt
}

type Download struct {
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
	if err != nil || claims.Attachment != id {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken)
	}
	opening, cancel := context.WithCancel(ctx)
	deadline := time.AfterFunc(downloadOpenTimeout, cancel)
	download, err := s.openDownload(opening, id, claims, cancel)
	if err != nil {
		deadline.Stop()
		cancel()
		return Download{}, err
	}
	if !deadline.Stop() {
		return Download{}, errors.Join(context.DeadlineExceeded, download.Body.Close())
	}
	return download, nil
}

func (s *Service) openDownload(ctx context.Context, id domain.AttachmentID, claims downloadClaims, cancel context.CancelFunc) (Download, error) {
	file, err := s.deps.Store.Repositories().Attachments.ServableChat(ctx, id, claims.Message)
	if errors.Is(err, repositories.ErrNotFound) {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedUnreferenced)
	}
	if err != nil {
		return Download{}, err
	}
	if claims.Disposition == domain.DispositionInline && file.Width == nil {
		return Download{}, s.refusedDownload(ctx, id, domain.RefusedToken)
	}
	select {
	case s.downloads <- struct{}{}:
	default:
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "download.busy", attachmentAttribute(id))
		return Download{}, ErrDownloadsBusy
	}
	release := sync.OnceFunc(func() {
		<-s.downloads
		cancel()
	})
	body, err := s.deps.Objects.Open(ctx, file.StorageKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		release()
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "download.object_missing", attachmentAttribute(id))
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

func (s *Service) refusedDownload(ctx context.Context, id domain.AttachmentID, reason domain.DownloadRefusal) error {
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "download.refused", attachmentAttribute(id), slog.String("reason", string(reason)))
	return ErrNotFound
}

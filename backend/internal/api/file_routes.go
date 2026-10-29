package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/files"
)

const (
	filePolicy         = "default-src 'none'; sandbox; frame-ancestors 'none'"
	fileCacheControl   = "private, no-store"
	fieldDisposition   = "query.disposition"
	dispositionParam   = "disposition"
	fileEventsPrefix   = "files.get"
	avatarEventsPrefix = "avatars.get"
)

type queryKey struct{}

type attachmentInput struct {
	ID          string `path:"id"`
	Disposition string `query:"disposition"`
}

type attachmentOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.DownloadAddress
}

type fileInput struct {
	ID    string `path:"id"`
	Token string `query:"t"`
}

type fileOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Policy             string `header:"Content-Security-Policy"`
	ResourcePolicy     string `header:"Cross-Origin-Resource-Policy"`
	ContentLength      string `header:"Content-Length"`
	Body               func(huma.Context)
}

func (r *router) registerFileRoutes() {
	address := operation("attachments-get", http.MethodGet, "/api/v1/attachments/{id}")
	address.Middlewares = append(address.Middlewares, r.keepQuery)
	address.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, address, AudienceUnverifiedOK, r.attachmentAddress)

	file := operation("files-get", http.MethodGet, "/api/v1/files/{id}")
	file.Responses = map[string]*huma.Response{"200": {
		Description: "The file",
		Content:     map[string]*huma.MediaType{"application/octet-stream": {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}},
	}}
	file.Errors = []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable}
	register(r, file, AudiencePublic, r.file)
}

func (r *router) keepQuery(ctx huma.Context, next func(huma.Context)) {
	address := ctx.URL()
	next(huma.WithValue(ctx, queryKey{}, address.Query()))
}

func (r *router) attachmentAddress(ctx context.Context, input *attachmentInput) (*attachmentOutput, error) {
	id, ok := domain.ParseUUID[domain.AttachmentID](input.ID)
	if !ok {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	query, found := ctx.Value(queryKey{}).(url.Values)
	if !found {
		panic("an attachment address ran without its query")
	}
	address, err := r.files.DownloadAddress(ctx, principalFrom(ctx).UserID, id, files.DispositionRequest{Values: query[dispositionParam]})
	switch {
	case errors.Is(err, files.ErrNotFound):
		return nil, NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, files.ErrInvalidDisposition):
		return nil, fieldError(ctx, fieldDisposition, domain.FieldInvalid)
	case err != nil:
		return nil, r.internal(ctx, "attachments.get.failed", err)
	}
	return &attachmentOutput{CacheControl: noStore, Body: schemas.DownloadAddress{URL: address.URL, ExpiresAt: address.ExpiresAt}}, nil
}

func (r *router) file(ctx context.Context, input *fileInput) (*fileOutput, error) {
	id, ok := domain.ParseUUID[domain.AttachmentID](input.ID)
	if !ok {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	download, err := r.files.Download(ctx, id, input.Token)
	switch {
	case errors.Is(err, files.ErrNotFound):
		return nil, NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, files.ErrDownloadsBusy):
		return nil, NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", "1")
	case err != nil:
		return nil, r.internal(ctx, "files.get.failed", err)
	}
	return &fileOutput{
		ContentType: download.MediaType, ContentDisposition: download.ContentDisposition, CacheControl: fileCacheControl,
		Policy: filePolicy, ResourcePolicy: "same-origin", ContentLength: strconv.FormatInt(download.SizeBytes, 10),
		Body: func(response huma.Context) {
			_, writer := humago.Unwrap(response)
			deadline := r.uploads.deadline(download.SizeBytes)
			if err := http.NewResponseController(writer).SetWriteDeadline(r.now().Add(deadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				r.logger.LogAttrs(ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
			}
			r.streamObject(ctx, response, download.Body, download.SizeBytes, fileEventsPrefix)
		},
	}, nil
}

var errObjectSizeMismatch = errors.New("the stored object is longer than its recorded size")

type trackedWriter struct {
	io.Writer
	failed error
}

func (w *trackedWriter) Write(data []byte) (int, error) {
	written, err := w.Writer.Write(data)
	if err != nil {
		w.failed = err
	}
	return written, err
}

func (r *router) streamObject(ctx context.Context, response huma.Context, body files.ObjectBody, size int64, events string) {
	defer func() {
		if err := body.Close(); err != nil {
			r.logger.LogAttrs(ctx, slog.LevelWarn, events+".close_failed", slog.String("kind", core.ErrorKind(err)))
		}
	}()
	response.SetStatus(http.StatusOK)
	writer := &trackedWriter{Writer: response.BodyWriter()}
	if _, err := io.CopyN(writer, body, size-1); err != nil {
		r.abortObject(ctx, writer, err, events)
	}
	last := make([]byte, 1)
	if _, err := io.ReadFull(body, last); err != nil {
		r.abortObject(ctx, writer, err, events)
	}
	overruns, err := body.Overruns()
	if err != nil {
		r.abortObject(ctx, writer, err, events)
	}
	if overruns {
		r.abortObject(ctx, writer, errObjectSizeMismatch, events)
	}
	if _, err := writer.Write(last); err != nil {
		r.abortObject(ctx, writer, err, events)
	}
}

func (r *router) abortObject(ctx context.Context, writer *trackedWriter, err error, events string) {
	switch {
	case writer.failed != nil || ctx.Err() != nil:
		r.logger.LogAttrs(ctx, slog.LevelDebug, events+".abandoned", slog.String("kind", core.ErrorKind(err)))
	case errors.Is(err, context.DeadlineExceeded):
		r.logger.LogAttrs(ctx, slog.LevelWarn, events+".timed_out")
	case errors.Is(err, errObjectSizeMismatch) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		r.logger.LogAttrs(ctx, slog.LevelError, events+".failed", slog.String("kind", "size_mismatch"))
	default:
		r.logger.LogAttrs(ctx, slog.LevelError, events+".failed", slog.String("kind", core.ErrorKind(err)))
	}
	panic(http.ErrAbortHandler)
}

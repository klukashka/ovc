package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/export"
)

type exportRequestInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ExportRequest
}

type exportOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Export
}

type currentExportOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.CurrentExport
}

type exportDownloadOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ExportDownload
}

type exportDeletedOutput struct{}

func (r *router) registerExports() {
	request := operation("exports-create", http.MethodPost, "/api/v1/exports")
	request.DefaultStatus = http.StatusAccepted
	register(r, request, AudienceUser, r.requestExport)
	register(r, operation("exports-current", http.MethodGet, "/api/v1/exports/current"), AudienceUser, r.currentExport)
	remove := operation("exports-current-delete", http.MethodDelete, "/api/v1/exports/current")
	remove.DefaultStatus = http.StatusNoContent
	register(r, remove, AudienceUser, r.deleteExport)
	register(r, operation("exports-current-download", http.MethodPost, "/api/v1/exports/current/download"), AudienceUser, r.mintExportDownload)
}

func (r *router) requestExport(ctx context.Context, input *exportRequestInput) (*exportOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	view, err := r.exports.Request(ctx, principalFrom(ctx), input.Body.Password)
	if err != nil {
		return nil, r.exportError(ctx, "export.request_failed", err)
	}
	return &exportOutput{CacheControl: noStore, Body: exportBody(*view.Export, nil)}, nil
}

func (r *router) currentExport(ctx context.Context, _ *struct{}) (*currentExportOutput, error) {
	view, err := r.exports.Current(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.exportError(ctx, "export.read_failed", err)
	}
	body := schemas.CurrentExport{NextAllowedAt: view.NextAllowedAt}
	if view.Export != nil {
		current := exportBody(*view.Export, view.NextAllowedAt)
		body.Export = &current
	}
	return &currentExportOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) deleteExport(ctx context.Context, _ *struct{}) (*exportDeletedOutput, error) {
	if err := r.exports.Delete(ctx, principalFrom(ctx)); err != nil {
		return nil, r.exportError(ctx, "export.delete_failed", err)
	}
	return &exportDeletedOutput{}, nil
}

func (r *router) mintExportDownload(ctx context.Context, _ *struct{}) (*exportDownloadOutput, error) {
	download, err := r.exports.MintDownload(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.exportError(ctx, "export.download_failed", err)
	}
	return &exportDownloadOutput{CacheControl: noStore, Body: schemas.ExportDownload{URL: download.URL.Reveal(), ExpiresAt: download.ExpiresAt}}, nil
}

func exportBody(row export.Summary, nextAllowed *time.Time) schemas.Export {
	body := schemas.Export{
		ID: row.ID.String(), State: row.State, Failure: row.Failure, RequestedAt: row.RequestedAt, BuiltAt: row.BuiltAt, ExpiresAt: row.ExpiresAt,
		SizeBytes: row.SizeBytes, NextAllowedAt: nextAllowed,
	}
	if row.State == domain.ExportBuilding {
		body.Progress = &schemas.ExportProgress{Done: row.ProgressDone, Total: row.ProgressTotal}
	}
	return body
}

func (r *router) exportError(ctx context.Context, event string, err error) error {
	var throttled *export.ThrottledError
	switch {
	case errors.Is(err, export.ErrCapacityFull):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", strconv.Itoa(int(domain.ExportCapacityRetryAfter.Seconds())))
	case errors.Is(err, export.ErrInProgress):
		return NewError(ctx, domain.ErrorExportInProgress)
	case errors.Is(err, export.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, export.ErrEmailNotVerified):
		return NewError(ctx, domain.ErrorEmailNotVerified)
	case errors.Is(err, export.ErrUnauthenticated), errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	}
	return r.flowError(ctx, event, err)
}

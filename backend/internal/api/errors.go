package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
)

var safeFieldPath = regexp.MustCompile(`^[A-Za-z0-9._\[\]-]{1,128}$`)

type Error struct {
	status int
	schemas.ErrorEnvelope
}

func (e *Error) GetStatus() int {
	return e.status
}

func (e *Error) Error() string {
	return string(e.Body.Code)
}

func NewError(ctx context.Context, code domain.ErrorCode) *Error {
	return &Error{status: code.Status(), ErrorEnvelope: schemas.NewErrorEnvelope(code, core.RequestID(ctx), nil)}
}

var installErrorModel = sync.OnceFunc(func() {
	huma.NewError = func(status int, _ string, errs ...error) huma.StatusError {
		return errorForStatus(context.Background(), status, errs)
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, _ string, errs ...error) huma.StatusError {
		return errorForStatus(ctx.Context(), status, errs)
	}
})

func errorForStatus(ctx context.Context, status int, errs []error) *Error {
	code := domain.ErrorCodeForStatus(status)
	var fields []schemas.FieldError
	if code == domain.ErrorValidationFailed {
		fields = fieldErrors(errs)
	}
	envelope := schemas.NewErrorEnvelope(code, core.RequestID(ctx), fields)
	return &Error{status: code.Status(), ErrorEnvelope: envelope}
}

func fieldErrors(errs []error) []schemas.FieldError {
	var fields []schemas.FieldError
	for _, err := range errs {
		var detailer huma.ErrorDetailer
		if !errors.As(err, &detailer) {
			continue
		}
		detail := detailer.ErrorDetail()
		if detail == nil || detail.Location == "" {
			continue
		}
		path, code := detail.Location, "invalid"
		if detail.Message == validation.MsgUnexpectedProperty {
			path, code = "body", "unexpected_property"
		}
		if !safeFieldPath.MatchString(path) {
			path = "body"
		}
		fields = append(fields, schemas.FieldError{Path: path, Code: code})
	}
	return fields
}

func writeError(ctx context.Context, w http.ResponseWriter, code domain.ErrorCode) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Content-Length")
	w.WriteHeader(code.Status())
	encoded, err := json.Marshal(schemas.NewErrorEnvelope(code, core.RequestID(ctx), nil))
	if err != nil {
		return
	}
	if _, err := w.Write(encoded); err != nil {
		return
	}
}

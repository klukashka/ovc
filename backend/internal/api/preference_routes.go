package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/notifications"
)

const (
	formMediaType      = "application/x-www-form-urlencoded"
	multipartMediaType = "multipart/form-data"
	unsubscribeField   = "body." + domain.UnsubscribeFormField
)

var errMalformedUnsubscribeForm = errors.New("the unsubscribe form is malformed")

type emailPreferencesOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.EmailPreferences
}

type emailPreferencesInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.EmailPreferences
}

type unsubscribeInput struct {
	Token string `query:"token"`
}

type unsubscribePreviewOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.UnsubscribePreview
}

type unsubscribedOutput struct {
	CacheControl string `header:"Cache-Control"`
}

func (r *router) registerPreferenceRoutes() {
	read := operation("users-me-email-preferences", http.MethodGet, "/api/v1/users/me/email-preferences")
	read.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, read, AudienceUnverifiedOK, r.emailPreferences)

	save := operation("users-me-email-preferences-update", http.MethodPut, "/api/v1/users/me/email-preferences")
	save.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, save, AudienceUnverifiedOK, r.saveEmailPreferences)

	preview := operation("unsubscribe-preview", http.MethodGet, "/api/v1/unsubscribe/preview")
	preview.Errors = errorsOf(http.StatusTooManyRequests)
	register(r, preview, AudiencePublic, r.previewUnsubscribe)

	unsubscribe := operation("unsubscribe", http.MethodPost, "/api/v1/unsubscribe")
	unsubscribe.DefaultStatus = http.StatusNoContent
	unsubscribe.MaxBodyBytes = domain.UnsubscribeFormMaxBytes
	unsubscribe.Middlewares = append(unsubscribe.Middlewares, r.keepUnsubscribeRequest)
	unsubscribe.Errors = errorsOf(http.StatusTooManyRequests)
	register(r, unsubscribe, AudiencePublic, r.unsubscribe)
	oneClick := &huma.Schema{
		Type: huma.TypeObject, Required: []string{domain.UnsubscribeFormField}, AdditionalProperties: false,
		Properties: map[string]*huma.Schema{domain.UnsubscribeFormField: {Type: huma.TypeString, Pattern: "^" + domain.UnsubscribeFormValue + "$"}},
	}
	r.api.OpenAPI().Paths[unsubscribe.Path].Post.RequestBody = &huma.RequestBody{
		Required: false,
		Content:  map[string]*huma.MediaType{formMediaType: {Schema: oneClick}, multipartMediaType: {Schema: oneClick}},
	}
}

func (r *router) emailPreferences(ctx context.Context, _ *struct{}) (*emailPreferencesOutput, error) {
	listed, err := r.preferences.Of(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.flowError(ctx, "email_preferences.read_failed", err)
	}
	return &emailPreferencesOutput{CacheControl: noStore, Body: preferencesBody(listed)}, nil
}

func (r *router) saveEmailPreferences(ctx context.Context, input *emailPreferencesInput) (*emailPreferencesOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	changes := make([]notifications.EmailPreference, 0, len(input.Body.Groups))
	for _, change := range input.Body.Groups {
		changes = append(changes, notifications.EmailPreference{Group: change.Group, Enabled: change.Enabled})
	}
	saved, err := r.preferences.Save(ctx, principalFrom(ctx), changes)
	if err != nil {
		return nil, r.flowError(ctx, "email_preferences.update_failed", err)
	}
	return &emailPreferencesOutput{CacheControl: noStore, Body: preferencesBody(saved)}, nil
}

func (r *router) previewUnsubscribe(ctx context.Context, input *unsubscribeInput) (*unsubscribePreviewOutput, error) {
	group, err := r.preferences.PreviewUnsubscribe(ctx, input.Token)
	if err != nil {
		return nil, r.flowError(ctx, "unsubscribe.preview_failed", err)
	}
	return &unsubscribePreviewOutput{CacheControl: noStore, Body: schemas.UnsubscribePreview{Group: group}}, nil
}

func (r *router) unsubscribe(ctx context.Context, input *unsubscribeInput) (*unsubscribedOutput, error) {
	request, found := ctx.Value(rawRequestKey{}).(*http.Request)
	if !found {
		panic("an unsubscribe ran without its raw request")
	}
	if err := oneClickBody(ctx, request); err != nil {
		return nil, err
	}
	if err := r.preferences.Unsubscribe(ctx, input.Token); err != nil {
		return nil, r.flowError(ctx, "unsubscribe.failed", err)
	}
	return &unsubscribedOutput{CacheControl: noStore}, nil
}

func (r *router) keepUnsubscribeRequest(ctx huma.Context, next func(huma.Context)) {
	request, writer := humago.Unwrap(ctx)
	request.Body = http.MaxBytesReader(writer, request.Body, domain.UnsubscribeFormMaxBytes)
	next(huma.WithValue(ctx, rawRequestKey{}, request))
}

func oneClickBody(ctx context.Context, request *http.Request) error {
	body, err := io.ReadAll(request.Body)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return NewError(ctx, domain.ErrorPayloadTooLarge)
	case err != nil:
		return NewError(ctx, domain.ErrorBadRequest)
	}
	contentType := request.Header.Get("Content-Type")
	if contentType == "" && len(body) == 0 {
		return nil
	}
	mediaType, parameters, err := mime.ParseMediaType(contentType)
	if err != nil {
		return NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	var fields url.Values
	switch mediaType {
	case formMediaType:
		fields, err = url.ParseQuery(string(body))
	case multipartMediaType:
		fields, err = multipartFields(body, parameters["boundary"])
	default:
		return NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	if err != nil || len(fields) != 1 || len(fields[domain.UnsubscribeFormField]) != 1 || fields.Get(domain.UnsubscribeFormField) != domain.UnsubscribeFormValue {
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(
			domain.ErrorValidationFailed, core.RequestID(ctx), []schemas.FieldError{{Path: unsubscribeField, Code: string(domain.FieldInvalid)}},
		)}
	}
	return nil
}

func multipartFields(body []byte, boundary string) (url.Values, error) {
	if boundary == "" {
		return nil, errMalformedUnsubscribeForm
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	fields := url.Values{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return fields, nil
		}
		if err != nil {
			return nil, errors.Join(errMalformedUnsubscribeForm, err)
		}
		if part.FileName() != "" || part.FormName() == "" {
			return nil, errMalformedUnsubscribeForm
		}
		value, err := io.ReadAll(part)
		if err != nil {
			return nil, errors.Join(errMalformedUnsubscribeForm, err)
		}
		fields.Add(part.FormName(), string(value))
	}
}

func preferencesBody(listed []notifications.EmailPreference) schemas.EmailPreferences {
	body := schemas.EmailPreferences{Groups: make([]schemas.EmailPreference, 0, len(listed))}
	for _, preference := range listed {
		body.Groups = append(body.Groups, schemas.EmailPreference{Group: preference.Group, Enabled: preference.Enabled})
	}
	return body
}

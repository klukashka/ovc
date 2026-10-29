package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/files"
)

const (
	avatarBodyBytes     = domain.AvatarMaxBytes + domain.UploadOverheadBytes
	avatarReadDeadline  = 60 * time.Second
	uploadBodyBytes     = domain.ChatMaxBytes + domain.UploadOverheadBytes
	uploadReadDeadline  = domain.UploadDeadlineCeiling
	uploadWriteDeadline = 65 * time.Second
	avatarCacheControl  = "private, max-age=86400"
	avatarPolicy        = "default-src 'none'; sandbox"
)

type (
	rawRequestKey    struct{}
	rawControllerKey struct{}
)

var (
	errExtraUploadPart = errors.New("the upload has more than one part")
	errMalformedUpload = errors.New("the multipart body is malformed")
)

type profilePatchInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ProfilePatch
}

type deleteAccountInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.DeleteAccountRequest
}

type uploadInput struct {
	Context string `query:"context" enum:"avatar,chat" required:"true"`
	Scope   string `query:"scope" maxLength:"100"`
}

type uploadOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.UploadedFile
}

type avatarInput struct {
	ID string `path:"id"`
}

const avatarReadTimeout = 30 * time.Second

type avatarOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	Policy             string `header:"Content-Security-Policy"`
	ResourcePolicy     string `header:"Cross-Origin-Resource-Policy"`
	ContentLength      string `header:"Content-Length"`
	Body               func(huma.Context)
}

func (r *router) registerProfileRoutes() {
	patch := operation("users-me-update", http.MethodPatch, "/api/v1/users/me")
	patch.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, patch, AudienceUnverifiedOK, r.updateMe)

	remove := operation("users-me-delete", http.MethodDelete, "/api/v1/users/me")
	remove.DefaultStatus = http.StatusNoContent
	remove.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests)
	register(r, remove, AudienceUnverifiedOK, r.deleteMe)

	upload := operation("uploads-create", http.MethodPost, "/api/v1/uploads")
	upload.DefaultStatus = http.StatusCreated
	upload.MaxBodyBytes = uploadBodyBytes
	upload.BodyReadTimeout = uploadReadDeadline
	upload.Middlewares = append(upload.Middlewares, r.keepRawRequest)
	upload.Errors = errorsOf(
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge,
		http.StatusUnsupportedMediaType, http.StatusTooManyRequests, http.StatusInsufficientStorage,
	)
	register(r, upload, AudienceUnverifiedOK, r.upload)
	r.api.OpenAPI().Paths[upload.Path].Post.RequestBody = &huma.RequestBody{
		Required: true,
		Content: map[string]*huma.MediaType{"multipart/form-data": {Schema: &huma.Schema{
			Type: huma.TypeObject, Required: []string{"file"},
			Properties: map[string]*huma.Schema{"file": {Type: huma.TypeString, Format: "binary"}},
		}}},
	}

	avatar := operation("avatars-get", http.MethodGet, "/api/v1/avatars/{id}")
	avatar.Responses = map[string]*huma.Response{"200": {
		Description: "The avatar image",
		Content: map[string]*huma.MediaType{
			"image/png":  {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}},
			"image/jpeg": {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}},
		},
	}}
	avatar.Errors = []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable}
	register(r, avatar, AudiencePublic, r.avatar)
}

func (r *router) keepRawRequest(ctx huma.Context, next func(huma.Context)) {
	request, writer := humago.Unwrap(ctx)
	controller := http.NewResponseController(writer)
	now := r.now()
	readDeadline, writeDeadline, bodyBytes := avatarReadDeadline, uploadWriteDeadline, int64(avatarBodyBytes)
	if domain.AttachmentContext(ctx.Query("context")) == domain.ContextChat {
		principal := principalFrom(ctx.Context())
		if principal.State != domain.AccountActive {
			ctx.SetHeader("Connection", "close")
			r.writeError(ctx, domain.ErrorEmailNotVerified)
			return
		}
		if request.ContentLength > uploadBodyBytes {
			r.files.RecordRejection(ctx.Context(), principal.UserID, domain.RejectedTooLarge)
			ctx.SetHeader("Connection", "close")
			r.writeError(ctx, domain.ErrorPayloadTooLarge)
			return
		}
		readDeadline = r.uploads.deadline(domain.ChatMaxBytes)
		writeDeadline = readDeadline + chatWriteDeadline
		bodyBytes = uploadBodyBytes
	}
	if err := controller.SetReadDeadline(now.Add(readDeadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		r.logger.LogAttrs(ctx.Context(), slog.LevelWarn, "http.deadline.failed", slog.String("kind", "read"))
	}
	if err := controller.SetWriteDeadline(now.Add(writeDeadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		r.logger.LogAttrs(ctx.Context(), slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
	}
	request.Body = http.MaxBytesReader(writer, request.Body, bodyBytes)
	next(huma.WithValue(huma.WithValue(ctx, rawRequestKey{}, request), rawControllerKey{}, controller))
}

func (r *router) updateMe(ctx context.Context, input *profilePatchInput) (*meOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	change := accounts.ProfileChange{
		Name: input.Body.Name, Language: input.Body.Language, Timezone: input.Body.Timezone, Theme: input.Body.Theme,
		Bio: input.Body.Bio, Subjects: input.Body.Subjects,
	}
	if input.Body.AvatarID.Set {
		change.Avatar.Set = true
		if input.Body.AvatarID.Value != nil {
			id, ok := domain.ParseUUID[domain.AttachmentID](*input.Body.AvatarID.Value)
			if !ok {
				return nil, r.flowError(ctx, "users.me.update_failed", &accounts.ValidationError{
					Fields: []accounts.FieldProblem{{Path: "body.avatar_id", Code: domain.FieldInvalid}},
				})
			}
			change.Avatar.ID = &id
		}
	}
	profile, err := r.auth.Accounts.UpdateProfile(ctx, principalFrom(ctx), change)
	if err != nil {
		return nil, r.flowError(ctx, "users.me.update_failed", err)
	}
	return &meOutput{CacheControl: noStore, Body: meBody(profile)}, nil
}

func (r *router) deleteMe(ctx context.Context, input *deleteAccountInput) (*signOutOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	if err := r.auth.Accounts.DeleteAccount(ctx, principalFrom(ctx), input.Body.Password); err != nil {
		return nil, r.flowError(ctx, "users.me.delete_failed", err)
	}
	return &signOutOutput{SetCookie: clearedCookie(), CacheControl: noStore}, nil
}

func (r *router) upload(ctx context.Context, input *uploadInput) (*uploadOutput, error) {
	request, found := ctx.Value(rawRequestKey{}).(*http.Request)
	if !found {
		panic("an upload ran without its raw request")
	}
	if domain.AttachmentContext(input.Context) == domain.ContextChat {
		return r.uploadChat(ctx, input, request)
	}
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || parameters["boundary"] == "" {
		return nil, NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	reader := multipart.NewReader(request.Body, parameters["boundary"])
	part, err := reader.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, uploadFieldError(ctx)
	}
	if err != nil {
		return nil, r.uploadError(ctx, errors.Join(errMalformedUpload, err))
	}
	if part.FormName() != "file" || part.FileName() == "" {
		return nil, uploadFieldError(ctx)
	}
	read := func() ([]byte, error) {
		data, err := io.ReadAll(io.LimitReader(part, domain.AvatarMaxBytes+1))
		if err != nil {
			return nil, errors.Join(errMalformedUpload, err)
		}
		if len(data) > domain.AvatarMaxBytes {
			return data, nil
		}
		_, err = reader.NextPart()
		switch {
		case err == nil:
			return nil, errExtraUploadPart
		case !errors.Is(err, io.EOF):
			return nil, errors.Join(errMalformedUpload, err)
		}
		return data, nil
	}
	upload, err := r.files.UploadAvatar(ctx, principalFrom(ctx), path.Base(part.FileName()), read)
	if errors.Is(err, errExtraUploadPart) {
		return nil, uploadFieldError(ctx)
	}
	if err != nil {
		return nil, r.uploadError(ctx, err)
	}
	return &uploadOutput{CacheControl: noStore, Body: schemas.UploadedFile{
		ID: upload.ID.String(), Filename: upload.Filename, MediaType: upload.MediaType, SizeBytes: upload.SizeBytes,
	}}, nil
}

func uploadFieldError(ctx context.Context) error {
	return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(
		domain.ErrorValidationFailed, core.RequestID(ctx), []schemas.FieldError{{Path: "body.file", Code: string(domain.FieldRequired)}},
	)}
}

func (r *router) uploadError(ctx context.Context, err error) error {
	var tooLarge *http.MaxBytesError
	var throttled *files.ThrottledError
	switch {
	case errors.Is(err, files.ErrTooLarge), errors.As(err, &tooLarge):
		return NewError(ctx, domain.ErrorPayloadTooLarge)
	case errors.Is(err, files.ErrUnsupported):
		return NewError(ctx, domain.ErrorUnsupportedMediaType)
	case errors.Is(err, files.ErrPurgedDuringUpload), errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, files.ErrDecodeBusy):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", "5")
	case errors.Is(err, files.ErrBusy):
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", "60")
	case errors.As(err, &throttled):
		return r.flowError(ctx, "uploads.create.failed", &accounts.ThrottledError{RetryAfter: throttled.RetryAfter})
	case errors.Is(err, errMalformedUpload), errors.Is(err, multipart.ErrMessageTooLarge), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return NewError(ctx, domain.ErrorBadRequest)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorBadRequest)
	}
	return r.internal(ctx, "uploads.create.failed", err)
}

func (r *router) avatar(ctx context.Context, input *avatarInput) (*avatarOutput, error) {
	id, ok := domain.ParseUUID[domain.AttachmentID](input.ID)
	if !ok {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	reading, cancel := context.WithTimeout(ctx, avatarReadTimeout)
	avatar, err := r.files.Avatar(reading, id)
	if err != nil {
		cancel()
		return nil, r.avatarRefusal(ctx, err)
	}
	return &avatarOutput{
		ContentType: avatar.MediaType, ContentDisposition: "inline", CacheControl: avatarCacheControl,
		Policy: avatarPolicy, ResourcePolicy: "same-origin", ContentLength: strconv.FormatInt(avatar.SizeBytes, 10),
		Body: func(response huma.Context) {
			defer cancel()
			r.streamObject(ctx, response, avatar.Body, avatar.SizeBytes, avatarEventsPrefix)
		},
	}, nil
}

func (r *router) avatarRefusal(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, files.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, files.ErrAvatarReadsBusy):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", "1")
	}
	return r.internal(ctx, "avatars.get.failed", err)
}

func meBody(profile accounts.Profile) schemas.Me {
	body := schemas.Me{
		ID: profile.ID.String(), Role: profile.Role, State: profile.State, Email: profile.Email, EmailVerified: profile.EmailVerified,
		Name: profile.Name, Language: profile.Language, Timezone: profile.Timezone, Theme: profile.Theme,
		ReclaimPending: profile.ReclaimPending,
	}
	if profile.AvatarID != nil {
		id := profile.AvatarID.String()
		body.AvatarID = &id
	}
	if profile.Tutor != nil {
		bio := profile.Tutor.Bio
		subjects := append([]string{}, profile.Tutor.Subjects...)
		body.Bio, body.Subjects = &bio, &subjects
	}
	return body
}

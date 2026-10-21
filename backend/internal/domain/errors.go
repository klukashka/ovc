package domain

import "net/http"

type ErrorCode string

const (
	ErrorBadRequest              ErrorCode = "bad_request"
	ErrorUnauthenticated         ErrorCode = "unauthenticated"
	ErrorTokenExpired            ErrorCode = "token_expired"
	ErrorInvalidSignature        ErrorCode = "invalid_signature"
	ErrorSignInRefused           ErrorCode = "invalid_credentials"
	ErrorEmailNotVerified        ErrorCode = "email_not_verified"
	ErrorAccountBlocked          ErrorCode = "account_blocked"
	ErrorForbidden               ErrorCode = "forbidden"
	ErrorNotFound                ErrorCode = "not_found"
	ErrorMethodNotAllowed        ErrorCode = "method_not_allowed"
	ErrorConflict                ErrorCode = "conflict"
	ErrorLessonOverlap           ErrorCode = "lesson_overlap"
	ErrorLessonNotLive           ErrorCode = "lesson_not_live"
	ErrorOutsideJoinWindow       ErrorCode = "outside_join_window"
	ErrorNotEditable             ErrorCode = "not_editable"
	ErrorAnswerFrozen            ErrorCode = "answer_frozen"
	ErrorAlreadyLinked           ErrorCode = "already_linked"
	ErrorStudentExists           ErrorCode = "student_exists"
	ErrorRelationshipNotActive   ErrorCode = "relationship_not_active"
	ErrorAudienceNotActive       ErrorCode = "audience_not_active"
	ErrorRoleMismatch            ErrorCode = "role_mismatch"
	ErrorInvitationInvalid       ErrorCode = "invitation_invalid"
	ErrorInvitationEmailMismatch ErrorCode = "invitation_email_mismatch"
	ErrorReturnNotAllowed        ErrorCode = "return_not_allowed"
	ErrorIssueHasSubmissions     ErrorCode = "issue_has_submissions"
	ErrorChannelReadOnly         ErrorCode = "channel_read_only"
	ErrorEditWindowPassed        ErrorCode = "edit_window_passed"
	ErrorGone                    ErrorCode = "gone"
	ErrorPayloadTooLarge         ErrorCode = "payload_too_large"
	ErrorUnsupportedMediaType    ErrorCode = "unsupported_media_type"
	ErrorValidationFailed        ErrorCode = "validation_failed"
	ErrorLessonInPast            ErrorCode = "lesson_in_past"
	ErrorTutorCodeInvalid        ErrorCode = "tutor_code_invalid"
	ErrorTokenInvalid            ErrorCode = "token_invalid"
	ErrorRateLimited             ErrorCode = "rate_limited"
	ErrorTutorUnavailable        ErrorCode = "tutor_unavailable"
	ErrorInternal                ErrorCode = "internal"
	ErrorProviderUnavailable     ErrorCode = "provider_unavailable"
	ErrorServiceUnavailable      ErrorCode = "service_unavailable"
	ErrorQuotaExceeded           ErrorCode = "quota_exceeded"
)

type errorDefinition struct {
	code    ErrorCode
	status  int
	message string
}

var errorCatalogue = []errorDefinition{
	{ErrorBadRequest, http.StatusBadRequest, "The request is malformed."},
	{ErrorUnauthenticated, http.StatusUnauthorized, "Authentication is required."},
	{ErrorTokenExpired, http.StatusUnauthorized, "The access token has expired."},
	{ErrorInvalidSignature, http.StatusUnauthorized, "The request signature is invalid."},
	{ErrorSignInRefused, http.StatusUnauthorized, "The email or password is incorrect."},
	{ErrorEmailNotVerified, http.StatusForbidden, "The email address is not confirmed yet."},
	{ErrorAccountBlocked, http.StatusForbidden, "The account is blocked."},
	{ErrorForbidden, http.StatusForbidden, "This action is not allowed."},
	{ErrorNotFound, http.StatusNotFound, "The requested resource does not exist."},
	{ErrorMethodNotAllowed, http.StatusMethodNotAllowed, "This method is not allowed for the resource."},
	{ErrorConflict, http.StatusConflict, "The resource has changed; reload it and try again."},
	{ErrorLessonOverlap, http.StatusConflict, "The lesson overlaps another lesson."},
	{ErrorLessonNotLive, http.StatusConflict, "The lesson is not live."},
	{ErrorOutsideJoinWindow, http.StatusConflict, "The lesson cannot be joined at this time."},
	{ErrorNotEditable, http.StatusConflict, "The resource can no longer be edited."},
	{ErrorAnswerFrozen, http.StatusConflict, "The answer can no longer be changed."},
	{ErrorAlreadyLinked, http.StatusConflict, "The student is already linked."},
	{ErrorStudentExists, http.StatusConflict, "A student with this email already exists."},
	{ErrorRelationshipNotActive, http.StatusConflict, "The relationship is not active."},
	{ErrorAudienceNotActive, http.StatusConflict, "The audience is not active."},
	{ErrorRoleMismatch, http.StatusConflict, "This action is not available for the account's role."},
	{ErrorInvitationInvalid, http.StatusConflict, "The invitation is no longer valid."},
	{ErrorInvitationEmailMismatch, http.StatusConflict, "The invitation is for a different email address."},
	{ErrorReturnNotAllowed, http.StatusConflict, "The work cannot be returned for revision."},
	{ErrorIssueHasSubmissions, http.StatusConflict, "The assignment already has submissions."},
	{ErrorChannelReadOnly, http.StatusConflict, "The conversation is read-only."},
	{ErrorEditWindowPassed, http.StatusConflict, "The message can no longer be edited."},
	{ErrorGone, http.StatusGone, "The resource no longer exists."},
	{ErrorPayloadTooLarge, http.StatusRequestEntityTooLarge, "The request body is too large."},
	{ErrorUnsupportedMediaType, http.StatusUnsupportedMediaType, "The request content type is not supported."},
	{ErrorValidationFailed, http.StatusUnprocessableEntity, "The request failed validation."},
	{ErrorLessonInPast, http.StatusUnprocessableEntity, "The lesson time is in the past."},
	{ErrorTutorCodeInvalid, http.StatusUnprocessableEntity, "The tutor code is not valid."},
	{ErrorTokenInvalid, http.StatusUnprocessableEntity, "The link is invalid or has expired."},
	{ErrorRateLimited, http.StatusTooManyRequests, "Too many requests; try again later."},
	{ErrorTutorUnavailable, http.StatusTooManyRequests, "The tutor cannot accept requests right now."},
	{ErrorInternal, http.StatusInternalServerError, "An internal error occurred."},
	{ErrorProviderUnavailable, http.StatusServiceUnavailable, "The conferencing service is unavailable."},
	{ErrorServiceUnavailable, http.StatusServiceUnavailable, "The service is temporarily unavailable."},
	{ErrorQuotaExceeded, http.StatusInsufficientStorage, "The storage quota is exceeded."},
}

func AllErrorCodes() []ErrorCode {
	codes := make([]ErrorCode, 0, len(errorCatalogue))
	for _, definition := range errorCatalogue {
		codes = append(codes, definition.code)
	}
	return codes
}

func (c ErrorCode) definition() errorDefinition {
	for _, definition := range errorCatalogue {
		if definition.code == c {
			return definition
		}
	}
	if c == ErrorInternal {
		panic("the error catalogue has no internal entry")
	}
	return ErrorInternal.definition()
}

func (c ErrorCode) Listed() ErrorCode {
	return c.definition().code
}

func (c ErrorCode) Message() string {
	return c.definition().message
}

func (c ErrorCode) Status() int {
	return c.definition().status
}

func ErrorCodeForStatus(status int) ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return ErrorBadRequest
	case http.StatusNotFound:
		return ErrorNotFound
	case http.StatusMethodNotAllowed:
		return ErrorMethodNotAllowed
	case http.StatusUnprocessableEntity:
		return ErrorValidationFailed
	case http.StatusRequestEntityTooLarge:
		return ErrorPayloadTooLarge
	case http.StatusUnsupportedMediaType:
		return ErrorUnsupportedMediaType
	case http.StatusServiceUnavailable:
		return ErrorServiceUnavailable
	}
	if status >= 400 && status < 500 {
		return ErrorBadRequest
	}
	return ErrorInternal
}

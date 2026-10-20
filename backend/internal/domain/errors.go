package domain

import "net/http"

type ErrorCode string

const (
	ErrorBadRequest           ErrorCode = "bad_request"
	ErrorNotFound             ErrorCode = "not_found"
	ErrorMethodNotAllowed     ErrorCode = "method_not_allowed"
	ErrorValidationFailed     ErrorCode = "validation_failed"
	ErrorPayloadTooLarge      ErrorCode = "payload_too_large"
	ErrorUnsupportedMediaType ErrorCode = "unsupported_media_type"
	ErrorServiceUnavailable   ErrorCode = "service_unavailable"
	ErrorInternal             ErrorCode = "internal"
)

func (c ErrorCode) Message() string {
	switch c {
	case ErrorBadRequest:
		return "The request is malformed."
	case ErrorNotFound:
		return "The requested resource does not exist."
	case ErrorMethodNotAllowed:
		return "This method is not allowed for the resource."
	case ErrorValidationFailed:
		return "The request failed validation."
	case ErrorPayloadTooLarge:
		return "The request body is too large."
	case ErrorUnsupportedMediaType:
		return "The request content type is not supported."
	case ErrorServiceUnavailable:
		return "The service is temporarily unavailable."
	case ErrorInternal:
		return "An internal error occurred."
	}
	return "An internal error occurred."
}

func (c ErrorCode) Status() int {
	switch c {
	case ErrorBadRequest:
		return http.StatusBadRequest
	case ErrorNotFound:
		return http.StatusNotFound
	case ErrorMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case ErrorValidationFailed:
		return http.StatusUnprocessableEntity
	case ErrorPayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrorUnsupportedMediaType:
		return http.StatusUnsupportedMediaType
	case ErrorServiceUnavailable:
		return http.StatusServiceUnavailable
	case ErrorInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
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

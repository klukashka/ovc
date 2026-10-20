package domain_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

func TestEveryErrorStatusMapsToACodeWithThatFamily(t *testing.T) {
	for status := 400; status < 600; status++ {
		code := domain.ErrorCodeForStatus(status)

		mapped := code.Status()

		if status < 500 && (mapped < 400 || mapped >= 500) {
			t.Fatalf("client status %d maps to %s with status %d", status, code, mapped)
		}
		if status >= 500 && mapped < 500 {
			t.Fatalf("server status %d maps to %s with status %d", status, code, mapped)
		}
		if code.Message() == "" {
			t.Fatalf("%s has no message", code)
		}
	}
}

func TestKnownStatusesKeepTheirCodes(t *testing.T) {
	expected := map[int]domain.ErrorCode{
		http.StatusBadRequest:            domain.ErrorBadRequest,
		http.StatusNotFound:              domain.ErrorNotFound,
		http.StatusMethodNotAllowed:      domain.ErrorMethodNotAllowed,
		http.StatusUnprocessableEntity:   domain.ErrorValidationFailed,
		http.StatusRequestEntityTooLarge: domain.ErrorPayloadTooLarge,
		http.StatusUnsupportedMediaType:  domain.ErrorUnsupportedMediaType,
		http.StatusServiceUnavailable:    domain.ErrorServiceUnavailable,
		http.StatusInternalServerError:   domain.ErrorInternal,
		http.StatusNotAcceptable:         domain.ErrorBadRequest,
		http.StatusBadGateway:            domain.ErrorInternal,
	}

	for status, code := range expected {
		got := domain.ErrorCodeForStatus(status)

		if got != code || (status != http.StatusNotAcceptable && status != http.StatusBadGateway && got.Status() != status) {
			t.Fatalf("status %d: got %s (%d), want %s", status, got, got.Status(), code)
		}
	}
}

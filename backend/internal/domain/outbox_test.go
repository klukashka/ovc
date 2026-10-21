package domain_test

import (
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestOutboxRetryDelayDoublesWithJitterBetween30SecondsAndOneHour(t *testing.T) {
	cases := []struct {
		attempts int32
		fraction float64
		want     time.Duration
	}{
		{1, 0, 30 * time.Second},
		{1, 1, 45 * time.Second},
		{2, 0, time.Minute},
		{3, 0.5, 2*time.Minute + 30*time.Second},
		{7, 0, 32 * time.Minute},
		{7, 1, 48 * time.Minute},
		{8, 0, time.Hour},
		{40, 0.3, time.Hour},
		{1, -5, 30 * time.Second},
		{1, 7, 45 * time.Second},
	}

	for _, testCase := range cases {
		got := domain.OutboxRetryDelay(testCase.attempts, testCase.fraction)

		if got != testCase.want {
			t.Errorf("OutboxRetryDelay(%d, %v) = %v, want %v", testCase.attempts, testCase.fraction, got, testCase.want)
		}
	}
}

func TestParseUUIDAcceptsOnlyTheCanonicalForm(t *testing.T) {
	cases := map[string]bool{
		"0f8fad5b-d9cb-469f-a165-70867728950e":  true,
		"0F8FAD5B-D9CB-469F-A165-70867728950E":  true,
		"0f8fad5bd9cb469fa16570867728950e":      false,
		"0f8fad5b-d9cb-469f-a165-70867728950":   false,
		"0f8fad5b-d9cb-469f-a165-70867728950e0": false,
		"0f8fad5b+d9cb-469f-a165-70867728950e":  false,
		"0f8fad5b-d9cb-469f-a165-70867728950g":  false,
	}

	for text, want := range cases {
		id, got := domain.ParseUUID[domain.AttachmentID](text)

		if got != want || (got && !strings.EqualFold(id.String(), text)) {
			t.Errorf("ParseUUID(%q) = %v %s", text, got, id)
		}
	}
}

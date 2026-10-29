package storage

import (
	"errors"
	"testing"
)

func TestServedLengthRefusesInconsistentContentRanges(t *testing.T) {
	cases := []struct {
		name         string
		contentRange string
		offset       int64
		requested    int
	}{
		{name: "an inverted range", contentRange: "bytes 100-49/50", offset: 100, requested: 10},
		{name: "a range starting at the total", contentRange: "bytes 50-59/50", offset: 50, requested: 10},
		{name: "a range starting past the total", contentRange: "bytes 100-109/50", offset: 100, requested: 10},
		{name: "a range starting elsewhere", contentRange: "bytes 10-19/50", offset: 20, requested: 10},
		{name: "a range ending past the total", contentRange: "bytes 40-59/50", offset: 40, requested: 20},
		{name: "a range shorter than available", contentRange: "bytes 0-4/50", offset: 0, requested: 10},
		{name: "an unknown total", contentRange: "bytes 0-9/*", offset: 0, requested: 10},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contentRange := testCase.contentRange

			served, err := servedLength(&contentRange, testCase.offset, testCase.requested)

			if served != 0 || !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("served %d, %v", served, err)
			}
		})
	}
}

func TestServedLengthAcceptsTheRequestedOrRemainingRange(t *testing.T) {
	contentRange := "bytes 40-49/50"

	served, err := servedLength(&contentRange, 40, 20)

	if served != 10 || err != nil {
		t.Fatalf("served %d, %v", served, err)
	}
}

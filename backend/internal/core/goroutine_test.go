package core_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"ovc/backend/internal/core"
)

func TestRunReportsAPanicAsAnErrorWithoutItsValue(t *testing.T) {
	var out safeBuffer
	logging := core.NewLogging(&out, slog.LevelInfo, core.LogFormatJSON)

	result := <-core.Run(context.Background(), logging.Logger, "exploding", func() error {
		panic("sentinel-panic-value")
	})
	closeLogging(t, logging)

	if !errors.Is(result, core.ErrPanicked) {
		t.Fatalf("expected ErrPanicked, got %v", result)
	}
	logged := out.String()
	if !strings.Contains(logged, "panic.recovered") || strings.Contains(logged, "sentinel-panic-value") {
		t.Fatalf("expected a panic record without the value, got %s", logged)
	}
}

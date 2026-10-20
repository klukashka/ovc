package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
)

func Go(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context)) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				LogPanic(ctx, logger, name, fmt.Sprintf("%T", recovered))
			}
		}()
		fn(ctx)
	}()
}

var ErrPanicked = errors.New("the goroutine panicked")

func Run(ctx context.Context, logger *slog.Logger, name string, fn func() error) <-chan error {
	result := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				LogPanic(ctx, logger, name, fmt.Sprintf("%T", recovered))
				result <- ErrPanicked
			}
		}()
		result <- fn()
	}()
	return result
}

func LogPanic(ctx context.Context, logger *slog.Logger, name string, panicType string) {
	logger.LogAttrs(ctx, slog.LevelError, "panic.recovered",
		slog.String("goroutine", name),
		slog.String("panic_type", panicType),
		slog.String("stack", string(debug.Stack())),
	)
}

func NewRandomID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	return hex.EncodeToString(buffer)
}

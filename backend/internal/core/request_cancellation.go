package core

import (
	"context"
	"errors"
	"sync"
	"time"
)

type requestReadStateKey struct{}

type RequestReadState struct {
	mutex       sync.Mutex
	deadline    time.Time
	serverAbort bool
}

func WithRequestReadState(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestReadStateKey{}, &RequestReadState{})
}

func SetRequestReadDeadline(ctx context.Context, deadline time.Time, set func(time.Time) error) error {
	state, ok := ctx.Value(requestReadStateKey{}).(*RequestReadState)
	if !ok {
		return set(deadline)
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	cancelledBeforeChange := ctx.Err() != nil
	if err := set(deadline); err != nil {
		return err
	}
	if !cancelledBeforeChange && ctx.Err() == nil {
		state.deadline = deadline
	}
	return nil
}

func MarkRequestReadAborted(ctx context.Context) {
	if state, ok := ctx.Value(requestReadStateKey{}).(*RequestReadState); ok {
		state.mutex.Lock()
		defer state.mutex.Unlock()
		state.serverAbort = true
	}
}

func ClientWentAway(ctx context.Context, now time.Time) bool {
	if !errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	state, ok := ctx.Value(requestReadStateKey{}).(*RequestReadState)
	if !ok {
		return true
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	return !state.serverAbort && (state.deadline.IsZero() || now.Before(state.deadline))
}

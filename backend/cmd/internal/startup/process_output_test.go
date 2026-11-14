package startup_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

type processOutput struct {
	mu      sync.Mutex
	text    bytes.Buffer
	written chan struct{}
}

func newProcessOutput() *processOutput {
	return &processOutput{written: make(chan struct{})}
}

func (o *processOutput) Write(chunk []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	count, err := o.text.Write(chunk)
	close(o.written)
	o.written = make(chan struct{})
	return count, err
}

func (o *processOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

func (o *processOutput) logged(event string) (bool, <-chan struct{}) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.Contains(o.text.String(), `"msg":"`+event+`"`), o.written
}

func waitForLogged(t *testing.T, ctx context.Context, output *processOutput, event string) {
	t.Helper()
	for {
		found, written := output.logged(event)
		if found {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no %s before the deadline", event)
		case <-written:
		}
	}
}

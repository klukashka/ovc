package testmail

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"ovc/backend/internal/adapters/mail"
)

type heldSend struct {
	recipient string
	release   chan struct{}
	failure   error
}

type Sender struct {
	next    mail.Mailer
	mutex   sync.Mutex
	paused  bool
	held    []*heldSend
	changed chan struct{}
}

func New(next mail.Mailer) *Sender {
	return &Sender{next: next, changed: make(chan struct{})}
}

func (s *Sender) Send(ctx context.Context, message mail.Message) error {
	if err := s.waitIfPaused(ctx, message.To); err != nil {
		return err
	}
	return s.next.Send(ctx, message)
}

func (s *Sender) Pause() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.paused = true
}

func (s *Sender) Unpause() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.paused = false
}

func (s *Sender) ReleaseAll() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, send := range s.held {
		close(send.release)
	}
	s.held = nil
	s.notifyLocked()
}

func (s *Sender) Release(recipient string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	index := slices.IndexFunc(s.held, func(send *heldSend) bool { return send.recipient == recipient })
	if index < 0 {
		return false
	}
	close(s.held[index].release)
	s.held = slices.Delete(s.held, index, index+1)
	s.notifyLocked()
	return true
}

func (s *Sender) Fail(recipient string, failure error) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	index := slices.IndexFunc(s.held, func(send *heldSend) bool { return send.recipient == recipient })
	if index < 0 {
		return false
	}
	s.held[index].failure = failure
	close(s.held[index].release)
	s.held = slices.Delete(s.held, index, index+1)
	s.notifyLocked()
	return true
}

func (s *Sender) HeldRecipients() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	recipients := make([]string, 0, len(s.held))
	for _, send := range s.held {
		recipients = append(recipients, send.recipient)
	}
	return recipients
}

func (s *Sender) AwaitHeld(ctx context.Context, count int) error {
	for {
		s.mutex.Lock()
		held, changed := len(s.held), s.changed
		s.mutex.Unlock()
		if held == count {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("wait for %d held sends, %d held: %w", count, held, ctx.Err())
		}
	}
}

func (s *Sender) waitIfPaused(ctx context.Context, recipient string) error {
	s.mutex.Lock()
	if !s.paused {
		s.mutex.Unlock()
		return nil
	}
	send := &heldSend{recipient: recipient, release: make(chan struct{})}
	s.held = append(s.held, send)
	s.notifyLocked()
	s.mutex.Unlock()
	select {
	case <-send.release:
		return send.failure
	case <-ctx.Done():
		s.abandon(send)
		return fmt.Errorf("wait for a paused send: %w", ctx.Err())
	}
}

func (s *Sender) abandon(send *heldSend) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index := slices.Index(s.held, send); index >= 0 {
		s.held = slices.Delete(s.held, index, index+1)
		s.notifyLocked()
	}
}

func (s *Sender) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

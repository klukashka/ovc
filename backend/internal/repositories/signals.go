package repositories

import (
	"context"
	"fmt"
	"slices"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type Signals struct {
	queries *dbgen.Queries
}

func (r Signals) Notification(ctx context.Context, user domain.UserID, notification domain.NotificationID) error {
	return r.send(ctx, domain.NotificationSignal{User: user, Notification: notification}, "notification signal")
}

func (r Signals) Entity(ctx context.Context, users []domain.UserID, entity domain.EntityKind, id domain.EntityID) error {
	return r.Entities(ctx, []domain.EntitySignal{{Users: users, Entity: entity, ID: id}})
}

func (r Signals) Entities(ctx context.Context, signals []domain.EntitySignal) error {
	var payloads []string
	for _, signal := range signals {
		for _, recipients := range domain.SplitRecipients(signal.Users) {
			payload, err := domain.EncodeSignal(domain.EntitySignal{Users: recipients, Entity: signal.Entity, ID: signal.ID})
			if err != nil {
				return fmt.Errorf("entity signal: %w", err)
			}
			payloads = append(payloads, string(payload))
		}
	}
	for chunk := range slices.Chunk(payloads, domain.MaxSignalsPerStatement) {
		if err := r.queries.NotifySignals(ctx, dbgen.NotifySignalsParams{Channel: domain.SignalChannel, Payloads: chunk}); err != nil {
			return done(err, "entity signals")
		}
	}
	return nil
}

func (r Signals) Chat(ctx context.Context, signal domain.ChatSignal) error {
	var payloads []string
	for _, recipients := range domain.SplitRecipients(signal.Users) {
		chunk := signal
		chunk.Users = recipients
		payload, err := domain.EncodeSignal(chunk)
		if err != nil {
			return fmt.Errorf("chat signal: %w", err)
		}
		payloads = append(payloads, string(payload))
	}
	if len(payloads) == 0 {
		return nil
	}
	return done(r.queries.NotifySignals(ctx, dbgen.NotifySignalsParams{Channel: domain.SignalChannel, Payloads: payloads}), "chat signals")
}

func (r Signals) Typing(ctx context.Context, signal domain.TypingSignal) error {
	return r.send(ctx, signal, "typing signal")
}

func (r Signals) Presence(ctx context.Context, users []domain.UserID, account domain.UserID, online bool) error {
	var payloads []string
	for _, recipients := range domain.SplitRecipients(users) {
		payload, err := domain.EncodeSignal(domain.PresenceSignal{Users: recipients, Account: account, Online: online})
		if err != nil {
			return fmt.Errorf("presence signal: %w", err)
		}
		payloads = append(payloads, string(payload))
	}
	if len(payloads) == 0 {
		return nil
	}
	return done(r.queries.NotifySignals(ctx, dbgen.NotifySignalsParams{Channel: domain.SignalChannel, Payloads: payloads}), "presence signals")
}

func (r Signals) send(ctx context.Context, signal domain.Signal, what string) error {
	payload, err := domain.EncodeSignal(signal)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return done(r.queries.NotifySignal(ctx, dbgen.NotifySignalParams{Channel: domain.SignalChannel, Payload: string(payload)}), what)
}

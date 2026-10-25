package repositories

import (
	"context"
	"fmt"

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
	for _, recipients := range domain.SplitRecipients(users) {
		if err := r.send(ctx, domain.EntitySignal{Users: recipients, Entity: entity, ID: id}, "entity signal"); err != nil {
			return err
		}
	}
	return nil
}

func (r Signals) Heartbeat(ctx context.Context, instance domain.InstanceID) error {
	return r.send(ctx, domain.HeartbeatSignal{Instance: instance}, "heartbeat signal")
}

func (r Signals) send(ctx context.Context, signal domain.Signal, what string) error {
	payload, err := domain.EncodeSignal(signal)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return done(r.queries.NotifySignal(ctx, dbgen.NotifySignalParams{Channel: domain.SignalChannel, Payload: string(payload)}), what)
}

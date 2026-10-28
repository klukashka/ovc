package notifications

import (
	"context"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type tutorDeletedEmail struct {
	emails *RelationshipEmails
}

func (b tutorDeletedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (Composition, error) {
	var payload domain.TutorDeletedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	tutorID, err := decodeID[domain.UserID](payload.Tutor)
	if err != nil {
		return Composition{}, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	tutor, found, err := existingUser(ctx, repos, tutorID)
	if err != nil {
		return Composition{}, err
	}
	if !found || !stillDeleted(tutor, payload.DeletedAtMicros) {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	if err != nil {
		return Composition{}, err
	}
	if !found {
		return Composition{Obsolete: true}, nil
	}
	text, found := core.RelationshipEmailText(recipient.Language, row.Kind)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	cancelled, err := listCancelled(ctx, repos, b.emails.cities, recipient, repositories.EffectCancellations{
		Cause: domain.CancelTutorDeleted, CancelledAt: *tutor.DeletedAt, Tutor: tutor.ID, OnRosterOf: &recipient.ID,
	})
	if err != nil {
		return Composition{}, err
	}
	email, err := b.emails.render(recipient.Language, text, recipient.Email, tutor.Name, b.emails.publicURL+myTutorsPath, cancelled)
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Identity: tutor.ID.String() + ":" + recipient.ID.String(), Recipient: AccountRecipient(recipient.State)}, nil
}

func stillDeleted(tutor repositories.User, deletedAtMicros int64) bool {
	return tutor.State == domain.AccountDeleted && tutor.DeletedAt != nil && tutor.DeletedAt.UnixMicro() == deletedAtMicros
}

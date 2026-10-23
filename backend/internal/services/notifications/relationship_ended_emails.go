package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	myTutorsPath     = "/tutors"
	studentCardsPath = "/students/"
)

type relationshipEndedEmail struct {
	emails *RelationshipEmails
}

type endedEmailParties struct {
	recipient repositories.User
	name      string
	actionURL string
}

func (b relationshipEndedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (Composition, error) {
	var payload domain.RelationshipEndedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	id, err := decodeID[domain.RelationshipID](payload.Relationship)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	link, err := repos.Relationships.ByID(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if !stillEnded(link, payload.EndedAtMicros) {
		return Composition{Obsolete: true}, nil
	}
	parties, found, err := b.parties(ctx, repos, link)
	if err != nil {
		return Composition{}, err
	}
	if !found {
		return Composition{Obsolete: true}, nil
	}
	text, found := core.RelationshipEndedEmailText(parties.recipient.Language, *link.EndedBy)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, parties.recipient.Language))
	}
	email, err := b.emails.render(parties.recipient.Language, text, parties.recipient.Email, parties.name, parties.actionURL)
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Identity: link.ID.String(), Recipient: AccountRecipient(parties.recipient.State)}, nil
}

func stillEnded(link repositories.Relationship, endedAtMicros int64) bool {
	return link.State == domain.RelationshipEnded && link.EndedAt != nil && link.EndedBy != nil && link.StudentID != nil &&
		link.EndedAt.UnixMicro() == endedAtMicros
}

func (b relationshipEndedEmail) parties(ctx context.Context, repos repositories.Repositories, link repositories.Relationship) (endedEmailParties, bool, error) {
	switch *link.EndedBy {
	case domain.EndedByTutor:
		student, found, err := existingUser(ctx, repos, *link.StudentID)
		if err != nil || !found {
			return endedEmailParties{}, found, err
		}
		tutor, found, err := existingUser(ctx, repos, link.TutorID)
		if err != nil || !found {
			return endedEmailParties{}, found, err
		}
		return endedEmailParties{recipient: student, name: tutor.Name, actionURL: b.emails.publicURL + myTutorsPath}, true, nil
	case domain.EndedByStudent:
		tutor, found, err := existingUser(ctx, repos, link.TutorID)
		if err != nil || !found {
			return endedEmailParties{}, found, err
		}
		return endedEmailParties{recipient: tutor, name: link.DisplayName, actionURL: b.emails.publicURL + studentCardsPath + link.ID.String()}, true, nil
	case domain.EndedByDeletion:
	}
	return endedEmailParties{}, false, nil
}

func existingUser(ctx context.Context, repos repositories.Repositories, id domain.UserID) (repositories.User, bool, error) {
	user, err := repos.Users.ByID(ctx, id)
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		return repositories.User{}, false, nil
	case err != nil:
		return repositories.User{}, false, err
	}
	return user, true, nil
}

package accounts

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrInvitationInvalid       = errors.New("the invitation is not usable")
	ErrInvitationEmailMismatch = errors.New("the invitation is bound to another email or account")
)

type InvitationCheck struct {
	TokenHash    []byte
	Invitation   domain.InvitationID
	Relationship domain.RelationshipID
	Tutor        domain.UserID
	CardEmail    *string
	BoundStudent *domain.UserID
}

type Announce func(context.Context)

func Silent(context.Context) {}

type TutoringEffects interface {
	PrecheckInvitation(ctx context.Context, token string, now time.Time) (InvitationCheck, error)
	BindNewAccount(ctx context.Context, check InvitationCheck, email string, now time.Time) error
	LinkAtSignUp(ctx context.Context, repos repositories.Repositories, user domain.UserID, check InvitationCheck, now time.Time) (Announce, error)
	OnEmailConfirmed(ctx context.Context, repos repositories.Repositories, user domain.UserID, reclaim bool, now time.Time) (domain.LinkOutcome, Announce, error)
	OnAccountDeleted(ctx context.Context, repos repositories.Repositories, user domain.UserID, role domain.Role, now time.Time) (Announce, error)
	OnAccountPurged(ctx context.Context, repos repositories.Repositories, user domain.UserID, role domain.Role, now time.Time) (Announce, error)
	ReclaimPending(ctx context.Context, repos repositories.Repositories, user domain.UserID) (bool, error)
}

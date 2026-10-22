package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	Relationship       = dbgen.Relationship
	NewRelationship    = dbgen.CreateRelationshipParams
	RemovedCard        = dbgen.RemoveRelationshipRow
	NewInvitation      = dbgen.CreateInvitationParams
	ConsumedInvitation = dbgen.ConsumeInvitationRow
	NewJoinRequest     = dbgen.CreateJoinRequestParams
)

var ErrDiscardedStateListed = errors.New("discarded cards are never listed")

type RelationshipCursor struct {
	CreatedAt time.Time
	ID        domain.RelationshipID
}

type StudentListQuery struct {
	Tutor  domain.UserID
	States []domain.RelationshipState
	Group  *domain.GroupID
	Search *string
	After  *RelationshipCursor
	Limit  int32
}

type Relationships struct {
	queries *dbgen.Queries
}

func (r Relationships) Create(ctx context.Context, relationship NewRelationship) error {
	return done(r.queries.CreateRelationship(ctx, relationship), "create relationship")
}

func (r Relationships) ForTutor(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (Relationship, error) {
	relationship, err := r.queries.GetTutorRelationship(ctx, dbgen.GetTutorRelationshipParams{ID: id, TutorID: tutor})
	return one(relationship, err, "relationship of a tutor")
}

func (r Relationships) ListForTutor(ctx context.Context, query StudentListQuery) ([]Relationship, error) {
	params := dbgen.ListTutorRelationshipsParams{TutorID: query.Tutor, RowLimit: query.Limit}
	for _, state := range query.States {
		if state == domain.RelationshipDiscarded {
			return nil, ErrDiscardedStateListed
		}
		params.States = append(params.States, string(state))
	}
	if query.Group != nil {
		params.GroupID = pgtype.UUID{Bytes: *query.Group, Valid: true}
	}
	if query.Search != nil {
		pattern := "%" + escapeLikePattern(*query.Search) + "%"
		params.Pattern = &pattern
	}
	if query.After != nil {
		params.AfterCreatedAt = &query.After.CreatedAt
		params.AfterID = pgtype.UUID{Bytes: query.After.ID, Valid: true}
	}
	relationships, err := r.queries.ListTutorRelationships(ctx, params)
	return one(relationships, err, "list relationships of a tutor")
}

func escapeLikePattern(text string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
}

func (r Relationships) Remove(ctx context.Context, tutor domain.UserID, id domain.RelationshipID, now time.Time) (RemovedCard, error) {
	removed, err := r.queries.RemoveRelationship(ctx, dbgen.RemoveRelationshipParams{Now: now, ID: id, TutorID: tutor})
	return one(removed, err, "remove provisional relationship")
}

type Invitations struct {
	queries *dbgen.Queries
}

func (r Invitations) Create(ctx context.Context, invitation NewInvitation) error {
	return done(r.queries.CreateInvitation(ctx, invitation), "create invitation")
}

func (r Invitations) Consume(ctx context.Context, tokenHash []byte, consumer domain.UserID, now time.Time) (ConsumedInvitation, error) {
	consumed, err := r.queries.ConsumeInvitation(ctx, dbgen.ConsumeInvitationParams{Now: now, UsedBy: &consumer, TokenHash: tokenHash})
	return one(consumed, err, "consume invitation")
}

func (r Invitations) CountMismatch(ctx context.Context, tokenHash []byte, now time.Time) (int16, error) {
	count, err := r.queries.CountInvitationMismatch(ctx, dbgen.CountInvitationMismatchParams{
		MismatchLimit: domain.InvitationMismatchLimit, Now: now, LimitReason: string(domain.RevokeMismatchLimit), TokenHash: tokenHash,
	})
	return one(count, err, "count invitation mismatch")
}

func (r Invitations) RevokeOpen(
	ctx context.Context, tutor domain.UserID, relationship domain.RelationshipID, reason domain.RevokeReason, now time.Time,
) (bool, error) {
	rows, err := r.queries.RevokeOpenInvitation(ctx, dbgen.RevokeOpenInvitationParams{
		Now: now, Reason: string(reason), RelationshipID: relationship, TutorID: tutor,
	})
	return applied(rows, err, "revoke open invitation")
}

type JoinRequests struct {
	queries *dbgen.Queries
}

func (r JoinRequests) Create(ctx context.Context, request NewJoinRequest) error {
	return done(r.queries.CreateJoinRequest(ctx, request), "create join request")
}

func (r JoinRequests) Accept(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, now time.Time) (bool, error) {
	return r.decide(ctx, tutor, id, domain.JoinRequestAccepted, now)
}

func (r JoinRequests) Decline(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, now time.Time) (bool, error) {
	return r.decide(ctx, tutor, id, domain.JoinRequestDeclined, now)
}

func (r JoinRequests) decide(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, decision domain.JoinRequestState, now time.Time) (bool, error) {
	rows, err := r.queries.DecideJoinRequest(ctx, dbgen.DecideJoinRequestParams{Decision: string(decision), Now: now, ID: id, TutorID: tutor})
	return applied(rows, err, "decide join request")
}

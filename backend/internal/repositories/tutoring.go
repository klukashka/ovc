package repositories

import (
	"context"
	"errors"
	"fmt"
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
	CardText           = dbgen.UpdateCardTextParams
	CardAccount        = dbgen.ListCardAccountsRow
	CardGroup          = dbgen.ListCardGroupsRow
	CardInvitation     = dbgen.GetCardInvitationRow
	InvitationTutor    = dbgen.GetInvitationTutorRow
	InvitationEmail    = dbgen.GetInvitationForEmailRow
	UsableInvitation   = dbgen.GetUsableInvitationRow
)

type Link struct {
	ID      domain.RelationshipID
	State   domain.RelationshipState
	Student domain.UserID
	Colour  domain.Colour
	Now     time.Time
}

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

func (r Relationships) ByCreateKey(ctx context.Context, tutor domain.UserID, key string) (Relationship, error) {
	relationship, err := r.queries.GetTutorRelationshipByCreateKey(ctx, dbgen.GetTutorRelationshipByCreateKeyParams{TutorID: tutor, CreateKey: &key})
	return one(relationship, err, "relationship by create key")
}

func (r Relationships) CardWithEmail(ctx context.Context, tutor domain.UserID, email string) (domain.RelationshipID, error) {
	id, err := r.queries.GetTutorCardIDByEmail(ctx, dbgen.GetTutorCardIDByEmailParams{TutorID: tutor, Email: &email})
	return one(id, err, "card with email")
}

func (r Relationships) LockForTutor(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (Relationship, error) {
	relationship, err := r.queries.LockTutorRelationship(ctx, dbgen.LockTutorRelationshipParams{ID: id, TutorID: tutor})
	return one(relationship, err, "lock relationship of a tutor")
}

func (r Relationships) UpdateText(ctx context.Context, text CardText) (bool, error) {
	rows, err := r.queries.UpdateCardText(ctx, text)
	return applied(rows, err, "update card text")
}

func (r Relationships) Withdraw(ctx context.Context, tutor domain.UserID, id domain.RelationshipID, now time.Time) (bool, error) {
	rows, err := r.queries.WithdrawCard(ctx, dbgen.WithdrawCardParams{Now: now, ID: id, TutorID: tutor})
	return applied(rows, err, "withdraw card")
}

func (r Relationships) Accounts(ctx context.Context, students []domain.UserID) ([]CardAccount, error) {
	accounts, err := r.queries.ListCardAccounts(ctx, uuids(students))
	return one(accounts, err, "accounts of cards")
}

func (r Relationships) AvatarsOf(ctx context.Context, owners []domain.UserID) (map[domain.UserID]domain.AttachmentID, error) {
	rows, err := r.queries.ListAvatarsOf(ctx, uuids(owners))
	if err != nil {
		return nil, fmt.Errorf("avatars of accounts: %w", err)
	}
	avatars := make(map[domain.UserID]domain.AttachmentID, len(rows))
	for _, row := range rows {
		avatars[domain.UserID(row.RefID)] = row.AttachmentID
	}
	return avatars, nil
}

func (r Relationships) Groups(ctx context.Context, tutor domain.UserID, cards []domain.RelationshipID) ([]CardGroup, error) {
	groups, err := r.queries.ListCardGroups(ctx, dbgen.ListCardGroupsParams{TutorID: tutor, RelationshipIds: uuids(cards)})
	return one(groups, err, "groups of cards")
}

func uuids[ID ~[16]byte](ids []ID) []pgtype.UUID {
	converted := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		converted = append(converted, pgtype.UUID{Bytes: id, Valid: true})
	}
	return converted
}

func (r Relationships) Remove(ctx context.Context, tutor domain.UserID, id domain.RelationshipID, now time.Time) (RemovedCard, error) {
	removed, err := r.queries.RemoveRelationship(ctx, dbgen.RemoveRelationshipParams{Now: now, ID: id, TutorID: tutor})
	return one(removed, err, "remove provisional relationship")
}

func (r Relationships) LinkInvited(ctx context.Context, link Link) (bool, error) {
	rows, err := r.queries.LinkInvitedRelationship(ctx, dbgen.LinkInvitedRelationshipParams{
		State: link.State, StudentID: &link.Student, Colour: &link.Colour, Now: link.Now, ID: link.ID,
	})
	return applied(rows, err, "link invited relationship")
}

func (r Relationships) ColoursOfStudent(ctx context.Context, student domain.UserID) ([]domain.Colour, error) {
	rows, err := r.queries.ListStudentColours(ctx, &student)
	if err != nil {
		return nil, fmt.Errorf("colours of a student: %w", err)
	}
	colours := make([]domain.Colour, 0, len(rows))
	for _, colour := range rows {
		if colour != nil {
			colours = append(colours, *colour)
		}
	}
	return colours, nil
}

func (r Relationships) LockOfStudent(ctx context.Context, student domain.UserID) ([]Relationship, error) {
	relationships, err := r.queries.LockStudentRelationships(ctx, &student)
	return one(relationships, err, "lock relationships of a student")
}

func (r Relationships) Activate(ctx context.Context, id domain.RelationshipID, now time.Time) (bool, error) {
	rows, err := r.queries.ActivateProvisionalRelationship(ctx, dbgen.ActivateProvisionalRelationshipParams{Now: now, ID: id})
	return applied(rows, err, "activate provisional relationship")
}

func (r Relationships) ForgetPurgedStudent(ctx context.Context, student domain.UserID, now time.Time) error {
	if _, err := r.queries.DeleteStudentMemberships(ctx, &student); err != nil {
		return fmt.Errorf("delete memberships of a purged student: %w", err)
	}
	if _, err := r.queries.DiscardRelationshipsOfPurgedStudent(ctx, dbgen.DiscardRelationshipsOfPurgedStudentParams{Now: now, StudentID: &student}); err != nil {
		return fmt.Errorf("discard relationships of a purged student: %w", err)
	}
	if _, err := r.queries.ClearPurgedStudentFromRelationships(ctx, dbgen.ClearPurgedStudentFromRelationshipsParams{Now: now, StudentID: &student}); err != nil {
		return fmt.Errorf("clear a purged student from relationships: %w", err)
	}
	return nil
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

func (r Invitations) LatestOf(ctx context.Context, relationship domain.RelationshipID) (CardInvitation, error) {
	invitation, err := r.queries.GetCardInvitation(ctx, relationship)
	return one(invitation, err, "latest invitation of a card")
}

func (r Invitations) UsableTutor(ctx context.Context, tokenHash []byte, now time.Time) (InvitationTutor, error) {
	tutor, err := r.queries.GetInvitationTutor(ctx, dbgen.GetInvitationTutorParams{TokenHash: tokenHash, Now: now})
	return one(tutor, err, "tutor of a usable invitation")
}

func (r Invitations) Usable(ctx context.Context, tokenHash []byte, now time.Time) (UsableInvitation, error) {
	invitation, err := r.queries.GetUsableInvitation(ctx, dbgen.GetUsableInvitationParams{TokenHash: tokenHash, Now: now})
	return one(invitation, err, "usable invitation")
}

func (r Invitations) CountSend(ctx context.Context, id domain.InvitationID, now time.Time) (int16, error) {
	count, err := r.queries.CountInvitationSend(ctx, dbgen.CountInvitationSendParams{ID: id, Now: now})
	return one(count, err, "count invitation send")
}

func (r Invitations) ForEmail(ctx context.Context, id domain.InvitationID) (InvitationEmail, error) {
	invitation, err := r.queries.GetInvitationForEmail(ctx, id)
	return one(invitation, err, "invitation for its email")
}

func (r Invitations) ClearExpiredCiphertext(ctx context.Context, now time.Time, batchSize int32) (int64, error) {
	rows, err := r.queries.ClearExpiredInvitationCiphertext(ctx, dbgen.ClearExpiredInvitationCiphertextParams{Now: now, BatchSize: batchSize})
	return counted(rows, err, "clear expired invitation ciphertext")
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

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
	StudentTutor       = dbgen.ListStudentTutorsRow
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

func (r Relationships) HoldsProvisional(ctx context.Context, student domain.UserID) (bool, error) {
	held, err := r.queries.StudentHoldsProvisionalRelationship(ctx, &student)
	return one(held, err, "provisional relationship of a student")
}

func (r Relationships) LockForLink(ctx context.Context, tutor domain.UserID, card domain.RelationshipID, student domain.UserID) ([]Relationship, error) {
	relationships, err := r.queries.LockRelationshipsForLink(ctx, dbgen.LockRelationshipsForLinkParams{TutorID: tutor, CardID: card, StudentID: &student})
	return one(relationships, err, "lock relationships for a link")
}

func (r Relationships) MergeCard(ctx context.Context, tutor domain.UserID, id domain.RelationshipID, now time.Time) (bool, error) {
	rows, err := r.queries.MergeInvitedCard(ctx, dbgen.MergeInvitedCardParams{Now: now, ID: id, TutorID: tutor})
	return applied(rows, err, "merge invited card")
}

func (r Relationships) ActivateLinked(ctx context.Context, link Relationship, now time.Time) (bool, error) {
	rows, err := r.queries.ActivateLinkedRelationship(ctx, dbgen.ActivateLinkedRelationshipParams{
		Now: now, ID: link.ID, TutorID: link.TutorID, StudentID: link.StudentID, FromState: string(link.State),
	})
	return applied(rows, err, "activate linked relationship")
}

func (r Relationships) TutorsOfStudent(ctx context.Context, student domain.UserID, only *domain.RelationshipID) ([]StudentTutor, error) {
	params := dbgen.ListStudentTutorsParams{StudentID: &student}
	if only != nil {
		params.RelationshipID = pgtype.UUID{Bytes: *only, Valid: true}
	}
	tutors, err := r.queries.ListStudentTutors(ctx, params)
	return one(tutors, err, "tutors of a student")
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

func (r Relationships) End(ctx context.Context, link Relationship, by domain.EndedBy, now time.Time) (Relationship, error) {
	ended, err := r.queries.EndRelationship(ctx, dbgen.EndRelationshipParams{EndedBy: string(by), Now: now, ID: link.ID, TutorID: link.TutorID})
	return one(ended, err, "end relationship")
}

func (r Relationships) DeleteMemberships(ctx context.Context, link Relationship) ([]domain.GroupID, error) {
	groups, err := r.queries.DeleteRelationshipMemberships(ctx, dbgen.DeleteRelationshipMembershipsParams{RelationshipID: link.ID, TutorID: link.TutorID})
	return one(groups, err, "delete memberships of a relationship")
}

func (r Relationships) LockOfStudentByID(ctx context.Context, student domain.UserID, id domain.RelationshipID) (Relationship, error) {
	relationship, err := r.queries.LockStudentRelationship(ctx, dbgen.LockStudentRelationshipParams{ID: id, StudentID: &student})
	return one(relationship, err, "lock a relationship of a student")
}

func (r Relationships) ByID(ctx context.Context, id domain.RelationshipID) (Relationship, error) {
	relationship, err := r.queries.GetRelationship(ctx, id)
	return one(relationship, err, "relationship")
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

func (r JoinRequests) AcceptWaiting(ctx context.Context, tutor domain.UserID, student domain.UserID, now time.Time) (int64, error) {
	rows, err := r.queries.AcceptWaitingJoinRequests(ctx, dbgen.AcceptWaitingJoinRequestsParams{Now: now, TutorID: tutor, StudentID: student})
	return counted(rows, err, "accept waiting join requests")
}

type (
	ReclaimPendingLink = dbgen.ListReclaimPendingLinksRow
	OwnerChoiceRequest = dbgen.ListOwnerChoiceJoinRequestsRow
	JoinRequest        = dbgen.JoinRequest
)

func (r Relationships) Reclaim(ctx context.Context, id domain.RelationshipID, now time.Time) (bool, error) {
	rows, err := r.queries.ReclaimProvisionalRelationship(ctx, dbgen.ReclaimProvisionalRelationshipParams{Now: now, ID: id})
	return applied(rows, err, "move provisional relationship to reclaim pending")
}

func (r Relationships) ReclaimPendingOf(ctx context.Context, student domain.UserID) ([]ReclaimPendingLink, error) {
	links, err := r.queries.ListReclaimPendingLinks(ctx, &student)
	return one(links, err, "reclaim pending links of a student")
}

func (r Relationships) AwaitsReclaimChoice(ctx context.Context, student domain.UserID) (bool, error) {
	awaits, err := r.queries.StudentAwaitsReclaimChoice(ctx, &student)
	if err != nil {
		return false, fmt.Errorf("reclaim choice of a student: %w", err)
	}
	return awaits != nil && *awaits, nil
}

type (
	DiscardedForTutorDeletion = dbgen.DiscardRelationshipsForTutorDeletionRow
	EndedForTutorDeletion     = dbgen.EndRelationshipsForTutorDeletionRow
)

func (r Relationships) LockOfTutor(ctx context.Context, tutor domain.UserID) error {
	_, err := r.queries.LockTutorRelationships(ctx, tutor)
	return done(err, "lock relationships of a tutor")
}

func (r Relationships) DiscardForTutorDeletion(ctx context.Context, tutor domain.UserID, now time.Time) ([]DiscardedForTutorDeletion, error) {
	discarded, err := r.queries.DiscardRelationshipsForTutorDeletion(ctx, dbgen.DiscardRelationshipsForTutorDeletionParams{Now: now, TutorID: tutor})
	return one(discarded, err, "discard relationships for tutor deletion")
}

func (r Relationships) EndForTutorDeletion(ctx context.Context, tutor domain.UserID, now time.Time) ([]EndedForTutorDeletion, error) {
	ended, err := r.queries.EndRelationshipsForTutorDeletion(ctx, dbgen.EndRelationshipsForTutorDeletionParams{Now: now, TutorID: tutor})
	return one(ended, err, "end relationships for tutor deletion")
}

func (r Relationships) DeleteMembershipsOfTutor(ctx context.Context, tutor domain.UserID) ([]domain.RelationshipID, error) {
	removed, err := r.queries.DeleteTutorMemberships(ctx, pgtype.UUID{Bytes: tutor, Valid: true})
	return one(removed, err, "delete memberships of a tutor")
}

func (r Relationships) LinkedTutorsOfStudent(ctx context.Context, student domain.UserID) ([]domain.UserID, error) {
	tutors, err := r.queries.ListLinkedTutorsOfStudent(ctx, &student)
	return one(tutors, err, "tutors of a student's waiting links")
}

func (r Relationships) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorRelationshipsBatch(ctx, dbgen.DeleteTutorRelationshipsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's relationships")
}

func (r Invitations) RevokeAllOfTutor(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	rows, err := r.queries.RevokeTutorInvitations(ctx, dbgen.RevokeTutorInvitationsParams{Now: now, TutorID: tutor})
	return counted(rows, err, "revoke the open invitations of a tutor")
}

func (r Invitations) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorInvitationsBatch(ctx, dbgen.DeleteTutorInvitationsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's invitations")
}

func (r JoinRequests) ExpireAllOfTutor(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	rows, err := r.queries.ExpireJoinRequestsOfTutor(ctx, dbgen.ExpireJoinRequestsOfTutorParams{Now: now, TutorID: tutor})
	return counted(rows, err, "expire the open join requests of a tutor")
}

func (r JoinRequests) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorJoinRequestsBatch(ctx, dbgen.DeleteTutorJoinRequestsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's join requests")
}

func (r BlockedRequesters) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorBlockedRequestersBatch(ctx, dbgen.DeleteTutorBlockedRequestersBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's blocked requesters")
}

func (r JoinRequests) MarkHeldForOwnerChoice(ctx context.Context, student domain.UserID) (int64, error) {
	rows, err := r.queries.MarkHeldJoinRequestsForOwnerChoice(ctx, student)
	return counted(rows, err, "mark held join requests for the owner's choice")
}

func (r JoinRequests) OwnerChoiceOf(ctx context.Context, student domain.UserID) ([]OwnerChoiceRequest, error) {
	requests, err := r.queries.ListOwnerChoiceJoinRequests(ctx, student)
	return one(requests, err, "join requests waiting for the owner's choice")
}

func (r JoinRequests) OwnerChoiceRowsOf(ctx context.Context, student domain.UserID) ([]JoinRequest, error) {
	requests, err := r.queries.ListOwnerChoiceJoinRequestRows(ctx, student)
	return one(requests, err, "join request rows waiting for the owner's choice")
}

func (r JoinRequests) LockOwnerChoiceOf(ctx context.Context, student domain.UserID) ([]JoinRequest, error) {
	requests, err := r.queries.LockOwnerChoiceJoinRequests(ctx, student)
	return one(requests, err, "lock join requests waiting for the owner's choice")
}

func (r JoinRequests) ReleaseHeld(ctx context.Context, id domain.JoinRequestID, suppressed domain.Suppression) (bool, error) {
	rows, err := r.queries.ReleaseHeldJoinRequest(ctx, dbgen.ReleaseHeldJoinRequestParams{Suppressed: string(suppressed), ID: id})
	return applied(rows, err, "release held join request")
}

func (r JoinRequests) ExpireHeld(ctx context.Context, id domain.JoinRequestID, now time.Time) (bool, error) {
	rows, err := r.queries.ExpireHeldJoinRequest(ctx, dbgen.ExpireHeldJoinRequestParams{Now: now, ID: id})
	return applied(rows, err, "expire held join request")
}

func (r JoinRequests) VisiblePendingCount(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	count, err := r.queries.CountVisiblePendingJoinRequests(ctx, dbgen.CountVisiblePendingJoinRequestsParams{TutorID: tutor, Now: now})
	return one(count, err, "count visible pending join requests")
}

func (r Relationships) LockReclaimPendingOf(ctx context.Context, student domain.UserID) ([]Relationship, error) {
	relationships, err := r.queries.LockStudentReclaimPendingRelationships(ctx, &student)
	return one(relationships, err, "lock reclaim pending relationships of a student")
}

type (
	TutorJoinRequest    = dbgen.ListTutorJoinRequestsRow
	DueJoinRequest      = dbgen.ListDueJoinRequestsRow
	UnsuppressedRequest = dbgen.UnsuppressJoinRequestsRow
	BlockedRequester    = dbgen.ListBlockedRequestersRow
	CodeOwner           = dbgen.GetTutorByCodeRow
)

type JoinRequestCursor struct {
	CreatedAt time.Time
	ID        domain.JoinRequestID
}

type BlockedCursor struct {
	CreatedAt time.Time
	ID        domain.UserID
}

func (r TutorProfiles) ByCode(ctx context.Context, code domain.TutorCode) (CodeOwner, error) {
	owner, err := r.queries.GetTutorByCode(ctx, code)
	return one(owner, err, "tutor by code")
}

func (r Relationships) OfTutorAndStudent(ctx context.Context, tutor domain.UserID, student domain.UserID) (Relationship, error) {
	relationship, err := r.queries.GetTutorRelationshipOfStudent(ctx, dbgen.GetTutorRelationshipOfStudentParams{TutorID: tutor, StudentID: &student})
	return one(relationship, err, "relationship of a tutor and a student")
}

func (r Relationships) LockForJoinRequest(ctx context.Context, tutor domain.UserID, student domain.UserID, email string) ([]Relationship, error) {
	relationships, err := r.queries.LockRelationshipsForJoinRequest(ctx, dbgen.LockRelationshipsForJoinRequestParams{
		TutorID: tutor, StudentID: &student, Email: &email,
	})
	return one(relationships, err, "lock relationships for a join request")
}

func (r JoinRequests) LockOpen(ctx context.Context, tutor domain.UserID, student domain.UserID) (JoinRequest, error) {
	request, err := r.queries.LockOpenJoinRequest(ctx, dbgen.LockOpenJoinRequestParams{TutorID: tutor, StudentID: student})
	return one(request, err, "lock open join request")
}

func (r JoinRequests) HeldTutorsOf(ctx context.Context, student domain.UserID) ([]domain.UserID, error) {
	tutors, err := r.queries.ListHeldJoinRequestTutors(ctx, student)
	return one(tutors, err, "tutors of held join requests")
}

func (r JoinRequests) LockHeldOf(ctx context.Context, student domain.UserID) ([]JoinRequest, error) {
	requests, err := r.queries.LockHeldJoinRequests(ctx, student)
	return one(requests, err, "lock held join requests")
}

func (r JoinRequests) OpenTutorsOf(ctx context.Context, student domain.UserID) ([]domain.UserID, error) {
	tutors, err := r.queries.ListOpenJoinRequestTutors(ctx, student)
	return one(tutors, err, "tutors of open join requests")
}

func (r JoinRequests) CloseUnblockedOf(ctx context.Context, student domain.UserID, now time.Time) (int64, error) {
	rows, err := r.queries.CloseUnblockedJoinRequestsOfStudent(ctx, dbgen.CloseUnblockedJoinRequestsOfStudentParams{Now: now, StudentID: student})
	return counted(rows, err, "close join requests an unblock closed")
}

func (r JoinRequests) ExpireOpenOf(ctx context.Context, student domain.UserID, now time.Time) ([]domain.UserID, error) {
	tutors, err := r.queries.ExpireOpenJoinRequestsOfStudent(ctx, dbgen.ExpireOpenJoinRequestsOfStudentParams{Now: now, StudentID: student})
	return one(tutors, err, "expire open join requests of a student")
}

func (r JoinRequests) ForTutor(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, now time.Time) (JoinRequest, error) {
	request, err := r.queries.GetTutorJoinRequest(ctx, dbgen.GetTutorJoinRequestParams{ID: id, TutorID: tutor, Now: now})
	return one(request, err, "join request in a tutor's scope")
}

func (r JoinRequests) OfTutor(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID) (JoinRequest, error) {
	request, err := r.queries.GetJoinRequestOfTutor(ctx, dbgen.GetJoinRequestOfTutorParams{ID: id, TutorID: tutor})
	return one(request, err, "join request of a tutor")
}

func (r JoinRequests) LockForTutor(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID) (JoinRequest, error) {
	request, err := r.queries.LockTutorJoinRequest(ctx, dbgen.LockTutorJoinRequestParams{ID: id, TutorID: tutor})
	return one(request, err, "lock join request of a tutor")
}

func (r JoinRequests) VisibleOf(ctx context.Context, tutor domain.UserID, after *JoinRequestCursor, limit int32, now time.Time) ([]TutorJoinRequest, error) {
	params := dbgen.ListTutorJoinRequestsParams{TutorID: tutor, RowLimit: limit, Now: now}
	if after != nil {
		params.AfterCreatedAt = &after.CreatedAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	requests, err := r.queries.ListTutorJoinRequests(ctx, params)
	return one(requests, err, "visible join requests of a tutor")
}

func (r JoinRequests) SentBy(ctx context.Context, student domain.UserID, after *JoinRequestCursor, limit int32) ([]JoinRequest, error) {
	params := dbgen.ListStudentJoinRequestsParams{StudentID: student, RowLimit: limit}
	if after != nil {
		params.AfterCreatedAt = &after.CreatedAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	requests, err := r.queries.ListStudentJoinRequests(ctx, params)
	return one(requests, err, "join requests of a student")
}

func (r JoinRequests) Unsuppress(ctx context.Context, tutor domain.UserID, slots int64, now time.Time) ([]UnsuppressedRequest, error) {
	requests, err := r.queries.UnsuppressJoinRequests(ctx, dbgen.UnsuppressJoinRequestsParams{TutorID: tutor, Now: now, Slots: slots})
	return one(requests, err, "unsuppress join requests")
}

type DueCursor struct {
	ExpiresAt time.Time
	ID        domain.JoinRequestID
}

func (r JoinRequests) Due(ctx context.Context, now time.Time, after *DueCursor, batchSize int32) ([]DueJoinRequest, error) {
	params := dbgen.ListDueJoinRequestsParams{Now: now, BatchSize: batchSize}
	if after != nil {
		params.AfterExpiresAt = &after.ExpiresAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	requests, err := r.queries.ListDueJoinRequests(ctx, params)
	return one(requests, err, "join requests due to expire")
}

func (r JoinRequests) ExpireDue(ctx context.Context, id domain.JoinRequestID, now time.Time) (bool, error) {
	rows, err := r.queries.ExpireDueJoinRequest(ctx, dbgen.ExpireDueJoinRequestParams{Now: now, ID: id})
	return applied(rows, err, "expire a due join request")
}

func (r JoinRequests) CloseBlocked(ctx context.Context, tutor domain.UserID, student domain.UserID) (int64, error) {
	rows, err := r.queries.CloseBlockedJoinRequests(ctx, dbgen.CloseBlockedJoinRequestsParams{TutorID: tutor, StudentID: student})
	return counted(rows, err, "close join requests suppressed by a block")
}

type BlockedRequesters struct {
	queries *dbgen.Queries
}

func (r BlockedRequesters) Block(ctx context.Context, tutor domain.UserID, student domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.BlockRequester(ctx, dbgen.BlockRequesterParams{TutorID: tutor, StudentID: student, CreatedAt: now})
	return applied(rows, err, "block requester")
}

func (r BlockedRequesters) Unblock(ctx context.Context, tutor domain.UserID, student domain.UserID) (bool, error) {
	rows, err := r.queries.UnblockRequester(ctx, dbgen.UnblockRequesterParams{TutorID: tutor, StudentID: student})
	return applied(rows, err, "unblock requester")
}

func (r BlockedRequesters) Contains(ctx context.Context, tutor domain.UserID, student domain.UserID) (bool, error) {
	blocked, err := r.queries.StudentBlockedByTutor(ctx, dbgen.StudentBlockedByTutorParams{TutorID: tutor, StudentID: student})
	return one(blocked, err, "blocked requester")
}

func (r BlockedRequesters) Of(ctx context.Context, tutor domain.UserID, after *BlockedCursor, limit int32) ([]BlockedRequester, error) {
	params := dbgen.ListBlockedRequestersParams{TutorID: tutor, RowLimit: limit}
	if after != nil {
		params.AfterCreatedAt = &after.CreatedAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	blocked, err := r.queries.ListBlockedRequesters(ctx, params)
	return one(blocked, err, "blocked requesters of a tutor")
}

func (r JoinRequests) LockClosedByUnblock(ctx context.Context, tutor domain.UserID, student domain.UserID, now time.Time) (JoinRequest, error) {
	request, err := r.queries.LockJoinRequestClosedByUnblock(ctx, dbgen.LockJoinRequestClosedByUnblockParams{TutorID: tutor, StudentID: student, Now: now})
	return one(request, err, "lock join request closed by an unblock")
}

func (r JoinRequests) Reopen(ctx context.Context, id domain.JoinRequestID, suppressed domain.Suppression, now time.Time) (bool, error) {
	rows, err := r.queries.ReopenJoinRequest(ctx, dbgen.ReopenJoinRequestParams{Suppressed: string(suppressed), ID: id, Now: now})
	return applied(rows, err, "reopen join request")
}

func (r JoinRequests) DeleteOfStudent(ctx context.Context, student domain.UserID) (int64, error) {
	deleted, err := r.queries.DeleteJoinRequestsOfStudent(ctx, student)
	return counted(deleted, err, "delete join requests of student")
}

func (r Relationships) LockForStudentDeletion(ctx context.Context, student domain.UserID) ([]Relationship, error) {
	relationships, err := r.queries.LockStudentDeletionRelationships(ctx, &student)
	return one(relationships, err, "lock all relationships for student deletion")
}

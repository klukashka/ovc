package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var errArchivedWithoutTime = errors.New("an archived group returned no archiving time")

type (
	Group              = dbgen.Group
	NewGroup           = dbgen.CreateGroupParams
	GroupMember        = dbgen.ListGroupMembersRow
	FirstGroupMember   = dbgen.ListFirstGroupMembersRow
	GroupCandidate     = dbgen.LockTutorRelationshipsForGroupRow
	GroupDetailsChange = dbgen.UpdateGroupDetailsParams
)

type Groups struct {
	queries       *dbgen.Queries
	invalidations *exportInvalidations
}

func (r Groups) Create(ctx context.Context, group NewGroup) error {
	return done(r.queries.CreateGroup(ctx, group), "create group")
}

func (r Groups) ForTutor(ctx context.Context, tutor domain.UserID, id domain.GroupID) (Group, error) {
	group, err := r.queries.GetTutorGroup(ctx, dbgen.GetTutorGroupParams{ID: id, TutorID: tutor})
	return one(group, err, "group of a tutor")
}

func (r Groups) LockForTutor(ctx context.Context, tutor domain.UserID, id domain.GroupID) (Group, error) {
	group, err := r.queries.LockTutorGroup(ctx, dbgen.LockTutorGroupParams{ID: id, TutorID: tutor})
	return one(group, err, "lock group of a tutor")
}

func (r Groups) ByCreateKey(ctx context.Context, tutor domain.UserID, key string) (Group, error) {
	group, err := r.queries.GetTutorGroupByCreateKey(ctx, dbgen.GetTutorGroupByCreateKeyParams{TutorID: tutor, CreateKey: &key})
	return one(group, err, "group by create key")
}

func (r Groups) CountOfTutor(ctx context.Context, tutor domain.UserID) (int64, error) {
	count, err := r.queries.CountTutorGroups(ctx, tutor)
	return one(count, err, "count groups of a tutor")
}

func (r Groups) OfTutor(ctx context.Context, tutor domain.UserID) ([]Group, error) {
	groups, err := r.queries.ListTutorGroups(ctx, tutor)
	return one(groups, err, "groups of a tutor")
}

func (r Groups) UpdateDetails(ctx context.Context, change GroupDetailsChange) (bool, error) {
	rows, err := r.queries.UpdateGroupDetails(ctx, change)
	return applied(rows, err, "update group details")
}

func (r Groups) Archive(ctx context.Context, tutor domain.UserID, id domain.GroupID, now time.Time) (*time.Time, error) {
	archivedAt, err := r.queries.ArchiveGroup(ctx, dbgen.ArchiveGroupParams{Now: now, ID: id, TutorID: tutor})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, done(err, "archive group")
	}
	if archivedAt == nil {
		return nil, done(errArchivedWithoutTime, "archive group")
	}
	return archivedAt, nil
}

func (r Groups) Unarchive(ctx context.Context, tutor domain.UserID, id domain.GroupID, now time.Time) (bool, error) {
	rows, err := r.queries.UnarchiveGroup(ctx, dbgen.UnarchiveGroupParams{Now: now, ID: id, TutorID: tutor})
	return applied(rows, err, "unarchive group")
}

func (r Groups) AddMembers(ctx context.Context, tutor domain.UserID, id domain.GroupID, members []domain.RelationshipID, now time.Time) (int64, error) {
	if len(members) == 0 {
		return 0, nil
	}
	rows, err := r.queries.AddGroupMembers(ctx, dbgen.AddGroupMembersParams{
		GroupID: pgtype.UUID{Bytes: id, Valid: true}, TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, Now: now, RelationshipIds: uuids(members),
	})
	return counted(rows, err, "add group members")
}

func (r Groups) RemoveMembers(ctx context.Context, tutor domain.UserID, id domain.GroupID, members []domain.RelationshipID) (int64, error) {
	if err := r.invalidations.open(); err != nil {
		return 0, err
	}
	if len(members) == 0 {
		return 0, nil
	}
	students, err := r.queries.RemoveGroupMembers(ctx, dbgen.RemoveGroupMembersParams{GroupID: id, TutorID: tutor, RelationshipIds: uuids(members)})
	if err != nil {
		return 0, done(err, "remove group members")
	}
	if len(students) == 0 {
		return 0, nil
	}
	return int64(len(students)), r.invalidations.nameParties(domain.ExportCauseGroupMemberRemoved, tutor, students...)
}

func (r Groups) MemberIDs(ctx context.Context, tutor domain.UserID, id domain.GroupID) ([]domain.RelationshipID, error) {
	members, err := r.queries.ListGroupMemberIDs(ctx, dbgen.ListGroupMemberIDsParams{GroupID: id, TutorID: tutor})
	return one(members, err, "member identifiers of a group")
}

func (r Groups) Members(ctx context.Context, tutor domain.UserID, id domain.GroupID) ([]GroupMember, error) {
	members, err := r.queries.ListGroupMembers(ctx, dbgen.ListGroupMembersParams{GroupID: id, TutorID: tutor})
	return one(members, err, "members of a group")
}

func (r Groups) FirstMembers(ctx context.Context, tutor domain.UserID, groups []domain.GroupID, first int64) ([]FirstGroupMember, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	members, err := r.queries.ListFirstGroupMembers(ctx, dbgen.ListFirstGroupMembersParams{TutorID: tutor, GroupIds: uuids(groups), FirstMembers: first})
	return one(members, err, "first members of groups")
}

func (r Relationships) LockForGroup(ctx context.Context, tutor domain.UserID, ids []domain.RelationshipID) ([]GroupCandidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	candidates, err := r.queries.LockTutorRelationshipsForGroup(ctx, dbgen.LockTutorRelationshipsForGroupParams{TutorID: tutor, Ids: uuids(ids)})
	return one(candidates, err, "lock relationships for a group")
}

func (r Relationships) LockForGroupChange(ctx context.Context, tutor domain.UserID, ids []domain.RelationshipID) ([]GroupCandidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	locked, err := r.queries.LockTutorRelationshipsForGroupChange(ctx, dbgen.LockTutorRelationshipsForGroupChangeParams{TutorID: tutor, Ids: uuids(ids)})
	rows, err := one(locked, err, "lock relationships for a group change")
	if err != nil {
		return nil, err
	}
	candidates := make([]GroupCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, GroupCandidate(row))
	}
	return candidates, nil
}

func (r Groups) ArchiveAllOfTutor(ctx context.Context, tutor domain.UserID, now time.Time) ([]domain.GroupID, error) {
	archived, err := r.queries.ArchiveTutorGroups(ctx, dbgen.ArchiveTutorGroupsParams{Now: now, TutorID: tutor})
	return one(archived, err, "archive the groups of a tutor")
}

func (r Groups) DeleteMembersBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorGroupMembersBatch(ctx, dbgen.DeleteTutorGroupMembersBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's group members")
}

func (r Groups) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorGroupsBatch(ctx, dbgen.DeleteTutorGroupsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's groups")
}

func (r Groups) LockForStudentDeletion(ctx context.Context, student domain.UserID) ([]domain.GroupID, error) {
	groups, err := r.queries.LockStudentDeletionGroups(ctx, &student)
	return one(groups, err, "lock groups for student deletion")
}

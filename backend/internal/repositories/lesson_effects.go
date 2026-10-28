package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	EffectCancelledLesson   = dbgen.CancelLessonsForEffectRow
	ArchivedEmailGroup      = dbgen.GetGroupForArchivedEmailRow
	NotificationGroup       = dbgen.ListNotificationGroupsRow
	LessonCancelledByEffect = dbgen.ListLessonsCancelledByEffectRow
)

type EffectScope struct {
	Tutor         domain.UserID
	WholeTutor    bool
	Relationships []domain.RelationshipID
	Groups        []domain.GroupID
}

type RosterEntry struct {
	Lesson  domain.LessonID
	Student domain.UserID
}

type LinkStudent struct {
	Relationship domain.RelationshipID
	Student      domain.UserID
}

type GroupStudent struct {
	Group   domain.GroupID
	Student domain.UserID
}

type EffectCancellations struct {
	Cause       domain.CancelCause
	CancelledAt time.Time
	Tutor       domain.UserID
	Group       *domain.GroupID
	Link        *domain.RelationshipID
	OnRosterOf  *domain.UserID
	Limit       int32
}

func (s EffectScope) empty() bool {
	return !s.WholeTutor && len(s.Relationships) == 0 && len(s.Groups) == 0
}

func (r Lessons) EndSeriesForEffect(ctx context.Context, scope EffectScope, now time.Time) ([]domain.SeriesID, error) {
	if scope.empty() {
		return nil, nil
	}
	ended, err := r.queries.EndLessonSeriesForEffect(ctx, dbgen.EndLessonSeriesForEffectParams{
		Now: now, TutorID: pgtype.UUID{Bytes: scope.Tutor, Valid: true}, WholeTutor: scope.WholeTutor,
		RelationshipIds: uuids(scope.Relationships), GroupIds: uuids(scope.Groups),
	})
	return one(ended, err, "end the series of an audience change")
}

func (r Lessons) CancelForEffect(
	ctx context.Context, scope EffectScope, cause domain.CancelCause, cancelledAt time.Time,
) ([]EffectCancelledLesson, error) {
	if scope.empty() {
		return nil, nil
	}
	cancelled, err := r.queries.CancelLessonsForEffect(ctx, dbgen.CancelLessonsForEffectParams{
		CancelledAt: cancelledAt, CancelCause: string(cause), TutorID: pgtype.UUID{Bytes: scope.Tutor, Valid: true},
		WholeTutor: scope.WholeTutor, RelationshipIds: uuids(scope.Relationships), GroupIds: uuids(scope.Groups),
	})
	return one(cancelled, err, "cancel the lessons of an audience change")
}

func (r Lessons) FreezeRosters(ctx context.Context, entries []RosterEntry, now time.Time) (int64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	lessons := make([]pgtype.UUID, 0, len(entries))
	students := make([]pgtype.UUID, 0, len(entries))
	for _, entry := range entries {
		lessons = append(lessons, pgtype.UUID{Bytes: entry.Lesson, Valid: true})
		students = append(students, pgtype.UUID{Bytes: entry.Student, Valid: true})
	}
	rows, err := r.queries.FreezeEffectRosters(ctx, dbgen.FreezeEffectRostersParams{Now: now, LessonIds: lessons, StudentIds: students})
	return counted(rows, err, "freeze the rosters of an audience change")
}

func (r Lessons) DeleteRosterRowsOfLinks(ctx context.Context, links []LinkStudent) (int64, error) {
	if len(links) == 0 {
		return 0, nil
	}
	relationships := make([]pgtype.UUID, 0, len(links))
	students := make([]pgtype.UUID, 0, len(links))
	for _, link := range links {
		relationships = append(relationships, pgtype.UUID{Bytes: link.Relationship, Valid: true})
		students = append(students, pgtype.UUID{Bytes: link.Student, Valid: true})
	}
	rows, err := r.queries.DeleteRosterRowsOfLinks(ctx, dbgen.DeleteRosterRowsOfLinksParams{RelationshipIds: relationships, StudentIds: students})
	return counted(rows, err, "delete the roster rows of discarded links")
}

func (r Lessons) ActiveAudienceOfTutor(ctx context.Context, tutor domain.UserID) ([]LinkStudent, error) {
	rows, err := r.queries.ListActiveAudienceOfTutor(ctx, tutor)
	if err != nil {
		return nil, done(err, "the active relationships of a tutor")
	}
	links := make([]LinkStudent, 0, len(rows))
	for _, row := range rows {
		if row.StudentID != nil {
			links = append(links, LinkStudent{Relationship: row.ID, Student: *row.StudentID})
		}
	}
	return links, nil
}

func (r Lessons) GroupMembersOfTutor(ctx context.Context, tutor domain.UserID) ([]GroupStudent, error) {
	rows, err := r.queries.ListGroupMemberStudentsOfTutor(ctx, tutor)
	if err != nil {
		return nil, done(err, "the group members of a tutor")
	}
	members := make([]GroupStudent, 0, len(rows))
	for _, row := range rows {
		if row.StudentID != nil {
			members = append(members, GroupStudent{Group: row.GroupID, Student: *row.StudentID})
		}
	}
	return members, nil
}

func (r Lessons) CancelledByEffect(ctx context.Context, query EffectCancellations) ([]LessonCancelledByEffect, error) {
	rows, err := r.queries.ListLessonsCancelledByEffect(ctx, dbgen.ListLessonsCancelledByEffectParams{
		CancelCause: string(query.Cause), CancelledAt: query.CancelledAt, TutorID: pgtype.UUID{Bytes: query.Tutor, Valid: true},
		RelationshipID: optionalUUID(query.Link), GroupID: optionalUUID(query.Group), RosterStudentID: optionalUUID(query.OnRosterOf),
		RowLimit: query.Limit,
	})
	return one(rows, err, "lessons cancelled by an audience change")
}

func (r Groups) ForArchivedEmail(ctx context.Context, id domain.GroupID) (ArchivedEmailGroup, error) {
	row, err := r.queries.GetGroupForArchivedEmail(ctx, id)
	return one(row, err, "a group for its archived email")
}

func (r Notifications) Groups(ctx context.Context, student domain.UserID, ids []domain.GroupID) ([]NotificationGroup, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListNotificationGroups(ctx, dbgen.ListNotificationGroupsParams{
		StudentID: pgtype.UUID{Bytes: student, Valid: true}, Ids: uuids(ids),
	})
	return one(rows, err, "groups of notifications")
}

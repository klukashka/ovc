package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	EffectLesson            = dbgen.CancelLessonsOfTutorForEffectRow
	ArchivedEmailGroup      = dbgen.GetGroupForArchivedEmailRow
	NotificationGroup       = dbgen.ListNotificationGroupsRow
	LessonCancelledByEffect = dbgen.ListLessonsCancelledOnRosterRow
)

var (
	errMixedEffectScope         = errors.New("an effect scope names more than one kind of audience")
	errUnsupportedCancellations = errors.New("a listing of cancelled lessons names an unsupported combination of filters")
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

func (s EffectScope) kinds() int {
	kinds := 0
	for _, named := range []bool{s.WholeTutor, len(s.Relationships) > 0, len(s.Groups) > 0} {
		if named {
			kinds++
		}
	}
	return kinds
}

func (r Lessons) EndSeriesForEffect(ctx context.Context, scope EffectScope, now time.Time) ([]domain.SeriesID, error) {
	tutor := pgtype.UUID{Bytes: scope.Tutor, Valid: true}
	var ended []domain.SeriesID
	var err error
	switch {
	case scope.kinds() == 0:
		return nil, nil
	case scope.kinds() > 1:
		return nil, errMixedEffectScope
	case scope.WholeTutor:
		ended, err = r.queries.EndLessonSeriesOfTutorForEffect(ctx, dbgen.EndLessonSeriesOfTutorForEffectParams{Now: now, TutorID: tutor})
	case len(scope.Relationships) > 0:
		ended, err = r.queries.EndLessonSeriesOfRelationshipsForEffect(ctx, dbgen.EndLessonSeriesOfRelationshipsForEffectParams{
			Now: now, TutorID: tutor, RelationshipIds: uuids(scope.Relationships),
		})
	default:
		ended, err = r.queries.EndLessonSeriesOfGroupsForEffect(ctx, dbgen.EndLessonSeriesOfGroupsForEffectParams{
			Now: now, TutorID: tutor, GroupIds: uuids(scope.Groups),
		})
	}
	return one(ended, err, "end the series of an audience change")
}

func (r Lessons) CancelForEffect(ctx context.Context, scope EffectScope, cause domain.CancelCause, cancelledAt time.Time) ([]EffectLesson, error) {
	tutor := pgtype.UUID{Bytes: scope.Tutor, Valid: true}
	switch {
	case scope.kinds() == 0:
		return nil, nil
	case scope.kinds() > 1:
		return nil, errMixedEffectScope
	case scope.WholeTutor:
		cancelled, err := r.queries.CancelLessonsOfTutorForEffect(ctx, dbgen.CancelLessonsOfTutorForEffectParams{
			CancelledAt: cancelledAt, CancelCause: string(cause), TutorID: tutor,
		})
		return one(cancelled, err, "cancel the lessons of a tutor's deletion")
	case len(scope.Relationships) > 0:
		cancelled, err := r.queries.CancelLessonsOfRelationshipsForEffect(ctx, dbgen.CancelLessonsOfRelationshipsForEffectParams{
			CancelledAt: cancelledAt, CancelCause: string(cause), TutorID: tutor, RelationshipIds: uuids(scope.Relationships),
		})
		return effectLessons(cancelled, err, "cancel the lessons of relationships")
	}
	cancelled, err := r.queries.CancelLessonsOfGroupsForEffect(ctx, dbgen.CancelLessonsOfGroupsForEffectParams{
		CancelledAt: cancelledAt, CancelCause: string(cause), TutorID: tutor, GroupIds: uuids(scope.Groups),
	})
	return effectLessons(cancelled, err, "cancel the lessons of groups")
}

func (r Lessons) UnderWayOfTutor(ctx context.Context, tutor domain.UserID, at time.Time) ([]EffectLesson, error) {
	lessons, err := r.queries.ListUnderWayLessonsOfTutor(ctx, dbgen.ListUnderWayLessonsOfTutorParams{TutorID: tutor, At: at})
	return effectLessons(lessons, err, "the lessons of a tutor under way")
}

func (r Lessons) EndLiveOfTutor(ctx context.Context, tutor domain.UserID, now time.Time) ([]EffectLesson, error) {
	rows, err := r.queries.EndLiveLessonsOfTutor(ctx, dbgen.EndLiveLessonsOfTutorParams{TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, Now: now})
	return effectLessons(rows, err, "end the live lessons of a tutor")
}

func effectLessons[Row dbgen.CancelLessonsOfRelationshipsForEffectRow | dbgen.CancelLessonsOfGroupsForEffectRow | dbgen.ListUnderWayLessonsOfTutorRow | dbgen.EndLiveLessonsOfTutorRow](
	rows []Row, err error, what string,
) ([]EffectLesson, error) {
	if _, err := one(rows, err, what); err != nil {
		return nil, err
	}
	lessons := make([]EffectLesson, 0, len(rows))
	for _, row := range rows {
		lessons = append(lessons, EffectLesson(row))
	}
	return lessons, nil
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
	tutor := pgtype.UUID{Bytes: query.Tutor, Valid: true}
	cause := string(query.Cause)
	switch {
	case query.Link != nil && query.OnRosterOf == nil && query.Group == nil:
		rows, err := r.queries.ListLessonsCancelledOfLink(ctx, dbgen.ListLessonsCancelledOfLinkParams{
			RelationshipID: pgtype.UUID{Bytes: *query.Link, Valid: true}, CancelledAt: query.CancelledAt, TutorID: tutor, CancelCause: cause,
			RowLimit: query.Limit,
		})
		return cancelledLessons(rows, err, "lessons cancelled by the ending of a relationship")
	case query.OnRosterOf != nil && query.Link == nil && query.Group != nil:
		rows, err := r.queries.ListGroupLessonsCancelledOnRoster(ctx, dbgen.ListGroupLessonsCancelledOnRosterParams{
			StudentID: pgtype.UUID{Bytes: *query.OnRosterOf, Valid: true}, CancelledAt: query.CancelledAt,
			GroupID: pgtype.UUID{Bytes: *query.Group, Valid: true}, TutorID: tutor, CancelCause: cause, RowLimit: query.Limit,
		})
		return cancelledLessons(rows, err, "group lessons cancelled from a student's roster")
	case query.OnRosterOf != nil && query.Link == nil && query.Group == nil:
		rows, err := r.queries.ListLessonsCancelledOnRoster(ctx, dbgen.ListLessonsCancelledOnRosterParams{
			StudentID: pgtype.UUID{Bytes: *query.OnRosterOf, Valid: true}, CancelledAt: query.CancelledAt, TutorID: tutor, CancelCause: cause,
			RowLimit: query.Limit,
		})
		return one(rows, err, "lessons cancelled from a student's roster")
	}
	return nil, errUnsupportedCancellations
}

func cancelledLessons[Row dbgen.ListLessonsCancelledOfLinkRow | dbgen.ListGroupLessonsCancelledOnRosterRow](
	rows []Row, err error, what string,
) ([]LessonCancelledByEffect, error) {
	if _, err := one(rows, err, what); err != nil {
		return nil, err
	}
	lessons := make([]LessonCancelledByEffect, 0, len(rows))
	for _, row := range rows {
		lessons = append(lessons, LessonCancelledByEffect(row))
	}
	return lessons, nil
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

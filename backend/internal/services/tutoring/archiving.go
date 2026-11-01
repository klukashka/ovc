package tutoring

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
)

const archiveAttempts = 2

var (
	ErrConflict       = errors.New("the group's members changed while the archiving held its locks")
	errMembersChanged = errors.New("a group's members changed under the lock")
)

type archiving struct {
	group      Group
	archivedAt *time.Time
	lessons    lessonEffects
	notified   []*createdNotification
	emails     int64
}

func (s *Service) ArchiveGroup(ctx context.Context, actor domain.TutorActor, id domain.GroupID) (Group, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Group{}, err
	}
	var outcome archiving
	for attempt := 1; ; attempt++ {
		err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
			var err error
			outcome, err = s.archive(ctx, repos, actor, id, now)
			return err
		})
		if !errors.Is(err, errMembersChanged) {
			break
		}
		if attempt == archiveAttempts {
			return Group{}, ErrConflict
		}
	}
	if err != nil {
		return Group{}, err
	}
	if outcome.archivedAt != nil {
		attributes := append(groupAttributes(tutor, id), slog.Int("members", len(outcome.group.Members)), slog.Int64("emails_queued", outcome.emails))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.archived", attributes...)
		s.logLessonEffects(ctx, outcome.lessons)
		s.logNotified(ctx, outcome.notified...)
	}
	return outcome.group, nil
}

func (s *Service) archive(ctx context.Context, repos repositories.Repositories, actor domain.TutorActor, id domain.GroupID, now time.Time) (archiving, error) {
	tutor := actor.UserID
	if err := s.lockActingTutor(ctx, repos, actor); err != nil {
		return archiving{}, err
	}
	if _, err := repos.Groups.ForTutor(ctx, tutor, id); err != nil {
		return archiving{}, groupScope(err)
	}
	seen, err := repos.Groups.MemberIDs(ctx, tutor, id)
	if err != nil {
		return archiving{}, err
	}
	locked, err := repos.Relationships.LockForGroup(ctx, tutor, seen)
	if err != nil {
		return archiving{}, err
	}
	if _, err := repos.Groups.LockForTutor(ctx, tutor, id); err != nil {
		return archiving{}, groupScope(err)
	}
	current, err := repos.Groups.MemberIDs(ctx, tutor, id)
	if err != nil {
		return archiving{}, err
	}
	if len(locked) != len(seen) || !sameRelationships(seen, current) {
		return archiving{}, errMembersChanged
	}
	members, err := repos.Groups.Members(ctx, tutor, id)
	if err != nil {
		return archiving{}, err
	}
	var outcome archiving
	if outcome.archivedAt, err = repos.Groups.Archive(ctx, tutor, id, now); err != nil {
		return archiving{}, err
	}
	if outcome.archivedAt != nil {
		if err := s.mirror(ctx, repos, domain.PathArchiving, chat.Audiences{Groups: []domain.GroupID{id}}, now); err != nil {
			return archiving{}, err
		}
		if err := s.archiveEffects(ctx, repos, tutor, id, members, *outcome.archivedAt, &outcome); err != nil {
			return archiving{}, err
		}
	}
	outcome.group, err = s.currentGroup(ctx, repos, tutor, id)
	return outcome, err
}

func sameRelationships(first []domain.RelationshipID, second []domain.RelationshipID) bool {
	if len(first) != len(second) {
		return false
	}
	listed := make(map[domain.RelationshipID]bool, len(first))
	for _, id := range first {
		listed[id] = true
	}
	for _, id := range second {
		if !listed[id] {
			return false
		}
	}
	return true
}

func (s *Service) archiveEffects(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.GroupID, members []repositories.GroupMember,
	archivedAt time.Time, outcome *archiving,
) error {
	students := make([]domain.UserID, 0, len(members))
	for _, member := range members {
		if member.StudentID != nil {
			students = append(students, *member.StudentID)
		}
	}
	scope := repositories.EffectScope{Tutor: tutor, Groups: []domain.GroupID{id}}
	series, err := s.endSeries(ctx, repos, scope, archivedAt)
	if err != nil {
		return err
	}
	audience := lessonAudience{groups: map[domain.GroupID][]domain.UserID{id: students}}
	if outcome.lessons, err = s.cancelLessons(ctx, repos, lessonCancellation{
		scope: scope, cause: domain.CancelGroupArchived, at: archivedAt, audience: audience,
	}); err != nil {
		return err
	}
	outcome.lessons.series = series
	if outcome.lessons.cancelled == 0 {
		return nil
	}
	for _, member := range members {
		if member.StudentID == nil {
			continue
		}
		relationship := member.RelationshipID
		key := domain.GroupArchivedKey(id, archivedAt.UnixMicro(), *member.StudentID)
		created, err := s.notify(ctx, repos, notificationRow{
			event: domain.NotificationGroupArchived, recipient: *member.StudentID, tutor: tutor, student: *member.StudentID,
			subject: domain.EntityID(id), relationship: &relationship, key: key,
		}, archivedAt)
		if err != nil {
			return err
		}
		outcome.notified = append(outcome.notified, created)
	}
	outcome.emails, err = s.queueArchivedEmails(ctx, repos, id, students, archivedAt)
	return err
}

func (s *Service) queueArchivedEmails(
	ctx context.Context, repos repositories.Repositories, group domain.GroupID, students []domain.UserID, archivedAt time.Time,
) (int64, error) {
	if len(students) == 0 {
		return 0, nil
	}
	encodedGroup := hex.EncodeToString(group[:])
	rows := make([]repositories.QueuedOutboxRow, 0, len(students))
	for _, student := range students {
		payload, err := json.Marshal(domain.GroupArchivedPayload{
			Group: encodedGroup, ArchivedAtMicros: archivedAt.UnixMicro(), Recipient: hex.EncodeToString(student[:]),
		})
		if err != nil {
			return 0, fmt.Errorf("encode the group archived payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](s.deps.Random)
		if err != nil {
			return 0, err
		}
		rows = append(rows, repositories.QueuedOutboxRow{
			ID: id, Payload: payload, IdempotencyKey: domain.GroupArchivedKey(group, archivedAt.UnixMicro(), student),
		})
	}
	return repos.Outbox.InsertAll(ctx, repositories.NewOutboxRows{
		Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: domain.EventGroupArchived, NextAttemptAt: archivedAt, CreatedAt: archivedAt,
		Rows: rows,
	})
}

package schedule

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const jobAudienceAttempts = 2

var errAudienceMoved = errors.New("the audience changed between reading and locking it")

type JobSettings struct {
	Wall            func() time.Time
	StatusBatch     int32
	StatusBudget    time.Duration
	ReminderBatch   int32
	ReminderBudget  time.Duration
	ExtensionBatch  int32
	ExtensionBudget time.Duration
}

func DefaultJobSettings() JobSettings {
	return JobSettings{
		Wall: clock.System{}.Now, StatusBatch: 100, StatusBudget: 20 * time.Second, ReminderBatch: 50, ReminderBudget: 20 * time.Second,
		ExtensionBatch: 200, ExtensionBudget: 2 * time.Minute,
	}
}

type Jobs struct {
	service  *Service
	settings JobSettings
}

func NewJobs(service *Service, settings JobSettings) *Jobs {
	return &Jobs{service: service, settings: settings}
}

type audienceMember struct {
	student      domain.UserID
	relationship domain.RelationshipID
}

type jobAudience struct {
	members []audienceMember
	active  bool
}

func (a jobAudience) students() []domain.UserID {
	students := make([]domain.UserID, 0, len(a.members))
	for _, member := range a.members {
		students = append(students, member.student)
	}
	return students
}

func lockJobAudience(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship *domain.RelationshipID, group *domain.GroupID,
) (jobAudience, error) {
	audience, moved, err := lockAudienceOf(ctx, repos, tutor, relationship, group)
	if err != nil {
		return jobAudience{}, err
	}
	if moved {
		return jobAudience{}, errAudienceMoved
	}
	return audience, nil
}

func lockAudienceOf(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship *domain.RelationshipID, group *domain.GroupID,
) (jobAudience, bool, error) {
	if relationship != nil {
		locked, err := repos.Lessons.LockRelationship(ctx, tutor, *relationship)
		if errors.Is(err, repositories.ErrNotFound) {
			return jobAudience{}, false, nil
		}
		if err != nil {
			return jobAudience{}, false, err
		}
		if locked.StudentID == nil || (locked.State != domain.RelationshipActive && locked.State != domain.RelationshipProvisional) {
			return jobAudience{}, false, nil
		}
		return jobAudience{members: []audienceMember{{student: *locked.StudentID, relationship: locked.ID}}, active: true}, false, nil
	}
	if group == nil {
		return jobAudience{}, false, nil
	}
	read, err := repos.Groups.MemberIDs(ctx, tutor, *group)
	if err != nil {
		return jobAudience{}, false, err
	}
	locked, err := repos.Lessons.LockRelationships(ctx, tutor, sortedRelationships(read))
	if err != nil {
		return jobAudience{}, false, err
	}
	archivedAt, err := repos.Lessons.LockGroup(ctx, tutor, *group)
	if errors.Is(err, repositories.ErrNotFound) {
		return jobAudience{}, false, nil
	}
	if err != nil {
		return jobAudience{}, false, err
	}
	members, err := repos.Groups.MemberIDs(ctx, tutor, *group)
	if err != nil {
		return jobAudience{}, false, err
	}
	audience := jobAudience{active: archivedAt == nil}
	lockedIDs := make([]domain.RelationshipID, 0, len(locked))
	for _, row := range locked {
		lockedIDs = append(lockedIDs, row.ID)
		if row.StudentID != nil {
			audience.members = append(audience.members, audienceMember{student: *row.StudentID, relationship: row.ID})
		}
	}
	return audience, !slices.Equal(sortedRelationships(members), lockedIDs), nil
}

type jobItem struct {
	job     domain.JobName
	subject slog.Attr
}

func (j *Jobs) withAudienceRetry(ctx context.Context, item jobItem, work func(repos repositories.Repositories) error) error {
	for attempt := 1; ; attempt++ {
		err := j.service.deps.Store.WithTx(ctx, work)
		if !errors.Is(err, errAudienceMoved) {
			return err
		}
		if attempt == jobAudienceAttempts {
			j.service.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "lesson.job_deferred", slog.String("job", string(item.job)), item.subject)
			return nil
		}
	}
}

func lockSeriesOfLesson(ctx context.Context, repos repositories.Repositories, series *domain.SeriesID) (bool, error) {
	if series == nil {
		return true, nil
	}
	err := repos.Lessons.LockSeriesShared(ctx, *series)
	if errors.Is(err, repositories.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func lessonAttributes(lesson domain.LessonID) slog.Attr {
	return slog.String("lesson_id", lesson.String())
}

func (j *Jobs) EndDue(ctx context.Context, now time.Time) (int64, error) {
	deadline := j.settings.Wall().Add(j.settings.StatusBudget)
	unjoined, err := j.endUnjoinedBefore(ctx, now, deadline)
	live, liveErr := j.endLiveBefore(ctx, now, deadline)
	return unjoined + live, errors.Join(err, liveErr)
}

func (j *Jobs) endLiveBefore(ctx context.Context, now time.Time, deadline time.Time) (int64, error) {
	endedBefore := now.Add(-domain.LessonEndGrace)
	var after repositories.LessonDueToEnd
	var ended int64
	var failures []error
	for j.settings.Wall().Before(deadline) {
		batch, err := j.service.deps.Store.Repositories().Lessons.LiveDueToEnd(ctx, repositories.DueToEndQuery{
			EndedBefore: endedBefore, After: after, Limit: j.settings.StatusBatch,
		})
		if err != nil {
			failures = append(failures, err)
			break
		}
		for _, lesson := range batch {
			applied, err := j.endLive(ctx, lesson.ID, endedBefore, now)
			if err != nil {
				failures = append(failures, err)
				j.service.deps.Logger.LogAttrs(ctx, slog.LevelError, "lesson.end_failed", lessonAttributes(lesson.ID), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if applied {
				ended++
			}
		}
		if len(batch) < int(j.settings.StatusBatch) {
			break
		}
		after = batch[len(batch)-1]
	}
	j.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.ended_live", slog.Int64("count", ended))
	return ended, errors.Join(failures...)
}

func (j *Jobs) endLive(ctx context.Context, id domain.LessonID, endedBefore time.Time, now time.Time) (bool, error) {
	cycle, err := core.NewID[domain.OutboxID](j.service.deps.Random)
	if err != nil {
		return false, err
	}
	ended := false
	err = j.withAudienceRetry(ctx, jobItem{job: domain.JobLessonStatus, subject: lessonAttributes(id)}, func(repos repositories.Repositories) error {
		ended = false
		lesson, err := repos.Lessons.JobAudience(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		audience, err := lockJobAudience(ctx, repos, lesson.TutorID, lesson.RelationshipID, lesson.GroupID)
		if err != nil {
			return err
		}
		present, err := lockSeriesOfLesson(ctx, repos, lesson.SeriesID)
		if err != nil || !present {
			return err
		}
		locked, err := repos.Lessons.LockForJob(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil || locked.Status != domain.LessonLive {
			return err
		}
		ended, err = repos.Lessons.EndLiveDue(ctx, id, endedBefore, now)
		if err != nil || !ended {
			return err
		}
		if _, err := repos.CloseCurrentMeetingForCleanup(ctx, id, domain.MeetingCloseStatusJob, cycle, now); err != nil {
			return err
		}
		return repos.Signals.Entity(ctx, append([]domain.UserID{lesson.TutorID}, audience.students()...), domain.EntityLesson, domain.EntityID(id))
	})
	return ended, err
}

func (j *Jobs) EndUnjoined(ctx context.Context, now time.Time) (int64, error) {
	return j.endUnjoinedBefore(ctx, now, j.settings.Wall().Add(j.settings.StatusBudget))
}

func (j *Jobs) endUnjoinedBefore(ctx context.Context, now time.Time, deadline time.Time) (int64, error) {
	endedBefore := now.Add(-domain.LessonEndGrace)
	var after repositories.LessonDueToEnd
	var ended int64
	var failures []error
	for j.settings.Wall().Before(deadline) {
		batch, err := j.service.deps.Store.Repositories().Lessons.DueToEnd(ctx, repositories.DueToEndQuery{
			EndedBefore: endedBefore, After: after, Limit: j.settings.StatusBatch,
		})
		if err != nil {
			failures = append(failures, err)
			break
		}
		for _, lesson := range batch {
			applied, err := j.endLesson(ctx, lesson.ID, endedBefore, now)
			if err != nil {
				failures = append(failures, err)
				j.service.deps.Logger.LogAttrs(ctx, slog.LevelError, "lesson.end_failed", lessonAttributes(lesson.ID), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if applied {
				ended++
			}
		}
		if len(batch) < int(j.settings.StatusBatch) {
			break
		}
		after = batch[len(batch)-1]
	}
	j.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.ended_unjoined", slog.Int64("count", ended))
	return ended, errors.Join(failures...)
}

func (j *Jobs) endLesson(ctx context.Context, id domain.LessonID, endedBefore time.Time, now time.Time) (bool, error) {
	cycle, err := core.NewID[domain.OutboxID](j.service.deps.Random)
	if err != nil {
		return false, err
	}
	ended := false
	var tutor domain.UserID
	err = j.withAudienceRetry(ctx, jobItem{job: domain.JobLessonStatus, subject: lessonAttributes(id)}, func(repos repositories.Repositories) error {
		ended = false
		lesson, err := repos.Lessons.JobAudience(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		tutor = lesson.TutorID
		audience, err := lockJobAudience(ctx, repos, lesson.TutorID, lesson.RelationshipID, lesson.GroupID)
		if err != nil {
			return err
		}
		present, err := lockSeriesOfLesson(ctx, repos, lesson.SeriesID)
		if err != nil || !present {
			return err
		}
		locked, err := repos.Lessons.LockForJob(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil || locked.Status != domain.LessonScheduled {
			return err
		}
		ended, err = repos.Lessons.EndUnjoined(ctx, id, endedBefore, now)
		if err != nil || !ended {
			return err
		}
		students := audience.students()
		if err := repos.Lessons.FreezeRoster(ctx, id, students, now); err != nil {
			return err
		}
		if _, err := repos.CloseCurrentMeetingForCleanup(ctx, id, domain.MeetingCloseStatusJob, cycle, now); err != nil {
			return err
		}
		return repos.Signals.Entity(ctx, append([]domain.UserID{lesson.TutorID}, students...), domain.EntityLesson, domain.EntityID(id))
	})
	if err != nil {
		return false, err
	}
	if ended {
		j.service.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "lesson.ended_unjoined", tutorAttribute(tutor), lessonAttributes(id))
	}
	return ended, nil
}

type reminderTally struct {
	lessons    int64
	recipients int64
	created    []queuedNotification
}

type queuedNotification struct {
	id        domain.NotificationID
	recipient domain.UserID
	event     domain.NotificationEvent
}

func (j *Jobs) QueueReminders(ctx context.Context, now time.Time) (int64, error) {
	var total int64
	var failures []error
	deadline := j.settings.Wall().Add(j.settings.ReminderBudget)
	for _, kind := range domain.ReminderKinds() {
		queued, err := j.queueReminders(ctx, kind, now, deadline)
		total += queued
		if err != nil {
			failures = append(failures, err)
		}
	}
	return total, errors.Join(failures...)
}

func (j *Jobs) queueReminders(ctx context.Context, kind domain.ReminderKind, now time.Time, deadline time.Time) (int64, error) {
	windowStart, windowEnd := kind.Window(now)
	after := repositories.ReminderCandidate{StartsAt: windowStart}
	var tally reminderTally
	var failures []error
	for j.settings.Wall().Before(deadline) {
		batch, err := j.service.deps.Store.Repositories().Lessons.ReminderCandidates(ctx, repositories.ReminderQuery{
			DayBefore: kind.DayBefore, WindowStart: windowStart, WindowEnd: windowEnd, After: after, Limit: j.settings.ReminderBatch,
		})
		if err != nil {
			failures = append(failures, err)
			break
		}
		for _, candidate := range batch {
			if err := j.remind(ctx, kind, candidate.ID, windowStart, windowEnd, now, &tally); err != nil {
				failures = append(failures, err)
				j.service.deps.Logger.LogAttrs(ctx, slog.LevelError, "lesson.reminder_failed", lessonAttributes(candidate.ID),
					slog.String("kind", core.ErrorKind(err)))
			}
		}
		if len(batch) < int(j.settings.ReminderBatch) {
			break
		}
		after = batch[len(batch)-1]
	}
	for _, created := range tally.created {
		j.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "notification.created", slog.String("event", string(domain.NotificationLessonReminder1h)),
			slog.String("target_user_id", created.recipient.String()), slog.String("notification_id", created.id.String()))
	}
	j.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.reminders_queued", slog.String("reminder", string(kind.Event)),
		slog.Int64("lessons", tally.lessons), slog.Int64("recipients", tally.recipients))
	return tally.lessons, errors.Join(failures...)
}

func (j *Jobs) remind(
	ctx context.Context, kind domain.ReminderKind, id domain.LessonID, windowStart time.Time, windowEnd time.Time, now time.Time, tally *reminderTally,
) error {
	var queued reminderTally
	err := j.withAudienceRetry(ctx, jobItem{job: domain.JobLessonReminders, subject: lessonAttributes(id)}, func(repos repositories.Repositories) error {
		queued = reminderTally{}
		lesson, err := repos.Lessons.JobAudience(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		audience, err := lockJobAudience(ctx, repos, lesson.TutorID, lesson.RelationshipID, lesson.GroupID)
		if err != nil {
			return err
		}
		present, err := lockSeriesOfLesson(ctx, repos, lesson.SeriesID)
		if err != nil || !present {
			return err
		}
		locked, err := repos.Lessons.LockForJob(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !reminderDue(kind, locked, windowStart, windowEnd) {
			return nil
		}
		reminder := reminderRows{kind: kind, lesson: lesson, revision: locked.ScheduleRevision, now: now}
		if err := j.service.queueReminderRows(ctx, repos, reminder, audience, &queued); err != nil {
			return err
		}
		_, err = repos.Lessons.MarkReminded(ctx, id, kind.DayBefore, locked.ScheduleRevision)
		return err
	})
	if err != nil {
		return err
	}
	tally.lessons += queued.lessons
	tally.recipients += queued.recipients
	tally.created = append(tally.created, queued.created...)
	return nil
}

func reminderDue(kind domain.ReminderKind, lesson repositories.LockedJobLesson, windowStart time.Time, windowEnd time.Time) bool {
	reminded := lesson.Reminded1hRevision
	if kind.DayBefore {
		reminded = lesson.Reminded24hRevision
	}
	if lesson.Status != domain.LessonScheduled || (reminded != nil && *reminded == lesson.ScheduleRevision) {
		return false
	}
	return lesson.StartsAt.After(windowStart) && !lesson.StartsAt.After(windowEnd)
}

type reminderRows struct {
	kind     domain.ReminderKind
	lesson   repositories.JobLessonAudience
	revision int32
	now      time.Time
}

func (s *Service) queueReminderRows(
	ctx context.Context, repos repositories.Repositories, reminder reminderRows, audience jobAudience, queued *reminderTally,
) error {
	lesson := reminder.lesson
	var oneToOneStudent *domain.UserID
	if lesson.RelationshipID != nil && len(audience.members) == 1 {
		oneToOneStudent = &audience.members[0].student
	}
	recipients := []notificationTarget{{user: lesson.TutorID, student: oneToOneStudent}}
	for _, member := range audience.members {
		student, relationship := member.student, member.relationship
		recipients = append(recipients, notificationTarget{user: student, student: &student, relationship: &relationship})
	}
	for _, recipient := range recipients {
		if err := s.queueReminderEmail(ctx, repos, reminder, recipient.user); err != nil {
			return err
		}
		if !reminder.kind.InApp {
			continue
		}
		created, err := s.insertReminderRow(ctx, repos, reminder, recipient)
		if err != nil {
			return err
		}
		if created != nil {
			queued.created = append(queued.created, *created)
		}
	}
	queued.lessons++
	queued.recipients += int64(len(recipients))
	return nil
}

type notificationTarget struct {
	user         domain.UserID
	student      *domain.UserID
	relationship *domain.RelationshipID
}

func (s *Service) queueReminderEmail(ctx context.Context, repos repositories.Repositories, reminder reminderRows, recipient domain.UserID) error {
	payload, err := encodeJSON(domain.LessonReminderPayload{
		Lesson: hexID(reminder.lesson.ID), Recipient: hexID(recipient), Revision: reminder.revision,
	})
	if err != nil {
		return err
	}
	return s.queueLessonEmail(ctx, repos, lessonEmail{
		kind: reminder.kind.Event, priority: domain.PriorityHigh, payload: payload,
		key: domain.LessonReminderKey(reminder.kind.Event, reminder.lesson.ID, recipient, reminder.revision), now: reminder.now,
	})
}

type lessonEmail struct {
	kind     domain.OutboxKind
	priority domain.OutboxPriority
	payload  []byte
	key      string
	now      time.Time
}

func (s *Service) queueLessonEmail(ctx context.Context, repos repositories.Repositories, email lessonEmail) error {
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: email.priority, Kind: email.kind, Payload: email.payload,
		IdempotencyKey: email.key, NextAttemptAt: email.now, CreatedAt: email.now,
	})
	return err
}

func (s *Service) insertReminderRow(
	ctx context.Context, repos repositories.Repositories, reminder reminderRows, target notificationTarget,
) (*queuedNotification, error) {
	return s.insertLessonRow(ctx, repos, lessonRow{
		event: domain.NotificationLessonReminder1h, tutor: reminder.lesson.TutorID, subject: reminder.lesson.ID, target: target,
		key: domain.LessonReminderKey(reminder.kind.Event, reminder.lesson.ID, target.user, reminder.revision), now: reminder.now,
	})
}

type lessonRow struct {
	event   domain.NotificationEvent
	tutor   domain.UserID
	subject domain.LessonID
	target  notificationTarget
	key     string
	now     time.Time
}

func (s *Service) insertLessonRow(ctx context.Context, repos repositories.Repositories, row lessonRow) (*queuedNotification, error) {
	id, err := core.NewID[domain.NotificationID](s.deps.Random)
	if err != nil {
		return nil, err
	}
	inserted, err := repos.Notifications.Insert(ctx, repositories.NewNotification{
		ID: id, UserID: row.target.user, TutorID: row.tutor, StudentID: row.target.student, Event: row.event,
		SubjectID: domain.EntityID(row.subject), RelationshipID: row.target.relationship, IdempotencyKey: row.key, CreatedAt: row.now,
	})
	if err != nil || !inserted {
		return nil, err
	}
	if err := repos.Signals.Notification(ctx, row.target.user, id); err != nil {
		return nil, err
	}
	return &queuedNotification{id: id, recipient: row.target.user, event: row.event}, nil
}

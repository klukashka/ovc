package meetings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
)

var (
	ErrNotFound          = errors.New("the lesson is outside the caller's scope")
	ErrUnauthenticated   = errors.New("the session or the account no longer allows the action")
	ErrOutsideJoinWindow = errors.New("the lesson is outside its join window")
	ErrLessonNotLive     = errors.New("the lesson has no usable room for this action")
	ErrVideoUnavailable  = errors.New("the video workflow is not available in this deployment")
	ErrConflict          = errors.New("the audience kept changing while the request held its locks")
	errHintPayload       = errors.New("the ended hint effect has an invalid payload")
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "the rate limit is reached"
}

const (
	joinAttempts         = 4
	defaultHintPermits   = 4
	defaultHintWait      = 2 * time.Second
	defaultRequestBudget = 30 * time.Second
)

type LifecycleDependencies struct {
	Rooms        *Rooms
	Store        *repositories.Store
	Provider     meeting.Provider
	Clock        clock.Source
	Random       core.RandomSource
	Keys         core.Keys
	Logger       *slog.Logger
	Enabled      bool
	Embed        *core.Switch
	ProviderCall time.Duration
	HintPermits  int
	HintWait     time.Duration
	Budget       time.Duration
}

type Lifecycle struct {
	deps  LifecycleDependencies
	hints chan struct{}
}

func NewLifecycle(dependencies LifecycleDependencies) *Lifecycle {
	permits := dependencies.HintPermits
	if permits <= 0 {
		permits = defaultHintPermits
	}
	if dependencies.HintWait <= 0 {
		dependencies.HintWait = defaultHintWait
	}
	if dependencies.Budget <= 0 {
		dependencies.Budget = defaultRequestBudget
	}
	return &Lifecycle{deps: dependencies, hints: make(chan struct{}, permits)}
}

type Actor struct {
	UserID    domain.UserID
	SessionID domain.SessionID
	Role      domain.Role
}

type JoinTicket struct {
	Link         meeting.JoinLink
	EmbedAllowed bool
}

func (l *Lifecycle) Join(ctx context.Context, actor Actor, id domain.LessonID) (ticket JoinTicket, err error) {
	defer func() { l.reportJoin(ctx, actor, id, err) }()
	if !l.deps.Enabled {
		return JoinTicket{}, ErrVideoUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, l.deps.Budget)
	defer cancel()
	if err := l.admitJoin(ctx, actor, id); err != nil {
		return JoinTicket{}, err
	}
	return l.joinWithRetry(ctx, actor, id, joinWindowAllows)
}

func (l *Lifecycle) reportJoin(ctx context.Context, actor Actor, id domain.LessonID, err error) {
	attributes := []slog.Attr{slog.String("lesson_id", id.String()), slog.String("user_id", actor.UserID.String()), slog.String("role", string(actor.Role))}
	if err == nil {
		l.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "meeting.join.authorized", attributes...)
		return
	}
	l.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "meeting.join.refused", append(attributes, slog.String("reason", refusalReason(err)))...)
}

func refusalReason(err error) string {
	var throttled *ThrottledError
	switch {
	case errors.Is(err, ErrNotFound):
		return string(domain.ErrorNotFound)
	case errors.Is(err, ErrUnauthenticated):
		return string(domain.ErrorUnauthenticated)
	case errors.Is(err, ErrOutsideJoinWindow):
		return string(domain.ErrorOutsideJoinWindow)
	case errors.Is(err, ErrLessonNotLive):
		return string(domain.ErrorLessonNotLive)
	case errors.Is(err, ErrAdmissionCapacity), errors.Is(err, ErrVideoUnavailable):
		return string(domain.ErrorServiceUnavailable)
	case errors.Is(err, ErrProviderUnavailable):
		return string(domain.ErrorProviderUnavailable)
	case errors.As(err, &throttled):
		return string(domain.ErrorRateLimited)
	}
	return core.ErrorKind(err)
}

func (l *Lifecycle) joinWithRetry(ctx context.Context, actor Actor, id domain.LessonID, allowed statusRule) (JoinTicket, error) {
	for range joinAttempts {
		ticket, err := l.joinOnce(ctx, actor, id, allowed)
		if errors.Is(err, errRoomSuperseded) || errors.Is(err, errAudienceMoved) {
			continue
		}
		return ticket, err
	}
	return JoinTicket{}, ErrProviderUnavailable
}

func (l *Lifecycle) joinOnce(ctx context.Context, actor Actor, id domain.LessonID, allowed statusRule) (JoinTicket, error) {
	if actor.Role == domain.RoleTutor {
		return l.tutorJoin(ctx, actor, id, allowed)
	}
	return l.studentJoin(ctx, actor, id)
}

func (l *Lifecycle) admitJoin(ctx context.Context, actor Actor, id domain.LessonID) error {
	return l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		var now time.Time
		var err error
		if actor.Role == domain.RoleTutor {
			now, err = l.lockTutor(ctx, repos, actor)
			if err == nil {
				if _, targetErr := repos.Lessons.TutorTarget(ctx, actor.UserID, id); errors.Is(targetErr, repositories.ErrNotFound) {
					err = ErrNotFound
				} else {
					err = targetErr
				}
			}
		} else {
			now, err = l.lockStudent(ctx, repos, actor)
			if err == nil {
				_, err = lockStudentAudience(ctx, repos, actor.UserID, id)
			}
		}
		if err != nil {
			return err
		}
		return l.reserve(ctx, repos, domain.PurposeMeetingJoin, actor.UserID, now, domain.MeetingJoinWindow, domain.MeetingJoinsPerMinute)
	})
}

func (l *Lifecycle) admitWrite(ctx context.Context, actor Actor, id domain.LessonID) error {
	return l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := l.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		if _, err := repos.Lessons.TutorTarget(ctx, actor.UserID, id); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return l.reserve(ctx, repos, domain.PurposeLessonWrites, actor.UserID, now, domain.LessonWriteWindow, domain.LessonWritesPerHour)
	})
}

func (l *Lifecycle) reserve(ctx context.Context, repos repositories.Repositories, purpose domain.RateLimitPurpose, user domain.UserID, now time.Time, size time.Duration, limit int32) error {
	window, retryAfter := domain.FixedWindow(now, size)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: purpose, Subject: l.deps.Keys.ThrottleSubject(string(purpose), user.String()), WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > limit {
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (l *Lifecycle) lockTutor(ctx context.Context, repos repositories.Repositories, actor Actor) (time.Time, error) {
	if err := repos.TutorProfiles.Lock(ctx, actor.UserID); errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	} else if err != nil {
		return time.Time{}, err
	}
	now, err := l.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return time.Time{}, err
	}
	if err := checkSession(ctx, repos, actor, now); err != nil {
		return time.Time{}, err
	}
	account, err := repos.Users.ByID(ctx, actor.UserID)
	if err != nil {
		return time.Time{}, err
	}
	if account.State != domain.AccountActive {
		return time.Time{}, ErrUnauthenticated
	}
	return now, nil
}

func (l *Lifecycle) lockStudent(ctx context.Context, repos repositories.Repositories, actor Actor) (time.Time, error) {
	locked, err := repos.Users.LockCredentials(ctx, actor.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	now, err := l.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return time.Time{}, err
	}
	if err := checkSession(ctx, repos, actor, now); err != nil {
		return time.Time{}, err
	}
	if locked.State != domain.AccountActive && locked.State != domain.AccountUnverified {
		return time.Time{}, ErrUnauthenticated
	}
	return now, nil
}

func checkSession(ctx context.Context, repos repositories.Repositories, actor Actor, now time.Time) error {
	session, err := repos.Sessions.ByID(ctx, actor.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if session.UserID != actor.UserID || session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
		return ErrUnauthenticated
	}
	return nil
}

func lockStudentAudience(ctx context.Context, repos repositories.Repositories, student domain.UserID, id domain.LessonID) (repositories.JobLessonAudience, error) {
	lesson, err := repos.Lessons.JobAudience(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return lesson, ErrNotFound
	}
	if err != nil {
		return lesson, err
	}
	if lesson.RelationshipID != nil {
		link, err := repos.Lessons.LockRelationship(ctx, lesson.TutorID, *lesson.RelationshipID)
		if errors.Is(err, repositories.ErrNotFound) {
			return lesson, ErrNotFound
		}
		if err != nil {
			return lesson, err
		}
		if link.StudentID == nil || *link.StudentID != student || (link.State != domain.RelationshipActive && link.State != domain.RelationshipProvisional) {
			return lesson, ErrNotFound
		}
		return lesson, nil
	}
	if lesson.GroupID == nil {
		return lesson, ErrNotFound
	}
	own, err := repos.Lessons.StudentLink(ctx, lesson.TutorID, student)
	if errors.Is(err, repositories.ErrNotFound) {
		return lesson, ErrNotFound
	}
	if err != nil {
		return lesson, err
	}
	link, err := repos.Lessons.LockRelationship(ctx, lesson.TutorID, own)
	if errors.Is(err, repositories.ErrNotFound) {
		return lesson, ErrNotFound
	}
	if err != nil {
		return lesson, err
	}
	if link.State != domain.RelationshipActive || link.StudentID == nil || *link.StudentID != student {
		return lesson, ErrNotFound
	}
	if _, err := repos.Lessons.LockGroup(ctx, lesson.TutorID, *lesson.GroupID); errors.Is(err, repositories.ErrNotFound) {
		return lesson, ErrNotFound
	} else if err != nil {
		return lesson, err
	}
	member, err := repos.Lessons.IsCurrentMember(ctx, *lesson.GroupID, student)
	if err != nil {
		return lesson, err
	}
	if !member {
		return lesson, ErrNotFound
	}
	return lesson, nil
}

func lockLessonSeries(ctx context.Context, repos repositories.Repositories, series *domain.SeriesID) error {
	if series == nil {
		return nil
	}
	err := repos.Lessons.LockSeriesShared(ctx, *series)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

type statusRule func(repositories.LockedMeetingLesson, time.Time) error

func joinWindowAllows(lesson repositories.LockedMeetingLesson, now time.Time) error {
	if lesson.Status != domain.LessonScheduled && lesson.Status != domain.LessonLive {
		return ErrOutsideJoinWindow
	}
	end := lesson.StartsAt.Add(time.Duration(lesson.DurationMinutes) * time.Minute)
	if now.Before(lesson.StartsAt.Add(-domain.MeetingJoinWindowLead)) || !now.Before(end.Add(domain.LessonEndGrace)) {
		return ErrOutsideJoinWindow
	}
	return nil
}

func liveAllows(lesson repositories.LockedMeetingLesson, _ time.Time) error {
	if lesson.Status != domain.LessonLive {
		return ErrLessonNotLive
	}
	return nil
}

type tutorScope struct {
	account  repositories.User
	lesson   repositories.LockedMeetingLesson
	students []domain.UserID
	now      time.Time
}

func (l *Lifecycle) lockTutorScope(ctx context.Context, repos repositories.Repositories, actor Actor, id domain.LessonID) (tutorScope, error) {
	if _, err := l.lockTutor(ctx, repos, actor); err != nil {
		return tutorScope{}, err
	}
	account, err := repos.Users.ByID(ctx, actor.UserID)
	if err != nil {
		return tutorScope{}, err
	}
	target, err := repos.Lessons.TutorTarget(ctx, actor.UserID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorScope{}, ErrNotFound
	}
	if err != nil {
		return tutorScope{}, err
	}
	students, moved, err := lockRoomAudience(ctx, repos, actor.UserID, target.RelationshipID, target.GroupID)
	if err != nil {
		return tutorScope{}, err
	}
	if moved {
		return tutorScope{}, errAudienceMoved
	}
	if err := lockLessonSeries(ctx, repos, target.SeriesID); err != nil {
		return tutorScope{}, err
	}
	lesson, err := repos.Lessons.LockForMeeting(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorScope{}, ErrNotFound
	}
	if err != nil {
		return tutorScope{}, err
	}
	now, err := l.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return tutorScope{}, err
	}
	return tutorScope{account: account, lesson: lesson, students: students, now: now}, nil
}

func (l *Lifecycle) tutorJoin(ctx context.Context, actor Actor, id domain.LessonID, allowed statusRule) (JoinTicket, error) {
	var scope tutorScope
	guard := func(ctx context.Context, repos repositories.Repositories) (RoomSubject, error) {
		locked, err := l.lockTutorScope(ctx, repos, actor, id)
		if err != nil {
			return RoomSubject{}, err
		}
		if err := allowed(locked.lesson, locked.now); err != nil {
			return RoomSubject{}, err
		}
		scope = locked
		return RoomSubject{Lesson: id, Tutor: actor.UserID, Now: locked.now}, nil
	}
	publish := func(ctx context.Context, repos repositories.Repositories, subject RoomSubject) error {
		if scope.lesson.Status == domain.LessonScheduled {
			if _, err := repos.Lessons.MarkLive(ctx, id, subject.Now); err != nil {
				return err
			}
			if err := repos.Lessons.FreezeRoster(ctx, id, scope.students, subject.Now); err != nil {
				return err
			}
		}
		return repos.Signals.Entity(ctx, append([]domain.UserID{actor.UserID}, scope.students...), domain.EntityLesson, domain.EntityID(id))
	}
	room, err := l.deps.Rooms.Ensure(ctx, guard, publish)
	if err != nil {
		return JoinTicket{}, err
	}
	participant, err := l.participant(ctx, room, actor, guard)
	if err != nil {
		return JoinTicket{}, err
	}
	link, err := l.link(ctx, room.Meeting, domain.Participant{ID: participant, Name: scope.account.Name, Role: domain.ParticipantModerator})
	if err != nil {
		return JoinTicket{}, err
	}
	err = l.release(ctx, room, guard, nil)
	if err != nil {
		return JoinTicket{}, err
	}
	return JoinTicket{Link: link, EmbedAllowed: l.deps.Embed.On()}, nil
}

func (l *Lifecycle) participant(ctx context.Context, room Room, actor Actor, guard RoomGuard) (domain.ParticipantID, error) {
	candidate, err := core.NewID[domain.ParticipantID](l.deps.Random)
	if err != nil {
		return domain.ParticipantID{}, err
	}
	var participant domain.ParticipantID
	err = l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := guard(ctx, repos)
		if err != nil {
			return err
		}
		participant, err = repos.Meetings.Participant(ctx, room.Identity.LessonID, actor.UserID, candidate, subject.Now)
		return err
	})
	return participant, err
}

func (l *Lifecycle) link(ctx context.Context, room domain.Meeting, participant domain.Participant) (meeting.JoinLink, error) {
	call, cancel := context.WithTimeout(ctx, l.deps.ProviderCall)
	defer cancel()
	link, err := l.deps.Provider.JoinLink(call, room, participant)
	if err == nil {
		return link, nil
	}
	if cause := ctx.Err(); cause != nil {
		return meeting.JoinLink{}, cause
	}
	if errors.Is(err, meeting.FailureNotFound) {
		return meeting.JoinLink{}, errRoomSuperseded
	}
	return meeting.JoinLink{}, fmt.Errorf("%w: %w", ErrProviderUnavailable, err)
}

func (l *Lifecycle) release(ctx context.Context, room Room, guard RoomGuard, roster func(context.Context, repositories.Repositories, time.Time) error) error {
	return l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := guard(ctx, repos)
		if err != nil {
			return err
		}
		if roster != nil {
			if err := roster(ctx, repos, subject.Now); err != nil {
				return err
			}
		}
		joined, err := repos.Meetings.FirstJoinUnderLessonLock(ctx, room.Identity, room.Meeting.CreateTime, subject.Now)
		if err != nil {
			return err
		}
		if joined != domain.MeetingMutationApplied && joined != domain.MeetingAlreadyJoined {
			return errRoomSuperseded
		}
		return nil
	})
}

func (l *Lifecycle) studentJoin(ctx context.Context, actor Actor, id domain.LessonID) (JoinTicket, error) {
	var account string
	var room Room
	guard := func(ctx context.Context, repos repositories.Repositories) (RoomSubject, error) {
		now, err := l.lockStudent(ctx, repos, actor)
		if err != nil {
			return RoomSubject{}, err
		}
		audience, err := lockStudentAudience(ctx, repos, actor.UserID, id)
		if err != nil {
			return RoomSubject{}, err
		}
		if err := lockLessonSeries(ctx, repos, audience.SeriesID); err != nil {
			return RoomSubject{}, err
		}
		lesson, err := repos.Lessons.LockForMeeting(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return RoomSubject{}, ErrNotFound
		}
		if err != nil {
			return RoomSubject{}, err
		}
		if err := liveAllows(lesson, now); err != nil {
			return RoomSubject{}, err
		}
		end := lesson.StartsAt.Add(time.Duration(lesson.DurationMinutes) * time.Minute)
		if !now.Before(end.Add(domain.LessonEndGrace)) {
			return RoomSubject{}, ErrLessonNotLive
		}
		current, err := repos.Meetings.Current(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return RoomSubject{}, ErrLessonNotLive
		}
		if err != nil {
			return RoomSubject{}, err
		}
		if current.State != domain.MeetingOpen {
			return RoomSubject{}, ErrLessonNotLive
		}
		identity := domain.MeetingIdentity{LessonID: id, ID: current.ID, ProviderMeetingID: current.ProviderMeetingID}
		if room.Identity != (domain.MeetingIdentity{}) && (room.Identity != identity || room.Meeting.CreateTime != *current.CreateTime) {
			return RoomSubject{}, errRoomSuperseded
		}
		profile, err := repos.Users.ByID(ctx, actor.UserID)
		if err != nil {
			return RoomSubject{}, err
		}
		account = profile.Name
		room = Room{Identity: identity, Meeting: domain.Meeting{ID: current.ProviderMeetingID, Name: domain.MeetingRoomName, CreateTime: *current.CreateTime}}
		return RoomSubject{Lesson: id, Tutor: audience.TutorID, Now: now}, nil
	}
	participant, err := l.participant(ctx, Room{Identity: domain.MeetingIdentity{LessonID: id}}, actor, guard)
	if err != nil {
		return JoinTicket{}, err
	}
	if err := l.confirmStudentRoom(ctx, room); err != nil {
		return JoinTicket{}, err
	}
	link, err := l.link(ctx, room.Meeting, domain.Participant{ID: participant, Name: account, Role: domain.ParticipantViewer})
	if err != nil {
		return JoinTicket{}, err
	}
	roster := func(ctx context.Context, repos repositories.Repositories, now time.Time) error {
		return repos.Lessons.FreezeRoster(ctx, id, []domain.UserID{actor.UserID}, now)
	}
	if err := l.release(ctx, room, guard, roster); err != nil {
		return JoinTicket{}, err
	}
	return JoinTicket{Link: link, EmbedAllowed: l.deps.Embed.On()}, nil
}

func (l *Lifecycle) confirmStudentRoom(ctx context.Context, room Room) error {
	call, cancel := context.WithTimeout(ctx, l.deps.ProviderCall)
	defer cancel()
	info, err := l.deps.Provider.MeetingInfo(call, room.Meeting)
	if err != nil {
		if cause := ctx.Err(); cause != nil {
			return cause
		}
		return fmt.Errorf("%w: %w", ErrProviderUnavailable, err)
	}
	switch info.Status {
	case domain.MeetingCreated, domain.MeetingRunning:
		return nil
	case domain.MeetingNotFound:
		evidence, cancelEvidence := l.deps.Rooms.evidenceContext(ctx)
		defer cancelEvidence()
		reportEvidenceFailure(ctx, l.deps.Logger, l.deps.Rooms.Retire(evidence, room.Identity, domain.MeetingCloseProviderNotFound))
		return ErrLessonNotLive
	}
	return ErrProviderUnavailable
}

func (l *Lifecycle) End(ctx context.Context, actor Actor, id domain.LessonID) error {
	if !l.deps.Enabled {
		return ErrVideoUnavailable
	}
	if err := l.admitWrite(ctx, actor, id); err != nil {
		return err
	}
	cycle, err := core.NewID[domain.OutboxID](l.deps.Random)
	if err != nil {
		return err
	}
	ended := false
	for range joinAttempts {
		err = l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
			ended = false
			scope, err := l.lockTutorScope(ctx, repos, actor, id)
			if err != nil {
				return err
			}
			switch scope.lesson.Status {
			case domain.LessonEnded:
				return nil
			case domain.LessonLive:
			case domain.LessonScheduled, domain.LessonCancelled:
				return ErrLessonNotLive
			default:
				return ErrLessonNotLive
			}
			if ended, err = repos.Lessons.EndLive(ctx, id, scope.now); err != nil {
				return err
			}
			if _, err := repos.CloseCurrentMeetingForCleanup(ctx, id, domain.MeetingCloseTutorEnd, cycle, scope.now); err != nil {
				return err
			}
			return repos.Signals.Entity(ctx, append([]domain.UserID{actor.UserID}, scope.students...), domain.EntityLesson, domain.EntityID(id))
		})
		if !errors.Is(err, errAudienceMoved) {
			break
		}
	}
	if errors.Is(err, errAudienceMoved) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if ended {
		l.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "meeting.lesson.ended", slog.String("lesson_id", id.String()), slog.String("reason", string(domain.MeetingCloseTutorEnd)))
	}
	return nil
}

func (l *Lifecycle) Restart(ctx context.Context, actor Actor, id domain.LessonID) (JoinTicket, error) {
	if !l.deps.Enabled {
		return JoinTicket{}, ErrVideoUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, l.deps.Budget)
	defer cancel()
	if err := l.admitWrite(ctx, actor, id); err != nil {
		return JoinTicket{}, err
	}
	cycle, err := core.NewID[domain.OutboxID](l.deps.Random)
	if err != nil {
		return JoinTicket{}, err
	}
	for range joinAttempts {
		err = l.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
			scope, err := l.lockTutorScope(ctx, repos, actor, id)
			if err != nil {
				return err
			}
			if err := liveAllows(scope.lesson, scope.now); err != nil {
				return err
			}
			closed, err := repos.CloseOpenMeetingForCleanup(ctx, id, domain.MeetingCloseTutorRestart, cycle, scope.now)
			if err != nil || !closed {
				return err
			}
			return repos.Signals.Entity(ctx, append([]domain.UserID{actor.UserID}, scope.students...), domain.EntityLesson, domain.EntityID(id))
		})
		if !errors.Is(err, errAudienceMoved) {
			break
		}
	}
	if errors.Is(err, errAudienceMoved) {
		return JoinTicket{}, ErrConflict
	}
	if err != nil {
		return JoinTicket{}, err
	}
	l.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "meeting.room.restart_requested", slog.String("lesson_id", id.String()))
	return l.joinWithRetry(ctx, actor, id, liveAllows)
}

func (l *Lifecycle) AcceptEndedHint(ctx context.Context, key string, recordingMarks bool) {
	if !l.deps.Enabled {
		return
	}
	routing, valid := domain.ParseUUID[domain.MeetingRoutingKey](key)
	if !valid {
		return
	}
	select {
	case l.hints <- struct{}{}:
		defer func() { <-l.hints }()
	default:
		l.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "meeting.hint.declined", slog.String("reason", "saturated"))
		return
	}
	ctx, cancel := context.WithTimeout(ctx, l.deps.HintWait)
	defer cancel()
	accepted, err := l.acceptHint(ctx, routing)
	if err != nil {
		level := slog.LevelWarn
		if errors.Is(err, context.Canceled) {
			level = slog.LevelDebug
		}
		l.deps.Logger.LogAttrs(ctx, level, "meeting.hint.failed", slog.String("kind", core.ErrorKind(err)))
		return
	}
	l.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "meeting.hint.received", slog.Bool("accepted", accepted), slog.Bool("recording_marks", recordingMarks))
}

func (l *Lifecycle) acceptHint(ctx context.Context, routing domain.MeetingRoutingKey) (bool, error) {
	id, err := core.NewID[domain.OutboxID](l.deps.Random)
	if err != nil {
		return false, err
	}
	now, err := l.deps.Clock.Instant(ctx)
	if err != nil {
		return false, err
	}
	return l.deps.Store.Repositories().Meetings.AcceptEndedHint(ctx, routing, id, now)
}

func (l *Lifecycle) CheckEndedHint(ctx context.Context, row outbox.Row, now time.Time) (domain.OutboxDoneReason, error) {
	var payload domain.MeetingEndedHintPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return "", domain.PermanentEffect(err)
	}
	lesson, lessonValid := domain.ParseUUID[domain.LessonID](payload.Lesson)
	room, roomValid := domain.ParseUUID[domain.MeetingID](payload.Meeting)
	if row.Kind != domain.KindMeetingEndedHint || !lessonValid || !roomValid {
		return "", domain.PermanentEffect(errHintPayload)
	}
	repos := l.deps.Store.Repositories()
	current, err := repos.Meetings.ByID(ctx, lesson, room)
	if errors.Is(err, repositories.ErrNotFound) {
		return domain.DoneObsolete, nil
	}
	if err != nil {
		return "", err
	}
	checks, err := repos.Meetings.EndedHintChecks(ctx, lesson, room)
	if err != nil {
		return "", err
	}
	if current.State != domain.MeetingOpen || current.CreateTime == nil || checks != payload.Check {
		return domain.DoneObsolete, nil
	}
	info, err := l.deps.Provider.MeetingInfo(ctx, domain.Meeting{ID: current.ProviderMeetingID, CreateTime: *current.CreateTime})
	if err != nil {
		return "", fmt.Errorf("confirm the ended hint: %w", err)
	}
	identity := domain.MeetingIdentity{LessonID: lesson, ID: room, ProviderMeetingID: current.ProviderMeetingID}
	if info.Status == domain.MeetingNotFound {
		if err := l.deps.Rooms.Retire(ctx, identity, domain.MeetingCloseProviderNotFound); err != nil {
			return "", err
		}
		l.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "meeting.hint.confirmed", slog.String("lesson_id", lesson.String()))
		return domain.DoneSent, nil
	}
	next, err := core.NewID[domain.OutboxID](l.deps.Random)
	if err != nil {
		return "", err
	}
	if err := repos.Meetings.AdvanceEndedHint(ctx, identity, payload.Check, next, now); err != nil {
		return "", err
	}
	return domain.DoneSent, nil
}

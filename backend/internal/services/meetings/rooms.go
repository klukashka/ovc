package meetings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrProviderUnavailable = errors.New("the meeting provider did not confirm the operation")
	ErrAdmissionCapacity   = errors.New("meeting admission capacity is exhausted")
	errRoomSuperseded      = errors.New("the room changed while the provider call ran")
	errAudienceMoved       = errors.New("the audience changed between reading and locking it")
	errTutorAdmission      = errors.New("the tutor reached the limit of unsettled meetings")
	errUncertainPause      = errors.New("the lesson has a fresh uncertain meeting")
)

const roomAttempts = 5

type RoomTiming struct {
	CreatingWait time.Duration
	Poll         time.Duration
	ProviderCall time.Duration
	Evidence     time.Duration
}

func DefaultRoomTiming() RoomTiming {
	return RoomTiming{CreatingWait: domain.MeetingCreatingWait, Poll: 50 * time.Millisecond, ProviderCall: 15 * time.Second, Evidence: 10 * time.Second}
}

type RoomDependencies struct {
	Store     *repositories.Store
	Provider  meeting.Provider
	Kind      domain.MeetingProvider
	Clock     clock.Source
	Random    core.RandomSource
	Logger    *slog.Logger
	Monotonic func() time.Time
	Timing    RoomTiming
}

type Rooms struct {
	deps    RoomDependencies
	cleanup *Service
}

func NewRooms(dependencies RoomDependencies) *Rooms {
	return &Rooms{deps: dependencies, cleanup: New(dependencies.Store, dependencies.Provider, dependencies.Kind, dependencies.Logger)}
}

type RoomSubject struct {
	Lesson domain.LessonID
	Tutor  domain.UserID
	Now    time.Time
}

type RoomGuard func(context.Context, repositories.Repositories) (RoomSubject, error)

type RoomPublication func(context.Context, repositories.Repositories, RoomSubject) error

type Room struct {
	Identity domain.MeetingIdentity
	Meeting  domain.Meeting
}

type roomAction int

const (
	roomReplan roomAction = iota
	roomReuse
	roomAwait
	roomDispatch
	roomRevoke
)

type roomPlan struct {
	action  roomAction
	room    Room
	attempt domain.MeetingCreateAttemptID
	revoke  []repositories.MeetingRevocation
	lesson  domain.LessonID
}

type roomIdentifiers struct {
	meeting  domain.MeetingID
	provider domain.ProviderMeetingID
	routing  domain.MeetingRoutingKey
	attempt  domain.MeetingCreateAttemptID
	cycle    domain.OutboxID
}

func (r *Rooms) identifiers() (roomIdentifiers, error) {
	var ids roomIdentifiers
	var failures [5]error
	ids.meeting, failures[0] = core.NewID[domain.MeetingID](r.deps.Random)
	ids.provider, failures[1] = core.NewID[domain.ProviderMeetingID](r.deps.Random)
	ids.routing, failures[2] = core.NewID[domain.MeetingRoutingKey](r.deps.Random)
	ids.attempt, failures[3] = core.NewID[domain.MeetingCreateAttemptID](r.deps.Random)
	ids.cycle, failures[4] = core.NewID[domain.OutboxID](r.deps.Random)
	return ids, errors.Join(failures[:]...)
}

func (r *Rooms) Ensure(ctx context.Context, guard RoomGuard, publish RoomPublication) (Room, error) {
	for range roomAttempts {
		plan, err := r.plan(ctx, guard)
		if errors.Is(err, errAudienceMoved) {
			continue
		}
		if err != nil {
			return Room{}, err
		}
		room, err := r.perform(ctx, plan, guard, publish)
		if err == nil {
			return room, nil
		}
		if !errors.Is(err, errRoomSuperseded) {
			return Room{}, err
		}
	}
	return Room{}, ErrProviderUnavailable
}

func (r *Rooms) perform(ctx context.Context, plan roomPlan, guard RoomGuard, publish RoomPublication) (Room, error) {
	switch plan.action {
	case roomReuse:
		return r.reuse(ctx, plan.room)
	case roomAwait:
		return Room{}, r.await(ctx, plan.room.Identity)
	case roomDispatch:
		return r.dispatch(ctx, plan, guard, publish)
	case roomRevoke:
		return Room{}, r.revoke(ctx, plan)
	case roomReplan:
		return Room{}, errRoomSuperseded
	}
	return Room{}, errRoomSuperseded
}

func reservationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	if deadline, bounded := ctx.Deadline(); bounded {
		return context.WithDeadline(detached, deadline)
	}
	return context.WithCancel(detached)
}

func (r *Rooms) plan(ctx context.Context, guard RoomGuard) (roomPlan, error) {
	if err := ctx.Err(); err != nil {
		return roomPlan{}, err
	}
	reserving, cancel := reservationContext(ctx)
	defer cancel()
	return r.reserve(reserving, guard)
}

func (r *Rooms) reserve(ctx context.Context, guard RoomGuard) (roomPlan, error) {
	ids, err := r.identifiers()
	if err != nil {
		return roomPlan{}, err
	}
	var plan roomPlan
	err = r.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := guard(ctx, repos)
		if err != nil {
			return err
		}
		plan = roomPlan{}
		current, err := repos.Meetings.Current(ctx, subject.Lesson)
		if errors.Is(err, repositories.ErrNotFound) {
			pending, err := repos.Meetings.UnconfirmedRevocations(ctx, subject.Lesson)
			if err != nil {
				return err
			}
			if len(pending) > 0 {
				plan.action, plan.revoke, plan.lesson = roomRevoke, pending, subject.Lesson
				return nil
			}
			return r.insert(ctx, repos, subject, ids, &plan)
		}
		if err != nil {
			return err
		}
		identity := domain.MeetingIdentity{LessonID: current.LessonID, ID: current.ID, ProviderMeetingID: current.ProviderMeetingID}
		plan.room = Room{Identity: identity, Meeting: domain.Meeting{ID: current.ProviderMeetingID, Name: domain.MeetingRoomName}}
		if current.State == domain.MeetingOpen {
			plan.action, plan.room.Meeting.CreateTime = roomReuse, *current.CreateTime
			return nil
		}
		if subject.Now.Sub(current.CreatedAt) < domain.MeetingCreatingStaleAfter {
			plan.action = roomAwait
			return nil
		}
		plan.action = roomReplan
		_, err = repos.CloseMeetingForCleanup(ctx, domain.MeetingExpectation{Identity: identity, State: domain.MeetingCreating}, domain.MeetingCloseProviderError, ids.cycle, subject.Now)
		return err
	})
	if errors.Is(err, repositories.ErrMeetingRecoveryCapacity) {
		r.report(ctx, "meeting.room.create_refused", slog.String("reason", "capacity"))
		return roomPlan{}, ErrAdmissionCapacity
	}
	if errors.Is(err, errUncertainPause) {
		r.report(ctx, "meeting.room.create_refused", slog.String("reason", "uncertain_pause"))
		return roomPlan{}, ErrProviderUnavailable
	}
	if errors.Is(err, errTutorAdmission) {
		r.report(ctx, "meeting.room.create_refused", slog.String("reason", "tutor_limit"))
		return roomPlan{}, ErrAdmissionCapacity
	}
	return plan, err
}

func (r *Rooms) budgetCoversCreate(ctx context.Context) bool {
	deadline, bounded := ctx.Deadline()
	return !bounded || deadline.Sub(r.deps.Monotonic()) >= r.deps.Timing.ProviderCall
}

func (r *Rooms) insert(ctx context.Context, repos repositories.Repositories, subject RoomSubject, ids roomIdentifiers, plan *roomPlan) error {
	paused, err := repos.Meetings.HasFreshUncertain(ctx, subject.Lesson, subject.Now.Add(-domain.MeetingUncertainPause), subject.Now)
	if err != nil {
		return err
	}
	if paused {
		return errUncertainPause
	}
	unsettled, err := repos.Meetings.UnsettledOfTutor(ctx, subject.Tutor)
	if err != nil {
		return err
	}
	if unsettled >= domain.MeetingUnsettledPerTutor {
		return errTutorAdmission
	}
	if err := repos.Meetings.CreateUnderLessonLock(ctx, repositories.NewMeeting{
		ID: ids.meeting, LessonID: subject.Lesson, Provider: r.deps.Kind, ProviderMeetingID: ids.provider, RoutingKey: ids.routing, Now: subject.Now,
	}); err != nil {
		return err
	}
	identity := domain.MeetingIdentity{LessonID: subject.Lesson, ID: ids.meeting, ProviderMeetingID: ids.provider}
	begun, err := repos.MeetingRecovery.BeginCreateUnderLessonLock(ctx, identity, ids.attempt, 1, subject.Now)
	if err != nil {
		return err
	}
	if begun != domain.MeetingMutationApplied {
		return errRoomSuperseded
	}
	if !r.budgetCoversCreate(ctx) {
		return ErrProviderUnavailable
	}
	plan.action, plan.attempt = roomDispatch, ids.attempt
	plan.room = Room{Identity: identity, Meeting: domain.Meeting{ID: ids.provider, Name: domain.MeetingRoomName}}
	return nil
}

func (r *Rooms) revoke(ctx context.Context, plan roomPlan) error {
	for _, pending := range plan.revoke {
		call, cancel := context.WithTimeout(ctx, r.deps.Timing.ProviderCall)
		conflict, err := r.cleanup.end(call, pending.ProviderMeetingID, *pending.CreateTime)
		cancel()
		if err != nil || conflict {
			r.report(ctx, "meeting.room.revocation_unconfirmed", slog.String("lesson_id", plan.lesson.String()), slog.Bool("conflict", conflict))
			if cause := ctx.Err(); cause != nil {
				return cause
			}
			return ErrProviderUnavailable
		}
		now, err := r.deps.Clock.Instant(ctx)
		if err != nil {
			return err
		}
		if err := r.deps.Store.Repositories().Meetings.ConfirmRevocation(ctx, plan.lesson, pending.ID, now); err != nil {
			return err
		}
		r.report(ctx, "meeting.room.revoked", slog.String("lesson_id", plan.lesson.String()))
	}
	return errRoomSuperseded
}

func (r *Rooms) evidenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), r.deps.Timing.Evidence)
}

func (r *Rooms) dispatch(ctx context.Context, plan roomPlan, guard RoomGuard, publish RoomPublication) (Room, error) {
	call, cancelCall := context.WithTimeout(context.WithoutCancel(ctx), r.deps.Timing.ProviderCall)
	defer cancelCall()
	created, createErr := r.deps.Provider.CreateMeeting(call, plan.room.Meeting)
	evidence, cancelEvidence := r.evidenceContext(ctx)
	defer cancelEvidence()
	identity := plan.room.Identity
	if createErr != nil {
		return Room{}, r.createFailed(ctx, evidence, plan, createErr)
	}
	now, err := r.deps.Clock.Instant(evidence)
	if err != nil {
		return Room{}, errors.Join(err, r.Retire(evidence, identity, domain.MeetingCloseProviderError))
	}
	recorded, err := r.deps.Store.Repositories().MeetingRecovery.RecordCreateReturned(evidence, identity.ProviderMeetingID, plan.attempt, created, now)
	if err != nil {
		return Room{}, errors.Join(err, r.Retire(evidence, identity, domain.MeetingCloseProviderError))
	}
	if recorded != domain.MeetingMutationApplied {
		if err := r.Retire(evidence, identity, domain.MeetingCloseProviderError); err != nil {
			return Room{}, err
		}
		return Room{}, errRoomSuperseded
	}
	plan.room.Meeting.CreateTime = created
	if err := r.publish(ctx, plan.room, guard, publish); err != nil {
		r.report(ctx, "meeting.room.publication_refused", slog.String("kind", core.ErrorKind(err)))
		closing, cancelClosing := r.evidenceContext(ctx)
		defer cancelClosing()
		reportEvidenceFailure(ctx, r.deps.Logger, r.Retire(closing, identity, domain.MeetingCloseProviderError))
		return Room{}, err
	}
	r.report(ctx, "meeting.room.created", slog.String("lesson_id", identity.LessonID.String()))
	return plan.room, nil
}

func (r *Rooms) createFailed(ctx, evidence context.Context, plan roomPlan, createErr error) error {
	identity := plan.room.Identity
	now, err := r.deps.Clock.Instant(evidence)
	if err == nil {
		_, err = r.deps.Store.Repositories().MeetingRecovery.MarkCreateUncertain(evidence, identity.ProviderMeetingID, plan.attempt, now)
	}
	retired := r.Retire(evidence, identity, domain.MeetingCloseProviderError)
	r.report(ctx, "meeting.room.create_failed", slog.String("kind", core.ErrorKind(createErr)))
	reportEvidenceFailure(ctx, r.deps.Logger, errors.Join(err, retired))
	if cause := ctx.Err(); cause != nil {
		return errors.Join(cause, err, retired)
	}
	return errors.Join(ErrProviderUnavailable, err, retired)
}

func reportEvidenceFailure(ctx context.Context, logger *slog.Logger, err error) {
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "meeting.room.evidence_failed", slog.String("kind", core.ErrorKind(err)))
	}
}

func (r *Rooms) publish(ctx context.Context, room Room, guard RoomGuard, publish RoomPublication) error {
	var err error
	for range 2 {
		err = r.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
			subject, err := guard(ctx, repos)
			if err != nil {
				return err
			}
			opened, err := repos.Meetings.OpenUnderLessonLock(ctx, room.Identity, room.Meeting.CreateTime, subject.Now)
			if err != nil {
				return err
			}
			if opened != domain.MeetingMutationApplied {
				return errRoomSuperseded
			}
			return publish(ctx, repos, subject)
		})
		if !errors.Is(err, errAudienceMoved) {
			return err
		}
	}
	return err
}

func (r *Rooms) reuse(ctx context.Context, room Room) (Room, error) {
	call, cancel := context.WithTimeout(ctx, r.deps.Timing.ProviderCall)
	defer cancel()
	info, err := r.deps.Provider.MeetingInfo(call, room.Meeting)
	if err != nil {
		if cause := ctx.Err(); cause != nil {
			return Room{}, cause
		}
		return Room{}, fmt.Errorf("%w: %w", ErrProviderUnavailable, err)
	}
	switch info.Status {
	case domain.MeetingCreated, domain.MeetingRunning:
		return room, nil
	case domain.MeetingNotFound:
		evidence, cancelEvidence := r.evidenceContext(ctx)
		defer cancelEvidence()
		if err := r.Retire(evidence, room.Identity, domain.MeetingCloseProviderNotFound); err != nil {
			return Room{}, err
		}
		return Room{}, errRoomSuperseded
	}
	return Room{}, ErrProviderUnavailable
}

func (r *Rooms) await(ctx context.Context, identity domain.MeetingIdentity) error {
	deadline := r.deps.Monotonic().Add(r.deps.Timing.CreatingWait)
	timer := time.NewTimer(r.deps.Timing.Poll)
	defer timer.Stop()
	for {
		row, err := r.deps.Store.Repositories().Meetings.ByID(ctx, identity.LessonID, identity.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errRoomSuperseded
		}
		if err != nil {
			return err
		}
		if row.State != domain.MeetingCreating {
			return errRoomSuperseded
		}
		if !r.deps.Monotonic().Before(deadline) {
			return ErrProviderUnavailable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(r.deps.Timing.Poll)
		}
	}
}

func (r *Rooms) Retire(ctx context.Context, identity domain.MeetingIdentity, reason domain.MeetingCloseReason) error {
	_, err := retireRoom(ctx, retirement{store: r.deps.Store, random: r.deps.Random, instant: func(ctx context.Context, repos repositories.Repositories) (time.Time, error) {
		return r.deps.Clock.InstantWithin(ctx, repos.TestClock)
	}}, identity, reason)
	return err
}

type retirement struct {
	store   *repositories.Store
	random  core.RandomSource
	instant func(context.Context, repositories.Repositories) (time.Time, error)
}

func retireRoom(ctx context.Context, source retirement, identity domain.MeetingIdentity, reason domain.MeetingCloseReason) (bool, error) {
	cycle, err := core.NewID[domain.OutboxID](source.random)
	if err != nil {
		return false, err
	}
	closed := false
	for range 2 {
		err = source.store.WithTx(ctx, func(repos repositories.Repositories) error {
			closed = false
			return retireLocked(ctx, repos, source, identity, reason, cycle, &closed)
		})
		if !errors.Is(err, errAudienceMoved) {
			return closed && err == nil, err
		}
	}
	return false, err
}

func retireLocked(ctx context.Context, repos repositories.Repositories, source retirement, identity domain.MeetingIdentity, reason domain.MeetingCloseReason, cycle domain.OutboxID, closedRoom *bool) error {
	lesson, err := repos.Lessons.JobAudience(ctx, identity.LessonID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	students, moved, err := lockRoomAudience(ctx, repos, lesson.TutorID, lesson.RelationshipID, lesson.GroupID)
	if err != nil {
		return err
	}
	if moved {
		return errAudienceMoved
	}
	if lesson.SeriesID != nil {
		if err := repos.Lessons.LockSeriesShared(ctx, *lesson.SeriesID); err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
	}
	if _, err := repos.Lessons.LockForMeeting(ctx, identity.LessonID); errors.Is(err, repositories.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	row, err := repos.Meetings.ByID(ctx, identity.LessonID, identity.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.State == domain.MeetingClosed || row.ProviderMeetingID != identity.ProviderMeetingID {
		return nil
	}
	now, err := source.instant(ctx, repos)
	if err != nil {
		return err
	}
	closed, err := repos.CloseMeetingForCleanup(ctx, domain.MeetingExpectation{Identity: identity, State: row.State, CreateTime: row.CreateTime}, reason, cycle, now)
	if err != nil || closed != domain.MeetingMutationApplied {
		return err
	}
	*closedRoom = true
	if row.State != domain.MeetingOpen {
		return nil
	}
	return repos.Signals.Entity(ctx, append([]domain.UserID{lesson.TutorID}, students...), domain.EntityLesson, domain.EntityID(identity.LessonID))
}

func lockRoomAudience(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship *domain.RelationshipID, group *domain.GroupID) ([]domain.UserID, bool, error) {
	if relationship != nil {
		locked, err := repos.Lessons.LockRelationship(ctx, tutor, *relationship)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if locked.StudentID == nil || (locked.State != domain.RelationshipActive && locked.State != domain.RelationshipProvisional) {
			return nil, false, nil
		}
		return []domain.UserID{*locked.StudentID}, false, nil
	}
	if group == nil {
		return nil, false, nil
	}
	read, err := repos.Groups.MemberIDs(ctx, tutor, *group)
	if err != nil {
		return nil, false, err
	}
	slices.SortFunc(read, compareRelationships)
	locked, err := repos.Lessons.LockRelationships(ctx, tutor, read)
	if err != nil {
		return nil, false, err
	}
	if _, err := repos.Lessons.LockGroup(ctx, tutor, *group); errors.Is(err, repositories.ErrNotFound) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	members, err := repos.Groups.MemberIDs(ctx, tutor, *group)
	if err != nil {
		return nil, false, err
	}
	slices.SortFunc(members, compareRelationships)
	students := make([]domain.UserID, 0, len(locked))
	lockedIDs := make([]domain.RelationshipID, 0, len(locked))
	for _, row := range locked {
		lockedIDs = append(lockedIDs, row.ID)
		if row.StudentID != nil {
			students = append(students, *row.StudentID)
		}
	}
	return students, !slices.Equal(members, lockedIDs), nil
}

func compareRelationships(first, second domain.RelationshipID) int {
	return slices.Compare(first[:], second[:])
}

func (r *Rooms) report(ctx context.Context, event string, attributes ...slog.Attr) {
	r.deps.Logger.LogAttrs(ctx, slog.LevelInfo, event, attributes...)
}

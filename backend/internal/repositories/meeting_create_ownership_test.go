package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

var errGuardRefused = errors.New("the guard refused the room")

type lockedLogs struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (l *lockedLogs) Write(data []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.buffer.Write(data)
}

func (l *lockedLogs) String() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.buffer.String()
}

type createGate struct {
	source  clock.Source
	armed   atomic.Bool
	reached chan struct{}
	release chan struct{}
}

func (g *createGate) Instant(ctx context.Context) (time.Time, error) {
	if g.armed.CompareAndSwap(true, false) {
		close(g.reached)
		select {
		case <-g.release:
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
	return g.source.Instant(ctx)
}

func (g *createGate) arm() {
	g.reached, g.release = make(chan struct{}), make(chan struct{})
	g.armed.Store(true)
}

type roomFixture struct {
	meetingFixture
	clock    *clock.Shared
	gate     *createGate
	provider *meeting.SharedFake
	observer *meeting.SharedFake
	rooms    *meetings.Rooms
	worker   *worker.Worker
	scanner  *meetings.RecoveryScanner
	logs     *lockedLogs
	logger   *slog.Logger
}

func newRoomFixture(t *testing.T) *roomFixture {
	t.Helper()
	f := &roomFixture{meetingFixture: newMeetingFixture(t, testdb.NewForLockContention(t)), logs: &lockedLogs{}}
	f.logger = slog.New(slog.NewJSONHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.clock = clock.NewShared(f.pool)
	_, err := f.clock.Set(t.Context(), instant)
	meetingRequire(t, err)
	f.gate = &createGate{source: f.clock}
	limits := meeting.FakeLimits{Meetings: 16, Links: 16, Participants: 16, Generations: 64}
	f.provider, err = meeting.NewSharedFake(t.Context(), f.pool, f.gate, core.CryptoRandom(), f.logger, limits)
	meetingRequire(t, err)
	f.observer, err = meeting.NewSharedFake(t.Context(), f.pool, f.clock, core.CryptoRandom(), f.logger, limits)
	meetingRequire(t, err)
	f.rooms = f.newRooms(meetings.DefaultRoomTiming(), core.CryptoRandom(), f.provider)
	cleanup := meetings.New(f.store, f.observer, domain.MeetingProviderFake, f.logger)
	f.worker = worker.New(f.logger, f.clock, outbox.New(f.store, core.CryptoRandom(), f.logger), []worker.Lane{{Name: domain.LaneProviderCall, Concurrency: 4, PerformMeetingRecovery: cleanup.Cleanup}}, nil, worker.DefaultTiming())
	f.scanner = meetings.NewRecoveryScanner(f.store, core.CryptoRandom(), f.logger, clock.System{}.Now)
	return f
}

func (f *roomFixture) newRooms(timing meetings.RoomTiming, random io.Reader, provider meeting.Provider) *meetings.Rooms {
	return meetings.NewRooms(meetings.RoomDependencies{
		Store: f.store, Provider: provider, Kind: domain.MeetingProviderFake, Clock: f.clock, Random: random, Logger: f.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})
}

func (f *roomFixture) guard(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
	return f.guardOf(f.lesson, f.tutor)(ctx, repos)
}

func (f *roomFixture) guardOf(lesson domain.LessonID, tutor domain.UserID) meetings.RoomGuard {
	return func(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		if _, err := repos.Lessons.LockForMeeting(ctx, lesson); err != nil {
			return meetings.RoomSubject{}, err
		}
		now, err := f.clock.InstantWithin(ctx, repos.TestClock)
		return meetings.RoomSubject{Lesson: lesson, Tutor: tutor, Now: now}, err
	}
}

func publishNothing(context.Context, repositories.Repositories, meetings.RoomSubject) error {
	return nil
}

func (f *roomFixture) ensure(ctx context.Context) (meetings.Room, error) {
	return f.rooms.Ensure(ctx, f.guard, publishNothing)
}

func (f *roomFixture) count(t *testing.T, statement string) int64 {
	t.Helper()
	var value int64
	meetingRequire(t, f.pool.QueryRow(t.Context(), statement).Scan(&value))
	return value
}

func (f *roomFixture) capacity(t *testing.T) int64 {
	t.Helper()
	value, err := f.repos.MeetingRecovery.Capacity(t.Context())
	meetingRequire(t, err)
	return value
}

func (f *roomFixture) inFlight(t *testing.T, ctx context.Context) (<-chan error, domain.MeetingIdentity) {
	t.Helper()
	f.gate.arm()
	result := make(chan error, 1)
	go func() {
		_, err := f.ensure(ctx)
		result <- err
	}()
	meetingAwait(t, t.Context(), f.gate.reached)
	current, err := f.repos.Meetings.Current(t.Context(), f.lesson)
	meetingRequire(t, err)
	return result, domain.MeetingIdentity{LessonID: current.LessonID, ID: current.ID, ProviderMeetingID: current.ProviderMeetingID}
}

func (f *roomFixture) assertSafeDiagnostics(t *testing.T, identities ...domain.MeetingIdentity) {
	t.Helper()
	logs := f.logs.String()
	forbidden := []string{domain.MeetingRoomName, "attempt_id", "routing_key"}
	for _, identity := range identities {
		forbidden = append(forbidden, identity.ProviderMeetingID.String())
	}
	for _, private := range forbidden {
		if strings.Contains(logs, private) {
			t.Fatalf("unsafe coordinator diagnostic %q", private)
		}
	}
}

func roomIdentity(room meetings.Room) domain.MeetingIdentity {
	return room.Identity
}

func TestMeetingCreateOwnership(t *testing.T) {
	t.Run("persisted dispatch and generation", testRoomCreation)
	t.Run("concurrent callers share one room", testRoomConcurrentCreation)
	t.Run("a young creating row is awaited up to its deadline", testRoomAwaitDeadline)
	t.Run("an older creating row closes and keeps ownership", testRoomStaleReplacement)
	t.Run("caller cancellation during Create creates no uncertainty", testRoomCancelDuringCreate)
	t.Run("a Create past its own budget stays uncertain", testRoomCreateDeadline)
	t.Run("provider commit before cancellation", testRoomCommitBeforeCancel)
	t.Run("provider failure never retries the same identity", testRoomProviderFailure)
	t.Run("lost authorization records but never publishes", testRoomLostAuthorization)
	t.Run("late acknowledgement after closure requests the final cycle", testRoomLateAcknowledgement)
	t.Run("capacity refuses before the provider without leaks", testRoomCapacity)
	t.Run("a settlement releases its slot once", testRoomCapacityRelease)
	t.Run("the tutor limit refuses before any reservation", testRoomTutorLimit)
	t.Run("two joins at the last slot admit one", testRoomTutorLimitRace)
	t.Run("a fresh uncertain outcome pauses only its lesson", testRoomUncertainPause)
	t.Run("an outcome that is not uncertain starts no pause", testRoomNoPauseWithoutUncertainty)
	t.Run("a clock stepped back behind an outcome does not lengthen the pause", testRoomPauseAfterClockStepBack)
	t.Run("a cancel during the reserving transaction burns no slot", testRoomCancelDuringReservation)
	t.Run("a failed closure behind a refused publication is reported", testRoomFailedRetireReported)
	t.Run("the room name is the neutral constant", testRoomNeutralName)
	t.Run("Create carries the meeting's own routing key and lesson", testRoomCallbackIdentity)
	t.Run("a budget too short for Create reserves nothing", testRoomBudgetTooShort)
	t.Run("a clock stepped back cannot strand a close", testRoomCloseAfterClockStepBack)
	t.Run("the scanner counts only rows it closed", testRoomScannerCountsItsCloses)
	t.Run("entropy failure changes nothing", testRoomEntropyFailure)
	t.Run("an absent open room is replaced", testRoomAbsentReplacement)
	t.Run("the scanner closes abandoned rows", testRoomScannerClosesAbandoned)
}

func testRoomCreation(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	result, identity := f.inFlight(t, t.Context())

	dispatched := f.recovery(t, identity.ProviderMeetingID)
	rowsBeforeCreate := f.count(t, "SELECT count(id) FROM fake_meeting_rooms")
	close(f.gate.release)
	meetingRequire(t, meetingAwait(t, t.Context(), result))
	second, err := f.ensure(t.Context())
	meetingRequire(t, err)

	if dispatched.CreateState != domain.MeetingCreateStarted || dispatched.AttemptID == nil || rowsBeforeCreate != 0 {
		t.Fatalf("dispatch was not persisted first: state=%s rooms=%d", dispatched.CreateState, rowsBeforeCreate)
	}
	opened, recovered := f.read(t, identity), f.recovery(t, identity.ProviderMeetingID)
	if opened.State != domain.MeetingOpen || opened.CreateTime == nil || recovered.CreateState != domain.MeetingCreateReturned || *recovered.CreateTime != *opened.CreateTime {
		t.Fatalf("generation evidence: meeting=%+v ledger=%+v", opened, recovered)
	}
	if second.Identity != identity || second.Meeting.CreateTime != *opened.CreateTime || f.capacity(t) != 1 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 1 {
		t.Fatalf("reuse created another room: %+v capacity=%d", second, f.capacity(t))
	}
	f.assertSafeDiagnostics(t, identity)
}

func testRoomConcurrentCreation(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	const callers = 8
	results := make(chan meetings.Room, callers)
	failures := make(chan error, callers)

	for range callers {
		go func() {
			room, err := f.ensure(t.Context())
			results <- room
			failures <- err
		}()
	}
	var rooms []meetings.Room
	for range callers {
		rooms = append(rooms, meetingAwait(t, t.Context(), results))
		meetingRequire(t, meetingAwait(t, t.Context(), failures))
	}

	for _, room := range rooms {
		if room.Identity != rooms[0].Identity || room.Meeting.CreateTime != rooms[0].Meeting.CreateTime || room.Meeting.CreateTime <= 0 {
			t.Fatalf("callers saw different rooms: %+v and %+v", room, rooms[0])
		}
	}
	if f.capacity(t) != 1 || f.count(t, "SELECT count(id) FROM meetings") != 1 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 1 {
		t.Fatal("concurrent callers created more than one room")
	}
}

func testRoomAwaitDeadline(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	result, identity := f.inFlight(t, t.Context())
	timing := meetings.DefaultRoomTiming()
	timing.CreatingWait, timing.Poll = 50*time.Millisecond, time.Millisecond
	impatient := f.newRooms(timing, core.CryptoRandom(), f.provider)

	_, err := impatient.Ensure(t.Context(), f.guard, publishNothing)
	stillCreating := f.read(t, identity).State
	close(f.gate.release)
	meetingRequire(t, meetingAwait(t, t.Context(), result))
	awaited, reuseErr := impatient.Ensure(t.Context(), f.guard, publishNothing)

	meetingRequire(t, reuseErr)
	if !errors.Is(err, meetings.ErrProviderUnavailable) || stillCreating != domain.MeetingCreating || awaited.Identity != identity {
		t.Fatalf("await deadline: err=%v state=%s room=%+v", err, stillCreating, awaited)
	}
}

func (f *roomFixture) abandoned(t *testing.T) domain.MeetingIdentity {
	t.Helper()
	identity := f.create(t, 40)
	if f.begin(t, identity, domain.MeetingCreateAttemptID{40}, 1) != domain.MeetingMutationApplied {
		t.Fatal("fixture begin refused")
	}
	return identity
}

func testRoomStaleReplacement(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	crashed := f.abandoned(t)
	_, err := f.clock.Set(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter))
	meetingRequire(t, err)

	replacement, err := f.ensure(t.Context())
	meetingRequire(t, err)
	meetingRequire(t, f.worker.Tick(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter)))

	closed, retained := f.read(t, crashed), f.recovery(t, crashed.ProviderMeetingID)
	if closed.State != domain.MeetingClosed || *closed.CloseReason != domain.MeetingCloseProviderError || !retained.CleanupRequested {
		t.Fatalf("stale row: %+v ledger=%+v", closed, retained)
	}
	if retained.CreateState != domain.MeetingCreateStarted || retained.SettledAt != nil || retained.CycleResult != domain.MeetingRecoveryCycleUnresolved {
		t.Fatalf("absence settled uncertain evidence: %+v", retained)
	}
	if replacement.Identity.ProviderMeetingID == crashed.ProviderMeetingID || f.capacity(t) != 2 {
		t.Fatalf("replacement=%+v capacity=%d", replacement, f.capacity(t))
	}
}

func testRoomCancelDuringCreate(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, identity := f.inFlight(t, ctx)

	cancel()
	close(f.gate.release)
	err := meetingAwait(t, t.Context(), result)
	retained := f.recovery(t, identity.ProviderMeetingID)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	settled, closed := f.recovery(t, identity.ProviderMeetingID), f.read(t, identity)
	if !errors.Is(err, context.Canceled) || closed.State != domain.MeetingClosed || retained.CreateState != domain.MeetingCreateReturned {
		t.Fatalf("cancel during create: err=%v meeting=%s ledger=%s", err, closed.State, retained.CreateState)
	}
	if settled.SettledAt == nil || f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 {
		t.Fatalf("a cancelled caller left an obligation: %+v capacity=%d", settled, f.capacity(t))
	}
	waitPoolReleased(t, f.meetingFixture)
}

func shortCreateBudget() meetings.RoomTiming {
	timing := meetings.DefaultRoomTiming()
	timing.ProviderCall = 50 * time.Millisecond
	return timing
}

func (f *roomFixture) expiredCreate(t *testing.T) domain.MeetingIdentity {
	t.Helper()
	f.gate.arm()
	short := f.newRooms(shortCreateBudget(), core.CryptoRandom(), f.provider)
	_, err := short.Ensure(t.Context(), f.guard, publishNothing)
	if !errors.Is(err, meetings.ErrProviderUnavailable) {
		t.Fatalf("a Create past its budget answered %v", err)
	}
	var failed [16]byte
	meetingRequire(t, f.pool.QueryRow(t.Context(), "SELECT provider_meeting_id FROM meeting_recovery").Scan(&failed))
	row, err := f.repos.MeetingRecovery.ByProviderID(t.Context(), domain.ProviderMeetingID(failed))
	meetingRequire(t, err)
	return domain.MeetingIdentity{LessonID: f.lesson, ID: *row.MeetingID, ProviderMeetingID: row.ProviderMeetingID}
}

func testRoomCreateDeadline(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)

	identity := f.expiredCreate(t)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	retained, closed := f.recovery(t, identity.ProviderMeetingID), f.read(t, identity)
	if closed.State != domain.MeetingClosed || retained.CreateState != domain.MeetingCreateUncertain || retained.SettledAt != nil || f.capacity(t) != 1 {
		t.Fatalf("an expired create: meeting=%s ledger=%+v", closed.State, retained)
	}
	waitPoolReleased(t, f.meetingFixture)
}

func waitPoolReleased(t *testing.T, f meetingFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	waitScannerConnectionsReleased(t, ctx, f)
}

type cancellingLogs struct {
	logs   *lockedLogs
	cancel func()
}

func (c *cancellingLogs) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(`"meeting.provider.completed"`)) && bytes.Contains(data, []byte(`"create"`)) {
		c.cancel()
	}
	return c.logs.Write(data)
}

func testRoomCommitBeforeCancel(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(&cancellingLogs{logs: f.logs, cancel: cancel}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider, err := meeting.NewSharedFake(t.Context(), f.pool, f.clock, core.CryptoRandom(), logger, meeting.FakeLimits{Meetings: 16, Links: 16, Participants: 16, Generations: 64})
	meetingRequire(t, err)
	rooms := f.newRooms(meetings.DefaultRoomTiming(), core.CryptoRandom(), provider)

	_, ensureErr := rooms.Ensure(ctx, f.guard, publishNothing)

	current, currentErr := f.repos.Meetings.Current(t.Context(), f.lesson)
	if !errors.Is(currentErr, repositories.ErrNotFound) || !errors.Is(ensureErr, context.Canceled) {
		t.Fatalf("a cancelled request published a room: %+v %v %v", current, currentErr, ensureErr)
	}
	var provided [16]byte
	meetingRequire(t, f.pool.QueryRow(t.Context(), "SELECT id FROM fake_meeting_rooms").Scan(&provided))
	retained := f.recovery(t, domain.ProviderMeetingID(provided))
	if retained.CreateState != domain.MeetingCreateReturned || retained.CreateTime == nil || !retained.CleanupRequested {
		t.Fatalf("the returned generation was not retained: %+v", retained)
	}
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	settled := f.recovery(t, domain.ProviderMeetingID(provided))
	if settled.SettledAt == nil || f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 {
		t.Fatalf("cleanup did not end the acknowledged generation: %+v capacity=%d", settled, f.capacity(t))
	}
}

func testRoomProviderFailure(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), true))

	_, ensureErr := f.ensure(t.Context())
	var failed [16]byte
	meetingRequire(t, f.pool.QueryRow(t.Context(), "SELECT provider_meeting_id FROM meeting_recovery").Scan(&failed))
	retained := f.recovery(t, domain.ProviderMeetingID(failed))
	capacityWhileDown := f.capacity(t)
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), false))
	_, err := f.clock.Set(t.Context(), instant.Add(domain.MeetingUncertainPause))
	meetingRequire(t, err)
	replacement, replacementErr := f.ensure(t.Context())

	if !errors.Is(ensureErr, meetings.ErrProviderUnavailable) {
		t.Fatalf("provider failure answered %v", ensureErr)
	}
	if retained.CreateState != domain.MeetingCreateUncertain || !retained.CleanupRequested || capacityWhileDown != 1 {
		t.Fatalf("failed create evidence: %+v capacity=%d", retained, capacityWhileDown)
	}
	meetingRequire(t, replacementErr)
	if replacement.Identity.ProviderMeetingID == domain.ProviderMeetingID(failed) || f.capacity(t) != 2 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 1 {
		t.Fatalf("an uncertain identity was retried: %+v", replacement)
	}
}

func testRoomLostAuthorization(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	var calls atomic.Int32
	guard := func(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		if calls.Add(1) == 2 {
			return meetings.RoomSubject{}, errGuardRefused
		}
		return f.guard(ctx, repos)
	}

	_, err := f.rooms.Ensure(t.Context(), guard, publishNothing)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	if !errors.Is(err, errGuardRefused) {
		t.Fatalf("refusal was replaced by %v", err)
	}
	if _, currentErr := f.repos.Meetings.Current(t.Context(), f.lesson); !errors.Is(currentErr, repositories.ErrNotFound) {
		t.Fatal("a room stayed current after lost authorization")
	}
	if f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 || f.count(t, "SELECT count(provider_meeting_id) FROM meeting_recovery WHERE create_state = 'returned' AND settled_at IS NOT NULL") != 1 {
		t.Fatal("the refused generation was not recorded and ended")
	}
}

func testRoomLateAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	result, identity := f.inFlight(t, t.Context())
	meetingRequire(t, f.rooms.Retire(t.Context(), identity, domain.MeetingCloseTutorEnd))
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	beforeLate := f.recovery(t, identity.ProviderMeetingID)

	close(f.gate.release)
	meetingRequire(t, meetingAwait(t, t.Context(), result))
	late := f.recovery(t, identity.ProviderMeetingID)
	queued, err := f.scanner.Run(t.Context(), instant)
	meetingRequire(t, err)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	final := f.recovery(t, identity.ProviderMeetingID)
	info, err := f.observer.InspectMeeting(t.Context(), identity.ProviderMeetingID)
	meetingRequire(t, err)
	again, err := f.scanner.Run(t.Context(), instant)
	meetingRequire(t, err)

	if beforeLate.CycleResult != domain.MeetingRecoveryCycleUnresolved || beforeLate.SettledAt != nil {
		t.Fatalf("absence before the late create settled it: %+v", beforeLate)
	}
	if late.CreateState != domain.MeetingCreateReturned || !late.FinalCycleRequested || !late.CleanupRequested || queued != 1 {
		t.Fatalf("late acknowledgement: %+v queued=%d", late, queued)
	}
	if final.SettledAt == nil || !final.FinalCycleUsed || info.Status != domain.MeetingNotFound {
		t.Fatalf("the late room was not ended by its final cycle: %+v %+v", final, info)
	}
	if again != 0 || f.read(t, identity).State != domain.MeetingClosed {
		t.Fatal("a closed room reopened or another cycle was created")
	}
}

func testRoomCapacity(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery_capacity SET unsettled = 4096")
	meetingRequire(t, err)

	_, refused := f.ensure(t.Context())

	if !errors.Is(refused, meetings.ErrAdmissionCapacity) || f.capacity(t) != 4096 || f.count(t, "SELECT count(id) FROM meetings") != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 {
		t.Fatalf("capacity refusal: %v capacity=%d", refused, f.capacity(t))
	}
}

func testRoomCapacityRelease(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery_capacity SET unsettled = 4095")
	meetingRequire(t, err)
	room, err := f.ensure(t.Context())
	meetingRequire(t, err)
	reserved := f.capacity(t)

	meetingRequire(t, f.rooms.Retire(t.Context(), room.Identity, domain.MeetingCloseTutorEnd))
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	if reserved != 4096 || f.capacity(t) != 4095 {
		t.Fatalf("admission reserved to %d and settlement released to %d", reserved, f.capacity(t))
	}
}

func (f *roomFixture) seedClosedRows(t *testing.T, count int) {
	t.Helper()
	for index := range count {
		recoveryClose(t, f.meetingFixture, f.create(t, byte(100+index)))
	}
}

func testRoomTutorLimit(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	f.seedClosedRows(t, domain.MeetingUnsettledPerTutor-1)
	other, student := f.tutorAccount(t, 60), f.studentAccount(t, 61)
	card := f.card(t, linkedCard(62, other, student, domain.RelationshipActive))
	otherLesson := domain.LessonID{63}
	meetingRequire(t, f.repos.Lessons.Create(t.Context(), repositories.NewLessons{
		TutorID: other, RelationshipID: &card, Title: "Another lesson", DurationMinutes: 60, Timezone: "UTC", Now: instant,
		Items: []repositories.NewLessonItem{{ID: otherLesson, StartsAt: instant.Add(time.Hour)}},
	}))

	_, lastAllowed := f.ensure(t.Context())
	reserved := f.capacity(t)
	_, refused := f.rooms.Ensure(t.Context(), f.guardOf(f.otherLesson, f.tutor), publishNothing)
	afterRefusal := f.capacity(t)
	_, unaffected := f.rooms.Ensure(t.Context(), f.guardOf(otherLesson, other), publishNothing)

	meetingRequire(t, lastAllowed, unaffected)
	if !errors.Is(refused, meetings.ErrAdmissionCapacity) {
		t.Fatalf("the sixty-fifth obligation answered %v", refused)
	}
	if reserved != domain.MeetingUnsettledPerTutor || afterRefusal != reserved || f.capacity(t) != reserved+1 {
		t.Fatalf("reservations: %d, then %d after the refusal, then %d", reserved, afterRefusal, f.capacity(t))
	}
	if _, err := f.repos.Meetings.Current(t.Context(), f.otherLesson); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("the refused lesson has a row: %v", err)
	}
}

func (f *roomFixture) lockedGuardOf(lesson domain.LessonID) meetings.RoomGuard {
	return func(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		if err := repos.TutorProfiles.Lock(ctx, f.tutor); err != nil {
			return meetings.RoomSubject{}, err
		}
		return f.guardOf(lesson, f.tutor)(ctx, repos)
	}
}

func (f *roomFixture) holdCapacity(t *testing.T) func() {
	t.Helper()
	holder, err := f.pool.Begin(t.Context())
	meetingRequire(t, err)
	var unsettled int64
	meetingRequire(t, holder.QueryRow(t.Context(), "SELECT unsettled FROM meeting_recovery_capacity WHERE singleton FOR UPDATE").Scan(&unsettled))
	release := sync.OnceFunc(func() { meetingRequire(t, holder.Rollback(context.WithoutCancel(t.Context()))) })
	t.Cleanup(release)
	return release
}

func (f *roomFixture) awaitLockWaiters(t *testing.T, expected int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for {
		var waiting int64
		meetingRequire(t, f.pool.QueryRow(ctx, "SELECT count(pid) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting))
		if waiting >= expected {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("%d transactions wait on a lock instead of %d", waiting, expected)
		}
	}
}

func testRoomTutorLimitRace(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	f.seedClosedRows(t, domain.MeetingUnsettledPerTutor-1)
	release := f.holdCapacity(t)
	answers := make(chan error, 2)
	for _, lesson := range []domain.LessonID{f.lesson, f.otherLesson} {
		go func() {
			_, err := f.rooms.Ensure(t.Context(), f.lockedGuardOf(lesson), publishNothing)
			answers <- err
		}()
	}
	f.awaitLockWaiters(t, 2)

	release()
	first, second := meetingAwait(t, t.Context(), answers), meetingAwait(t, t.Context(), answers)

	refused, admitted := first, second
	if first == nil {
		refused, admitted = second, first
	}
	if admitted != nil || !errors.Is(refused, meetings.ErrAdmissionCapacity) {
		t.Fatalf("two joins at the last slot answered %v and %v", first, second)
	}
	if f.capacity(t) != domain.MeetingUnsettledPerTutor || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 1 {
		t.Fatalf("the race left %d unsettled, %d rooms", f.capacity(t), f.count(t, "SELECT count(id) FROM fake_meeting_rooms"))
	}
}

func testRoomUncertainPause(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), true))
	_, failed := f.ensure(t.Context())
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), false))
	_, err := f.clock.Set(t.Context(), instant.Add(domain.MeetingUncertainPause-time.Second))
	meetingRequire(t, err)

	_, paused := f.ensure(t.Context())
	capacityWhilePaused, rowsWhilePaused := f.capacity(t), f.count(t, "SELECT count(id) FROM meetings")
	_, otherLesson := f.rooms.Ensure(t.Context(), f.guardOf(f.otherLesson, f.tutor), publishNothing)
	_, err = f.clock.Set(t.Context(), instant.Add(domain.MeetingUncertainPause))
	meetingRequire(t, err)
	_, afterPause := f.ensure(t.Context())

	if !errors.Is(failed, meetings.ErrProviderUnavailable) || !errors.Is(paused, meetings.ErrProviderUnavailable) {
		t.Fatalf("the failure answered %v and the paused call %v", failed, paused)
	}
	if capacityWhilePaused != 1 || rowsWhilePaused != 1 {
		t.Fatalf("the paused call wrote: capacity=%d rows=%d", capacityWhilePaused, rowsWhilePaused)
	}
	meetingRequire(t, otherLesson, afterPause)
	if f.capacity(t) != 3 {
		t.Fatalf("after the pause capacity is %d", f.capacity(t))
	}
}

func testRoomPauseAfterClockStepBack(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), true))
	_, failed := f.ensure(t.Context())
	meetingRequire(t, f.observer.SimulateUnavailable(t.Context(), false))
	_, err := f.clock.Set(t.Context(), instant.Add(-time.Hour))
	meetingRequire(t, err)

	_, afterStep := f.ensure(t.Context())

	if !errors.Is(failed, meetings.ErrProviderUnavailable) {
		t.Fatalf("the failure answered %v", failed)
	}
	meetingRequire(t, afterStep)
}

func testRoomNoPauseWithoutUncertainty(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	var calls atomic.Int32
	refusing := func(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		if calls.Add(1) == 2 {
			return meetings.RoomSubject{}, errGuardRefused
		}
		return f.guard(ctx, repos)
	}
	_, refused := f.rooms.Ensure(t.Context(), refusing, publishNothing)

	_, next := f.ensure(t.Context())

	if !errors.Is(refused, errGuardRefused) {
		t.Fatalf("the refusal answered %v", refused)
	}
	meetingRequire(t, next)
}

func testRoomCancelDuringReservation(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls atomic.Int32
	cancelling := func(guardCtx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		subject, err := f.guard(guardCtx, repos)
		if calls.Add(1) == 1 {
			cancel()
		}
		return subject, err
	}

	_, err := f.rooms.Ensure(ctx, cancelling, publishNothing)
	reservedRows := f.count(t, "SELECT count(id) FROM meetings")
	dispatched := f.count(t, "SELECT generations FROM fake_meeting_state WHERE id")
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancel during reservation answered %v", err)
	}
	if reservedRows != 1 || dispatched != 1 {
		t.Fatalf("the reservation did not survive the cancel: %d rows, %d provider Creates", reservedRows, dispatched)
	}
	if f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 || f.count(t, "SELECT count(provider_meeting_id) FROM meeting_recovery WHERE create_state IN ('started', 'uncertain')") != 0 {
		t.Fatalf("a cancel during reservation burned a slot: capacity=%d", f.capacity(t))
	}
}

func testRoomFailedRetireReported(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	exhausted := io.MultiReader(io.LimitReader(core.CryptoRandom(), 5*16), brokenEntropy{})
	rooms := f.newRooms(meetings.DefaultRoomTiming(), exhausted, f.provider)
	var calls atomic.Int32
	refusing := func(ctx context.Context, repos repositories.Repositories) (meetings.RoomSubject, error) {
		if calls.Add(1) == 2 {
			return meetings.RoomSubject{}, errGuardRefused
		}
		return f.guard(ctx, repos)
	}

	_, err := rooms.Ensure(t.Context(), refusing, publishNothing)

	if !errors.Is(err, errGuardRefused) || !strings.Contains(f.logs.String(), `"msg":"meeting.room.evidence_failed"`) {
		t.Fatalf("the refusal answered %v without the report:\n%s", err, f.logs.String())
	}
}

func testRoomNeutralName(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)

	_, err := f.ensure(t.Context())

	meetingRequire(t, err)
	var name string
	meetingRequire(t, f.pool.QueryRow(t.Context(), "SELECT name FROM fake_meeting_rooms").Scan(&name))
	if name != domain.MeetingRoomName {
		t.Fatalf("the provider received the room name %q", name)
	}
}

func testRoomCallbackIdentity(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	provider, err := meeting.NewFake(f.clock, core.CryptoRandom(), f.logger, meeting.DefaultFakeLimits())
	meetingRequire(t, err)
	rooms := f.newRooms(meetings.DefaultRoomTiming(), core.CryptoRandom(), provider)

	room, err := rooms.Ensure(t.Context(), f.guard, publishNothing)

	meetingRequire(t, err)
	var routingKey string
	meetingRequire(t, f.pool.QueryRow(t.Context(), "SELECT routing_key::text FROM meetings").Scan(&routingKey))
	requested, found := provider.Requested(room.Identity.ProviderMeetingID)
	if !found || requested.RoutingKey.String() != routingKey || requested.LessonID != f.lesson {
		t.Fatal("Create did not receive the persisted routing key and the lesson")
	}
}

func testRoomBudgetTooShort(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	ctx, cancel := context.WithDeadline(t.Context(), clock.System{}.Now().Add(meetings.DefaultRoomTiming().ProviderCall-time.Second))
	defer cancel()

	_, err := f.rooms.Ensure(ctx, f.guard, publishNothing)

	if !errors.Is(err, meetings.ErrProviderUnavailable) || f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM meetings") != 0 {
		t.Fatalf("a short budget: %v capacity=%d", err, f.capacity(t))
	}
}

func testRoomCloseAfterClockStepBack(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	room, err := f.ensure(t.Context())
	meetingRequire(t, err)
	_, err = f.clock.Set(t.Context(), instant.Add(-time.Hour))
	meetingRequire(t, err)

	retired := f.rooms.Retire(t.Context(), room.Identity, domain.MeetingCloseTutorEnd)

	meetingRequire(t, retired)
	closed, retained := f.read(t, room.Identity), f.recovery(t, room.Identity.ProviderMeetingID)
	if closed.State != domain.MeetingClosed || !retained.CleanupRequested || retained.CycleNumber != 1 {
		t.Fatalf("a stepped-back close: meeting=%s ledger=%+v", closed.State, retained)
	}
}

func testRoomScannerCountsItsCloses(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	abandoned := f.abandoned(t)
	calls := 0
	racing := func() time.Time {
		calls++
		if calls == 2 {
			meetingRequire(t, f.rooms.Retire(t.Context(), abandoned, domain.MeetingCloseTutorEnd))
		}
		return clock.System{}.Now()
	}
	logs := &lockedLogs{}
	scanner := meetings.NewRecoveryScanner(f.store, core.CryptoRandom(), slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), racing)

	_, err := scanner.Run(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter))

	meetingRequire(t, err)
	if !strings.Contains(logs.String(), `"closed":0`) {
		t.Fatalf("the scanner counted a row another path closed: %s", logs.String())
	}
}

type brokenEntropy struct{}

func (brokenEntropy) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func testRoomEntropyFailure(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	broken := f.newRooms(meetings.DefaultRoomTiming(), brokenEntropy{}, f.provider)

	_, err := broken.Ensure(t.Context(), f.guard, publishNothing)

	if !errors.Is(err, io.ErrUnexpectedEOF) || f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM meetings") != 0 {
		t.Fatalf("entropy failure: %v capacity=%d", err, f.capacity(t))
	}
}

func testRoomAbsentReplacement(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	first, err := f.ensure(t.Context())
	meetingRequire(t, err)
	meetingRequire(t, f.observer.EndMeeting(t.Context(), first.Meeting))

	second, err := f.ensure(t.Context())
	meetingRequire(t, err)

	closed := f.read(t, first.Identity)
	if closed.State != domain.MeetingClosed || *closed.CloseReason != domain.MeetingCloseProviderNotFound || second.Identity.ProviderMeetingID == first.Identity.ProviderMeetingID {
		t.Fatalf("absent room: %+v replacement=%+v", closed, roomIdentity(second))
	}
}

func testRoomScannerClosesAbandoned(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	young := f.abandoned(t)
	queuedYoung, err := f.scanner.Run(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter-time.Second))
	meetingRequire(t, err)
	stateWhileYoung := f.read(t, young).State

	_, err = f.scanner.Run(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter))
	meetingRequire(t, err)
	meetingRequire(t, f.worker.Tick(t.Context(), instant.Add(domain.MeetingCreatingStaleAfter)))
	renewed, err := f.scanner.Run(t.Context(), instant.Add(2*domain.MeetingCreatingStaleAfter))
	meetingRequire(t, err)

	if stateWhileYoung != domain.MeetingCreating || queuedYoung != 0 {
		t.Fatalf("a young row was closed: %s queued=%d", stateWhileYoung, queuedYoung)
	}
	retained, closed := f.recovery(t, young.ProviderMeetingID), f.read(t, young)
	if closed.State != domain.MeetingClosed || retained.CycleResult != domain.MeetingRecoveryCycleUnresolved || retained.SettledAt != nil || f.capacity(t) != 1 || renewed != 0 {
		t.Fatalf("abandoned row: meeting=%s ledger=%+v renewed=%d", closed.State, retained, renewed)
	}
}

func TestMeetingRecoveryPurgeRace(t *testing.T) {
	t.Run("a returned generation survives a purge during Create", testRoomPurgeDuringCreate)
	t.Run("an uncertain orphan is retained beyond receipt cleanup", testRoomPurgeUncertain)
}

func (f *roomFixture) purgeLesson(t *testing.T) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), "DELETE FROM lessons WHERE id = $1", [16]byte(f.lesson))
	meetingRequire(t, err)
}

func testRoomPurgeDuringCreate(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	result, identity := f.inFlight(t, t.Context())
	f.purgeLesson(t)

	close(f.gate.release)
	err := meetingAwait(t, t.Context(), result)
	orphan := f.recovery(t, identity.ProviderMeetingID)
	queued, scanErr := f.scanner.Run(t.Context(), instant)
	meetingRequire(t, scanErr)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))

	if !errors.Is(err, repositories.ErrNotFound) || orphan.MeetingID != nil || orphan.CreateState != domain.MeetingCreateReturned || queued != 1 {
		t.Fatalf("purge race: err=%v ledger=%+v queued=%d", err, orphan, queued)
	}
	settled := f.recovery(t, identity.ProviderMeetingID)
	if settled.SettledAt == nil || f.capacity(t) != 0 || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 {
		t.Fatalf("the orphaned generation was not ended: %+v", settled)
	}
	f.assertSafeDiagnostics(t, identity)
}

func testRoomPurgeUncertain(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	identity := f.expiredCreate(t)

	f.purgeLesson(t)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	removed, err := f.repos.MeetingRecovery.CleanupReceipts(t.Context(), instant.Add(8*24*time.Hour), 100)
	meetingRequire(t, err)

	retained := f.recovery(t, identity.ProviderMeetingID)
	if removed != 0 || retained.MeetingID != nil || retained.CreateState != domain.MeetingCreateUncertain || retained.SettledAt != nil || f.capacity(t) != 1 {
		t.Fatalf("uncertain orphan: removed=%d ledger=%+v", removed, retained)
	}
}

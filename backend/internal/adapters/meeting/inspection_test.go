package meeting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

type inspectionProvider interface {
	Provider
	Enter(context.Context, JoinLink, domain.Meeting, domain.Participant) error
	Leave(context.Context, domain.Meeting, domain.ParticipantID) error
	SimulateUnavailable(context.Context, bool) error
}

func fakeInspectionState(provider *Fake) string {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()
	rooms := make([]string, 0, len(provider.meetings))
	for id, room := range provider.meetings {
		rooms = append(rooms, fmt.Sprintf("%v %v %v %v", id, room.meeting, room.participants, room.links))
	}
	slices.Sort(rooms)
	return fmt.Sprintf("%v %v %v %v %v %v %v", rooms, provider.limits, provider.generations, provider.lastCreateTime, provider.linkSequence, provider.links, provider.unavailable)
}

func sharedInspectionState(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var state string
	err := pool.QueryRow(t.Context(), `SELECT json_build_array(
		(SELECT json_agg(s ORDER BY id) FROM fake_meeting_state s),
		(SELECT json_agg(r ORDER BY id) FROM fake_meeting_rooms r),
		(SELECT json_agg(p ORDER BY room_id, participant_id) FROM fake_meeting_participants p),
		(SELECT json_agg(l ORDER BY digest) FROM fake_meeting_links l))::text`).Scan(&state)
	requireNoErrors(t, err)
	return state
}

func TestMeetingProviderInspectionReadOnly(t *testing.T) {
	for _, shared := range []bool{false, true} {
		for _, leave := range []bool{false, true} {
			t.Run(fmt.Sprintf("shared=%v/leave=%v", shared, leave), func(t *testing.T) {
				var provider inspectionProvider
				var snapshot func() string
				if shared {
					database := testdb.New(t)
					provider = newShared(t, database.Pool, testLimits(), 1)
					snapshot = func() string { return sharedInspectionState(t, database.Pool) }
				} else {
					fake := newTestFake(t, testLimits())
					provider = fake
					snapshot = func() string { return fakeInspectionState(fake) }
				}
				room := createdMeeting(t, provider)
				participant := testParticipant()
				link := issuedLink(t, provider, room, participant)
				requireNoErrors(t, provider.Enter(t.Context(), link, room, participant))
				want := domain.MeetingInfo{Status: domain.MeetingRunning, CreateTime: room.CreateTime, Participants: 1}
				if leave {
					requireNoErrors(t, provider.Leave(t.Context(), room, participant.ID))
					want.Status, want.Participants = domain.MeetingCreated, 0
				}
				before := snapshot()

				info, err := provider.InspectMeeting(t.Context(), room.ID)
				missing, missingErr := provider.InspectMeeting(t.Context(), domain.ProviderMeetingID{99})
				after := snapshot()

				requireNoErrors(t, err, missingErr)
				if info != want || missing != (domain.MeetingInfo{Status: domain.MeetingNotFound}) || before != after {
					t.Fatal("inspection changed provider state or returned an inconsistent snapshot")
				}
			})
		}
	}
}

func TestMeetingProviderInspectionGenerationSafety(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared=%v", shared), func(t *testing.T) {
			var provider inspectionProvider = newTestFake(t, testLimits())
			if shared {
				provider = newShared(t, testdb.New(t).Pool, testLimits(), 1)
			}
			room := createdMeeting(t, provider)
			participant := testParticipant()
			link := issuedLink(t, provider, room, participant)
			discovered, discoverErr := provider.InspectMeeting(t.Context(), room.ID)
			requireNoErrors(t, discoverErr)
			stale := domain.Meeting{ID: room.ID, CreateTime: discovered.CreateTime}
			requireNoErrors(t, provider.EndMeeting(t.Context(), stale))
			replacement := createdMeeting(t, provider)
			currentLink := issuedLink(t, provider, replacement, participant)
			requireNoErrors(t, provider.Enter(t.Context(), currentLink, replacement, participant))

			oldInfo, oldInfoErr := provider.MeetingInfo(t.Context(), stale)
			endErr := provider.EndMeeting(t.Context(), stale)
			_, joinErr := provider.JoinLink(t.Context(), stale, participant)
			enterErr := provider.Enter(t.Context(), link, stale, participant)
			leaveErr := provider.Leave(t.Context(), stale, participant.ID)
			current, currentErr := provider.InspectMeeting(t.Context(), room.ID)

			requireNoErrors(t, oldInfoErr, currentErr)
			for _, err := range []error{endErr, joinErr, enterErr, leaveErr} {
				requireFailure(t, err, FailureNotFound)
			}
			if discovered.CreateTime != room.CreateTime || replacement.CreateTime <= room.CreateTime || oldInfo != (domain.MeetingInfo{Status: domain.MeetingNotFound}) || current != (domain.MeetingInfo{Status: domain.MeetingRunning, CreateTime: replacement.CreateTime, Participants: 1}) {
				t.Fatal("discovered old generation affected the replacement")
			}
		})
	}
	t.Run("in-memory creation barrier", testFakeInspectionCreationBarrier)
	t.Run("shared replacement barrier", testSharedInspectionReplacementBarrier)
}

func testFakeInspectionCreationBarrier(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	provider := configuredFake(t, heldClock{entered: entered, release: release}, sharedRandom(1), testLogger(), testLimits())
	var created domain.ProviderCreateTime
	creator := core.Run(ctx, testLogger(), "inspection-create", func() error {
		var err error
		created, err = provider.CreateMeeting(ctx, meetingInput())
		return err
	})
	await(t, ctx, entered)
	started := make(chan struct{})
	var info domain.MeetingInfo

	inspector := core.Run(ctx, testLogger(), "inspection-waiter", func() error {
		close(started)
		var err error
		info, err = provider.InspectMeeting(ctx, meetingInput().ID)
		return err
	})
	await(t, ctx, started)
	close(release)
	createErr, inspectErr := await(t, ctx, creator), await(t, ctx, inspector)

	requireNoErrors(t, createErr, inspectErr)
	if created <= 0 || info != (domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: created}) {
		t.Fatal("inspection did not observe the serialized create")
	}
}

func holdInspectionState(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	holder, err := pool.Begin(ctx)
	requireNoErrors(t, err)
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error("release inspection state lock failed")
		}
	})
	_, err = holder.Exec(ctx, `SELECT id FROM fake_meeting_state WHERE id FOR UPDATE`)
	requireNoErrors(t, err)
	return holder
}

func testSharedInspectionReplacementBarrier(t *testing.T) {
	database := testdb.NewForLockContention(t)
	provider := newShared(t, database.Pool, testLimits(), 1)
	old := createdMeeting(t, provider)
	discovered, err := provider.InspectMeeting(t.Context(), old.ID)
	requireNoErrors(t, err, provider.EndMeeting(t.Context(), old))
	replacement := createdMeeting(t, provider)
	stale := domain.Meeting{ID: old.ID, CreateTime: discovered.CreateTime}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	holder := holdInspectionState(t, ctx, database.Pool)
	var info domain.MeetingInfo
	before := sharedInspectionState(t, database.Pool)

	inspector := core.Run(ctx, testLogger(), "shared-inspection-race", func() error {
		var err error
		info, err = provider.InspectMeeting(ctx, old.ID)
		return err
	})
	ender := core.Run(ctx, testLogger(), "shared-stale-end", func() error { return provider.EndMeeting(ctx, stale) })
	joiner := core.Run(ctx, testLogger(), "shared-stale-join", func() error {
		_, err := provider.JoinLink(ctx, stale, testParticipant())
		return err
	})
	waitSharedLocks(t, ctx, database.Pool, 3)
	releaseErr := holder.Commit(ctx)
	inspectErr, endErr, joinErr := await(t, ctx, inspector), await(t, ctx, ender), await(t, ctx, joiner)
	after := sharedInspectionState(t, database.Pool)

	requireNoErrors(t, releaseErr, inspectErr)
	requireFailure(t, endErr, FailureNotFound)
	requireFailure(t, joinErr, FailureNotFound)
	if before != after || info != (domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: replacement.CreateTime}) {
		t.Fatal("concurrent stale operations affected inspected replacement")
	}
}

func TestSharedFakeInspectionCancellation(t *testing.T) {
	database := testdb.New(t)
	pool := testdb.OpenPool(t, database.URL)
	provider := newShared(t, pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	issuedLink(t, provider, room, testParticipant())
	before := sharedInspectionState(t, pool)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	holder := holdInspectionState(t, ctx, database.Pool)
	waiting, cancelWaiting := context.WithCancel(ctx)
	defer cancelWaiting()
	var info domain.MeetingInfo

	result := core.Run(ctx, testLogger(), "shared-cancelled-inspection", func() error {
		var err error
		info, err = provider.InspectMeeting(waiting, room.ID)
		return err
	})
	waitSharedLocks(t, ctx, database.Pool, 1)
	cancelWaiting()
	operationErr := await(t, ctx, result)
	for pool.Stat().AcquiredConns() != 0 {
		if ctx.Err() != nil {
			t.Fatal("cancelled inspection did not release its connection within the original deadline")
		}
		runtime.Gosched()
	}
	releaseErr := holder.Commit(ctx)
	after := sharedInspectionState(t, pool)
	current, subsequentErr := provider.InspectMeeting(ctx, room.ID)

	requireFailure(t, operationErr, context.Canceled)
	requireNoErrors(t, releaseErr, subsequentErr)
	if info != (domain.MeetingInfo{}) {
		t.Fatal("cancelled inspection returned a partial snapshot")
	}
	if before != after {
		t.Fatal("cancelled inspection changed provider state")
	}
	if current != (domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: room.CreateTime}) {
		t.Fatal("inspection after cancellation returned an incorrect snapshot")
	}
}

func TestMeetingProviderInspectionDiagnostics(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared=%v", shared), func(t *testing.T) {
			var records bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug}))
			var provider inspectionProvider
			if shared {
				var err error
				provider, err = NewSharedFake(t.Context(), testdb.New(t).Pool, sharedClock(), sharedRandom(1), logger, testLimits())
				requireNoErrors(t, err)
			} else {
				provider = configuredFake(t, sharedClock(), sharedRandom(1), logger, testLimits())
			}
			room := createdMeeting(t, provider)
			participant := testParticipant()
			link := issuedLink(t, provider, room, participant)
			records.Reset()

			_, successErr := provider.InspectMeeting(t.Context(), room.ID)
			_, invalidErr := provider.InspectMeeting(t.Context(), domain.ProviderMeetingID{})
			unavailableErr := provider.SimulateUnavailable(t.Context(), true)
			_, failureErr := provider.InspectMeeting(t.Context(), room.ID)
			output := records.String()

			requireNoErrors(t, successErr, unavailableErr)
			requireFailure(t, invalidErr, FailureInvalidInput)
			requireFailure(t, failureErr, FailureUnavailable)
			for _, marker := range []string{`"msg":"meeting.provider.completed","operation":"inspect","outcome":"ok"`, `"msg":"meeting.provider.failed","operation":"inspect","kind":"invalid_input"`, `"msg":"meeting.provider.failed","operation":"inspect","kind":"unavailable"`} {
				if !strings.Contains(output, marker) {
					t.Fatal("inspection diagnostic missing fixed fields")
				}
			}
			for _, forbidden := range []string{room.Name, participant.Name, link.Reveal(), fmt.Sprint(room.ID), fmt.Sprint(participant.ID), `"level":"INFO"`} {
				if strings.Contains(output, forbidden) {
					t.Fatal("inspection diagnostics contain unsafe data or level")
				}
			}
		})
	}
}

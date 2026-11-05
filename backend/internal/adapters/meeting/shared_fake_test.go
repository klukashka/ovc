package meeting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	clockadapter "ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func sharedClock() Clock { return fixedClock{instant: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)} }

func sharedRandom(value byte) io.Reader { return bytes.NewReader(bytes.Repeat([]byte{value}, 32*1024)) }

func newShared(t *testing.T, pool *pgxpool.Pool, limits FakeLimits, seed byte) *SharedFake {
	t.Helper()
	provider, err := NewSharedFake(t.Context(), pool, sharedClock(), sharedRandom(seed), testLogger(), limits)
	requireNoErrors(t, err)
	return provider
}

func sharedSize(t *testing.T, pool *pgxpool.Pool) stateSize {
	t.Helper()
	var size stateSize
	err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM fake_meeting_rooms), (SELECT count(*) FROM fake_meeting_links), (SELECT count(*) FROM fake_meeting_participants)`).Scan(&size.meetings, &size.links, &size.participants)
	requireNoErrors(t, err)
	return size
}

func TestSharedFakeProviderContract(t *testing.T) {
	runProviderContract(t, func(t *testing.T) Provider { return newShared(t, testdb.New(t).Pool, testLimits(), 1) })
}

func TestSharedFakeInitializationAndLimits(t *testing.T) {
	t.Run("concurrent initialization", testSharedInitialization)
	t.Run("global room and generation bounds", func(t *testing.T) {
		database := testdb.New(t)
		limits := FakeLimits{Meetings: 1, Links: 2, Participants: 1, Generations: 2}
		first := newShared(t, database.Pool, limits, 1)
		second := newShared(t, testdb.OpenPool(t, database.URL), limits, 2)
		room := createdMeeting(t, first)
		other := meetingInput()
		other.ID[0] = 3

		_, roomCapacity := second.CreateMeeting(t.Context(), other)
		endErr := second.EndMeeting(t.Context(), room)
		recreated, recreateErr := second.CreateMeeting(t.Context(), meetingInput())
		endReplacement := first.EndMeeting(t.Context(), domain.Meeting{ID: room.ID, CreateTime: recreated})
		_, generationCapacity := first.CreateMeeting(t.Context(), other)
		size := sharedSize(t, database.Pool)

		requireFailure(t, roomCapacity, FailureCapacity)
		requireFailure(t, generationCapacity, FailureCapacity)
		requireNoErrors(t, endErr, recreateErr, endReplacement)
		if recreated != room.CreateTime+1 || size != (stateSize{}) {
			t.Fatalf("generation or cleanup mismatch: %d, %+v", recreated, size)
		}
	})
	t.Run("global links and persistent participant binding", func(t *testing.T) {
		database := testdb.New(t)
		first := newShared(t, database.Pool, FakeLimits{Meetings: 2, Links: 2, Participants: 1, Generations: 8}, 1)
		second := newShared(t, testdb.OpenPool(t, database.URL), first.limits, 2)
		room := createdMeeting(t, first)
		participant := testParticipant()
		link := issuedLink(t, first, room, participant)
		other := participant
		other.ID[0] = 3
		changedRole := participant
		changedRole.Role = domain.ParticipantViewer

		_, participantCapacity := second.JoinLink(t.Context(), room, other)
		enterErr := second.Enter(t.Context(), link, room, participant)
		leaveErr := first.Leave(t.Context(), room, participant.ID)
		_, roleErr := second.JoinLink(t.Context(), room, changedRole)
		_, secondLinkErr := second.JoinLink(t.Context(), room, participant)
		_, linkCapacity := first.JoinLink(t.Context(), room, participant)
		size := sharedSize(t, database.Pool)

		requireFailure(t, participantCapacity, FailureCapacity)
		requireFailure(t, roleErr, FailureConflict)
		requireFailure(t, linkCapacity, FailureCapacity)
		requireNoErrors(t, enterErr, leaveErr, secondLinkErr)
		if size != (stateSize{meetings: 1, links: 2, participants: 1}) {
			t.Fatalf("global bounds: %+v", size)
		}
	})
	t.Run("signed counters", func(t *testing.T) {
		database := testdb.New(t)
		provider := newShared(t, database.Pool, testLimits(), 1)
		room := createdMeeting(t, provider)
		_, setupErr := database.Pool.Exec(t.Context(), `UPDATE fake_meeting_state SET link_sequence = $1, last_create_time = $1 WHERE id`, int64(math.MaxInt64))
		requireNoErrors(t, setupErr)
		other := meetingInput()
		other.ID[0] = 4
		overflow := testLimits()
		overflow.Generations = math.MaxUint64

		_, linkErr := provider.JoinLink(t.Context(), room, testParticipant())
		_, createErr := provider.CreateMeeting(t.Context(), other)
		_, constructorErr := NewSharedFake(t.Context(), database.Pool, sharedClock(), sharedRandom(2), testLogger(), overflow)
		size := sharedSize(t, database.Pool)

		requireFailure(t, linkErr, FailureCapacity)
		requireFailure(t, createErr, FailureCapacity)
		requireFailure(t, constructorErr, FailureInvalidInput)
		if size != (stateSize{meetings: 1}) {
			t.Fatalf("counter refusal changed state: %+v", size)
		}
	})
}

func testSharedInitialization(t *testing.T) {
	database := testdb.New(t)
	otherPool := testdb.OpenPool(t, database.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lock, err := database.Pool.Begin(ctx)
	requireNoErrors(t, err)
	_, err = lock.Exec(ctx, `LOCK TABLE fake_meeting_state IN SHARE MODE`)
	requireNoErrors(t, err)
	t.Cleanup(func() {
		if err := lock.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error("release initialization lock failed")
		}
	})
	start := func(pool *pgxpool.Pool, seed byte) <-chan error {
		return core.Run(ctx, testLogger(), "shared-initialize", func() error {
			_, err := NewSharedFake(ctx, pool, sharedClock(), sharedRandom(seed), testLogger(), testLimits())
			return err
		})
	}

	first, second := start(database.Pool, 1), start(otherPool, 2)
	waitSharedLocks(t, ctx, database.Pool, 2)
	releaseErr := lock.Commit(ctx)
	firstErr, secondErr := await(t, ctx, first), await(t, ctx, second)
	state, readErr := scanSharedState(database.Pool.QueryRow(ctx, `SELECT `+sharedStateColumns+` FROM fake_meeting_state WHERE id`))
	existing, existingErr := NewSharedFake(ctx, otherPool, sharedClock(), strings.NewReader(""), testLogger(), testLimits())
	mismatch := testLimits()
	mismatch.Links++
	_, mismatchErr := NewSharedFake(ctx, otherPool, sharedClock(), sharedRandom(3), testLogger(), mismatch)

	requireNoErrors(t, releaseErr, firstErr, secondErr, readErr, existingErr)
	requireFailure(t, mismatchErr, FailureConflict)
	if existing == nil || state.limits != testLimits() || (hex.EncodeToString(state.namespace) != strings.Repeat("01", 32) && hex.EncodeToString(state.namespace) != strings.Repeat("02", 32)) {
		t.Fatal("constructors did not converge")
	}
}

func waitSharedLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, minimum int) {
	t.Helper()
	for {
		var count int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND cardinality(pg_blocking_pids(pid)) > 0`).Scan(&count)
		requireNoErrors(t, err)
		if count >= minimum {
			return
		}
		if ctx.Err() != nil {
			t.Fatal("provider did not reach database lock")
		}
	}
}

func TestSharedFakeGenerationAndRevocation(t *testing.T) {
	database := testdb.New(t)
	first := newShared(t, database.Pool, testLimits(), 1)
	second := newShared(t, testdb.OpenPool(t, database.URL), testLimits(), 2)
	room := createdMeeting(t, first)
	participant := testParticipant()
	link := issuedLink(t, first, room, participant)
	changedParticipant := participant
	changedParticipant.ID[0] = 3
	changedRole := participant
	changedRole.Role = domain.ParticipantViewer
	address := link.Reveal()
	parts := strings.Split(address, "/")
	parts[len(parts)-1] = strings.Repeat("00", 32)
	modified := NewJoinLink(strings.Join(parts, "/"))

	wrongParticipant := second.Enter(t.Context(), link, room, changedParticipant)
	wrongRole := second.Enter(t.Context(), link, room, changedRole)
	wrongToken := second.Enter(t.Context(), modified, room, participant)
	firstEnter := second.Enter(t.Context(), NewJoinLink(address), room, participant)
	secondEnter := first.Enter(t.Context(), link, room, participant)
	running, infoErr := second.MeetingInfo(t.Context(), room)
	ended := first.EndMeeting(t.Context(), room)
	released := sharedSize(t, database.Pool)
	replacementTime, createErr := second.CreateMeeting(t.Context(), meetingInput())
	staleEnd := first.EndMeeting(t.Context(), room)
	staleLink := second.Enter(t.Context(), link, room, participant)
	replacement := room
	replacement.CreateTime = replacementTime
	reboundLink := second.Enter(t.Context(), link, replacement, participant)
	replacementInfo, replacementErr := first.MeetingInfo(t.Context(), replacement)

	requireFailure(t, wrongParticipant, FailureInvalidInput)
	requireFailure(t, wrongRole, FailureInvalidInput)
	requireFailure(t, wrongToken, FailureInvalidInput)
	requireFailure(t, staleEnd, FailureNotFound)
	requireFailure(t, staleLink, FailureNotFound)
	requireFailure(t, reboundLink, FailureInvalidInput)
	requireNoErrors(t, firstEnter, secondEnter, infoErr, ended, createErr, replacementErr)
	if running != (domain.MeetingInfo{Status: domain.MeetingRunning, CreateTime: room.CreateTime, Participants: 1}) || released != (stateSize{}) || replacementTime != room.CreateTime+1 || replacementInfo.Status != domain.MeetingCreated {
		t.Fatal("shared lifecycle mismatch")
	}
}

func TestSharedFakeCancellationAndResources(t *testing.T) {
	t.Run("observed lock cancellation", testSharedLockCancellation)
	t.Run("one connection shared clock", func(t *testing.T) {
		database := testdb.New(t)
		database.HoldAllConnectionsBut(t, 1)
		clock := clockadapter.NewShared(database.Pool)
		_, setErr := clock.Set(t.Context(), time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC))
		requireNoErrors(t, setErr)
		provider, setupErr := NewSharedFake(t.Context(), database.Pool, clock, sharedRandom(1), testLogger(), testLimits())
		requireNoErrors(t, setupErr)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		created, err := provider.CreateMeeting(ctx, meetingInput())

		requireNoErrors(t, err)
		if int64(created) != time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC).UnixMilli() {
			t.Fatalf("shared clock create time = %d", created)
		}
	})
	t.Run("resource failures and existing create", testSharedResourceFailures)
	t.Run("concurrent entropy", func(t *testing.T) {
		database := testdb.New(t)
		limits := testLimits()
		limits.Links = 8
		provider := newShared(t, database.Pool, limits, 1)
		room := createdMeeting(t, provider)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		results := make([]<-chan error, 16)

		for index := range results {
			results[index] = core.Run(ctx, testLogger(), "shared-link", func() error {
				_, err := provider.JoinLink(ctx, room, testParticipant())
				return err
			})
		}
		failures := make([]error, 0, len(results))
		for _, result := range results {
			failures = append(failures, await(t, ctx, result))
		}
		size := sharedSize(t, database.Pool)

		var succeeded, capacity int
		for _, err := range failures {
			if err == nil {
				succeeded++
			} else if errors.Is(err, FailureCapacity) {
				capacity++
			} else {
				t.Fatal(err)
			}
		}
		if succeeded != 8 || capacity != 8 {
			t.Fatalf("concurrent capacity: succeeded=%d refused=%d", succeeded, capacity)
		}
		if size != (stateSize{meetings: 1, links: 8, participants: 1}) {
			t.Fatalf("concurrent entropy size = %+v", size)
		}
	})
}

func testSharedLockCancellation(t *testing.T) {
	database := testdb.New(t)
	provider := newShared(t, database.Pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	holder, err := database.Pool.Begin(ctx)
	requireNoErrors(t, err)
	_, err = holder.Exec(ctx, `SELECT id FROM fake_meeting_state WHERE id FOR UPDATE`)
	requireNoErrors(t, err)
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error("release provider lock failed")
		}
	})
	waiting, cancelWaiting := context.WithCancel(ctx)
	defer cancelWaiting()

	result := core.Run(ctx, testLogger(), "shared-cancelled-link", func() error { _, err := provider.JoinLink(waiting, room, testParticipant()); return err })
	waitSharedLocks(t, ctx, database.Pool, 1)
	cancelWaiting()
	operationErr := await(t, ctx, result)
	releaseErr := holder.Commit(ctx)
	size := sharedSize(t, database.Pool)
	_, subsequentErr := provider.JoinLink(ctx, room, testParticipant())

	requireFailure(t, operationErr, context.Canceled)
	requireNoErrors(t, releaseErr, subsequentErr)
	if size != (stateSize{meetings: 1}) {
		t.Fatal("cancelled lock waiter mutated state")
	}
}

func testSharedResourceFailures(t *testing.T) {
	database := testdb.New(t)
	provider := newShared(t, database.Pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	broken, setupErr := NewSharedFake(t.Context(), database.Pool, failingClock{failure: errors.New("private clock error")}, strings.NewReader(""), testLogger(), testLimits())
	requireNoErrors(t, setupErr)
	other := meetingInput()
	other.ID[0] = 3

	existing, existingErr := broken.CreateMeeting(t.Context(), meetingInput())
	_, clockErr := broken.CreateMeeting(t.Context(), other)
	_, randomErr := broken.JoinLink(t.Context(), room, testParticipant())
	size := sharedSize(t, database.Pool)

	requireNoErrors(t, existingErr)
	requireFailure(t, clockErr, FailureClock)
	requireFailure(t, randomErr, FailureRandomness)
	if existing != room.CreateTime || size != (stateSize{meetings: 1}) {
		t.Fatal("resource failure changed shared state")
	}
}

func TestSharedFakePrivacyAndStorage(t *testing.T) {
	database := testdb.New(t)
	var logs bytes.Buffer
	provider, setupErr := NewSharedFake(t.Context(), database.Pool, sharedClock(), sharedRandom(1), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), testLimits())
	requireNoErrors(t, setupErr)
	room := createdMeeting(t, provider)
	participant := testParticipant()
	link := issuedLink(t, provider, room, participant)
	independent := newShared(t, testdb.New(t).Pool, testLimits(), 2)
	independentRoom := createdMeeting(t, independent)
	otherLink := issuedLink(t, independent, independentRoom, participant)
	encoded, encodeErr := json.Marshal(link)
	requireNoErrors(t, encodeErr)
	digest := sha256.Sum256([]byte(link.Reveal()))
	var storedDigest []byte
	var storedRows string
	var columns int
	separatePool := testdb.OpenPool(t, database.URL)
	broken, brokenSetupErr := NewSharedFake(t.Context(), separatePool, sharedClock(), sharedRandom(4), provider.logger, testLimits())
	requireNoErrors(t, brokenSetupErr)
	separatePool.Close()

	enterErr := provider.Enter(t.Context(), link, room, participant)
	foreignErr := provider.Enter(t.Context(), otherLink, room, participant)
	readErr := database.Pool.QueryRow(t.Context(), `SELECT digest FROM fake_meeting_links`).Scan(&storedDigest)
	rowsErr := database.Pool.QueryRow(t.Context(), `SELECT json_agg(row_to_json(p))::text FROM fake_meeting_participants p`).Scan(&storedRows)
	columnsErr := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_name IN ('fake_meeting_links', 'fake_meeting_participants') AND column_name IN ('name', 'token', 'address')`).Scan(&columns)
	_, databaseErr := broken.MeetingInfo(t.Context(), room)
	_, inspectionErr := broken.InspectMeeting(t.Context(), room.ID)
	container := struct{ Link JoinLink }{Link: link}
	textValue, textErr := link.MarshalText()
	containerJSON, containerErr := json.Marshal(container)
	formats := fmt.Sprintf("%s %v %+v %#v %q %x %v %#v", link, link, link, link, link, link, container, container) + string(encoded) + string(textValue) + string(containerJSON)
	provider.logger.DebugContext(t.Context(), "redaction", "link", link, "container", container)

	requireNoErrors(t, enterErr, readErr, rowsErr, columnsErr, textErr, containerErr)
	if !strings.Contains(logs.String(), `"kind":"database"`) {
		t.Fatal("missing safe database failure diagnostic")
	}
	requireFailure(t, foreignErr, FailureInvalidInput)
	requireFailure(t, databaseErr, FailureDatabase)
	requireFailure(t, inspectionErr, FailureDatabase)
	if !strings.Contains(logs.String(), `"operation":"inspect","kind":"database"`) {
		t.Fatal("inspection database failure diagnostic missing")
	}
	if !bytes.Equal(storedDigest, digest[:]) || columns != 0 {
		t.Fatal("stored capability schema mismatch")
	}
	for _, forbidden := range []string{participant.Name, room.Name, link.Reveal(), database.URL.Reveal(), hex.EncodeToString(digest[:])} {
		if strings.Contains(logs.String(), forbidden) || strings.Contains(formats, forbidden) || strings.Contains(storedRows, forbidden) {
			t.Fatal("private provider data escaped")
		}
	}
}

func TestSharedFakeMigrationRoundTrip(t *testing.T) {
	database := testdb.NewMigratedTo(t, 22)
	_, seedErr := database.Pool.Exec(t.Context(), `INSERT INTO users (id, role, state, email, password_hash, name, language, timezone, theme, consent_version, consent_at, unverified_since, created_at, updated_at) VALUES ('00000000-0000-0000-0000-000000000001', 'tutor', 'unverified', 'migration@example.test', '$argon2id$unused', 'Migration account', 'en', 'UTC', 'system', 'v1', $1, $1, $1, $1)`, time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC))
	requireNoErrors(t, seedErr)

	upErr := database.MigrateUpTo(t.Context(), 23)
	requireNoErrors(t, upErr)
	provider := newShared(t, database.Pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	issuedLink(t, provider, room, testParticipant())
	downErr := database.MigrateDownTo(t.Context(), 22)
	var removed bool
	removedErr := database.Pool.QueryRow(t.Context(), `SELECT to_regclass('public.fake_meeting_state') IS NULL`).Scan(&removed)
	secondUp := database.MigrateUpTo(t.Context(), 23)
	var users int
	preservedErr := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE email = 'migration@example.test'`).Scan(&users)

	requireNoErrors(t, upErr, downErr, removedErr, secondUp, preservedErr)
	if !removed || users != 1 {
		t.Fatal("fake provider migration affected business state")
	}
}

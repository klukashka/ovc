package meeting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

func requireNoErrors(t *testing.T, failures ...error) {
	t.Helper()
	for _, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func requireFailure(t *testing.T, err error, expected error) {
	t.Helper()
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
}

func mustEnter(t *testing.T, provider *Fake, meeting domain.Meeting, participant domain.Participant) JoinLink {
	t.Helper()
	link := issuedLink(t, provider, meeting, participant)
	if err := provider.Enter(t.Context(), link, meeting, participant); err != nil {
		t.Fatal(err)
	}
	return link
}

type stateSize struct {
	meetings     int
	links        int
	participants int
}

func sizeOf(provider *Fake) stateSize {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()
	size := stateSize{meetings: len(provider.meetings), links: provider.links}
	for _, room := range provider.meetings {
		size.participants += len(room.participants)
	}
	return size
}

func TestFakeMeetingLifecycle(t *testing.T) {
	t.Run("reusable link has one participant", func(t *testing.T) {
		provider := newTestFake(t, testLimits())
		meeting := createdMeeting(t, provider)
		participant := testParticipant()
		link := issuedLink(t, provider, meeting, participant)

		firstErr := provider.Enter(t.Context(), link, meeting, participant)
		secondErr := provider.Enter(t.Context(), link, meeting, participant)
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)
		size := sizeOf(provider)

		requireNoErrors(t, firstErr, secondErr, infoErr)
		if info.Status != domain.MeetingRunning || info.Participants != 1 || size.participants != 1 {
			t.Fatalf("info = %+v, size = %+v", info, size)
		}
	})
	t.Run("departure leaves an empty created room", func(t *testing.T) {
		provider := newTestFake(t, testLimits())
		meeting := createdMeeting(t, provider)
		participant := testParticipant()
		mustEnter(t, provider, meeting, participant)

		leaveErr := provider.Leave(t.Context(), meeting, participant.ID)
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		requireNoErrors(t, leaveErr, infoErr)
		if info.Status != domain.MeetingCreated || info.Participants != 0 {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("end revokes links and frozen clock recreation cannot reuse handles", func(t *testing.T) {
		provider := newTestFake(t, testLimits())
		oldMeeting := createdMeeting(t, provider)
		participant := testParticipant()
		oldLink := mustEnter(t, provider, oldMeeting, participant)

		endErr := provider.EndMeeting(t.Context(), oldMeeting)
		endedSize := sizeOf(provider)
		created, createErr := provider.CreateMeeting(t.Context(), meetingInput())
		newMeeting := meetingInput()
		newMeeting.CreateTime = created
		oldEnterErr := provider.Enter(t.Context(), oldLink, newMeeting, participant)
		oldHandleErr := provider.Enter(t.Context(), oldLink, oldMeeting, participant)
		oldEndErr := provider.EndMeeting(t.Context(), oldMeeting)
		oldInfo, oldInfoErr := provider.MeetingInfo(t.Context(), oldMeeting)
		oldJoin, oldJoinErr := provider.JoinLink(t.Context(), oldMeeting, participant)
		newLink, linkErr := provider.JoinLink(t.Context(), newMeeting, participant)
		info, infoErr := provider.MeetingInfo(t.Context(), newMeeting)

		requireNoErrors(t, endErr, createErr, oldInfoErr, linkErr, infoErr)
		requireFailure(t, oldEnterErr, FailureInvalidInput)
		requireFailure(t, oldHandleErr, FailureNotFound)
		requireFailure(t, oldEndErr, FailureNotFound)
		requireFailure(t, oldJoinErr, FailureNotFound)
		if endedSize != (stateSize{}) || created <= oldMeeting.CreateTime || newLink.Reveal() == oldLink.Reveal() || !oldJoin.IsZero() || oldInfo.Status != domain.MeetingNotFound || info.Status != domain.MeetingCreated {
			t.Fatalf("generation or cleanup invariant failed: ended size = %+v, old = %+v, new = %+v", endedSize, oldInfo, info)
		}
	})
	t.Run("generation exhaustion is explicit", func(t *testing.T) {
		limits := testLimits()
		limits.Generations = 1
		provider := newTestFake(t, limits)
		meeting := createdMeeting(t, provider)
		requireNoErrors(t, provider.EndMeeting(t.Context(), meeting))

		created, err := provider.CreateMeeting(t.Context(), meetingInput())
		size := sizeOf(provider)

		requireFailure(t, err, FailureCapacity)
		if created != 0 || size != (stateSize{}) {
			t.Fatalf("exhaustion mutated state: created = %d, size = %+v", created, size)
		}
	})
}

func TestFakeMeetingIsolationAndBounds(t *testing.T) {
	t.Run("instances are independent", func(t *testing.T) {
		first := newTestFake(t, testLimits())
		second := newTestFake(t, testLimits())
		meeting := createdMeeting(t, first)
		link := issuedLink(t, first, meeting, testParticipant())

		info, infoErr := second.MeetingInfo(t.Context(), meeting)
		enterErr := second.Enter(t.Context(), link, meeting, testParticipant())
		firstInfo, firstErr := first.MeetingInfo(t.Context(), meeting)

		requireNoErrors(t, infoErr, firstErr)
		requireFailure(t, enterErr, FailureNotFound)
		if info.Status != domain.MeetingNotFound || firstInfo.Status != domain.MeetingCreated {
			t.Fatalf("instance states = %+v / %+v", firstInfo, info)
		}
	})
	t.Run("identical deterministic instances cannot redeem foreign capabilities", testForeignCapabilities)
	t.Run("tampered credentials and bindings are refused", func(t *testing.T) {
		provider := newTestFake(t, testLimits())
		meeting := createdMeeting(t, provider)
		participant := testParticipant()
		link := issuedLink(t, provider, meeting, participant)
		otherParticipant := participant
		otherParticipant.ID = domain.ParticipantID{3}
		otherRole := participant
		otherRole.Role = domain.ParticipantViewer
		otherGeneration := meeting
		otherGeneration.CreateTime++
		otherMeeting := meeting
		otherMeeting.ID = domain.ProviderMeetingID{3}
		before := sizeOf(provider)

		unknownErr := provider.Enter(t.Context(), NewJoinLink("ovc-fake://unknown"), meeting, participant)
		tamperedErr := provider.Enter(t.Context(), NewJoinLink(link.Reveal()+"x"), meeting, participant)
		participantErr := provider.Enter(t.Context(), link, meeting, otherParticipant)
		roleErr := provider.Enter(t.Context(), link, meeting, otherRole)
		generationErr := provider.Enter(t.Context(), link, otherGeneration, participant)
		meetingErr := provider.Enter(t.Context(), link, otherMeeting, participant)
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)
		after := sizeOf(provider)

		requireNoErrors(t, infoErr)
		for _, err := range []error{unknownErr, tamperedErr, participantErr, roleErr} {
			requireFailure(t, err, FailureInvalidInput)
		}
		requireFailure(t, generationErr, FailureNotFound)
		requireFailure(t, meetingErr, FailureNotFound)
		if before != after || info.Status != domain.MeetingCreated || info.Participants != 0 {
			t.Fatalf("refused input mutated state: %+v -> %+v, info = %+v", before, after, info)
		}
	})
	t.Run("meeting and link caps release on end", func(t *testing.T) {
		limits := testLimits()
		limits.Meetings, limits.Links = 1, 1
		provider := newTestFake(t, limits)
		meeting := createdMeeting(t, provider)
		issuedLink(t, provider, meeting, testParticipant())
		other := meetingInput()
		other.ID = domain.ProviderMeetingID{3}

		_, meetingErr := provider.CreateMeeting(t.Context(), other)
		_, linkErr := provider.JoinLink(t.Context(), meeting, testParticipant())
		endErr := provider.EndMeeting(t.Context(), meeting)
		created, createErr := provider.CreateMeeting(t.Context(), other)
		other.CreateTime = created
		link, newLinkErr := provider.JoinLink(t.Context(), other, testParticipant())
		size := sizeOf(provider)

		requireFailure(t, meetingErr, FailureCapacity)
		requireFailure(t, linkErr, FailureCapacity)
		requireNoErrors(t, endErr, createErr, newLinkErr)
		if link.IsZero() || size != (stateSize{meetings: 1, links: 1, participants: 1}) {
			t.Fatalf("capacity did not recover: %+v", size)
		}
	})
	t.Run("participant cap and role binding survive departures", func(t *testing.T) {
		limits := testLimits()
		limits.Participants = 1
		provider := newTestFake(t, limits)
		meeting := createdMeeting(t, provider)
		participant := testParticipant()
		link := mustEnter(t, provider, meeting, participant)
		requireNoErrors(t, provider.Leave(t.Context(), meeting, participant.ID))
		other := participant
		other.ID = domain.ParticipantID{3}
		otherRole := participant
		otherRole.Role = domain.ParticipantViewer

		_, capacityErr := provider.JoinLink(t.Context(), meeting, other)
		_, conflictErr := provider.JoinLink(t.Context(), meeting, otherRole)
		enterErr := provider.Enter(t.Context(), link, meeting, participant)
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		requireFailure(t, capacityErr, FailureCapacity)
		requireFailure(t, conflictErr, FailureConflict)
		requireNoErrors(t, enterErr, infoErr)
		if info.Participants != 1 || sizeOf(provider).participants != 1 {
			t.Fatalf("participant bound failed: %+v", info)
		}
	})
	t.Run("invalid input and conflicting create do not mutate", testInvalidInput)
	t.Run("provider failures are explicit and sanitized", testProviderFailures)
	t.Run("clock and randomness failures do not publish state", testSourceFailures)
	t.Run("constructor requires finite positive bounds", testConstructorBounds)
	t.Run("diagnostics contain fixed safe fields", testDiagnostics)
}

func testInvalidInput(t *testing.T) {
	provider := newTestFake(t, testLimits())
	meeting := createdMeeting(t, provider)
	conflicting := meetingInput()
	conflicting.Name = "Different lesson"
	participant := testParticipant()
	participant.Role = domain.ParticipantRole("invalid")
	before := sizeOf(provider)

	_, conflictErr := provider.CreateMeeting(t.Context(), conflicting)
	_, roleErr := provider.JoinLink(t.Context(), meeting, participant)
	_, unknownErr := provider.JoinLink(t.Context(), meetingInput(), testParticipant())
	_, zeroErr := provider.CreateMeeting(t.Context(), domain.Meeting{Name: "Name"})
	_, nameErr := provider.CreateMeeting(t.Context(), domain.Meeting{ID: domain.ProviderMeetingID{3}, Name: strings.Repeat("x", domain.LessonTitleMaxRunes+1)})
	after := sizeOf(provider)

	requireFailure(t, conflictErr, FailureConflict)
	requireFailure(t, roleErr, FailureInvalidInput)
	requireFailure(t, unknownErr, FailureNotFound)
	requireFailure(t, zeroErr, FailureInvalidInput)
	requireFailure(t, nameErr, FailureInvalidInput)
	if before != after {
		t.Fatalf("invalid input mutated state: %+v -> %+v", before, after)
	}
}

func testProviderFailures(t *testing.T) {
	provider := newTestFake(t, testLimits())
	meeting := createdMeeting(t, provider)
	requireNoErrors(t, provider.SimulateUnavailable(t.Context(), true))
	before := sizeOf(provider)

	_, createErr := provider.CreateMeeting(t.Context(), meetingInput())
	_, infoErr := provider.MeetingInfo(t.Context(), meeting)
	_, joinErr := provider.JoinLink(t.Context(), meeting, testParticipant())
	endErr := provider.EndMeeting(t.Context(), meeting)
	after := sizeOf(provider)
	restoreErr := provider.SimulateUnavailable(t.Context(), false)
	info, restoredInfoErr := provider.MeetingInfo(t.Context(), meeting)

	for _, err := range []error{createErr, infoErr, joinErr, endErr} {
		requireFailure(t, err, FailureUnavailable)
	}
	requireNoErrors(t, restoreErr, restoredInfoErr)
	if before != after || info.Status != domain.MeetingCreated {
		t.Fatalf("failure changed state: %+v -> %+v, info = %+v", before, after, info)
	}
}

func testDiagnostics(t *testing.T) {
	var records bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug})).With("component", "meeting")
	provider := configuredFake(t, fixedClock{instant: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}, bytes.NewReader(bytes.Repeat([]byte{42}, 64)), logger, testLimits())
	meeting := createdMeeting(t, provider)
	participant := testParticipant()
	link := issuedLink(t, provider, meeting, participant)

	enterErr := provider.Enter(t.Context(), link, meeting, participant)
	_, infoErr := provider.MeetingInfo(t.Context(), meeting)
	leaveErr := provider.Leave(t.Context(), meeting, participant.ID)
	failureErr := provider.SimulateUnavailable(t.Context(), true)
	endErr := provider.EndMeeting(t.Context(), meeting)
	output := records.String()

	requireNoErrors(t, enterErr, infoErr, leaveErr, failureErr)
	requireFailure(t, endErr, FailureUnavailable)
	for _, secret := range []string{meeting.Name, participant.Name, link.Reveal(), strings.Repeat("2a", 32)} {
		if strings.Contains(output, secret) {
			t.Fatal("diagnostics disclosed private content")
		}
	}
	for _, marker := range []string{`"msg":"meeting.provider.completed"`, `"msg":"meeting.provider.failed"`, `"kind":"unavailable"`, `"operation":"info"`, `"component":"meeting"`} {
		if !strings.Contains(output, marker) {
			t.Fatalf("diagnostics missing %s", marker)
		}
	}
	if strings.Contains(output, `"level":"INFO"`) {
		t.Fatal("provider poll logged at INFO")
	}
}

type heldClock struct {
	entered chan<- struct{}
	release <-chan struct{}
}

func (clock heldClock) Instant(ctx context.Context) (time.Time, error) {
	select {
	case clock.entered <- struct{}{}:
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	}
	select {
	case <-clock.release:
		return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), nil
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	}
}

func await[T any](t *testing.T, ctx context.Context, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatal("barrier deadline exceeded")
		var zero T
		return zero
	}
}

func TestFakeMeetingCancellationAndConcurrency(t *testing.T) {
	t.Run("cancelled calls leave state unchanged", testCancelledCalls)
	t.Run("cancellation while waiting for state lock", testWaitingCancellation)
	t.Run("cancellation after random read prevents publishing", testRandomReadCancellation)
	t.Run("completed transition is not rolled back", func(t *testing.T) {
		provider := newTestFake(t, testLimits())
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		meeting := meetingInput()

		created, createErr := provider.CreateMeeting(ctx, meeting)
		cancel()
		meeting.CreateTime = created
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		requireNoErrors(t, createErr, infoErr)
		if info.Status != domain.MeetingCreated || info.CreateTime != created {
			t.Fatalf("completed create was rolled back: %+v", info)
		}
	})
	t.Run("parallel provider lifecycle preserves invariants", testConcurrentLifecycle)
}

func testCancelledCalls(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *Fake, domain.Meeting, JoinLink) error
	}{
		{name: "create", run: func(ctx context.Context, provider *Fake, _ domain.Meeting, _ JoinLink) error {
			input := meetingInput()
			input.ID = domain.ProviderMeetingID{3}
			_, err := provider.CreateMeeting(ctx, input)
			return err
		}},
		{name: "info", run: func(ctx context.Context, provider *Fake, meeting domain.Meeting, _ JoinLink) error {
			_, err := provider.MeetingInfo(ctx, meeting)
			return err
		}},
		{name: "join", run: func(ctx context.Context, provider *Fake, meeting domain.Meeting, _ JoinLink) error {
			_, err := provider.JoinLink(ctx, meeting, testParticipant())
			return err
		}},
		{name: "end", run: func(ctx context.Context, provider *Fake, meeting domain.Meeting, _ JoinLink) error {
			return provider.EndMeeting(ctx, meeting)
		}},
		{name: "enter", run: func(ctx context.Context, provider *Fake, meeting domain.Meeting, link JoinLink) error {
			return provider.Enter(ctx, link, meeting, testParticipant())
		}},
		{name: "leave", run: func(ctx context.Context, provider *Fake, meeting domain.Meeting, _ JoinLink) error {
			return provider.Leave(ctx, meeting, testParticipant().ID)
		}},
		{name: "availability", run: func(ctx context.Context, provider *Fake, _ domain.Meeting, _ JoinLink) error {
			return provider.SimulateUnavailable(ctx, true)
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			provider := newTestFake(t, testLimits())
			meeting := createdMeeting(t, provider)
			link := mustEnter(t, provider, meeting, testParticipant())
			before := sizeOf(provider)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := operation.run(ctx, provider, meeting, link)
			after := sizeOf(provider)
			info, infoErr := provider.MeetingInfo(t.Context(), meeting)

			requireFailure(t, err, context.Canceled)
			requireNoErrors(t, infoErr)
			if before != after || info.Status != domain.MeetingRunning || info.Participants != 1 {
				t.Fatalf("cancelled operation changed state: %+v -> %+v, info = %+v", before, after, info)
			}
		})
	}
}

func testWaitingCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	provider := configuredFake(t, heldClock{entered: entered, release: release}, strings.NewReader(""), testLogger(), testLimits())
	first := core.Run(ctx, testLogger(), "meeting-create-holder", func() error {
		_, err := provider.CreateMeeting(ctx, meetingInput())
		return err
	})
	await(t, ctx, entered)
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	defer cancelWaiter()
	started := make(chan struct{})

	waiter := core.Run(ctx, testLogger(), "meeting-create-waiter", func() error {
		close(started)
		other := meetingInput()
		other.ID = domain.ProviderMeetingID{3}
		_, err := provider.CreateMeeting(waiterCtx, other)
		return err
	})
	await(t, ctx, started)
	cancelWaiter()
	close(release)
	firstErr := await(t, ctx, first)
	waiterErr := await(t, ctx, waiter)
	size := sizeOf(provider)

	requireNoErrors(t, firstErr)
	requireFailure(t, waiterErr, context.Canceled)
	if size.meetings != 1 || provider.generations != 1 {
		t.Fatalf("cancelled waiter committed: %+v", size)
	}
}

func testRandomReadCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	t.Cleanup(func() { requireNoErrors(t, reader.Close(), writer.Close()) })
	provider := configuredFake(t, fixedClock{instant: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}, reader, testLogger(), testLimits())
	meeting := createdMeeting(t, provider)
	joinCtx, cancelJoin := context.WithCancel(ctx)
	defer cancelJoin()
	joined := core.Run(ctx, testLogger(), "meeting-join-held-randomness", func() error {
		_, err := provider.JoinLink(joinCtx, meeting, testParticipant())
		return err
	})
	firstHalf := core.Run(ctx, testLogger(), "meeting-randomness-first-half", func() error {
		_, err := writer.Write(bytes.Repeat([]byte{42}, 16))
		return err
	})
	requireNoErrors(t, await(t, ctx, firstHalf))

	cancelJoin()
	secondHalf := core.Run(ctx, testLogger(), "meeting-randomness-second-half", func() error {
		_, err := writer.Write(bytes.Repeat([]byte{42}, 16))
		return err
	})
	writeErr := await(t, ctx, secondHalf)
	joinErr := await(t, ctx, joined)
	size := sizeOf(provider)

	requireNoErrors(t, writeErr)
	requireFailure(t, joinErr, context.Canceled)
	if size.links != 0 || size.participants != 0 || provider.linkSequence != 0 {
		t.Fatalf("cancelled link was published: %+v", size)
	}
}

func testConcurrentLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	provider := newTestFake(t, testLimits())
	start := make(chan struct{})
	results := make([]<-chan error, 16)
	for index := range results {
		results[index] = core.Run(ctx, testLogger(), "meeting-concurrent-lifecycle", func() error {
			select {
			case <-start:
			case <-ctx.Done():
				return ctx.Err()
			}
			for range 4 {
				if err := concurrentLifecycle(ctx, provider); err != nil {
					return err
				}
			}
			return nil
		})
	}

	close(start)
	failures := make([]error, len(results))
	for index, result := range results {
		failures[index] = await(t, ctx, result)
	}
	created, createErr := provider.CreateMeeting(ctx, meetingInput())
	meeting := meetingInput()
	meeting.CreateTime = created
	endErr := provider.EndMeeting(ctx, meeting)
	size := sizeOf(provider)

	requireNoErrors(t, failures...)
	requireNoErrors(t, createErr, endErr)
	if size != (stateSize{}) {
		t.Fatalf("concurrent lifecycle leaked state: %+v", size)
	}
}

func concurrentLifecycle(ctx context.Context, provider *Fake) error {
	meeting := meetingInput()
	created, err := provider.CreateMeeting(ctx, meeting)
	if err != nil {
		return err
	}
	meeting.CreateTime = created
	info, err := provider.MeetingInfo(ctx, meeting)
	if err != nil {
		return err
	}
	if info.Participants != 0 || (info.Status != domain.MeetingCreated && info.Status != domain.MeetingNotFound) {
		return errors.New("concurrent info violates the unjoined lifecycle")
	}
	if _, err := provider.JoinLink(ctx, meeting, testParticipant()); err != nil && !errors.Is(err, FailureNotFound) {
		return err
	}
	if err := provider.EndMeeting(ctx, meeting); err != nil && !errors.Is(err, FailureNotFound) {
		return err
	}
	return nil
}

func TestMeetingLinkRedaction(t *testing.T) {
	provider := newTestFake(t, testLimits())
	meeting := createdMeeting(t, provider)
	link := issuedLink(t, provider, meeting, testParticipant())
	var records bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&records, nil))
	container := struct{ Link JoinLink }{Link: link}

	formatted := fmt.Sprintf("%s %v %+v %#v %q %x %v %#v", link, link, link, link, link, link, container, container)
	jsonValue, jsonErr := json.Marshal(container)
	textValue, textErr := link.MarshalText()
	logger.InfoContext(t.Context(), "redaction.check", "link", link, "container", container)
	wrapped := fmt.Errorf("meeting link %v: %w", link, FailureInvalidInput)
	output := formatted + string(jsonValue) + string(textValue) + records.String() + wrapped.Error()

	requireNoErrors(t, jsonErr, textErr)
	if link.IsZero() || !strings.HasPrefix(link.Reveal(), "ovc-fake://join/") || !strings.Contains(output, "[redacted]") {
		t.Fatal("explicit access or redaction marker is missing")
	}
	if strings.Contains(output, link.Reveal()) || strings.Contains(output, strings.Repeat("2a", 32)) {
		t.Fatal("sensitive link escaped redaction")
	}
}

type failingClock struct {
	failure error
}

func (clock failingClock) Instant(context.Context) (time.Time, error) {
	return time.Time{}, clock.failure
}

func testSourceFailures(t *testing.T) {
	var records bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug}))
	privateError := errors.New("Private participant name and provider credentials")
	clockProvider := configuredFake(t, failingClock{failure: privateError}, strings.NewReader(""), logger, testLimits())
	randomProvider := configuredFake(t, fixedClock{instant: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}, strings.NewReader(""), logger, testLimits())
	meeting := createdMeeting(t, randomProvider)

	created, clockErr := clockProvider.CreateMeeting(t.Context(), meetingInput())
	link, randomErr := randomProvider.JoinLink(t.Context(), meeting, testParticipant())
	clockSize := sizeOf(clockProvider)
	randomSize := sizeOf(randomProvider)
	output := records.String() + fmt.Sprintf("%v %v", clockErr, randomErr)

	requireFailure(t, clockErr, FailureClock)
	requireFailure(t, randomErr, FailureRandomness)
	if created != 0 || !link.IsZero() || clockSize != (stateSize{}) || randomSize != (stateSize{meetings: 1}) {
		t.Fatalf("source failure published state: clock = %+v, random = %+v", clockSize, randomSize)
	}
	if strings.Contains(output, privateError.Error()) || !strings.Contains(output, `"kind":"clock"`) || !strings.Contains(output, `"kind":"randomness"`) {
		t.Fatal("source error diagnostics are missing or unsafe")
	}
}

func testConstructorBounds(t *testing.T) {
	limits := testLimits()
	cases := []FakeLimits{
		{Links: limits.Links, Participants: limits.Participants, Generations: limits.Generations},
		{Meetings: limits.Meetings, Participants: limits.Participants, Generations: limits.Generations},
		{Meetings: limits.Meetings, Links: limits.Links, Generations: limits.Generations},
		{Meetings: limits.Meetings, Links: limits.Links, Participants: limits.Participants},
	}

	failures := make([]error, len(cases))
	for index, invalid := range cases {
		_, failures[index] = NewFake(fixedClock{}, strings.NewReader(""), testLogger(), invalid)
	}

	for _, err := range failures {
		requireFailure(t, err, FailureInvalidInput)
	}
}

func testForeignCapabilities(t *testing.T) {
	first := newTestFake(t, testLimits())
	second := newTestFake(t, testLimits())
	firstMeeting := createdMeeting(t, first)
	secondMeeting := createdMeeting(t, second)
	participant := testParticipant()
	firstLink := issuedLink(t, first, firstMeeting, participant)
	secondLink := issuedLink(t, second, secondMeeting, participant)

	foreignErr := second.Enter(t.Context(), firstLink, secondMeeting, participant)
	reconstructedErr := second.Enter(t.Context(), NewJoinLink(secondLink.Reveal()), secondMeeting, participant)
	beforeOwn, beforeErr := second.MeetingInfo(t.Context(), secondMeeting)
	ownErr := second.Enter(t.Context(), secondLink, secondMeeting, participant)
	afterOwn, afterErr := second.MeetingInfo(t.Context(), secondMeeting)

	requireFailure(t, foreignErr, FailureInvalidInput)
	requireFailure(t, reconstructedErr, FailureInvalidInput)
	requireNoErrors(t, beforeErr, ownErr, afterErr)
	if firstLink.Reveal() != secondLink.Reveal() || firstMeeting.CreateTime != secondMeeting.CreateTime || beforeOwn.Status != domain.MeetingCreated || afterOwn.Status != domain.MeetingRunning || afterOwn.Participants != 1 {
		t.Fatal("instance capability isolation did not preserve deterministic inputs and own entry")
	}
}

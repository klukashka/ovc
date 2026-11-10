package meeting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func TestSharedFakeCanonicalCapabilities(t *testing.T) {
	cases := []struct {
		name      string
		transform func(string) string
	}{
		{name: "uppercase token", transform: func(address string) string { return address[:len(address)-64] + strings.Repeat("AB", 32) }},
		{name: "query", transform: func(address string) string { return address + "?x=1" }},
		{name: "fragment", transform: func(address string) string { return address + "#x" }},
		{name: "leading zero", transform: func(address string) string { return strings.Replace(address, "/1/", "/01/", 1) }},
		{name: "signed sequence", transform: func(address string) string { return strings.Replace(address, "/1/", "/+1/", 1) }},
		{name: "oversized", transform: func(address string) string { return address + strings.Repeat("a", 8192) }},
		{name: "wrong namespace", transform: func(address string) string {
			return strings.Replace(address, strings.Repeat("01", 32), strings.Repeat("02", 32), 1)
		}},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			database := testdb.New(t)
			provider := newShared(t, database.Pool, testLimits(), 1)
			room := createdMeeting(t, provider)
			link := issuedLink(t, provider, room, testParticipant())
			modified := NewJoinLink(entry.transform(link.Reveal()))

			err := provider.Enter(t.Context(), modified, room, testParticipant())
			info, infoErr := provider.MeetingInfo(t.Context(), room)
			size := sharedSize(t, database.Pool)

			requireFailure(t, err, FailureInvalidInput)
			requireNoErrors(t, infoErr)
			if info.Status != domain.MeetingCreated || info.Participants != 0 || size != (stateSize{meetings: 1, links: 1, participants: 1}) {
				t.Fatal("malformed capability mutated state")
			}
		})
	}
}

func TestSharedFakeAvailabilityAndCancelledOperations(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *SharedFake, domain.Meeting, JoinLink) error
	}{
		{name: "create", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			_, err := p.CreateMeeting(ctx, m, nil)
			return err
		}},
		{name: "inspect", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			_, err := p.InspectMeeting(ctx, m.ID)
			return err
		}},
		{name: "info", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			_, err := p.MeetingInfo(ctx, m)
			return err
		}},
		{name: "join", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			_, err := p.JoinLink(ctx, m, testParticipant())
			return err
		}},
		{name: "end", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			return p.EndMeeting(ctx, m)
		}},
		{name: "enter", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, l JoinLink) error {
			return p.Enter(ctx, l, m, testParticipant())
		}},
		{name: "leave", run: func(ctx context.Context, p *SharedFake, m domain.Meeting, _ JoinLink) error {
			return p.Leave(ctx, m, testParticipant().ID)
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			database := testdb.New(t)
			first := newShared(t, database.Pool, testLimits(), 1)
			second := newShared(t, testdb.OpenPool(t, database.URL), testLimits(), 2)
			room := createdMeeting(t, first)
			link := issuedLink(t, first, room, testParticipant())
			requireNoErrors(t, first.Enter(t.Context(), link, room, testParticipant()), first.SimulateUnavailable(t.Context(), true))
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			unavailable := operation.run(t.Context(), second, room, link)
			cancelledAvailability := second.SimulateUnavailable(ctx, false)
			stillUnavailable := operation.run(t.Context(), first, room, link)
			restore := second.SimulateUnavailable(t.Context(), false)
			cancelled := operation.run(ctx, second, room, link)
			size := sharedSize(t, database.Pool)
			info, infoErr := first.MeetingInfo(t.Context(), room)

			requireFailure(t, unavailable, FailureUnavailable)
			requireFailure(t, stillUnavailable, FailureUnavailable)
			requireFailure(t, cancelledAvailability, context.Canceled)
			requireFailure(t, cancelled, context.Canceled)
			requireNoErrors(t, restore, infoErr)
			if size != (stateSize{meetings: 1, links: 1, participants: 1}) || info.Participants != 1 {
				t.Fatal("availability or cancellation mutated room")
			}
		})
	}
}

func TestSharedFakeDatabaseConstraints(t *testing.T) {
	statements := []string{
		`UPDATE fake_meeting_state SET namespace = NULL`,
		`UPDATE fake_meeting_state SET namespace = decode('01','hex')`,
		`UPDATE fake_meeting_state SET generations = -1`,
		`UPDATE fake_meeting_state SET link_sequence = -1`,
		`UPDATE fake_meeting_state SET last_create_time = NULL`,
		`UPDATE fake_meeting_state SET max_generations = 0`,
		`UPDATE fake_meeting_rooms SET create_time = 0`,
		`UPDATE fake_meeting_rooms SET id = '00000000-0000-0000-0000-000000000000'`,
		`UPDATE fake_meeting_participants SET participant_id = '00000000-0000-0000-0000-000000000000'`,
		`UPDATE fake_meeting_participants SET role = NULL`,
		`UPDATE fake_meeting_participants SET role = 'owner'`,
		`UPDATE fake_meeting_participants SET present = NULL`,
		`UPDATE fake_meeting_links SET create_time = NULL`,
		`UPDATE fake_meeting_links SET digest = decode('01','hex')`,
	}
	database := testdb.New(t)
	provider := newShared(t, database.Pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	issuedLink(t, provider, room, testParticipant())
	failures := make([]error, 0, len(statements))

	for _, statement := range statements {
		_, err := database.Pool.Exec(t.Context(), statement)
		failures = append(failures, err)
	}
	size := sharedSize(t, database.Pool)

	for index, err := range failures {
		if err == nil {
			t.Errorf("constraint case %d accepted invalid state", index)
		}
	}
	if size != (stateSize{meetings: 1, links: 1, participants: 1}) {
		t.Fatal("constraint refusal changed provider state")
	}
}

func TestSharedFakeInitializationResourceFailure(t *testing.T) {
	database := testdb.New(t)

	_, err := NewSharedFake(t.Context(), database.Pool, sharedClock(), strings.NewReader(""), testLogger(), testLimits())
	_, queryErr := scanSharedState(database.Pool.QueryRow(t.Context(), `SELECT `+sharedStateColumns+` FROM fake_meeting_state WHERE id`))

	requireFailure(t, err, FailureRandomness)
	if !errors.Is(queryErr, pgx.ErrNoRows) {
		t.Fatal("failed initialization published provider state")
	}
}

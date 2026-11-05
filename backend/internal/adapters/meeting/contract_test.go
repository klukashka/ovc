package meeting

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type fixedClock struct {
	instant time.Time
}

func (clock fixedClock) Instant(context.Context) (time.Time, error) {
	return clock.instant, nil
}

func testLimits() FakeLimits {
	return FakeLimits{Meetings: 8, Links: 128, Participants: 8, Generations: 128}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func newTestFake(t *testing.T, limits FakeLimits) *Fake {
	t.Helper()
	return configuredFake(t, fixedClock{instant: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)},
		bytes.NewReader(bytes.Repeat([]byte{42}, 32*1024)), testLogger(), limits)
}

func configuredFake(t *testing.T, clock Clock, random io.Reader, logger *slog.Logger, limits FakeLimits) *Fake {
	t.Helper()
	provider, err := NewFake(clock, random, logger, limits)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func meetingInput() domain.Meeting {
	return domain.Meeting{ID: domain.ProviderMeetingID{1}, Name: "Private lesson title"}
}

func testParticipant() domain.Participant {
	return domain.Participant{ID: domain.ParticipantID{2}, Name: "Private participant name", Role: domain.ParticipantModerator}
}

func createdMeeting(t *testing.T, provider Provider) domain.Meeting {
	t.Helper()
	meeting := meetingInput()
	created, err := provider.CreateMeeting(t.Context(), meeting)
	if err != nil {
		t.Fatal(err)
	}
	meeting.CreateTime = created
	return meeting
}

func issuedLink(t *testing.T, provider Provider, meeting domain.Meeting, participant domain.Participant) JoinLink {
	t.Helper()
	link, err := provider.JoinLink(t.Context(), meeting, participant)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

func TestMeetingProviderContract(t *testing.T) {
	runProviderContract(t, func(t *testing.T) Provider { return newTestFake(t, testLimits()) })
}

func runProviderContract(t *testing.T, factory func(*testing.T) Provider) {
	t.Helper()
	t.Run("inspection", func(t *testing.T) {
		provider := factory(t)

		missing, missingErr := provider.InspectMeeting(t.Context(), meetingInput().ID)
		invalid, invalidErr := provider.InspectMeeting(t.Context(), domain.ProviderMeetingID{})
		room := createdMeeting(t, provider)
		created, createdErr := provider.InspectMeeting(t.Context(), room.ID)
		endErr := provider.EndMeeting(t.Context(), room)
		ended, endedErr := provider.InspectMeeting(t.Context(), room.ID)

		requireNoErrors(t, missingErr, createdErr, endErr, endedErr)
		requireFailure(t, invalidErr, FailureInvalidInput)
		if missing != (domain.MeetingInfo{Status: domain.MeetingNotFound}) || ended != missing || invalid != (domain.MeetingInfo{}) || room.CreateTime <= 0 || created != (domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: room.CreateTime}) {
			t.Fatal("inspection did not return the current generation or exact absence")
		}
	})
	t.Run("absent meeting", func(t *testing.T) {
		provider := factory(t)

		info, err := provider.MeetingInfo(t.Context(), meetingInput())

		if err != nil || info.Status != domain.MeetingNotFound || info.Participants != 0 {
			t.Fatalf("absent info = %+v, error = %v", info, err)
		}
	})
	t.Run("idempotent create", func(t *testing.T) {
		provider := factory(t)
		meeting := createdMeeting(t, provider)

		created, createErr := provider.CreateMeeting(t.Context(), meetingInput())
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		if createErr != nil || infoErr != nil || created != meeting.CreateTime || created <= 0 || info.CreateTime != created || info.Status != domain.MeetingCreated {
			t.Fatalf("create = %d, info = %+v, errors = %v / %v", created, info, createErr, infoErr)
		}
	})
	t.Run("link does not join", func(t *testing.T) {
		provider := factory(t)
		meeting := createdMeeting(t, provider)

		link, joinErr := provider.JoinLink(t.Context(), meeting, testParticipant())
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		if joinErr != nil || infoErr != nil || link.IsZero() || info.Status != domain.MeetingCreated || info.Participants != 0 {
			t.Fatalf("link empty = %v, info = %+v, errors = %v / %v", link.IsZero(), info, joinErr, infoErr)
		}
	})
	t.Run("idempotent end", func(t *testing.T) {
		provider := factory(t)
		meeting := createdMeeting(t, provider)

		firstErr := provider.EndMeeting(t.Context(), meeting)
		secondErr := provider.EndMeeting(t.Context(), meeting)
		info, infoErr := provider.MeetingInfo(t.Context(), meeting)

		if firstErr != nil || secondErr != nil || infoErr != nil || info.Status != domain.MeetingNotFound {
			t.Fatalf("end errors = %v / %v, info = %+v, error = %v", firstErr, secondErr, info, infoErr)
		}
	})
}

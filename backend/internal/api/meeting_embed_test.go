package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/schedule"
)

type embedFlags struct {
	lessonDetail bool
	joinAnswer   bool
}

func TestEmbedOverrideReachesTheLessonDetailAndTheJoinAnswerUntilTheNextOverride(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	embed := &core.Switch{}
	base := h.embedInstance(t, embed)

	before := h.embedFlagsAt(t, base, parties.tutorToken, lesson)
	embed.Set(true)
	during := h.embedFlagsAt(t, base, parties.tutorToken, lesson)
	embed.Set(false)
	after := h.embedFlagsAt(t, base, parties.tutorToken, lesson)

	off := embedFlags{lessonDetail: false, joinAnswer: false}
	on := embedFlags{lessonDetail: true, joinAnswer: true}
	if before != off || during != on || after != off {
		t.Fatalf("before %+v, during %+v, after %+v", before, during, after)
	}
}

func (h *authHarness) embedInstance(t *testing.T, embed *core.Switch) string {
	t.Helper()
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: h.store, Provider: h.provider, Presentations: meetings.NoPresentations{}, Kind: domain.MeetingProviderFake, Clock: h.clock, Random: h.random, Logger: h.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})
	options := h.options
	options.Schedule = schedule.New(schedule.Dependencies{Store: h.store, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger, Addresses: h.files, Embed: embed})
	options.Meetings = meetings.NewLifecycle(meetings.LifecycleDependencies{
		Rooms: rooms, Store: h.store, Provider: h.provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger,
		Enabled: true, Embed: embed, ProviderCall: timing.ProviderCall,
	})
	return h.serve(t, options)
}

func (h *authHarness) embedFlagsAt(t *testing.T, base string, token string, lesson string) embedFlags {
	t.Helper()
	detail, err := h.trySendTo(base, http.MethodGet, lessonsRoute+"/"+lesson, token)
	if err != nil || detail.status != http.StatusOK {
		t.Fatalf("the lesson detail answered %d %s: %v", detail.status, detail.body, err)
	}
	join, err := h.trySendTo(base, http.MethodPost, lessonRoute(lesson, "join"), token)
	if err != nil || join.status != http.StatusOK {
		t.Fatalf("the join answered %d %s: %v", join.status, join.body, err)
	}
	var view roomLesson
	if err := json.Unmarshal([]byte(detail.body), &view); err != nil {
		t.Fatalf("decode the lesson detail: %v", err)
	}
	var answer joinAnswer
	if err := json.Unmarshal([]byte(join.body), &answer); err != nil {
		t.Fatalf("decode the join answer: %v", err)
	}
	return embedFlags{lessonDetail: view.EmbedAllowed, joinAnswer: answer.EmbedAllowed}
}

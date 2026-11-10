package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/worker"
)

const providerStateLock = "SELECT " + lockedRowPosition + " FROM fake_meeting_state WHERE id AND $1::text = 'state' FOR UPDATE"

type providerClockGate struct {
	source  clock.Source
	mutex   sync.Mutex
	armed   atomic.Bool
	reached chan struct{}
	release chan struct{}
}

func (g *providerClockGate) Instant(ctx context.Context) (time.Time, error) {
	if g.armed.CompareAndSwap(true, false) {
		g.mutex.Lock()
		reached, release := g.reached, g.release
		g.mutex.Unlock()
		close(reached)
		select {
		case <-release:
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
	return g.source.Instant(ctx)
}

func (g *providerClockGate) arm() (<-chan struct{}, func()) {
	reached, release := make(chan struct{}), make(chan struct{})
	g.mutex.Lock()
	g.reached, g.release = reached, release
	g.mutex.Unlock()
	g.armed.Store(true)
	return reached, sync.OnceFunc(func() { close(release) })
}

type providerEntropyGate struct {
	source  io.Reader
	mutex   sync.Mutex
	armed   atomic.Bool
	reached chan struct{}
	release chan struct{}
}

func (g *providerEntropyGate) Read(buffer []byte) (int, error) {
	if g.armed.CompareAndSwap(true, false) {
		g.mutex.Lock()
		reached, release := g.reached, g.release
		g.mutex.Unlock()
		close(reached)
		<-release
	}
	return g.source.Read(buffer)
}

func (g *providerEntropyGate) arm(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	reached, release := make(chan struct{}), make(chan struct{})
	g.mutex.Lock()
	g.reached, g.release = reached, release
	g.mutex.Unlock()
	g.armed.Store(true)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	return reached, releaseOnce
}

type joinAnswer struct {
	URL          string `json:"url"`
	EmbedAllowed bool   `json:"embed_allowed"`
}

type roomLesson struct {
	lessonView
	RoomOpen     bool `json:"room_open"`
	EmbedAllowed bool `json:"embed_allowed"`
	TookPlace    bool `json:"took_place"`
}

func newMeetingHarness(t *testing.T) *authHarness {
	t.Helper()
	h := newAuthHarness(t)
	cleanup := meetings.New(h.store, h.provider, domain.MeetingProviderFake, h.logger)
	lanes := []worker.Lane{{Name: domain.LaneProviderCall, Concurrency: 4, PerformMeetingRecovery: cleanup.Cleanup, Perform: h.lifecycle.CheckEndedHint}}
	scanner := meetings.NewRecoveryScanner(h.store, h.random, h.logger, clock.System{}.Now)
	jobs := []worker.Job{
		{Name: domain.JobMeetingRecovery, Every: time.Minute, Run: scanner.Run},
		{Name: domain.JobAccountsPurge, Every: time.Hour, Run: h.purger.PurgeDeleted},
		{Name: domain.JobLessonStatus, Every: time.Minute, Run: h.jobs.run(endDueLessons)},
	}
	h.worker = worker.New(h.logger, h.clock, outbox.New(h.store, h.random, h.logger), lanes, jobs, worker.DefaultTiming())
	h.at(t, firstLessonStart.Add(-5*time.Minute))
	return h
}

func (h *authHarness) secondInstance(t *testing.T, permits int) string {
	t.Helper()
	return h.instance(t, func(dependencies *meetings.LifecycleDependencies) { dependencies.HintPermits = permits })
}

func (h *authHarness) instance(t *testing.T, adjust func(*meetings.LifecycleDependencies)) string {
	t.Helper()
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: h.store, Provider: h.provider, Presentations: meetings.NoPresentations{}, Kind: domain.MeetingProviderFake, Clock: h.clock, Random: h.random, Logger: h.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})
	dependencies := meetings.LifecycleDependencies{
		Rooms: rooms, Store: h.store, Provider: h.provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger,
		Enabled: true, ProviderCall: timing.ProviderCall,
	}
	adjust(&dependencies)
	options := h.options
	options.Meetings = meetings.NewLifecycle(dependencies)
	return h.serve(t, options)
}

type linkIssuedBarrier struct {
	logs    *safeBuffer
	issued  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *linkIssuedBarrier) Write(data []byte) (int, error) {
	count, err := b.logs.Write(data)
	if err == nil && bytes.Contains(data, []byte(`"meeting.provider.completed"`)) && bytes.Contains(data, []byte(`"operation":"join_link"`)) {
		first := false
		b.once.Do(func() { first = true })
		if first {
			close(b.issued)
			<-b.release
		}
	}
	return count, err
}

func (h *authHarness) releaseOnce(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}

func (h *authHarness) instanceLogging(t *testing.T, logger *slog.Logger) string {
	t.Helper()
	provider, err := meeting.NewSharedFake(context.Background(), h.database.Pool, h.clock, h.random, logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: h.store, Provider: provider, Presentations: meetings.NoPresentations{}, Kind: domain.MeetingProviderFake, Clock: h.clock, Random: h.random, Logger: h.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})
	options := h.options
	options.Meetings = meetings.NewLifecycle(meetings.LifecycleDependencies{
		Rooms: rooms, Store: h.store, Provider: provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger,
		Enabled: true, ProviderCall: timing.ProviderCall,
	})
	return h.serve(t, options)
}

func (h *authHarness) gatedInstance(t *testing.T) (string, *providerEntropyGate) {
	t.Helper()
	gate := &providerEntropyGate{source: h.random}
	provider, err := meeting.NewSharedFake(context.Background(), h.database.Pool, h.clock, gate, h.logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: h.store, Provider: provider, Presentations: meetings.NoPresentations{}, Kind: domain.MeetingProviderFake, Clock: h.clock, Random: h.random, Logger: h.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})
	options := h.options
	options.Meetings = meetings.NewLifecycle(meetings.LifecycleDependencies{
		Rooms: rooms, Store: h.store, Provider: provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger,
		Enabled: true, ProviderCall: timing.ProviderCall,
	})
	return h.serve(t, options), gate
}

func lessonRoute(lesson string, action string) string {
	return lessonsRoute + "/" + lesson + "/" + action
}

func (h *authHarness) joinCall(t *testing.T, token string, lesson string) response {
	t.Helper()
	return h.send(t, http.MethodPost, lessonRoute(lesson, "join"), "", jsonCall(token))
}

func decodeJoin(t *testing.T, result response) joinAnswer {
	t.Helper()
	var answer joinAnswer
	if err := json.Unmarshal([]byte(result.body), &answer); err != nil || result.status != http.StatusOK || answer.URL == "" {
		t.Fatalf("not a join answer: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("a join answer has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return answer
}

func (h *authHarness) joined(t *testing.T, token string, lesson string) joinAnswer {
	t.Helper()
	return decodeJoin(t, h.joinCall(t, token, lesson))
}

func (h *authHarness) endCall(t *testing.T, token string, lesson string) response {
	t.Helper()
	return h.send(t, http.MethodPost, lessonRoute(lesson, "end"), "", jsonCall(token))
}

func (h *authHarness) restartCall(t *testing.T, token string, lesson string) response {
	t.Helper()
	return h.send(t, http.MethodPost, lessonRoute(lesson, "restart-room"), "", jsonCall(token))
}

func (h *authHarness) inBackground(lesson string, action string, token string) <-chan pendingAnswer {
	return h.sendInBackground(http.MethodPost, lessonRoute(lesson, action), "", jsonCall(token))
}

func (h *authHarness) roomLesson(t *testing.T, token string, lesson string) roomLesson {
	t.Helper()
	result := h.lessonPage(t, token, lesson)
	var view roomLesson
	if err := json.Unmarshal([]byte(result.body), &view); err != nil || result.status != http.StatusOK {
		t.Fatalf("not a lesson: %d %s", result.status, result.body)
	}
	return view
}

func lessonUUID(t *testing.T, lesson string) domain.LessonID {
	t.Helper()
	id, valid := domain.ParseUUID[domain.LessonID](lesson)
	if !valid {
		t.Fatalf("%q is not a lesson identifier", lesson)
	}
	return id
}

func (h *authHarness) currentRoom(t *testing.T, lesson string) domain.Meeting {
	t.Helper()
	row, err := h.store.Repositories().Meetings.Current(context.Background(), lessonUUID(t, lesson))
	if err != nil {
		t.Fatal(err)
	}
	if row.State != domain.MeetingOpen || row.CreateTime == nil {
		t.Fatalf("the current room is %s", row.State)
	}
	return domain.Meeting{ID: row.ProviderMeetingID, CreateTime: *row.CreateTime}
}

func (h *authHarness) participantOf(t *testing.T, lesson string, account domain.UserID) domain.ParticipantID {
	t.Helper()
	var id domain.ParticipantID
	err := h.database.Pool.QueryRow(context.Background(), "SELECT participant_id FROM lesson_participants WHERE lesson_id = $1::uuid AND account_id = $2::uuid", lesson, account.String()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *authHarness) enter(t *testing.T, answer joinAnswer, room domain.Meeting, lesson string, account domain.UserID, role domain.ParticipantRole) error {
	t.Helper()
	participant := domain.Participant{ID: h.participantOf(t, lesson, account), Name: "A participant", Role: role}
	return h.provider.Enter(context.Background(), meeting.NewJoinLink(answer.URL), room, participant)
}

func (h *authHarness) providerRooms(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM fake_meeting_rooms WHERE $1::text = 'rooms'", "rooms")
}

func (h *authHarness) closeReason(t *testing.T, lesson string) string {
	t.Helper()
	var reason string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT coalesce(close_reason, state) FROM meetings WHERE lesson_id = $1::uuid ORDER BY close_reason IS NULL, ended_at, id LIMIT 1", lesson).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	return reason
}

func (h *authHarness) meetingCount(t *testing.T, lesson string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM meetings WHERE lesson_id = $1::uuid", lesson)
}

func (h *authHarness) unsettled(t *testing.T) int64 {
	t.Helper()
	count, err := h.store.Repositories().MeetingRecovery.Capacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func endDueLessons(jobs *schedule.Jobs, ctx context.Context, now time.Time) (int64, error) {
	return jobs.EndDue(ctx, now)
}

func (h *authHarness) serve(t *testing.T, options api.Options) string {
	t.Helper()
	handler, err := api.NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func (h *authHarness) trySendTo(base string, method string, path string, token string) (response, error) {
	request, err := http.NewRequestWithContext(context.Background(), method, base+path, nil)
	if err != nil {
		return response{}, err
	}
	for name, value := range jsonCall(token) {
		request.Header.Set(name, value)
	}
	answered, err := http.DefaultClient.Do(request)
	if err != nil {
		return response{}, err
	}
	body, err := io.ReadAll(answered.Body)
	if closeErr := answered.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return response{}, err
	}
	return response{status: answered.StatusCode, headers: answered.Header, body: string(body)}, nil
}

func awaitClosed(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(socketDeadline)
	defer deadline.Stop()
	select {
	case <-signal:
	case <-deadline.C:
		t.Fatal("the provider call did not reach its gate")
	}
}

func (h *authHarness) cycleCount(t *testing.T, provider string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(outbox_id) FROM meeting_recovery_cycles WHERE provider_meeting_id = $1::uuid", provider)
}

func (h *authHarness) uncertainRows(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(provider_meeting_id) FROM meeting_recovery WHERE create_state = 'uncertain' AND $1::text = 'uncertain'", "uncertain")
}

func (h *authHarness) seedTutorObligations(t *testing.T, lesson string, count int) {
	t.Helper()
	h.exec2(t, `WITH created AS (
    INSERT INTO meetings (id, lesson_id, provider, provider_meeting_id, routing_key, state, created_at, updated_at, ended_at, close_reason)
    SELECT gen_random_uuid(), $1::uuid, 'fake', gen_random_uuid(), gen_random_uuid(), 'closed', lessons.created_at, lessons.created_at, lessons.created_at, 'provider_error'
    FROM lessons CROSS JOIN generate_series(1, $2::text::int) WHERE lessons.id = $1::uuid
    RETURNING id, provider_meeting_id, created_at
), ledgers AS (
    INSERT INTO meeting_recovery (provider_meeting_id, provider, meeting_id, origin, create_state, cleanup_requested, created_at, updated_at)
    SELECT provider_meeting_id, 'fake', id, 'attempt', 'prepared', true, created_at, created_at FROM created
    RETURNING provider_meeting_id
)
UPDATE meeting_recovery_capacity SET unsettled = unsettled + (SELECT count(provider_meeting_id) FROM ledgers)`, lesson, fmt.Sprint(count))
}

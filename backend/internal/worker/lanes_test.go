package worker_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testmail"
	"ovc/backend/internal/worker"
)

const (
	laneDeadline       = 10 * time.Second
	panickingAddress   = "panicking@example.org"
	unreachableAddress = "unreachable@example.org"
)

type laneSizes struct {
	email  int32
	delete int32
}

var productionLanes = laneSizes{email: 10, delete: 4}

type laneHarness struct {
	logs      *bytes.Buffer
	store     *repositories.Store
	sender    *testmail.Sender
	worker    *worker.Worker
	delivered chan string
	deleted   chan domain.OutboxID
	emails    atomic.Int32
	deletes   atomic.Int32
	nextID    byte
	base      time.Time
}

type emailPayload struct {
	To string `json:"to"`
}

func newLaneHarness(t *testing.T, sizes laneSizes, random core.RandomSource) *laneHarness {
	t.Helper()
	pool := testdb.New(t).Pool
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := &laneHarness{
		logs:      logs,
		store:     repositories.NewStore(pool),
		sender:    testmail.New(mail.NewSink(pool, &countingRandom{})),
		delivered: make(chan string, 100),
		deleted:   make(chan domain.OutboxID, 100),
		base:      start,
	}
	t.Cleanup(h.releaseEverySend)
	lanes := []worker.Lane{
		{Name: domain.LaneEmail, Concurrency: sizes.email, Perform: h.sendEmail},
		{Name: domain.LaneStorageDelete, Concurrency: sizes.delete, Perform: h.deleteObject},
	}
	h.worker = worker.New(logger, clock.System{}, outbox.New(h.store, random, logger), lanes, nil, worker.DefaultTiming())
	return h
}

func (h *laneHarness) sendEmail(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (domain.OutboxDoneReason, error) {
	h.emails.Add(1)
	var payload emailPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return "", domain.PermanentEffect(err)
	}
	if payload.To == panickingAddress {
		panic("effect bug")
	}
	if payload.To == unreachableAddress {
		return "", errUnreachableProvider
	}
	message := mail.Message{To: payload.To, Subject: "Lane test", Text: "Lane test", HTML: "<p>Lane test</p>", MessageID: row.IdempotencyKey + "@ovc.example", Date: now}
	if err := h.sender.Send(ctx, message); err != nil {
		return "", err
	}
	h.delivered <- payload.To
	return domain.DoneSent, nil
}

func (h *laneHarness) deleteObject(_ context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (domain.OutboxDoneReason, error) {
	h.deletes.Add(1)
	h.deleted <- row.ID
	return domain.DoneSent, nil
}

var (
	errUnreachableProvider = errors.New("unreachable provider")
	errRandomExhausted     = errors.New("only lease tokens can be read")
)

type leaseTokensOnly struct{}

func (leaseTokensOnly) Read(buffer []byte) (int, error) {
	if len(buffer) != len(domain.LeaseToken{}) {
		return 0, errRandomExhausted
	}
	clear(buffer)
	return len(buffer), nil
}

type countingRandom struct {
	counter atomic.Uint64
}

func (c *countingRandom) Read(buffer []byte) (int, error) {
	clear(buffer)
	counter := binary.LittleEndian.AppendUint64(nil, c.counter.Add(1))
	copy(buffer, counter)
	return len(buffer), nil
}

func recipient(number int) string {
	return fmt.Sprintf("student-%d@example.org", number)
}

func (h *laneHarness) insertEmail(t *testing.T, to string, priority domain.OutboxPriority, due time.Time) domain.OutboxID {
	t.Helper()
	payload, err := json.Marshal(emailPayload{To: to})
	if err != nil {
		t.Fatal(err)
	}
	return h.insertRow(t, domain.LaneEmail, priority, payload, due)
}

func (h *laneHarness) insertRow(t *testing.T, lane domain.OutboxLane, priority domain.OutboxPriority, payload []byte, due time.Time) domain.OutboxID {
	t.Helper()
	h.nextID++
	id := domain.OutboxID{h.nextID}
	inserted, err := h.store.Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID: id, Lane: lane, Priority: priority, Kind: "test.effect", Payload: payload,
		IdempotencyKey: fmt.Sprintf("effect-%d", h.nextID), NextAttemptAt: due, CreatedAt: due,
	})
	if err != nil || !inserted {
		t.Fatalf("insert: %v, %v", inserted, err)
	}
	return id
}

func (h *laneHarness) insertEmails(t *testing.T, count int) []domain.OutboxID {
	t.Helper()
	ids := make([]domain.OutboxID, 0, count)
	for number := 1; number <= count; number++ {
		ids = append(ids, h.insertEmail(t, recipient(number), domain.PriorityNormal, h.base.Add(time.Duration(number-count-1)*time.Second)))
	}
	return ids
}

func (h *laneHarness) insertDeletes(t *testing.T, count int) []domain.OutboxID {
	t.Helper()
	ids := make([]domain.OutboxID, 0, count)
	for range count {
		ids = append(ids, h.insertRow(t, domain.LaneStorageDelete, domain.PriorityNormal, []byte(`{"key":"objects/lane-test"}`), h.base))
	}
	return ids
}

func (h *laneHarness) row(t *testing.T, id domain.OutboxID) repositories.OutboxRow {
	t.Helper()
	row, err := h.store.Repositories().Outbox.ByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *laneHarness) statuses(t *testing.T, ids []domain.OutboxID) map[domain.OutboxStatus]int {
	t.Helper()
	counts := map[domain.OutboxStatus]int{}
	for _, id := range ids {
		counts[h.row(t, id).Status]++
	}
	return counts
}

func (h *laneHarness) tickInBackground(now time.Time) <-chan error {
	return core.Run(context.Background(), slog.New(slog.DiscardHandler), "tick-under-test", func() error { return h.worker.Tick(context.Background(), now) })
}

func (h *laneHarness) runInBackground(ctx context.Context) <-chan error {
	return core.Run(ctx, slog.New(slog.DiscardHandler), "worker-under-test", func() error { return h.worker.Run(ctx) })
}

func (h *laneHarness) releaseEverySend() {
	h.sender.Unpause()
	h.sender.ReleaseAll()
}

func (h *laneHarness) finish(t *testing.T, done <-chan error) error {
	t.Helper()
	h.releaseEverySend()
	return awaitResult(t, done)
}

func (h *laneHarness) awaitHeld(t *testing.T, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), laneDeadline)
	defer cancel()
	if err := h.sender.AwaitHeld(ctx, count); err != nil {
		t.Fatal(err)
	}
}

func awaitSignals[T any](t *testing.T, signals <-chan T, count int) []T {
	t.Helper()
	received := make([]T, 0, count)
	deadline := time.After(laneDeadline)
	for len(received) < count {
		select {
		case signal := <-signals:
			received = append(received, signal)
		case <-deadline:
			t.Fatalf("received %d of %d signals", len(received), count)
		}
	}
	return received
}

func awaitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(laneDeadline):
		t.Fatal("the worker did not finish")
		return nil
	}
}

func TestALaneRefillsASlotAsSoonAsAnEffectFinishes(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	ids := h.insertEmails(t, 11)
	h.sender.Pause()
	tick := h.tickInBackground(start)
	h.awaitHeld(t, 10)

	h.sender.Release(recipient(1))
	h.awaitHeld(t, 10)
	held := h.sender.HeldRecipients()
	firstStatus := h.row(t, ids[0]).Status
	tickErr := h.finish(t, tick)

	if tickErr != nil {
		t.Fatal(tickErr)
	}
	if firstStatus != domain.OutboxDone || slices.Contains(held, recipient(1)) || !slices.Contains(held, recipient(11)) {
		t.Fatalf("first row %s, held while it finished %v", firstStatus, held)
	}
	if counts := h.statuses(t, ids); counts[domain.OutboxDone] != 11 {
		t.Fatalf("statuses %v", counts)
	}
}

func TestOneSlowEmailDoesNotHoldBackTheRestOfTheLane(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	ids := h.insertEmails(t, 21)
	h.sender.Pause()
	tick := h.tickInBackground(start)
	h.awaitHeld(t, 10)

	h.sender.Unpause()
	for number := 2; number <= 10; number++ {
		h.sender.Release(recipient(number))
	}
	delivered := awaitSignals(t, h.delivered, 20)
	held := h.sender.HeldRecipients()
	tickErr := h.finish(t, tick)

	if tickErr != nil {
		t.Fatal(tickErr)
	}
	if !slices.Equal(held, []string{recipient(1)}) || slices.Contains(delivered, recipient(1)) {
		t.Fatalf("held %v after the other twenty were delivered", held)
	}
	if counts := h.statuses(t, ids); counts[domain.OutboxDone] != 21 {
		t.Fatalf("statuses %v", counts)
	}
}

func TestLanesRunSideBySide(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	emails := h.insertEmails(t, 10)
	deletes := h.insertDeletes(t, 9)
	h.sender.Pause()
	tick := h.tickInBackground(start)
	h.awaitHeld(t, 10)

	deleted := awaitSignals(t, h.deleted, 9)
	held := len(h.sender.HeldRecipients())
	tickErr := h.finish(t, tick)

	if tickErr != nil {
		t.Fatal(tickErr)
	}
	if held != 10 || len(deleted) != 9 {
		t.Fatalf("%d storage deletes ran while %d emails were held", len(deleted), held)
	}
	if counts := h.statuses(t, append(emails, deletes...)); counts[domain.OutboxDone] != 19 {
		t.Fatalf("statuses %v", counts)
	}
}

func TestHighPriorityRowsAreClaimedFirstWhenASlotFrees(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	h.insertEmails(t, 15)
	h.sender.Pause()
	tick := h.tickInBackground(start)
	h.awaitHeld(t, 10)
	urgent := "urgent@example.org"
	h.insertEmail(t, urgent, domain.PriorityHigh, start)

	h.sender.Release(recipient(1))
	h.awaitHeld(t, 10)
	held := h.sender.HeldRecipients()
	tickErr := h.finish(t, tick)

	if tickErr != nil {
		t.Fatal(tickErr)
	}
	if !slices.Contains(held, urgent) || slices.Contains(held, recipient(11)) {
		t.Fatalf("the freed slot went to %v", held)
	}
}

func TestTickDrainsEveryDueRowExactlyOnceAcrossLanes(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	emails := h.insertEmails(t, 25)
	deletes := h.insertDeletes(t, 9)
	later := h.insertEmail(t, "later@example.org", domain.PriorityHigh, start.Add(time.Second))

	if err := awaitResult(t, h.tickInBackground(start)); err != nil {
		t.Fatal(err)
	}
	if err := awaitResult(t, h.tickInBackground(start)); err != nil {
		t.Fatal(err)
	}

	for _, id := range append(emails, deletes...) {
		if row := h.row(t, id); row.Status != domain.OutboxDone || row.Attempts != 1 {
			t.Fatalf("row %x is %s after %d attempts", id, row.Status, row.Attempts)
		}
	}
	if row := h.row(t, later); row.Status != domain.OutboxPending || row.Attempts != 0 {
		t.Fatalf("the row due later is %s after %d attempts", row.Status, row.Attempts)
	}
	if h.emails.Load() != 25 || h.deletes.Load() != 9 {
		t.Fatalf("performed %d emails and %d deletes", h.emails.Load(), h.deletes.Load())
	}
}

func TestARunningLanePollsForARowQueuedWhileSomeSlotsAreBusy(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	h.base = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	ids := h.insertEmails(t, 5)
	h.sender.Pause()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := h.runInBackground(ctx)
	h.awaitHeld(t, 5)

	queued := h.insertEmail(t, "queued@example.org", domain.PriorityNormal, h.base)
	h.awaitHeld(t, 6)
	h.sender.Release(recipient(1))
	firstDelivered := awaitSignals(t, h.delivered, 1)
	heldAfterRelease := h.sender.HeldRecipients()
	cancel()
	runErr := h.finish(t, run)

	if runErr != nil {
		t.Fatal(runErr)
	}
	if !slices.Equal(firstDelivered, []string{recipient(1)}) || len(heldAfterRelease) != 5 || slices.Contains(heldAfterRelease, recipient(1)) {
		t.Fatalf("delivered %v, held after the release %v", firstDelivered, heldAfterRelease)
	}
	if counts := h.statuses(t, append(ids, queued)); counts[domain.OutboxDone] != 6 {
		t.Fatalf("statuses %v", counts)
	}
}

func TestShutdownStopsClaimingAndLetsStartedEffectsFinish(t *testing.T) {
	h := newLaneHarness(t, productionLanes, &countingRandom{})
	h.base = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	ids := h.insertEmails(t, 12)
	h.sender.Pause()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := h.runInBackground(ctx)
	h.awaitHeld(t, 10)

	cancel()
	runErr := h.finish(t, run)

	if runErr != nil {
		t.Fatal(runErr)
	}
	counts := h.statuses(t, ids)
	unclaimed := h.row(t, ids[11])
	if counts[domain.OutboxDone] != 10 || counts[domain.OutboxPending] != 2 || unclaimed.Attempts != 0 || h.emails.Load() != 10 {
		t.Fatalf("statuses %v, performed %d, the last row has %d attempts", counts, h.emails.Load(), unclaimed.Attempts)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"msg":"outbox.lane.started","lane":"email"`) || !strings.Contains(logs, `"msg":"outbox.lane.stopped","lane":"email"`) {
		t.Fatalf("no lane start and stop records in:\n%s", logs)
	}
}

func TestAPanickingEffectFreesItsSlotAndCountsAsAFailedAttempt(t *testing.T) {
	h := newLaneHarness(t, laneSizes{email: 1, delete: 1}, &countingRandom{})
	panicking := h.insertEmail(t, panickingAddress, domain.PriorityHigh, start)
	others := h.insertEmails(t, 2)

	tickErr := awaitResult(t, h.tickInBackground(start))

	if tickErr != nil {
		t.Fatal(tickErr)
	}
	row := h.row(t, panicking)
	if row.Status != domain.OutboxPending || row.Attempts != 1 || row.LastErrorCode == nil || !strings.Contains(h.logs.String(), "panic.recovered") {
		t.Fatalf("the panicking row is %s after %d attempts with code %v", row.Status, row.Attempts, row.LastErrorCode)
	}
	if counts := h.statuses(t, others); counts[domain.OutboxDone] != 2 {
		t.Fatalf("the rows behind the panic are %v", counts)
	}
}

type claimFailure struct {
	Level string `json:"level"`
	Lane  string `json:"lane"`
	Kind  string `json:"kind"`
}

func TestAClaimFailureEndsTheRun(t *testing.T) {
	h := newLaneHarness(t, productionLanes, bytes.NewReader(nil))
	h.insertEmails(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := awaitResult(t, h.runInBackground(ctx))

	if runErr == nil {
		t.Fatal("the run ended without an error")
	}
	var failures []claimFailure
	for line := range strings.Lines(h.logs.String()) {
		var record claimFailure
		if strings.Contains(line, `"msg":"outbox.claim.failed"`) && json.Unmarshal([]byte(line), &record) == nil {
			failures = append(failures, record)
		}
	}
	if len(failures) == 0 || failures[0].Level != "ERROR" || failures[0].Lane == "" || failures[0].Kind == "" {
		t.Fatalf("no ERROR outbox.claim.failed record with a lane and a kind: %+v", failures)
	}
}

func TestAFailedRetryUpdateEndsThePass(t *testing.T) {
	h := newLaneHarness(t, productionLanes, leaseTokensOnly{})
	unreachable := h.insertEmail(t, unreachableAddress, domain.PriorityHigh, start)
	behind := h.insertEmail(t, recipient(1), domain.PriorityNormal, start)

	tickErr := awaitResult(t, h.tickInBackground(start))

	if !errors.Is(tickErr, errRandomExhausted) {
		t.Fatalf("the pass ended with %v", tickErr)
	}
	if row := h.row(t, unreachable); row.Status != domain.OutboxInProgress || row.Attempts != 1 {
		t.Fatalf("the row whose retry update failed is %s after %d attempts", row.Status, row.Attempts)
	}
	if row := h.row(t, behind); row.Attempts > 1 {
		t.Fatalf("the row behind it was claimed %d times", row.Attempts)
	}
}

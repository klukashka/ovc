//go:build slice6_measurement

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"ovc/backend/internal/adapters/clock"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const measurementMessageSeed = `WITH inserted AS (
INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT md5($1::text || ':' || series.seq::text)::uuid, $1::uuid, series.seq, $2::uuid,
    'Synthetic measurement message', 1, $4::timestamptz, $4::timestamptz
FROM generate_series(1, $3::bigint) AS series(seq)
RETURNING seq
)
UPDATE channels SET last_seq = (SELECT max(seq) FROM inserted), last_activity_at = $4::timestamptz WHERE id = $1::uuid`

func seedMeasurementHistory(t *testing.T, h *authHarness, channel string, sender domain.UserID, count int) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), measurementMessageSeed, channel, sender.String(), count, authStart); err != nil {
		t.Fatal("measurement history seed failed")
	}
}

func measurementResources(t *testing.T, h *authHarness, phase string) {
	t.Helper()
	var synchronousCommit string
	if err := h.database.Pool.QueryRow(t.Context(), "SHOW synchronous_commit").Scan(&synchronousCommit); err != nil {
		t.Fatal("measurement database durability observation failed")
	}
	if synchronousCommit != "on" {
		t.Fatal("Go measurement database must retain synchronous_commit=on")
	}
	t.Logf("event=measurement.database synchronous_commit=%s", synchronousCommit)
	var activation time.Time
	var dailyRows, dailyBytes int64
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT active_at, (SELECT count(*) FROM daily_chat_upload_bytes), (SELECT coalesce(sum(completed_bytes), 0)::bigint FROM daily_chat_upload_bytes) FROM chat_upload_accounting_activation").Scan(&activation, &dailyRows, &dailyBytes); err != nil {
		t.Fatal("measurement daily accounting observation failed")
	}
	if h.now(t).Before(activation) {
		t.Fatal("measurement fixture clock predates immutable accounting activation")
	}
	t.Logf("event=measurement.daily_accounting phase=%s retained_rows=%d completed_bytes=%d retention=expected_until_past_day_cleanup", phase, dailyRows, dailyBytes)
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	var intentRows, capacity, receipts int64
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM upload_intents), (SELECT used FROM upload_intent_capacity), (SELECT count(*) FROM upload_cleanup_receipts)").Scan(&intentRows, &capacity, &receipts); err != nil {
		t.Fatal("measurement upload intent schema unavailable; integrate the final upload implementation first")
	}
	t.Logf("event=measurement.mode smoke=%t", *slice6Smoke)
	t.Logf("event=measurement.upload_resources phase=%s intents=%d capacity=%d receipts=%d fixture_log_bytes=%d", phase, intentRows, capacity, receipts, len(h.logs.String()))
	pool := h.database.Pool.Stat()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		t.Fatal("measurement RSS unavailable")
	}
	rss, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		t.Fatal("measurement RSS malformed")
	}
	descriptorPath := "/dev/fd"
	if runtime.GOOS == "linux" {
		descriptorPath = "/proc/self/fd"
	}
	descriptors, err := os.ReadDir(descriptorPath)
	if err != nil {
		t.Fatal("measurement descriptors unavailable")
	}
	t.Logf("event=measurement.resources phase=%s heap_bytes=%d heap_objects=%d system_bytes=%d goroutines=%d pool_acquired=%d pool_total=%d rss_kib=%d descriptors=%d", phase,
		memory.HeapAlloc, memory.HeapObjects, memory.Sys, runtime.NumGoroutine(), pool.AcquiredConns(), pool.TotalConns(), rss, len(descriptors))
}

func measureHistoryReads(t *testing.T, h *authHarness, token string, channel string, query string, label string) {
	t.Helper()
	latencies := make([]time.Duration, 0, 100)
	bytes := make([]int, 0, 100)
	for index := range measurementCount(110, 12) {
		started := (clock.System{}).Now()
		answer, page := h.history(t, token, channel, query)
		elapsed := time.Since(started)
		if answer.status != http.StatusOK || len(page.Items) != 100 {
			t.Fatalf("measurement request failed: status=%d rows=%d", answer.status, len(page.Items))
		}
		if index >= 10 {
			latencies = append(latencies, elapsed)
			bytes = append(bytes, len(answer.body))
			t.Logf("event=measurement.read route=%s sample=%d duration_ns=%d bytes=%d status=%d", label, index-10, elapsed.Nanoseconds(), len(answer.body), answer.status)
		}
	}
	slices.Sort(latencies)
	slices.Sort(bytes)
	t.Logf("event=measurement.read_summary route=%s samples=%d warmup=10 p95_ms=%.3f bytes_p95=%d query_count=unverified reference_certification=false", label, len(latencies), float64(latencies[(len(latencies)*95+99)/100-1])/float64(time.Millisecond), bytes[(len(bytes)*95+99)/100-1])
}

func TestSlice6HistoryMeasurement(t *testing.T) {
	h, trace := newQueryMeasurementHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	seedMeasurementHistory(t, h, channel, parties.tutor, measurementCount(100000, 1000))
	_, page := h.history(t, parties.tutorToken, channel, "?limit=100")
	for range measurementCount(900, 5) {
		if page.NextCursor == nil {
			t.Fatal("measurement history ended before the deep keyset cursor")
		}
		answer, next := h.history(t, parties.tutorToken, channel, "?limit=100&cursor="+url.QueryEscape(*page.NextCursor))
		h.mustAnswer(t, answer, http.StatusOK)
		page = next
	}
	if page.NextCursor == nil {
		t.Fatal("measurement deep cursor missing")
	}
	measurementResources(t, h, "seeded")

	measureHistoryReads(t, h, parties.tutorToken, channel, "?limit=100", "history_first")
	measureHistoryReads(t, h, parties.tutorToken, channel, "?limit=100&cursor="+url.QueryEscape(*page.NextCursor), "history_deep")

	trace.Start()
	firstAnswer, _ := h.history(t, parties.tutorToken, channel, "?limit=1")
	one := trace.Finish()
	trace.Start()
	fullAnswer, _ := h.history(t, parties.tutorToken, channel, "?limit=100")
	hundred := trace.Finish()
	h.mustAnswer(t, firstAnswer, http.StatusOK)
	h.mustAnswer(t, fullAnswer, http.StatusOK)
	measurementQueryPlans(t, h, one, "history_limit_1")
	measurementQueryPlans(t, h, hundred, "history_limit_100")
	trace.Start()
	deepAnswer, _ := h.history(t, parties.tutorToken, channel, "?limit=100&cursor="+url.QueryEscape(*page.NextCursor))
	deep := trace.Finish()
	h.mustAnswer(t, deepAnswer, http.StatusOK)
	measurementQueryPlans(t, h, deep, "history_deep")
	if len(one) != len(hundred) {
		t.Fatal("history query count grows with returned rows")
	}
	measurementResources(t, h, "complete")
	if h.messageCount(t, channel) != int64(measurementCount(100000, 1000)) {
		t.Fatal("history reads changed the dataset")
	}
}

func TestSlice6ErasureMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, sender := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID).ID
	seedMeasurementHistory(t, h, channel, sender, measurementCount(300000, 1000))
	h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
	measurementResources(t, h, "seeded")
	before := h.messageCount(t, channel)
	started := (clock.System{}).Now()
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pass := 0

	for before != 0 {
		pass++
		h.advance(t, time.Minute)
		logOffset := len(h.logs.String())
		passStarted := (clock.System{}).Now()
		err := h.worker.Tick(deadline, h.now(t))
		elapsed := time.Since(passStarted)
		measurementJobs(t, h, logOffset)
		after := h.messageCount(t, channel)
		t.Logf("event=measurement.worker_pass workload=erasure pass=%d duration_ns=%d completed=%d remaining=%d error=%t", pass, elapsed.Nanoseconds(), before-after, after, err != nil)
		measurementResources(t, h, fmt.Sprintf("pass_%d", pass))
		if err != nil {
			t.Fatal("measurement worker pass failed; retain this sample")
		}
		if after >= before {
			t.Fatal("measurement worker made no forward progress")
		}
		before = after
	}

	t.Logf("event=measurement.drain workload=erasure duration_ns=%d passes=%d rows=%d", time.Since(started).Nanoseconds(), pass, measurementCount(300000, 1000))
	if h.directChannel(t, card.ID).ErasePending {
		t.Fatal("erasure completion marker remains")
	}
	measurementResources(t, h, "complete")
}

func measurementJobs(t *testing.T, h *authHarness, since int) {
	t.Helper()
	failed := false
	for _, line := range strings.Split(h.logs.String()[since:], "\n") {
		if line == "" {
			continue
		}
		var record struct {
			Message  string `json:"msg"`
			Job      string `json:"job"`
			Result   string `json:"result"`
			Batch    int64  `json:"batch_size"`
			Duration int64  `json:"duration_ms"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal("measurement job diagnostics could not be decoded")
		}
		if record.Message == "job.run" {
			t.Logf("event=measurement.job_pass job=%s result=%s batch_size=%d duration_ms=%d", record.Job, record.Result, record.Batch, record.Duration)
			failed = failed || record.Result != "ok"
		}
	}
	if failed {
		t.Fatal("measurement worker recorded a failed job; retain this sample")
	}
}

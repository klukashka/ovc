//go:build slice6_measurement

package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"ovc/backend/internal/adapters/clock"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

const measurementStudents = `INSERT INTO users
(id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version,
consent_at, unverified_since, created_at, updated_at)
SELECT md5('measurement-student-' || seq)::uuid, 'student', 'active', 'measurement-' || seq || '@example.org',
source.email_verified_at, source.password_hash, 'Synthetic student', source.language, source.timezone, source.theme,
source.consent_version, source.consent_at, source.unverified_since, source.created_at, source.updated_at
FROM users AS source CROSS JOIN generate_series($2::integer, $3::integer) AS series(seq) WHERE source.id = $1::uuid`

const measurementRelationships = `INSERT INTO relationships
(id, tutor_id, student_id, state, display_name, email, email_set_at, colour, linked_at, created_at, updated_at)
SELECT md5('measurement-relationship-' || seq)::uuid, $1::uuid, md5('measurement-student-' || seq)::uuid,
'active', 'Synthetic student', 'measurement-' || seq || '@example.org', $4::timestamptz, 'moss', $4::timestamptz, $4::timestamptz, $4::timestamptz
FROM generate_series($2::integer, $3::integer) AS series(seq)`

const measurementChannels = `INSERT INTO channels
(id, kind, tutor_id, relationship_id, last_seq, last_activity_at, created_at, updated_at)
SELECT md5('measurement-channel-' || seq)::uuid, 'direct', $1::uuid, md5('measurement-relationship-' || seq)::uuid,
1, $4::timestamptz, $4::timestamptz, $4::timestamptz FROM generate_series($2::integer, $3::integer) AS series(seq)`

const measurementMembers = `INSERT INTO channel_members
(channel_id, user_id, last_read_seq, digested_through_seq, created_at, updated_at)
SELECT md5('measurement-channel-' || seq)::uuid, md5('measurement-student-' || seq)::uuid, 0, 0, $4::timestamptz, $4::timestamptz
FROM generate_series($2::integer, $3::integer) AS series(seq)
UNION ALL
SELECT md5('measurement-channel-' || seq)::uuid, $1::uuid, 1, 1, $4::timestamptz, $4::timestamptz
FROM generate_series($2::integer, $3::integer) AS series(seq)`

const measurementUnread = `INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT md5('measurement-message-' || seq)::uuid, md5('measurement-channel-' || seq)::uuid, 1, $1::uuid,
'Synthetic unread message', 1, $4::timestamptz, $4::timestamptz FROM generate_series($2::integer, $3::integer) AS series(seq)`

func seedMeasurementStudents(t *testing.T, h *authHarness, template domain.UserID, tutor domain.UserID, first int, count int) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), measurementStudents, template.String(), first, first+count-1); err != nil {
		t.Fatalf("measurement seed failed stage=users kind=%s", core.ErrorKind(err))
	}
	for _, stage := range []struct {
		name  string
		query string
	}{
		{"relationships", measurementRelationships}, {"channels", measurementChannels}, {"members", measurementMembers}, {"unread", measurementUnread},
	} {
		if _, err := h.database.Pool.Exec(context.Background(), stage.query, tutor.String(), first, first+count-1, authStart); err != nil {
			t.Fatalf("measurement seed failed stage=%s kind=%s", stage.name, core.ErrorKind(err))
		}
	}
}

func TestSlice6ChannelListMeasurement(t *testing.T) {
	h, trace := newQueryMeasurementHarness(t)
	parties := h.activeStudentInAGroup(t)
	seedMeasurementStudents(t, h, parties.student, parties.tutor, 1, 498)
	latencies := make([]time.Duration, 0, 100)
	measurementResources(t, h, "seeded")

	for index := range measurementCount(110, 12) {
		started := (clock.System{}).Now()
		answer, page := h.channelList(t, parties.tutorToken, "?limit=100")
		elapsed := time.Since(started)
		if answer.status != http.StatusOK || len(page.Items) != 100 {
			t.Fatalf("channel list failed: status=%d rows=%d", answer.status, len(page.Items))
		}
		if index >= 10 {
			latencies = append(latencies, elapsed)
			t.Logf("event=measurement.channel_read sample=%d duration_ns=%d bytes=%d", index-10, elapsed.Nanoseconds(), len(answer.body))
		}
	}

	trace.Start()
	firstAnswer, _ := h.channelList(t, parties.tutorToken, "?limit=1")
	one := trace.Finish()
	trace.Start()
	fullAnswer, _ := h.channelList(t, parties.tutorToken, "?limit=100")
	hundred := trace.Finish()
	h.mustAnswer(t, firstAnswer, http.StatusOK)
	h.mustAnswer(t, fullAnswer, http.StatusOK)
	measurementQueryPlans(t, h, one, "channels_limit_1")
	measurementQueryPlans(t, h, hundred, "channels_limit_100")
	if len(one) != len(hundred) {
		t.Fatal("channel list query count grows with returned rows")
	}
	slices.Sort(latencies)
	t.Logf("event=measurement.channel_summary channels=500 samples=%d warmup=10 p95_ms=%.3f query_count=recorded query_plan=recorded reference_certification=false", len(latencies), float64(latencies[(len(latencies)*95+99)/100-1])/float64(time.Millisecond))
	measurementResources(t, h, "complete")
}

func TestSlice6DigestMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	for index := range measurementCount(10, 1) {
		tutor := parties.tutor
		if index != 0 {
			tutor, _ = h.signedInAs(t, domain.RoleTutor, domain.AccountActive, byte(index+2), fmt.Sprintf("measurement-tutor-%d@example.org", index))
		}
		seedMeasurementStudents(t, h, parties.student, tutor, 1+index*500, measurementCount(500, 20))
	}
	if _, err := h.database.Pool.Exec(context.Background(), `INSERT INTO digest_pending (user_id, pending_since)
SELECT md5('measurement-student-' || seq)::uuid, $1 FROM generate_series(1, $2::integer) AS series(seq)`, authStart, measurementCount(5000, 20)); err != nil {
		t.Fatal("measurement digest seed failed")
	}
	h.advance(t, 11*time.Minute)
	measurementResources(t, h, "seeded")
	started := (clock.System{}).Now()
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	remaining := int64(measurementCount(5000, 20))
	passes := 0

	for remaining != 0 {
		passes++
		before := remaining
		logOffset := len(h.logs.String())
		passStarted := (clock.System{}).Now()
		err := h.worker.Tick(deadline, h.now(t))
		elapsed := time.Since(passStarted)
		measurementJobs(t, h, logOffset)
		remaining = h.countRows(t, "SELECT count(*) FROM digest_pending WHERE pending_since = $1::timestamptz", authStart.Format(time.RFC3339))
		t.Logf("event=measurement.worker_pass workload=digest pass=%d duration_ns=%d completed=%d remaining=%d error=%t", passes, elapsed.Nanoseconds(), before-remaining, remaining, err != nil)
		if err != nil || remaining >= before {
			t.Fatal("measurement digest failed or made no forward progress")
		}
		h.advance(t, time.Minute)
	}

	t.Logf("event=measurement.drain workload=digest recipients=%d passes=%d duration_ns=%d dispatched_mail=unverified", measurementCount(5000, 20), passes, time.Since(started).Nanoseconds())
	measurementResources(t, h, "complete")
}

func TestSlice6PresenceCleanupMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	if _, err := h.database.Pool.Exec(context.Background(), measurementStudents, parties.student.String(), 1, measurementCount(2000, 20)); err != nil {
		t.Fatal("measurement presence accounts failed")
	}
	for _, query := range []string{
		`INSERT INTO user_presence SELECT md5('measurement-student-' || seq)::uuid, $1, $1 FROM generate_series(1, $2::integer) AS series(seq)`,
		`INSERT INTO user_presence_instances SELECT md5('measurement-student-' || seq)::uuid, md5('measurement-instance-1')::uuid, $1, $1 FROM generate_series(1, $2::integer) AS series(seq)`,
		`INSERT INTO user_presence_instances SELECT md5('measurement-student-' || seq)::uuid, md5('measurement-instance-2')::uuid, $1, $1 FROM generate_series(1, $2::integer) AS series(seq)`,
	} {
		if _, err := h.database.Pool.Exec(context.Background(), query, authStart, measurementCount(2000, 20)); err != nil {
			t.Fatal("measurement expired presence seed failed")
		}
	}
	h.advance(t, time.Minute)
	logger := slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service := chat.New(chat.Dependencies{Scheduler: &manualScheduler{}, Store: h.store, Logger: logger})
	running := worker.New(logger, h.clock, outbox.New(h.store, randomPerTest(t), logger), nil,
		[]worker.Job{{Name: domain.JobExpirePresence, Every: time.Minute, Run: service.ExpirePresence}}, worker.DefaultTiming())
	remaining := int64(2 * measurementCount(2000, 20))
	passes := 0
	started := (clock.System{}).Now()
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	measurementResources(t, h, "seeded")

	for remaining != 0 {
		passes++
		before := remaining
		logOffset := len(h.logs.String())
		passStarted := (clock.System{}).Now()
		err := running.Tick(deadline, h.now(t))
		elapsed := time.Since(passStarted)
		measurementJobs(t, h, logOffset)
		remaining = h.countRows(t, "SELECT count(*) FROM user_presence_instances WHERE online_until <= $1::timestamptz", authStart.Format(time.RFC3339))
		t.Logf("event=measurement.worker_pass workload=presence pass=%d duration_ns=%d completed=%d remaining=%d error=%t", passes, elapsed.Nanoseconds(), before-remaining, remaining, err != nil)
		if err != nil || remaining >= before {
			t.Fatal("measurement presence pass failed or made no forward progress")
		}
		h.advance(t, time.Minute)
	}

	t.Logf("event=measurement.drain workload=presence accounts=%d leases=%d passes=%d duration_ns=%d", measurementCount(2000, 20), 2*measurementCount(2000, 20), passes, time.Since(started).Nanoseconds())
	measurementResources(t, h, "complete")
}

func TestSlice6StudentChannelListMeasurement(t *testing.T) {
	h, trace := newQueryMeasurementHarness(t)
	parties := h.activeStudentInAGroup(t)
	for index := range 4 {
		_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, byte(index+3), fmt.Sprintf("measurement-student-tutor-%d@example.org", index))
		h.linkedSignedIn(t, token, parties.studentToken, "Synthetic student", studentEmail)
	}
	for index := range 2 {
		h.newGroup(t, parties.tutorToken, fmt.Sprintf("Synthetic group %d", index), []string{parties.card.ID})
	}
	latencies := make([]time.Duration, 0, 100)

	for sample := range measurementCount(110, 12) {
		started := (clock.System{}).Now()
		answer, page := h.channelList(t, parties.studentToken, "?limit=100")
		elapsed := time.Since(started)
		if answer.status != http.StatusOK || len(page.Items) != 8 {
			t.Fatalf("student list measurement failed: status=%d channels=%d", answer.status, len(page.Items))
		}
		if sample >= 10 {
			latencies = append(latencies, elapsed)
			t.Logf("event=measurement.student_channel_read sample=%d duration_ns=%d bytes=%d", sample-10, elapsed.Nanoseconds(), len(answer.body))
		}
	}

	trace.Start()
	answer, _ := h.channelList(t, parties.studentToken, "?limit=100")
	queries := trace.Finish()
	h.mustAnswer(t, answer, http.StatusOK)
	measurementQueryPlans(t, h, queries, "student_five_tutors_three_groups")
	slices.Sort(latencies)
	t.Logf("event=measurement.student_channel_summary tutors=5 groups=3 samples=%d p95_ms=%.3f", len(latencies), float64(latencies[(len(latencies)*95+99)/100-1])/float64(time.Millisecond))
	measurementResources(t, h, "complete")
}

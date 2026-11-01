//go:build slice6_measurement

package api_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

func measurementDailyRows(t *testing.T, h *authHarness, before time.Time) map[string]int64 {
	t.Helper()
	rows, err := h.database.Pool.Query(t.Context(), "SELECT student_id::text, count(*) FROM daily_chat_upload_bytes WHERE upload_day < $1 GROUP BY student_id", before)
	if err != nil {
		t.Fatal("measurement daily rows unavailable")
	}
	result := make(map[string]int64)
	for rows.Next() {
		var user string
		var count int64
		if err := rows.Scan(&user, &count); err != nil {
			rows.Close()
			t.Fatal("measurement daily row invalid")
		}
		result[user] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal("measurement daily rows stream failed")
	}
	return result
}

func TestSlice6DailyUploadCleanupMeasurement(t *testing.T) {
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	students, days := measurementCount(500, 4), measurementCount(20, 3)
	seedMeasurementStudents(t, h, parties.student, parties.tutor, 1, students)
	now := h.now(t)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO daily_chat_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
SELECT md5('measurement-student-' || student)::uuid, $1::uuid, $2::timestamptz - age * interval '1 day', 1024
FROM generate_series(1,$3::integer) AS students(student) CROSS JOIN generate_series(0,$4::integer) AS days(age)`, parties.tutor.String(), day, students, days); err != nil {
		t.Fatal("measurement historical daily seed failed")
	}
	held, err := h.database.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal("measurement daily lock transaction failed")
	}
	released := false
	defer func() {
		if !released {
			if err := held.Rollback(context.Background()); err != nil {
				t.Error("measurement daily lock cleanup failed")
			}
		}
	}()
	var blocked string
	if err := held.QueryRow(t.Context(), "SELECT id::text FROM users WHERE id=md5('measurement-student-1')::uuid FOR UPDATE").Scan(&blocked); err != nil {
		t.Fatal("measurement daily student lock failed")
	}
	logger := slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	running := worker.New(logger, h.clock, outbox.New(h.store, randomPerTest(t), logger), nil,
		[]worker.Job{{Name: domain.JobDailyUploadsCleanup, Every: time.Minute, Run: h.files.CleanupDailyUploads}}, worker.DefaultTiming())
	before := measurementDailyRows(t, h, day)
	started := (clock.System{}).Now()
	deadline, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	passes := 0
	measurementResources(t, h, "daily_seeded")

	for len(before) != 0 {
		passes++
		h.advance(t, time.Minute)
		offset := len(h.logs.String())
		passStarted := (clock.System{}).Now()
		err := running.Tick(deadline, h.now(t))
		elapsed := time.Since(passStarted)
		measurementJobs(t, h, offset)
		after := measurementDailyRows(t, h, day)
		var deleted int64
		touched := 0
		for user, count := range before {
			if count != after[user] {
				deleted += count - after[user]
				touched++
			}
		}
		t.Logf("event=measurement.worker_pass workload=daily_upload_cleanup pass=%d duration_ns=%d completed=%d students=%d student_limit=100 row_limit=1000 batch_limit=100 job_budget_seconds=20 blocked_student=%t error=%t", passes, elapsed.Nanoseconds(), deleted, touched, !released, err != nil)
		if err != nil || deleted <= 0 || deleted > 1000 || touched > 100 {
			t.Fatal("measurement daily cleanup failed progress or batch limits")
		}
		if !released && after[blocked] != int64(days) {
			t.Fatal("measurement daily cleanup changed the locked student")
		}
		if !released && len(after) == 1 {
			if err := held.Commit(t.Context()); err != nil {
				t.Fatal("measurement daily lock release failed")
			}
			released = true
		}
		before = after
	}

	if !released {
		t.Fatal("measurement daily lock was not released")
	}
	if count := h.countRows(t, "SELECT count(*) FROM daily_chat_upload_bytes WHERE upload_day=$1::timestamptz", day.Format(time.RFC3339)); count != int64(students) {
		t.Fatal("measurement daily cleanup removed current-day accounting")
	}
	t.Logf("event=measurement.drain workload=daily_upload_cleanup students=%d past_rows=%d retained_current_rows=%d passes=%d duration_ns=%d", students, students*days, students, passes, time.Since(started).Nanoseconds())
	measurementResources(t, h, "daily_quiescent")
}

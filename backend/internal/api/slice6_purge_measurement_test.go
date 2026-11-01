//go:build slice6_measurement

package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"ovc/backend/internal/adapters/clock"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

func seedMeasurementFiles(t *testing.T, h *authHarness, channel string, count int) []string {
	t.Helper()
	queries := []string{
		`INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, quota_tutor_id, storage_key,
filename, media_type, size_bytes, sha256, state, created_at, updated_at)
SELECT md5('measurement-file-' || seq)::uuid, channels.tutor_id, 'chat', 'channel', channels.id, channels.tutor_id,
'chat/measurement-' || channels.id || '-' || md5(seq::text), 'synthetic.txt', 'text/plain', 1, sha256('x'), 'bound',
channels.created_at - interval '100 days', channels.created_at
FROM channels CROSS JOIN generate_series(1,$2::integer) AS series(seq) WHERE channels.id=$1::uuid`,
		`INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at)
SELECT md5('measurement-file-' || seq)::uuid, 'message', md5($1::text || ':' || seq)::uuid, now()
FROM generate_series(1,$2::integer) AS series(seq)`,
		`INSERT INTO quota_charges (tutor_id, attachment_id, size_bytes, created_at)
SELECT channels.tutor_id, md5('measurement-file-' || seq)::uuid, 1, channels.created_at
FROM channels CROSS JOIN generate_series(1,$2::integer) AS series(seq) WHERE channels.id=$1::uuid`,
		`UPDATE tutor_profiles SET file_usage_bytes = file_usage_bytes + $2::bigint
WHERE user_id = (SELECT tutor_id FROM channels WHERE id=$1::uuid)`,
	}
	for _, query := range queries {
		if _, err := h.database.Pool.Exec(t.Context(), query, channel, count); err != nil {
			t.Fatal("measurement historical file seed failed")
		}
	}
	rows, err := h.database.Pool.Query(t.Context(), "SELECT storage_key FROM attachments WHERE scope_id=$1::uuid ORDER BY id", channel)
	if err != nil {
		t.Fatal("measurement object seed could not enumerate keys")
	}
	keys := make([]string, 0, count)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			t.Fatal("measurement object seed key failed")
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(keys) != count {
		t.Fatal("measurement object seed count differs")
	}
	for _, key := range keys {
		if err := h.objects.Put(t.Context(), key, []byte("x"), "text/plain"); err != nil {
			t.Fatal("measurement object seed failed")
		}
	}
	return keys
}

func seedMeasurementReminder(t *testing.T, h *authHarness) {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 50, "measurement-reminder@example.org")
	card := h.linkedCard(t, tutor, 51, domain.RelationshipActive)
	start := authStart.Add(domain.DeletedAccountRetention + 2*time.Hour).In(time.FixedZone("Europe/Moscow", 3*60*60))
	h.lessonAt(t, token, card, start.Format("2006-01-02"), start.Format("15:04"))
}

func measurePurgeDrain(t *testing.T, h *authHarness, user domain.UserID, workload string) {
	t.Helper()
	measurementResources(t, h, "seeded")
	logOffset := len(h.logs.String())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := (clock.System{}).Now()
	running := core.Run(ctx, slog.New(slog.DiscardHandler), "measurement-worker", func() error { return h.worker.Run(ctx) })
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	paused := h.countLogs(`"msg":"account.purge_paused"`)
	reminderObserved := false
	complete := false
	for !complete {
		select {
		case <-deadline.C:
			cancel()
			if err := <-running; err != nil {
				t.Error("measurement worker shutdown after deadline failed")
			}
			t.Fatal("measurement purge drain deadline exceeded")
		case <-poll.C:
			users := h.countRows(t, "SELECT count(*) FROM users WHERE id=$1::uuid", user.String())
			storagePending := h.countRows(t, "SELECT count(*) FROM outbox WHERE kind=$1 AND status <> 'done'", string(domain.KindStorageDelete))
			locks := h.countRows(t, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND cardinality(pg_blocking_pids(pid)) > 0", h.database.Name)
			t.Logf("event=measurement.purge_progress workload=%s elapsed_ns=%d users_remaining=%d storage_pending=%d lock_waiters=%d", workload, time.Since(started).Nanoseconds(), users, storagePending, locks)
			if !reminderObserved && h.outboxOf(t, domain.EventLessonReminder1h, "done:sent") >= 2 {
				reminderObserved = true
				t.Logf("event=measurement.concurrent_reminder workload=%s elapsed_ns=%d", workload, time.Since(started).Nanoseconds())
			}
			currentPaused := h.countLogs(`"msg":"account.purge_paused"`)
			if currentPaused > paused {
				paused = currentPaused
				h.advance(t, time.Hour)
			}
			complete = users == 0 && storagePending == 0 && reminderObserved
		case err := <-running:
			if err != nil {
				t.Fatal("measurement worker stopped with an error")
			}
			t.Fatal("measurement worker ended before cleanup")
		}
	}
	cancel()
	if err := <-running; err != nil {
		t.Fatal("measurement worker did not shut down cleanly")
	}
	measurementJobs(t, h, logOffset)
	t.Logf("event=measurement.drain workload=%s duration_ns=%d paused_passes=%d reminder_delivered=%t", workload, time.Since(started).Nanoseconds(), paused, reminderObserved)
	measurementResources(t, h, "quiescent_worker_stopped")
}

func TestSlice6TutorPurgeMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	seedMeasurementHistory(t, h, channel, parties.tutor, measurementCount(100000, 1000))
	keys := seedMeasurementFiles(t, h, channel, measurementCount(5000, 20))
	seedMeasurementReminder(t, h)
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour+time.Minute)

	measurePurgeDrain(t, h, parties.tutor, "tutor_100000_messages_5000_files")

	if h.countRows(t, "SELECT count(*) FROM quota_charges WHERE tutor_id=$1::uuid", parties.tutor.String()) != 0 {
		t.Fatal("measurement tutor purge retained charges")
	}
	for _, key := range keys {
		if _, found := h.storedObject(t, key); found {
			t.Fatal("measurement tutor purge retained an object")
		}
	}
}

func TestSlice6StudentPurgeMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	channels := []string{h.directChannel(t, parties.card.ID).ID}
	for index := range 4 {
		_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, byte(index+3), fmt.Sprintf("measurement-tutor-%d@example.org", index))
		card := h.linkedSignedIn(t, token, parties.studentToken, "Synthetic student", studentEmail)
		channels = append(channels, h.directChannel(t, card.ID).ID)
	}
	for _, channel := range channels {
		seedMeasurementHistory(t, h, channel, parties.student, measurementCount(10000, 200))
	}
	seedMeasurementReminder(t, h)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour+time.Minute)

	measurePurgeDrain(t, h, parties.student, "student_50000_messages_five_tutors")

	for _, channel := range channels {
		if h.countRows(t, "SELECT count(*) FROM messages WHERE channel_id=$1::uuid AND (body IS NOT NULL OR sender_id IS NOT NULL)", channel) != 0 {
			t.Fatal("measurement student purge retained message content")
		}
	}
}

package startup_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

func seedAdminMeeting(t *testing.T, f *adminFixture, index byte) domain.ProviderMeetingID {
	t.Helper()
	id := domain.ProviderMeetingID{index}
	_, err := f.database.Pool.Exec(t.Context(), `INSERT INTO meeting_recovery
 (provider_meeting_id, provider, origin, create_state, cleanup_requested, created_at, updated_at)
 VALUES ($1, 'fake', 'attempt', 'prepared', true, $2, $2)`, id, adminStart)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(t.Context(), "UPDATE meeting_recovery_capacity SET unsettled = unsettled + 1"); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMeetingRecoveryAdminRevisionAndRefusals(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	id := seedAdminMeeting(t, f, 1)
	missing := domain.ProviderMeetingID{2}

	first := f.admin(t, "meetings", "retry-cleanup", id.String(), "--revision", "1")
	replay := f.admin(t, "meetings", "retry-cleanup", id.String(), "--revision", "1")
	pending := f.admin(t, "meetings", "retry-cleanup", id.String(), "--revision", "2")
	absent := f.admin(t, "meetings", "retry-cleanup", missing.String(), "--revision", "1")
	invalid := f.admin(t, "meetings", "retry-cleanup", id.String())
	inspection := f.admin(t, "meetings", "inspect", id.String())

	if first.exitCode != 0 || !strings.Contains(first.stdout, "\toperator\tpending") || replay.exitCode != 1 || pending.exitCode != 1 || absent.exitCode != 1 || invalid.exitCode != 2 {
		t.Fatalf("retry outcomes: first=%+v replay=%+v pending=%+v absent=%+v invalid=%+v", first, replay, pending, absent, invalid)
	}
	if inspection.exitCode != 0 || !strings.Contains(inspection.stdout, "revision=2") || !strings.Contains(inspection.stdout, "generation=unknown") || !strings.Contains(inspection.stdout, "orphaned=true") {
		t.Fatalf("inspection: %+v", inspection)
	}
	var receipts, queued int
	if err := f.database.Pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM meeting_recovery_cycles), (SELECT count(*) FROM outbox WHERE kind='meeting_cleanup')").Scan(&receipts, &queued); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || queued != 1 {
		t.Fatalf("receipt/outbox counts: %d/%d", receipts, queued)
	}
	var queuedID string
	if err := f.database.Pool.QueryRow(t.Context(), "SELECT replace(id::text, '-', '') FROM outbox WHERE kind='meeting_cleanup'").Scan(&queuedID); err != nil {
		t.Fatal(err)
	}
	if !hasRecord(first.stderr, `"msg":"meeting.recovery.retry.queued"`, `"outbox_id":"`+queuedID+`"`) || strings.Contains(replay.stderr+pending.stderr, "meeting.recovery.retry.queued") {
		t.Fatalf("queued retry audit: first=%s replay=%s pending=%s", first.stderr, replay.stderr, pending.stderr)
	}
	for _, result := range []outcome{first, replay, pending, absent, invalid, inspection} {
		if strings.Contains(result.stderr, id.String()) || strings.Contains(result.stderr, missing.String()) {
			t.Fatal("provider identifier entered operator diagnostics")
		}
		assertMeetingAdminAudit(t, result)
	}
}

func TestMeetingRecoveryAdminConcurrentRetry(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	id := seedAdminMeeting(t, f, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var outputs [2]bytes.Buffer
	var diagnostics [2]bytes.Buffer
	var commands [2]*exec.Cmd
	for index := range commands {
		command := exec.CommandContext(ctx, filepath.Join(binaries, "ovc-admin"), "meetings", "retry-cleanup", id.String(), "--revision", "1")
		command.Env = withPublicURL([]string{"OVC_DATABASE_URL=" + f.database.URL.Reveal()})
		command.Stdout, command.Stderr = &outputs[index], &diagnostics[index]
		commands[index] = command
		t.Cleanup(func() {
			cancel()
			if command.Process != nil && command.ProcessState == nil {
				if err := command.Wait(); err != nil {
					var exitErr *exec.ExitError
					if !errors.As(err, &exitErr) {
						t.Error("operator cleanup wait failed")
					}
				}
			}
		})
	}

	for _, command := range commands {
		if err := command.Start(); err != nil {
			t.Fatal("operator process start failed")
		}
	}
	var results [2]outcome
	for index, command := range commands {
		exit := 0
		if err := command.Wait(); err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatal("operator process wait failed")
			}
			exit = exitErr.ExitCode()
		}
		results[index] = outcome{exitCode: exit, stdout: outputs[index].String(), stderr: diagnostics[index].String()}
	}
	first, second := results[0], results[1]

	if first.exitCode+second.exitCode != 1 || (first.exitCode != 0 && second.exitCode != 0) {
		t.Fatalf("concurrent retries: %+v %+v", first, second)
	}
	var receipts, queued int
	if err := f.database.Pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM meeting_recovery_cycles), (SELECT count(*) FROM outbox WHERE kind='meeting_cleanup')").Scan(&receipts, &queued); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || queued != 1 {
		t.Fatalf("concurrent receipt/outbox counts: %d/%d", receipts, queued)
	}
}

func TestMeetingRecoveryAdminPagination(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	var id domain.ProviderMeetingID
	for index := byte(1); index <= 101; index++ {
		id = seedAdminMeeting(t, f, index)
	}
	for index := byte(1); index <= 101; index++ {
		_, err := f.database.Pool.Exec(t.Context(), `INSERT INTO meeting_recovery_cycles (outbox_id, provider_meeting_id, cycle_number, mode, result, created_at, completed_at) VALUES ($1,$2,$3,'operator','exhausted',$4,$4)`, domain.OutboxID{index}, id, int64(index), adminStart)
		if err != nil {
			t.Fatal(err)
		}
	}
	cursor := domain.ProviderMeetingID{100}

	first := f.admin(t, "meetings", "status")
	last := f.admin(t, "meetings", "status", "--after", cursor.String())
	receipts := f.admin(t, "meetings", "inspect", id.String())
	tail := f.admin(t, "meetings", "inspect", id.String(), "--after", "100")
	invalid := f.admin(t, "meetings", "inspect", id.String(), "--after", "-1")

	if first.exitCode != 0 || strings.Count(first.stdout, "recovery\t") != 100 || !strings.Contains(first.stdout, "next\t"+cursor.String()) || !strings.Contains(first.stdout, "unsettled=101") {
		t.Fatalf("status first: %+v", first)
	}
	if last.exitCode != 0 || strings.Count(last.stdout, "recovery\t") != 1 || !strings.Contains(last.stdout, id.String()) || strings.Contains(last.stdout, "next\t") {
		t.Fatalf("status last: %+v", last)
	}
	if receipts.exitCode != 0 || strings.Count(receipts.stdout, "cycle\t") != 100 || !strings.Contains(receipts.stdout, "next\t100") || tail.exitCode != 0 || strings.Count(tail.stdout, "cycle\t") != 1 || !strings.Contains(tail.stdout, "cycle\t101\t") || strings.Contains(tail.stdout, "next\t") || invalid.exitCode != 2 {
		t.Fatalf("receipt pagination: %+v %+v %+v", receipts, tail, invalid)
	}
}

func TestMeetingRecoveryAdminSettledAndSubsequentCycle(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	settled := seedAdminMeeting(t, f, 1)
	retryable := seedAdminMeeting(t, f, 2)
	if _, err := f.database.Pool.Exec(t.Context(), "UPDATE meeting_recovery SET settled_at=$2, settlement='revoked' WHERE provider_meeting_id=$1", settled, adminStart); err != nil {
		t.Fatal(err)
	}
	first := f.admin(t, "meetings", "retry-cleanup", retryable.String(), "--revision", "1")
	if first.exitCode != 0 {
		t.Fatalf("fixture cycle: %+v", first)
	}
	if _, err := f.database.Pool.Exec(t.Context(), "UPDATE meeting_recovery SET cycle_result='exhausted', revision=revision+1 WHERE provider_meeting_id=$1", retryable); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(t.Context(), "UPDATE meeting_recovery_cycles SET result='exhausted', completed_at=created_at WHERE provider_meeting_id=$1", retryable); err != nil {
		t.Fatal(err)
	}

	refused := f.admin(t, "meetings", "retry-cleanup", settled.String(), "--revision", "1")
	stale := f.admin(t, "meetings", "retry-cleanup", retryable.String(), "--revision", "1")
	settledView := f.admin(t, "meetings", "inspect", settled.String())
	spentView := f.admin(t, "meetings", "inspect", retryable.String())
	second := f.admin(t, "meetings", "retry-cleanup", retryable.String(), "--revision", "3")
	inspection := f.admin(t, "meetings", "inspect", retryable.String())

	if refused.exitCode != 1 || stale.exitCode != 1 || second.exitCode != 0 || second.stdout == first.stdout || !strings.HasPrefix(second.stdout, "cycle\t2\t") || strings.Count(inspection.stdout, "cycle\t") != 2 {
		t.Fatalf("bounded cycles: refused=%+v second=%+v inspect=%+v", refused, second, inspection)
	}
	for _, field := range []string{"settled=true", "settlement=revoked", "dispatched_at=none", "reconcile_cycle_used=false", "operator_action=false"} {
		if !strings.Contains(settledView.stdout, field) {
			t.Fatalf("settled inspection lacks %s: %+v", field, settledView)
		}
	}
	for _, field := range []string{"settled=false", "settlement=none", "result=exhausted", "operator_action=true"} {
		if !strings.Contains(spentView.stdout, field) {
			t.Fatalf("spent inspection lacks %s: %+v", field, spentView)
		}
	}
}

func hasRecord(diagnostics string, fragments ...string) bool {
	for _, line := range strings.Split(diagnostics, "\n") {
		matched := line != ""
		for _, fragment := range fragments {
			matched = matched && strings.Contains(line, fragment)
		}
		if matched {
			return true
		}
	}
	return false
}

func assertMeetingAdminAudit(t *testing.T, result outcome) {
	t.Helper()
	if !strings.Contains(result.stderr, "admin.command") || !strings.Contains(result.stderr, "operator_uid") || !strings.Contains(result.stderr, "outcome") {
		t.Fatalf("missing operator audit: %s", result.stderr)
	}
	for _, forbidden := range []string{"database_url", "namespace", "checksum", "join_link", "payload", "attempt_id"} {
		if strings.Contains(result.stdout+result.stderr, forbidden) {
			t.Fatal("unsafe operator output")
		}
	}
}

func TestMeetingRecoveryAdminStartupFailureAudit(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	pending := strings.Replace(f.database.URL.Reveal(), f.database.Name, "ovc_missing_"+core.NewRandomID()[:8]+"_test", 1)
	id := domain.ProviderMeetingID{21}
	commands := [][]string{{"meetings", "status"}, {"meetings", "inspect", id.String()}, {"meetings", "retry-cleanup", id.String(), "--revision", "1"}}
	results := make([]outcome, 0, len(commands))

	for _, command := range commands {
		results = append(results, runBinary(t, "ovc-admin", []string{"OVC_DATABASE_URL=" + pending}, command...))
	}

	for index, result := range results {
		expected := `"subcommand":"meetings ` + commands[index][1] + `"`
		if result.exitCode != 1 || !strings.Contains(result.stderr, expected) || !strings.Contains(result.stderr, `"outcome":"failed"`) || strings.Contains(result.stderr, id.String()) || strings.Contains(result.stderr, pending) {
			t.Fatalf("meeting startup audit refused expected command %d", index)
		}
	}
}

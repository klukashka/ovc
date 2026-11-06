package startup_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestMeetingRecoveryRuntime(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := clock.NewShared(f.database.Pool)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := source.Set(ctx, now); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	provider, err := meeting.NewSharedFake(ctx, f.database.Pool, source, core.CryptoRandom(), logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	returned := seedAdminMeeting(t, f, 1)
	uncertain := seedAdminMeeting(t, f, 2)
	for _, id := range []domain.ProviderMeetingID{returned, uncertain} {
		generation, err := provider.CreateMeeting(ctx, domain.Meeting{ID: id, Name: "Private recovery room sentinel"})
		if err != nil {
			t.Fatal(err)
		}
		state := domain.MeetingCreateUncertain
		var acknowledged *domain.ProviderCreateTime
		if id == returned {
			state = domain.MeetingCreateReturned
			acknowledged = &generation
		}
		if _, err := f.database.Pool.Exec(ctx, "UPDATE meeting_recovery SET create_state=$2, attempt_id=$1, create_time=$3 WHERE provider_meeting_id=$1", id, state, acknowledged); err != nil {
			t.Fatal(err)
		}
	}
	reconstructed, err := meeting.NewSharedFake(ctx, f.database.Pool, source, core.CryptoRandom(), logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	originalInfo, err := provider.InspectMeeting(ctx, returned)
	if err != nil {
		t.Fatal(err)
	}
	reconstructedInfo, err := reconstructed.InspectMeeting(ctx, returned)
	if err != nil || reconstructedInfo != originalInfo {
		t.Fatal("provider reconstruction lost exact state")
	}

	address := freeAddress(t)
	base := "http://" + address
	server, output := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + f.database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test", "OVC_LOG_LEVEL=debug"})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	headers := map[string]string{"Content-Type": "application/json"}

	first := call(t, ctx, http.MethodPost, base+"/__test__/tick", `{}`, headers)
	advance := call(t, ctx, http.MethodPost, base+"/__test__/clock", `{"set":"2030-01-01T00:01:00Z"}`, headers)
	second := call(t, ctx, http.MethodPost, base+"/__test__/tick", `{}`, headers)
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	if first.status != http.StatusOK || advance.status != http.StatusOK || second.status != http.StatusOK || signalErr != nil || waitErr != nil {
		t.Fatalf("runtime responses=%d/%d/%d signal=%v exit=%v", first.status, advance.status, second.status, signalErr, waitErr)
	}
	repo := repositories.NewStore(f.database.Pool).Repositories().MeetingRecovery
	for _, id := range []domain.ProviderMeetingID{returned, uncertain} {
		row, err := repo.ByProviderID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		info, err := provider.InspectMeeting(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want := domain.MeetingRecoveryCycleUnresolved
		if id == returned {
			want = domain.MeetingRecoveryCycleSettled
		}
		if row.CycleResult != want || row.CycleNumber != 1 || (row.SettledAt != nil) != (id == returned) || info.Status != domain.MeetingNotFound {
			t.Fatalf("runtime recovery result=%s cycle=%d status=%s", row.CycleResult, row.CycleNumber, info.Status)
		}
	}
	var correct bool
	if err := f.database.Pool.QueryRow(ctx, `SELECT (SELECT unsettled=1 FROM meeting_recovery_capacity)
 AND (SELECT count(*)=2 FROM outbox WHERE kind='meeting_cleanup' AND status='done')
 AND (SELECT count(*)=2 FROM meeting_recovery_cycles)
 AND EXISTS (SELECT 1 FROM job_runs WHERE job='meeting.recovery' AND last_result='ok')`).Scan(&correct); err != nil {
		t.Fatal(err)
	}
	if !correct || !strings.Contains(output.String(), `"lane":"provider_call"`) || !strings.Contains(output.String(), `"slots":4`) || !strings.Contains(output.String(), "meeting.recovery") {
		t.Fatal("production runtime did not persist exact bounded recovery state and diagnostics")
	}
	for _, private := range []string{"Private recovery room sentinel", "provider_meeting_id", "create_time", "attempt_id", "payload"} {
		if strings.Contains(output.String(), private) {
			t.Fatal("unsafe runtime diagnostic")
		}
	}
}

func TestMeetingRecoveryRuntimeRefusesDifferentProviderLimits(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := clock.NewShared(f.database.Pool)
	if _, err := source.Set(ctx, adminStart); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	limits := meeting.DefaultFakeLimits()
	limits.Meetings--
	provider, err := meeting.NewSharedFake(ctx, f.database.Pool, source, core.CryptoRandom(), logger, limits)
	if err != nil {
		t.Fatal(err)
	}
	id := domain.ProviderMeetingID{9}
	generation, err := provider.CreateMeeting(ctx, domain.Meeting{ID: id, Name: "Private mismatch room sentinel"})
	if err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, "worker", []string{"OVC_DATABASE_URL=" + f.database.URL.Reveal()})
	info, infoErr := provider.InspectMeeting(ctx, id)

	if result.exitCode != 1 || infoErr != nil || info.Status != domain.MeetingCreated || info.CreateTime != generation {
		t.Fatalf("mismatched runtime accepted or changed provider: exit=%d info=%v", result.exitCode, info)
	}
	if !strings.Contains(result.stdout, "process.failed") || strings.Contains(result.stdout+result.stderr, "Private mismatch room sentinel") || strings.Contains(result.stdout+result.stderr, id.String()) {
		t.Fatal("mismatch refusal diagnostic missing or unsafe")
	}
}

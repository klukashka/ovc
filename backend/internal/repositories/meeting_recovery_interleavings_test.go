package repositories_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

type recoveryLogBarrier struct {
	ctx             context.Context
	inspected       chan struct{}
	continueEnd     chan struct{}
	refused         chan struct{}
	continueInspect chan struct{}
	output          bytes.Buffer
	inspects        int
}

func (b *recoveryLogBarrier) Write(data []byte) (int, error) {
	var event struct {
		Message   string `json:"msg"`
		Operation string `json:"operation"`
		Kind      string `json:"kind"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return 0, err
	}
	count, err := b.output.Write(data)
	if err != nil {
		return count, err
	}
	var release <-chan struct{}
	if event.Operation == "inspect" {
		b.inspects++
		if b.inspects == 1 && event.Message == "meeting.provider.completed" {
			close(b.inspected)
			release = b.continueEnd
		}
	}
	if event.Operation == "end" && event.Message == "meeting.provider.failed" && event.Kind == "not_found" {
		close(b.refused)
		release = b.continueInspect
	}
	if release != nil {
		select {
		case <-release:
		case <-b.ctx.Done():
		}
	}
	return count, nil
}

func recoveryInterleavingService(t *testing.T, ctx context.Context, f recoveryWorkerFixture) (*meetings.Service, *recoveryLogBarrier) {
	t.Helper()
	barrier := &recoveryLogBarrier{ctx: ctx, inspected: make(chan struct{}), continueEnd: make(chan struct{}), refused: make(chan struct{}), continueInspect: make(chan struct{})}
	logger := slog.New(slog.NewJSONHandler(barrier, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider, err := meeting.NewSharedFake(ctx, f.pool, clock.NewShared(f.pool), bytes.NewReader(make([]byte, 4096)), logger, meeting.FakeLimits{Meetings: 8, Links: 8, Participants: 8, Generations: 32})
	meetingRequire(t, err)
	return meetings.New(f.store, provider, domain.MeetingProviderFake, f.logger), barrier
}

func recoveryReplaceDuringInspection(t *testing.T, ctx context.Context, f recoveryWorkerFixture, barrier *recoveryLogBarrier) domain.ProviderCreateTime {
	t.Helper()
	meetingAwait(t, ctx, barrier.inspected)
	meetingRequire(t, f.provider.EndMeeting(ctx, domain.Meeting{ID: f.identity.ProviderMeetingID, CreateTime: f.generation}))
	replacement, err := f.provider.CreateMeeting(ctx, domain.Meeting{ID: f.identity.ProviderMeetingID, Name: "Replacement sentinel"})
	meetingRequire(t, err)
	close(barrier.continueEnd)
	meetingAwait(t, ctx, barrier.refused)
	return replacement
}

func recoveryAssertReinspection(t *testing.T, ctx context.Context, f recoveryWorkerFixture, barrier *recoveryLogBarrier) {
	t.Helper()
	if barrier.inspects != 2 {
		t.Fatal("cleanup did not reinspect exactly once")
	}
	waitScannerConnectionsReleased(t, ctx, f.meetingFixture)
	for _, private := range []string{"Private lesson sentinel", "Replacement sentinel", "provider_meeting_id", "create_time", "attempt_id", "payload"} {
		if strings.Contains(barrier.output.String(), private) || strings.Contains(f.logs.String(), private) {
			t.Fatalf("unsafe interleaving diagnostic field %s", private)
		}
	}
}

func TestMeetingRecoveryWorkerReinspection(t *testing.T) {
	for _, scenario := range []string{"replacement", "absence", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			service, barrier := recoveryInterleavingService(t, ctx, f)
			w := worker.New(f.logger, clock.NewShared(f.pool), f.queue, []worker.Lane{{Name: domain.LaneProviderCall, Concurrency: 4, PerformMeetingRecovery: service.Cleanup}}, nil, worker.DefaultTiming())

			running := recoveryRun(t, ctx, "meeting-cleanup-reinspection", func() error { return w.Tick(ctx, instant) })
			replacement := recoveryReplaceDuringInspection(t, ctx, f, barrier)
			switch scenario {
			case "absence":
				meetingRequire(t, f.provider.EndMeeting(ctx, domain.Meeting{ID: f.identity.ProviderMeetingID, CreateTime: replacement}))
			case "unavailable":
				meetingRequire(t, f.provider.SimulateUnavailable(ctx, true))
			}
			close(barrier.continueInspect)
			runErr := meetingAwait(t, ctx, running)
			meetingRequire(t, f.provider.SimulateUnavailable(ctx, false))
			info, infoErr := f.provider.InspectMeeting(ctx, f.identity.ProviderMeetingID)

			meetingRequire(t, runErr, infoErr)
			recoveryAssertReinspection(t, ctx, f, barrier)
			switch scenario {
			case "replacement":
				f.assertResult(t, domain.MeetingRecoveryCycleConflict, domain.OutboxDone, 1)
			case "absence":
				f.assertResult(t, domain.MeetingRecoveryCycleSettled, domain.OutboxDone, 0)
			case "unavailable":
				f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxPending, 1)
				if !strings.Contains(f.logs.String(), "outbox.row.retried") || strings.Contains(f.logs.String(), "outbox.row.done") {
					t.Fatal("unavailable reinspection did not retain retryable evidence")
				}
			}
			if scenario == "absence" {
				if info.Status != domain.MeetingNotFound {
					t.Fatal("ended replacement remained present")
				}
			} else if replacement == f.generation || info.CreateTime != replacement || info.Status != domain.MeetingCreated {
				t.Fatal("stale cleanup destroyed the replacement generation")
			}
		})
	}
}

func TestMeetingRecoveryReinspectionCancellation(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()
	service, barrier := recoveryInterleavingService(t, ctx, f)

	running := recoveryRun(t, ctx, "meeting-cleanup-reinspection-cancelled", func() error {
		_, err := service.Cleanup(waiting, outbox.Row{ID: domain.OutboxID{30}, Kind: domain.KindMeetingCleanup}, instant)
		return err
	})
	replacement := recoveryReplaceDuringInspection(t, ctx, f, barrier)
	cancelWait()
	close(barrier.continueInspect)
	cleanupErr := meetingAwait(t, ctx, running)
	info, infoErr := f.provider.InspectMeeting(ctx, f.identity.ProviderMeetingID)

	meetingRequire(t, infoErr)
	recoveryAssertReinspection(t, ctx, f, barrier)
	f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxPending, 1)
	if !errors.Is(cleanupErr, context.Canceled) || info.CreateTime != replacement || info.Status != domain.MeetingCreated || !strings.Contains(f.logs.String(), "meeting.cleanup.failed") || strings.Contains(f.logs.String(), "meeting.cleanup.evaluated") {
		t.Fatal("cancelled reinspection lost evidence, destroyed the replacement or reported success")
	}
}

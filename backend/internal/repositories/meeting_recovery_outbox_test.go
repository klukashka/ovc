package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
)

type recoveryEffectFixture struct {
	meetingFixture
	identity domain.MeetingIdentity
	claim    outbox.Claim
	queue    *outbox.Service
	logs     *bytes.Buffer
}

func newRecoveryEffect(t *testing.T, state domain.MeetingCreateState) recoveryEffectFixture {
	t.Helper()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	if state == domain.MeetingCreateReturned {
		f.acknowledge(t, identity, 100)
	} else if state != domain.MeetingCreatePrepared {
		if f.begin(t, identity, domain.MeetingCreateAttemptID{20}, 1) != domain.MeetingMutationApplied {
			t.Fatal("begin refused")
		}
	}
	recoveryClose(t, f, identity)
	if recoveryQueue(t, f, identity.ProviderMeetingID, domain.OutboxID{30}, domain.MeetingRecoveryAutomatic) != domain.MeetingMutationApplied {
		t.Fatal("queue refused")
	}
	logs := &bytes.Buffer{}
	queue := outbox.New(f.store, bytes.NewReader(make([]byte, 4096)), slog.New(slog.NewJSONHandler(logs, nil)))
	claims, err := queue.Claim(t.Context(), domain.LaneProviderCall, 1, instant)
	meetingRequire(t, err)
	if len(claims) != 1 {
		t.Fatal("expected one cleanup claim")
	}
	return recoveryEffectFixture{meetingFixture: f, identity: identity, claim: claims[0], queue: queue, logs: logs}
}

func (f recoveryEffectFixture) completion(t *testing.T, result domain.MeetingRecoveryCycleResult) repositories.MeetingRecoveryEffectCompletion {
	t.Helper()
	row := f.recovery(t, f.identity.ProviderMeetingID)
	return repositories.MeetingRecoveryEffectCompletion{OutboxID: f.claim.Row.ID, Lease: f.claim.Lease, Revision: row.Revision, ObservedCreateTime: row.CreateTime, Result: result, Now: instant}
}

func (f recoveryEffectFixture) assertResult(t *testing.T, result domain.MeetingRecoveryCycleResult, status domain.OutboxStatus, capacity int64) {
	t.Helper()
	row := f.recovery(t, f.identity.ProviderMeetingID)
	cycle, cycleErr := f.repos.MeetingRecovery.Cycle(t.Context(), f.claim.Row.ID)
	effect, effectErr := f.repos.Outbox.ByID(t.Context(), f.claim.Row.ID)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())
	meetingRequire(t, cycleErr, effectErr, countErr)
	if row.CycleResult != result || cycle.Result != result || effect.Status != status || count != capacity || (row.SettledAt != nil) != (result == domain.MeetingRecoveryCycleSettled) {
		t.Fatalf("inconsistent terminal result: ledger=%s receipt=%s outbox=%s capacity=%d", row.CycleResult, cycle.Result, effect.Status, count)
	}
}

func TestMeetingRecoveryTerminal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		state    domain.MeetingCreateState
		result   domain.MeetingRecoveryCycleResult
		capacity int64
	}{
		{"prepared revoked", domain.MeetingCreatePrepared, domain.MeetingRecoveryCycleSettled, 0},
		{"returned generation", domain.MeetingCreateReturned, domain.MeetingRecoveryCycleSettled, 0},
		{"uncertain retained", domain.MeetingCreateStarted, domain.MeetingRecoveryCycleUnresolved, 1},
		{"generation conflict", domain.MeetingCreateReturned, domain.MeetingRecoveryCycleConflict, 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newRecoveryEffect(t, test.state)
			input := f.completion(t, test.result)

			applied, err := f.store.FinishMeetingRecovery(t.Context(), input)
			repeated, repeatErr := f.store.FinishMeetingRecovery(t.Context(), input)
			deleted, deleteErr := f.repos.Outbox.DeleteFinished(t.Context(), repositories.FinishedOutboxPurge{DoneBefore: instant.Add(time.Second), DeadBefore: instant.Add(time.Second), BatchSize: 100})
			receipt, receiptErr := f.repos.MeetingRecovery.Cycle(t.Context(), input.OutboxID)
			count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

			meetingRequire(t, err, repeatErr, deleteErr, receiptErr, countErr)
			if !applied || repeated || deleted != 1 || receipt.Result != test.result || receipt.CompletedAt == nil || count != test.capacity {
				t.Fatal("terminal completion, exact release or independent receipt retention failed")
			}
		})
	}
}

func TestMeetingRecoveryTerminalFences(t *testing.T) {
	t.Parallel()
	cases := []string{"wrong lease", "stale revision", "expired lease", "wrong generation", "uncertain settlement"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			state := domain.MeetingCreateReturned
			if name == "uncertain settlement" {
				state = domain.MeetingCreateStarted
			}
			f := newRecoveryEffect(t, state)
			input := f.completion(t, domain.MeetingRecoveryCycleSettled)
			switch name {
			case "wrong lease":
				input.Lease = domain.LeaseToken{99}
			case "stale revision":
				input.Revision--
			case "expired lease":
				input.Now = instant.Add(outbox.LeaseDuration)
			case "wrong generation":
				created := domain.ProviderCreateTime(101)
				input.ObservedCreateTime = &created
			}

			applied, err := f.store.FinishMeetingRecovery(t.Context(), input)

			meetingRequire(t, err)
			if applied {
				t.Fatal("invalid terminal proof accepted")
			}
			f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxInProgress, 1)
		})
	}
}

func TestMeetingRecoveryTerminalDispatch(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateReturned)
	input := f.completion(t, domain.MeetingRecoveryCycleSettled)

	genericErr := f.queue.Complete(t.Context(), f.claim, domain.DoneSent, instant)
	generic, genericRepoErr := f.repos.Outbox.Complete(t.Context(), repositories.OutboxCompletion{ID: f.claim.Row.ID, Lease: f.claim.Lease, Reason: domain.DoneSent, Now: instant})
	killed, killErr := f.repos.Outbox.Fail(t.Context(), repositories.OutboxFailure{ID: f.claim.Row.ID, Lease: f.claim.Lease, ErrorCode: "permanent", Now: instant})
	err := f.queue.CompleteMeetingRecovery(t.Context(), f.claim, outbox.MeetingRecoveryOutcome{Revision: input.Revision, ObservedCreateTime: input.ObservedCreateTime, Result: input.Result}, instant)

	meetingRequire(t, genericRepoErr, killErr, err)
	if !errors.Is(genericErr, outbox.ErrMeetingRecoveryOutcomeRequired) || generic || killed || !bytes.Contains(f.logs.Bytes(), []byte(`"reason":"settled"`)) || bytes.Contains(f.logs.Bytes(), []byte("provider_meeting_id")) {
		t.Fatal("generic terminal bypass or unsafe/missing typed completion diagnostics")
	}
	f.assertResult(t, domain.MeetingRecoveryCycleSettled, domain.OutboxDone, 0)
}

func TestMeetingRecoveryEighthFailure(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateStarted)
	failure := errors.New("private provider output")
	claim := f.claim
	now := instant
	var failures error

	for attempt := int32(1); attempt <= domain.OutboxMaxAttempts; attempt++ {
		if err := f.queue.Fail(t.Context(), claim, failure, now); err != nil {
			failures = errors.Join(failures, err)
			break
		}
		if attempt < domain.OutboxMaxAttempts {
			now = now.Add(time.Hour)
			claims, err := f.queue.Claim(t.Context(), domain.LaneProviderCall, 1, now)
			if err != nil || len(claims) != 1 {
				failures = errors.Join(failures, err, errors.New("next cleanup attempt was not claimable"))
				break
			}
			claim = claims[0]
		}
	}
	spent := recoveryQueue(t, f.meetingFixture, f.identity.ProviderMeetingID, domain.OutboxID{31}, domain.MeetingRecoveryAutomatic)

	meetingRequire(t, failures)
	f.assertResult(t, domain.MeetingRecoveryCycleExhausted, domain.OutboxDead, 1)
	if claim.Row.Attempts != domain.OutboxMaxAttempts || spent != domain.MeetingMutationRefused || bytes.Contains(f.logs.Bytes(), []byte(failure.Error())) {
		t.Fatal("attempt budget, finite cycle or safe failure diagnostics failed")
	}
}

func TestMeetingRecoveryExpiredLeases(t *testing.T) {
	t.Parallel()
	for _, attempts := range []int32{1, domain.OutboxMaxAttempts} {
		t.Run(map[int32]string{1: "retry", domain.OutboxMaxAttempts: "exhausted"}[attempts], func(t *testing.T) {
			f := newRecoveryEffect(t, domain.MeetingCreateStarted)
			_, err := f.pool.Exec(t.Context(), "UPDATE outbox SET attempts = $1 WHERE id = $2", attempts, f.claim.Row.ID)
			meetingRequire(t, err)

			generic, genericErr := f.repos.Outbox.ReclaimExpired(t.Context(), instant.Add(outbox.LeaseDuration))
			reclaimErr := f.queue.ReclaimExpired(t.Context(), instant.Add(outbox.LeaseDuration))
			repeated, repeatErr := f.store.ReclaimMeetingRecovery(t.Context(), instant.Add(outbox.LeaseDuration), 100)

			meetingRequire(t, genericErr, reclaimErr, repeatErr)
			if len(generic) != 0 || len(repeated.Reclaimed) != 0 || len(repeated.Failures) != 0 {
				t.Fatal("generic or repeated reclaim took a meeting lease")
			}
			if attempts == domain.OutboxMaxAttempts {
				f.assertResult(t, domain.MeetingRecoveryCycleExhausted, domain.OutboxDead, 1)
			} else {
				f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxPending, 1)
			}
		})
	}
}

func TestMeetingRecoveryTerminalCancellation(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateReturned)
	input := f.completion(t, domain.MeetingRecoveryCycleSettled)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	release, holder := holdRecoveryTransaction(t, ctx, f.meetingFixture, func(repos repositories.Repositories) error {
		return repos.Meetings.CreateUnderLessonLock(ctx, meetingInput(11, f.otherLesson))
	})
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	completion := recoveryRun(t, ctx, "terminal-cancellation", func() error {
		_, err := f.store.FinishMeetingRecovery(waiting, input)
		return err
	})
	waitMeetingLock(t, ctx, f.meetingFixture)
	cancelWait()
	cancelled := meetingAwait(t, ctx, completion)
	release()
	holderErr := meetingAwait(t, ctx, holder)
	row := f.recovery(t, f.identity.ProviderMeetingID)
	count, countErr := f.repos.MeetingRecovery.Capacity(ctx)

	meetingRequire(t, holderErr, countErr)
	if !errors.Is(cancelled, context.Canceled) || row.SettledAt != nil || row.CycleResult != domain.MeetingRecoveryCyclePending || count != 2 {
		t.Fatal("cancelled terminal persistence claimed settlement or leaked capacity")
	}
}

func TestMeetingRecoveryLateAcknowledgementFence(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateStarted)
	input := f.completion(t, domain.MeetingRecoveryCycleUnresolved)
	_, err := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID{20}, 100, instant)
	meetingRequire(t, err)

	stale, staleErr := f.store.FinishMeetingRecovery(t.Context(), input)
	failed, failErr := f.store.FailMeetingRecovery(t.Context(), repositories.OutboxFailure{ID: f.claim.Row.ID, Lease: f.claim.Lease, ErrorCode: "permanent", Now: instant})
	final := recoveryQueue(t, f.meetingFixture, f.identity.ProviderMeetingID, domain.OutboxID{31}, domain.MeetingRecoveryAutomatic)
	row := f.recovery(t, f.identity.ProviderMeetingID)
	first, firstErr := f.repos.MeetingRecovery.Cycle(t.Context(), f.claim.Row.ID)

	meetingRequire(t, staleErr, failErr, firstErr)
	if stale || !failed || final != domain.MeetingMutationApplied || !row.FinalCycleRequested || !row.FinalCycleUsed || row.CycleNumber != 2 || row.CreateState != domain.MeetingCreateReturned || first.Result != domain.MeetingRecoveryCycleExhausted {
		t.Fatal("late acknowledgement lost its evidence fence or final bounded cycle")
	}
}

func TestMeetingRecoveryMissingReceiptRefused(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateReturned)
	input := f.completion(t, domain.MeetingRecoveryCycleSettled)
	_, err := f.pool.Exec(t.Context(), "DELETE FROM meeting_recovery_cycles WHERE outbox_id = $1", f.claim.Row.ID)
	meetingRequire(t, err)

	applied, finishErr := f.store.FinishMeetingRecovery(t.Context(), input)
	row := f.recovery(t, f.identity.ProviderMeetingID)
	effect, effectErr := f.repos.Outbox.ByID(t.Context(), f.claim.Row.ID)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

	meetingRequire(t, finishErr, effectErr, countErr)
	if applied || row.SettledAt != nil || row.CycleResult != domain.MeetingRecoveryCyclePending || effect.Status != domain.OutboxInProgress || count != 1 {
		t.Fatal("missing receipt permitted partial terminal persistence")
	}
}

func TestMeetingRecoveryReclaimSkipsLockedLedger(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateStarted)
	otherInput := meetingInput(11, f.otherLesson)
	meetingRequire(t, f.repos.Meetings.CreateUnderLessonLock(t.Context(), otherInput))
	otherIdentity := meetingIdentity(otherInput)
	_, err := f.repos.Meetings.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: otherIdentity, State: domain.MeetingCreating}, domain.MeetingCloseProviderError, instant)
	meetingRequire(t, err)
	if recoveryQueue(t, f.meetingFixture, otherIdentity.ProviderMeetingID, domain.OutboxID{31}, domain.MeetingRecoveryAutomatic) != domain.MeetingMutationApplied {
		t.Fatal("second queue refused")
	}
	_, err = f.queue.Claim(t.Context(), domain.LaneProviderCall, 1, instant)
	meetingRequire(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	release, holder := holdRecoveryTransaction(t, ctx, f.meetingFixture, func(repos repositories.Repositories) error {
		_, err := repos.MeetingRecovery.MarkCreateUncertain(ctx, f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID{20}, instant)
		return err
	})

	reclaimed, reclaimErr := f.store.ReclaimMeetingRecovery(ctx, instant.Add(outbox.LeaseDuration), 100)
	release()
	holderErr := meetingAwait(t, ctx, holder)

	meetingRequire(t, reclaimErr, holderErr)
	if len(reclaimed.Reclaimed) != 1 || len(reclaimed.Failures) != 0 || reclaimed.Reclaimed[0].ID != (domain.OutboxID{31}) || reclaimed.Reclaimed[0].Status != domain.OutboxPending {
		t.Fatal("locked first ledger pinned another expired lease")
	}
}

func TestMeetingRecoveryReceiptMismatchRollsBack(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateReturned)
	input := f.completion(t, domain.MeetingRecoveryCycleSettled)
	_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery_cycles SET result = 'exhausted', completed_at = $1 WHERE outbox_id = $2", instant, f.claim.Row.ID)
	meetingRequire(t, err)

	applied, finishErr := f.store.FinishMeetingRecovery(t.Context(), input)
	row := f.recovery(t, f.identity.ProviderMeetingID)
	effect, effectErr := f.repos.Outbox.ByID(t.Context(), f.claim.Row.ID)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

	meetingRequire(t, finishErr, effectErr, countErr)
	if applied || row.Revision != input.Revision || row.SettledAt != nil || row.CycleResult != domain.MeetingRecoveryCyclePending || effect.Status != domain.OutboxInProgress || count != 1 {
		t.Fatal("receipt failure committed partial settlement or capacity release")
	}
}

func TestMeetingRecoveryTerminalReclaimRace(t *testing.T) {
	t.Parallel()
	f := newRecoveryEffect(t, domain.MeetingCreateReturned)
	input := f.completion(t, domain.MeetingRecoveryCycleSettled)
	_, err := f.pool.Exec(t.Context(), "UPDATE outbox SET attempts = $1 WHERE id = $2", domain.OutboxMaxAttempts, f.claim.Row.ID)
	meetingRequire(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	terminal := recoveryRun(t, ctx, "terminal-racer", func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-start:
		}
		_, err := f.store.FinishMeetingRecovery(ctx, input)
		return err
	})
	reclaimer := recoveryRun(t, ctx, "reclaim-racer", func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-start:
		}
		_, err := f.store.ReclaimMeetingRecovery(ctx, instant.Add(outbox.LeaseDuration), 100)
		return err
	})

	close(start)
	terminalErr := meetingAwait(t, ctx, terminal)
	reclaimErr := meetingAwait(t, ctx, reclaimer)
	row := f.recovery(t, f.identity.ProviderMeetingID)

	meetingRequire(t, terminalErr, reclaimErr)
	if row.CycleResult == domain.MeetingRecoveryCycleSettled {
		f.assertResult(t, domain.MeetingRecoveryCycleSettled, domain.OutboxDone, 0)
	} else {
		f.assertResult(t, domain.MeetingRecoveryCycleExhausted, domain.OutboxDead, 1)
	}
}

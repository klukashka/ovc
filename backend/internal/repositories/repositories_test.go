package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

var instant = time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)

func hash(text string) []byte {
	sum := sha256.Sum256([]byte(text))
	return sum[:]
}

func fixturePasswordHash() string {
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%x", hash("fixture"))
}

func newStore(t *testing.T) *repositories.Store {
	t.Helper()
	return repositories.NewStore(testdb.New(t).Pool)
}

func createUser(t *testing.T, store *repositories.Store, id domain.UserID, email string) {
	t.Helper()
	err := store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID:             id,
		Role:           domain.RoleTutor,
		State:          domain.AccountUnverified,
		Email:          email,
		PasswordHash:   fixturePasswordHash(),
		Name:           "A language tutor",
		Language:       domain.LanguageEnglish,
		Timezone:       "Europe/Moscow",
		Theme:          domain.ThemeSystem,
		ConsentVersion: "2026-10",
		ConsentAt:      instant,
		CreatedAt:      instant,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func insertOutbox(t *testing.T, store *repositories.Store, id byte, priority domain.OutboxPriority, due time.Time) {
	t.Helper()
	inserted, err := store.Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID:             domain.OutboxID{id},
		Lane:           domain.LaneEmail,
		Priority:       priority,
		Kind:           "account.verify_email",
		Payload:        []byte(`{}`),
		IdempotencyKey: string(rune('a' + id)),
		NextAttemptAt:  due,
		CreatedAt:      instant,
	})
	if err != nil || !inserted {
		t.Fatalf("insert outbox row %d: inserted %v, %v", id, inserted, err)
	}
}

func claim(t *testing.T, store *repositories.Store, lease byte, limit int32) []domain.OutboxID {
	t.Helper()
	rows, err := store.Repositories().Outbox.Claim(context.Background(), repositories.OutboxClaim{
		Lane: domain.LaneEmail, Now: instant, Limit: limit, Lease: domain.LeaseToken{lease}, LeaseEnds: instant.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]domain.OutboxID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func TestUsersRoundTripNamedIdentifiersAndCaseInsensitiveEmail(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{7}, "Tutor@Example.org")

	user, err := store.Repositories().Users.ByEmail(context.Background(), "tutor@example.org")

	if err != nil || user.ID != (domain.UserID{7}) || user.State != domain.AccountUnverified || !user.UnverifiedSince.Equal(instant) {
		t.Fatalf("got %+v, %v", user, err)
	}
	_, missing := store.Repositories().Users.ByID(context.Background(), domain.UserID{8})
	if !errors.Is(missing, repositories.ErrNotFound) {
		t.Fatalf("missing user: %v", missing)
	}
}

func TestRateLimitReservationIsAtomicUnderConcurrency(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	slot := repositories.RateLimitSlot{Purpose: "sign_in_email", Subject: hash("subject"), WindowStart: instant}
	const attempts = 20

	counts := make(chan int32, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Go(func() {
			count, err := store.Repositories().RateLimits.Reserve(context.Background(), slot)
			if err != nil {
				t.Error(err)
			}
			counts <- count
		})
	}
	wait.Wait()
	close(counts)

	seen := slices.Sorted(func(yield func(int32) bool) {
		for count := range counts {
			if !yield(count) {
				return
			}
		}
	})
	for index, count := range seen {
		if count != int32(index+1) {
			t.Fatalf("reservations returned %v, want 1 to %d once each", seen, attempts)
		}
	}
}

func TestRateLimitReleaseNeverGoesBelowZero(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	slot := repositories.RateLimitSlot{Purpose: "sign_in_email", Subject: hash("subject"), WindowStart: instant}
	limits := store.Repositories().RateLimits
	if _, err := limits.Reserve(context.Background(), slot); err != nil {
		t.Fatal(err)
	}

	first, firstErr := limits.Release(context.Background(), slot)
	second, secondErr := limits.Release(context.Background(), slot)

	if firstErr != nil || secondErr != nil || !first || second {
		t.Fatalf("releases: %v %v, %v %v", first, firstErr, second, secondErr)
	}
}

func TestOutboxClaimsByPriorityWithoutOverlapAndOnlyDueRows(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	insertOutbox(t, store, 1, domain.PriorityNormal, instant.Add(-time.Minute))
	insertOutbox(t, store, 2, domain.PriorityNormal, instant.Add(-2*time.Minute))
	insertOutbox(t, store, 3, domain.PriorityHigh, instant)
	insertOutbox(t, store, 4, domain.PriorityHigh, instant.Add(time.Hour))

	first := claim(t, store, 1, 2)
	second := claim(t, store, 2, 10)

	if !slices.Equal(first, []domain.OutboxID{{3}, {2}}) || !slices.Equal(second, []domain.OutboxID{{1}}) {
		t.Fatalf("first claim %v, second claim %v", first, second)
	}
}

func TestOutboxConcurrentClaimsNeverShareARow(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	for id := byte(1); id <= 12; id++ {
		insertOutbox(t, store, id, domain.PriorityNormal, instant)
	}

	leases := []domain.LeaseToken{{1}, {2}, {3}, {4}}
	results := make([][]domain.OutboxID, len(leases))
	var wait sync.WaitGroup
	for worker, lease := range leases {
		wait.Go(func() {
			rows, err := store.Repositories().Outbox.Claim(context.Background(), repositories.OutboxClaim{
				Lane: domain.LaneEmail, Now: instant, Limit: 5, Lease: lease, LeaseEnds: instant.Add(time.Minute),
			})
			if err != nil {
				t.Error(err)
			}
			for _, row := range rows {
				results[worker] = append(results[worker], row.ID)
			}
		})
	}
	wait.Wait()

	claimed := map[domain.OutboxID]bool{}
	for _, ids := range results {
		for _, id := range ids {
			if claimed[id] {
				t.Fatalf("row %v claimed twice: %v", id, results)
			}
			claimed[id] = true
		}
	}
	if len(claimed) != 12 {
		t.Fatalf("claimed %d of 12 rows: %v", len(claimed), results)
	}
}

func TestOutboxTransitionsRequireTheCurrentLease(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	outbox := store.Repositories().Outbox
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)
	claim(t, store, 1, 1)
	completion := func(lease byte) repositories.OutboxCompletion {
		return repositories.OutboxCompletion{ID: domain.OutboxID{1}, Lease: domain.LeaseToken{lease}, Reason: domain.DoneSent, Now: instant}
	}

	foreign, foreignErr := outbox.Complete(context.Background(), completion(9))
	owned, ownedErr := outbox.Complete(context.Background(), completion(1))
	repeated, repeatedErr := outbox.Complete(context.Background(), completion(1))

	if foreignErr != nil || ownedErr != nil || repeatedErr != nil || foreign || !owned || repeated {
		t.Fatalf("foreign %v %v, owned %v %v, repeated %v %v", foreign, foreignErr, owned, ownedErr, repeated, repeatedErr)
	}
}

func TestOutboxExpiredLeaseKillsOnlyRowsThatUsedEveryAttempt(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	outbox := store.Repositories().Outbox
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)
	for range domain.OutboxMaxAttempts - 1 {
		claim(t, store, 1, 1)
		if _, err := outbox.ReclaimExpired(context.Background(), instant.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	insertOutbox(t, store, 2, domain.PriorityNormal, instant)
	claim(t, store, 1, 2)

	reclaimed, err := outbox.ReclaimExpired(context.Background(), instant.Add(2*time.Minute))

	statuses := map[domain.OutboxID]domain.OutboxStatus{}
	for _, row := range reclaimed {
		statuses[row.ID] = row.Status
	}
	if err != nil || statuses[domain.OutboxID{1}] != domain.OutboxDead || statuses[domain.OutboxID{2}] != domain.OutboxPending {
		t.Fatalf("reclaimed %+v, %v", reclaimed, err)
	}
	dead, lookupErr := outbox.ByID(context.Background(), domain.OutboxID{1})
	if lookupErr != nil || dead.LastErrorCode == nil || *dead.LastErrorCode != "lease_expired" || dead.Attempts != domain.OutboxMaxAttempts {
		t.Fatalf("dead row %+v, %v", dead, lookupErr)
	}
	pending, pendingErr := outbox.ByID(context.Background(), domain.OutboxID{2})
	if pendingErr != nil || pending.LastErrorCode != nil {
		t.Fatalf("pending row %+v, %v", pending, pendingErr)
	}
	if again := claim(t, store, 2, 5); !slices.Equal(again, []domain.OutboxID{{2}}) {
		t.Fatalf("claimed after reclaim: %v", again)
	}
}

func TestOutboxRowFirstReclaimedReturnsToPending(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)
	claim(t, store, 1, 1)

	reclaimed, err := store.Repositories().Outbox.ReclaimExpired(context.Background(), instant.Add(2*time.Minute))
	again := claim(t, store, 2, 1)

	if err != nil || len(reclaimed) != 1 || reclaimed[0].Status != domain.OutboxPending || !slices.Equal(again, []domain.OutboxID{{1}}) {
		t.Fatalf("reclaimed %+v (%v), claimed again %v", reclaimed, err, again)
	}
}

func TestOutboxRetryAndFailureFollowTheLease(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	outbox := store.Repositories().Outbox
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)
	claim(t, store, 1, 1)

	retried, retryErr := outbox.Retry(context.Background(), repositories.OutboxRetry{
		ID: domain.OutboxID{1}, Lease: domain.LeaseToken{1}, NextAttemptAt: instant.Add(-time.Second), ErrorCode: "network",
	})
	claim(t, store, 2, 1)
	staleFailure, staleErr := outbox.Fail(context.Background(), repositories.OutboxFailure{
		ID: domain.OutboxID{1}, Lease: domain.LeaseToken{1}, Now: instant, ErrorCode: "network",
	})
	failed, failErr := outbox.Fail(context.Background(), repositories.OutboxFailure{
		ID: domain.OutboxID{1}, Lease: domain.LeaseToken{2}, Now: instant, ErrorCode: "network",
	})

	if retryErr != nil || staleErr != nil || failErr != nil || !retried || staleFailure || !failed {
		t.Fatalf("retried %v %v, stale failure %v %v, failed %v %v", retried, retryErr, staleFailure, staleErr, failed, failErr)
	}
}

func TestFinishedOutboxRowsAreDeletedByTheirOwnRetention(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	outbox := store.Repositories().Outbox
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)
	insertOutbox(t, store, 2, domain.PriorityNormal, instant)
	claim(t, store, 1, 2)
	if _, err := outbox.Complete(context.Background(), repositories.OutboxCompletion{ID: domain.OutboxID{1}, Lease: domain.LeaseToken{1}, Reason: domain.DoneSent, Now: instant}); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Fail(context.Background(), repositories.OutboxFailure{ID: domain.OutboxID{2}, Lease: domain.LeaseToken{1}, Now: instant, ErrorCode: "network"}); err != nil {
		t.Fatal(err)
	}

	deleted, err := outbox.DeleteFinished(context.Background(), repositories.FinishedOutboxPurge{
		DoneBefore: instant.Add(time.Second), DeadBefore: instant, BatchSize: 10,
	})

	if err != nil || deleted != 1 {
		t.Fatalf("deleted %d rows, %v; want only the done row", deleted, err)
	}
	if _, doneErr := outbox.ByID(context.Background(), domain.OutboxID{1}); !errors.Is(doneErr, repositories.ErrNotFound) {
		t.Fatalf("the done row survived: %v", doneErr)
	}
	if _, deadErr := outbox.ByID(context.Background(), domain.OutboxID{2}); deadErr != nil {
		t.Fatalf("the dead row was deleted before its own cutoff: %v", deadErr)
	}
}

func TestOutboxInsertIsIdempotentByKey(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	insertOutbox(t, store, 1, domain.PriorityNormal, instant)

	inserted, err := store.Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID: domain.OutboxID{2}, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: "account.verify_email",
		Payload: []byte(`{}`), IdempotencyKey: "b", NextAttemptAt: instant, CreatedAt: instant,
	})

	if err != nil || inserted {
		t.Fatalf("duplicate key inserted %v, %v", inserted, err)
	}
}

func TestSessionRotationIsCompareAndSwap(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	sessions := store.Repositories().Sessions
	err := sessions.Create(context.Background(), repositories.NewSession{
		ID: domain.SessionID{1}, UserID: domain.UserID{1}, RefreshHash: hash("t0"), ExpiresAt: instant.Add(30 * 24 * time.Hour), CreatedAt: instant,
	})
	if err != nil {
		t.Fatal(err)
	}
	rotation := func(generation int64, current, next string) repositories.SessionRotation {
		return repositories.SessionRotation{ID: domain.SessionID{1}, Generation: generation, RefreshHash: hash(current), NewRefreshHash: hash(next), Now: instant}
	}

	results := make(chan error, 2)
	for _, next := range []string{"t1-a", "t1-b"} {
		go func() {
			_, rotateErr := sessions.Rotate(context.Background(), rotation(1, "t0", next))
			results <- rotateErr
		}()
	}
	outcomes := []error{<-results, <-results}
	_, staleErr := sessions.Rotate(context.Background(), rotation(1, "t0", "t1-c"))

	won := 0
	for _, outcome := range outcomes {
		switch {
		case outcome == nil:
			won++
		case !errors.Is(outcome, repositories.ErrNotFound):
			t.Fatal(outcome)
		}
	}
	if won != 1 || !errors.Is(staleErr, repositories.ErrNotFound) {
		t.Fatalf("concurrent rotations %v, stale rotation %v", outcomes, staleErr)
	}
}

func TestRevokedOrExpiredSessionsNeverRotate(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	sessions := store.Repositories().Sessions
	for id := byte(1); id <= 2; id++ {
		if err := sessions.Create(context.Background(), repositories.NewSession{
			ID: domain.SessionID{id}, UserID: domain.UserID{1}, RefreshHash: hash("t0"), ExpiresAt: instant.Add(time.Hour), CreatedAt: instant,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sessions.Revoke(context.Background(), domain.SessionID{1}, instant); err != nil {
		t.Fatal(err)
	}
	rotate := func(id byte, now time.Time) error {
		_, err := sessions.Rotate(context.Background(), repositories.SessionRotation{
			ID: domain.SessionID{id}, Generation: 1, RefreshHash: hash("t0"), NewRefreshHash: hash("t1"), Now: now,
		})
		return err
	}

	revoked := rotate(1, instant)
	expired := rotate(2, instant.Add(time.Hour))

	if !errors.Is(revoked, repositories.ErrNotFound) || !errors.Is(expired, repositories.ErrNotFound) {
		t.Fatalf("revoked rotation %v, expired rotation %v", revoked, expired)
	}
}

func TestAUserHasAtMostOneAvatarReference(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	attachments := store.Repositories().Attachments
	for id := byte(1); id <= 2; id++ {
		expires := instant.Add(time.Hour)
		if err := attachments.Create(context.Background(), repositories.NewAttachment{
			ID: domain.AttachmentID{id}, UploaderID: domain.UserID{1}, Context: domain.ContextAvatar, StorageKey: string(rune('a' + id)),
			Filename: "avatar.png", MediaType: "image/png", SizeBytes: 10, Sha256: hash("avatar"), ExpiresAt: &expires, CreatedAt: instant,
		}); err != nil {
			t.Fatal(err)
		}
	}
	reference := func(id byte) error {
		return attachments.Reference(context.Background(), repositories.NewAttachmentRef{
			AttachmentID: domain.AttachmentID{id}, RefKind: domain.ReferenceAvatar, RefID: domain.EntityID{1}, CreatedAt: instant,
		})
	}

	first := reference(1)
	second := reference(2)

	var violation *pgconn.PgError
	if first != nil || !errors.As(second, &violation) || violation.ConstraintName != "attachment_references_one_avatar_idx" {
		t.Fatalf("first avatar reference %v, second %v", first, second)
	}
}

func TestEmailTokenIsConsumedOnceAndNeverAfterExpiry(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	tokens := store.Repositories().EmailTokens
	if err := tokens.CreateRequest(context.Background(), repositories.NewEmailRequest{
		ID: domain.EmailRequestID{1}, UserID: domain.UserID{1}, Purpose: domain.EmailPurposeVerify, CreatedAt: instant,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Create(context.Background(), repositories.NewEmailToken{
		ID: domain.EmailTokenID{1}, RequestID: domain.EmailRequestID{1}, UserID: domain.UserID{1}, Purpose: domain.EmailPurposeVerify,
		TokenHash: hash("token"), ExpiresAt: instant.Add(time.Hour), CreatedAt: instant,
	}); err != nil {
		t.Fatal(err)
	}
	lookup := func(now time.Time) repositories.EmailTokenLookup {
		return repositories.EmailTokenLookup{TokenHash: hash("token"), Purpose: domain.EmailPurposeVerify, Now: now}
	}

	_, expiredErr := tokens.Consume(context.Background(), lookup(instant.Add(2*time.Hour)))
	consumed, firstErr := tokens.Consume(context.Background(), lookup(instant))
	_, secondErr := tokens.Consume(context.Background(), lookup(instant))

	if !errors.Is(expiredErr, repositories.ErrNotFound) || firstErr != nil || consumed.UserID != (domain.UserID{1}) {
		t.Fatalf("expired %v, first %+v %v", expiredErr, consumed, firstErr)
	}
	if !errors.Is(secondErr, repositories.ErrNotFound) {
		t.Fatalf("second consumption: %v", secondErr)
	}
}

func TestEmailTokenIsNeverConsumedForTheOtherPurpose(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	tokens := store.Repositories().EmailTokens
	if err := tokens.CreateRequest(context.Background(), repositories.NewEmailRequest{
		ID: domain.EmailRequestID{1}, UserID: domain.UserID{1}, Purpose: domain.EmailPurposeVerify, CreatedAt: instant,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Create(context.Background(), repositories.NewEmailToken{
		ID: domain.EmailTokenID{1}, RequestID: domain.EmailRequestID{1}, UserID: domain.UserID{1}, Purpose: domain.EmailPurposeVerify,
		TokenHash: hash("token"), ExpiresAt: instant.Add(time.Hour), CreatedAt: instant,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := tokens.Consume(context.Background(), repositories.EmailTokenLookup{TokenHash: hash("token"), Purpose: domain.EmailPurposeReset, Now: instant})

	if !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("a verification token was consumed as a reset token: %v", err)
	}
}

func TestExpiredRateLimitCountersAreDeletedInBatches(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	limits := store.Repositories().RateLimits
	for index := range 5 {
		window := instant.Add(time.Duration(index-3) * time.Hour)
		if _, err := limits.Reserve(context.Background(), repositories.RateLimitSlot{Purpose: "sign_in_email", Subject: hash(fmt.Sprint(index)), WindowStart: window}); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := limits.DeleteBefore(context.Background(), instant, 2, 10)
	again, againErr := limits.DeleteBefore(context.Background(), instant, 2, 10)

	if err != nil || againErr != nil || deleted != 3 || again != 0 {
		t.Fatalf("deleted %d (%v), then %d (%v); want 3, then 0", deleted, err, again, againErr)
	}
}

func TestPasswordRehashReplacesOnlyTheHashItWasComputedFrom(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	createUser(t, store, domain.UserID{1}, "tutor@example.org")
	users := store.Repositories().Users
	original, err := users.ByID(context.Background(), domain.UserID{1})
	if err != nil {
		t.Fatal(err)
	}
	changed := fixturePasswordHash() + "changed"
	if replaced, err := users.UpdatePasswordHash(context.Background(), repositories.PasswordRehash{
		PasswordHash: changed, Now: instant, ID: domain.UserID{1}, CurrentPasswordHash: original.PasswordHash,
	}); err != nil || !replaced {
		t.Fatalf("first change: %v %v", replaced, err)
	}

	stale, staleErr := users.UpdatePasswordHash(context.Background(), repositories.PasswordRehash{
		PasswordHash: fixturePasswordHash() + "stale", Now: instant, ID: domain.UserID{1}, CurrentPasswordHash: original.PasswordHash,
	})

	current, err := users.ByID(context.Background(), domain.UserID{1})
	if staleErr != nil || stale || err != nil || current.PasswordHash != changed {
		t.Fatalf("stale rehash replaced %v (%v); stored %q", stale, staleErr, current.PasswordHash)
	}
}

func TestOfTwoEmailRequestsAtTheSameInstantExactlyOneIsCurrent(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	user := domain.UserID{1}
	createUser(t, store, user, "tutor@example.org")
	requests := []repositories.NewEmailRequest{
		{ID: domain.EmailRequestID{1}, UserID: user, Purpose: domain.EmailPurposeVerify, CreatedAt: instant},
		{ID: domain.EmailRequestID{2}, UserID: user, Purpose: domain.EmailPurposeVerify, CreatedAt: instant},
	}
	for _, request := range requests {
		if err := store.Repositories().EmailTokens.CreateRequest(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}

	var superseded []bool
	for _, request := range requests {
		stored, err := store.Repositories().EmailTokens.Request(context.Background(), request.ID)
		if err != nil {
			t.Fatal(err)
		}
		newer, err := store.Repositories().EmailTokens.Superseded(context.Background(), stored)
		if err != nil {
			t.Fatal(err)
		}
		superseded = append(superseded, newer)
	}

	if superseded[0] == superseded[1] {
		t.Fatalf("both requests superseded: %v", superseded)
	}
}

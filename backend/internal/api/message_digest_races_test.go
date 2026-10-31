package api_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/chat"
)

type digestRun struct {
	processed int64
	err       error
}

func runDigest(service *chat.Service, now time.Time) <-chan digestRun {
	done := make(chan digestRun, 1)
	go func() {
		count, err := service.QueueDigests(context.Background(), now)
		done <- digestRun{processed: count, err: err}
	}()
	return done
}

func awaitDigest(t *testing.T, done <-chan digestRun) digestRun {
	t.Helper()
	timer := time.NewTimer(pauseDeadline)
	defer timer.Stop()
	select {
	case result := <-done:
		return result
	case <-timer.C:
		t.Fatal("digest did not finish")
		return digestRun{}
	}
}

func TestADigestNeverMissesAMessageSentWhileItRuns(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.at(t, authStart.Add(11*time.Minute))
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel)
	service := h.digestService(t, chat.DefaultDigestSettings())

	running := runDigest(service, h.now(t))
	h.waitForLockWaiters(t, 1)
	sending := h.sendMessageInBackground(token, channel, "Committed after digest snapshot", nil)
	h.waitForLockWaiters(t, 2)
	release()
	result := awaitDigest(t, running)
	sent := awaitAnswer(t, sending)

	if result.err != nil {
		t.Fatal(result.err)
	}
	h.mustAnswer(t, sent, http.StatusCreated)
	if h.outboxOf(t, domain.EventMessagesDigest, "pending") != 1 || !h.digestPending(t, parties.student) {
		t.Fatal("concurrent send was lost")
	}
}

func digestJoiningChannel(t *testing.T, h *authHarness, parties endingParties, token string, readded bool) studentGroup {
	t.Helper()
	members := []string{}
	if readded {
		members = append(members, parties.card.ID)
	}
	group := h.newGroup(t, token, "A channel outside the digest lock set", members)
	if readded {
		h.mustAnswer(t, h.groupCall(t, token, http.MethodPatch, group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
	}
	return group
}

func TestDigestDefersWhenANewChannelChangesItsRevision(t *testing.T) {
	for _, readded := range []bool{false, true} {
		t.Run(fmt.Sprintf("readded-%t", readded), func(t *testing.T) {
			t.Parallel()
			h, parties, channel := digestFixture(t)
			h.at(t, authStart.Add(11*time.Minute))
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			added := digestJoiningChannel(t, h, parties, token, readded)
			release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel)
			service := h.digestService(t, chat.DefaultDigestSettings())

			running := runDigest(service, h.now(t))
			h.waitForLockWaiters(t, 1)
			h.mustAnswer(t, h.groupCall(t, token, http.MethodPatch, added.ID, "", fmt.Sprintf(`{"add":[%q]}`, parties.card.ID)), http.StatusOK)
			h.sent(t, token, h.groupChannel(t, added.ID).ID, "New channel unread")
			release()
			result := awaitDigest(t, running)

			if result.err != nil || result.processed != 0 || h.outboxOf(t, domain.EventMessagesDigest, "pending") != 0 || !h.digestPending(t, parties.student) {
				t.Fatalf("changed channel set was consumed: %+v", result)
			}
		})
	}
}

type pausingDigestRandom struct {
	source  io.Reader
	started chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (r *pausingDigestRandom) Read(bytes []byte) (int, error) {
	r.once.Do(func() { close(r.started); <-r.resume })
	return r.source.Read(bytes)
}

func TestANewChannelSendAfterDigestLocksRecreatesItsFlag(t *testing.T) {
	for _, readded := range []bool{false, true} {
		t.Run(fmt.Sprintf("readded-%t", readded), func(t *testing.T) {
			t.Parallel()
			h, parties, _ := digestFixture(t)
			h.at(t, authStart.Add(11*time.Minute))
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			added := digestJoiningChannel(t, h, parties, token, readded)
			random := &pausingDigestRandom{source: randomPerTest(t), started: make(chan struct{}), resume: make(chan struct{})}
			var resume sync.Once
			release := func() { resume.Do(func() { close(random.resume) }) }
			t.Cleanup(release)
			service := chat.New(chat.Dependencies{Store: h.store, Clock: h.clock, Random: random, Logger: slog.New(slog.DiscardHandler)})

			running := runDigest(service, h.now(t))
			select {
			case <-random.started:
			case <-time.After(pauseDeadline):
				t.Fatal("digest did not reach outbox")
			}
			h.mustAnswer(t, h.groupCall(t, token, http.MethodPatch, added.ID, "", fmt.Sprintf(`{"add":[%q]}`, parties.card.ID)), http.StatusOK)
			sending := h.sendMessageInBackground(token, h.groupChannel(t, added.ID).ID, "New channel unread", nil)
			h.waitDigestPendingWriter(t)
			release()
			result := awaitDigest(t, running)
			sent := awaitAnswer(t, sending)

			if result.err != nil || result.processed != 1 {
				t.Fatalf("digest failed: %+v", result)
			}
			h.mustAnswer(t, sent, http.StatusCreated)
			if !h.digestPending(t, parties.student) {
				t.Fatal("pending UPSERT failed to recreate the deleted row")
			}
		})
	}
}

func (h *authHarness) waitDigestPendingWriter(t *testing.T) {
	t.Helper()
	timeout := time.NewTimer(pauseDeadline)
	defer timeout.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		var waiting bool
		err := h.database.Pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%INSERT INTO digest_pending%')").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-timeout.C:
			t.Fatal("send did not wait on pending revision")
		case <-poll.C:
		}
	}
}

func TestConcurrentDigestRunsDoNotBypassThirtyMinuteSpacing(t *testing.T) {
	t.Parallel()
	h, _, channel := digestFixture(t)
	h.at(t, authStart.Add(11*time.Minute))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel)
	service := h.digestService(t, chat.DefaultDigestSettings())

	first := runDigest(service, h.now(t))
	h.waitForLockWaiters(t, 1)
	second := runDigest(service, h.now(t).Add(time.Minute))
	h.waitForLockWaiters(t, 2)
	release()
	firstResult, secondResult := awaitDigest(t, first), awaitDigest(t, second)

	if firstResult.err != nil || secondResult.err != nil || firstResult.processed+secondResult.processed != 1 || h.outboxOf(t, domain.EventMessagesDigest, "pending") != 1 {
		t.Fatalf("overlapping jobs duplicated digest: %+v, %+v", firstResult, secondResult)
	}
}

func TestDigestRevisionsNeverRepeatAfterDeletionAndRecreation(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	original, err := h.store.Repositories().Digests.Revision(context.Background(), parties.student)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := h.readChannel(t, parties.studentToken, channel, 1)
	h.mustAnswer(t, read, http.StatusOK)

	h.sent(t, parties.tutorToken, channel, "New pending lifetime")
	current, err := h.store.Repositories().Digests.Revision(context.Background(), parties.student)
	if err != nil {
		t.Fatal(err)
	}
	err = h.store.Repositories().Digests.ClearRead(context.Background(), parties.student, original)

	if err != nil || current <= original || !h.digestPending(t, parties.student) {
		t.Fatalf("revision reused or stale clear applied: %d -> %d: %v", original, current, err)
	}
}

func TestTheDigestJobStaysWithinItsBudget(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.sent(t, parties.studentToken, channel, "Tutor unread")
	h.at(t, authStart.Add(11*time.Minute))
	settings := chat.DefaultDigestSettings()
	settings.Batch = 1
	settings.Budget = 90 * time.Second
	settings.Wall = minuteStepWall()
	service := h.digestService(t, settings)

	processed, err := service.QueueDigests(context.Background(), h.now(t))

	if err != nil || processed != 0 || h.outboxOf(t, domain.EventMessagesDigest, "pending") != 0 {
		t.Fatalf("expired budget processed %d: %v", processed, err)
	}
	if !h.digestPending(t, parties.tutor) || !h.digestPending(t, parties.student) {
		t.Fatal("expired budget lost pending work")
	}
}

func TestDigestDeadlineCancelsDatabaseWaits(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.at(t, authStart.Add(11*time.Minute))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel)
	settings := chat.DefaultDigestSettings()
	settings.Budget = 200 * time.Millisecond
	service := h.digestService(t, settings)

	running := runDigest(service, h.now(t))
	h.waitForLockWaiters(t, 1)
	result := awaitDigest(t, running)
	release()

	if !errors.Is(result.err, context.DeadlineExceeded) || result.processed != 0 || !h.digestPending(t, parties.student) {
		t.Fatalf("blocked digest ignored deadline: %+v", result)
	}
	if h.outboxOf(t, domain.EventMessagesDigest, "pending") != 0 {
		t.Fatal("cancelled digest queued email")
	}
}

func TestDigestFailureDoesNotStopOtherRecipients(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.sent(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "Tutor unread")
	h.at(t, authStart.Add(11*time.Minute))
	statement := fmt.Sprintf(`CREATE FUNCTION fail_one_digest() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.user_id = '%s'::uuid THEN RAISE EXCEPTION 'test digest failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER fail_one_digest BEFORE INSERT ON digest_state FOR EACH ROW EXECUTE FUNCTION fail_one_digest()`, parties.tutor)
	if _, err := h.database.Pool.Exec(context.Background(), statement); err != nil {
		t.Fatal(err)
	}

	processed, err := h.digestService(t, chat.DefaultDigestSettings()).QueueDigests(context.Background(), h.now(t))

	if err == nil || processed != 1 || !h.digestPending(t, parties.tutor) || h.digestPending(t, parties.student) || h.messageCount(t, channel) != 1 {
		t.Fatalf("item failure isolation: processed %d, error %v", processed, err)
	}
}

func TestAReadClearingPendingCannotLoseANewChannelSend(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM digest_pending WHERE user_id = $1::uuid FOR UPDATE", parties.student.String())

	reading := h.sendInBackground(http.MethodPost, channelsRoute+"/"+channel+"/read", `{"seq":1}`, jsonCall(parties.studentToken))
	h.waitForLockWaiters(t, 1)
	added := h.newGroup(t, parties.tutorToken, "New channel while reading", []string{parties.card.ID})
	sending := h.sendMessageInBackground(parties.tutorToken, h.groupChannel(t, added.ID).ID, "Unread after clear", nil)
	h.waitForLockWaiters(t, 2)
	release()
	read, sent := awaitAnswer(t, reading), awaitAnswer(t, sending)

	h.mustAnswer(t, read, http.StatusOK)
	h.mustAnswer(t, sent, http.StatusCreated)
	if !h.digestPending(t, parties.student) {
		t.Fatal("read cleared a later send's pending state")
	}
}

func TestAStaleReadRevisionDoesNotClearRecreatedPending(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	original, err := h.store.Repositories().Digests.Revision(context.Background(), parties.student)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := h.readChannel(t, parties.studentToken, channel, 1)
	h.mustAnswer(t, read, http.StatusOK)
	h.sent(t, parties.tutorToken, channel, "New flag lifetime")
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE channel_members SET last_read_seq = 2 WHERE user_id = $1 AND channel_id = $2::uuid", parties.student, channel); err != nil {
		t.Fatal(err)
	}

	err = h.store.Repositories().Digests.ClearRead(context.Background(), parties.student, original)

	if err != nil || !h.digestPending(t, parties.student) {
		t.Fatalf("stale read revision cleared a different lifetime: %v", err)
	}
}

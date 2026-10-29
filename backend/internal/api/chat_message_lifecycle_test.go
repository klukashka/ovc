package api_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/tutoring"
)

func (h *authHarness) storageKeyOf(t *testing.T, attachment string) string {
	t.Helper()
	var key string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT storage_key FROM attachments WHERE id = $1::uuid", attachment).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestDeletedSendersShowAsDeletedUserUntilThePurge(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	fromStudent := h.sent(t, parties.studentToken, direct.ID, "Until the purge")
	fromTutor := h.sent(t, parties.tutorToken, direct.ID, "The tutor's reply")
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)

	beforePurge := h.messagesOf(t, parties.tutorToken, direct.ID)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	h.tick(t)
	afterPurge := h.messagesOf(t, h.relogin(t, tutorEmail), direct.ID)

	deleted, _ := messageWithID(beforePurge, fromStudent.ID)
	if deleted.SenderLabel != nil || deleted.SenderLabelKind == nil || *deleted.SenderLabelKind != string(domain.LabelDeletedUser) ||
		deleted.Body == nil || *deleted.Body != "Until the purge" || deleted.Erased {
		t.Fatalf("before the purge the tutor sees %+v", deleted)
	}
	if h.userExists(t, studentEmail) {
		t.Fatal("the student was not purged")
	}
	purged, _ := messageWithID(afterPurge, fromStudent.ID)
	if purged.SenderLabelKind == nil || *purged.SenderLabelKind != string(domain.LabelDeletedUser) || purged.Body != nil || !purged.Erased || purged.Deleted {
		t.Fatalf("after the purge the tutor sees %+v", purged)
	}
	if own, _ := messageWithID(afterPurge, fromTutor.ID); !own.Own || own.Body == nil {
		t.Fatalf("the tutor's own message became %+v", own)
	}
	if stored := h.storedMessage(t, fromStudent.ID); stored.Sender != nil || stored.Body != nil {
		t.Fatalf("the purged sender's message is stored as %+v", stored)
	}
}

func TestStudentPurgeErasesMessagesAndFilesInBatches(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "second-tutor@example.org")
	secondCard := h.linkedSignedIn(t, secondToken, parties.studentToken, "Second tutor's card", studentEmail)
	direct := h.directChannel(t, parties.card.ID)
	secondChat := h.directChannel(t, secondCard.ID)
	files := []string{
		h.pendingUpload(t, parties.studentToken, direct.ID, "one.txt", 10),
		h.pendingUpload(t, parties.studentToken, direct.ID, "two.txt", 20),
		h.pendingUpload(t, parties.studentToken, secondChat.ID, "three.txt", 30),
	}
	sent := []chatMessage{
		h.sent(t, parties.studentToken, direct.ID, "First", files[0]),
		h.sent(t, parties.studentToken, direct.ID, "Second"),
		h.sent(t, parties.studentToken, direct.ID, "Third", files[1]),
		h.sent(t, parties.studentToken, secondChat.ID, "Fourth", files[2]),
		h.sent(t, parties.studentToken, secondChat.ID, "Fifth"),
	}
	h.mustAnswer(t, h.deleteMessage(t, parties.studentToken, sent[1].ID), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	batches := tutoring.New(tutoring.Dependencies{Store: h.store, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler), PurgeBatchRows: 2})

	var erased []int64
	for range 4 {
		err := h.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			count, err := batches.PurgeBatch(context.Background(), repos, parties.student, domain.RoleStudent, h.now(t))
			erased = append(erased, count)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	h.tick(t)

	if !slices.Equal(erased, []int64{2, 2, 1, 0}) {
		t.Fatalf("the batches erased %v messages", erased)
	}
	for index, message := range sent {
		stored := h.storedMessage(t, message.ID)
		if stored.Body != nil || stored.Sender != nil || stored.Erased == (index == 1) || stored.Deleted != (index == 1) {
			t.Fatalf("the purged student's message %d is %+v", index, stored)
		}
	}
	for _, file := range files {
		if h.chargesOf(t, file) != 0 || h.referencesOf(t, file) != 0 || !removedFile(h.uploadState(t, file)) || !h.deletionQueued(t, file) {
			t.Fatalf("the purged student's file %s is %s with %d charges", file, h.uploadState(t, file), h.chargesOf(t, file))
		}
	}
	if h.usage(t, parties.tutor) != 0 || h.usage(t, second) != 0 || h.userExists(t, studentEmail) || h.countLogs(`"msg":"account.purge_failed"`) != 0 {
		t.Fatalf("after the purge the tutors use %d and %d bytes", h.usage(t, parties.tutor), h.usage(t, second))
	}
}

func TestTutorPurgeRemovesChargesAndStudentUploadsBeforeTheProfile(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	files := []string{
		h.pendingUpload(t, parties.studentToken, direct.ID, "student-direct.txt", 10),
		h.pendingUpload(t, parties.studentToken, group.ID, "student-group.txt", 20),
		h.pendingUpload(t, parties.tutorToken, group.ID, "tutor-group.txt", 30),
	}
	h.sent(t, parties.studentToken, direct.ID, "Mine", files[0])
	h.sent(t, parties.studentToken, group.ID, "Ours", files[1])
	h.sent(t, parties.tutorToken, group.ID, "The tutor's", files[2])
	h.sent(t, parties.studentToken, direct.ID, "Text only")
	keys := make([]string, 0, len(files))
	for _, file := range files {
		keys = append(keys, h.storageKeyOf(t, file))
	}
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)

	h.tick(t)
	h.tick(t)

	if h.userExists(t, tutorEmail) || h.countLogs(`"msg":"account.purge_failed"`) != 0 || !h.userExists(t, studentEmail) {
		t.Fatalf("the tutor's purge did not finish:\n%s", h.failures())
	}
	if h.countRows(t, "SELECT count(*) FROM quota_charges WHERE tutor_id = $1::uuid", parties.tutor.String()) != 0 ||
		h.countRows(t, "SELECT count(id) FROM channels WHERE tutor_id = $1::uuid", parties.tutor.String()) != 0 ||
		h.countRows(t, "SELECT count(id) FROM messages WHERE channel_id = ANY ($1::uuid[])", fmt.Sprintf("{%s,%s}", direct.ID, group.ID)) != 0 {
		t.Fatal("the tutor's purge left charges, channels or messages")
	}
	for index, file := range files {
		if h.uploadState(t, file) != "gone" || !h.deletionQueued(t, file) {
			t.Fatalf("the file %s is %s after the tutor's purge", file, h.uploadState(t, file))
		}
		if _, stored := h.storedObject(t, keys[index]); stored {
			t.Fatalf("the object of %s outlived the tutor's purge", file)
		}
	}
}

func TestTheErasureJobErasesADiscardedChannelInBatches(t *testing.T) {
	t.Parallel()
	for _, path := range everyChannelDiscardPath() {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			channel := h.directChannel(t, card.ID)
			squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			file := h.pendingUpload(t, tutorToken, channel.ID, "welcome.txt", 40)
			h.sent(t, tutorToken, channel.ID, "Welcome", file)
			h.sent(t, squatterToken, channel.ID, "Hello")
			h.sent(t, squatterToken, channel.ID, "A link to look at")
			charged := h.usage(t, tutor)

			path.discard(t, h, tutorToken, card)
			h.advance(t, time.Minute)
			h.tick(t)

			erased := h.directChannel(t, card.ID)
			if charged != 40 || h.messageCount(t, channel.ID) != 0 || erased.ErasePending || !erased.Discarded {
				t.Fatalf("after the erasure job the channel is %+v with %d messages", erased, h.messageCount(t, channel.ID))
			}
			if h.usage(t, tutor) != 0 || h.chargesOf(t, file) != 0 || !removedFile(h.uploadState(t, file)) || !h.deletionQueued(t, file) {
				t.Fatalf("the erased channel's file is %s with %d bytes charged", h.uploadState(t, file), h.usage(t, tutor))
			}
			if h.countLogs(`"msg":"channel.erased"`, channel.ID) != 1 {
				t.Fatal("the erasure was not recorded")
			}
		})
	}
	t.Run("in batches within its budget", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		first, firstSquatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		second, secondSquatter := h.squat(t, tutorToken, "second-newcomer@example.org", "second-newcomer@example.org")
		firstChat, secondChat := h.directChannel(t, first.ID), h.directChannel(t, second.ID)
		h.seedMessages(t, firstChat.ID, firstSquatter, 5)
		h.seedMessages(t, secondChat.ID, secondSquatter, 5)
		h.mustAnswer(t, h.removeStudent(t, tutorToken, first.ID), http.StatusOK)
		h.advance(t, time.Minute)
		h.mustAnswer(t, h.removeStudent(t, h.relogin(t, tutorEmail), second.ID), http.StatusOK)
		var ticks atomic.Int64
		wall := func() time.Time { return authStart.Add(time.Duration(ticks.Add(1)) * time.Second) }
		job := func(budget time.Duration) *chat.Service {
			return chat.New(chat.Dependencies{
				Store: h.store, Clock: h.clock, Random: rand.Reader, Addresses: h.files, Logger: slog.New(slog.DiscardHandler),
				Erasure: chat.ErasureSettings{Wall: wall, Budget: budget, MessageBatch: 2},
			})
		}

		cut, cutErr := job(4*time.Second).EraseChannels(context.Background(), h.now(t))
		afterCut := []int64{h.messageCount(t, firstChat.ID), h.messageCount(t, secondChat.ID)}
		stillPending := h.directChannel(t, first.ID).ErasePending
		finished, finishErr := job(time.Hour).EraseChannels(context.Background(), h.now(t))

		if cutErr != nil || finishErr != nil {
			t.Fatal(cutErr, finishErr)
		}
		if cut != 0 || !slices.Equal(afterCut, []int64{1, 5}) || !stillPending {
			t.Fatalf("the budget stopped after erasing %d channels with %v messages left", cut, afterCut)
		}
		if finished != 2 || h.messageCount(t, firstChat.ID) != 0 || h.messageCount(t, secondChat.ID) != 0 || h.directChannel(t, second.ID).ErasePending {
			t.Fatalf("the next run erased %d channels", finished)
		}
	})
}

func TestAKeptReclaimLinkKeepsItsMessages(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.sent(t, squatterToken, channel.ID, "From the provisional period")
	h.sent(t, tutorToken, channel.ID, "The tutor's answer")
	owner := h.reclaimed(t, newcomerEmail)
	h.sent(t, tutorToken, channel.ID, "While the link waits")

	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	forOwner := h.messagesOf(t, owner, channel.ID)
	forTutor := h.messagesOf(t, tutorToken, channel.ID)

	if !slices.Equal(sequences(forOwner), []int64{3, 2, 1}) || !slices.Equal(sequences(forTutor), []int64{3, 2, 1}) {
		t.Fatalf("the kept link shows %v to the owner and %v to the tutor", sequences(forOwner), sequences(forTutor))
	}
	if after := h.sent(t, owner, channel.ID, "The owner writes"); after.Seq != 4 {
		t.Fatalf("the owner's first message has the sequence %d", after.Seq)
	}
}

func TestAKeptReclaimLinkCountsTheProvisionalPeriodsOtherMessagesAsUnread(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		before []bool
		marker int64
		unread int64
	}{
		{"the tutor wrote during the provisional period", []bool{true, true, false, true}, 2, 3},
		{"only the account wrote during the provisional period", []bool{true, true}, 2, 1},
		{"the tutor wrote first", []bool{false, true}, 0, 3},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			channel := h.directChannel(t, card.ID)
			squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			for index, fromAccount := range testCase.before {
				sender := tutorToken
				if fromAccount {
					sender = squatterToken
				}
				h.sent(t, sender, channel.ID, fmt.Sprintf("Provisional message %d", index))
			}
			owner := h.reclaimed(t, newcomerEmail)
			h.sent(t, tutorToken, channel.ID, "While the link waits")

			h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
			_, page := h.channelList(t, owner, "")

			member := h.channelMembers(t, channel.ID)[squatter.String()]
			item, _ := itemOf(page, channel.ID)
			if member.LastRead != testCase.marker || member.Digested != member.ChannelSeq || item.UnreadCount != testCase.unread {
				t.Fatalf("the restored row is %+v and the owner counts %d unread", member, item.UnreadCount)
			}
		})
	}
}

func TestMessagesSentBeforeTheReclaimCarryTheCueForBothSides(t *testing.T) {
	t.Parallel()
	t.Run("a kept reclaimed link", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		channel := h.directChannel(t, card.ID)
		squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		before := h.sent(t, squatterToken, channel.ID, "Sent by whoever held the account")
		h.sent(t, tutorToken, channel.ID, "The tutor's answer")
		owner := h.reclaimed(t, newcomerEmail)
		h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
		log := h.recordFrames(t)

		after := h.sent(t, owner, channel.ID, "Sent by the owner")
		frames := log.settle(t, h)
		forTutor := h.messagesOf(t, tutorToken, channel.ID)
		forOwner := h.messagesOf(t, owner, channel.ID)
		_, caughtUp := h.history(t, tutorToken, channel.ID, "?after_seq=2")

		cues := func(messages []chatMessage) []bool {
			flags := make([]bool, 0, len(messages))
			for _, message := range messages {
				flags = append(flags, message.SentBeforeReclaim)
			}
			return flags
		}
		if !slices.Equal(cues(forTutor), []bool{false, false, true}) || !slices.Equal(cues(forOwner), []bool{false, false, true}) {
			t.Fatalf("the cues are %v for the tutor and %v for the owner", cues(forTutor), cues(forOwner))
		}
		if first, _ := messageWithID(forOwner, before.ID); !first.Own {
			t.Fatal("the owner does not see the account's own message as its own")
		}
		if len(caughtUp.Items) != 1 || caughtUp.Items[0].ID != after.ID || caughtUp.Items[0].SentBeforeReclaim {
			t.Fatalf("the refetch after the frame shows %+v", caughtUp.Items)
		}
		newFrame := []sentFrame{{kind: string(domain.FrameMessageNew), id: after.ID}}
		if !slices.Equal(chatFramesTo(frames, tutor), newFrame) || !slices.Equal(chatFramesTo(frames, squatter), newFrame) {
			t.Fatalf("the frames were %v and %v", chatFramesTo(frames, tutor), chatFramesTo(frames, squatter))
		}
	})
	t.Run("a link confirmed from the account's own browser", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		channel := h.directChannel(t, card.ID)
		accountToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		h.sent(t, accountToken, channel.ID, "Before the confirmation")

		h.mustAnswer(t, h.confirmInOwnBrowser(t, newcomerEmail, accountToken), http.StatusOK)
		listed := h.messagesOf(t, tutorToken, channel.ID)

		if len(listed) != 1 || listed[0].SentBeforeReclaim || h.directChannel(t, card.ID).ProvisionalThroughSeq != nil {
			t.Fatalf("a confirmed link shows %+v", listed)
		}
	})
}

func TestAStudentPurgeBatchReadsAgainWhenItsMessagesWentMeanwhile(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	taken := h.sent(t, parties.studentToken, direct.ID, "Taken by another purge")
	batches := tutoring.New(tutoring.Dependencies{Store: h.store, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler), PurgeBatchRows: 2})
	release := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM messages WHERE id = $1::uuid FOR UPDATE", taken.ID,
		"DELETE FROM messages WHERE id = $1::uuid")

	counted := make(chan int64, 1)
	failed := make(chan error, 1)
	go func() {
		var count int64
		err := h.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			var batchErr error
			count, batchErr = batches.PurgeBatch(context.Background(), repos, parties.student, domain.RoleStudent, authStart)
			return batchErr
		})
		counted <- count
		failed <- err
	}()
	h.waitForLockWaiters(t, 1)
	release()
	count, err := <-counted, <-failed

	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || h.messageCount(t, direct.ID) != 0 {
		t.Fatalf("the batch whose messages went meanwhile reported %d", count)
	}
}

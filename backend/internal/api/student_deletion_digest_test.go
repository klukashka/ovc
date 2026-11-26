package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/tutoring"
)

type studentDeletionDigestFixture struct {
	h            *authHarness
	student      domain.UserID
	studentToken string
	other        domain.UserID
	otherToken   string
	tutors       []domain.UserID
	tutorTokens  []string
	channels     []domain.ChannelID
	links        []string
	lessons      []string
	attachments  []string
	usage        []int64
}

func newStudentDeletionDigestFixture(t *testing.T, purge bool) studentDeletionDigestFixture {
	t.Helper()
	h := newFlowHarness(t)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 10, studentEmail)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 11, "digest-peer@example.org")
	f := studentDeletionDigestFixture{h: h, student: student, studentToken: studentToken, other: other, otherToken: otherToken}
	mirror := chat.Mirror{Random: randomPerTest(t), Logger: slog.New(slog.DiscardHandler)}
	for index := byte(1); index <= 2; index++ {
		tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, index, fmt.Sprintf("digest-tutor-%d@example.org", index))
		ownCard := h.seedCard(t, tutor, index, domain.RelationshipActive, &student)
		h.seedCard(t, tutor, index+2, domain.RelationshipActive, &other)
		group := domain.GroupID{0xd0, index}
		channel := domain.ChannelID{0xe0, 3 - index}
		links := []domain.RelationshipID{{0xc0, index}, {0xc0, index + 2}}
		ctx := context.Background()
		err := h.store.WithTx(ctx, func(repos repositories.Repositories) error {
			if err := repos.Groups.Create(ctx, repositories.NewGroup{ID: group, TutorID: tutor, Name: fmt.Sprintf("Shared group %d", index), Colour: domain.ColourClay, CreatedAt: authStart}); err != nil {
				return err
			}
			_, err := repos.Groups.AddMembers(ctx, tutor, group, links, authStart)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.database.Pool.Exec(ctx, `INSERT INTO channels (id, kind, tutor_id, group_id, last_activity_at, created_at, updated_at)
VALUES ($1, 'group', $2, $3, $4, $4, $4)`, channel, tutor, group, authStart); err != nil {
			t.Fatal(err)
		}
		err = h.store.WithTx(ctx, func(repos repositories.Repositories) error {
			return mirror.Apply(ctx, repos, domain.PathLinking, chat.Audiences{Relationships: links, Groups: []domain.GroupID{group}}, authStart)
		})
		if err != nil {
			t.Fatal(err)
		}
		attachment := uploadedID(t, h.chatUpload(t, token, channel.String(), "retained.png", pngBytes(t, 2, 2)))
		h.sent(t, token, channel.String(), "Only the shared group's members may read this", attachment)
		f.attachments = append(f.attachments, attachment)
		f.usage = append(f.usage, h.usage(t, tutor))
		read, _ := h.readChannel(t, studentToken, channel.String(), 1)
		h.mustAnswer(t, read, http.StatusOK)
		lesson := h.scheduled(t, token, oneOffFor(ownCard, authStart.Add(24*time.Hour).Format("2006-01-02"), "12:00"))
		f.tutors = append(f.tutors, tutor)
		f.tutorTokens = append(f.tutorTokens, token)
		f.channels = append(f.channels, channel)
		f.links = append(f.links, ownCard)
		f.lessons = append(f.lessons, lesson.Lessons[0].ID)
	}
	if purge {
		h.markDeleted(t, student, authStart.Add(-31*24*time.Hour))
	}
	h.at(t, authStart.Add(11*time.Minute))
	return f
}

func (f studentDeletionDigestFixture) run(t *testing.T, purge bool) <-chan pendingAnswer {
	t.Helper()
	if !purge {
		return f.h.deleteInBackground(f.studentToken, accountPassword)
	}
	logger := slog.New(slog.NewJSONHandler(f.h.logs, nil))
	fieldKey := sha256.Sum256([]byte("test-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), randomPerTest(t))
	if err != nil {
		t.Fatal(err)
	}
	effects := tutoring.New(tutoring.Dependencies{
		Cipher: cipher, PublicURL: publicOrigin, Store: f.h.store, Clock: f.h.clock, Random: randomPerTest(t), Keys: f.h.keys, Logger: logger, Exports: f.h.effects,
	})
	purger := accounts.NewPurger(f.h.store, f.h.files, effects, f.h.effects, logger)
	now := f.h.now(t)
	done := make(chan pendingAnswer, 1)
	go func() {
		count, err := purger.PurgeDeleted(context.Background(), now)
		status := http.StatusInternalServerError
		if count == 1 {
			status = http.StatusNoContent
		}
		done <- pendingAnswer{result: response{status: status}, err: err}
	}()
	return done
}

func TestStudentDeletionAndPurgeSerializeWithOtherMemberDigest(t *testing.T) {
	for _, purge := range []bool{false, true} {
		for _, digestFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("purge-%t/digest-first-%t", purge, digestFirst), func(t *testing.T) {
				t.Parallel()
				f := newStudentDeletionDigestFixture(t, purge)
				h := f.h
				release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", f.channels[0].String())
				service := h.digestService(t, chat.DefaultDigestSettings())

				var deleting <-chan pendingAnswer
				var digesting <-chan digestRun
				if digestFirst {
					digesting = runDigest(service, h.now(t))
					h.waitForLockWaiters(t, 1)
					deleting = f.run(t, purge)
				} else {
					deleting = f.run(t, purge)
					h.waitForLockWaiters(t, 1)
					digesting = runDigest(service, h.now(t))
				}
				h.waitForLockWaiters(t, 2)
				release()
				deleted := awaitAnswer(t, deleting)
				digested := awaitDigest(t, digesting)

				h.mustAnswer(t, deleted, http.StatusNoContent)
				if digested.err != nil || digested.processed != 1 {
					t.Fatalf("other member digest processed %d: %v", digested.processed, digested.err)
				}
				f.assertEffects(t, purge)
			})
		}
	}
}

func (f studentDeletionDigestFixture) assertEffects(t *testing.T, purge bool) {
	t.Helper()
	h := f.h
	for index, channel := range f.channels {
		if h.usage(t, f.tutors[index]) != f.usage[index] || h.chargesOf(t, f.attachments[index]) != 1 || h.referencesOf(t, f.attachments[index]) != 1 {
			t.Fatal("deletion changed another member's attachment charge or references")
		}
		members := h.channelMembers(t, channel.String())
		if len(members) != 2 || members[f.other.String()].User != f.other.String() || members[f.tutors[index].String()].User != f.tutors[index].String() {
			t.Fatalf("wrong retained members: %+v", members)
		}
		if h.membershipsOf(t, f.links[index]) != 0 || h.messageCount(t, channel.String()) != 1 {
			t.Fatal("deletion changed another member's data or kept a group membership")
		}
		visible, _ := h.history(t, f.otherToken, channel.String(), "")
		outsider, _ := h.history(t, f.tutorTokens[1-index], channel.String(), "")
		h.mustAnswer(t, visible, http.StatusOK)
		h.mustAnswer(t, outsider, http.StatusNotFound)
		lesson := decodeLesson(t, h.lessonPage(t, f.tutorTokens[index], f.lessons[index]))
		if lesson.Status != "cancelled" {
			t.Fatalf("student's scheduled lesson stayed %s", lesson.Status)
		}
		link := h.purgedRow(t, f.links[index])
		if link.EndedBy == nil || *link.EndedBy != "deletion" || (purge && (link.StudentID != nil || link.State != "discarded")) || (!purge && link.State != "ended") {
			t.Fatalf("wrong deletion history: %+v", link)
		}
	}
	var payloadBytes []byte
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT payload FROM outbox WHERE kind = $1", domain.EventMessagesDigest).Scan(&payloadBytes); err != nil {
		t.Fatal(err)
	}
	var payload domain.DigestPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatal(err)
	}
	wanted := []string{hex.EncodeToString(f.channels[0][:]), hex.EncodeToString(f.channels[1][:])}
	if payload.Recipient != hex.EncodeToString(f.other[:]) || len(payload.Channels) != 2 {
		t.Fatalf("wrong digest recipient or bounds: %+v", payload)
	}
	for _, channel := range payload.Channels {
		if !slices.Contains(wanted, channel.Channel) || channel.ThroughSeq != 1 {
			t.Fatalf("digest escaped shared channels: %+v", payload)
		}
	}
	if h.countRows(t, "SELECT count(channel_id) FROM channel_members WHERE user_id = $1::uuid", f.student.String()) != 0 && purge {
		t.Fatal("purge kept memberships")
	}
	if strings.Contains(h.logs.String(), "Only the shared group's members may read this") {
		t.Fatal("message content reached diagnostics")
	}
}

func (f studentDeletionDigestFixture) historicalChannel(t *testing.T, state domain.RelationshipState) (string, string) {
	t.Helper()
	h := f.h
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "historical-tutor@example.org")
	link := h.seedCard(t, tutor, 5, state, &f.student)
	channel := domain.ChannelID{1}
	var discarded *time.Time
	if state == domain.RelationshipDiscarded {
		discarded = &authStart
	}
	ctx := context.Background()
	_, err := h.database.Pool.Exec(ctx, `INSERT INTO channels (id, kind, tutor_id, relationship_id, last_activity_at, discarded_at, erase_pending_since, created_at, updated_at)
VALUES ($1, 'direct', $2, $3::uuid, $4, $5, $5, $4, $4)`, channel, tutor, link, authStart, discarded)
	if err != nil {
		t.Fatal(err)
	}
	mirror := chat.Mirror{Random: randomPerTest(t), Logger: slog.New(slog.DiscardHandler)}
	err = h.store.WithTx(ctx, func(repos repositories.Repositories) error {
		return mirror.Apply(ctx, repos, domain.PathLinking, chat.Audiences{Relationships: []domain.RelationshipID{{0xc0, 5}}}, h.now(t))
	})
	if err != nil {
		t.Fatal(err)
	}
	return channel.String(), link
}

func TestStudentDeletionAndPurgeLockHistoricalChannelsBeforeSharedGroups(t *testing.T) {
	for _, state := range []domain.RelationshipState{domain.RelationshipEnded, domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipDiscarded} {
		for _, purge := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/purge-%t", state, purge), func(t *testing.T) {
				t.Parallel()
				f := newStudentDeletionDigestFixture(t, purge)
				h := f.h
				historical, link := f.historicalChannel(t, state)
				inaccessible, _ := h.channelCall(t, f.otherToken, historical)
				if state == domain.RelationshipReclaimPending && !h.channelMembers(t, historical)[f.student.String()].Suspended {
					t.Fatal("reclaim fixture has no suspended member")
				}
				release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", historical)
				service := h.digestService(t, chat.DefaultDigestSettings())

				deleting := f.run(t, purge)
				h.waitForLockWaiters(t, 1)
				digested := awaitDigest(t, runDigest(service, h.now(t)))
				release()
				deleted := awaitAnswer(t, deleting)

				h.mustAnswer(t, inaccessible, http.StatusNotFound)
				h.mustAnswer(t, deleted, http.StatusNoContent)
				if digested.err != nil || digested.processed != 1 {
					t.Fatalf("historical channel was locked after the shared groups; digest processed %d: %v", digested.processed, digested.err)
				}
				members := h.channelMembers(t, historical)
				_, retainsStudent := members[f.student.String()]
				if retainsStudent && (purge || state != domain.RelationshipEnded) {
					t.Fatalf("historical membership survived: %+v", members)
				}
				if state == domain.RelationshipProvisional || state == domain.RelationshipReclaimPending {
					channel := h.directChannel(t, link)
					if !channel.Discarded || !channel.ErasePending {
						t.Fatalf("waiting link was not queued for erasure: %+v", channel)
					}
				}
				f.assertEffects(t, purge)
			})
		}
	}
}

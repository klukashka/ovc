package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

func parsedSessionResource[ID ~[16]byte](t *testing.T, value string) ID {
	t.Helper()
	id, valid := domain.ParseUUID[ID](value)
	if !valid {
		t.Fatalf("invalid resource identifier %q", value)
	}
	return id
}

func (h *authHarness) authenticatedTutor(t *testing.T, token string) domain.TutorActor {
	t.Helper()
	principal, err := h.accounts.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return domain.TutorActor{UserID: principal.UserID, SessionID: principal.SessionID}
}

func (h *authHarness) cycleTutorBlock(t *testing.T) {
	t.Helper()
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := h.operators().Unblock(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) tutorBusinessSnapshot(t *testing.T) string {
	t.Helper()
	var snapshot string
	err := h.database.Pool.QueryRow(context.Background(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM relationships r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM invitations r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM groups r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY group_id, relationship_id) FROM group_members r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM join_requests r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM lessons r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM channels r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY channel_id, user_id) FROM channel_members r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM messages r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM outbox r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY user_id) FROM tutor_profiles r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY purpose, subject, window_start) FROM rate_limit_counters r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY tutor_id, student_id) FROM blocked_requesters r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM attachments r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY tutor_id, attachment_id) FROM quota_charges r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY attachment_id, ref_kind, ref_id, question_id) FROM attachment_references r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM notifications r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY user_id) FROM digest_pending r),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM lesson_series r)
 )::text`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestTutorMutationsRejectTheAuthenticatedSessionAfterBlockAndUnblock(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.activeStudentInAGroup(t)
	actor := h.authenticatedTutor(t, parties.tutorToken)
	ctx := context.Background()
	card := parsedSessionResource[domain.RelationshipID](t, parties.card.ID)
	group := parsedSessionResource[domain.GroupID](t, parties.group.ID)
	invited := h.newCard(t, parties.tutorToken, "Another student", "another@example.org")
	invitedID := parsedSessionResource[domain.RelationshipID](t, invited.ID)
	_, requesterToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 8, requestingEmail)
	request := h.enteredRequest(t, requesterToken, h.storedCode(t, parties.tutor).Display())
	channel := parsedSessionResource[domain.ChannelID](t, h.directChannel(t, parties.card.ID).ID)
	author := chat.Author{UserID: actor.UserID, SessionID: actor.SessionID, Role: domain.RoleTutor}
	message, err := h.chat.Send(ctx, author, channel, chat.Draft{Body: "Before revocation"})
	if err != nil {
		t.Fatal(err)
	}
	lesson, err := h.schedule.CreateLessons(ctx, actor, schedule.NewLesson{RelationshipID: &card, Title: "Before revocation", Date: "2030-03-12", Start: "18:00", DurationMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	provisionalID := parsedSessionResource[domain.RelationshipID](t, h.linkedCard(t, parties.tutor, 9, domain.RelationshipProvisional))
	blocked, blockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 10, "blocked@example.org")
	blockedRequest := h.enteredRequest(t, blockedToken, h.storedCode(t, parties.tutor).Display())
	if _, err := h.tutoring.DeclineRequest(ctx, actor, blockedRequest, true); err != nil {
		t.Fatal(err)
	}
	name := "After revocation"
	mutations := map[string]func() error{
		"remove provisional": func() error { _, err := h.tutoring.RemoveStudent(ctx, actor, provisionalID); return err },
		"unblock requester":  func() error { return h.tutoring.Unblock(ctx, actor, blocked) },
		"create card": func() error {
			_, err := h.tutoring.CreateCard(ctx, actor, tutoring.NewCard{DisplayName: name})
			return err
		},
		"update card": func() error {
			_, err := h.tutoring.UpdateCard(ctx, actor, card, tutoring.CardChange{DisplayName: &name})
			return err
		},
		"withdraw card":         func() error { return h.tutoring.WithdrawCard(ctx, actor, invitedID) },
		"regenerate invitation": func() error { _, err := h.tutoring.RegenerateInvitation(ctx, actor, invitedID); return err },
		"send invitation":       func() error { return h.tutoring.SendInvitation(ctx, actor, invitedID) },
		"end relationship":      func() error { _, err := h.tutoring.EndByTutor(ctx, actor, card); return err },
		"create group": func() error {
			_, err := h.tutoring.CreateGroup(ctx, actor, tutoring.NewGroup{Name: name, Colour: domain.Colour("moss")})
			return err
		},
		"change membership": func() error {
			_, err := h.tutoring.UpdateGroup(ctx, actor, group, tutoring.GroupChange{Remove: []domain.RelationshipID{card}})
			return err
		},
		"archive group":   func() error { _, err := h.tutoring.ArchiveGroup(ctx, actor, group); return err },
		"unarchive group": func() error { _, err := h.tutoring.UnarchiveGroup(ctx, actor, group); return err },
		"regenerate code": func() error { _, err := h.tutoring.RegenerateCode(ctx, actor); return err },
		"accept request":  func() error { _, err := h.tutoring.AcceptRequest(ctx, actor, request); return err },
		"decline request": func() error { _, err := h.tutoring.DeclineRequest(ctx, actor, request, false); return err },
		"block requester": func() error { _, err := h.tutoring.DeclineRequest(ctx, actor, request, true); return err },
		"send message":    func() error { _, err := h.chat.Send(ctx, author, channel, chat.Draft{Body: name}); return err },
		"edit message":    func() error { _, err := h.chat.Edit(ctx, author, message.ID, name, 1); return err },
		"delete message":  func() error { return h.chat.Delete(ctx, author, message.ID) },
		"create lesson": func() error {
			_, err := h.schedule.CreateLessons(ctx, actor, schedule.NewLesson{RelationshipID: &card, Title: name, Date: "2030-03-13", Start: "18:00", DurationMinutes: 60})
			return err
		},
		"edit lesson": func() error {
			_, err := h.schedule.EditLesson(ctx, actor, lesson.Lessons[0].ID, schedule.LessonEdit{Scope: domain.ScopeThis, Version: 1, Title: &name})
			return err
		},
		"cancel lesson": func() error {
			_, err := h.schedule.CancelLesson(ctx, actor, lesson.Lessons[0].ID, schedule.LessonCancellation{Scope: domain.ScopeThis, Version: 1})
			return err
		},
	}
	h.cycleTutorBlock(t)
	logStart := len(h.logs.String())
	before := h.tutorBusinessSnapshot(t)

	results := make(map[string]error, len(mutations))
	for name, mutate := range mutations {
		results[name] = mutate()
	}
	after := h.tutorBusinessSnapshot(t)

	for name, err := range results {
		if !errors.Is(err, accounts.ErrUnauthenticated) && !errors.Is(err, chat.ErrUnauthenticated) && !errors.Is(err, schedule.ErrUnauthenticated) {
			t.Errorf("%s returned %v", name, err)
		}
	}
	if after != before {
		t.Fatal("revoked tutor mutations changed business rows, effects or rate charges")
	}
	records := h.logs.String()[logStart:]
	for _, event := range []string{"chat.message.persisted", "chat.message.edited", "chat.message.deleted", "lesson.created", "lesson.edited", "lesson.cancelled", "group.archived", "group.created", "relationship.ended"} {
		if strings.Contains(records, `"msg":"`+event+`"`) {
			t.Errorf("refusal emitted successful outcome %s", event)
		}
	}

}

func TestTutorSessionExpiryIsCheckedAfterWaitingForTheProfile(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	actor := h.authenticatedTutor(t, token)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE sessions SET expires_at = $2 WHERE id = $1", actor.SessionID, authStart.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	release := h.holdTutorProfile(t, tutor)
	done := h.sendInBackground(http.MethodPost, "/api/v1/students", `{"display_name":"Too late"}`, jsonCall(token))
	h.waitForLockWaiters(t, 1)

	h.advance(t, 2*time.Minute)
	release()
	result := awaitAnswer(t, done)

	if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
		t.Fatalf("expired session answered %s", describe(result))
	}
	if h.countRows(t, "SELECT count(*) FROM relationships WHERE tutor_id=$1::uuid", tutor.String()) != 0 {
		t.Fatal("expired actor created a card")
	}
}

func TestTutorChatUploadDiscardedAfterBlockAndUnblockDuringCompletion(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(request *http.Request) bool {
		return request.Method == http.MethodPost && request.URL.Query().Has("uploadId")
	})
	upload := h.startChatUpload(t, parties.tutorToken, channel)
	upload.begin(t, "notes.txt", plainText(1000))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	during := h.onlyUploadBy(t, parties.tutor)

	h.cycleTutorBlock(t)
	held.resume()
	result := upload.answer(t)
	writeErr := <-written
	h.tick(t)

	if err := writeErr; err != nil {
		t.Fatal(err)
	}
	if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
		t.Fatalf("revoked uploader answered %s", describe(result))
	}
	if h.usage(t, parties.tutor) != 0 || h.countRows(t, "SELECT count(*) FROM attachment_references WHERE attachment_id=$1::uuid", during.ID) != 0 {
		t.Fatal("revoked upload created ownership or a charge")
	}
	if state := h.uploadState(t, during.ID); state != "gone" && state != string(domain.AttachmentDeleted) {
		t.Fatalf("refused upload state: %s", state)
	}
	if _, stored := h.storedObject(t, during.Key); stored {
		t.Fatal("worker left the refused object")
	}
	if incomplete := h.incompleteUploads(t, during.Key); len(incomplete) != 0 {
		t.Fatal("worker left an incomplete multipart upload")
	}
}

type tutorSessionParties struct {
	tutorToken   string
	studentToken string
	card         studentCard
}

func (h *authHarness) directTutorSessionParties(t *testing.T) tutorSessionParties {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	h.setAccount(t, student, endingStudentName, domain.LanguageEnglish)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	return tutorSessionParties{tutorToken: tutorToken, studentToken: studentToken, card: card}
}

type unusableTutorActor struct {
	kind  string
	actor domain.TutorActor
}

func (h *authHarness) unusableTutorIdentities(t *testing.T, actor domain.TutorActor, studentToken string) []unusableTutorActor {
	t.Helper()
	principal, err := h.accounts.Authenticate(context.Background(), studentToken)
	if err != nil {
		t.Fatal(err)
	}
	missing, foreign := actor, actor
	missing.SessionID = domain.SessionID{99}
	foreign.SessionID = principal.SessionID
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE sessions SET expires_at=$2 WHERE id=$1", actor.SessionID, authStart); err != nil {
		t.Fatal(err)
	}
	return []unusableTutorActor{{kind: "missing", actor: missing}, {kind: "foreign", actor: foreign}, {kind: "expired", actor: actor}}
}

type tutorGuardObservation struct {
	kind      string
	cardErr   error
	sendErr   error
	lessonErr error
	uploadErr error
	before    string
	after     string
}

func (h *authHarness) observeTutorGuards(t *testing.T, unusable unusableTutorActor, channel domain.ChannelID, lessonRequest schedule.NewLesson, key string) tutorGuardObservation {
	t.Helper()
	ctx := context.Background()
	actor := unusable.actor
	author := chat.Author{UserID: actor.UserID, SessionID: actor.SessionID, Role: domain.RoleTutor}
	observation := tutorGuardObservation{kind: unusable.kind, before: h.tutorBusinessSnapshot(t)}
	_, observation.cardErr = h.tutoring.CreateCard(ctx, actor, tutoring.NewCard{DisplayName: "Refused"})
	_, observation.sendErr = h.chat.Send(ctx, author, channel, chat.Draft{Body: "Replay me", ClientKey: &key})
	_, observation.lessonErr = h.schedule.CreateLessons(ctx, actor, lessonRequest)
	admission, uploadErr := h.files.BeginChatUpload(ctx, accounts.Principal{UserID: actor.UserID, SessionID: actor.SessionID, Role: domain.RoleTutor, State: domain.AccountActive}, channel, 100)
	observation.uploadErr = uploadErr
	if admission != nil {
		admission.Close(ctx)
	}
	observation.after = h.tutorBusinessSnapshot(t)
	return observation
}

func (observation tutorGuardObservation) assertRefused(t *testing.T) {
	t.Helper()
	if !errors.Is(observation.cardErr, accounts.ErrUnauthenticated) || !errors.Is(observation.sendErr, chat.ErrUnauthenticated) || !errors.Is(observation.lessonErr, schedule.ErrUnauthenticated) || !errors.Is(observation.uploadErr, accounts.ErrUnauthenticated) {
		t.Errorf("%s guard errors: card=%v chat=%v lesson=%v upload=%v", observation.kind, observation.cardErr, observation.sendErr, observation.lessonErr, observation.uploadErr)
	}
	if observation.before != observation.after {
		t.Errorf("%s refused identity changed business state or reserved a rate slot", observation.kind)
	}
}

func TestTutorSessionGuardsRejectUnusableIdentitiesAndKeepFreshReplays(t *testing.T) {
	t.Parallel()
	cases := []string{"identities", "logout", "reset", "restore"}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			parties := h.directTutorSessionParties(t)
			actor := h.authenticatedTutor(t, parties.tutorToken)
			ctx := context.Background()
			card := parsedSessionResource[domain.RelationshipID](t, parties.card.ID)
			channel := parsedSessionResource[domain.ChannelID](t, h.directChannel(t, parties.card.ID).ID)
			author := chat.Author{UserID: actor.UserID, SessionID: actor.SessionID, Role: domain.RoleTutor}
			key := "550e8400-e29b-41d4-a716-446655440000"
			message, err := h.chat.Send(ctx, author, channel, chat.Draft{Body: "Replay me", ClientKey: &key})
			if err != nil {
				t.Fatal(err)
			}
			replay, freshReplayErr := h.chat.Send(ctx, author, channel, chat.Draft{Body: "Replay me", ClientKey: &key})
			lessonRequest := schedule.NewLesson{RelationshipID: &card, Title: "Replay lesson", Date: "2030-03-12", Start: "18:00", DurationMinutes: 60, CreateKey: &key}
			lesson, err := h.schedule.CreateLessons(ctx, actor, lessonRequest)
			if err != nil {
				t.Fatal(err)
			}
			replayedLesson, freshLessonErr := h.schedule.CreateLessons(ctx, actor, lessonRequest)
			actors := []unusableTutorActor{{kind: kind, actor: actor}}
			switch kind {
			case "identities":
				actors = h.unusableTutorIdentities(t, actor, parties.studentToken)
			case "logout":
				principal, err := h.accounts.Authenticate(ctx, parties.tutorToken)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.accounts.Logout(ctx, principal); err != nil {
					t.Fatal(err)
				}
			case "restore":
				principal, err := h.accounts.Authenticate(ctx, parties.tutorToken)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.accounts.DeleteAccount(ctx, principal, accountPassword); err != nil {
					t.Fatal(err)
				}
				if _, err := h.operators().Restore(ctx, tutorEmail); err != nil {
					t.Fatal(err)
				}
			case "reset":
				h.forgot(t, tutorEmail)
				h.tick(t)
				token := latestToken(t, h.mailTo(t, tutorEmail), resetSubject)
				if _, err := h.accounts.ResetPassword(ctx, token, "replacement tutor password"); err != nil {
					t.Fatal(err)
				}
			}

			var observations []tutorGuardObservation

			for _, unusable := range actors {
				observations = append(observations, h.observeTutorGuards(t, unusable, channel, lessonRequest, key))
			}

			if freshReplayErr != nil || replay.ID != message.ID {
				t.Fatalf("fresh replay: %v", freshReplayErr)
			}
			if freshLessonErr != nil || len(replayedLesson.Lessons) != 1 || replayedLesson.Lessons[0].ID != lesson.Lessons[0].ID {
				t.Fatalf("fresh lesson replay: %v", freshLessonErr)
			}
			for _, observation := range observations {
				observation.assertRefused(t)
			}
		})
	}
}

func TestTutorLessonExecutionRechecksSessionAndRetainsCommittedAdmission(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "edit", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			actor := h.authenticatedTutor(t, token)
			principal, err := h.accounts.Authenticate(context.Background(), token)
			if err != nil {
				t.Fatal(err)
			}
			card := parsedSessionResource[domain.RelationshipID](t, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
			request := schedule.NewLesson{RelationshipID: &card, Title: "Before revocation", Date: "2030-03-12", Start: "18:00", DurationMinutes: 60}
			created, err := h.schedule.CreateLessons(context.Background(), actor, request)
			if err != nil {
				t.Fatal(err)
			}
			title := "After admission"
			mutate := func() error {
				switch operation {
				case "edit":
					_, err := h.schedule.EditLesson(context.Background(), actor, created.Lessons[0].ID, schedule.LessonEdit{Scope: domain.ScopeThis, Version: 1, Title: &title})
					return err
				case "cancel":
					_, err := h.schedule.CancelLesson(context.Background(), actor, created.Lessons[0].ID, schedule.LessonCancellation{Scope: domain.ScopeThis, Version: 1})
					return err
				default:
					request.Date = "2030-03-13"
					_, err := h.schedule.CreateLessons(context.Background(), actor, request)
					return err
				}
			}
			attempts := h.countRows(t, "SELECT coalesce(sum(count),0) FROM rate_limit_counters WHERE purpose=$1", string(domain.PurposeLessonWrites))
			release := h.holdTutorProfile(t, tutor)
			done := make(chan error, 1)
			go func() { done <- mutate() }()
			h.waitForLockWaiters(t, 1)
			between := h.queueRowHolder(t, "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id=$1::uuid FOR NO KEY UPDATE", tutor.String())
			h.waitForLockWaiters(t, 2)
			release()
			h.awaitHolder(t, between)
			h.waitForLockWaiters(t, 1)
			before := h.tutorBusinessSnapshot(t)
			admitted := h.countRows(t, "SELECT coalesce(sum(count),0) FROM rate_limit_counters WHERE purpose=$1", string(domain.PurposeLessonWrites))

			if err := h.accounts.Logout(context.Background(), principal); err != nil {
				t.Fatal(err)
			}
			h.commitHolderAfter(t, between, "SELECT $1::text", tutor.String())
			var refused error
			select {
			case refused = <-done:
			case <-time.After(pauseDeadline):
				t.Fatal("lesson execution did not finish")
			}
			after := h.tutorBusinessSnapshot(t)

			if !errors.Is(refused, schedule.ErrUnauthenticated) {
				t.Fatalf("execution after logout returned %v", refused)
			}
			if admitted != attempts+1 || before != after {
				t.Fatal("the admitted attempt was lost or revoked execution changed business state")
			}
		})
	}
}

func TestTutorMessageReplaysAndSessionRefusalsDoNotUseWriteSlots(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.directTutorSessionParties(t)
	actor := h.authenticatedTutor(t, parties.tutorToken)
	secondToken := accessToken(t, h.login(t, tutorEmail, accountPassword))
	revoked, err := h.accounts.Authenticate(context.Background(), secondToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.accounts.Logout(context.Background(), revoked); err != nil {
		t.Fatal(err)
	}
	channel := parsedSessionResource[domain.ChannelID](t, h.directChannel(t, parties.card.ID).ID)
	active := chat.Author{UserID: actor.UserID, SessionID: actor.SessionID, Role: domain.RoleTutor}
	missing := active
	missing.SessionID = domain.SessionID{99}
	old := active
	old.SessionID = revoked.SessionID
	key := "stable-replay-key"
	draft := chat.Draft{Body: "Original", ClientKey: &key}
	original, err := h.chat.Send(context.Background(), active, channel, draft)
	if err != nil {
		t.Fatal(err)
	}

	var wrongReplay bool
	for range 35 {
		replay, err := h.chat.Send(context.Background(), active, channel, draft)
		if err != nil || replay.ID != original.ID {
			wrongReplay = true
		}
		for _, refused := range []chat.Author{missing, old} {
			if _, err := h.chat.Send(context.Background(), refused, channel, draft); !errors.Is(err, chat.ErrUnauthenticated) {
				wrongReplay = true
			}
		}
	}
	var writeErrors []error
	for range 29 {
		_, err := h.chat.Send(context.Background(), active, channel, chat.Draft{Body: "Another message"})
		writeErrors = append(writeErrors, err)
	}
	_, limited := h.chat.Send(context.Background(), active, channel, chat.Draft{Body: "Beyond thirty"})

	if wrongReplay {
		t.Fatal("a fresh replay failed or an unusable session replayed a message")
	}
	for _, err := range writeErrors {
		if err != nil {
			t.Fatalf("replays or refusals consumed a write slot: %v", err)
		}
	}
	var throttled *chat.ThrottledError
	if !errors.As(limited, &throttled) {
		t.Fatalf("the thirty-first write returned %v", limited)
	}
}

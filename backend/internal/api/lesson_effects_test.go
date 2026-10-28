package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

func TestEndingCancelsFutureOneToOneLessonsBeforeTheRelationshipEnds(t *testing.T) {
	t.Parallel()
	endings := map[string]func(t *testing.T, h *authHarness, tutorToken string, card string) response{
		"by the tutor": func(t *testing.T, h *authHarness, tutorToken string, card string) response {
			t.Helper()
			return h.endAsTutor(t, tutorToken, card)
		},
		"by the student": func(t *testing.T, h *authHarness, _ string, card string) response {
			t.Helper()
			return h.stopStudying(t, h.studentToken(t, 10), card)
		},
	}
	for name, end := range endings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
			started := h.lessonAt(t, token, card, firstLessonDate, "12:00")
			series := h.mondays(t, token, card, 3)
			oneOff := h.lessonAt(t, token, card, "2030-03-12", "18:00")
			h.at(t, time.Date(2030, time.March, 11, 9, 30, 0, 0, time.UTC))
			fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))

			result := end(t, h, fresh, card)

			if result.status != http.StatusOK {
				t.Fatalf("the ending answered %d %s", result.status, result.body)
			}
			h.cancelledBy(t, append(lessonIDs(series.Lessons), oneOff), "relationship_ended", h.endedAtOf(t, card), domain.UserID{10})
			if stored := h.storedLesson(t, started); stored.Status != "scheduled" || stored.Cause != nil {
				t.Fatalf("the lesson under way is stored as %+v", stored)
			}
			if ended := h.storedSeries(t, series.Series.ID); ended.EndDate == nil || *ended.EndDate != "2030-03-11" || ended.Count != nil || ended.Ended {
				t.Fatalf("the series is stored as %+v", ended)
			}
			if page := decodeLesson(t, h.lessonPage(t, h.studentToken(t, 10), oneOff)); page.Status != "cancelled" || page.CancelledAt == nil {
				t.Fatalf("the student sees the cancelled lesson as %+v", page)
			}
			if h.countLogs(`"msg":"lessons.cancelled_by_effect"`, `"cause":"relationship_ended"`, `"count":4`) != 1 || strings.Contains(h.logs.String(), lessonTitle) {
				t.Fatalf("the cancellations were reported as:\n%s", h.logs.String())
			}
		})
	}
}

func TestEndingRemovesTheStudentFromFutureGroupLessonsWithoutRowChanges(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0].ID
	before := h.storedLesson(t, groupLesson)
	frames := h.recordFrames(t)

	ended := h.endAsTutor(t, parties.tutorToken, parties.card)
	sent := frames.settle(t, h)

	if ended.status != http.StatusOK {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}
	if after := h.storedLesson(t, groupLesson); after.Status != "scheduled" || after.Version != before.Version || len(h.rosterOf(t, groupLesson)) != 0 {
		t.Fatalf("the group lesson changed from %+v to %+v", before, after)
	}
	if hasFrame(sent, parties.student, domain.EntityLesson, groupLesson) || !hasFrame(sent, parties.student, domain.EntityRelationship, parties.card) {
		t.Fatalf("the former member got the frames %v", framesTo(sent, parties.student))
	}
	if gone := h.lessonPage(t, parties.studentToken, groupLesson); gone.status != http.StatusNotFound {
		t.Fatalf("the former member's group lesson answered %d %s", gone.status, gone.body)
	}
	if kept := h.lessonPage(t, h.studentToken(t, 11), groupLesson); kept.status != http.StatusOK {
		t.Fatalf("the remaining member's group lesson answered %d %s", kept.status, kept.body)
	}
}

func TestTheEndedEmailListsTheCancelledLessons(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	other := h.linkedCard(t, tutor, 20, domain.RelationshipActive)
	h.mondays(t, token, card, 23)
	h.lessonAt(t, token, other, "2030-03-12", "18:00")
	if ended := h.endAsTutor(t, token, card); ended.status != http.StatusOK {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}

	h.tick(t)

	email := onlyMessage(t, h.mailTo(t, memberEmail(10)), "Your lessons with a tutor on OVC have ended")
	listed := listedLessons(email)
	if len(listed) != 20 || listed[0] != lessonTitle+", Monday, 11 March 2030, 18:00" || listed[19] != lessonTitle+", Monday, 22 July 2030, 18:00" {
		t.Fatalf("the email lists %v", listed)
	}
	if !strings.Contains(email.Text, "and 3 more") || strings.Count(email.HTML, "<li>") != 20 || !strings.Contains(email.HTML, "These lessons were cancelled:") {
		t.Fatalf("the email reads:\n%s\n%s", email.Text, email.HTML)
	}
}

func TestRemovingAProvisionalStudentCancelsSilently(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	link := seededProvisionalLink(t, h)
	series := h.mondays(t, link.tutorToken, link.card, 3)
	oneOff := h.lessonAt(t, link.tutorToken, link.card, "2030-03-12", "18:00")
	removedAt := h.now(t)

	removed := h.removeStudent(t, link.tutorToken, link.card)
	h.tick(t)

	if removed.status != http.StatusOK {
		t.Fatalf("the removal answered %d %s", removed.status, removed.body)
	}
	h.cancelledBy(t, append(lessonIDs(series.Lessons), oneOff), "provisional_removed", removedAt)
	if ended := h.storedSeries(t, series.Series.ID); !ended.Ended || ended.EndDate == nil || *ended.EndDate != "2030-03-10" {
		t.Fatalf("the series is stored as %+v", ended)
	}
	if queued, rows := h.effectEmailsQueued(t), h.notificationRows(t, link.student); queued != 0 || rows != 0 || len(h.mailTo(t, memberEmail(provisionalStudent))) != 0 {
		t.Fatalf("the silent removal queued %d emails and %d rows", queued, rows)
	}
	if h.countLogs(`"msg":"lessons.cancelled_by_effect"`, `"cause":"provisional_removed"`, `"count":4`) != 1 {
		t.Fatalf("the cancellations were reported as:\n%s", h.logs.String())
	}
}

func TestADroppedReclaimLinkTheCleanupAndATutorDeletionCancelLikeARemoval(t *testing.T) {
	t.Parallel()
	for name, path := range discardPaths() {
		if name == "a removal" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			link := path.link(t, h)
			series := h.effectSeries(t, link.tutorToken, link.card)

			path.discard(t, h, link)

			for _, lesson := range lessonIDs(series.Lessons) {
				if stored := h.storedLesson(t, lesson); stored.Status != "cancelled" || stored.Cause == nil || *stored.Cause != "provisional_removed" {
					t.Fatalf("lesson %s is stored as %+v", lesson, stored)
				}
			}
			if ended := h.storedSeries(t, series.Series.ID); !ended.Ended {
				t.Fatalf("the series is stored as %+v", ended)
			}
			if rows, queued := h.rosterRowsOf(t, link.student), h.countRows(t, "SELECT count(id) FROM outbox WHERE kind LIKE $1", "lesson.%"); rows != 0 || queued != 0 {
				t.Fatalf("the discarded link keeps %d roster rows and queued %d lesson emails", rows, queued)
			}
			if h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventRelationshipEnded)) != 0 {
				t.Fatal("the discarded link queued an ended email")
			}
		})
	}
}

func TestARemovedProvisionalAccountNoLongerSeesItsLessons(t *testing.T) {
	t.Parallel()
	for name, path := range discardPaths() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			link := path.link(t, h)
			cancelled := h.lessonAt(t, link.tutorToken, link.card, "2030-03-12", "18:00")
			ended := h.lessonAt(t, link.tutorToken, link.card, "2030-03-10", "13:00")
			future := h.effectSeries(t, link.tutorToken, link.card)
			if result := h.cancelCall(t, link.tutorToken, cancelled, "this", 1); result.status != http.StatusOK {
				t.Fatalf("the cancellation answered %d %s", result.status, result.body)
			}
			h.tickAt(t, time.Date(2030, time.March, 10, 11, 31, 0, 0, time.UTC))
			link.tutorToken = accessToken(t, h.login(t, tutorEmail, accountPassword))
			if h.lessonState(t, ended) != "ended" || h.rosterRowsOf(t, link.student) != 2 {
				t.Fatalf("the seed holds the lesson %s and %d roster rows", h.lessonState(t, ended), h.rosterRowsOf(t, link.student))
			}

			token := path.discard(t, h, link)

			if rows := h.rosterRowsOf(t, link.student); rows != 0 {
				t.Fatalf("the discarded link keeps %d roster rows", rows)
			}
			if token == "" {
				return
			}
			if listed := h.calendarOf(t, token, calendarFrom, calendarTo); len(listed.Items) != 0 {
				t.Fatalf("the former provisional account lists %v", calendarIDs(listed))
			}
			for _, lesson := range append([]string{cancelled, ended}, lessonIDs(future.Lessons)...) {
				if page := h.lessonPage(t, token, lesson); page.status != http.StatusNotFound {
					t.Fatalf("lesson %s answered %d %s", lesson, page.status, page.body)
				}
			}
		})
	}
}

func TestArchivingCancelsAndEndsAndSendsOneEmailPerMember(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 3)
	group := h.newGroup(t, token, archivedGroupName, cards[:2]).ID
	series := h.groupMondays(t, token, group, 3)
	oneOff := h.scheduled(t, token, groupLessonFor(group, "2030-03-12", "18:00")).Lessons[0].ID
	byTutor := h.scheduled(t, token, groupLessonFor(group, "2030-03-13", "18:00")).Lessons[0].ID
	if result := h.cancelCall(t, token, byTutor, "this", 1); result.status != http.StatusOK {
		t.Fatalf("the cancellation answered %d %s", result.status, result.body)
	}

	archived := h.archive(t, token, group)
	repeated := h.archive(t, token, group)
	h.tick(t)

	if archived.status != http.StatusOK || repeated.status != http.StatusOK {
		t.Fatalf("archiving answered %d %s and %d %s", archived.status, archived.body, repeated.status, repeated.body)
	}
	members := []domain.UserID{{11}, {12}}
	h.cancelledBy(t, append(lessonIDs(series.Lessons), oneOff), "group_archived", h.archivedAtOf(t, group), members...)
	if kept := h.storedLesson(t, byTutor); kept.Cause == nil || *kept.Cause != "tutor" {
		t.Fatalf("the lesson the tutor cancelled is stored as %+v", kept)
	}
	if ended := h.storedSeries(t, series.Series.ID); !ended.Ended {
		t.Fatalf("the series is stored as %+v", ended)
	}
	for _, member := range []byte{11, 12} {
		emails := h.emailsWithSubject(t, memberEmail(member), "Group archived: "+archivedGroupName)
		if len(emails) != 1 || len(listedLessons(emails[0])) != 4 || emails[0].Headers["List-Unsubscribe"] == "" {
			t.Fatalf("member %d got %v", member, subjects(h.mailTo(t, memberEmail(member))))
		}
		var rows []notificationItem
		for _, row := range h.notificationsOf(t, h.studentToken(t, member)) {
			if row.Event == "group.archived" {
				rows = append(rows, row)
			}
		}
		if len(rows) != 1 || rows[0].Label == nil || *rows[0].Label != archivedGroupName || rows[0].Link != nil {
			t.Fatalf("member %d has the rows %+v", member, rows)
		}
	}
	if len(h.mailTo(t, memberEmail(13))) != 0 || h.notificationRows(t, tutor) != 0 || h.outboxKeysOf(t, domain.EventGroupArchived) != 2 {
		t.Fatal("archiving reached someone outside its members")
	}
	if h.countLogs(`"msg":"lessons.cancelled_by_effect"`, `"cause":"group_archived"`, `"count":4`) != 1 {
		t.Fatalf("the cancellations were reported as:\n%s", h.logs.String())
	}
}

func TestArchivingAndAnEndingNeverDeadlock(t *testing.T) {
	t.Parallel()
	t.Run("archiving first", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		parties := h.visibilityParties(t)
		groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0].ID
		release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM groups WHERE id = $1::uuid FOR UPDATE", parties.group)
		archiving := h.sendInBackground(http.MethodPost, groupsRoute+"/"+parties.group+"/archive", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 1)
		ending := h.sendInBackground(http.MethodPost, tutorsPath+parties.card+"/end", "", jsonCall(parties.studentToken))
		h.waitForLockWaiters(t, 2)

		release()
		answers := []response{awaitAnswer(t, archiving), awaitAnswer(t, ending)}

		for _, answer := range answers {
			if answer.status != http.StatusOK {
				t.Fatalf("a write answered %d %s", answer.status, answer.body)
			}
		}
		h.cancelledBy(t, []string{groupLesson}, "group_archived", h.archivedAtOf(t, parties.group), parties.student, parties.otherStudent)
	})
	t.Run("ending first", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		parties := h.visibilityParties(t)
		groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0].ID
		ownLesson := h.lessonAt(t, parties.tutorToken, parties.card, "2030-03-13", "18:00")
		release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM lessons WHERE id = $1::uuid FOR UPDATE", ownLesson)
		ending := h.sendInBackground(http.MethodPost, tutorsPath+parties.card+"/end", "", jsonCall(parties.studentToken))
		h.waitForLockWaiters(t, 1)
		archiving := h.sendInBackground(http.MethodPost, groupsRoute+"/"+parties.group+"/archive", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 2)

		release()
		answers := []response{awaitAnswer(t, ending), awaitAnswer(t, archiving)}

		for _, answer := range answers {
			if answer.status != http.StatusOK {
				t.Fatalf("a write answered %d %s", answer.status, answer.body)
			}
		}
		h.cancelledBy(t, []string{ownLesson}, "relationship_ended", h.endedAtOf(t, parties.card), parties.student)
		h.cancelledBy(t, []string{groupLesson}, "group_archived", h.archivedAtOf(t, parties.group), parties.otherStudent)
	})
}

func TestAReactivatedAndEndedAgainRelationshipListsOnlyItsOwnCancellations(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.lessonAt(t, token, card, "2030-03-12", "18:00")
	h.endAsTutor(t, token, card)
	h.tick(t)
	h.exec(t, "UPDATE relationships SET state = 'active', ended_at = NULL, ended_by = NULL WHERE id = $1::uuid", card)
	h.at(t, authStart.Add(time.Hour))
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.lessonAt(t, fresh, card, "2030-03-13", "18:00")

	if ended := h.endAsTutor(t, fresh, card); ended.status != http.StatusOK {
		t.Fatalf("the second ending answered %d %s", ended.status, ended.body)
	}
	h.tick(t)

	emails := h.emailsWithSubject(t, memberEmail(10), "Your lessons with a tutor on OVC have ended")
	if len(emails) != 2 {
		t.Fatalf("the student got %v", subjects(h.mailTo(t, memberEmail(10))))
	}
	slices.SortFunc(emails, func(first, second mail.SinkMessage) int { return first.SentAt.Compare(second.SentAt) })
	first, second := listedLessons(emails[0]), listedLessons(emails[1])
	if len(first) != 1 || !strings.Contains(first[0], "12 March") || len(second) != 1 || !strings.Contains(second[0], "13 March") {
		t.Fatalf("the emails list %v and %v", first, second)
	}
}

func TestUnarchivingRestoresNothingScheduled(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 2)
	group := h.newGroup(t, token, archivedGroupName, cards).ID
	request := groupLessonFor(group, firstLessonDate, "20:00").until([]int{1}, "2030-12-30")
	series := h.scheduled(t, token, request)
	before := h.lessonRows(t, tutor)
	h.archive(t, token, group)

	unarchived := h.groupCall(t, token, http.MethodPost, group, "/unarchive", "")
	h.tickAt(t, authStart.Add(2*time.Hour))
	h.tickAt(t, authStart.Add(7*24*time.Hour))

	if unarchived.status != http.StatusOK {
		t.Fatalf("unarchiving answered %d %s", unarchived.status, unarchived.body)
	}
	if after := h.lessonRows(t, tutor); after != before || before == 0 {
		t.Fatalf("the tutor had %d lessons and has %d", before, after)
	}
	if scheduled := h.countRows(t, "SELECT count(id) FROM lessons WHERE group_id = $1::uuid AND status = 'scheduled'", group); scheduled != 0 {
		t.Fatalf("%d lessons are scheduled again", scheduled)
	}
	if ended := h.storedSeries(t, series.Series.ID); !ended.Ended {
		t.Fatalf("the series is stored as %+v", ended)
	}
}

func TestTutorDeletionCancelsWithoutLockingAStudentRow(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	oneToOne := h.lessonAt(t, parties.tutorToken, parties.card, "2030-03-12", "18:00")
	groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0].ID
	release := h.holdUserRow(t, parties.student)

	deleted := awaitAnswer(t, h.deleteInBackground(parties.tutorToken, accountPassword))
	release()
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	deletedAt := h.deletedAtOf(t, parties.tutor)
	h.cancelledBy(t, []string{oneToOne}, "tutor_deleted", deletedAt, parties.student)
	h.cancelledBy(t, []string{groupLesson}, "tutor_deleted", deletedAt, parties.student, parties.otherStudent)
	for member, want := range map[byte]int{10: 2, 11: 1} {
		messages := h.mailTo(t, memberEmail(member))
		if len(messages) != 1 || messages[0].Subject != "A tutor closed their OVC account" || len(listedLessons(messages[0])) != want {
			t.Fatalf("member %d got %v", member, subjects(messages))
		}
	}
	if others := h.effectEmailsQueued(t) - h.outboxKeysOf(t, domain.EventTutorDeleted); others != 0 {
		t.Fatalf("the deletion queued %d other emails", others)
	}
}

func TestStudentDeletionEndsWithoutEmails(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.mondays(t, token, card, 2)
	oneOff := h.lessonAt(t, token, card, "2030-03-12", "18:00")

	deleted := h.deleteMe(t, h.studentToken(t, 10), accountPassword)
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	h.cancelledBy(t, append(lessonIDs(series.Lessons), oneOff), "relationship_ended", h.endedAtOf(t, card), domain.UserID{10})
	if queued := h.effectEmailsQueued(t); queued != 0 || len(h.mailTo(t, tutorEmail)) != 0 || h.notificationRows(t, tutor) != 0 {
		t.Fatalf("the student's deletion queued %d emails", queued)
	}
}

func TestRosterInsertsRespectAConcurrentStudentPurge(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	link := seededProvisionalLink(t, h)
	lesson := h.lessonAt(t, link.tutorToken, link.card, effectSeriesDate, "18:00")
	h.at(t, authStart.Add(8*24*time.Hour))
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	release := h.holdTutorProfile(t, link.tutor)
	cleanup := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	cancellation := h.sendInBackground(http.MethodPost, cancelPath(lesson), cancelBody("this", 1), jsonCall(fresh))
	h.waitForLockWaiters(t, 2)

	release()
	awaitTick(t, cleanup)
	cancelled := awaitAnswer(t, cancellation)

	if cancelled.status != http.StatusOK || h.userExists(t, memberEmail(provisionalStudent)) {
		t.Fatalf("the cancellation answered %d %s after the purge", cancelled.status, cancelled.body)
	}
	if stored := h.storedLesson(t, lesson); stored.Cause == nil || *stored.Cause != "provisional_removed" || len(h.rosterOf(t, lesson)) != 0 {
		t.Fatalf("the lesson is stored as %+v with the roster %v", stored, h.rosterOf(t, lesson))
	}
}

func TestPurgeDeletesLessonNotificationRows(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.visibilityParties(t)
	lesson := h.lessonAt(t, parties.tutorToken, parties.card, "2030-03-12", "18:00")
	h.cancelCall(t, parties.tutorToken, lesson, "this", 1)
	h.archive(t, parties.tutorToken, parties.group)
	seeded := [2]int64{h.rowsNaming(t, parties.student), h.rowsNaming(t, parties.otherStudent)}
	h.deleteMe(t, parties.studentToken, accountPassword)
	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)
	afterStudent := [2]int64{h.rowsNaming(t, parties.student), h.rowsNaming(t, parties.otherStudent)}
	h.deleteMe(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if seeded != [2]int64{2, 1} || afterStudent != [2]int64{0, 1} {
		t.Fatalf("the rows naming the students were %v and then %v", seeded, afterStudent)
	}
	if h.userExists(t, tutorEmail) || h.rowsNaming(t, parties.tutor) != 0 || h.rowsNaming(t, parties.otherStudent) != 0 {
		t.Fatalf("the purged tutor is still named by %d rows", h.rowsNaming(t, parties.tutor))
	}
}

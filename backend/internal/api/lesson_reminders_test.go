package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

var errSendRefused = errors.New("the provider refused the send for now")

func TestRemindersAreQueuedOncePerRevisionInTheirWindows(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)

	h.tickAt(t, firstLessonStart.Add(-24*time.Hour).Add(-15*time.Minute))
	beforeTheWindow := h.outboxOf(t, domain.EventLessonReminder24h, "done:sent")
	h.tickAt(t, dayBeforeFirst)
	h.tick(t)
	h.tickAt(t, dayBeforeFirst.Add(time.Minute))
	dayBefore := h.outboxOf(t, domain.EventLessonReminder24h, "done:sent")
	rowsAfterTheDayBefore := h.countRows(t, "SELECT count(id) FROM notifications WHERE event = $1", string(domain.NotificationLessonReminder1h))
	h.tickAt(t, hourBeforeFirst)
	h.tickAt(t, hourBeforeFirst.Add(time.Minute))

	if beforeTheWindow != 0 || dayBefore != 2 || rowsAfterTheDayBefore != 0 {
		t.Fatalf("before the window %d, the day before %d emails and %d rows", beforeTheWindow, dayBefore, rowsAfterTheDayBefore)
	}
	if hourBefore := h.outboxOf(t, domain.EventLessonReminder1h, "done:sent"); hourBefore != 2 {
		t.Fatalf("%d one-hour reminders were sent", hourBefore)
	}
	if len(h.remindersFor(t, tutorEmail, domain.EventLessonReminder24h)) != 1 || len(h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)) != 1 {
		t.Fatalf("the sink holds %v and %v", h.mailTo(t, tutorEmail), h.mailTo(t, "member-10@example.org"))
	}
	if rows := h.countRows(t, "SELECT count(id) FROM notifications WHERE event = $1", string(domain.NotificationLessonReminder1h)); rows != 2 {
		t.Fatalf("%d one-hour rows were written", rows)
	}
	if h.countLogs(`"msg":"lesson.reminders_queued"`, `"lessons":1`, `"recipients":2`) != 2 {
		t.Fatalf("the reminder job reported:\n%s", h.logs.String())
	}
}

func (h *authHarness) burstOf(t *testing.T, lesson string, copies int) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO lessons (id, tutor_id, relationship_id, group_id, title, starts_at, duration_minutes, status, timezone, created_at, updated_at)
SELECT gen_random_uuid(), tutor_id, relationship_id, group_id, title, starts_at, duration_minutes, status, timezone, created_at, updated_at
FROM lessons CROSS JOIN generate_series(1, $2::int)
WHERE id = $1::uuid`, lesson, copies)
	if err != nil {
		t.Fatal(err)
	}
}

func TestABurstOfLessonsIsWorkedThroughWithinTheBudget(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.burstOf(t, h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal), 120)
	dayAhead := h.lessonAt(t, token, card, "2030-03-12", "17:00")
	h.withJobSettings(oneReminderBatchSettings())

	h.tickAt(t, hourBeforeFirst)
	withinOneBatch := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND reminded_1h_revision = 1", tutor.String())
	dayAheadInTheFirstRun := h.countRows(t, "SELECT count(id) FROM lessons WHERE id = $1::uuid AND reminded_24h_revision = 1", dayAhead)
	h.withJobSettings(scheduleDefaults())
	h.tickAt(t, hourBeforeFirst.Add(time.Minute))
	h.tickAt(t, hourBeforeFirst.Add(2*time.Minute))

	if withinOneBatch != 50 || dayAheadInTheFirstRun != 0 {
		t.Fatalf("the budget's one batch reminded %d one-hour lessons and %d day-ahead lessons", withinOneBatch, dayAheadInTheFirstRun)
	}
	if reminded := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND reminded_1h_revision = 1", tutor.String()); reminded != 121 {
		t.Fatalf("%d of 121 lessons were reminded", reminded)
	}
	if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventLessonReminder1h)); queued != 242 {
		t.Fatalf("%d one-hour reminders were queued for 121 lessons", queued)
	}
	if dayAheadLater := h.countRows(t, "SELECT count(id) FROM lessons WHERE id = $1::uuid AND reminded_24h_revision = 1", dayAhead); dayAheadLater != 1 {
		t.Fatal("the day-ahead lesson was never reminded after the burst")
	}
	if h.countLogs(`"msg":"lesson.reminders_queued"`, `"lessons":71`) != 1 {
		t.Fatalf("the second run reported:\n%s", h.logs.String())
	}
}

func TestRemindersOfAMovedLessonBecomeObsolete(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	h.at(t, hourBeforeFirst)
	h.queueReminders(t)
	h.exec(t, `UPDATE lessons SET starts_at = starts_at + interval '2 hours', schedule_revision = 2, version = 2 WHERE id = $1::uuid`, lesson)

	h.tick(t)
	obsolete := h.outboxOf(t, domain.EventLessonReminder1h, "done:obsolete")
	h.tickAt(t, hourBeforeFirst.Add(2*time.Hour))

	if obsolete != 2 || len(h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)) != 1 {
		t.Fatalf("%d reminders of the old time became obsolete and the student got %v", obsolete, h.mailTo(t, "member-10@example.org"))
	}
	if sent := h.outboxOf(t, domain.EventLessonReminder1h, "done:sent"); sent != 2 {
		t.Fatalf("%d reminders of the new time were sent", sent)
	}
}

func TestAReminderLaterThanFifteenMinutesIsObsolete(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	h.at(t, hourBeforeFirst)
	h.queueReminders(t)

	h.tickAt(t, firstLessonStart.Add(-time.Hour).Add(15*time.Minute).Add(time.Second))

	if obsolete := h.outboxOf(t, domain.EventLessonReminder1h, "done:obsolete"); obsolete != 2 || len(h.mailTo(t, "member-10@example.org")) != 0 {
		t.Fatalf("%d late reminders became obsolete", obsolete)
	}
}

func TestOneHourReminderCreatesAnInAppRow(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	group := h.newGroup(t, token, "Evening club", h.activeCards(t, tutor, 11, 2))
	h.linkedCard(t, tutor, 13, domain.RelationshipActive)
	lesson := h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, firstLessonLocal)).Lessons[0].ID

	h.tickAt(t, hourBeforeFirst)

	member := h.notificationsOf(t, h.studentToken(t, 11))
	own := h.notificationsOf(t, accessToken(t, h.login(t, tutorEmail, accountPassword)))
	if len(member) != 1 || member[0].Event != "lesson.reminder_1h" || member[0].Label == nil || *member[0].Label != lessonTitle ||
		member[0].Colour == nil || member[0].Link == nil || member[0].Link.Kind != "lesson" || member[0].Link.ID != lesson {
		t.Fatalf("the member's rows are %+v", member)
	}
	if len(own) != 1 || own[0].Label == nil || *own[0].Label != lessonTitle || own[0].Colour != nil || own[0].Link == nil || own[0].Link.ID != lesson {
		t.Fatalf("the tutor's rows are %+v", own)
	}
	if outsider := h.notificationsOf(t, h.studentToken(t, 13)); len(outsider) != 0 {
		t.Fatalf("a student outside the group got %+v", outsider)
	}
	if h.countLogs(`"msg":"notification.created"`, `"event":"lesson.reminder_1h"`) != 3 {
		t.Fatalf("the rows were reported as:\n%s", h.logs.String())
	}
}

func TestAReminderToAFormerGroupMemberIsDroppedAtDispatch(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 2)
	group := h.newGroup(t, token, "Evening club", cards)
	h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, firstLessonLocal))
	h.at(t, hourBeforeFirst)
	h.queueReminders(t)
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	if removed := h.groupCall(t, fresh, http.MethodPatch, group.ID, "", `{"remove":`+idList(cards[:1])+`}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}

	h.tick(t)

	if len(h.mailTo(t, "member-11@example.org")) != 0 || len(h.remindersFor(t, "member-12@example.org", domain.EventLessonReminder1h)) != 1 {
		t.Fatalf("the former member got %v", h.mailTo(t, "member-11@example.org"))
	}
	if h.outboxOf(t, domain.EventLessonReminder1h, "done:obsolete") != 1 || h.outboxOf(t, domain.EventLessonReminder1h, "done:sent") != 2 {
		t.Fatal("the former member's reminder was not the one dropped")
	}
}

func TestALessonEmailAfterAnEndingOrAReclaimIsDroppedAtDispatch(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	ended := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	reclaimed := h.linkedCard(t, tutor, 20, domain.RelationshipActive)
	endedLesson := h.lessonAt(t, token, ended, firstLessonDate, firstLessonLocal)
	h.lessonAt(t, token, reclaimed, firstLessonDate, firstLessonLocal)
	h.at(t, hourBeforeFirst)
	h.queueReminders(t)
	if result := h.endAsTutor(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), ended); result.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", result.status, result.body)
	}
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", reclaimed)

	h.tick(t)

	if len(h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)) != 0 || len(h.mailTo(t, "member-20@example.org")) != 0 {
		t.Fatal("a student outside the audience got a reminder")
	}
	if h.lessonState(t, endedLesson) != "cancelled" || h.outboxOf(t, domain.EventLessonReminder1h, "done:obsolete") != 3 ||
		len(h.remindersFor(t, tutorEmail, domain.EventLessonReminder1h)) != 1 {
		t.Fatalf("the tutor got %d reminders", len(h.remindersFor(t, tutorEmail, domain.EventLessonReminder1h)))
	}
}

func TestLessonEmailsToStudentsAreDroppedWhileTheTutorIsBlocked(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	during := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
	after := h.lessonAt(t, token, card, firstLessonDate, "20:00")
	h.at(t, hourBeforeFirst)
	h.queueReminders(t)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	h.tick(t)
	whileBlocked := len(h.mailTo(t, "member-10@example.org"))
	if _, err := h.operators().Unblock(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	h.tickAt(t, hourBeforeFirst.Add(2*time.Hour))

	if whileBlocked != 0 || h.countLogs(`"msg":"email.dropped"`, `"reason":"tutor_inactive"`, `"group":"lesson_reminders"`) != 1 {
		t.Fatalf("while the tutor was blocked the student got %d emails:\n%s", whileBlocked, h.logs.String())
	}
	if len(h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)) != 1 || h.lessonState(t, during) != "scheduled" ||
		h.lessonState(t, after) != "scheduled" {
		t.Fatalf("after the unblock the student got %v", h.mailTo(t, "member-10@example.org"))
	}
}

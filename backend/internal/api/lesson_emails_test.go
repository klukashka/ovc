package api_test

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func (h *authHarness) remindedPair(t *testing.T) (domain.UserID, string) {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	return tutor, token
}

func TestOptionalEmailsCarryOneClickHeadersAndAVisibleLink(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.remindedPair(t)
	h.tickAt(t, hourBeforeFirst)
	if ended := h.endAsTutor(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), h.cardIDOf(t, 10)); ended.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}

	h.tick(t)

	for _, recipient := range []string{tutorEmail, "member-10@example.org"} {
		reminders := h.remindersFor(t, recipient, domain.EventLessonReminder1h)
		if len(reminders) != 1 {
			t.Fatalf("%s got %d reminders", recipient, len(reminders))
		}
		reminder := reminders[0]
		token := headerToken(t, reminder)
		page := publicOrigin + "/unsubscribe?token=" + url.QueryEscape(token)
		if reminder.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || !strings.Contains(reminder.Text, "Turn off these emails: "+page) {
			t.Fatalf("the reminder to %s has the headers %v and the text %q", recipient, reminder.Headers, reminder.Text)
		}
		if !strings.Contains(reminder.HTML, `href="`+strings.ReplaceAll(page, "&", "&amp;")+`"`) || !containsLink(reminder.Links, page) {
			t.Fatalf("the reminder to %s links to %v", recipient, reminder.Links)
		}
	}
	for _, message := range h.mailTo(t, "member-10@example.org") {
		if strings.HasPrefix(message.Subject, "Your lessons with a tutor") && message.Headers["List-Unsubscribe"] != "" {
			t.Fatalf("a required email carries %v", message.Headers)
		}
	}
}

func containsLink(links []string, wanted string) bool {
	for _, link := range links {
		if link == wanted {
			return true
		}
	}
	return false
}

func (h *authHarness) cardIDOf(t *testing.T, student byte) string {
	t.Helper()
	var card string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT id::text FROM relationships WHERE student_id = $1", domain.UserID{student}).Scan(&card)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func TestAnUnsubscribeLinkFromAnEmailTurnsOffItsGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.remindedPair(t)
	h.tickAt(t, hourBeforeFirst)
	token := headerToken(t, h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)[0])

	answer := h.unsubscribe(t, token, formType, oneClickForm)

	if answer.status != http.StatusNoContent {
		t.Fatalf("the one-click unsubscribe answered %d %s", answer.status, answer.body)
	}
	if stored := h.storedPreferences(t, domain.UserID{10}); len(stored) != 1 || stored[0] != "lesson_reminders:false" {
		t.Fatalf("the unsubscribe stored %v", stored)
	}
}

func TestATurnedOffGroupDropsTheEmailAtDispatch(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.remindedPair(t)
	student := h.studentToken(t, 10)
	if saved := h.savePreferences(t, student, `{"groups":[{"group":"lesson_reminders","enabled":false}]}`); saved.status != http.StatusOK {
		t.Fatalf("saving answered %d %s", saved.status, saved.body)
	}

	h.tickAt(t, hourBeforeFirst)

	if len(h.mailTo(t, "member-10@example.org")) != 0 || len(h.remindersFor(t, tutorEmail, domain.EventLessonReminder1h)) != 1 {
		t.Fatal("the turned-off group still reached the student, or the tutor's reminder was dropped")
	}
	if h.countLogs(`"msg":"email.dropped"`, `"reason":"preference"`, `"group":"lesson_reminders"`, `"event":"lesson.reminder_1h"`) != 1 {
		t.Fatalf("the drop was reported as:\n%s", h.logs.String())
	}
	if rows := h.notificationsOf(t, h.studentToken(t, 10)); len(rows) != 1 || rows[0].Event != "lesson.reminder_1h" {
		t.Fatalf("the in-app rows are %+v", rows)
	}
}

func TestRequiredEmailsIgnorePreferences(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	allOff := `{"groups":[{"group":"lesson_reminders","enabled":false},{"group":"lesson_changes","enabled":false},` +
		`{"group":"homework","enabled":false},{"group":"results","enabled":false},{"group":"message_digest","enabled":false}]}`
	if saved := h.savePreferences(t, h.studentToken(t, 10), allOff); saved.status != http.StatusOK {
		t.Fatalf("saving answered %d %s", saved.status, saved.body)
	}
	if ended := h.endAsTutor(t, token, card); ended.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}

	h.tick(t)

	sent := h.mailTo(t, "member-10@example.org")
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Your lessons with a tutor on OVC have ended") || sent[0].Headers["List-Unsubscribe"] != "" {
		t.Fatalf("the student with every group off got %v", sent)
	}
}

func TestLessonEmailsUseTheRecipientsLanguageAndTimezone(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setAccount(t, tutor, "Мария Петрова", domain.LanguageRussian)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.setAccount(t, domain.UserID{10}, "Анна", domain.LanguageRussian)
	h.setTimezone(t, domain.UserID{10}, "Europe/Berlin")
	english := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	h.setTimezone(t, domain.UserID{11}, "Europe/Berlin")
	h.lessonAt(t, token, card, "2030-03-12", firstLessonLocal)
	h.lessonAt(t, token, english, "2030-03-12", "18:00")

	h.tickAt(t, firstLessonStart.Add(23*time.Hour).Add(time.Minute))

	tutorReminders := h.remindersFor(t, tutorEmail, domain.EventLessonReminder1h)
	anna := h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)
	other := h.remindersFor(t, "member-11@example.org", domain.EventLessonReminder1h)
	if len(tutorReminders) != 2 || len(anna) != 1 || len(other) != 1 {
		t.Fatalf("the reminders are %d, %d and %d", len(tutorReminders), len(anna), len(other))
	}
	if !strings.HasPrefix(tutorReminders[0].Subject, "Урок через 1 час: ") || !strings.Contains(tutorReminders[0].Text, "Когда: вторник, 12 марта 2030, 18:00\n") ||
		!strings.Contains(tutorReminders[0].Text, "Ученик: "+seededCardName) {
		t.Fatalf("the tutor's reminder reads %q: %q", tutorReminders[0].Subject, tutorReminders[0].Text)
	}
	if !strings.Contains(anna[0].Text, "Когда: вторник, 12 марта 2030, 16:00 по времени Берлина") ||
		!strings.Contains(anna[0].Text, "Урок: Индивидуальный урок") || !strings.Contains(anna[0].Text, "Преподаватель: Мария Петрова") ||
		strings.Contains(anna[0].Text, seededCardName) {
		t.Fatalf("Anna's reminder reads %q", anna[0].Text)
	}
	if !strings.HasPrefix(other[0].Subject, "Lesson in 1 hour: ") || !strings.Contains(other[0].Text, "When: Tuesday, 12 March 2030, 16:00 Berlin time") {
		t.Fatalf("the English reminder reads %q: %q", other[0].Subject, other[0].Text)
	}
}

const hiddenCharacters = "\u2061\u2062\u2063\u2064\u180e\u00ad\u034f\U000E0041\u115f\u1160\u3164\uffa0" +
	"\u2800\ufe00\ufe0e\U000E0100\U000E01EF\u180b\u180c\u180d\u180f\u17b4\u17b5"

func TestTitlesInSubjectsAndBodiesAreStrippedAndEncoded(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	request := oneOffFor(h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	request.Title = "Grammar\u202e <b>club</b>\u200b" + hiddenCharacters + "  \u2014 \u2066practice\u2069"
	h.scheduled(t, token, request)
	encodedWord := oneOffFor(h.linkedCard(t, tutor, 11, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	encodedWord.Title = "=?utf-8?b?4oCuZXZpbA==?="
	encodedWord.AllowOverlap = true
	h.scheduled(t, token, encodedWord)

	h.tickAt(t, hourBeforeFirst)

	reminder := h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)[0]
	if reminder.Subject != "Lesson in 1 hour: Grammar <b>club</b> \u2014 practice" || !strings.HasPrefix(reminder.Headers["Subject"], "=?utf-8?b?") {
		t.Fatalf("the subject is %q, sent as %q", reminder.Subject, reminder.Headers["Subject"])
	}
	if !strings.Contains(reminder.HTML, "Grammar &lt;b&gt;club&lt;/b&gt; \u2014 practice") || strings.Contains(reminder.HTML, "<b>club") ||
		!strings.Contains(reminder.Text, "\nGrammar <b>club</b> \u2014 practice\n") {
		t.Fatalf("the bodies are %q and %q", reminder.HTML, reminder.Text)
	}
	for _, part := range []string{reminder.Subject, reminder.Text, reminder.HTML, reminder.Headers["Subject"]} {
		if strings.ContainsAny(part, "\u202e\u200b\u2066\u2069"+hiddenCharacters) {
			t.Fatalf("an invisible formatting character reached %q", part)
		}
	}
	ascii := h.remindersFor(t, "member-11@example.org", domain.EventLessonReminder1h)[0]
	decoded, err := new(mime.WordDecoder).DecodeHeader(ascii.Headers["Subject"])
	if err != nil || !strings.HasPrefix(ascii.Headers["Subject"], "=?utf-8?b?") || decoded != "Lesson in 1 hour: =?utf-8?b?4oCuZXZpbA==?=" ||
		strings.ContainsRune(decoded, 0x202e) {
		t.Fatalf("an ASCII title that is an encoded word was sent as %q and reads %q (%v)", ascii.Headers["Subject"], decoded, err)
	}
}

func TestLessonEmailsCarryNoDescription(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	request := oneOffFor(h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	request.Description = pointer("Bring the green workbook")
	h.scheduled(t, token, request)

	h.tickAt(t, dayBeforeFirst)
	h.tickAt(t, hourBeforeFirst)

	for _, recipient := range []string{tutorEmail, "member-10@example.org"} {
		messages := h.mailTo(t, recipient)
		if len(messages) != 2 {
			t.Fatalf("%s got %d emails", recipient, len(messages))
		}
		for _, message := range messages {
			if strings.Contains(message.Text+message.HTML+message.Subject, "workbook") {
				t.Fatalf("a lesson email to %s carries the description", recipient)
			}
		}
	}
}

func TestUnsubscribeTokensInEmailsNeverReachRecords(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.remindedPair(t)

	h.tickAt(t, hourBeforeFirst)

	records := h.logs.String()
	for _, recipient := range []string{tutorEmail, "member-10@example.org"} {
		token := headerToken(t, h.remindersFor(t, recipient, domain.EventLessonReminder1h)[0])
		mac := token[strings.LastIndex(token, ".")+1:]
		if strings.Contains(records, token) || strings.Contains(records, mac) || strings.Contains(records, "token=") {
			t.Fatalf("a record carries the unsubscribe token of %s", recipient)
		}
	}
	for _, private := range []string{lessonTitle, tutorEmail, "member-10@example.org", seededCardName} {
		if strings.Contains(records, private) {
			t.Fatalf("a record carries %q", private)
		}
	}
}

func TestEachMintedTokenNamesTheRecipientAndTheEmailsGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, _ := h.remindedPair(t)

	h.tickAt(t, hourBeforeFirst)

	for recipient, user := range map[string]domain.UserID{tutorEmail: tutor, "member-10@example.org": {10}} {
		token := headerToken(t, h.remindersFor(t, recipient, domain.EventLessonReminder1h)[0])
		claims, err := h.links.Verify(token, hourBeforeFirst)
		if err != nil || claims.UserID != user || claims.Group != domain.EmailGroupLessonReminders ||
			!claims.ExpiresAt.Equal(hourBeforeFirst.Add(domain.UnsubscribeTokenLifetime).Truncate(time.Second)) {
			t.Fatalf("the token to %s names %+v (%v)", recipient, claims, err)
		}
	}
}

func TestARetriedReminderKeepsItsMessageID(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, _ := h.remindedPair(t)
	h.at(t, hourBeforeFirst)
	h.mailer.Pause()

	ticked := h.tickInBackground(t)
	if err := h.mailer.AwaitHeld(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	failed := h.mailer.Fail("member-10@example.org", errSendRefused)
	h.mailer.Release(tutorEmail)
	awaitTick(t, ticked)
	h.mailer.Unpause()
	h.tickAt(t, hourBeforeFirst.Add(time.Minute))

	lesson := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid", tutor.String())
	reminders := h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)
	if !failed || lesson != 1 || len(reminders) != 1 {
		t.Fatalf("failed %v, %d reminders after the retry", failed, len(reminders))
	}
	var key string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT idempotency_key FROM outbox WHERE kind = $1 AND attempts = 2",
		string(domain.EventLessonReminder1h)).Scan(&key)
	if err != nil {
		t.Fatal(err)
	}
	if reminders[0].MessageID != expectedMessageID(key) || reminders[0].Headers["Message-ID"] != "<"+expectedMessageID(key)+">" {
		t.Fatalf("the retried reminder is %q, not derived from %q", reminders[0].MessageID, key)
	}
}

func TestLessonEmailsStopAtTheDailyCapWhileRowsArrive(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	student := domain.UserID{10}
	lessons := map[string]string{}
	for _, start := range []string{"18:00", "18:20", "18:40", "19:00", "19:30"} {
		lessons[start] = h.lessonAt(t, token, card, firstLessonDate, start)
	}
	h.lessonAt(t, token, card, firstLessonDate, "19:00")
	h.at(t, hourBeforeFirst)
	h.presetLessonEmailSlots(t, student, 17)

	h.tick(t)
	afterASend := h.lessonEmailSlots(t, student)

	h.at(t, hourBeforeFirst.Add(20*time.Minute))
	h.queueReminders(t)
	h.exec2(t, "UPDATE lessons SET status = 'cancelled', cancelled_at = $2::timestamptz, cancel_cause = 'tutor' WHERE id = $1::uuid",
		lessons["18:20"], rfc3339(h.now(t)))
	h.tick(t)
	afterAnObsoleteRow := h.lessonEmailSlots(t, student)

	h.at(t, hourBeforeFirst.Add(40*time.Minute))
	h.mailer.Pause()
	ticked := h.tickInBackground(t)
	if err := h.mailer.AwaitHeld(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	h.mailer.Fail("member-10@example.org", errSendRefused)
	h.mailer.Release(tutorEmail)
	awaitTick(t, ticked)
	afterAFailedSend := h.lessonEmailSlots(t, student)
	h.mailer.Unpause()
	h.tickAt(t, hourBeforeFirst.Add(41*time.Minute))
	afterTheRetry := h.lessonEmailSlots(t, student)

	h.at(t, hourBeforeFirst.Add(time.Hour))
	h.mailer.Pause()
	ticked = h.tickInBackground(t)
	if err := h.mailer.AwaitHeld(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	h.waitFor(t, "the parallel reminder dropped over the cap", func() bool {
		return h.countLogs(`"msg":"email.dropped"`, `"reason":"daily_cap"`) == 1
	})
	h.mailer.Fail("member-10@example.org", errSendRefused)
	h.mailer.ReleaseAll()
	awaitTick(t, ticked)
	whileTheTwentiethFailed := h.lessonEmailSlots(t, student)
	h.mailer.Unpause()
	h.tickAt(t, hourBeforeFirst.Add(time.Hour+time.Minute))
	afterItsRetry := h.lessonEmailSlots(t, student)
	h.tickAt(t, hourBeforeFirst.Add(90*time.Minute))

	counts := []int64{afterASend, afterAnObsoleteRow, afterAFailedSend, afterTheRetry, whileTheTwentiethFailed, afterItsRetry}
	if want := []int64{18, 18, 18, 19, 19, 20}; !equalCounts(counts, want) {
		t.Fatalf("the student's slots went %v instead of %v", counts, want)
	}
	if sent := len(h.remindersFor(t, "member-10@example.org", domain.EventLessonReminder1h)); sent != 3 {
		t.Fatalf("the student got %d of the capped reminders", sent)
	}
	if own := len(h.remindersFor(t, tutorEmail, domain.EventLessonReminder1h)); own != 5 || h.lessonEmailSlots(t, tutor) != 0 {
		t.Fatalf("the tutor got %d reminders of their own lessons and used %d slots", own, h.lessonEmailSlots(t, tutor))
	}
	if h.countLogs(`"msg":"email.dropped"`, `"reason":"daily_cap"`, `"group":"lesson_reminders"`) != 2 {
		t.Fatalf("the cap drops were reported as:\n%s", h.logs.String())
	}
	if rows := h.notificationsOf(t, h.studentToken(t, 10)); len(rows) != 6 {
		t.Fatalf("the student has %d one-hour rows instead of one per lesson", len(rows))
	}
}

func TestAFailedLessonEmailReleasesTheSlotOfItsOwnDayAcrossUTCMidnight(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), "2030-03-12", "03:50")
	student := domain.UserID{10}
	beforeMidnight := time.Date(2030, time.March, 11, 23, 59, 0, 0, time.UTC)
	afterMidnight := beforeMidnight.Add(2 * time.Minute)
	h.at(t, beforeMidnight)
	h.presetLessonEmailSlotsOn(t, student, beforeMidnight, 10)
	h.presetLessonEmailSlotsOn(t, student, afterMidnight, 5)
	h.mailer.Pause()

	ticked := h.tickInBackground(t)
	if err := h.mailer.AwaitHeld(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	reservedBeforeMidnight := h.lessonEmailSlotsOn(t, student, beforeMidnight)
	h.at(t, afterMidnight)
	failed := h.mailer.Fail("member-10@example.org", errSendRefused)
	h.mailer.ReleaseAll()
	awaitTick(t, ticked)

	if !failed || reservedBeforeMidnight != 11 {
		t.Fatalf("failed %v, the send reserved the day before midnight up to %d", failed, reservedBeforeMidnight)
	}
	if day, next := h.lessonEmailSlotsOn(t, student, beforeMidnight), h.lessonEmailSlotsOn(t, student, afterMidnight); day != 10 || next != 5 {
		t.Fatalf("after the failure past midnight the slots are %d and %d instead of 10 and 5", day, next)
	}
}

func equalCounts(got []int64, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

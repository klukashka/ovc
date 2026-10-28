package api_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	visibilityGroupName = "Evening club"
	monday              = 1
	sunday              = 7
	underWayDate        = "2030-03-10"
)

var (
	endedUnderWayStart  = time.Date(2030, time.March, 10, 10, 0, 0, 0, time.UTC)
	stillUnderWayStart  = time.Date(2030, time.March, 10, 12, 0, 0, 0, time.UTC)
	statusJobEndsLesson = endedUnderWayStart.Add(time.Hour + 31*time.Minute)
)

type visibilityAccount struct {
	id        domain.UserID
	email     string
	password  string
	confirmed bool
}

func seededAccount(id byte, confirmed bool) visibilityAccount {
	return visibilityAccount{id: domain.UserID{id}, email: memberEmail(id), password: accountPassword, confirmed: confirmed}
}

type visibleLesson struct {
	id     string
	tutor  domain.UserID
	status domain.LessonStatus
}

type visibilityCase struct {
	name     string
	account  visibilityAccount
	card     string
	lesson   visibleLesson
	visible  bool
	audience bool
}

func hiddenCase(name string, account visibilityAccount, card string, lesson visibleLesson) visibilityCase {
	return visibilityCase{name: name, account: account, card: card, lesson: lesson}
}

func rosterCase(name string, account visibilityAccount, card string, lesson visibleLesson) visibilityCase {
	return visibilityCase{name: name, account: account, card: card, lesson: lesson, visible: true}
}

func audienceCase(name string, account visibilityAccount, card string, lesson visibleLesson) visibilityCase {
	return visibilityCase{name: name, account: account, card: card, lesson: lesson, visible: true, audience: true}
}

type visibilityCopies struct {
	calendar     bool
	page         bool
	notification bool
	email        bool
	series       bool
	partSeries   bool
}

func (c visibilityCase) want() visibilityCopies {
	emailed := c.visible && c.account.confirmed && c.lesson.status != domain.LessonEnded
	return visibilityCopies{calendar: c.visible, page: c.visible, notification: c.visible, email: emailed, series: c.audience}
}

func (h *authHarness) lessonRowFor(t *testing.T, testCase visibilityCase) {
	t.Helper()
	recipient := testCase.account.id
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
VALUES (gen_random_uuid(), $1, $2, $1, 'lesson.reminder_1h', $3::uuid, $4::uuid, $5, $6)`,
		recipient, testCase.lesson.tutor, testCase.lesson.id, testCase.card, testCase.lesson.id+":"+recipient.String(), authStart)
	if err != nil {
		t.Fatal(err)
	}
}

func identifierBytes(t *testing.T, text string) [16]byte {
	t.Helper()
	var id [16]byte
	decoded, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(decoded) != len(id) {
		t.Fatalf("not an identifier: %q", text)
	}
	copy(id[:], decoded)
	return id
}

func (h *authHarness) cardOfSecondTutor(t *testing.T, tutorToken string, account visibilityAccount) string {
	t.Helper()
	student := accessToken(t, h.login(t, account.email, account.password))
	return h.linkedSignedIn(t, tutorToken, student, "A second tutor's student", account.email).ID
}

func (h *authHarness) squattedProvisional(t *testing.T, tutorToken string, id byte) (visibilityAccount, string) {
	t.Helper()
	card, student := h.squat(t, tutorToken, memberEmail(id), memberEmail(id))
	return visibilityAccount{id: student, email: memberEmail(id), password: newcomerPassword}, card.ID
}

func (h *authHarness) firstOfSeries(t *testing.T, token string, request lessonRequest, weekday int) lessonView {
	t.Helper()
	request.AllowOverlap = true
	return h.scheduled(t, token, request.weekly([]int{weekday}, 2)).Lessons[0]
}

func (h *authHarness) cancelledThis(t *testing.T, token string, lesson lessonView) {
	t.Helper()
	if answer := h.cancelCall(t, token, lesson.ID, "this", lesson.Version); answer.status != http.StatusOK {
		t.Fatalf("cancelling answered %d %s", answer.status, answer.body)
	}
}

func (h *authHarness) ended(t *testing.T, token string, card string) {
	t.Helper()
	if answer := h.endAsTutor(t, token, card); answer.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", answer.status, answer.body)
	}
}

func (h *authHarness) queueLessonEmail(t *testing.T, testCase visibilityCase) domain.OutboxID {
	t.Helper()
	recipient := testCase.account.id
	lessonHex, recipientHex := strings.ReplaceAll(testCase.lesson.id, "-", ""), hex.EncodeToString(recipient[:])
	kind := domain.EventLessonCancelled
	var payload []byte
	var err error
	if testCase.lesson.status == domain.LessonScheduled {
		kind = domain.EventLessonMoved
		var revision int32
		if err := h.database.Pool.QueryRow(context.Background(), "SELECT schedule_revision FROM lessons WHERE id = $1::uuid", testCase.lesson.id).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		payload, err = json.Marshal(domain.LessonReminderPayload{Lesson: lessonHex, Recipient: recipientHex, Revision: revision})
	} else {
		payload, err = json.Marshal(domain.LessonCancelledPayload{Lesson: lessonHex, Recipient: recipientHex})
	}
	if err != nil {
		t.Fatal(err)
	}
	var id domain.OutboxID
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	inserted, err := h.store.Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: kind, Payload: payload,
		IdempotencyKey: "visibility-copies:" + testCase.name, NextAttemptAt: authStart, CreatedAt: authStart,
	})
	if err != nil || !inserted {
		t.Fatalf("queueing the email of %s inserted %v: %v", testCase.name, inserted, err)
	}
	return id
}

func (h *authHarness) deliverAll(t *testing.T, ids []domain.OutboxID) map[domain.OutboxID]domain.OutboxDoneReason {
	t.Helper()
	reasons := map[domain.OutboxID]domain.OutboxDoneReason{}
	for attempt := 0; attempt < 5 && len(reasons) < len(ids); attempt++ {
		h.tick(t)
		for _, id := range ids {
			row, err := h.store.Repositories().Outbox.ByID(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if row.Status == domain.OutboxDone && row.DoneReason != nil {
				reasons[id] = *row.DoneReason
			}
		}
	}
	if len(reasons) != len(ids) {
		t.Fatalf("only %d of %d lesson emails finished", len(reasons), len(ids))
	}
	return reasons
}

type reclaimedLink struct {
	account visibilityAccount
	card    string
	owner   string
}

func (h *authHarness) squattedWithLessons(t *testing.T, token string, tutor domain.UserID, id byte) (reclaimedLink, visibleLesson, visibleLesson) {
	t.Helper()
	card, student := h.squat(t, token, memberEmail(id), memberEmail(id))
	scheduled := h.firstOfSeries(t, token, oneOffFor(card.ID, firstLessonDate, "10:00"), monday)
	cancelled := h.firstOfSeries(t, token, oneOffFor(card.ID, firstLessonDate, "11:00"), monday)
	h.cancelledThis(t, token, cancelled)
	owner := h.reclaimed(t, memberEmail(id))
	account := visibilityAccount{id: student, email: memberEmail(id), password: ownerPassword, confirmed: true}
	return reclaimedLink{account: account, card: card.ID, owner: owner},
		visibleLesson{id: scheduled.ID, tutor: tutor, status: domain.LessonScheduled},
		visibleLesson{id: cancelled.ID, tutor: tutor, status: domain.LessonCancelled}
}

type visibilityWorld struct {
	group string
	cases []visibilityCase
}

func (h *authHarness) visibilityWorld(t *testing.T) visibilityWorld {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	cards := h.activeCards(t, tutor, 11, 5)
	provisionalAccount, provisional := h.squattedProvisional(t, token, 30)
	member, former, endedStudent, stillUnderWay, endedUnderWay := seededAccount(11, true), seededAccount(12, true), seededAccount(13, true),
		seededAccount(14, true), seededAccount(15, true)
	group := h.newGroup(t, token, visibilityGroupName, cards[:2])
	first := func(view lessonView, status domain.LessonStatus) visibleLesson {
		return visibleLesson{id: view.ID, tutor: tutor, status: status}
	}
	scheduledGroup := first(h.firstOfSeries(t, token, groupLessonFor(group.ID, firstLessonDate, firstLessonLocal), monday), domain.LessonScheduled)
	cancelledGroupView := h.firstOfSeries(t, token, groupLessonFor(group.ID, firstLessonDate, "20:00"), monday)
	cancelledGroup := first(cancelledGroupView, domain.LessonCancelled)
	ownLesson := first(h.firstOfSeries(t, token, oneOffFor(cards[0], firstLessonDate, "21:00"), monday), domain.LessonScheduled)
	endedLesson := first(h.firstOfSeries(t, token, oneOffFor(cards[2], firstLessonDate, "21:00"), monday), domain.LessonCancelled)
	provisionalLesson := first(h.firstOfSeries(t, token, oneOffFor(provisional, firstLessonDate, "21:00"), monday), domain.LessonScheduled)
	stillUnderWayLesson := first(h.firstOfSeries(t, token, oneOffFor(cards[3], underWayDate, "15:00"), sunday), domain.LessonScheduled)
	endedUnderWayLesson := first(h.firstOfSeries(t, token, oneOffFor(cards[4], underWayDate, "13:00"), sunday), domain.LessonEnded)
	h.cancelledThis(t, token, cancelledGroupView)
	pending, pendingScheduled, pendingCancelled := h.squattedWithLessons(t, token, tutor, 20)
	kept, keptScheduled, keptCancelled := h.squattedWithLessons(t, token, tutor, 21)
	removed, removedScheduled, removedCancelled := h.squattedWithLessons(t, token, tutor, 22)
	removedScheduled.status = domain.LessonCancelled
	secondOfShared := h.cardOfSecondTutor(t, secondToken, member)
	secondOfPending := h.cardOfSecondTutor(t, secondToken, pending.account)
	secondOwn := visibleLesson{id: h.firstOfSeries(t, secondToken, oneOffFor(secondOfShared, firstLessonDate, "08:00"), monday).ID, tutor: second, status: domain.LessonScheduled}
	secondEnded := visibleLesson{id: h.firstOfSeries(t, secondToken, oneOffFor(secondOfPending, firstLessonDate, "08:00"), monday).ID, tutor: second, status: domain.LessonCancelled}
	if answer := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"remove":`+idList(cards[1:2])+`}`); answer.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", answer.status, answer.body)
	}
	h.ended(t, token, cards[2])
	h.ended(t, secondToken, secondOfPending)
	if answer := h.decideReclaim(t, kept.owner, reclaimChoice{KeepLinks: []string{kept.card}}); answer.status != http.StatusOK {
		t.Fatalf("keeping the link answered %d %s", answer.status, answer.body)
	}
	if answer := h.removeStudent(t, token, removed.card); answer.status != http.StatusOK {
		t.Fatalf("removing the reclaim-pending link answered %d %s", answer.status, answer.body)
	}
	h.at(t, endedUnderWayStart.Add(10*time.Minute))
	h.ended(t, h.tutorToken(t), cards[4])
	h.tickAt(t, statusJobEndsLesson)
	h.at(t, stillUnderWayStart.Add(10*time.Minute))
	h.ended(t, h.tutorToken(t), cards[3])
	return visibilityWorld{group: group.ID, cases: []visibilityCase{
		audienceCase("a current member's scheduled group lesson", member, cards[0], scheduledGroup),
		audienceCase("a current member's cancelled roster lesson", member, cards[0], cancelledGroup),
		audienceCase("an active relationship's scheduled lesson", member, cards[0], ownLesson),
		hiddenCase("a former member's scheduled group lesson", former, cards[1], scheduledGroup),
		rosterCase("a former member's cancelled roster lesson", former, cards[1], cancelledGroup),
		hiddenCase("another student's lesson", former, cards[1], ownLesson),
		rosterCase("an ended relationship's lesson cancelled by the ending", endedStudent, cards[2], endedLesson),
		audienceCase("a provisional link's scheduled lesson", provisionalAccount, provisional, provisionalLesson),
		hiddenCase("a reclaim-pending link's scheduled lesson", pending.account, pending.card, pendingScheduled),
		hiddenCase("a reclaim-pending link's cancelled roster lesson", pending.account, pending.card, pendingCancelled),
		rosterCase("another tutor's lesson cancelled by an ending beside a reclaim-pending link", pending.account, secondOfPending, secondEnded),
		audienceCase("a kept link's scheduled lesson", kept.account, kept.card, keptScheduled),
		audienceCase("a kept link's cancelled roster lesson", kept.account, kept.card, keptCancelled),
		hiddenCase("a removed link's lesson cancelled by the removal", removed.account, removed.card, removedScheduled),
		hiddenCase("a removed link's cancelled roster lesson", removed.account, removed.card, removedCancelled),
		audienceCase("the second tutor's lesson of a student with two tutors", member, secondOfShared, secondOwn),
		hiddenCase("the second tutor's lesson for another student", former, cards[1], secondOwn),
		hiddenCase("a lesson under way at an ending, still scheduled", stillUnderWay, cards[3], stillUnderWayLesson),
		rosterCase("a lesson under way at an ending, ended by the status job", endedUnderWay, cards[4], endedUnderWayLesson),
	}}
}

func (h *authHarness) tutorToken(t *testing.T) string {
	t.Helper()
	return accessToken(t, h.login(t, tutorEmail, accountPassword))
}

func (h *authHarness) labelledInNotifications(t *testing.T, token string, lesson string) bool {
	t.Helper()
	for _, row := range h.notificationsOf(t, token) {
		if row.Link != nil && row.Link.ID == lesson && row.Label != nil && *row.Label == lessonTitle {
			return true
		}
	}
	return false
}

func carriesItsSeries(view lessonView) bool {
	return view.SeriesID != nil && view.Series != nil && view.Series.ID == *view.SeriesID
}

func TestTheStudentVisibilityCopiesAgree(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.visibilityWorld(t)
	emails := make([]domain.OutboxID, 0, len(world.cases))
	for _, testCase := range world.cases {
		h.lessonRowFor(t, testCase)
		emails = append(emails, h.queueLessonEmail(t, testCase))
	}

	reasons := h.deliverAll(t, emails)
	tokens := map[domain.UserID]string{}
	results := map[string]visibilityCopies{}
	groupLabels := map[string][2]bool{}
	for index, testCase := range world.cases {
		if _, signedIn := tokens[testCase.account.id]; !signedIn {
			tokens[testCase.account.id] = accessToken(t, h.login(t, testCase.account.email, testCase.account.password))
		}
		student := tokens[testCase.account.id]
		page := h.lessonPage(t, student, testCase.lesson.id)
		copies := visibilityCopies{
			calendar:     slices.Contains(calendarIDs(h.calendarOf(t, student, calendarFrom, calendarTo)), testCase.lesson.id),
			page:         page.status == http.StatusOK,
			notification: h.labelledInNotifications(t, student, testCase.lesson.id),
			email:        reasons[emails[index]] == domain.DoneSent,
		}
		if page.status == http.StatusOK {
			view := decodeLesson(t, page)
			copies.series = carriesItsSeries(view)
			if view.Audience.Kind == string(domain.AudienceGroup) {
				member, err := h.store.Repositories().Lessons.IsCurrentMember(context.Background(), identifierBytes(t, world.group), testCase.account.id)
				if err != nil {
					t.Fatal(err)
				}
				groupLabels[testCase.name] = [2]bool{view.Audience.Label != nil && *view.Audience.Label == visibilityGroupName, member}
			}
			copies.partSeries = !copies.series && (view.SeriesID != nil || view.Series != nil)
		}
		results[testCase.name] = copies
	}

	for _, testCase := range world.cases {
		if results[testCase.name] != testCase.want() {
			t.Errorf("%s: calendar, page, notification label, lesson email, series and partial series are %+v, want %+v", testCase.name, results[testCase.name], testCase.want())
		}
	}
	if len(groupLabels) != 3 {
		t.Errorf("the group label was compared for %d lessons, want 3", len(groupLabels))
	}
	for name, label := range groupLabels {
		if label[0] != label[1] {
			t.Errorf("%s: the page shows the group's name %v while the email membership check answers %v", name, label[0], label[1])
		}
	}
	if dropped := h.countLogs(`"msg":"email.dropped"`); dropped != 0 {
		t.Errorf("%d lesson emails were dropped, so the email copy was not compared", dropped)
	}
}

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/schemas"
)

const joinRequestCountPath = "/api/v1/join-requests/count"

var recentUpdateKeys = []string{"id", "event", "created_at", "label", "label_deleted", "label_kind", "colour", "link"}

func (h *authHarness) seedRecentUpdate(t *testing.T, row repositories.NewNotification) string {
	t.Helper()
	id, err := core.NewID[domain.NotificationID](h.random)
	if err != nil {
		t.Fatal(err)
	}
	row.ID = id
	row.IdempotencyKey = id.String()
	inserted, err := h.store.Repositories().Notifications.Insert(t.Context(), row)
	if err != nil || !inserted {
		t.Fatalf("seed a recent update: inserted=%t err=%v", inserted, err)
	}
	return id.String()
}

func recentUpdates(t *testing.T, result response) []notificationItem {
	t.Helper()
	if result.status != http.StatusOK || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("home answered %d %s", result.status, result.body)
	}
	parts := decodedAs[map[string]json.RawMessage](t, json.RawMessage(result.body))
	if string(parts["recent_updates"]) == "null" {
		t.Fatal("recent updates is null")
	}
	for _, raw := range assertEachKeys(t, "recent update", parts["recent_updates"], recentUpdateKeys) {
		fields := decodedAs[map[string]json.RawMessage](t, raw)
		if string(fields["link"]) != "null" {
			assertKeys(t, "recent update link", fields["link"], []string{"kind", "id"})
		}
	}
	return decodedAs[[]notificationItem](t, parts["recent_updates"])
}

func TestRecentUpdatesListOnlyInformationalKindsOfTheLast14Days(t *testing.T) {
	t.Parallel()
	for _, role := range []domain.Role{domain.RoleTutor, domain.RoleStudent} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			parties := h.workParties(t, readyBlocks(40)...)
			submission := parsedSessionResource[domain.EntityID](t, parties.submission)
			user, token := parties.tutor, parties.tutorToken
			want := []string{"invitation.accepted", "relationship.ended", "export.ready", "export.failed"}
			if role == domain.RoleStudent {
				user, token = parties.student, parties.studentToken
				want = []string{"join_request.accepted", "join_request.declined", "relationship.ended", "group.archived", "issue.withdrawn", "export.ready", "export.failed"}
			}
			now := h.now(t)
			h.exec(t, "DELETE FROM notifications WHERE user_id = $1::uuid", user.String())
			for _, event := range domain.NotificationEvents() {
				tutorID := &parties.tutor
				if event == domain.NotificationExportReady || event == domain.NotificationExportFailed {
					tutorID = nil
				}
				for _, age := range []time.Duration{14*24*time.Hour - time.Microsecond, 14 * 24 * time.Hour, 14*24*time.Hour + time.Microsecond} {
					h.seedRecentUpdate(t, repositories.NewNotification{UserID: user, StudentID: &parties.student, TutorID: tutorID,
						Event: event, SubjectID: submission, CreatedAt: now.Add(-age)})
				}
			}
			h.exec(t, "UPDATE notifications SET read_at = created_at WHERE user_id = $1::uuid", user.String())

			result := h.homeAnswer(t, token, "")

			items := recentUpdates(t, result)
			got := eventsOf(items)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("recent update events = %v, want %v", got, want)
			}
			for _, item := range items {
				if !item.CreatedAt.Equal(now.Add(-(14*24*time.Hour - time.Microsecond))) {
					t.Fatalf("an update outside the window was returned: %+v", item)
				}
			}
		})
	}
}

func TestRecentUpdatesStopAtTen(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	now := h.now(t)
	ids := make([]string, 0, 12)
	for index := range 12 {
		ids = append(ids, h.seedRecentUpdate(t, repositories.NewNotification{UserID: user, Event: domain.NotificationExportReady,
			CreatedAt: now.Add(-time.Duration(index/2) * time.Minute)}))
	}
	want := slices.Clone(ids[:10])
	for index := 0; index < len(want); index += 2 {
		pair := want[index : index+2]
		slices.Sort(pair)
		slices.Reverse(pair)
	}

	result := h.homeAnswer(t, token, "")

	items := recentUpdates(t, result)
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("recent update order = %v, want %v", got, want)
	}
}

func TestRecentUpdatesAreScopedToTheCallerAndNullLinksOutsideScope(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	first, firstToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "second-tutor@example.org")
	firstCard := h.linkedCard(t, first, 10, domain.RelationshipActive)
	secondCard := h.linkedCard(t, second, 11, domain.RelationshipActive)
	firstID := parsedSessionResource[domain.RelationshipID](t, firstCard)
	secondID := parsedSessionResource[domain.RelationshipID](t, secondCard)
	own := h.seedRecentUpdate(t, repositories.NewNotification{UserID: first, TutorID: &first, Event: domain.NotificationInvitationAccepted,
		RelationshipID: &firstID, CreatedAt: h.now(t)})
	foreign := h.seedRecentUpdate(t, repositories.NewNotification{UserID: first, TutorID: &first, Event: domain.NotificationInvitationAccepted,
		RelationshipID: &secondID, CreatedAt: h.now(t)})
	other := h.seedRecentUpdate(t, repositories.NewNotification{UserID: second, TutorID: &second, Event: domain.NotificationInvitationAccepted,
		RelationshipID: &secondID, CreatedAt: h.now(t)})

	firstAnswer := h.homeAnswer(t, firstToken, "")
	secondAnswer := h.homeAnswer(t, secondToken, "")

	firstItems, secondItems := recentUpdates(t, firstAnswer), recentUpdates(t, secondAnswer)
	if len(firstItems) != 2 || len(secondItems) != 1 || secondItems[0].ID != other {
		t.Fatalf("caller scopes disagree: %+v / %+v", firstItems, secondItems)
	}
	for _, item := range firstItems {
		switch item.ID {
		case own:
			if item.Link == nil || item.Link.ID != firstCard || item.Label == nil {
				t.Fatalf("own card lost its label or link: %+v", item)
			}
		case foreign:
			if item.Link != nil || item.Label != nil || item.Colour != nil {
				t.Fatalf("another tutor's card leaked: %+v", item)
			}
		default:
			t.Fatalf("another caller's notification leaked: %+v", item)
		}
	}
}

func TestRecentUpdatesHideHomeworkRowsOfAnUnreadableSubmission(t *testing.T) {
	t.Parallel()
	for _, state := range []domain.RelationshipState{domain.RelationshipActive, domain.RelationshipProvisional, domain.RelationshipEnded, domain.RelationshipReclaimPending, domain.RelationshipDiscarded} {
		for _, erased := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/erased_%t", state, erased), func(t *testing.T) {
				t.Parallel()
				h := newAuthHarness(t)
				parties := h.workParties(t, readyBlocks(40)...)
				h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, parties.issue), http.StatusOK)
				if erased {
					h.markIssueForErasure(t, parties.issue)
				}
				h.setRelationshipState(t, parties.card.ID, state)
				positive := h.seedRecentUpdate(t, repositories.NewNotification{UserID: parties.student, Event: domain.NotificationExportReady, CreatedAt: h.now(t)})
				visible := !erased && state != domain.RelationshipReclaimPending && state != domain.RelationshipDiscarded

				result := h.homeAnswer(t, parties.studentToken, "")

				items := recentUpdates(t, result)
				if !slices.ContainsFunc(items, func(item notificationItem) bool { return item.ID == positive }) {
					t.Fatal("the readable update disappeared with the homework rows")
				}
				withdrawals := eventsNamed(items, string(domain.NotificationIssueWithdrawn))
				if visible && (len(withdrawals) != 1 || withdrawals[0].Link != nil || withdrawals[0].Label != nil) {
					t.Fatalf("the retained withdrawal must have no label or link: %+v", withdrawals)
				}
				if !visible && len(withdrawals) != 0 {
					t.Fatalf("an unreadable submission leaked: %+v", withdrawals)
				}
			})
		}
	}
}

func TestTheJoinRequestCountLeavesOutHeldSuppressedAndExpiredRequests(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, heldToken := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)
	_, blockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
	_, pendingToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 4, "pending@example.org")
	_, expiredToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 5, "expired@example.org")
	other, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 6, "other-tutor@example.org")
	operatorBlocked, operatorBlockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 7, "operator-blocked@example.org")
	code := h.storedCode(t, tutor).Display()
	h.enteredRequest(t, heldToken, code)
	h.mustAnswer(t, h.declineRequest(t, token, requestText(h.enteredRequest(t, blockedToken, code)), true), http.StatusOK)
	h.enteredRequest(t, blockedToken, code)
	h.enteredRequest(t, pendingToken, code)
	expired := h.enteredRequest(t, expiredToken, code)
	h.exec(t, "UPDATE join_requests SET expires_at = created_at WHERE id = $1::uuid", expired.String())
	h.enteredRequest(t, pendingToken, h.storedCode(t, other).Display())
	h.enteredRequest(t, operatorBlockedToken, code)
	h.exec(t, "UPDATE users SET state_before_block = state, state = 'blocked' WHERE id = $1::uuid", operatorBlocked.String())
	requireArrangedRequestStates(t, h, tutor)

	result := h.send(t, http.MethodGet, joinRequestCountPath, "", jsonCall(token))

	h.mustAnswer(t, result, http.StatusOK)
	body := assertKeys(t, "join request count", json.RawMessage(result.body), []string{"count"})
	if string(body["count"]) != "1" || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("visible join request count = %s", result.body)
	}
	records := countAccessRecords(t, h.logs.String())
	if len(records) != 1 || string(records[0]["join_requests"]) != "1" {
		t.Fatalf("join request count access records = %v", records)
	}
}

func countAccessRecords(t *testing.T, logs string) []homeAccessRecord {
	t.Helper()
	var found []homeAccessRecord
	for line := range strings.Lines(logs) {
		record := decodedAs[homeAccessRecord](t, json.RawMessage(line))
		if string(record["msg"]) == `"http.request.completed"` && string(record["route"]) == `"GET /api/v1/join-requests/count"` {
			found = append(found, record)
		}
	}
	return found
}

func requireArrangedRequestStates(t *testing.T, h *authHarness, tutor domain.UserID) {
	t.Helper()
	held := h.countRows(t, "SELECT count(*) FROM join_requests WHERE tutor_id = $1::uuid AND state = 'held'", tutor.String())
	blocked := h.countRows(t, "SELECT count(*) FROM join_requests WHERE tutor_id = $1::uuid AND state = 'pending' AND suppressed = 'blocked'", tutor.String())
	visible := h.countRows(t, "SELECT count(*) FROM join_requests WHERE tutor_id = $1::uuid AND state = 'pending' AND suppressed = 'none'", tutor.String())
	if held != 1 || blocked != 1 || visible != 3 {
		t.Fatalf("arranged requests: held %d, blocked %d, unsuppressed pending %d", held, blocked, visible)
	}
}

func TestAStudentGetsNotFoundFromTheJoinRequestCount(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)

	result := h.send(t, http.MethodGet, joinRequestCountPath, "", jsonCall(token))

	if result.status != http.StatusNotFound || code(t, result) != "not_found" {
		t.Fatalf("student count answered %d %s", result.status, result.body)
	}
}

func TestAnUnconfirmedTutorIsRefusedTheJoinRequestCount(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)

	result := h.send(t, http.MethodGet, joinRequestCountPath, "", jsonCall(token))

	if result.status != http.StatusForbidden || code(t, result) != "email_not_verified" {
		t.Fatalf("unconfirmed tutor count answered %d %s", result.status, result.body)
	}
}

func calendarBody(t *testing.T, result response) []schemas.CalendarItem {
	t.Helper()
	if result.status != http.StatusOK {
		t.Fatalf("calendar answered %d %s", result.status, result.body)
	}
	return decodedAs[schemas.Calendar](t, json.RawMessage(result.body)).Items
}

func TestMovingALessonMarksItMovedUntilItHasPassed(t *testing.T) {
	t.Parallel()
	for _, edit := range []struct {
		name  string
		patch lessonPatch
		end   time.Time
		moved bool
	}{
		{"start", thisLesson(1).startingAt("19:00"), firstLessonEnd.Add(time.Hour), true},
		{"duration", thisLesson(1).lasting(90), firstLessonEnd.Add(30 * time.Minute), true},
		{"title", thisLesson(1).titled("New title"), firstLessonEnd, false},
	} {
		for _, atEnd := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/at_end_%t", edit.name, atEnd), func(t *testing.T) {
				t.Parallel()
				h := newAuthHarness(t)
				tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
				lesson := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
				h.mustAnswer(t, h.editLesson(t, token, lesson, edit.patch), http.StatusOK)
				if atEnd {
					h.at(t, edit.end)
				}
				token = h.relogin(t, tutorEmail)
				studentToken := h.studentToken(t, 10)

				tutorCalendar := h.calendarCall(t, token, firstLessonStart.Add(-time.Hour), edit.end.Add(time.Hour), "")
				studentCalendar := h.calendarCall(t, studentToken, firstLessonStart.Add(-time.Hour), edit.end.Add(time.Hour), "")
				home := h.homeAnswer(t, studentToken, "")

				for _, answer := range []response{tutorCalendar, studentCalendar} {
					items := calendarBody(t, answer)
					if len(items) != 1 || items[0].ID != lesson || items[0].Moved != (edit.moved && !atEnd) {
						t.Fatalf("moved marker = %+v", items)
					}
				}
				body := decodedAs[schemas.StudentHome](t, json.RawMessage(home.body))
				if atEnd {
					if body.NextLesson != nil || len(body.NextSevenDays) != 0 {
						t.Fatalf("an ended lesson remains on home: %+v", body)
					}
				} else if body.NextLesson == nil || body.NextLesson.Moved != edit.moved || len(body.NextSevenDays) != 1 || body.NextSevenDays[0].Moved != edit.moved {
					t.Fatalf("home moved markers = %+v", body)
				}
				var movedAt *time.Time
				if err := h.database.Pool.QueryRow(t.Context(), "SELECT moved_at FROM lessons WHERE id = $1::uuid", lesson).Scan(&movedAt); err != nil {
					t.Fatal(err)
				}
				if edit.moved && (movedAt == nil || !movedAt.Equal(authStart)) || !edit.moved && movedAt != nil {
					t.Fatalf("moved_at = %v", movedAt)
				}
			})
		}
	}
}

func TestCancellingClearsTheMovedMark(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	h.mustAnswer(t, h.editLesson(t, token, lesson, thisLesson(1).lasting(90)), http.StatusOK)
	studentToken := h.studentToken(t, 10)

	cancelled := h.cancelCall(t, token, lesson, "this", 2)
	calendar := h.calendarCall(t, studentToken, firstLessonStart, firstLessonEnd.Add(time.Hour), "")
	home := h.homeAnswer(t, studentToken, "")

	h.mustAnswer(t, cancelled, http.StatusOK)
	items := calendarBody(t, calendar)
	if len(items) != 1 || items[0].Moved || items[0].Status != domain.LessonCancelled {
		t.Fatalf("cancelled calendar item = %+v", items)
	}
	body := decodedAs[schemas.StudentHome](t, json.RawMessage(home.body))
	if body.NextLesson != nil || len(body.NextSevenDays) != 1 || body.NextSevenDays[0].Moved || body.NextSevenDays[0].Status != domain.LessonCancelled {
		t.Fatalf("cancelled home item = %+v", body)
	}
	if h.countRows(t, "SELECT count(*) FROM lessons WHERE id = $1::uuid AND moved_at IS NOT NULL", lesson) != 1 {
		t.Fatal("cancelling cleared the recorded move")
	}
}

func TestASeriesMoveMarksEachMovedOccurrence(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 3)
	studentToken := h.studentToken(t, 10)

	edited := h.editLesson(t, token, series.Lessons[1].ID, following(1).startingAt("19:00"))
	calendar := h.calendarCall(t, studentToken, authStart, authStart.Add(30*24*time.Hour), "")

	h.mustAnswer(t, edited, http.StatusOK)
	items := calendarBody(t, calendar)
	if len(items) != 3 {
		t.Fatalf("series calendar has %d items", len(items))
	}
	for index, item := range items {
		if item.ID != series.Lessons[index].ID || item.Moved != (index > 0) {
			t.Fatalf("series occurrence %d = %+v", index, item)
		}
	}
}

func TestNextSevenDaysKeepsCancelledLessonsWithTheirStatus(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	past := h.lessonAt(t, token, card, "2030-03-10", "13:00")
	future := h.lessonAt(t, token, card, "2030-03-10", "15:00")
	next := h.lessonAt(t, token, card, "2030-03-10", "17:00")
	for _, lesson := range []string{past, future} {
		h.mustAnswer(t, h.cancelCall(t, token, lesson, "this", 1), http.StatusOK)
	}
	h.at(t, time.Date(2030, time.March, 10, 10, 30, 0, 0, time.UTC))
	studentToken := h.studentToken(t, 10)

	result := h.homeAnswer(t, studentToken, "")

	h.mustAnswer(t, result, http.StatusOK)
	body := decodedAs[schemas.StudentHome](t, json.RawMessage(result.body))
	if len(body.NextSevenDays) != 2 || body.NextSevenDaysCount != 2 || body.NextSevenDays[0].ID != future ||
		body.NextSevenDays[0].Status != domain.LessonCancelled || body.NextSevenDays[1].ID != next {
		t.Fatalf("seven-day lessons = %+v", body)
	}
	if body.NextLesson == nil || body.NextLesson.ID != next || body.TodayLessonsCount != 1 {
		t.Fatalf("cancelled lessons affected next lesson or today's count: %+v", body)
	}
}

func TestRecentUpdatesIgnoreTheStudentTutorFilter(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.workParties(t, readyBlocks(40)...)
	other, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "other-tutor@example.org")
	otherCard := h.seedCard(t, other, 5, domain.RelationshipActive, &parties.student)
	otherID := parsedSessionResource[domain.RelationshipID](t, otherCard)
	h.seedRecentUpdate(t, repositories.NewNotification{UserID: parties.student, TutorID: &other, Event: domain.NotificationRelationshipEnded,
		RelationshipID: &otherID, CreatedAt: h.now(t)})
	h.seedRecentUpdate(t, repositories.NewNotification{UserID: parties.student, Event: domain.NotificationExportReady, CreatedAt: h.now(t)})

	all := h.homeAnswer(t, parties.studentToken, "")
	filtered := h.homeAnswer(t, parties.studentToken, homeQuery(parties.card.ID))

	allItems, filteredItems := recentUpdates(t, all), recentUpdates(t, filtered)
	if len(allItems) != 2 || !slices.Equal(eventsOf(allItems), eventsOf(filteredItems)) {
		t.Fatalf("the tutor filter changed recent updates: %+v / %+v", allItems, filteredItems)
	}
	for index, item := range allItems {
		if item.ID != filteredItems[index].ID {
			t.Fatalf("the tutor filter changed update %d", index)
		}
	}
}

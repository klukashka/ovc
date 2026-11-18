package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type homeTutorWorld struct {
	parties  endingParties
	joiner   string
	upcoming string
	later    string
}

func (h *authHarness) homeTutorWorld(t *testing.T) homeTutorWorld {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	card := toCard(parties.card.ID)
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-10", "14:00"))
	cancelled := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-10", "16:00")).Lessons[0].ID
	h.mustAnswer(t, h.cancelCall(t, parties.tutorToken, cancelled, "this", 1), http.StatusOK)
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-11", "10:00"))
	template := h.readyTemplate(t, parties.tutorToken, "Trip to Paris", 40).ID
	soon, later := authStart.Add(24*time.Hour), authStart.Add(5*24*time.Hour)
	upcoming := h.issued(t, parties.tutorToken, issueJSONWith(template, card, nil, &soon, "after_check")).ID
	distant := h.issued(t, parties.tutorToken, issueJSONWith(template, card, nil, &later, "after_check")).ID
	h.mustAnswer(t, h.submitWork(t, parties.studentToken, h.submissionsOf(t, upcoming)[0].id, 1), http.StatusOK)
	_, joinerToken := h.requestingStudent(t, 3, "joiner@example.org", "A joining student")
	h.mustAnswer(t, h.enterCode(t, joinerToken, h.storedCode(t, parties.tutor).Display()), http.StatusOK)
	h.mustAnswer(t, h.sendMessage(t, parties.studentToken, h.firstChannelOf(t, parties.studentToken), "Hello", nil, ""), http.StatusCreated)
	return homeTutorWorld{parties: parties, joiner: joinerToken, upcoming: upcoming, later: distant}
}

func (h *authHarness) expectedUnreadChannels(t *testing.T, token string) []string {
	t.Helper()
	body := h.routeBody(t, token, channelsRoute)
	type channel struct {
		ID          string          `json:"id"`
		Label       json.RawMessage `json:"label"`
		LabelKind   json.RawMessage `json:"label_kind"`
		Colour      json.RawMessage `json:"colour"`
		UnreadCount int64           `json:"unread_count"`
	}
	var expected []string
	for _, item := range decodedAs[[]channel](t, body["items"]) {
		if item.UnreadCount == 0 || len(expected) == 5 {
			continue
		}
		encoded, err := json.Marshal(map[string]json.RawMessage{
			"channel_id": rawOf(t, item.ID), "label": item.Label, "label_kind": item.LabelKind, "colour": item.Colour, "unread_count": rawOf(t, item.UnreadCount),
		})
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, string(encoded))
	}
	return expected
}

func normalisedItems(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("not an array of objects: %s", raw)
	}
	normalised := make([]string, 0, len(items))
	for _, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		normalised = append(normalised, string(encoded))
	}
	return normalised
}

func TestHomeTutorParts(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.homeTutorWorld(t)
	token := world.parties.tutorToken
	now := h.homeNow(t)
	dayStart := homeDayStart(now)

	parts := h.home(t, token, "")
	h.mustAnswer(t, h.patchMe(t, token, `{"bio":"Teaches French"}`), http.StatusOK)
	described := h.home(t, token, "")

	assertTutorHomeShape(t, parts)
	calendar, items := h.calendarItems(t, token, dayStart, dayStart.Add(24*time.Hour), "")
	var today []string
	for index, item := range items {
		if !item.StartsAt.Before(dayStart) && item.StartsAt.Before(dayStart.Add(24*time.Hour)) {
			today = append(today, calendar[index])
		}
	}
	if got := compactItems(t, parts["today_lessons"]); len(today) != 2 || !slices.Equal(got, today) {
		t.Errorf("today_lessons is %v, want the calendar's day %v", got, today)
	}
	if count := decodedAs[int64](t, parts["today_lessons_count"]); count != 2 {
		t.Errorf("today_lessons_count is %d, want 2", count)
	}
	queue := h.routeBody(t, token, reviewQueueRoute+"?limit=5")
	checking := decodedAs[map[string]json.RawMessage](t, parts["needs_checking"])
	if !slices.Equal(compactItems(t, checking["items"]), compactItems(t, queue["items"])) || compactValue(t, checking["count"]) != compactValue(t, queue["count"]) ||
		len(compactItems(t, queue["items"])) != 1 {
		t.Errorf("needs_checking is %s, want the queue %s", parts["needs_checking"], queue["items"])
	}
	issues := h.routeBody(t, token, issuesRoute+"?"+url.Values{
		"status": {"active"}, "deadline_before": {now.Add(72 * time.Hour).Format(time.RFC3339Nano)}, "limit": {"5"},
	}.Encode())
	deadlines := decodedAs[map[string]json.RawMessage](t, parts["upcoming_deadlines"])
	if got := compactItems(t, deadlines["items"]); !slices.Equal(got, compactItems(t, issues["items"])) || len(got) != 1 || !strings.Contains(got[0], world.upcoming) {
		t.Errorf("upcoming_deadlines lists %v, want %s", got, issues["items"])
	}
	if count := decodedAs[int64](t, deadlines["count"]); count != 1 {
		t.Errorf("upcoming_deadlines counts %d, want 1", count)
	}
	unread := decodedAs[map[string]json.RawMessage](t, parts["unread_channels"])
	if got, want := normalisedItems(t, unread["items"]), h.expectedUnreadChannels(t, token); len(want) != 1 || !slices.Equal(got, want) {
		t.Errorf("unread_channels lists %v, want %v", got, want)
	}
	if total := compactValue(t, unread["unread_total"]); total != compactValue(t, h.routeBody(t, token, channelsRoute+"/unread")["unread_channels"]) {
		t.Errorf("unread_total is %s", total)
	}
	requests := h.routeBody(t, token, joinRequestsPath+"?limit=5")
	joins := decodedAs[map[string]json.RawMessage](t, parts["join_requests"])
	if !slices.Equal(compactItems(t, joins["items"]), compactItems(t, requests["items"])) || compactValue(t, joins["next_cursor"]) != "null" ||
		decodedAs[int64](t, joins["count"]) != 1 {
		t.Errorf("join_requests is %s, want %s", parts["join_requests"], requests["items"])
	}
	code := h.send(t, http.MethodGet, "/api/v1/tutor/code", "", jsonCall(token))
	if compactValue(t, parts["tutor_code"]) != compactValue(t, json.RawMessage(code.body)) {
		t.Errorf("tutor_code is %s, want %s", parts["tutor_code"], code.body)
	}
	wantSteps := `{"onboarding_dismissed":false,"steps":{"profile":false,"student":true,"lesson":true,"assignment":true}}`
	if got := compactValue(t, parts["checklist"]); got != wantSteps {
		t.Errorf("checklist is %s, want %s", got, wantSteps)
	}
	wantDescribed := `{"onboarding_dismissed":false,"steps":{"profile":true,"student":true,"lesson":true,"assignment":true}}`
	if got := compactValue(t, described["checklist"]); got != wantDescribed {
		t.Errorf("the checklist after the bio is %s, want %s", got, wantDescribed)
	}
}

func TestHomeTutorChecklistStartsEmpty(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	parts := h.home(t, token, "")

	assertTutorHomeShape(t, parts)
	want := `{"onboarding_dismissed":false,"steps":{"profile":false,"student":false,"lesson":false,"assignment":false}}`
	if got := compactValue(t, parts["checklist"]); got != want {
		t.Errorf("checklist is %s, want %s", got, want)
	}
	if got := compactValue(t, parts["today_lessons"]); got != "[]" {
		t.Errorf("today_lessons is %s", got)
	}
}

type homeStudentWorld struct {
	parties     endingParties
	other       string
	otherTutor  string
	checkedWork string
}

func (h *authHarness) homeStudentWorld(t *testing.T) homeStudentWorld {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	card := toCard(parties.card.ID)
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-10", "14:00"))
	cancelled := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-10", "16:00")).Lessons[0].ID
	h.mustAnswer(t, h.cancelCall(t, parties.tutorToken, cancelled, "this", 1), http.StatusOK)
	h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group.ID, "2030-03-14", "10:00"))
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-20", "10:00"))
	template := h.readyTemplate(t, parties.tutorToken, "Trip to Paris", 40).ID
	automatic := h.workTemplate(t, parties.tutorToken, "Automatic", contractAutomaticBlocks()...)
	soon := authStart.Add(48 * time.Hour)
	h.issued(t, parties.tutorToken, issueJSONWith(template, card, nil, &soon, "after_check"))
	checked := h.issued(t, parties.tutorToken, issueJSONWith(automatic, card, nil, nil, "after_check")).ID
	checkedWork := h.submissionsOf(t, checked)[0].id
	h.mustAnswer(t, h.submitWork(t, parties.studentToken, checkedWork, 1), http.StatusOK)
	otherTutor := h.seedTutorAccount(t, 5)
	other := h.seedCard(t, otherTutor, 5, domain.RelationshipActive, &parties.student)
	return homeStudentWorld{parties: parties, other: other, otherTutor: otherTutor.String(), checkedWork: checkedWork}
}

func (h *authHarness) assertStudentHomeMatchesReads(t *testing.T, token string, tutor string, parts map[string]json.RawMessage) {
	t.Helper()
	now := h.homeNow(t)
	dayStart := homeDayStart(now)
	calendar, items := h.calendarItems(t, token, dayStart, dayStart.AddDate(0, 0, 63), tutor)
	next := "null"
	var week []string
	today := int64(0)
	for index, item := range items {
		if item.Status == "cancelled" {
			continue
		}
		if !item.StartsAt.Before(dayStart) && item.StartsAt.Before(dayStart.Add(24*time.Hour)) {
			today++
		}
		if !item.EndsAt.After(now) {
			continue
		}
		if next == "null" {
			next = calendar[index]
		}
		if item.StartsAt.Before(now.Add(7 * 24 * time.Hour)) {
			week = append(week, calendar[index])
		}
	}
	if got := compactValue(t, parts["next_lesson"]); got != next {
		t.Errorf("next_lesson is %s, want %s", got, next)
	}
	if got := decodedAs[int64](t, parts["today_lessons_count"]); got != today {
		t.Errorf("today_lessons_count is %d, want %d", got, today)
	}
	if got := compactItems(t, parts["next_seven_days"]); !slices.Equal(got, week) || decodedAs[int64](t, parts["next_seven_days_count"]) != int64(len(week)) {
		t.Errorf("next_seven_days is %v, want %v", got, week)
	}
	filter := url.Values{"limit": {"5"}}
	if tutor != "" {
		filter.Set("tutor", tutor)
	}
	todoPage := h.routeBody(t, token, assignmentsRoute+"?"+filter.Encode())
	todo := decodedAs[map[string]json.RawMessage](t, parts["todo"])
	counts := decodedAs[map[string]json.RawMessage](t, todoPage["counts"])
	if !slices.Equal(compactItems(t, todo["items"]), compactItems(t, todoPage["items"])) || compactValue(t, todo["count"]) != compactValue(t, counts["todo"]) {
		t.Errorf("todo is %s, want %s and %s", parts["todo"], todoPage["items"], counts["todo"])
	}
	filter.Set("status", "checked")
	checkedPage := h.routeBody(t, token, assignmentsRoute+"?"+filter.Encode())
	if got := compactItems(t, parts["recently_checked"]); !slices.Equal(got, compactItems(t, checkedPage["items"])) {
		t.Errorf("recently_checked is %v, want %s", got, checkedPage["items"])
	}
	tutors := decodedAs[[]struct {
		RelationshipID string          `json:"relationship_id"`
		State          string          `json:"state"`
		Colour         string          `json:"colour"`
		ChannelID      json.RawMessage `json:"channel_id"`
		Tutor          struct {
			Name     string          `json:"name"`
			AvatarID json.RawMessage `json:"avatar_id"`
			Subjects []string        `json:"subjects"`
		} `json:"tutor"`
	}](t, h.routeBody(t, token, "/api/v1/tutors")["tutors"])
	var want []string
	for _, tutor := range tutors {
		if tutor.State != "active" && tutor.State != "provisional" {
			continue
		}
		encoded, err := json.Marshal(map[string]json.RawMessage{
			"relationship_id": rawOf(t, tutor.RelationshipID), "state": rawOf(t, tutor.State), "name": rawOf(t, tutor.Tutor.Name),
			"avatar_id": tutor.Tutor.AvatarID, "colour": rawOf(t, tutor.Colour), "subjects": rawOf(t, tutor.Tutor.Subjects), "channel_id": tutor.ChannelID,
		})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, string(encoded))
	}
	if got := normalisedItems(t, parts["tutors"]); !slices.Equal(got, want) || decodedAs[int64](t, parts["tutors_count"]) != int64(len(want)) {
		t.Errorf("tutors is %v, want %v", got, want)
	}
}

func TestHomeStudentParts(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.homeStudentWorld(t)
	token := world.parties.studentToken

	unfiltered := h.home(t, token, "")
	filtered := h.home(t, token, homeQuery(world.parties.card.ID))
	other := h.home(t, token, homeQuery(world.other))

	for label, parts := range map[string]map[string]json.RawMessage{"unfiltered": unfiltered, "filtered": filtered, "other": other} {
		raw, err := json.Marshal(parts)
		if err != nil {
			t.Fatal(err)
		}
		contractCheck{t: t}.studentHome(label, raw)
	}
	h.assertStudentHomeMatchesReads(t, token, "", unfiltered)
	h.assertStudentHomeMatchesReads(t, token, world.parties.card.ID, filtered)
	h.assertStudentHomeMatchesReads(t, token, world.other, other)
	if decodedAs[int64](t, unfiltered["tutors_count"]) != 2 || compactValue(t, filtered["tutors"]) != compactValue(t, unfiltered["tutors"]) {
		t.Errorf("the tutors part follows the filter: %s %s", unfiltered["tutors"], filtered["tutors"])
	}
	if decodedAs[int64](t, unfiltered["today_lessons_count"]) != 1 || len(compactItems(t, unfiltered["recently_checked"])) != 1 {
		t.Errorf("the seeded lesson and checked work are missing: %v", unfiltered)
	}
	otherTodo := decodedAs[map[string]json.RawMessage](t, other["todo"])
	if compactValue(t, otherTodo["items"]) != "[]" || decodedAs[int64](t, otherTodo["count"]) != 1 || compactValue(t, other["recently_checked"]) != "[]" {
		t.Errorf("the other tutor's filter shows the first tutor's work: %s %s", other["todo"], other["recently_checked"])
	}
}

func TestHomeScope(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.homeTutorWorld(t)
	shared := world.parties.student
	otherTutor, otherTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 7, "other-tutor@example.org")
	otherCard := h.seedCard(t, otherTutor, 7, domain.RelationshipActive, &shared)
	otherLesson := h.scheduled(t, otherTutorToken, oneOffFor(otherCard, "2030-03-10", "15:00")).Lessons[0].ID
	otherTemplate := h.readyTemplate(t, otherTutorToken, "Another trip", 41).ID
	soon := authStart.Add(24 * time.Hour)
	otherIssue := h.issued(t, otherTutorToken, issueJSONWith(otherTemplate, toCard(otherCard), nil, &soon, "after_check")).ID
	otherWork := h.submissionsOf(t, otherIssue)[0].id
	h.mustAnswer(t, h.submitWork(t, world.parties.studentToken, otherWork, 1), http.StatusOK)
	classmate := h.seedStudentAccount(t, 8, domain.AccountActive)
	classmateCard := h.seedCard(t, world.parties.tutor, 8, domain.RelationshipActive, &classmate)
	classmateTemplate := h.readyTemplate(t, world.parties.tutorToken, "A classmate's trip", 42).ID
	classmateIssue := h.issued(t, world.parties.tutorToken, issueJSONWith(classmateTemplate, toCard(classmateCard), nil, &soon, "after_check")).ID
	classmateWork := h.submissionsOf(t, classmateIssue)[0].id
	class := h.newGroup(t, world.parties.tutorToken, "A shared class", []string{world.parties.card.ID, classmateCard})
	classLesson := h.scheduled(t, world.parties.tutorToken, groupLessonFor(class.ID, "2030-03-11", "12:00")).Lessons[0].ID
	pendingTutor, pendingTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 9, "pending-tutor@example.org")
	pendingCard := h.seedCard(t, pendingTutor, 9, domain.RelationshipActive, &shared)
	pendingLesson := h.scheduled(t, pendingTutorToken, oneOffFor(pendingCard, "2030-03-10", "18:00")).Lessons[0].ID
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", pendingCard)
	entered := h.enterCode(t, world.joiner, h.storedCode(t, otherTutor).Display())
	h.mustAnswer(t, entered, http.StatusOK)
	otherRequest := decodedAs[struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}](t, json.RawMessage(entered.body)).Request.ID
	foreign := []string{otherLesson, otherIssue, otherWork, otherCard, pendingCard, pendingLesson, otherRequest}

	tutorBody := h.homeAnswer(t, world.parties.tutorToken, "").body
	studentBody := h.homeAnswer(t, world.parties.studentToken, "").body
	filteredBody := h.homeAnswer(t, world.parties.studentToken, homeQuery(world.parties.card.ID)).body
	classmateBody := h.homeAnswer(t, h.seededStudentToken(t, 8), "").body
	pendingFilter := h.homeAnswer(t, world.parties.studentToken, homeQuery(pendingCard))

	if found := namedIn(tutorBody, foreign...); len(found) != 0 {
		t.Errorf("the tutor's home names another tutor's data: %v", found)
	}
	if found := namedIn(studentBody, pendingCard, pendingLesson, classmateWork, classmateCard); len(found) != 0 {
		t.Errorf("the student's home names a reclaim-pending link or a classmate's work: %v", found)
	}
	if found := namedIn(studentBody, otherLesson, otherCard); len(found) != 2 {
		t.Errorf("the student's home misses the second tutor's own data: %v", found)
	}
	if found := namedIn(filteredBody, otherLesson, otherWork, otherCard); len(found) != 1 || found[0] != otherCard {
		t.Errorf("the filtered home names the other tutor's data beyond its tutors row: %v", found)
	}
	if found := namedIn(classmateBody, otherWork, world.upcoming, h.submissionsOf(t, world.upcoming)[0].id); len(found) != 0 {
		t.Errorf("a classmate's home names the student's work: %v", found)
	}
	if !strings.Contains(classmateBody, classmateWork) || !strings.Contains(classmateBody, classLesson) || !strings.Contains(studentBody, classLesson) {
		t.Error("a member of the shared class misses its own work or the class lesson")
	}
	if pendingFilter.status != http.StatusNotFound {
		t.Errorf("a reclaim-pending filter answered %d %s", pendingFilter.status, pendingFilter.body)
	}
}

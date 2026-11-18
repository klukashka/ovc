package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestHomeRemovedParticipant(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.homeStudentWorld(t)
	parties := world.parties
	removedCard := h.newCard(t, parties.tutorToken, keptCardName, newcomerEmail)
	h.mustAnswer(t, h.acceptInvitation(t, linkToken(t, removedCard.Invitation), newcomerEmail), http.StatusAccepted)
	removedLesson := h.scheduled(t, parties.tutorToken, oneOffFor(removedCard.ID, "2030-03-10", "20:00")).Lessons[0].ID
	newcomerToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	before := h.home(t, newcomerToken, "")

	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.mustAnswer(t, h.removeStudent(t, parties.tutorToken, removedCard.ID), http.StatusOK)
	h.tick(t)
	student := h.home(t, parties.studentToken, "")
	tutor := h.home(t, parties.tutorToken, "")
	newcomer := h.homeAnswer(t, newcomerToken, "")

	if !strings.Contains(compactValue(t, before["next_lesson"]), removedLesson) {
		t.Fatalf("the provisional link's lesson is missing before the removal: %s", before["next_lesson"])
	}
	h.assertStudentHomeMatchesReads(t, parties.studentToken, "", student)
	if found := namedIn(compactValue(t, student["tutors"]), parties.card.ID); len(found) != 0 {
		t.Errorf("the ended link stays among the current tutors: %s", student["tutors"])
	}
	dayStart := homeDayStart(h.homeNow(t))
	calendar, items := h.calendarItems(t, parties.tutorToken, dayStart, dayStart.Add(24*time.Hour), "")
	var today []string
	for index, item := range items {
		if !item.StartsAt.Before(dayStart) {
			today = append(today, calendar[index])
		}
	}
	if got := compactItems(t, tutor["today_lessons"]); !slices.Equal(got, today) {
		t.Errorf("today_lessons after the ending is %v, want %v", got, today)
	}
	unread := decodedAs[map[string]json.RawMessage](t, tutor["unread_channels"])
	if got, want := normalisedItems(t, unread["items"]), h.expectedUnreadChannels(t, parties.tutorToken); !slices.Equal(got, want) {
		t.Errorf("unread_channels after the ending is %v, want %v", got, want)
	}
	if newcomer.status != http.StatusOK {
		t.Fatalf("the removed newcomer's home answered %d %s", newcomer.status, newcomer.body)
	}
	parts := decodedAs[map[string]json.RawMessage](t, json.RawMessage(newcomer.body))
	for _, empty := range []string{"tutors", "next_seven_days", "recently_checked"} {
		if compactValue(t, parts[empty]) != "[]" {
			t.Errorf("the removed newcomer's %s is %s", empty, parts[empty])
		}
	}
	if compactValue(t, parts["next_lesson"]) != "null" || strings.Contains(newcomer.body, removedLesson) || strings.Contains(newcomer.body, removedCard.ID) {
		t.Errorf("the removed newcomer still sees the link or its lesson: %s", newcomer.body)
	}
}

func TestHomeGroupRemoval(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	leaving := h.newGroup(t, parties.tutorToken, "A class to leave", []string{parties.card.ID})
	leavingLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(leaving.ID, "2030-03-10", "15:00")).Lessons[0].ID
	inGroup := h.homeAnswer(t, parties.studentToken, "")

	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, leaving.ID, "", `{"remove":["`+parties.card.ID+`"]}`), http.StatusOK)
	h.tick(t)
	leftGroup := h.home(t, parties.studentToken, "")

	if !strings.Contains(inGroup.body, leavingLesson) {
		t.Fatalf("the group lesson is missing before the removal: %s", inGroup.body)
	}
	h.assertStudentHomeMatchesReads(t, parties.studentToken, "", leftGroup)
	if found := namedIn(compactValue(t, leftGroup["next_lesson"])+compactValue(t, leftGroup["next_seven_days"]), leavingLesson); len(found) != 0 {
		t.Errorf("the removed member still sees the group's future lesson: %v", found)
	}
	if count := decodedAs[int64](t, leftGroup["today_lessons_count"]); count != 0 {
		t.Errorf("today_lessons_count after leaving the group is %d, want 0", count)
	}
}

func TestHomeUnconfirmedCaller(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, unconfirmedTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "active-tutor@example.org")
	pendingTutor, pendingToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "pending-tutor@example.org")
	provisional := h.linkedCard(t, tutor, 20, domain.RelationshipProvisional)
	student := domain.UserID{20}
	pendingCard := h.seedCard(t, pendingTutor, 21, domain.RelationshipActive, &student)
	provisionalLesson := h.scheduled(t, tutorToken, oneOffFor(provisional, "2030-03-10", "14:00")).Lessons[0].ID
	pendingLesson := h.scheduled(t, pendingToken, oneOffFor(pendingCard, "2030-03-10", "15:00")).Lessons[0].ID
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", pendingCard)
	studentToken := h.seededStudentToken(t, 20)

	tutorParts := h.home(t, unconfirmedTutor, "")
	studentParts := h.home(t, studentToken, "")
	refusals := map[string]response{
		"review queue":  h.send(t, http.MethodGet, reviewQueueRoute, "", jsonCall(unconfirmedTutor)),
		"issues":        h.send(t, http.MethodGet, issuesRoute, "", jsonCall(unconfirmedTutor)),
		"join requests": h.send(t, http.MethodGet, joinRequestsPath, "", jsonCall(unconfirmedTutor)),
	}

	assertTutorHomeShape(t, tutorParts)
	for _, refused := range []string{"needs_checking", "upcoming_deadlines", "join_requests"} {
		if string(tutorParts[refused]) != "null" {
			t.Errorf("the unconfirmed tutor's %s is %s, want null", refused, tutorParts[refused])
		}
	}
	steps := decodedAs[map[string]json.RawMessage](t, decodedAs[map[string]json.RawMessage](t, tutorParts["checklist"])["steps"])
	if string(steps["student"]) != "null" || string(steps["lesson"]) != "false" {
		t.Errorf("the unconfirmed tutor's steps are %v", steps)
	}
	if got := compactValue(t, tutorParts["today_lessons"]); got != "[]" {
		t.Errorf("the unconfirmed tutor's today_lessons is %s", got)
	}
	unread := decodedAs[map[string]json.RawMessage](t, tutorParts["unread_channels"])
	if got, want := normalisedItems(t, unread["items"]), h.expectedUnreadChannels(t, unconfirmedTutor); !slices.Equal(got, want) ||
		compactValue(t, unread["unread_total"]) != compactValue(t, h.routeBody(t, unconfirmedTutor, channelsRoute+"/unread")["unread_channels"]) {
		t.Errorf("the unconfirmed tutor's unread_channels is %s, want %v", tutorParts["unread_channels"], want)
	}
	code := h.send(t, http.MethodGet, "/api/v1/tutor/code", "", jsonCall(unconfirmedTutor))
	if compactValue(t, tutorParts["tutor_code"]) != compactValue(t, json.RawMessage(code.body)) {
		t.Errorf("the unconfirmed tutor's code is %s, want %s", tutorParts["tutor_code"], code.body)
	}
	for name, refusal := range refusals {
		if refusal.status != http.StatusForbidden {
			t.Errorf("the unconfirmed tutor's %s answered %d", name, refusal.status)
		}
	}
	h.assertStudentHomeMatchesReads(t, studentToken, "", studentParts)
	body := compactValue(t, studentParts["next_seven_days"]) + compactValue(t, studentParts["tutors"])
	if !strings.Contains(body, provisionalLesson) || !strings.Contains(body, provisional) {
		t.Errorf("the unconfirmed student misses the provisional link: %s", body)
	}
	if found := namedIn(body, pendingCard, pendingLesson); len(found) != 0 {
		t.Errorf("the unconfirmed student sees the reclaim-pending link: %v", found)
	}
}

func TestHomeTutorFilter(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	classmate := h.seedStudentAccount(t, 30, domain.AccountActive)
	classmateCard := h.seedCard(t, parties.tutor, 30, domain.RelationshipActive, &classmate)
	endedTutor := h.seedTutorAccount(t, 31)
	ended := h.seedCard(t, endedTutor, 31, domain.RelationshipEnded, &parties.student)
	discardedTutor := h.seedTutorAccount(t, 32)
	discarded := h.seedCard(t, discardedTutor, 32, domain.RelationshipDiscarded, &parties.student)
	pendingTutor := h.seedTutorAccount(t, 33)
	pending := h.seedCard(t, pendingTutor, 33, domain.RelationshipActive, &parties.student)
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", pending)
	queries := map[string]string{
		"another student's link": homeQuery(classmateCard), "an ended link": homeQuery(ended), "a discarded link": homeQuery(discarded),
		"a reclaim-pending link": homeQuery(pending), "a random identifier": homeQuery(randomSubmissionID), "a malformed value": "?tutor=not-a-uuid",
		"an empty value": "?tutor=", "a repeated value": "?tutor=" + parties.card.ID + "&tutor=" + parties.card.ID,
	}

	answers := map[string]response{}
	for name, query := range queries {
		answers[name] = h.homeAnswer(t, parties.studentToken, query)
	}
	random := answers["a random identifier"]
	fromTutor := h.homeAnswer(t, parties.tutorToken, homeQuery(parties.card.ID))
	emptyFromTutor := h.homeAnswer(t, parties.tutorToken, "?tutor=")
	own := h.homeAnswer(t, parties.studentToken, homeQuery(parties.card.ID))

	want := refusalWithoutRequestID(t, random)
	for name, answer := range answers {
		if answer.status != http.StatusNotFound || refusalWithoutRequestID(t, answer) != want || want.Error.Code != "not_found" {
			t.Errorf("%s answered %d %s, want the random identifier's 404 %s", name, answer.status, answer.body, random.body)
		}
	}
	for name, answer := range map[string]response{"a filter": fromTutor, "an empty filter": emptyFromTutor} {
		if answer.status != http.StatusUnprocessableEntity || !strings.Contains(answer.body, `"query.tutor"`) {
			t.Errorf("a tutor sending %s answered %d %s", name, answer.status, answer.body)
		}
	}
	if own.status != http.StatusOK {
		t.Errorf("the student's own filter answered %d %s", own.status, own.body)
	}
}

func TestHomeRefusesAnonymousCaller(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)

	anonymous := h.send(t, http.MethodGet, homeRoute, "", nil)

	if anonymous.status != http.StatusUnauthorized {
		t.Fatalf("an anonymous home answered %d %s", anonymous.status, anonymous.body)
	}
}

func TestHomeBounds(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	card := toCard(parties.card.ID)
	for range 51 {
		request := oneOffFor(parties.card.ID, "2030-03-10", "14:00")
		request.AllowOverlap = true
		h.scheduled(t, parties.tutorToken, request)
	}
	manual := h.readyTemplate(t, parties.tutorToken, "Trip to Paris", 40).ID
	automatic := h.workTemplate(t, parties.tutorToken, "Automatic", contractAutomaticBlocks()...)
	soon := authStart.Add(48 * time.Hour)
	var hidden []string
	for index := range 9 {
		issue := h.issued(t, parties.tutorToken, issueJSONWith(manual, card, nil, &soon, "after_check")).ID
		if index >= 6 {
			hidden = append(hidden, issue)
			continue
		}
		h.mustAnswer(t, h.submitWork(t, parties.studentToken, h.submissionsOf(t, issue)[0].id, 1), http.StatusOK)
	}
	h.exec(t, "UPDATE issues SET status = 'withdrawn', withdrawn_at = deadline_at WHERE id = $1::uuid", hidden[0])
	h.exec(t, "UPDATE issues SET erase_pending_since = deadline_at WHERE id = $1::uuid", hidden[1])
	h.exec(t, "UPDATE issues SET deadline_at = issued_at - interval '1 hour' WHERE id = $1::uuid", hidden[2])
	for index := range 12 {
		work := h.issued(t, parties.tutorToken, issueJSONWith(automatic, card, nil, nil, "after_check")).ID
		if index >= 6 {
			h.mustAnswer(t, h.submitWork(t, parties.studentToken, h.submissionsOf(t, work)[0].id, 1), http.StatusOK)
		}
	}
	for index := range 4 {
		h.newGroup(t, parties.tutorToken, fmt.Sprintf("Class %d", index), []string{parties.card.ID})
	}
	channels := decodedAs[[]struct {
		ID string `json:"id"`
	}](t, h.routeBody(t, parties.studentToken, channelsRoute)["items"])
	for _, channel := range channels {
		h.mustAnswer(t, h.sendMessage(t, parties.studentToken, channel.ID, "Hello", nil, ""), http.StatusCreated)
	}
	code := h.storedCode(t, parties.tutor).Display()
	var joiners []domain.UserID
	for index := range 8 {
		joiner, token := h.requestingStudent(t, byte(40+index), fmt.Sprintf("joiner-%d@example.org", index), "A joining student")
		h.mustAnswer(t, h.enterCode(t, token, code), http.StatusOK)
		joiners = append(joiners, joiner)
	}
	h.exec(t, "UPDATE join_requests SET expires_at = created_at WHERE student_id = $1::uuid", joiners[6].String())
	h.exec(t, "UPDATE users SET state_before_block = state, state = 'blocked' WHERE id = $1::uuid", joiners[7].String())

	tutor := h.home(t, parties.tutorToken, "")
	student := h.home(t, parties.studentToken, "")

	if len(channels) != 6 {
		t.Fatalf("the student has %d channels, want 6", len(channels))
	}
	counted := func(label string, raw json.RawMessage, items int, count int64) {
		t.Helper()
		part := decodedAs[map[string]json.RawMessage](t, raw)
		if got := len(compactItems(t, part["items"])); got != items || decodedAs[int64](t, part["count"]) != count {
			t.Errorf("%s lists %d with count %s, want %d of %d", label, got, part["count"], items, count)
		}
	}
	if got := len(compactItems(t, tutor["today_lessons"])); got != 50 || decodedAs[int64](t, tutor["today_lessons_count"]) != 51 {
		t.Errorf("today_lessons lists %d of %s, want 50 of 51", got, tutor["today_lessons_count"])
	}
	counted("needs_checking", tutor["needs_checking"], 5, 6)
	counted("upcoming_deadlines", tutor["upcoming_deadlines"], 5, 6)
	counted("join_requests", tutor["join_requests"], 5, 6)
	unread := decodedAs[map[string]json.RawMessage](t, tutor["unread_channels"])
	if got := len(compactItems(t, unread["items"])); got != 5 || decodedAs[int64](t, unread["unread_total"]) != 6 {
		t.Errorf("unread_channels lists %d of %s, want 5 of 6", got, unread["unread_total"])
	}
	joins := decodedAs[map[string]json.RawMessage](t, tutor["join_requests"])
	page := h.routeBody(t, parties.tutorToken, joinRequestsPath+"?limit=5")
	if compactValue(t, joins["next_cursor"]) != compactValue(t, page["next_cursor"]) || compactValue(t, joins["next_cursor"]) == "null" {
		t.Errorf("join_requests continues at %s, want %s", joins["next_cursor"], page["next_cursor"])
	}
	if got := len(compactItems(t, student["next_seven_days"])); got != 50 || decodedAs[int64](t, student["next_seven_days_count"]) != 51 {
		t.Errorf("next_seven_days lists %d of %s, want 50 of 51", got, student["next_seven_days_count"])
	}
	counted("todo", student["todo"], 5, 7)
	if got := len(compactItems(t, student["recently_checked"])); got != 5 {
		t.Errorf("recently_checked lists %d, want 5", got)
	}
}

func TestHomeStudentTutorsBound(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	student := h.seedStudentAccount(t, 200, domain.AccountActive)
	for index := range 51 {
		id := byte(100 + index)
		h.seedCard(t, h.seedTutorAccount(t, id), id, domain.RelationshipActive, &student)
	}

	parts := h.home(t, h.seededStudentToken(t, 200), "")

	if got := len(compactItems(t, parts["tutors"])); got != 50 || decodedAs[int64](t, parts["tutors_count"]) != 51 {
		t.Errorf("tutors lists %d of %s, want 50 of 51", got, parts["tutors_count"])
	}
}

type homeAccessRecord map[string]json.RawMessage

func homeAccessRecords(t *testing.T, logs string) []homeAccessRecord {
	t.Helper()
	var found []homeAccessRecord
	for line := range strings.Lines(logs) {
		record := decodedAs[homeAccessRecord](t, json.RawMessage(line))
		if string(record["msg"]) == `"http.request.completed"` && string(record["route"]) == `"GET /api/v1/home"` {
			found = append(found, record)
		}
	}
	return found
}

func recordKeys(record homeAccessRecord) []string {
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestHomeAccessRecord(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.homeTutorWorld(t)
	_, unconfirmed := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 9, "unconfirmed-tutor@example.org")

	h.home(t, world.parties.tutorToken, "")
	h.home(t, world.parties.studentToken, homeQuery(world.parties.card.ID))
	h.home(t, unconfirmed, "")
	h.homeAnswer(t, world.parties.studentToken, homeQuery(randomSubmissionID))
	h.homeAnswer(t, world.parties.tutorToken, "?tutor=x")
	h.send(t, http.MethodGet, homeRoute, "", nil)

	records := homeAccessRecords(t, h.logs.String())
	if len(records) != 6 {
		t.Fatalf("found %d home access records, want 6", len(records))
	}
	base := []string{"duration_ms", "level", "method", "msg", "request_id", "route", "status", "time"}
	actor := append(slices.Clone(base), "role", "user_id")
	tutorKeys := append(slices.Clone(actor), "join_requests", "needs_checking", "today_lessons", "unread_channels", "upcoming_deadlines")
	studentKeys := append(slices.Clone(actor), "next_seven_days", "recently_checked", "todo", "tutors")
	unconfirmedKeys := append(slices.Clone(actor), "today_lessons", "unread_channels")
	for index, want := range [][]string{tutorKeys, studentKeys, unconfirmedKeys, actor, actor, base} {
		slices.Sort(want)
		if got := recordKeys(records[index]); !slices.Equal(got, want) {
			t.Errorf("home access record %d carries %v, want %v", index, got, want)
		}
	}
	for _, key := range []string{"today_lessons", "needs_checking", "upcoming_deadlines", "unread_channels", "join_requests"} {
		var count int64
		if err := json.Unmarshal(records[0][key], &count); err != nil {
			t.Errorf("the tutor record's %s is not a number: %s", key, records[0][key])
		}
	}
	if string(records[0]["needs_checking"]) != "1" || string(records[0]["today_lessons"]) != "2" || string(records[1]["tutors"]) != "1" {
		t.Errorf("the records carry %v and %v", records[0], records[1])
	}
	encoded := string(rawOf(t, records))
	if found := namedIn(encoded, world.parties.card.ID, "Trip to Paris", cardName, studentEmail, h.storedCode(t, world.parties.tutor).Display()); len(found) != 0 {
		t.Errorf("a home access record carries %v", found)
	}
}

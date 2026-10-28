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

type overlapDetails struct {
	Lessons []struct {
		Title    string    `json:"title"`
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
	} `json:"lessons"`
	More int `json:"more"`
}

func decodeOverlap(t *testing.T, result response) overlapDetails {
	t.Helper()
	var decoded struct {
		Error struct {
			Code    string         `json:"code"`
			Details overlapDetails `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusConflict || decoded.Error.Code != "lesson_overlap" {
		t.Fatalf("not an overlap refusal: %d %s", result.status, result.body)
	}
	return decoded.Error.Details
}

func TestATutorSchedulesAOneOffLessonInTheirTimezone(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.setTimezone(t, domain.UserID{10}, "Europe/Berlin")

	result := h.createLessons(t, token, oneOffFor(card, "2030-03-12", "18:00"), "")

	created := decodeCreated(t, result)
	if created.Series != nil || len(created.Lessons) != 1 || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("a one-off lesson answered %s", result.body)
	}
	lesson := created.Lessons[0]
	if !lesson.StartsAt.Equal(time.Date(2030, time.March, 12, 15, 0, 0, 0, time.UTC)) || !lesson.EndsAt.Equal(lesson.StartsAt.Add(time.Hour)) {
		t.Fatalf("18:00 in Moscow was stored as %s", lesson.StartsAt)
	}
	if lesson.Timezone != "Europe/Moscow" || lesson.Status != "scheduled" || lesson.SeriesID != nil || lesson.Version != 1 || lesson.Tutor != nil {
		t.Fatalf("the lesson is %+v", lesson)
	}
	if lesson.Audience.Kind != "one_to_one" || lesson.Audience.Label == nil || *lesson.Audience.Label != seededCardName ||
		lesson.Audience.Members == nil || len(*lesson.Audience.Members) != 0 || lesson.Audience.Colour == nil {
		t.Fatalf("the tutor's audience is %+v", lesson.Audience)
	}
	if lesson.OtherTimezone == nil || *lesson.OtherTimezone != "Europe/Berlin" {
		t.Fatalf("the tutor's second timezone is %v", lesson.OtherTimezone)
	}
	if page := decodeLesson(t, h.lessonPage(t, token, lesson.ID)); page.ID != lesson.ID || !page.StartsAt.Equal(lesson.StartsAt) {
		t.Fatalf("the lesson page shows %+v", page)
	}
}

func TestASeriesIsMaterialisedOnCreation(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)

	created := h.scheduled(t, token, oneOffFor(card, "2030-03-11", "18:00").weekly([]int{2, 4}, 8))

	if created.Series == nil || created.Series.Timezone != "Europe/Moscow" || created.Series.Start != "18:00" ||
		created.Series.FirstDate != "2030-03-11" || created.Series.Count == nil || *created.Series.Count != 8 || !slices.Equal(created.Series.Weekdays, []int{2, 4}) {
		t.Fatalf("the series is %+v", created.Series)
	}
	var days []string
	for _, lesson := range created.Lessons {
		local := lesson.StartsAt.In(time.FixedZone("MSK", 3*60*60))
		days = append(days, local.Format("Mon 01-02 15:04"))
		if lesson.SeriesID == nil || *lesson.SeriesID != created.Series.ID {
			t.Fatalf("an occurrence names series %v", lesson.SeriesID)
		}
	}
	want := []string{"Tue 03-12 18:00", "Thu 03-14 18:00", "Tue 03-19 18:00", "Thu 03-21 18:00", "Tue 03-26 18:00", "Thu 03-28 18:00", "Tue 04-02 18:00", "Thu 04-04 18:00"}
	if !slices.Equal(days, want) || h.lessonRows(t, tutor) != 8 {
		t.Fatalf("the occurrences are %v", days)
	}
}

func TestALessonInThePastIsRefused(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)

	oneOff := h.createLessons(t, token, oneOffFor(card, "2030-03-10", "12:00"), "")
	pastSeries := h.createLessons(t, token, oneOffFor(card, "2030-03-01", "10:00").until([]int{1, 3}, "2030-03-08"), "")
	partly := h.createLessons(t, token, oneOffFor(card, "2030-03-04", "10:00").weekly([]int{1}, 2), "")

	for name, refused := range map[string]response{"a one-off lesson at now": oneOff, "a series in the past": pastSeries} {
		if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "lesson_in_past" {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	created := decodeCreated(t, partly)
	if len(created.Lessons) != 2 || created.Lessons[0].StartsAt.Format(time.DateOnly) != "2030-03-11" {
		t.Fatalf("a counted series starting in the past answered %s", partly.body)
	}
	if h.lessonRows(t, tutor) != 2 {
		t.Fatal("a refused lesson was stored")
	}
}

type refusalCase struct {
	request lessonRequest
	status  int
	code    string
}

func TestAudienceStatesAndTheRefusalOrder(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	_, unconfirmed := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 3, "unconfirmed-tutor@example.org")
	active := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	invited := h.seedCard(t, tutor, 11, domain.RelationshipInvited, nil)
	ended := h.seedCard(t, tutor, 12, domain.RelationshipEnded, pointer(h.seedStudentAccount(t, 12, domain.AccountActive)))
	pending := h.seedCard(t, tutor, 13, domain.RelationshipReclaimPending, pointer(h.seedStudentAccount(t, 13, domain.AccountActive)))
	discarded := h.seedCard(t, tutor, 14, domain.RelationshipDiscarded, pointer(h.seedStudentAccount(t, 14, domain.AccountActive)))
	foreign := h.linkedCard(t, intruder, 15, domain.RelationshipActive)
	archived := h.newGroup(t, token, groupName, []string{active}).ID
	if result := h.groupCall(t, token, http.MethodPost, archived, "/archive", ""); result.status != http.StatusOK {
		t.Fatalf("archiving answered %d %s", result.status, result.body)
	}
	existing := h.scheduled(t, token, oneOffFor(active, "2030-03-12", "18:00"))
	badDate := func(request lessonRequest) lessonRequest { request.Date = "2030-02-31"; return request }
	overlapping := func(request lessonRequest) lessonRequest {
		request.Date = "2030-03-12"
		request.Start = "18:30"
		return request
	}
	cases := map[string]refusalCase{
		"an invited card":                    {oneOffFor(invited, "2030-03-13", "18:00"), http.StatusConflict, "audience_not_active"},
		"an ended relationship":              {oneOffFor(ended, "2030-03-13", "18:00"), http.StatusConflict, "audience_not_active"},
		"a reclaim-pending link":             {oneOffFor(pending, "2030-03-13", "18:00"), http.StatusConflict, "audience_not_active"},
		"an archived group":                  {groupLessonFor(archived, "2030-03-13", "18:00"), http.StatusConflict, "audience_not_active"},
		"a discarded card":                   {oneOffFor(discarded, "2030-03-13", "18:00"), http.StatusNotFound, "not_found"},
		"another tutor's card":               {oneOffFor(foreign, "2030-03-13", "18:00"), http.StatusNotFound, "not_found"},
		"an out-of-scope card with bad body": {badDate(oneOffFor(foreign, "2030-03-13", "18:00")), http.StatusNotFound, "not_found"},
		"an ended card with a bad body":      {badDate(oneOffFor(ended, "2030-03-13", "18:00")), http.StatusConflict, "audience_not_active"},
		"a bad body that also overlaps":      {badDate(overlapping(oneOffFor(active, "2030-03-13", "18:00"))), http.StatusUnprocessableEntity, "validation_failed"},
		"an overlap":                         {overlapping(oneOffFor(active, "", "")), http.StatusConflict, "lesson_overlap"},
	}
	cases["a past lesson"] = refusalCase{oneOffFor(active, "2030-03-10", "11:00"), http.StatusUnprocessableEntity, "lesson_in_past"}

	for name, testCase := range cases {
		result := h.createLessons(t, token, testCase.request, "")

		if result.status != testCase.status || code(t, result) != testCase.code {
			t.Errorf("%s answered %d %s, want %d %s", name, result.status, result.body, testCase.status, testCase.code)
		}
	}
	if result := h.createLessons(t, unconfirmed, oneOffFor(foreign, "2030-03-13", "18:00"), ""); result.status != http.StatusForbidden || code(t, result) != "email_not_verified" {
		t.Fatalf("an unconfirmed tutor answered %d %s", result.status, result.body)
	}
	replayKey := "replay-before-the-limit"
	h.scheduled(t, token, lessonRequest{RelationshipID: &active, Title: lessonTitle, Date: "2030-03-20", Start: "08:00", Duration: 15})
	first := h.createLessons(t, token, oneOffFor(active, "2030-03-21", "08:00"), replayKey)
	attempts := len(cases) + 3
	for range domain.LessonWritesPerHour - attempts {
		if refused := h.createLessons(t, token, oneOffFor(foreign, "2030-05-01", "18:00"), ""); refused.status != http.StatusNotFound {
			t.Fatalf("a refused attempt below the limit answered %d %s", refused.status, refused.body)
		}
	}
	limited := h.createLessons(t, token, oneOffFor(active, "2030-05-01", "18:00"), "")
	replayed := h.createLessons(t, token, oneOffFor(foreign, "2030-05-01", "18:00"), replayKey)
	limitedAgain := h.createLessons(t, token, oneOffFor(active, "2030-05-02", "18:00"), "")
	if used := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", "lesson_writes_hour"); used != domain.LessonWritesPerHour {
		t.Fatalf("the hourly counter holds %d slots after refused attempts, a replay and two refusals over the limit", used)
	}
	if limitedAgain.status != http.StatusTooManyRequests {
		t.Fatalf("the second write over the limit answered %d %s", limitedAgain.status, limitedAgain.body)
	}
	if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Retry-After") == "" {
		t.Fatalf("the write after the hourly limit answered %d %s", limited.status, limited.body)
	}
	if replayed.status != http.StatusCreated || decodeCreated(t, replayed).Lessons[0].ID != decodeCreated(t, first).Lessons[0].ID {
		t.Fatalf("a replay over the limit answered %d %s", replayed.status, replayed.body)
	}
	if existing.Lessons[0].ID == "" {
		t.Fatal("no existing lesson")
	}
}

func TestControlCharactersInTitlesAndDescriptionsAreRefused(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	nulTitle := oneOffFor(card, "2030-03-12", "10:00")
	nulTitle.Title = "A title\u0000with a NUL byte"
	nulDescription := oneOffFor(card, "2030-03-12", "12:00")
	nulDescription.Description = pointer("A description\u0000with a NUL byte")
	tabTitle := oneOffFor(card, "2030-03-12", "14:00")
	tabTitle.Title = "A title\twith a tab"
	lineBreaks := oneOffFor(card, "2030-03-12", "16:00")
	lineBreaks.Description = pointer("First line\nSecond line")

	type refusedText struct {
		result response
		field  string
	}
	refused := map[string]refusedText{
		"a NUL byte in a title":       {h.createLessons(t, token, nulTitle, ""), "body.title:invalid"},
		"a NUL byte in a description": {h.createLessons(t, token, nulDescription, ""), "body.description:invalid"},
		"a tab in a title":            {h.createLessons(t, token, tabTitle, ""), "body.title:invalid"},
	}
	kept := h.createLessons(t, token, lineBreaks, "")

	for name, refusal := range refused {
		if refusal.result.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, refusal.result), []string{refusal.field}) {
			t.Errorf("%s answered %d %s, want %s", name, refusal.result.status, refusal.result.body, refusal.field)
		}
	}
	if kept.status != http.StatusCreated {
		t.Fatalf("a description with line breaks answered %d %s", kept.status, kept.body)
	}
	if h.lessonRows(t, tutor) != 1 {
		t.Fatal("a refused lesson was stored")
	}
}

func TestAProvisionalStudentGetsOneToOneLessonsOnly(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	provisional := h.linkedCard(t, tutor, 10, domain.RelationshipProvisional)

	oneToOne := h.createLessons(t, token, oneOffFor(provisional, "2030-03-12", "18:00").weekly([]int{2}, 2), "")
	group := h.createGroup(t, token, newGroupBody(groupName, []string{provisional}), "")

	if created := decodeCreated(t, oneToOne); len(created.Lessons) != 2 {
		t.Fatalf("a provisional student's series answered %s", oneToOne.body)
	}
	if group.status != http.StatusConflict || code(t, group) != "relationship_not_active" {
		t.Fatalf("a group with a provisional student answered %d %s", group.status, group.body)
	}
}

func TestOverlapIsRefusedUntilAllowed(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	other := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	long := oneOffFor(card, "2030-03-11", "18:00").until([]int{1}, "2030-09-30")
	long.Title = "The weekly series"
	h.scheduled(t, token, long)
	beyond := oneOffFor(other, "2030-07-01", "18:30")
	series := oneOffFor(other, "2030-03-11", "18:30").until([]int{1}, "2030-09-30")
	touching := oneOffFor(other, "2030-07-01", "19:00")

	beyondHorizon := h.createLessons(t, token, beyond, "")
	manyOverlaps := h.createLessons(t, token, series, "")
	touches := h.createLessons(t, token, touching, "")
	series.AllowOverlap = true
	allowed := h.createLessons(t, token, series, "")

	details := decodeOverlap(t, beyondHorizon)
	if len(details.Lessons) != 1 || details.More != 0 || details.Lessons[0].Title != "The weekly series" ||
		!details.Lessons[0].StartsAt.Equal(time.Date(2030, time.July, 1, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("the overlap beyond the horizon answered %+v", details)
	}
	many := decodeOverlap(t, manyOverlaps)
	if len(many.Lessons) != domain.OverlapListLimit || many.More != 30-domain.OverlapListLimit || !many.Lessons[0].StartsAt.Before(many.Lessons[1].StartsAt) {
		t.Fatalf("the series overlaps answered %d lessons and %d more", len(many.Lessons), many.More)
	}
	if touches.status != http.StatusCreated {
		t.Fatalf("a touching lesson answered %d %s", touches.status, touches.body)
	}
	if allowed.status != http.StatusCreated {
		t.Fatalf("an allowed overlap answered %d %s", allowed.status, allowed.body)
	}
	if !strings.Contains(h.logs.String(), `"msg":"lesson.overlap_warned"`) {
		t.Fatal("no lesson.overlap_warned record")
	}
}

func TestOverlapsWithAnotherTutorsLessonsAreNeverChecked(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.shareStudentWith(t, second, domain.UserID{10})
	shared := domain.RelationshipID{0xc0, sharedStudentCard}.String()
	h.scheduled(t, token, oneOffFor(card, "2030-03-12", "18:00"))

	result := h.createLessons(t, secondToken, oneOffFor(shared, "2030-03-12", "18:00"), "")

	if result.status != http.StatusCreated {
		t.Fatalf("the second tutor's lesson at the same time answered %d %s", result.status, result.body)
	}
}

func TestRepeatedIdempotencyKeyReturnsTheFirstResult(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	seriesRequest := oneOffFor(card, "2030-03-11", "18:00").weekly([]int{2, 4}, 4)

	firstOneOff := decodeCreated(t, h.createLessons(t, token, oneOffFor(card, "2030-03-12", "10:00"), "one-off-key"))
	repeatedOneOff := decodeCreated(t, h.createLessons(t, token, oneOffFor(card, "2030-03-15", "11:00"), "one-off-key"))
	firstSeries := decodeCreated(t, h.createLessons(t, token, seriesRequest, "series-key"))
	repeatedSeries := decodeCreated(t, h.createLessons(t, token, seriesRequest.weekly([]int{5}, 2), "series-key"))

	if repeatedOneOff.Series != nil || len(repeatedOneOff.Lessons) != 1 || repeatedOneOff.Lessons[0].ID != firstOneOff.Lessons[0].ID {
		t.Fatalf("the repeated one-off key answered %+v", repeatedOneOff)
	}
	if repeatedSeries.Series == nil || repeatedSeries.Series.ID != firstSeries.Series.ID || len(repeatedSeries.Lessons) != 4 ||
		repeatedSeries.Lessons[3].ID != firstSeries.Lessons[3].ID {
		t.Fatalf("the repeated series key answered %+v", repeatedSeries)
	}
	if h.lessonRows(t, tutor) != 5 || h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid", tutor.String()) != 1 {
		t.Fatal("a repeated key created rows")
	}
	for _, key := range []string{"one-off-key", "series-key"} {
		if strings.Contains(h.logs.String(), key) {
			t.Fatalf("the key %q was logged", key)
		}
	}
}

func (h *authHarness) seedFutureLessons(t *testing.T, tutor domain.UserID, card string, count int) {
	t.Helper()
	h.exec3(t, `INSERT INTO lessons (id, tutor_id, relationship_id, title, starts_at, duration_minutes, status, timezone, created_at, updated_at)
SELECT gen_random_uuid(), $1::uuid, $2::uuid, 'Seeded', timestamptz '2031-01-01T00:00:00Z' + make_interval(hours => item), 15, 'scheduled',
    'Europe/Moscow', now(), now()
FROM generate_series(1, $3::int) AS item`, tutor.String(), card, fmt.Sprint(count))
}

func TestPerTutorCapsAreEnforcedUnderTheProfileLock(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	other := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	h.seedFutureLessons(t, tutor, card, domain.FutureLessonCapPerTutor-1)
	release := h.holdTutorProfile(t, tutor)
	first := h.sendInBackground(http.MethodPost, lessonsRoute, oneOffFor(card, "2030-03-12", "18:00").body(t), jsonCall(token))
	second := h.sendInBackground(http.MethodPost, lessonsRoute, oneOffFor(other, "2030-03-13", "18:00").body(t), jsonCall(token))
	h.waitForLockWaiters(t, 2)

	release()
	answers := []response{awaitAnswer(t, first), awaitAnswer(t, second)}
	badBody := oneOffFor(card, "2030-02-31", "18:00")
	bodyFirst := h.createLessons(t, token, badBody, "")
	capsBeforeOverlap := []response{
		h.createLessons(t, token, oneOffFor(card, "2030-03-12", "18:00"), ""),
		h.createLessons(t, token, oneOffFor(other, "2030-03-13", "18:00"), ""),
	}

	statuses := map[int]int{}
	for _, answer := range answers {
		statuses[answer.status]++
		if answer.status == http.StatusUnprocessableEntity && !slices.Equal(fieldsOf(t, answer), []string{"body:too_many"}) {
			t.Fatalf("the refused creation answered %s", answer.body)
		}
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusUnprocessableEntity] != 1 {
		t.Fatalf("the racing creations at the lesson cap answered %v", statuses)
	}
	if bodyFirst.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, bodyFirst), []string{"body.date:invalid"}) {
		t.Fatalf("a bad body at the cap answered %d %s", bodyFirst.status, bodyFirst.body)
	}
	for _, refused := range capsBeforeOverlap {
		if refused.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, refused), []string{"body:too_many"}) {
			t.Fatalf("a lesson at the cap, overlapping or not, answered %d %s", refused.status, refused.body)
		}
	}
	if h.lessonRows(t, tutor) != domain.FutureLessonCapPerTutor {
		t.Fatalf("the tutor has %d lessons", h.lessonRows(t, tutor))
	}
}

func (h *authHarness) seedSeries(t *testing.T, tutor domain.UserID, card string, count int, end string) {
	t.Helper()
	h.exec4(t, `INSERT INTO lesson_series (id, tutor_id, relationship_id, weekdays, local_start, duration_minutes, timezone, first_date, end_date,
    title, materialised_until, created_at, updated_at)
SELECT gen_random_uuid(), $1::uuid, $2::uuid, '{1}', '09:00', 15, 'Europe/Moscow', '2029-12-01', $4::date, 'Seeded', $4::date, now(), now()
FROM generate_series(1, $3::int)`, tutor.String(), card, fmt.Sprint(count), end)
}

func TestTheOpenSeriesCapCountsOnlySeriesThatHaveNotEnded(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.seedSeries(t, tutor, card, domain.OpenSeriesCapPerTutor, "2031-06-01")
	h.exec2(t, "UPDATE lesson_series SET ended_at = $2::timestamptz WHERE id = (SELECT min(id::text)::uuid FROM lesson_series WHERE tutor_id = $1::uuid)",
		tutor.String(), rfc3339(authStart))
	h.seedSeries(t, tutor, card, 1, "2030-01-01")

	last := h.createLessons(t, token, oneOffFor(card, "2030-03-11", "10:00").weekly([]int{1}, 1), "")
	beyond := h.createLessons(t, token, oneOffFor(card, "2030-03-11", "12:00").weekly([]int{1}, 1), "")
	oneOff := h.createLessons(t, token, oneOffFor(card, "2030-03-12", "12:00"), "")

	if last.status != http.StatusCreated {
		t.Fatalf("the hundredth open series, beside an ended and a past series, answered %d %s", last.status, last.body)
	}
	if beyond.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, beyond), []string{"body:too_many"}) {
		t.Fatalf("a series beyond the series cap answered %d %s", beyond.status, beyond.body)
	}
	if oneOff.status != http.StatusCreated || h.lessonRows(t, tutor) >= domain.FutureLessonCapPerTutor {
		t.Fatalf("a one-off lesson at the series cap answered %d %s", oneOff.status, oneOff.body)
	}
}

type racingLessonWrite struct {
	prepare func(t *testing.T, h *authHarness, token string, card string) (string, string, string)
	lessons int64
}

func racingLessonWrites() map[string]racingLessonWrite {
	scheduledSeries := func(t *testing.T, h *authHarness, token string, card string) string {
		t.Helper()
		return h.scheduled(t, token, oneOffFor(card, "2030-03-11", "18:00").weekly([]int{1}, 3)).Lessons[1].ID
	}
	return map[string]racingLessonWrite{
		"creating a one-off lesson": {prepare: func(t *testing.T, _ *authHarness, _ string, card string) (string, string, string) {
			return http.MethodPost, lessonsRoute, oneOffFor(card, "2030-03-12", "18:00").body(t)
		}},
		"creating a series": {prepare: func(t *testing.T, _ *authHarness, _ string, card string) (string, string, string) {
			return http.MethodPost, lessonsRoute, oneOffFor(card, "2030-03-11", "18:00").weekly([]int{2}, 3).body(t)
		}},
		"editing following lessons": {lessons: 3, prepare: func(t *testing.T, h *authHarness, token string, card string) (string, string, string) {
			return http.MethodPatch, editPath(scheduledSeries(t, h, token, card)), following(1).startingAt("19:00").body(t)
		}},
		"cancelling a lesson": {lessons: 3, prepare: func(t *testing.T, h *authHarness, token string, card string) (string, string, string) {
			return http.MethodPost, cancelPath(scheduledSeries(t, h, token, card)), cancelBody("this", 1)
		}},
	}
}

func TestLessonWritesRacingATutorDeletionOrBlockAnswer401(t *testing.T) {
	t.Parallel()
	for writeName, write := range racingLessonWrites() {
		for changeName, change := range racingStateChanges() {
			t.Run(changeName+" before "+writeName, func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
				method, path, body := write.prepare(t, h, token, card)
				series := h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid", tutor.String())
				release := h.holdTutorProfile(t, tutor)
				changed := change.start(t, h, token)
				h.waitForLockWaiters(t, 1)
				answer := h.sendInBackground(method, path, body, jsonCall(token))
				h.waitForLockWaiters(t, 2)

				release()
				awaitChange(t, changed)
				refused := awaitAnswer(t, answer)

				if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
					t.Fatalf("the write answered %d %s", refused.status, refused.body)
				}
				if h.lessonRows(t, tutor) != write.lessons || h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid", tutor.String()) != series {
					t.Fatal("the refused write stored a lesson or a series")
				}
				if changedRows := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND (schedule_revision > 1 OR cancel_cause = 'tutor')", tutor.String()); changedRows != 0 {
					t.Fatalf("the refused write changed %d lessons", changedRows)
				}
			})
		}
	}
}

func TestChangingAProfileTimezoneMovesNoLesson(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	before := h.scheduled(t, token, oneOffFor(card, "2030-03-11", "18:00").weekly([]int{2}, 2))

	changed := h.send(t, http.MethodPatch, "/api/v1/users/me", `{"timezone":"Asia/Tokyo"}`, jsonCall(token))
	after := decodeLesson(t, h.lessonPage(t, token, before.Lessons[1].ID))

	if changed.status != http.StatusOK {
		t.Fatalf("changing the timezone answered %d %s", changed.status, changed.body)
	}
	if !after.StartsAt.Equal(before.Lessons[1].StartsAt) || after.Timezone != "Europe/Moscow" {
		t.Fatalf("the lesson moved to %s in %s", after.StartsAt, after.Timezone)
	}
}

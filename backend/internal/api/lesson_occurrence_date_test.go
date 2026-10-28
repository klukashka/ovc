package api_test

import (
	"net/http"
	"strings"
	"testing"
)

const (
	movedEarlierDate = "2030-03-14"
	movedBeyondDate  = "2030-04-02"
)

func carriesOccurrenceDate(body string) bool {
	return strings.Contains(body, `"occurrence_date"`)
}

func occurrenceDatesOf(lessons []lessonView) string {
	dates := make([]string, 0, len(lessons))
	for _, lesson := range lessons {
		dates = append(dates, presentOrAbsent(lesson.Occurrence))
	}
	return strings.Join(dates, " ")
}

func TestTutorLessonAnswersCarryTheOriginalOccurrenceDate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	series := h.mondays(t, parties.tutorToken, parties.card, 3)
	oneOff := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0]
	earlier, beyond := series.Lessons[1], series.Lessons[2]
	replayedRequest := oneOffFor(parties.card, mondaySeriesStart, "20:00").weekly([]int{1}, 3)
	keyed := decodeCreated(t, h.createLessons(t, parties.tutorToken, replayedRequest, "occurrence-date-key"))

	replayed := decodeCreated(t, h.createLessons(t, parties.tutorToken, replayedRequest, "occurrence-date-key"))
	split := decodeChanged(t, h.editLesson(t, parties.tutorToken, keyed.Lessons[1].ID, following(1).titled(newLessonTitle)))
	movedEarlier := decodeChanged(t, h.editLesson(t, parties.tutorToken, earlier.ID, thisLesson(1).on(movedEarlierDate)))
	movedBeyond := decodeChanged(t, h.editLesson(t, parties.tutorToken, beyond.ID, thisLesson(1).on(movedBeyondDate)))
	cancelled := decodeChanged(t, h.cancelCall(t, parties.tutorToken, beyond.ID, "this", 2))
	earlierPage := decodeLesson(t, h.lessonPage(t, parties.tutorToken, earlier.ID))
	beyondPage := decodeLesson(t, h.lessonPage(t, parties.tutorToken, beyond.ID))
	oneOffPage := h.lessonPage(t, parties.tutorToken, oneOff.ID)

	if dates := occurrenceDatesOf(series.Lessons); dates != "2030-03-11 2030-03-18 2030-03-25" {
		t.Fatalf("the created series answered the occurrence dates %s", dates)
	}
	if dates := occurrenceDatesOf(replayed.Lessons); dates != "2030-03-11 2030-03-18 2030-03-25" {
		t.Fatalf("the replayed series answered the occurrence dates %s", dates)
	}
	if dates := occurrenceDatesOf(split.Lessons); dates != "2030-03-18 2030-03-25" || split.Series == nil || split.Series.ID == *keyed.Lessons[1].SeriesID {
		t.Fatalf("the split answered the occurrence dates %s in the series %+v", dates, split.Series)
	}
	if dates := occurrenceDatesOf(append(movedEarlier.Lessons, movedBeyond.Lessons...)); dates != "2030-03-18 2030-03-25" {
		t.Fatalf("the moves answered the occurrence dates %s", dates)
	}
	if dates := occurrenceDatesOf(cancelled.Lessons); dates != "2030-03-25" {
		t.Fatalf("the cancellation answered the occurrence dates %s", dates)
	}
	if presentOrAbsent(earlierPage.Occurrence) != "2030-03-18" || earlierPage.StartsAt.Format("2006-01-02") != movedEarlierDate {
		t.Fatalf("the occurrence moved earlier reads %+v", earlierPage)
	}
	if presentOrAbsent(beyondPage.Occurrence) != "2030-03-25" || beyondPage.StartsAt.Format("2006-01-02") != movedBeyondDate ||
		beyondPage.Status != "cancelled" {
		t.Fatalf("the occurrence moved beyond the series reads %+v", beyondPage)
	}
	if oneOffPage.status != http.StatusOK || carriesOccurrenceDate(oneOffPage.body) {
		t.Fatalf("the one-off lesson answered %d %s", oneOffPage.status, oneOffPage.body)
	}
}

func TestStudentLessonResponsesCarryNoOccurrenceDate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	series := h.mondays(t, parties.tutorToken, parties.card, 2)
	groupSeries := h.groupMondays(t, parties.tutorToken, parties.group, 2)
	moved := series.Lessons[1]
	decodeChanged(t, h.editLesson(t, parties.tutorToken, moved.ID, thisLesson(1).on(movedEarlierDate)))

	responses := map[string]response{
		"the first one-to-one occurrence": h.lessonPage(t, parties.studentToken, series.Lessons[0].ID),
		"the moved one-to-one occurrence": h.lessonPage(t, parties.studentToken, moved.ID),
		"the group occurrence":            h.lessonPage(t, parties.studentToken, groupSeries.Lessons[0].ID),
		"the student's calendar":          h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, ""),
		"the tutor's calendar":            h.calendarCall(t, parties.tutorToken, calendarFrom, calendarTo, ""),
	}

	for name, result := range responses {
		if result.status != http.StatusOK || carriesOccurrenceDate(result.body) || !strings.Contains(result.body, `"id"`) {
			t.Errorf("%s answered %d %s", name, result.status, result.body)
		}
	}
}

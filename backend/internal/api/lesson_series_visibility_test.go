package api_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

const tuesday = 2

type seriesOfLesson struct {
	id     *string
	series *lessonSeries
}

func (h *authHarness) seriesOnStudentPage(t *testing.T, student byte, lesson string) seriesOfLesson {
	t.Helper()
	view := decodeLesson(t, h.lessonPage(t, h.studentToken(t, student), lesson))
	return seriesOfLesson{id: view.SeriesID, series: view.Series}
}

func (s seriesOfLesson) is(series string) bool {
	return s.id != nil && *s.id == series && s.series != nil && s.series.ID == series
}

func (s seriesOfLesson) absent() bool {
	return s.id == nil && s.series == nil
}

func TestAFormerMembersRosterLessonCarriesNoSeriesAfterASplit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 2)
	group := h.newGroup(t, token, "Evening club", cards)
	occurrences := h.scheduled(t, token, groupLessonFor(group.ID, "2030-03-12", "18:00").weekly([]int{tuesday}, 4)).Lessons
	cancelled, scheduled := occurrences[2], occurrences[3]
	if answer := h.cancelCall(t, token, cancelled.ID, "this", cancelled.Version); answer.status != http.StatusOK {
		t.Fatalf("cancelling answered %d %s", answer.status, answer.body)
	}
	if removed := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"remove":`+idList(cards[1:])+`}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}
	split := decodeChanged(t, h.editLesson(t, token, occurrences[1].ID, following(occurrences[1].Version).startingAt("19:00")))

	formerMember := h.seriesOnStudentPage(t, 12, cancelled.ID)
	currentMemberCancelled := h.seriesOnStudentPage(t, 11, cancelled.ID)
	currentMemberScheduled := h.seriesOnStudentPage(t, 11, scheduled.ID)

	if split.Series == nil || h.storedLesson(t, cancelled.ID).SeriesID == nil || *h.storedLesson(t, cancelled.ID).SeriesID != split.Series.ID {
		t.Fatalf("the split did not move the cancelled lesson into its new series %+v", split.Series)
	}
	if !formerMember.absent() {
		t.Fatalf("the former member sees the series %v %+v", formerMember.id, formerMember.series)
	}
	if !currentMemberCancelled.is(split.Series.ID) || currentMemberCancelled.series.Start != "19:00" {
		t.Fatalf("the current member's cancelled lesson carries the series %v %+v", currentMemberCancelled.id, currentMemberCancelled.series)
	}
	if !currentMemberScheduled.is(split.Series.ID) {
		t.Fatalf("the current member's scheduled lesson carries the series %v %+v", currentMemberScheduled.id, currentMemberScheduled.series)
	}
}

func TestAnEndedRelationshipsFrozenLessonCarriesNoSeries(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	endedCard, activeCard := h.linkedCard(t, tutor, 13, domain.RelationshipActive), h.linkedCard(t, tutor, 14, domain.RelationshipActive)
	endedSeries := h.scheduled(t, token, oneOffFor(endedCard, "2030-03-12", "18:00").weekly([]int{tuesday}, 3)).Lessons[0]
	activeSeries := h.scheduled(t, token, oneOffFor(activeCard, "2030-03-12", "10:00").weekly([]int{tuesday}, 3)).Lessons[0]
	if ended := h.endAsTutor(t, token, endedCard); ended.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}

	frozen := decodeLesson(t, h.lessonPage(t, h.studentToken(t, 13), endedSeries.ID))
	current := h.seriesOnStudentPage(t, 14, activeSeries.ID)

	if frozen.Status != string(domain.LessonCancelled) || frozen.Series != nil || frozen.SeriesID != nil {
		t.Fatalf("the ended relationship's frozen lesson answers %+v", frozen)
	}
	if !current.is(*activeSeries.SeriesID) {
		t.Fatalf("the active relationship's lesson carries the series %v %+v", current.id, current.series)
	}
}

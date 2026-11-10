package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	studentsRoute       = "/api/v1/students"
	otherGroupName      = "Morning reading group"
	queryCompletedEvent = `"msg":"db.query.completed"`
	historyPageSize     = 3
)

type cardWithLessons struct {
	ID          string       `json:"id"`
	NextLessons []lessonView `json:"next_lessons"`
}

type groupWithLessons struct {
	ID                string       `json:"id"`
	NextLessons       []lessonView `json:"next_lessons"`
	FutureLessonCount int64        `json:"future_lesson_count"`
	HasOpenSeries     bool         `json:"has_open_series"`
}

type listedWithNextLesson struct {
	ID         string      `json:"id"`
	NextLesson *lessonView `json:"next_lesson"`
}

type cardListPage struct {
	Items      []listedWithNextLesson `json:"items"`
	NextCursor *string                `json:"next_cursor"`
}

type groupListPage struct {
	Groups []listedWithNextLesson `json:"groups"`
}

type lessonHistoryPage struct {
	Items      []lessonView `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}

func decodeInto[Body any](t *testing.T, result response) Body {
	t.Helper()
	var body Body
	if err := json.Unmarshal([]byte(result.body), &body); err != nil || result.status != http.StatusOK {
		t.Fatalf("unexpected answer %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the answer has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return body
}

func (h *authHarness) cardWithLessons(t *testing.T, token string, card string) cardWithLessons {
	t.Helper()
	return decodeInto[cardWithLessons](t, h.cardCall(t, token, http.MethodGet, card, "", ""))
}

func (h *authHarness) groupWithLessons(t *testing.T, token string, group string) groupWithLessons {
	t.Helper()
	return decodeInto[groupWithLessons](t, h.groupCall(t, token, http.MethodGet, group, "", ""))
}

func (h *authHarness) cardList(t *testing.T, token string, limit int) cardListPage {
	t.Helper()
	return decodeInto[cardListPage](t, h.send(t, http.MethodGet, studentsRoute+"?limit="+strconv.Itoa(limit), "", jsonCall(token)))
}

func (h *authHarness) groupList(t *testing.T, token string) groupListPage {
	t.Helper()
	return decodeInto[groupListPage](t, h.send(t, http.MethodGet, groupsRoute, "", jsonCall(token)))
}

func (h *authHarness) historyCall(t *testing.T, token string, card string, limit int, cursor string) response {
	t.Helper()
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return h.cardCall(t, token, http.MethodGet, card, "/lessons?"+query.Encode(), "")
}

func (h *authHarness) wholeHistory(t *testing.T, token string, card string) ([]lessonView, int) {
	t.Helper()
	var lessons []lessonView
	pages := 0
	cursor := ""
	for {
		page := decodeInto[lessonHistoryPage](t, h.historyCall(t, token, card, historyPageSize, cursor))
		pages++
		lessons = append(lessons, page.Items...)
		if page.NextCursor == nil {
			return lessons, pages
		}
		if len(page.Items) != historyPageSize || pages > 10 {
			t.Fatalf("page %d holds %d lessons and names a next page", pages, len(page.Items))
		}
		cursor = *page.NextCursor
	}
}

func (h *authHarness) setClockAndSignIn(t *testing.T, instant time.Time, email string) string {
	t.Helper()
	if _, err := h.clock.Set(context.Background(), instant); err != nil {
		t.Fatal(err)
	}
	return accessToken(t, h.login(t, email, accountPassword))
}

func (h *authHarness) cancelLesson(t *testing.T, lesson string) {
	t.Helper()
	h.cancelWithRoster(t, lesson)
}

func lessonIDs(lessons []lessonView) []string {
	ids := make([]string, 0, len(lessons))
	for _, lesson := range lessons {
		ids = append(ids, lesson.ID)
	}
	return ids
}

func newestFirst(lessons []lessonView) []string {
	sorted := slices.Clone(lessons)
	slices.SortFunc(sorted, func(left lessonView, right lessonView) int {
		if order := right.StartsAt.Compare(left.StartsAt); order != 0 {
			return order
		}
		return strings.Compare(right.ID, left.ID)
	})
	return lessonIDs(sorted)
}

func labelOf(lesson lessonView) string {
	if lesson.Audience.Label == nil {
		return ""
	}
	return *lesson.Audience.Label
}

func lessonCursor(micros string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(micros + "." + randomCardID))
}

func TestStudentLessonHistoryPagesNewestFirstWithoutGapsOrRepeats(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	otherGroup := h.newGroup(t, parties.tutorToken, otherGroupName, []string{parties.other}).ID
	series := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00").weekly([]int{2}, 4)).Lessons
	tied := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	sameInstant := oneOffFor(parties.card, "2030-03-13", "18:00")
	sameInstant.AllowOverlap = true
	cancelledOneToOne := h.scheduled(t, parties.tutorToken, sameInstant).Lessons[0]
	h.cancelLesson(t, cancelledOneToOne.ID)
	scheduledGroup := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-14", "18:00")).Lessons[0]
	onRoster := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-15", "18:00")).Lessons[0]
	h.cancelWithRoster(t, onRoster.ID, parties.student, parties.otherStudent)
	offRoster := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-16", "18:00")).Lessons[0]
	h.cancelWithRoster(t, offRoster.ID, parties.otherStudent)
	h.scheduled(t, parties.tutorToken, groupLessonFor(otherGroup, "2030-03-17", "18:00"))
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.other, "2030-03-18", "18:00"))
	expected := newestFirst(append(slices.Clone(series), tied, cancelledOneToOne, scheduledGroup, onRoster))
	tieStraddlesAPage := slices.Index(expected, tied.ID)/historyPageSize != slices.Index(expected, cancelledOneToOne.ID)/historyPageSize
	if !tieStraddlesAPage {
		t.Fatalf("the two lessons at one instant share a page: %v", expected)
	}

	history, pages := h.wholeHistory(t, parties.tutorToken, parties.card)
	refusedCursors := map[string]response{
		"a malformed cursor":       h.historyCall(t, parties.tutorToken, parties.card, historyPageSize, "not-a-cursor"),
		"a cursor after year 9999": h.historyCall(t, parties.tutorToken, parties.card, historyPageSize, lessonCursor("253402300800000000")),
		"a cursor before year 1":   h.historyCall(t, parties.tutorToken, parties.card, historyPageSize, lessonCursor("-62135596800000001")),
		"an overflowing cursor":    h.historyCall(t, parties.tutorToken, parties.card, historyPageSize, lessonCursor("99999999999999999999")),
	}

	if !slices.Equal(lessonIDs(history), expected) || pages != 3 {
		t.Fatalf("the history over %d pages is %v, want %v", pages, lessonIDs(history), expected)
	}
	cancelled := 0
	for _, lesson := range history {
		want := privateCardName
		if lesson.Audience.Kind == "group" {
			want = groupName
		}
		if labelOf(lesson) != want || lesson.Audience.LabelKind != nil || lesson.Tutor != nil || lesson.Audience.Members != nil {
			t.Fatalf("a history item is labelled %+v for the tutor", lesson)
		}
		if lesson.Status == "cancelled" {
			cancelled++
		}
	}
	if cancelled != 2 {
		t.Fatalf("the history holds %d cancelled lessons instead of 2", cancelled)
	}
	for name, refused := range refusedCursors {
		if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "validation_failed" || !strings.Contains(refused.body, "query.cursor") {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
}

func TestLessonHistoryAnswersForAnEndedCardAndNotForADiscardedOne(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	lesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	h.cancelLesson(t, lesson.ID)
	h.exec2(t, "UPDATE relationships SET state = 'ended', ended_at = $2::timestamptz, ended_by = 'tutor' WHERE id = $1::uuid", parties.card, rfc3339(authStart))
	removed, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 12, "removed@example.org")
	discarded := h.seedCard(t, parties.tutor, 12, domain.RelationshipDiscarded, &removed)

	ended := h.historyCall(t, parties.tutorToken, parties.card, historyPageSize, "")
	refused := h.historyCall(t, parties.tutorToken, discarded, historyPageSize, "")
	random := h.historyCall(t, parties.tutorToken, randomCardID, historyPageSize, "")

	if page := decodeInto[lessonHistoryPage](t, ended); !slices.Equal(lessonIDs(page.Items), []string{lesson.ID}) {
		t.Fatalf("the ended card's history is %v", lessonIDs(page.Items))
	}
	if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
		t.Fatalf("the discarded card's history answered %d %s", refused.status, refused.body)
	}
}

type sharedStudentLessons struct {
	parties        visibilityParties
	secondToken    string
	secondCard     string
	secondLesson   lessonView
	firstOneToOne  lessonView
	firstGroup     lessonView
	secondOneToOne lessonView
	laterGroup     lessonView
}

func (h *authHarness) sharedStudentLessons(t *testing.T) sharedStudentLessons {
	t.Helper()
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	secondCard := domain.RelationshipID{0xc0, sharedStudentCard}.String()
	started := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-10", "12:30")).Lessons[0]
	cancelled := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-11", "09:00")).Lessons[0]
	h.cancelLesson(t, cancelled.ID)
	fixture := sharedStudentLessons{
		parties: parties, secondToken: secondToken, secondCard: secondCard,
		secondLesson:   h.scheduled(t, secondToken, oneOffFor(secondCard, "2030-03-11", "12:00")).Lessons[0],
		firstOneToOne:  h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0],
		firstGroup:     h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "12:00")).Lessons[0],
		secondOneToOne: h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-13", "10:00")).Lessons[0],
		laterGroup:     h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-14", "12:00")).Lessons[0],
	}
	fixture.parties.tutorToken = h.setClockAndSignIn(t, started.StartsAt.Add(time.Minute), tutorEmail)
	fixture.secondToken = accessToken(t, h.login(t, secondTutorEmail, accountPassword))
	return fixture
}

func nextLessonOf(t *testing.T, items []listedWithNextLesson, id string) *lessonView {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item.NextLesson
		}
	}
	t.Fatalf("%s is not listed", id)
	return nil
}

func TestNextLessonsOnCardsAndGroupsAreScopedToTheTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	fixture := h.sharedStudentLessons(t)
	parties := fixture.parties

	card := h.cardWithLessons(t, parties.tutorToken, parties.card)
	cards := h.cardList(t, parties.tutorToken, 30)
	group := h.groupWithLessons(t, parties.tutorToken, parties.group)
	groups := h.groupList(t, parties.tutorToken)
	secondCard := h.cardWithLessons(t, fixture.secondToken, fixture.secondCard)
	secondCards := h.cardList(t, fixture.secondToken, 30)
	secondHistory, _ := h.wholeHistory(t, fixture.secondToken, fixture.secondCard)
	refusals := map[string]response{
		"the first tutor's card":    h.cardCall(t, fixture.secondToken, http.MethodGet, parties.card, "", ""),
		"the first tutor's history": h.historyCall(t, fixture.secondToken, parties.card, historyPageSize, ""),
		"the first tutor's group":   h.groupCall(t, fixture.secondToken, http.MethodGet, parties.group, "", ""),
	}

	wantCard := []string{fixture.firstOneToOne.ID, fixture.firstGroup.ID, fixture.secondOneToOne.ID}
	if !slices.Equal(lessonIDs(card.NextLessons), wantCard) {
		t.Fatalf("the card's next lessons are %v, want %v", lessonIDs(card.NextLessons), wantCard)
	}
	if labelOf(card.NextLessons[0]) != privateCardName || labelOf(card.NextLessons[1]) != groupName || card.NextLessons[1].Tutor != nil {
		t.Fatalf("the card's next lessons are labelled %+v", card.NextLessons)
	}
	if next := nextLessonOf(t, cards.Items, parties.card); next == nil || next.ID != fixture.firstOneToOne.ID {
		t.Fatalf("the card's row shows the next lesson %+v", next)
	}
	if next := nextLessonOf(t, cards.Items, parties.other); next == nil || next.ID != fixture.firstGroup.ID {
		t.Fatalf("the other card's row shows the next lesson %+v", next)
	}
	if !slices.Equal(lessonIDs(group.NextLessons), []string{fixture.firstGroup.ID, fixture.laterGroup.ID}) {
		t.Fatalf("the group's next lessons are %v", lessonIDs(group.NextLessons))
	}
	if next := nextLessonOf(t, groups.Groups, parties.group); next == nil || next.ID != fixture.firstGroup.ID {
		t.Fatalf("the group's row shows the next lesson %+v", next)
	}
	if !slices.Equal(lessonIDs(secondCard.NextLessons), []string{fixture.secondLesson.ID}) ||
		!slices.Equal(lessonIDs(secondHistory), []string{fixture.secondLesson.ID}) {
		t.Fatalf("the second tutor sees %v and %v", lessonIDs(secondCard.NextLessons), lessonIDs(secondHistory))
	}
	if next := nextLessonOf(t, secondCards.Items, fixture.secondCard); next == nil || next.ID != fixture.secondLesson.ID {
		t.Fatalf("the second tutor's row shows the next lesson %+v", next)
	}
	for name, refused := range refusals {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" || strings.Contains(refused.body, lessonTitle) {
			t.Errorf("%s answered the second tutor %d %s", name, refused.status, refused.body)
		}
	}
}

type tracedSchedule struct {
	mutex sync.Mutex
	logs  bytes.Buffer
}

func (s *tracedSchedule) Write(p []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.logs.Write(p)
}

func (s *tracedSchedule) queriesDuring(t *testing.T, call func() response) (response, int) {
	t.Helper()
	s.mutex.Lock()
	s.logs.Reset()
	s.mutex.Unlock()
	result := call()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return result, strings.Count(s.logs.String(), queryCompletedEvent)
}

func (s *tracedSchedule) hook(t *testing.T) optionsHook {
	return func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database) {
		traced := slog.New(slog.NewJSONHandler(s, &slog.HandlerOptions{Level: slog.LevelDebug}))
		pool, err := core.OpenPool(context.Background(), core.Settings{DatabaseURL: database.URL, DBPoolSize: 2}, traced, clock.System{}.Now)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		options.Schedule = schedule.New(schedule.Dependencies{
			Store: repositories.NewStore(pool), Clock: shared, Random: rand.Reader, Keys: keys, Logger: logger, Addresses: options.Files,
		})
	}
}

func (h *authHarness) cardsWithLessons(t *testing.T, tutor domain.UserID, token string, first byte, count int) []string {
	t.Helper()
	cards := h.activeCards(t, tutor, first, count)
	for _, card := range cards {
		request := oneOffFor(card, "2030-03-12", "18:00")
		request.AllowOverlap = true
		h.scheduled(t, token, request)
	}
	return cards
}

func (h *authHarness) groupsWithLessons(t *testing.T, token string, first int, count int) {
	t.Helper()
	for index := first; index < first+count; index++ {
		group := h.newGroup(t, token, otherGroupName+" "+strconv.Itoa(index), nil).ID
		request := groupLessonFor(group, "2030-03-13", "18:00")
		request.AllowOverlap = true
		h.scheduled(t, token, request)
	}
}

func everyItemHasANextLesson(items []listedWithNextLesson) bool {
	for _, item := range items {
		if item.NextLesson == nil {
			return false
		}
	}
	return len(items) > 0
}

func TestStudentAndGroupListsReadNextLessonsInOneQueryPerPage(t *testing.T) {
	t.Parallel()
	traced := &tracedSchedule{}
	h := newAuthHarnessStoring(t, nil, api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store }, traced.hook(t))
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.cardsWithLessons(t, tutor, token, 10, 40)
	h.groupsWithLessons(t, token, 0, 1)
	listGroups := func() response { return h.send(t, http.MethodGet, groupsRoute, "", jsonCall(token)) }
	oneGroup, queriesForOneGroup := traced.queriesDuring(t, listGroups)
	h.groupsWithLessons(t, token, 1, 19)

	smallCards, queriesForTwoCards := traced.queriesDuring(t, func() response { return h.send(t, http.MethodGet, studentsRoute+"?limit=2", "", jsonCall(token)) })
	largeCards, queriesForFortyCards := traced.queriesDuring(t, func() response { return h.send(t, http.MethodGet, studentsRoute+"?limit=40", "", jsonCall(token)) })
	twentyGroups, queriesForTwentyGroups := traced.queriesDuring(t, listGroups)

	small, large := decodeInto[cardListPage](t, smallCards), decodeInto[cardListPage](t, largeCards)
	if len(small.Items) != 2 || len(large.Items) != 40 || !everyItemHasANextLesson(small.Items) || !everyItemHasANextLesson(large.Items) {
		t.Fatalf("the card pages list %d and %d cards: %+v", len(small.Items), len(large.Items), large.Items)
	}
	if queriesForTwoCards == 0 || queriesForFortyCards != queriesForTwoCards {
		t.Fatalf("next lessons for 2 cards took %d queries and for 40 cards %d", queriesForTwoCards, queriesForFortyCards)
	}
	few, many := decodeInto[groupListPage](t, oneGroup), decodeInto[groupListPage](t, twentyGroups)
	if len(few.Groups) != 1 || len(many.Groups) != 20 || !everyItemHasANextLesson(few.Groups) || !everyItemHasANextLesson(many.Groups) {
		t.Fatalf("the group lists hold %d and %d groups: %+v", len(few.Groups), len(many.Groups), many.Groups)
	}
	if queriesForOneGroup == 0 || queriesForTwentyGroups != queriesForOneGroup {
		t.Fatalf("next lessons for 1 group took %d queries and for 20 groups %d", queriesForOneGroup, queriesForTwentyGroups)
	}
}

type plannedGroup struct {
	h       *authHarness
	parties visibilityParties
	oneOff  lessonView
	counted createdLessons
	dated   createdLessons
}

func (h *authHarness) plannedGroup(t *testing.T) plannedGroup {
	t.Helper()
	parties := h.visibilityParties(t)
	planned := plannedGroup{
		h: h, parties: parties,
		oneOff:  h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-11", "10:00")).Lessons[0],
		counted: h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00").weekly([]int{2}, 6)),
		dated:   h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00").until([]int{3}, "2030-03-20")),
	}
	h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-11", "12:00"))
	return planned
}

func (g plannedGroup) readAt(t *testing.T, instant time.Time) groupWithLessons {
	t.Helper()
	token := g.h.setClockAndSignIn(t, instant, tutorEmail)
	return g.h.groupWithLessons(t, token, g.parties.group)
}

var (
	afterTheDatedSeries = time.Date(2030, time.March, 21, 12, 0, 0, 0, time.UTC)
	afterTheLastLesson  = time.Date(2030, time.April, 17, 12, 0, 0, 0, time.UTC)
)

func TestGroupDetailCountsFutureLessonsAndOpenSeriesForTheTutor(t *testing.T) {
	t.Parallel()

	t.Run("an empty group", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		parties := h.visibilityParties(t)

		group := h.groupWithLessons(t, parties.tutorToken, parties.group)

		if group.FutureLessonCount != 0 || group.HasOpenSeries || len(group.NextLessons) != 0 {
			t.Fatalf("an empty group shows %+v", group)
		}
	})

	t.Run("a planned group", func(t *testing.T) {
		t.Parallel()
		planned := newAuthHarness(t).plannedGroup(t)

		group := planned.readAt(t, authStart)

		want := []string{planned.oneOff.ID, planned.counted.Lessons[0].ID, planned.dated.Lessons[0].ID, planned.counted.Lessons[1].ID, planned.dated.Lessons[1].ID}
		if group.FutureLessonCount != 9 || !group.HasOpenSeries || !slices.Equal(lessonIDs(group.NextLessons), want) {
			t.Fatalf("the planned group shows %d future lessons, open %t, next %v", group.FutureLessonCount, group.HasOpenSeries, lessonIDs(group.NextLessons))
		}
	})

	t.Run("after a cancellation", func(t *testing.T) {
		t.Parallel()
		planned := newAuthHarness(t).plannedGroup(t)
		planned.h.cancelLesson(t, planned.counted.Lessons[0].ID)

		group := planned.readAt(t, authStart)

		if group.FutureLessonCount != 8 || len(group.NextLessons) != domain.GroupNextLessonCount ||
			group.NextLessons[0].ID != planned.oneOff.ID || group.NextLessons[1].ID != planned.dated.Lessons[0].ID {
			t.Fatalf("after a cancellation the group shows %d, next %v", group.FutureLessonCount, lessonIDs(group.NextLessons))
		}
	})

	t.Run("after the dated series has passed", func(t *testing.T) {
		t.Parallel()
		planned := newAuthHarness(t).plannedGroup(t)

		group := planned.readAt(t, afterTheDatedSeries)

		if group.FutureLessonCount != 4 || !group.HasOpenSeries || group.NextLessons[0].ID != planned.counted.Lessons[2].ID {
			t.Fatalf("after the dated series the group shows %+v", group)
		}
	})

	t.Run("after the counted series is ended", func(t *testing.T) {
		t.Parallel()
		planned := newAuthHarness(t).plannedGroup(t)
		planned.h.exec2(t, "UPDATE lesson_series SET ended_at = $2::timestamptz WHERE id = $1::uuid", planned.counted.Series.ID, rfc3339(authStart))

		group := planned.readAt(t, afterTheDatedSeries)

		if group.HasOpenSeries || group.FutureLessonCount != 4 {
			t.Fatalf("after ending the counted series the group shows %d, open %t", group.FutureLessonCount, group.HasOpenSeries)
		}
	})

	t.Run("after the last lesson", func(t *testing.T) {
		t.Parallel()
		planned := newAuthHarness(t).plannedGroup(t)

		group := planned.readAt(t, afterTheLastLesson)

		if group.FutureLessonCount != 0 || group.HasOpenSeries || len(group.NextLessons) != 0 {
			t.Fatalf("after the last lesson the group shows %+v", group)
		}
	})
}

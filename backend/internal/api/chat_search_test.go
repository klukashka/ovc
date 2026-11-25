package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	searchQueueWait    = 3 * time.Second
	blockingHolderPoll = `SELECT count(*) FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid))`
)

type searchPage struct {
	Items      []chatMessage `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type searchRequest struct {
	Q      string  `json:"q"`
	Cursor *string `json:"cursor,omitempty"`
}

func searchBody(t *testing.T, term string, cursor *string) string {
	t.Helper()
	encoded, err := json.Marshal(searchRequest{Q: term, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func (h *authHarness) searchCall(t *testing.T, token string, channel string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPost, channelsRoute+"/"+channel+"/messages/search", body, jsonCall(token))
}

func (h *authHarness) searched(t *testing.T, token string, channel string, term string, cursor *string) searchPage {
	t.Helper()
	result := h.searchCall(t, token, channel, searchBody(t, term, cursor))
	h.mustAnswer(t, result, http.StatusOK)
	var page searchPage
	if err := json.Unmarshal([]byte(result.body), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func bodiesOf(page searchPage) []string {
	bodies := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Body != nil {
			bodies = append(bodies, *item.Body)
		}
	}
	return bodies
}

func searchSettingsHook(t *testing.T, settings chat.SearchSettings, scheduler *manualScheduler) optionsHook {
	t.Helper()
	return func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database) {
		service := chat.New(chat.Dependencies{
			Store: repositories.NewStore(database.Pool), Clock: shared, Random: randomPerTest(t), Addresses: options.Files,
			Erasure: chat.DefaultErasureSettings(), Logger: logger, Scheduler: scheduler, Search: settings,
		})
		t.Cleanup(service.Close)
		options.Chat = service
	}
}

func newSearchHarness(t *testing.T, settings chat.SearchSettings, scheduler *manualScheduler) *authHarness {
	t.Helper()
	return newAuthHarnessWithDatabase(t, testdb.NewForLockContention(t), flowProxies(), api.NewHandler,
		func(local *teststorage.Storage) storage.Store { return local.Store }, searchSettingsHook(t, settings, scheduler))
}

type messagesTableHolder struct {
	blocked func() int64
	release func()
}

func (h *authHarness) holdMessagesTable(t *testing.T) messagesTableHolder {
	t.Helper()
	transaction, err := h.database.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if err := transaction.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(release)
	if _, err := transaction.Exec(t.Context(), "LOCK TABLE messages IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	var pid int32
	if err := transaction.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	blocked := func() int64 {
		var count int64
		if _, err := transaction.Exec(t.Context(), "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal(err)
		}
		if err := transaction.QueryRow(t.Context(), blockingHolderPoll, pid).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	return messagesTableHolder{blocked: blocked, release: release}
}

func (h *authHarness) awaitBlockedBy(t *testing.T, holder messagesTableHolder, expected int64) {
	t.Helper()
	h.waitFor(t, fmt.Sprintf("%d statements blocked by the held messages table", expected), func() bool {
		return holder.blocked() >= expected
	})
}

func TestSearchFindsOnlyTheCallersChannelsLiveBodies(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	_, otherTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "another-tutor@example.org")
	elsewhere := h.directChannel(t, h.linkedSignedIn(t, otherTutorToken, parties.studentToken, cardName, studentEmail).ID)
	kept := h.sent(t, parties.tutorToken, direct.ID, "The needle in this chat")
	deleted := h.sent(t, parties.tutorToken, direct.ID, "A deleted needle")
	erased := h.sent(t, parties.tutorToken, direct.ID, "An erased needle")
	quoting := h.reply(t, parties.studentToken, direct.ID, "A reply about it", kept.ID, "")
	h.mustAnswer(t, quoting, http.StatusCreated)
	h.sent(t, parties.tutorToken, group.ID, "A needle in the group")
	h.sent(t, otherTutorToken, elsewhere.ID, "A needle with another tutor")
	h.mustAnswer(t, h.deleteMessage(t, parties.tutorToken, deleted.ID), http.StatusNoContent)
	h.exec(t, "UPDATE messages SET body = NULL, erased_at = now() WHERE id = $1::uuid", erased.ID)

	found := h.searched(t, parties.studentToken, direct.ID, "NEEDLE", nil)

	if !slices.Equal(bodiesOf(found), []string{"The needle in this chat"}) || found.NextCursor != nil {
		t.Fatalf("the search found %v with cursor %v", bodiesOf(found), found.NextCursor)
	}
	if found.Items[0].ID != kept.ID || found.Items[0].SentBeforeReclaim {
		t.Fatalf("the result is %+v", found.Items[0])
	}
}

func TestSearchPagesNewestFirstAndRefusesAForgedCursor(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 31)
	forged := "bm90LWEtc2Vx"

	first := h.searched(t, parties.tutorToken, direct.ID, "message", nil)
	second := h.searched(t, parties.tutorToken, direct.ID, "message", first.NextCursor)
	refused := h.searchCall(t, parties.tutorToken, direct.ID, searchBody(t, "message", &forged))

	if len(first.Items) != 30 || first.Items[0].Seq != 31 || first.Items[29].Seq != 2 || first.NextCursor == nil {
		t.Fatalf("the first page holds %d items from %d", len(first.Items), first.Items[0].Seq)
	}
	if len(second.Items) != 1 || second.Items[0].Seq != 1 || second.NextCursor != nil {
		t.Fatalf("the second page is %+v", second)
	}
	if refused.status != http.StatusUnprocessableEntity || !strings.Contains(refused.body, `"path":"body.cursor"`) {
		t.Fatalf("a forged cursor answered %s", describe(refused))
	}
}

func TestSearchMatchesPercentUnderscoreAndBackslashLiterally(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	for _, body := range []string{"Sure at 100%% today", "Use a_b here", "Use axb there", `The path\x stays`, `It ends with \`} {
		h.sent(t, parties.tutorToken, direct.ID, body)
	}

	percent := h.searched(t, parties.tutorToken, direct.ID, "%%", nil)
	underscore := h.searched(t, parties.tutorToken, direct.ID, "a_b", nil)
	backslash := h.searched(t, parties.tutorToken, direct.ID, `\x`, nil)
	trailing := h.searched(t, parties.tutorToken, direct.ID, `with \`, nil)

	if !slices.Equal(bodiesOf(percent), []string{"Sure at 100%% today"}) || !slices.Equal(bodiesOf(underscore), []string{"Use a_b here"}) {
		t.Fatalf("percent found %v and underscore %v", bodiesOf(percent), bodiesOf(underscore))
	}
	if !slices.Equal(bodiesOf(backslash), []string{`The path\x stays`}) || !slices.Equal(bodiesOf(trailing), []string{`It ends with \`}) {
		t.Fatalf("backslash found %v and the trailing backslash %v", bodiesOf(backslash), bodiesOf(trailing))
	}
}

func TestSearchFoldsCyrillicAndLatinCase(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.sent(t, parties.tutorToken, direct.ID, "привет, как дела")
	h.sent(t, parties.tutorToken, direct.ID, "hello world")

	cyrillic := h.searched(t, parties.studentToken, direct.ID, "ПРИВЕТ", nil)
	latin := h.searched(t, parties.studentToken, direct.ID, "Hello", nil)

	if !slices.Equal(bodiesOf(cyrillic), []string{"привет, как дела"}) || !slices.Equal(bodiesOf(latin), []string{"hello world"}) {
		t.Fatalf("Cyrillic found %v and Latin %v", bodiesOf(cyrillic), bodiesOf(latin))
	}
}

func TestSearchRefusesShortLongAndControlTermsWithoutEchoingThem(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	terms := map[string]string{
		"one character":       "ж",
		"one after trimming":  "   ж   ",
		"101 code points":     strings.Repeat("я", 101),
		"a control character": "zq\u0007pr",
	}

	answers := map[string]response{}
	for name, term := range terms {
		answers[name] = h.searchCall(t, parties.tutorToken, direct.ID, searchBody(t, term, nil))
	}

	for name, answer := range answers {
		if answer.status != http.StatusUnprocessableEntity || !strings.Contains(answer.body, `"path":"body.q"`) {
			t.Fatalf("%s answered %s", name, describe(answer))
		}
		if strings.Contains(answer.body, strings.TrimSpace(terms[name])) {
			t.Fatalf("%s echoed its term: %s", name, answer.body)
		}
	}
}

func TestSearchReachesOnlyActiveMembers(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	h.sent(t, parties.tutorToken, group.ID, "Group words")
	_, removedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	removedCard := h.linkedSignedIn(t, parties.tutorToken, removedToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, removedCard.ID)), http.StatusOK)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, removedCard.ID)), http.StatusOK)
	_, outsiderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 6, "outsider-tutor@example.org")
	card, _ := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	squatted := h.directChannel(t, card.ID)
	provisionalToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.sent(t, provisionalToken, squatted.ID, "Provisional words")
	h.sent(t, parties.tutorToken, direct.ID, "Direct words")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	provisional := h.searched(t, provisionalToken, squatted.ID, "words", nil)
	ended := h.searched(t, parties.studentToken, direct.ID, "words", nil)
	owner := h.reclaimed(t, newcomerEmail)
	pending := h.searchCall(t, owner, squatted.ID, searchBody(t, "words", nil))
	outsider := h.searchCall(t, outsiderToken, direct.ID, searchBody(t, "words", nil))
	former := h.searchCall(t, removedToken, group.ID, searchBody(t, "words", nil))
	random := h.searchCall(t, outsiderToken, randomChannelID, searchBody(t, "words", nil))

	if !slices.Equal(bodiesOf(provisional), []string{"Provisional words"}) || !slices.Equal(bodiesOf(ended), []string{"Direct words"}) {
		t.Fatalf("the provisional account found %v and the ended student %v", bodiesOf(provisional), bodiesOf(ended))
	}
	for name, refused := range map[string]response{"a reclaim-pending account": pending, "another tutor": outsider, "a removed member": former} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
}

func TestSearchLimitCountsOnlySearchesOfReadableChannels(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	for range 5 {
		h.sent(t, parties.studentToken, direct.ID, "A write that uses no search")
		h.searchCall(t, parties.studentToken, direct.ID, searchBody(t, "x", nil))
		h.searchCall(t, parties.studentToken, randomChannelID, searchBody(t, "write", nil))
	}

	accepted := 0
	for range domain.MessageSearchesPerMinute {
		if h.searchCall(t, parties.studentToken, direct.ID, searchBody(t, "write", nil)).status == http.StatusOK {
			accepted++
		}
	}
	limited := h.searchCall(t, parties.studentToken, direct.ID, searchBody(t, "write", nil))
	h.advance(t, domain.MessageSearchWindow)
	renewed := h.searchCall(t, parties.studentToken, direct.ID, searchBody(t, "write", nil))

	if accepted != domain.MessageSearchesPerMinute {
		t.Fatalf("only %d searches were accepted", accepted)
	}
	if limited.status != http.StatusTooManyRequests || limited.headers.Get("Retry-After") == "" {
		t.Fatalf("the search past the limit answered %s", describe(limited))
	}
	h.mustAnswer(t, renewed, http.StatusOK)
}

func TestSearchRecordsTheTermsLengthButNeverTheTerm(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	term := "zebrafinch-quartz"
	h.sent(t, parties.tutorToken, direct.ID, "A message without the term")

	h.searched(t, parties.tutorToken, direct.ID, term, nil)
	h.searchCall(t, parties.tutorToken, randomChannelID, searchBody(t, term, nil))

	if h.countLogs(term) != 0 {
		t.Fatal("a log record carries the search term")
	}
	if h.countLogs(`"msg":"http.request.completed"`, `/messages/search`, fmt.Sprintf(`"query_length":%d`, len(term))) != 2 {
		t.Fatal("the access records do not carry the term's length")
	}
	if h.countLogs(`"msg":"chat.searched"`, direct.ID) != 1 {
		t.Fatal("the search was not recorded")
	}
}

func TestSearchResultsCarryTheFieldsOfHistory(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.sent(t, parties.tutorToken, direct.ID, "Fields of a message")

	searched := h.searchCall(t, parties.studentToken, direct.ID, searchBody(t, "fields", nil))
	history := h.send(t, http.MethodGet, channelsRoute+"/"+direct.ID+"/messages", "", jsonCall(parties.studentToken))

	var searchedPage, historyPage struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(searched.body), &searchedPage); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(history.body), &historyPage); err != nil {
		t.Fatal(err)
	}
	if len(searchedPage.Items) != 1 || len(historyPage.Items) != 1 || !slices.Equal(fieldNames(searchedPage.Items[0]), fieldNames(historyPage.Items[0])) {
		t.Fatalf("a result carries %v, a history item %v", searchedPage.Items, historyPage.Items)
	}
	if searched.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the search answered with Cache-Control %q", searched.headers.Get("Cache-Control"))
	}
}

func TestASearchThatOutlivesItsStatementTimeoutAnswersBusy(t *testing.T) {
	t.Parallel()
	h := newSearchHarness(t, chat.SearchSettings{StatementTimeout: 2 * time.Second, QueueWait: searchQueueWait}, &manualScheduler{})
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.sent(t, parties.tutorToken, direct.ID, "Waiting words")
	term := "zebrafinch-waiting"
	holder := h.holdMessagesTable(t)

	answer := h.sendInBackground(http.MethodPost, channelsRoute+"/"+direct.ID+"/messages/search", searchBody(t, term, nil), jsonCall(parties.tutorToken))
	h.awaitBlockedBy(t, holder, 1)
	timedOut := awaitAnswer(t, answer)
	holder.release()

	if timedOut.status != http.StatusServiceUnavailable || timedOut.headers.Get("Retry-After") != "5" {
		t.Fatalf("the search that timed out answered %s", describe(timedOut))
	}
	if h.countLogs(`"msg":"chat.search_timed_out"`, `"level":"WARN"`, direct.ID) != 1 || h.countLogs(term) != 0 {
		t.Fatal("the timeout was not recorded, or a record carries the term")
	}
}

func TestASearchTheClientAbortsEndsQuietly(t *testing.T) {
	t.Parallel()
	h := newSearchHarness(t, chat.SearchSettings{StatementTimeout: time.Hour, QueueWait: searchQueueWait}, &manualScheduler{})
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.sent(t, parties.tutorToken, direct.ID, "Aborted words")
	holder := h.holdMessagesTable(t)
	abortable, abort := context.WithCancel(t.Context())
	aborted := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(abortable, http.MethodPost, h.server.URL+channelsRoute+"/"+direct.ID+"/messages/search",
			strings.NewReader(searchBody(t, "aborted", nil)))
		if err != nil {
			aborted <- err
			return
		}
		for name, value := range jsonCall(parties.tutorToken) {
			request.Header.Set(name, value)
		}
		answer, err := http.DefaultClient.Do(request)
		if err == nil {
			err = answer.Body.Close()
		}
		aborted <- err
	}()
	h.awaitBlockedBy(t, holder, 1)

	abort()
	clientErr := <-aborted
	h.awaitLogs(t, 1, `"msg":"http.request.completed"`, "/messages/search")
	holder.release()

	if clientErr == nil {
		t.Fatal("the aborted search was answered")
	}
	if h.countLogs("channels.messages.search.failed") != 0 || h.countLogs(`"msg":"chat.search_timed_out"`) != 0 {
		t.Fatal("the aborted search was logged as a failure or a timeout")
	}
}

func TestSearchResultsCarryTheReclaimCue(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	h.sent(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), channel.ID, "Provisional words")
	owner := h.reclaimed(t, newcomerEmail)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)

	found := h.searched(t, tutorToken, channel.ID, "provisional", nil)

	if len(found.Items) != 1 || !found.Items[0].SentBeforeReclaim {
		t.Fatalf("the search found %+v", found.Items)
	}
}

func TestAFifthConcurrentSearchAnswersBusyAfterItsQueueWait(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newSearchHarness(t, chat.SearchSettings{StatementTimeout: time.Hour, QueueWait: searchQueueWait}, scheduler)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.sent(t, parties.tutorToken, direct.ID, "Held words")
	holder := h.holdMessagesTable(t)
	held := make([]<-chan pendingAnswer, 0, domain.MessageSearchSlots)
	for range domain.MessageSearchSlots {
		held = append(held, h.sendInBackground(http.MethodPost, channelsRoute+"/"+direct.ID+"/messages/search", searchBody(t, "held", nil), jsonCall(parties.tutorToken)))
	}
	h.awaitBlockedBy(t, holder, domain.MessageSearchSlots)

	fifth := h.sendInBackground(http.MethodPost, channelsRoute+"/"+direct.ID+"/messages/search", searchBody(t, "held", nil), jsonCall(parties.studentToken))
	h.waitFor(t, "the fifth search queued for a slot", func() bool { return scheduler.pendingTimers(searchQueueWait) == 1 })
	scheduler.advance(searchQueueWait)
	busy := awaitAnswer(t, fifth)
	holder.release()
	finished := make([]response, 0, len(held))
	for _, answer := range held {
		finished = append(finished, awaitAnswer(t, answer))
	}

	if busy.status != http.StatusServiceUnavailable || busy.headers.Get("Retry-After") != "5" {
		t.Fatalf("the fifth search answered %s", describe(busy))
	}
	for _, answer := range finished {
		h.mustAnswer(t, answer, http.StatusOK)
	}
	if h.countLogs(`"msg":"chat.search_busy"`, direct.ID) != 1 {
		t.Fatal("the busy refusal was not recorded")
	}
}

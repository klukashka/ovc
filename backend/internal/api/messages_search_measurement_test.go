//go:build messages_search_measurement

package api_test

import (
	"encoding/base64"
	commandline "flag"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
)

var messagesSearchSmoke = commandline.Bool("messages-search-smoke", false, "Exercise the search measurement with a small history; never report performance acceptance")

const (
	searchMeasurementMessages = 50000
	searchSmokeMessages       = 500
	searchTimedCalls          = 20
	searchWarmupCalls         = 3
	seedSearchHistory         = `WITH advanced AS (
    UPDATE channels SET last_seq = last_seq + $3::int, last_activity_at = $4::timestamptz WHERE id = $1::uuid RETURNING last_seq
)
INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT gen_random_uuid(), $1::uuid, series.seq, $2::uuid,
    CASE
        WHEN series.seq = 7 THEN 'Редкостное слово seldomword ' || series.seq
        WHEN series.seq % 10 = 0 THEN 'Частое сообщение Frequent ' || series.seq
        ELSE 'Обычный текст plain message number ' || series.seq
    END,
    1, $4::timestamptz, $4::timestamptz
FROM advanced, generate_series(advanced.last_seq - $3::int + 1, advanced.last_seq) AS series(seq)`
)

type searchShape struct {
	label  string
	term   string
	cursor *string
}

func searchMessagesStatement(t *testing.T) string {
	t.Helper()
	text, err := os.ReadFile("../../queries/messages.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(text)
	start := strings.Index(source, "-- name: SearchMessages :many")
	if start < 0 {
		t.Fatal("the search statement is missing")
	}
	body := source[start+len("-- name: SearchMessages :many"):]
	if end := strings.Index(body, "-- name:"); end >= 0 {
		body = body[:end]
	}
	replacements := []struct {
		pattern *regexp.Regexp
		with    string
	}{
		{regexp.MustCompile(`@channel_id`), "$1::uuid"},
		{regexp.MustCompile(`sqlc\.arg\(pattern\)`), "$2"},
		{regexp.MustCompile(`sqlc\.narg\(before_seq\)`), "$3"},
		{regexp.MustCompile(`sqlc\.arg\(row_limit\)`), "$4"},
	}
	for _, replacement := range replacements {
		body = replacement.pattern.ReplaceAllLiteralString(body, replacement.with)
	}
	return strings.TrimSuffix(strings.TrimSpace(body), ";")
}

func searchCursorAt(seq int) *string {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(seq)))
	return &cursor
}

func explainSearch(t *testing.T, h *authHarness, statement string, channel string, shape searchShape) {
	t.Helper()
	var before *int64
	if shape.cursor != nil {
		decoded, err := base64.RawURLEncoding.DecodeString(*shape.cursor)
		if err != nil {
			t.Fatal(err)
		}
		seq, err := strconv.ParseInt(string(decoded), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		before = &seq
	}
	rows, err := h.database.Pool.Query(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+statement,
		channel, domain.LikeLiteral(shape.term), before, int32(domain.MessageSearchPage+1))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		t.Logf("event=measurement.search_plan shape=%s line=%s", shape.label, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func timeSearches(t *testing.T, h *authHarness, token string, channel string, shape searchShape) {
	t.Helper()
	h.advance(t, domain.MessageSearchWindow)
	latencies := make([]time.Duration, 0, searchTimedCalls)
	for index := range searchWarmupCalls + searchTimedCalls {
		started := (clock.System{}).Now()
		answer := h.searchCall(t, token, channel, searchBody(t, shape.term, shape.cursor))
		elapsed := time.Since(started)
		h.mustAnswer(t, answer, http.StatusOK)
		if index >= searchWarmupCalls {
			latencies = append(latencies, elapsed)
		}
		if index == searchWarmupCalls-1 {
			h.advance(t, domain.MessageSearchWindow)
		}
	}
	slices.Sort(latencies)
	t.Logf("event=measurement.search_summary shape=%s samples=%d median_ms=%.3f p95_ms=%.3f", shape.label, len(latencies),
		float64(latencies[len(latencies)/2])/float64(time.Millisecond), float64(latencies[(len(latencies)*95+99)/100-1])/float64(time.Millisecond))
}

func TestMessagesSearchMeasurement(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	count := searchMeasurementMessages
	if *messagesSearchSmoke {
		count = searchSmokeMessages
	}
	if _, err := h.database.Pool.Exec(t.Context(), seedSearchHistory, channel, parties.tutor.String(), count, authStart); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), "ANALYZE messages"); err != nil {
		t.Fatal(err)
	}
	var collation, ctype string
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT datcollate, datctype FROM pg_database WHERE datname = current_database()").Scan(&collation, &ctype); err != nil {
		t.Fatal(err)
	}
	statement := searchMessagesStatement(t)
	shapes := []searchShape{
		{label: "frequent_first_page", term: "frequent"},
		{label: "rare_oldest", term: "seldomword"},
		{label: "no_match", term: "отсутствует"},
		{label: "two_character_no_match", term: "щъ"},
		{label: "frequent_deep_cursor", term: "frequent", cursor: searchCursorAt(count / 10)},
	}

	t.Logf("event=measurement.search_setup messages=%d smoke=%t collation=%s ctype=%s", count, *messagesSearchSmoke, collation, ctype)
	for _, shape := range shapes {
		explainSearch(t, h, statement, channel, shape)
		timeSearches(t, h, parties.tutorToken, channel, shape)
	}

	if ctype == "C" || ctype == "POSIX" {
		t.Fatalf("the database's character type %s cannot fold Cyrillic", ctype)
	}
}

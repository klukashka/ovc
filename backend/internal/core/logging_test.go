package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
)

type safeBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type gatedWriter struct {
	gate chan struct{}
	out  safeBuffer
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	<-w.gate
	return w.out.Write(p)
}

func TestLongValuesAreTruncatedAndRecordsStayValidJSON(t *testing.T) {
	var out safeBuffer
	logging := core.NewLogging(&out, slog.LevelInfo, core.LogFormatJSON)

	logging.Logger.Info("big.record", slog.String("payload", strings.Repeat("я", 20*1024)))
	closeLogging(t, logging)

	var record struct {
		Msg     string `json:"msg"`
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal([]byte(out.String()), &record); err != nil {
		t.Fatalf("the record is not valid JSON: %v", err)
	}
	if record.Msg != "big.record" || len(record.Payload) > 4*1024+len("...[truncated]") || !strings.HasSuffix(record.Payload, "...[truncated]") || !utf8.ValidString(record.Payload) {
		t.Fatalf("unexpected payload of %d bytes", len(record.Payload))
	}
}

func TestOversizedRecordsAreReplacedByAValidNotice(t *testing.T) {
	var out safeBuffer
	logging := core.NewLogging(&out, slog.LevelInfo, core.LogFormatJSON)
	attrs := make([]slog.Attr, 0, 10)
	for i := range 10 {
		attrs = append(attrs, slog.String("part"+string(rune('a'+i)), strings.Repeat("x", 3*1024)))
	}

	logging.Logger.LogAttrs(context.Background(), slog.LevelInfo, "wide.record", attrs...)
	closeLogging(t, logging)

	var notice struct {
		Msg           string `json:"msg"`
		Event         string `json:"event"`
		OriginalBytes int    `json:"original_bytes"`
	}
	if err := json.Unmarshal([]byte(out.String()), &notice); err != nil {
		t.Fatalf("the notice is not valid JSON: %v", err)
	}
	if notice.Msg != "logging.record_truncated" || notice.Event != "wide.record" || notice.OriginalBytes <= 16*1024 {
		t.Fatalf("unexpected notice %+v", notice)
	}
}

func TestDroppedRecordsAreCountedAndReported(t *testing.T) {
	writer := &gatedWriter{gate: make(chan struct{})}
	logging := core.NewLogging(writer, slog.LevelInfo, core.LogFormatJSON)

	for range 6000 {
		logging.Logger.Info("burst")
	}
	close(writer.gate)
	closeLogging(t, logging)

	written := strings.Count(writer.out.String(), `"msg":"burst"`)
	reported := 0
	for _, line := range strings.Split(strings.TrimSpace(writer.out.String()), "\n") {
		var record struct {
			Dropped int `json:"dropped_records"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid record %q: %v", line, err)
		}
		reported += record.Dropped
	}
	if written+reported != 6000 || reported == 0 {
		t.Fatalf("expected written plus reported dropped records to equal 6000, got %d + %d", written, reported)
	}
}

func TestQueryTracerNeverLogsSQLOrArguments(t *testing.T) {
	database := testdb.New(t)
	var out safeBuffer
	logging := core.NewLogging(&out, slog.LevelDebug, core.LogFormatJSON)
	settings := core.Settings{DatabaseURL: database.URL, DBPoolSize: 2}
	pool, err := core.OpenPool(context.Background(), settings, logging.Logger, clock.System{}.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var echoed string
	queryErr := pool.QueryRow(context.Background(), "SELECT $1::text", "sentinel-argument-7f3a").Scan(&echoed)
	_, failErr := pool.Exec(context.Background(), "SELECT * FROM missing_table_sentinel WHERE note = $1", "sentinel-argument-7f3a")
	closeLogging(t, logging)

	if queryErr != nil || echoed != "sentinel-argument-7f3a" || failErr == nil {
		t.Fatalf("unexpected query results: %v, %q, %v", queryErr, echoed, failErr)
	}
	logged := out.String()
	for _, forbidden := range []string{"sentinel-argument-7f3a", "missing_table_sentinel", "SELECT"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("log contains %q: %s", forbidden, logged)
		}
	}
	if !strings.Contains(logged, "db.query.completed") || !strings.Contains(logged, "sqlstate_42P01") {
		t.Fatalf("expected query events, got %s", logged)
	}
}

func TestTestControlRefusesNonTestDatabaseNames(t *testing.T) {
	production := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_prod", false)
	testDatabase := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_test", false)
	logger := slog.New(slog.DiscardHandler)

	_, productionErr := core.Connect(context.Background(), core.Settings{DatabaseURL: production.URL, DBPoolSize: 1, TestControl: true}, logger, clock.System{}.Now)
	accepted, testErr := core.Connect(context.Background(), core.Settings{DatabaseURL: testDatabase.URL, DBPoolSize: 1, TestControl: true}, logger, clock.System{}.Now)

	var refusal *core.TestControlRefusal
	if !errors.As(productionErr, &refusal) {
		t.Fatalf("expected a refusal for %s, got %v", production.Name, productionErr)
	}
	if testErr != nil {
		t.Fatalf("expected acceptance for %s, got %v", testDatabase.Name, testErr)
	}
	if err := accepted.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTestControlChecksEveryDatabaseHostBeforeConnecting(t *testing.T) {
	cases := map[string]struct {
		url     string
		allowed bool
	}{
		"remote address":           {url: "postgres://ovc@10.255.255.1:5432/ovc_dev"},
		"mixed host list":          {url: "postgres://ovc@127.0.0.1:5432,10.0.0.1:5432/ovc_dev"},
		"host name":                {url: "postgres://ovc@db.example.com/ovc_dev"},
		"keyword form remote host": {url: "host=10.0.0.1 user=ovc dbname=ovc_dev"},
		"query host override":      {url: "postgres://ovc@127.0.0.1:5432/ovc_dev?host=10.0.0.1"},
		"keyword form host list":   {url: "host=127.0.0.1,db.example.com user=ovc dbname=ovc_dev"},
		"non-test database name":   {url: "postgres://ovc@127.0.0.1:5432/ovc_prod"},
		"IPv4-mapped loopback":     {url: "postgres://ovc@[::ffff:127.0.0.1]:5432/ovc_dev", allowed: true},
		"Unix socket in the query": {url: "postgres://ovc@/ovc_dev?host=/tmp", allowed: true},
		"loopback address":         {url: "postgres://ovc@127.0.0.1:5432/ovc_dev", allowed: true},
		"IPv6 loopback":            {url: "postgres://ovc@[::1]:5432/ovc_dev", allowed: true},
		"localhost":                {url: "postgres://ovc@localhost:5432/ovc_dev", allowed: true},
		"Unix socket":              {url: "host=/tmp user=ovc dbname=ovc_dev", allowed: true},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			settings := core.Settings{DatabaseURL: core.NewSecret(testCase.url), TestControl: true}

			err := core.VerifyTestControlHosts(context.Background(), settings)

			var refusal *core.TestControlRefusal
			if testCase.allowed && err != nil {
				t.Fatalf("expected acceptance, got %v", err)
			}
			if !testCase.allowed && !errors.As(err, &refusal) {
				t.Fatalf("expected a refusal, got %v", err)
			}
		})
	}
}

func closeLogging(t *testing.T, logging *core.Logging) {
	t.Helper()
	if err := logging.Close(); err != nil {
		t.Fatal(err)
	}
}

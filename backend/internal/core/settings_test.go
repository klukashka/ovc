package core_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"ovc/backend/internal/core"
)

func lookupFrom(values map[string]string) core.Lookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func validValues() map[string]string {
	databaseURL := url.URL{Scheme: "postgres", User: url.UserPassword("user", "sentinel-password"), Host: "127.0.0.1:5432", Path: "/ovc_dev"}
	return map[string]string{"OVC_DATABASE_URL": databaseURL.String()}
}

func TestLoadAppliesDefaults(t *testing.T) {
	lookup := lookupFrom(validValues())

	api, apiErr := core.Load(lookup, core.ProcessAPI)
	worker, workerErr := core.Load(lookup, core.ProcessWorker)

	if apiErr != nil || workerErr != nil {
		t.Fatalf("unexpected errors: %v, %v", apiErr, workerErr)
	}
	if api.HTTPAddr != "127.0.0.1:8000" || api.DBPoolSize != 20 || worker.DBPoolSize != 10 {
		t.Fatalf("unexpected defaults: %+v / %+v", api, worker)
	}
	if api.LogLevel != slog.LevelInfo || api.LogFormat != core.LogFormatJSON || api.APIDocsEnabled || api.TestControl {
		t.Fatalf("unexpected defaults: %+v", api)
	}
	if api.Clock != core.ClockSystem || api.MeetingProvider != core.MeetingProviderFake || api.MailBackend != core.MailBackendSink {
		t.Fatalf("unexpected defaults: %+v", api)
	}
}

func TestLoadRefusesInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"missing database URL":        {},
		"unknown log level":           {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_LOG_LEVEL": "loud"},
		"unknown test control flag":   {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_TEST_CONTROL": "yes"},
		"unknown docs flag":           {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_API_DOCS_ENABLED": "1"},
		"real meeting provider":       {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_MEETING_PROVIDER": "bbb"},
		"real mail backend":           {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_MAIL_BACKEND": "smtp"},
		"pool size out of range":      {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_DB_POOL_SIZE": "0"},
		"listen address not h:p":      {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_HTTP_ADDR": "localhost"},
		"unencoded slash in password": {"OVC_DATABASE_URL": "postgres://x:4471/tail@127.0.0.1:1/a_test"},
		"unencoded at in password":    {"OVC_DATABASE_URL": "postgres://x:p@q@127.0.0.1/a"},
		"driver host variable":        {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "PGHOST": "10.0.0.1"},
		"driver password variable":    {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "PGPASSWORD": "anything"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			lookup := lookupFrom(values)

			_, err := core.Load(lookup, core.ProcessAPI)

			var settingsError *core.SettingsError
			if !errors.As(err, &settingsError) {
				t.Fatalf("expected a settings error, got %v", err)
			}
		})
	}
}

func TestLoadRefusesUnsafeTestControlCombinations(t *testing.T) {
	cases := map[string]map[string]string{
		"test clock without the flag":     {"OVC_CLOCK": "test"},
		"flag with a non-loopback listen": {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "0.0.0.0:8000"},
		"flag with a host name listen":    {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "localhost:8000"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			for key, value := range extra {
				values[key] = value
			}

			_, err := core.Load(lookupFrom(values), core.ProcessAPI)

			if err == nil {
				t.Fatal("expected a refusal")
			}
		})
	}
}

func TestLoadAcceptsTestControlOnLoopback(t *testing.T) {
	values := validValues()
	values["OVC_TEST_CONTROL"] = "1"
	values["OVC_CLOCK"] = "test"
	values["OVC_HTTP_ADDR"] = "[::1]:8000"

	settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err != nil || !settings.TestControl || settings.Clock != core.ClockTest {
		t.Fatalf("expected acceptance, got %+v, %v", settings, err)
	}
}

func TestLoadErrorsNeverContainValues(t *testing.T) {
	values := map[string]string{"OVC_DATABASE_URL": "", "OVC_LOG_LEVEL": "sentinel-value"}

	_, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err == nil || strings.Contains(err.Error(), "sentinel-value") {
		t.Fatalf("expected an error without the value, got %v", err)
	}
}

func TestSecretIsRedactedEverywhere(t *testing.T) {
	settings, err := core.Load(lookupFrom(validValues()), core.ProcessAPI)
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))

	formatted := fmt.Sprintf("%v %+v %#v %s %q", settings, settings, settings, settings.DatabaseURL, settings.DatabaseURL)
	encoded, encodeErr := json.Marshal(settings)
	logger.Info("settings", slog.Any("settings", settings), slog.Any("url", settings.DatabaseURL))

	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	for name, output := range map[string]string{"fmt": formatted, "json": string(encoded), "slog": logged.String()} {
		if strings.Contains(output, "sentinel-password") {
			t.Fatalf("%s output exposes the secret: %s", name, output)
		}
	}
	if settings.DatabaseURL.Reveal() == "" {
		t.Fatal("the secret value must stay available to Reveal")
	}
}

type settingsHolder struct {
	settings core.Settings
}

func writeFormatted[T any](t *testing.T, out *strings.Builder, format string, value T) {
	t.Helper()
	if _, err := fmt.Fprintf(out, format, value); err != nil {
		t.Fatal(err)
	}
}

func TestSecretInsideAnUnexportedFieldStaysRedacted(t *testing.T) {
	settings, err := core.Load(lookupFrom(validValues()), core.ProcessAPI)
	if err != nil {
		t.Fatal(err)
	}
	holder := settingsHolder{settings: settings}
	var text bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&text, nil))

	var formatted strings.Builder
	for _, verb := range []string{"v", "s", "q", "x", "X", "t", "c", "d", "U", "e", "E", "f", "g", "G", "b", "o", "p"} {
		for _, flag := range []string{"", "+", "#", "-", " ", "0"} {
			writeFormatted(t, &formatted, "%"+flag+verb+" ", holder)
			writeFormatted(t, &formatted, "%"+flag+verb+" ", settings)
		}
	}
	logger.Info("holder", slog.Any("holder", holder))

	for name, output := range map[string]string{"fmt": formatted.String(), "slog text": text.String()} {
		if strings.Contains(output, "sentinel-password") {
			t.Fatalf("%s output exposes the secret: %s", name, output)
		}
	}
}

func TestDatabaseURLRefusalNeverShowsTheValue(t *testing.T) {
	lookup := lookupFrom(map[string]string{"OVC_DATABASE_URL": "postgres://x:4471/sentinel-tail@127.0.0.1:1/a_test"})

	_, err := core.Load(lookup, core.ProcessAPI)

	if err == nil || strings.Contains(err.Error(), "sentinel-tail") {
		t.Fatalf("expected a refusal without the value, got %v", err)
	}
}

func TestEveryVariableIsDocumentedInEnvExample(t *testing.T) {
	example, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, line := range strings.Split(string(example), "\n") {
		name, _, found := strings.Cut(line, "=")
		if found {
			documented[strings.TrimSpace(name)] = true
		}
	}

	var missing []string
	for _, name := range core.VariableNames() {
		if !documented[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("backend/.env.example lacks %v", missing)
	}
}

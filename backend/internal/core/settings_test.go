package core_test

import (
	"bytes"
	"context"
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

func sentinel(name string) string {
	return "sentinel-" + name
}

func validValues() map[string]string {
	databaseURL := url.URL{Scheme: "postgres", User: url.UserPassword("user", "sentinel-password"), Host: "127.0.0.1:5432", Path: "/ovc_dev"}
	values := map[string]string{"OVC_DATABASE_URL": databaseURL.String(), "OVC_PUBLIC_URL": "https://ovc.example"}
	values["OVC_JWT_SECRET"] = sentinel("jwt-signing-material-0123456789")
	values["OVC_STORAGE_ENDPOINT"] = "https://storage.ovc.example"
	values["OVC_STORAGE_REGION"] = "ru-central1"
	values["OVC_STORAGE_BUCKET"] = "ovc-files"
	values["OVC_STORAGE_ACCESS_KEY"] = sentinel("storage-access-key")
	values["OVC_STORAGE_"+"SECRET_KEY"] = sentinel("storage-secret-key")
	values["OVC_FIELD_ENCRYPTION_KEY"] = sentinelFieldKey
	values["OVC_UNSUBSCRIBE_SIGNING_SECRET"] = sentinelUnsubscribeSecret
	return values
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
		"unknown mail backend":        {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_MAIL_BACKEND": "pigeon"},
		"pool size out of range":      {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_DB_POOL_SIZE": "0"},
		"listen address not h:p":      {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "OVC_HTTP_ADDR": "localhost"},
		"unencoded slash in password": {"OVC_DATABASE_URL": "postgres://x:4471/tail@127.0.0.1:1/a_test"},
		"unencoded at in password":    {"OVC_DATABASE_URL": "postgres://x:p@q@127.0.0.1/a"},
		"driver host variable":        {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "PGHOST": "10.0.0.1"},
		"driver password variable":    {"OVC_DATABASE_URL": "postgres://x@127.0.0.1/a", "PGPASSWORD": "anything"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			values["OVC_PUBLIC_URL"] = "https://ovc.example"
			values["OVC_JWT_SECRET"] = sentinel("jwt-signing-material-0123456789")
			lookup := lookupFrom(values)

			_, err := core.Load(lookup, core.ProcessAPI)

			var settingsError *core.SettingsError
			if !errors.As(err, &settingsError) {
				t.Fatalf("expected a settings error, got %v", err)
			}
		})
	}
}

func TestLoadRefusesTheBigBlueButtonProviderInEveryProcess(t *testing.T) {
	for name, process := range map[string]core.Process{"api": core.ProcessAPI, "worker": core.ProcessWorker, "admin": core.ProcessAdmin} {
		t.Run(name, func(t *testing.T) {
			lookup := lookupFrom(map[string]string{
				"OVC_DATABASE_URL":     "postgres://x@127.0.0.1/a",
				"OVC_PUBLIC_URL":       "https://ovc.example",
				"OVC_JWT_SECRET":       sentinel("jwt-signing-material-0123456789"),
				"OVC_MEETING_PROVIDER": "bbb",
			})

			_, err := core.Load(lookup, process)

			var settingsError *core.SettingsError
			if !errors.As(err, &settingsError) || !strings.Contains(err.Error(), "OVC_MEETING_PROVIDER") {
				t.Fatalf("expected a refusal naming the provider setting, got %v", err)
			}
		})
	}
}

func TestLoadRefusesUnsafeTestControlCombinations(t *testing.T) {
	cases := map[string]map[string]string{
		"test clock without the flag":      {"OVC_CLOCK": "test"},
		"flag with a non-loopback listen":  {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "0.0.0.0:8000"},
		"flag with a host name listen":     {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "localhost:8000"},
		"flag with remote storage":         {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "127.0.0.1:8000", "OVC_STORAGE_ENDPOINT": "https://storage.ovc.example"},
		"flag with storage by host name":   {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "127.0.0.1:8000", "OVC_STORAGE_ENDPOINT": "http://localhost:58333"},
		"flag with storage on any address": {"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "127.0.0.1:8000", "OVC_STORAGE_ENDPOINT": "https://0.0.0.0:58333"},
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

func TestLoadRefusesTestControlWithSMTPMail(t *testing.T) {
	values := validValues()
	for key, value := range map[string]string{
		"OVC_TEST_CONTROL": "1", "OVC_HTTP_ADDR": "127.0.0.1:8000", "OVC_STORAGE_ENDPOINT": "http://127.0.0.1:58333",
		"OVC_MAIL_BACKEND": "smtp", "OVC_SMTP_HOST": "smtp.example", "OVC_MAIL_FROM": "noreply@ovc.example",
	} {
		values[key] = value
	}

	_, err := core.Load(lookupFrom(values), core.ProcessAPI)

	var settingsError *core.SettingsError
	if !errors.As(err, &settingsError) || !strings.Contains(err.Error(), "OVC_MAIL_BACKEND") {
		t.Fatalf("expected a refusal naming OVC_MAIL_BACKEND, got %v", err)
	}
}

func TestLoadChecksTheFakeRoomOrigin(t *testing.T) {
	cases := map[string]struct {
		flag     string
		origin   string
		accepted bool
	}{
		"without the flag": {"0", "https://127.0.0.1:18900", false},
		"a remote origin":  {"1", "https://room.ovc.example:18900", false},
		"a host name":      {"1", "https://localhost:18900", false},
		"another address":  {"1", "https://127.0.0.2:18900", false},
		"a path":           {"1", "https://127.0.0.1:18900/room", false},
		"an empty query":   {"1", "https://127.0.0.1:18900?", false},
		"an empty anchor":  {"1", "https://127.0.0.1:18900#", false},
		"another scheme":   {"1", "ftp://127.0.0.1:18900", false},
		"no port":          {"1", "https://127.0.0.1", false},
		"an invalid port":  {"1", "https://127.0.0.1:99999", false},
		"port zero":        {"1", "https://127.0.0.1:0", false},
		"an exact origin":  {"1", "https://127.0.0.1:18900", true},
		"an IPv6 origin":   {"1", "http://[::1]:18900", true},
	}
	for name, testCase := range cases {
		for _, process := range []core.Process{core.ProcessAPI, core.ProcessWorker} {
			t.Run(name+" in "+string(process), func(t *testing.T) {
				values := validValues()
				values["OVC_TEST_CONTROL"] = testCase.flag
				values["OVC_HTTP_ADDR"] = "127.0.0.1:8000"
				values["OVC_STORAGE_ENDPOINT"] = "http://127.0.0.1:58333"
				values["OVC_FAKE_ROOM_ORIGIN"] = testCase.origin

				settings, err := core.Load(lookupFrom(values), process)

				if testCase.accepted {
					if err != nil || settings.FakeRoomOrigin != testCase.origin {
						t.Fatalf("expected acceptance, got %q, %v", settings.FakeRoomOrigin, err)
					}
					return
				}
				var settingsError *core.SettingsError
				if !errors.As(err, &settingsError) || settingsError.Variable != "OVC_FAKE_ROOM_ORIGIN" {
					t.Fatalf("expected a refusal naming the origin, got %v", err)
				}
			})
		}
	}
}

func TestLoadAcceptsTestControlOnLoopback(t *testing.T) {
	values := validValues()
	values["OVC_TEST_CONTROL"] = "1"
	values["OVC_CLOCK"] = "test"
	values["OVC_HTTP_ADDR"] = "[::1]:8000"
	values["OVC_STORAGE_ENDPOINT"] = "http://[::1]:58333"

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

func TestPublicURLMustBeAnHTTPSOrigin(t *testing.T) {
	cases := map[string]struct {
		value       string
		testControl string
		accepted    string
	}{
		"https origin":           {value: "https://ovc.example", accepted: "https://ovc.example"},
		"trailing dot":           {value: "https://ovc.example."},
		"trailing slash":         {value: "https://ovc.example/", accepted: "https://ovc.example"},
		"http with test control": {value: "http://127.0.0.1:4173", testControl: "1", accepted: "http://127.0.0.1:4173"},
		"http without the flag":  {value: "http://ovc.example"},
		"path":                   {value: "https://ovc.example/app"},
		"query":                  {value: "https://ovc.example?next=x"},
		"user info":              {value: "https://user@ovc.example"},
		"missing":                {},
		"scheme without host":    {value: "https:ovc.example"},
		"fragment":               {value: "https://ovc.example#top"},
		"empty port":             {value: "https://ovc.example:"},
		"upper case host":        {value: "https://OVC.Example:8443", accepted: "https://ovc.example:8443"},
		"default https port":     {value: "https://ovc.example:443", accepted: "https://ovc.example"},
		"unicode host":           {value: "https://овц.example"},
		"long IPv6 form":         {value: "https://[2001:DB8:0:0::1]", accepted: "https://[2001:db8::1]"},
		"IPv4 inside IPv6":       {value: "https://[::ffff:1.2.3.4]"},
		"hexadecimal IPv4":       {value: "https://0x7f.0.0.1"},
		"numeric last label":     {value: "https://ovc.123"},
		"padded port":            {value: "https://ovc.example:0443"},
		"short IPv4 form":        {value: "http://127.1:4173", testControl: "1"},
		"loopback IPv6":          {value: "http://[::1]:4173", testControl: "1", accepted: "http://[::1]:4173"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			delete(values, "OVC_PUBLIC_URL")
			if testCase.value != "" {
				values["OVC_PUBLIC_URL"] = testCase.value
			}
			if testCase.testControl != "" {
				values["OVC_TEST_CONTROL"] = testCase.testControl
				values["OVC_STORAGE_ENDPOINT"] = "http://127.0.0.1:58333"
			}

			settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

			if testCase.accepted == "" && err == nil {
				t.Fatalf("accepted %q", settings.PublicURL)
			}
			if testCase.accepted != "" && (err != nil || settings.PublicURL != testCase.accepted) {
				t.Fatalf("got %q, %v; want %q", settings.PublicURL, err, testCase.accepted)
			}
		})
	}
}

func TestWorkerSMTPSettingsAreCompleteAndSafe(t *testing.T) {
	complete := map[string]string{
		"OVC_MAIL_BACKEND": "smtp", "OVC_SMTP_HOST": "smtp.example", "OVC_SMTP_USERNAME": sentinel("smtp-user"),
		"OVC_MAIL_FROM": "OVC <noreply@ovc.example>",
	}
	complete["OVC_SMTP_PASSWORD"] = sentinel("smtp-password")
	cases := map[string]map[string]string{
		"missing host":      {"OVC_SMTP_HOST": ""},
		"host with a port":  {"OVC_SMTP_HOST": "smtp.example:25"},
		"username alone":    {"OVC_SMTP_PASSWORD": ""},
		"two senders":       {"OVC_MAIL_FROM": "a@ovc.example, b@ovc.example"},
		"header injection":  {"OVC_MAIL_FROM": "noreply@ovc.example\r\nBcc: x@y.example"},
		"unknown TLS mode":  {"OVC_SMTP_TLS": "none"},
		"port out of range": {"OVC_SMTP_PORT": "70000"},
	}
	values := validValues()
	for key, value := range complete {
		values[key] = value
	}
	settings, err := core.Load(lookupFrom(values), core.ProcessWorker)
	if err != nil || settings.SMTP.Port != 587 || settings.SMTP.TLS != core.SMTPStartTLS || settings.MailFrom != "noreply@ovc.example" || settings.RequiresTestDatabase() {
		t.Fatalf("complete settings: %+v, %v", settings, err)
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			changed := validValues()
			for key, value := range complete {
				changed[key] = value
			}
			for key, value := range change {
				changed[key] = value
			}

			_, refusal := core.Load(lookupFrom(changed), core.ProcessWorker)

			var settingsError *core.SettingsError
			if !errors.As(refusal, &settingsError) || strings.Contains(refusal.Error(), sentinel("smtp-password")) {
				t.Fatalf("expected a settings error without the password, got %v", refusal)
			}
		})
	}
}

func TestWorkerSMTPCredentialsStayOutOfFormattingAndTheStartupRecord(t *testing.T) {
	values := validValues()
	for key, value := range map[string]string{"OVC_MAIL_BACKEND": "smtp", "OVC_SMTP_HOST": "smtp.example", "OVC_SMTP_USERNAME": sentinel("smtp-user"), "OVC_MAIL_FROM": "noreply@ovc.example"} {
		values[key] = value
	}
	values["OVC_SMTP_PASSWORD"] = sentinel("smtp-password")
	settings, err := core.Load(lookupFrom(values), core.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	var record bytes.Buffer

	core.LogStarted(context.Background(), slog.New(slog.NewJSONHandler(&record, nil)), core.ProcessWorker, settings)
	formatted := fmt.Sprintf("%+v %#v %v", settings, settings, settings.SMTP)

	for _, output := range []string{record.String(), formatted} {
		if strings.Contains(output, sentinel("smtp-password")) || strings.Contains(output, sentinel("smtp-user")) {
			t.Fatalf("credentials leaked into %s", output)
		}
	}
}

func TestOnlyProcessesThatWriteTheSinkRequireATestDatabase(t *testing.T) {
	values := validValues()

	api, apiErr := core.Load(lookupFrom(values), core.ProcessAPI)
	worker, workerErr := core.Load(lookupFrom(values), core.ProcessWorker)

	if apiErr != nil || workerErr != nil || api.RequiresTestDatabase() || !worker.RequiresTestDatabase() {
		t.Fatalf("api %v (%v), worker %v (%v)", api.RequiresTestDatabase(), apiErr, worker.RequiresTestDatabase(), workerErr)
	}
}

func TestSessionSettingsAreBoundedAndOnlyTheAPINeedsThem(t *testing.T) {
	cases := map[string]map[string]string{
		"short signing secret":         {"OVC_JWT_SECRET": "too-short"},
		"missing signing secret":       {"OVC_JWT_SECRET": ""},
		"access lifetime too long":     {"OVC_ACCESS_TOKEN_TTL_SECONDS": "3601"},
		"access lifetime too short":    {"OVC_ACCESS_TOKEN_TTL_SECONDS": "59"},
		"refresh lifetime too long":    {"OVC_REFRESH_TOKEN_TTL_SECONDS": "7776001"},
		"every IPv4 address trusted":   {"OVC_TRUSTED_PROXY_CIDRS": "10.0.0.0/8, 0.0.0.0/0"},
		"every IPv6 address trusted":   {"OVC_TRUSTED_PROXY_CIDRS": "::/0"},
		"halves of every IPv4 address": {"OVC_TRUSTED_PROXY_CIDRS": "0.0.0.0/1,128.0.0.0/1"},
		"a short IPv6 range":           {"OVC_TRUSTED_PROXY_CIDRS": "2000::/3"},
		"an IPv4-mapped range":         {"OVC_TRUSTED_PROXY_CIDRS": "::ffff:10.0.0.0/104"},
		"not a range":                  {"OVC_TRUSTED_PROXY_CIDRS": "10.0.0.1"},
		"negative file quota":          {"OVC_DEFAULT_FILE_QUOTA_BYTES": "-1"},
		"recording quota too large":    {"OVC_DEFAULT_RECORDING_QUOTA_BYTES": "1125899906842625"},
		"quota not a number":           {"OVC_DEFAULT_FILE_QUOTA_BYTES": "5GB"},
	}
	worker := validValues()
	delete(worker, "OVC_JWT_SECRET")
	if _, err := core.Load(lookupFrom(worker), core.ProcessWorker); err != nil {
		t.Fatalf("the worker needed the signing secret: %v", err)
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			for key, value := range change {
				values[key] = value
			}

			_, err := core.Load(lookupFrom(values), core.ProcessAPI)

			var settingsError *core.SettingsError
			if !errors.As(err, &settingsError) {
				t.Fatalf("expected a settings error, got %v", err)
			}
		})
	}
}

func TestPlainHTTPStorageIsAcceptedOnLocalhostInAnyCase(t *testing.T) {
	values := validValues()
	values["OVC_STORAGE_ENDPOINT"] = "http://LocalHost:58333"

	settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err != nil || settings.Storage.Endpoint != "http://LocalHost:58333" {
		t.Fatalf("got %q, %v", settings.Storage.Endpoint, err)
	}
}

func TestTrustedProxyRangesAtTheWidestAllowedPrefixAreAccepted(t *testing.T) {
	values := validValues()
	values["OVC_TRUSTED_PROXY_CIDRS"] = "10.0.0.0/8, 2001:db8::/32"

	settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err != nil || len(settings.TrustedProxies) != 2 {
		t.Fatalf("got %v, %v", settings.TrustedProxies, err)
	}
}

func TestTutorQuotasDefaultToFiveAndFiftyGigabytes(t *testing.T) {
	values := validValues()

	settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err != nil || settings.TutorQuotas != (core.TutorQuotas{FileBytes: 5_000_000_000, RecordingBytes: 50_000_000_000}) {
		t.Fatalf("quotas %+v, %v", settings.TutorQuotas, err)
	}
}

func TestStorageSettingsAreRequiredAndSafe(t *testing.T) {
	cases := map[string]map[string]string{
		"missing endpoint":     {"OVC_STORAGE_ENDPOINT": ""},
		"plain HTTP elsewhere": {"OVC_STORAGE_ENDPOINT": "http://storage.ovc.example"},
		"endpoint with a path": {"OVC_STORAGE_ENDPOINT": "https://storage.ovc.example/bucket"},
		"endpoint with a user": {"OVC_STORAGE_ENDPOINT": "https://key@storage.ovc.example"},
		"in-memory backend":    {"OVC_STORAGE_BACKEND": "memory"},
		"upper-case bucket":    {"OVC_STORAGE_BUCKET": "OVC"},
		"missing region":       {"OVC_STORAGE_REGION": ""},
		"missing access key":   {"OVC_STORAGE_ACCESS_KEY": ""},
		"missing secret key":   {"OVC_STORAGE_SECRET_KEY": ""},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			for key, value := range change {
				values[key] = value
			}

			_, apiErr := core.Load(lookupFrom(values), core.ProcessAPI)
			_, workerErr := core.Load(lookupFrom(values), core.ProcessWorker)

			var settingsError *core.SettingsError
			if !errors.As(apiErr, &settingsError) || !errors.As(workerErr, &settingsError) {
				t.Fatalf("expected settings errors, got %v and %v", apiErr, workerErr)
			}
		})
	}
}

func TestStorageOnLoopbackMayUsePlainHTTPAndKeysStayRedacted(t *testing.T) {
	values := validValues()
	values["OVC_STORAGE_ENDPOINT"] = "http://127.0.0.1:58333"

	settings, err := core.Load(lookupFrom(values), core.ProcessAPI)

	if err != nil || settings.Storage.Endpoint != "http://127.0.0.1:58333" || settings.Storage.Backend != core.StorageBackendS3 {
		t.Fatalf("storage %+v, %v", settings.Storage, err)
	}
	if printed := fmt.Sprintf("%v %+v %#v", settings, settings, settings); strings.Contains(printed, "storage-access-key") || strings.Contains(printed, "storage-secret-key") {
		t.Fatal("a storage key was printed")
	}
}

func TestTheAdministrationCommandNeedsOnlyTheDatabaseAndLogging(t *testing.T) {
	databaseURL := url.URL{Scheme: "postgres", User: url.UserPassword("operator", "sentinel-password"), Host: "127.0.0.1:5432", Path: "/ovc"}
	values := map[string]string{"OVC_DATABASE_URL": databaseURL.String()}

	settings, err := core.Load(lookupFrom(values), core.ProcessAdmin)

	if err != nil || settings.Process != core.ProcessAdmin {
		t.Fatalf("the administration settings needed more than the database: %v", err)
	}
}

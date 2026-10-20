package core

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

type Process string

const (
	ProcessAPI    Process = "api"
	ProcessWorker Process = "worker"
)

type ClockMode string

const (
	ClockSystem ClockMode = "system"
	ClockTest   ClockMode = "test"
)

type MeetingProvider string

const MeetingProviderFake MeetingProvider = "fake"

type MailBackend string

const MailBackendSink MailBackend = "sink"

type LogFormat string

const (
	LogFormatJSON    LogFormat = "json"
	LogFormatConsole LogFormat = "console"
)

const (
	varHTTPAddr        = "OVC_HTTP_ADDR"
	varDatabaseURL     = "OVC_DATABASE_URL"
	varDBPoolSize      = "OVC_DB_POOL_SIZE"
	varLogLevel        = "OVC_LOG_LEVEL"
	varLogFormat       = "OVC_LOG_FORMAT"
	varAPIDocsEnabled  = "OVC_API_DOCS_ENABLED"
	varTestControl     = "OVC_TEST_CONTROL"
	varClock           = "OVC_CLOCK"
	varMeetingProvider = "OVC_MEETING_PROVIDER"
	varMailBackend     = "OVC_MAIL_BACKEND"
	varTestDatabaseURL = "OVC_TEST_DATABASE_ADMIN_URL"
)

func VariableNames() []string {
	return []string{
		varHTTPAddr, varDatabaseURL, varDBPoolSize, varLogLevel, varLogFormat, varAPIDocsEnabled,
		varTestControl, varClock, varMeetingProvider, varMailBackend, varTestDatabaseURL,
	}
}

type Lookup func(name string) (string, bool)

type Settings struct {
	HTTPAddr        string
	DatabaseURL     Secret
	DBPoolSize      int32
	LogLevel        slog.Level
	LogFormat       LogFormat
	APIDocsEnabled  bool
	TestControl     bool
	Clock           ClockMode
	MeetingProvider MeetingProvider
	MailBackend     MailBackend
}

type SettingsError struct {
	Variable string
	Problem  string
}

func (e *SettingsError) Error() string {
	return e.Variable + ": " + e.Problem
}

func LoadFromEnvironment(process Process) (Settings, error) {
	return Load(os.LookupEnv, process)
}

func TestDatabaseAdminURLFromEnvironment() (Secret, error) {
	value, ok := os.LookupEnv(varTestDatabaseURL)
	if !ok || strings.TrimSpace(value) == "" {
		return Secret{}, &SettingsError{Variable: varTestDatabaseURL, Problem: "is required to run the integration tests"}
	}
	return NewSecret(value), nil
}

func Load(lookup Lookup, process Process) (Settings, error) {
	var problems []error
	settings := Settings{}

	settings.HTTPAddr = stringOr(lookup, varHTTPAddr, "127.0.0.1:8000")
	if _, _, err := net.SplitHostPort(settings.HTTPAddr); err != nil {
		problems = append(problems, &SettingsError{Variable: varHTTPAddr, Problem: "must be host:port"})
	}

	databaseURL, ok := lookup(varDatabaseURL)
	if !ok || strings.TrimSpace(databaseURL) == "" {
		problems = append(problems, &SettingsError{Variable: varDatabaseURL, Problem: "is required"})
	}
	settings.DatabaseURL = NewSecret(databaseURL)

	defaultPool := int32(20)
	if process == ProcessWorker {
		defaultPool = 10
	}
	poolSize, err := int32Or(lookup, varDBPoolSize, defaultPool)
	if err != nil {
		problems = append(problems, err)
	}
	settings.DBPoolSize = poolSize

	level, err := logLevelOr(lookup, varLogLevel, slog.LevelInfo)
	if err != nil {
		problems = append(problems, err)
	}
	settings.LogLevel = level

	format, err := oneOf(lookup, varLogFormat, LogFormatJSON, LogFormatJSON, LogFormatConsole)
	if err != nil {
		problems = append(problems, err)
	}
	settings.LogFormat = format

	docs, err := boolOr(lookup, varAPIDocsEnabled, false, "true", "false")
	if err != nil {
		problems = append(problems, err)
	}
	settings.APIDocsEnabled = docs

	testControl, err := boolOr(lookup, varTestControl, false, "1", "0")
	if err != nil {
		problems = append(problems, err)
	}
	settings.TestControl = testControl

	clock, err := oneOf(lookup, varClock, ClockSystem, ClockSystem, ClockTest)
	if err != nil {
		problems = append(problems, err)
	}
	settings.Clock = clock

	provider, err := oneOf(lookup, varMeetingProvider, MeetingProviderFake, MeetingProviderFake)
	if err != nil {
		problems = append(problems, err)
	}
	settings.MeetingProvider = provider

	mail, err := oneOf(lookup, varMailBackend, MailBackendSink, MailBackendSink)
	if err != nil {
		problems = append(problems, err)
	}
	settings.MailBackend = mail

	problems = append(problems, settings.testControlProblems()...)
	if len(problems) > 0 {
		return Settings{}, errors.Join(problems...)
	}
	return settings, nil
}

func (s Settings) testControlProblems() []error {
	var problems []error
	if s.Clock == ClockTest && !s.TestControl {
		problems = append(problems, &SettingsError{Variable: varClock, Problem: "test is allowed only with OVC_TEST_CONTROL=1"})
	}
	if !s.TestControl {
		return problems
	}
	if s.MeetingProvider != MeetingProviderFake {
		problems = append(problems, &SettingsError{Variable: varMeetingProvider, Problem: "must be fake with OVC_TEST_CONTROL=1"})
	}
	if s.MailBackend != MailBackendSink {
		problems = append(problems, &SettingsError{Variable: varMailBackend, Problem: "must be sink with OVC_TEST_CONTROL=1"})
	}
	if !isLoopbackListenAddress(s.HTTPAddr) {
		problems = append(problems, &SettingsError{Variable: varHTTPAddr, Problem: "must be a loopback IP address with OVC_TEST_CONTROL=1"})
	}
	return problems
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func stringOr(lookup Lookup, name string, fallback string) string {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func int32Or(lookup Lookup, name string, fallback int32) (int32, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 1 || parsed > 200 {
		return 0, &SettingsError{Variable: name, Problem: "must be an integer from 1 to 200"}
	}
	return int32(parsed), nil
}

func boolOr(lookup Lookup, name string, fallback bool, trueValue string, falseValue string) (bool, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	switch value {
	case trueValue:
		return true, nil
	case falseValue:
		return false, nil
	}
	return false, &SettingsError{Variable: name, Problem: fmt.Sprintf("must be %s or %s", trueValue, falseValue)}
}

func oneOf[T ~string](lookup Lookup, name string, fallback T, allowed ...T) (T, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	for _, candidate := range allowed {
		if string(candidate) == value {
			return candidate, nil
		}
	}
	names := make([]string, len(allowed))
	for i, candidate := range allowed {
		names[i] = string(candidate)
	}
	return fallback, &SettingsError{Variable: name, Problem: "must be one of " + strings.Join(names, ", ")}
}

func logLevelOr(lookup Lookup, name string, fallback slog.Level) (slog.Level, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return fallback, &SettingsError{Variable: name, Problem: "must be one of debug, info, warn, error"}
}

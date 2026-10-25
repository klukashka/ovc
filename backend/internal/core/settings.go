package core

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Process string

const (
	ProcessAPI    Process = "api"
	ProcessWorker Process = "worker"
	ProcessAdmin  Process = "admin"
)

type ClockMode string

const (
	ClockSystem ClockMode = "system"
	ClockTest   ClockMode = "test"
)

type MeetingProvider string

const MeetingProviderFake MeetingProvider = "fake"

type MailBackend string

const (
	MailBackendSMTP MailBackend = "smtp"
	MailBackendSink MailBackend = "sink"
)

type SMTPTLSMode string

const (
	SMTPStartTLS SMTPTLSMode = "starttls"
	SMTPImplicit SMTPTLSMode = "implicit"
)

type SMTPSettings struct {
	Host     string
	Port     int
	TLS      SMTPTLSMode
	Username Secret
	Password Secret
}

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
	varPublicURL       = "OVC_PUBLIC_URL"
	varSMTPHost        = "OVC_SMTP_HOST"
	varSMTPPort        = "OVC_SMTP_PORT"
	varSMTPTLS         = "OVC_SMTP_TLS"
	varSMTPUsername    = "OVC_SMTP_USERNAME"
	varSMTPPassword    = "OVC_SMTP_PASSWORD"
	varMailFrom        = "OVC_MAIL_FROM"
	varJWTSecret       = "OVC_JWT_SECRET"
	varAccessTTL       = "OVC_ACCESS_TOKEN_TTL_SECONDS"
	varRefreshTTL      = "OVC_REFRESH_TOKEN_TTL_SECONDS"
	varTrustedProxies  = "OVC_TRUSTED_PROXY_CIDRS"
	varTestDatabaseURL = "OVC_TEST_DATABASE_ADMIN_URL"
	varFileQuota       = "OVC_DEFAULT_FILE_QUOTA_BYTES"
	varRecordingQuota  = "OVC_DEFAULT_RECORDING_QUOTA_BYTES"
	varFieldKey        = "OVC_FIELD_ENCRYPTION_KEY"
	varUnsubscribeKey  = "OVC_UNSUBSCRIBE_SIGNING_SECRET"

	minimumTrustedIPv4Bits = 8
	minimumTrustedIPv6Bits = 32
)

func VariableNames() []string {
	return append([]string{
		varHTTPAddr, varDatabaseURL, varDBPoolSize, varLogLevel, varLogFormat, varAPIDocsEnabled,
		varTestControl, varClock, varMeetingProvider, varMailBackend, varPublicURL, varSMTPHost, varSMTPPort,
		varSMTPTLS, varSMTPUsername, varSMTPPassword, varMailFrom, varJWTSecret, varAccessTTL, varRefreshTTL,
		varTrustedProxies, varTestDatabaseURL, varFileQuota, varRecordingQuota, varFieldKey, varUnsubscribeKey,
	}, storageVariableNames()...)
}

type Lookup func(name string) (string, bool)

type Settings struct {
	Process         Process
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
	PublicURL       string
	SMTP            SMTPSettings
	MailFrom        string
	JWTSecret       Secret
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	TrustedProxies  []netip.Prefix
	TutorQuotas     TutorQuotas
	Storage         StorageSettings
	FieldKey        Secret
	UnsubscribeKey  Secret
}

type TutorQuotas struct {
	FileBytes      int64
	RecordingBytes int64
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

func TestDatabaseTLSChosenInEnvironment() bool {
	for _, name := range []string{"PGSSLMODE", "PGSERVICE"} {
		if _, set := os.LookupEnv(name); set {
			return true
		}
	}
	return false
}

func TestStorageFromEnvironment(bucket string) (StorageSettings, error) {
	return testStorageFrom(os.LookupEnv, bucket)
}

func Load(lookup Lookup, process Process) (Settings, error) {
	var problems []error
	settings := Settings{Process: process}

	settings.HTTPAddr = stringOr(lookup, varHTTPAddr, "127.0.0.1:8000")
	if _, _, err := net.SplitHostPort(settings.HTTPAddr); err != nil {
		problems = append(problems, &SettingsError{Variable: varHTTPAddr, Problem: "must be host:port"})
	}

	databaseURL, ok := lookup(varDatabaseURL)
	if !ok || strings.TrimSpace(databaseURL) == "" {
		problems = append(problems, &SettingsError{Variable: varDatabaseURL, Problem: "is required"})
	}
	settings.DatabaseURL = NewSecret(databaseURL)
	if ok {
		problems = append(problems, databaseURLProblems(databaseURL)...)
	}
	problems = append(problems, driverEnvironmentProblems(lookup)...)

	defaultPool := int32(20)
	if process == ProcessAPI {
		problems = append(problems, settings.loadSessionSettings(lookup)...)
	}

	if process == ProcessWorker {
		defaultPool = 10
	}
	if process == ProcessAPI || process == ProcessWorker {
		problems = append(problems, settings.loadStorageSettings(lookup)...)
		problems = append(problems, settings.loadFieldKey(lookup)...)
		problems = append(problems, settings.loadUnsubscribeKey(lookup)...)
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

	mail, err := oneOf(lookup, varMailBackend, MailBackendSink, MailBackendSMTP, MailBackendSink)
	if err != nil {
		problems = append(problems, err)
	}
	settings.MailBackend = mail

	if process != ProcessAdmin {
		publicURL, err := publicURLFrom(lookup, settings.TestControl)
		if err != nil {
			problems = append(problems, err)
		}
		settings.PublicURL = publicURL
	}

	if process == ProcessWorker {
		smtp, from, mailProblems := mailSettings(lookup, settings.MailBackend)
		problems = append(problems, mailProblems...)
		settings.SMTP = smtp
		settings.MailFrom = from
	}

	problems = append(problems, settings.testControlProblems()...)
	if len(problems) > 0 {
		return Settings{}, errors.Join(problems...)
	}
	return settings, nil
}

var driverEnvironmentVariables = []string{
	"PGHOST", "PGHOSTADDR", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGPASSFILE", "PGSERVICE",
	"PGSERVICEFILE", "PGSSLMODE", "PGSSLCERT", "PGSSLKEY", "PGSSLROOTCERT", "PGSSLPASSWORD", "PGSSLSNI",
	"PGSSLNEGOTIATION", "PGAPPNAME", "PGCONNECT_TIMEOUT", "PGTARGETSESSIONATTRS", "PGOPTIONS", "PGTZ",
	"PGMINPROTOCOLVERSION", "PGMAXPROTOCOLVERSION", "PGCHANNELBINDING", "PGREQUIREAUTH",
}

func driverEnvironmentProblems(lookup Lookup) []error {
	var problems []error
	for _, name := range driverEnvironmentVariables {
		if _, set := lookup(name); set {
			problems = append(problems, &SettingsError{Variable: name, Problem: "is read by the database driver outside OVC settings; unset it and put the value in OVC_DATABASE_URL"})
		}
	}
	return problems
}

func databaseURLProblems(value string) []error {
	if !strings.HasPrefix(value, "postgres://") && !strings.HasPrefix(value, "postgresql://") {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return []error{&SettingsError{Variable: varDatabaseURL, Problem: "is not a valid PostgreSQL connection URL"}}
	}
	authority := strings.SplitN(strings.SplitN(value, "://", 2)[1], "/", 2)[0]
	unencodedUserInfo := parsed.User == nil && strings.Contains(value, "@")
	misplacedSeparators := strings.ContainsAny(strings.TrimPrefix(parsed.Path, "/"), "/@") || strings.Count(authority, "@") > 1
	if unencodedUserInfo || misplacedSeparators {
		return []error{&SettingsError{Variable: varDatabaseURL, Problem: "must percent-encode reserved characters such as / @ : in the user name and password"}}
	}
	return nil
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
	if s.Storage.Endpoint != "" && !onLoopbackAddress(s.Storage.Endpoint) {
		problems = append(problems, &SettingsError{Variable: varStorageEndpoint, Problem: "must use a loopback IP address with OVC_TEST_CONTROL=1"})
	}
	return problems
}

func publicURLFrom(lookup Lookup, testControl bool) (string, error) {
	value, ok := lookup(varPublicURL)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &SettingsError{Variable: varPublicURL, Problem: "is required"}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || strings.HasSuffix(parsed.Host, ":") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", &SettingsError{Variable: varPublicURL, Problem: "must be an origin such as https://ovc.example, without a path, query or user info"}
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !testControl) {
		return "", &SettingsError{Variable: varPublicURL, Problem: "must use https; http is allowed only with OVC_TEST_CONTROL=1"}
	}
	host, ok := canonicalHost(parsed.Hostname())
	if !ok {
		return "", &SettingsError{Variable: varPublicURL, Problem: "must use an ASCII host name, a dotted IPv4 address or an IPv6 address; write an internationalised name in its punycode form"}
	}
	port := parsed.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || strconv.Itoa(number) != port || number < 1 || number > 65535 {
			return "", &SettingsError{Variable: varPublicURL, Problem: "must use a port without leading zeros"}
		}
	}
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return parsed.Scheme + "://" + host, nil
}

func canonicalHost(raw string) (string, bool) {
	host := strings.ToLower(raw)
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String(), address.Zone() == "" && !address.Is4In6()
	}
	for _, character := range host {
		if character > 127 {
			return "", false
		}
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	numeric := last != ""
	for _, character := range last {
		if character < '0' || character > '9' {
			numeric = false
		}
	}
	return host, last != "" && !numeric && !strings.HasPrefix(last, "0x")
}

func mailSettings(lookup Lookup, backend MailBackend) (SMTPSettings, string, []error) {
	if backend != MailBackendSMTP {
		return SMTPSettings{}, "", nil
	}
	var problems []error
	smtp := SMTPSettings{}
	host, ok := lookup(varSMTPHost)
	if !ok || strings.TrimSpace(host) == "" || strings.ContainsAny(host, " /:@") {
		problems = append(problems, &SettingsError{Variable: varSMTPHost, Problem: "is required with OVC_MAIL_BACKEND=smtp and must be a host name"})
	}
	smtp.Host = host
	port, err := portOr(lookup, varSMTPPort, 587)
	if err != nil {
		problems = append(problems, err)
	}
	smtp.Port = port
	tlsMode, err := oneOf(lookup, varSMTPTLS, SMTPStartTLS, SMTPStartTLS, SMTPImplicit)
	if err != nil {
		problems = append(problems, err)
	}
	smtp.TLS = tlsMode
	smtp.Username = NewSecret(stringOr(lookup, varSMTPUsername, ""))
	password, _ := lookup(varSMTPPassword)
	smtp.Password = NewSecret(password)
	if smtp.Username.IsZero() != smtp.Password.IsZero() {
		problems = append(problems, &SettingsError{Variable: varSMTPPassword, Problem: "must be set together with OVC_SMTP_USERNAME"})
	}
	from, ok := lookup(varMailFrom)
	address, parseErr := mail.ParseAddress(from)
	if !ok || parseErr != nil || strings.ContainsAny(from, "\r\n") {
		problems = append(problems, &SettingsError{Variable: varMailFrom, Problem: "is required with OVC_MAIL_BACKEND=smtp and must be one email address"})
		return smtp, "", problems
	}
	return smtp, address.Address, problems
}

func portOr(lookup Lookup, name string, fallback int) (int, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		return 0, &SettingsError{Variable: name, Problem: "must be a port from 1 to 65535"}
	}
	return parsed, nil
}

const minimumJWTSecretBytes = 32

func (s *Settings) loadSessionSettings(lookup Lookup) []error {
	var problems []error
	secret, _ := lookup(varJWTSecret)
	s.JWTSecret = NewSecret(secret)
	if len(secret) < minimumJWTSecretBytes {
		problems = append(problems, &SettingsError{Variable: varJWTSecret, Problem: "is required and must be at least 32 bytes"})
	}
	access, err := secondsOr(lookup, varAccessTTL, 900, 60, 3600)
	if err != nil {
		problems = append(problems, err)
	}
	s.AccessTokenTTL = access
	refresh, err := secondsOr(lookup, varRefreshTTL, 30*24*60*60, 24*60*60, 90*24*60*60)
	if err != nil {
		problems = append(problems, err)
	}
	s.RefreshTokenTTL = refresh
	proxies, err := trustedProxies(lookup)
	if err != nil {
		problems = append(problems, err)
	}
	s.TrustedProxies = proxies
	files, err := bytesOr(lookup, varFileQuota, defaultFileQuotaBytes)
	if err != nil {
		problems = append(problems, err)
	}
	recordings, err := bytesOr(lookup, varRecordingQuota, defaultRecordingQuotaBytes)
	if err != nil {
		problems = append(problems, err)
	}
	s.TutorQuotas = TutorQuotas{FileBytes: files, RecordingBytes: recordings}
	return problems
}

const (
	defaultFileQuotaBytes      = 5_000_000_000
	defaultRecordingQuotaBytes = 50_000_000_000
	maximumQuotaBytes          = 1 << 50
)

func bytesOr(lookup Lookup, name string, fallback int64) (int64, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || parsed > maximumQuotaBytes {
		return 0, &SettingsError{Variable: name, Problem: fmt.Sprintf("must be a number of bytes from 0 to %d", int64(maximumQuotaBytes))}
	}
	return parsed, nil
}

func secondsOr(lookup Lookup, name string, fallback int64, lowest int64, highest int64) (time.Duration, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return time.Duration(fallback) * time.Second, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < lowest || parsed > highest {
		return 0, &SettingsError{Variable: name, Problem: fmt.Sprintf("must be a number of seconds from %d to %d", lowest, highest)}
	}
	return time.Duration(parsed) * time.Second, nil
}

func trustedProxies(lookup Lookup) ([]netip.Prefix, error) {
	value, ok := lookup(varTrustedProxies)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var prefixes []netip.Prefix
	for _, entry := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(entry))
		if err != nil || !narrowEnoughToTrust(prefix) {
			return nil, &SettingsError{Variable: varTrustedProxies, Problem: "must be a comma-separated list of CIDR ranges, each IPv4 range at least /8 and each IPv6 range at least /32, with no IPv4-mapped IPv6 ranges"}
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func narrowEnoughToTrust(prefix netip.Prefix) bool {
	if prefix.Addr().Is4In6() {
		return false
	}
	if prefix.Addr().Is4() {
		return prefix.Bits() >= minimumTrustedIPv4Bits
	}
	return prefix.Bits() >= minimumTrustedIPv6Bits
}

func (s Settings) RequiresTestDatabase() bool {
	return s.TestControl || (s.Process == ProcessWorker && s.MailBackend == MailBackendSink)
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

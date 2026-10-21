package startup_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

var binaries string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "ovc-binaries-")
	if err != nil {
		panic(err)
	}
	binaries = directory
	for _, name := range []string{"api", "worker", "ovc-admin"} {
		build := exec.CommandContext(context.Background(), "go", "build", "-o", filepath.Join(binaries, name), "ovc/backend/cmd/"+name)
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			panic(err)
		}
	}
	settings, err := core.TestStorageFromEnvironment(teststorage.Bucket)
	if err != nil {
		panic(err)
	}
	if err := storage.NewS3(settings).EnsureBucket(context.Background()); err != nil {
		panic(core.ErrorKind(err))
	}
	code := m.Run()
	if err := os.RemoveAll(binaries); err != nil {
		code = 1
	}
	os.Exit(code)
}

type outcome struct {
	exitCode int
	stdout   string
	stderr   string
}

func storageDefaults() map[string]string {
	settings, err := core.TestStorageFromEnvironment(teststorage.Bucket)
	if err != nil {
		panic(err)
	}
	return map[string]string{
		"OVC_STORAGE_ENDPOINT": settings.Endpoint, "OVC_STORAGE_REGION": settings.Region, "OVC_STORAGE_BUCKET": settings.Bucket,
		"OVC_STORAGE_ACCESS_KEY": settings.AccessKey.Reveal(), "OVC_STORAGE_" + "SECRET_KEY": settings.SecretKey.Reveal(),
	}
}

func withPublicURL(env []string) []string {
	defaults := storageDefaults()
	defaults["OVC_PUBLIC_URL"] = "https://ovc.example"
	defaults["OVC_JWT_SECRET"] = "process-test-signing-material-0123456789"
	result := env
	for name, value := range defaults {
		set := false
		for _, variable := range env {
			if strings.HasPrefix(variable, name+"=") {
				set = true
			}
		}
		if !set {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func runBinary(t *testing.T, name string, env []string, args ...string) outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(binaries, name), args...)
	command.Env = withPublicURL(env)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run %s: %v", name, err)
		}
		exitCode = exitError.ExitCode()
	}
	return outcome{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func TestOpenAPICommandNeedsNoSettingsAndIgnoresFlags(t *testing.T) {
	bare := runBinary(t, "api", []string{}, "openapi")
	flagged := runBinary(t, "api", []string{"OVC_TEST_CONTROL=1", "OVC_API_DOCS_ENABLED=true", "OVC_CLOCK=test"}, "openapi")

	if bare.exitCode != 0 || flagged.exitCode != 0 {
		t.Fatalf("exit codes %d %d: %s %s", bare.exitCode, flagged.exitCode, bare.stderr, flagged.stderr)
	}
	if bare.stdout != flagged.stdout || strings.Contains(bare.stdout, "__test__") || !strings.Contains(bare.stdout, "/api/v1/health/live") {
		t.Fatalf("the document must be identical and free of test routes")
	}
}

func TestProcessesRefuseUnsafeTestControl(t *testing.T) {
	production := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_prod", true)
	cases := map[string]struct {
		env       []string
		message   string
		processes []string
	}{
		"test clock without the flag": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_CLOCK=test"},
			message: "OVC_CLOCK",
		},
		"non-loopback listen address": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1", "OVC_HTTP_ADDR=0.0.0.0:0"},
			message: "OVC_HTTP_ADDR",
		},
		"remote storage": {
			env:       []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1", "OVC_HTTP_ADDR=127.0.0.1:0", "OVC_STORAGE_ENDPOINT=https://storage.ovc.example"},
			message:   "OVC_STORAGE_ENDPOINT",
			processes: storageProcesses,
		},
		"remote database host": {
			env:     []string{"OVC_DATABASE_URL=postgres://ovc:sentinel-pw-remote@10.255.255.1:5432/ovc_test", "OVC_TEST_CONTROL=1"},
			message: "database host",
		},
		"database host overridden in the query": {
			env:     []string{"OVC_DATABASE_URL=postgres://ovc:sentinel-pw-query@127.0.0.1:5432/ovc_test?host=10.255.255.1", "OVC_TEST_CONTROL=1"},
			message: "database host",
		},
		"keyword form host list": {
			env:     []string{"OVC_DATABASE_URL=host=127.0.0.1,db.example.com user=ovc password=sentinel-pw-keyword dbname=ovc_test", "OVC_TEST_CONTROL=1"},
			message: "database host",
		},
		"driver environment variable": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "PGHOST=10.255.255.1"},
			message: "PGHOST",
		},
		"listen address without a host": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1", "OVC_HTTP_ADDR=:0"},
			message: "OVC_HTTP_ADDR",
		},
		"connection string the driver cannot parse": {
			env:     []string{"OVC_DATABASE_URL=not-a-dsn-sentinel", "OVC_TEST_CONTROL=1"},
			message: "OVC_DATABASE_URL: is not a valid PostgreSQL connection string",
		},
		"unencoded password separator": {
			env:     []string{"OVC_DATABASE_URL=postgres://ovc:4471/sentinel-tail@127.0.0.1:1/ovc_test"},
			message: "OVC_DATABASE_URL",
		},
		"non-test database name": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1"},
			message: "_test or _dev",
		},
	}
	for name, testCase := range cases {
		processes := testCase.processes
		if processes == nil {
			processes = everyProcess
		}
		for _, binary := range processes {
			t.Run(name+"/"+binary, func(t *testing.T) {
				result := runBinary(t, binary, testCase.env, argumentsFor[binary]...)

				if result.exitCode == 0 || !strings.Contains(result.stderr, testCase.message) {
					t.Fatalf("expected a refusal mentioning %q, got %d: %s", testCase.message, result.exitCode, result.stderr)
				}
				for _, secret := range secretsIn(testCase.env) {
					if strings.Contains(result.stdout+result.stderr, secret) {
						t.Fatal("part of the database URL leaked")
					}
				}
			})
		}
	}
}

var (
	everyProcess     = []string{"api", "worker", "ovc-admin"}
	storageProcesses = []string{"api", "worker"}
	argumentsFor     = map[string][]string{"ovc-admin": {"users", "list"}}
)

func secretsIn(env []string) []string {
	secrets := []string{"sentinel-tail", "sentinel-pw-keyword", "not-a-dsn-sentinel"}
	for _, variable := range env {
		value, found := strings.CutPrefix(variable, "OVC_DATABASE_URL=")
		if !found {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.User == nil {
			continue
		}
		if password, set := parsed.User.Password(); set && password != "" {
			secrets = append(secrets, password)
		}
	}
	return secrets
}

func TestProcessesRefusePendingMigrations(t *testing.T) {
	unmigrated := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_test", false)
	env := []string{"OVC_DATABASE_URL=" + unmigrated.URL.Reveal()}

	api := runBinary(t, "api", env)
	worker := runBinary(t, "worker", env)

	for name, result := range map[string]outcome{"api": api, "worker": worker} {
		if result.exitCode == 0 || !strings.Contains(result.stderr, "migrations are pending") || !strings.Contains(result.stdout, "migrations.pending") {
			t.Fatalf("%s: %d %s", name, result.exitCode, result.stderr)
		}
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func TestMigrateServeAndShutDown(t *testing.T) {
	database := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_test", false)
	address := freeAddress(t)
	env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1"}
	migrated := runBinary(t, "api", env, "migrate")
	if migrated.exitCode != 0 || !strings.Contains(migrated.stdout, "migrate.applied") {
		t.Fatalf("migrate: %d %s", migrated.exitCode, migrated.stderr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := exec.CommandContext(ctx, filepath.Join(binaries, "api"))
	server.Env = withPublicURL(env)
	var stdout bytes.Buffer
	server.Stdout = &stdout
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}

	status := waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	if status != http.StatusOK || signalErr != nil || waitErr != nil {
		t.Fatalf("ready %d, signal %v, exit %v", status, signalErr, waitErr)
	}
	logs := stdout.String()
	for _, event := range []string{"process.started", "process.stopped", `"reason":"shutdown"`} {
		if !strings.Contains(logs, event) {
			t.Fatalf("missing %s in %s", event, logs)
		}
	}
	if strings.Contains(logs, `"smtp_host"`) {
		t.Fatalf("the API logged SMTP settings it never loads: %s", logs)
	}
	if strings.Contains(logs, database.URL.Reveal()) {
		t.Fatal("the database URL leaked into the logs")
	}
}

func waitForStatus(t *testing.T, ctx context.Context, address string) int {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("the server did not become ready before the deadline")
		case <-ticker.C:
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				continue
			}
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			return response.StatusCode
		}
	}
}

func TestWorkerRunsAndShutsDown(t *testing.T) {
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	worker := exec.CommandContext(ctx, filepath.Join(binaries, "worker"))
	worker.Env = withPublicURL([]string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_LOG_LEVEL=debug"})
	output, err := worker.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	started := nextLine(t, ctx, lines, "process.started")
	ticked := waitForLine(t, ctx, lines, "worker.tick")
	signalErr := worker.Process.Signal(syscall.SIGTERM)
	stopped := waitForLine(t, ctx, lines, "process.stopped")
	drainLines(t, ctx, lines)
	waitErr := worker.Wait()

	if !strings.Contains(started, `"smtp_host"`) || !ticked || signalErr != nil || !stopped || waitErr != nil {
		t.Fatalf("started %s, ticked %v, signal %v, stopped %v, exit %v", started, ticked, signalErr, stopped, waitErr)
	}
}

func drainLines(t *testing.T, ctx context.Context, lines <-chan string) {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("the process output did not end before the deadline")
		case _, open := <-lines:
			if !open {
				return
			}
		}
	}
}

func startServer(t *testing.T, ctx context.Context, env []string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	server := exec.CommandContext(ctx, filepath.Join(binaries, "api"))
	server.Env = withPublicURL(env)
	var stdout bytes.Buffer
	server.Stdout = &stdout
	server.Stderr = &stdout
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	return server, &stdout
}

func postJSON(t *testing.T, ctx context.Context, address string, path string, body string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode
}

func TestTestControlRoutesExistOnlyWithTheFlag(t *testing.T) {
	database := testdb.New(t)
	cases := map[string]struct {
		flag        string
		clock       string
		mailStatus  int
		clockStatus int
		tickStatus  int
	}{
		"with the flag and the test clock":   {flag: "1", clock: "test", mailStatus: http.StatusOK, clockStatus: http.StatusOK, tickStatus: http.StatusOK},
		"with the flag and the system clock": {flag: "1", clock: "system", mailStatus: http.StatusOK, clockStatus: http.StatusNotFound, tickStatus: http.StatusNotFound},
		"without the flag":                   {flag: "0", clock: "system", mailStatus: http.StatusNotFound, clockStatus: http.StatusNotFound, tickStatus: http.StatusNotFound},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			address := freeAddress(t)
			env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=" + testCase.flag, "OVC_CLOCK=" + testCase.clock}
			server, output := startServer(t, ctx, env)
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("server output:\n%s", output.String())
				}
			})
			waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")

			mailStatus := postJSON(t, ctx, address, "/__test__/mail", `{"to":"tutor@example.org"}`)
			clockStatus := postJSON(t, ctx, address, "/__test__/clock", `{"set":"2030-01-01T00:00:00Z"}`)
			tickStatus := postJSON(t, ctx, address, "/__test__/tick", `{}`)
			signalErr := server.Process.Signal(syscall.SIGTERM)
			waitErr := server.Wait()

			if mailStatus != testCase.mailStatus || clockStatus != testCase.clockStatus || tickStatus != testCase.tickStatus || signalErr != nil || waitErr != nil {
				t.Fatalf("mail %d (want %d), clock %d (want %d), tick %d (want %d), signal %v, exit %v",
					mailStatus, testCase.mailStatus, clockStatus, testCase.clockStatus, tickStatus, testCase.tickStatus, signalErr, waitErr)
			}
		})
	}
}

func waitForLine(t *testing.T, ctx context.Context, lines <-chan string, event string) bool {
	t.Helper()
	return nextLine(t, ctx, lines, event) != ""
}

func nextLine(t *testing.T, ctx context.Context, lines <-chan string, event string) string {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("no %s before the deadline", event)
		case line, open := <-lines:
			if !open {
				return ""
			}
			if strings.Contains(line, `"msg":"`+event+`"`) {
				return line
			}
		}
	}
}

func TestSinkRefusedForProductionDatabaseName(t *testing.T) {
	production := testdb.NewNamed(t, "ovc_"+core.NewRandomID()[:8]+"_prod", true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	address := freeAddress(t)
	env := []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_HTTP_ADDR=" + address}

	worker := runBinary(t, "worker", env)
	server, output := startServer(t, ctx, env)
	waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	if worker.exitCode == 0 || !strings.Contains(worker.stderr, "_test or _dev") {
		t.Fatalf("the worker with the default sink accepted %s: %d %s", production.Name, worker.exitCode, worker.stderr)
	}
	if signalErr != nil || waitErr != nil {
		t.Fatalf("the api, which writes no mail, refused the database: signal %v, exit %v\n%s", signalErr, waitErr, output.String())
	}
	for _, secret := range secretsIn(env) {
		if strings.Contains(worker.stdout+worker.stderr+output.String(), secret) {
			t.Fatal("part of the database URL leaked")
		}
	}
}

type exchange struct {
	status int
	body   string
}

func call(t *testing.T, ctx context.Context, method string, url string, body string, headers map[string]string) exchange {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return exchange{status: response.StatusCode, body: string(read)}
}

func seedTutor(t *testing.T, database *testdb.Database, password string) {
	t.Helper()
	hasher, err := core.NewPasswordHasher(core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, core.CryptoRandom(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := hasher.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	err = repositories.NewStore(database.Pool).Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: domain.UserID{1}, Role: domain.RoleTutor, State: domain.AccountActive, Email: "tutor@example.org", EmailVerifiedAt: &now,
		PasswordHash: encoded, Name: "A language tutor", Language: domain.LanguageEnglish, Timezone: "Europe/Moscow",
		Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = repositories.NewStore(database.Pool).Repositories().TutorProfiles.Create(context.Background(), repositories.NewTutorProfile{
		UserID: domain.UserID{1}, FileQuotaBytes: 1, RecordingQuotaBytes: 1, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransportDeadlinesIgnoreTheSharedClock(t *testing.T) {
	database := testdb.New(t)
	seedTutor(t, database, "correct horse battery")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	address := freeAddress(t)
	base := "http://" + address
	env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test"}
	server, output := startServer(t, ctx, env)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server output:\n%s", output.String())
		}
	})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	signIn := func() string {
		result := call(t, ctx, http.MethodPost, base+"/api/v1/auth/login", `{"email":"tutor@example.org","password":"correct horse battery"}`,
			map[string]string{"Content-Type": "application/json", "Origin": "https://ovc.example"})
		var decoded struct {
			AccessToken string `json:"access_token"`
		}
		if result.status != http.StatusOK || decodeJSON(result.body, &decoded) != nil {
			t.Fatalf("sign-in answered %d %s", result.status, result.body)
		}
		return decoded.AccessToken
	}
	me := func(token string) exchange {
		return call(t, ctx, http.MethodGet, base+"/api/v1/users/me", "", map[string]string{"Authorization": "Bearer " + token})
	}

	var statuses []int
	for _, instant := range []string{"2000-01-01T00:00:00Z", "2100-01-01T00:00:00Z"} {
		statuses = append(statuses, call(t, ctx, http.MethodPost, base+"/__test__/clock", `{"set":"`+instant+`"}`, jsonHeaders).status)
		statuses = append(statuses, me(signIn()).status)
	}
	token := signIn()
	before := me(token)
	advanced := call(t, ctx, http.MethodPost, base+"/__test__/clock", `{"advance_seconds":960}`, jsonHeaders)
	after := me(token)
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	for _, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("with the shared clock in 2000 and 2100: %v", statuses)
		}
	}
	if before.status != http.StatusOK || advanced.status != http.StatusOK || after.status != http.StatusUnauthorized || !strings.Contains(after.body, "token_expired") {
		t.Fatalf("expiry by the shared clock: before %d, advance %d, after %d %s", before.status, advanced.status, after.status, after.body)
	}
	if signalErr != nil || waitErr != nil {
		t.Fatalf("signal %v, exit %v", signalErr, waitErr)
	}
}

func decodeJSON[Target any](text string, target *Target) error {
	return json.Unmarshal([]byte(text), target)
}

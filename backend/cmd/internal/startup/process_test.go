package startup_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
)

var binaries string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "ovc-binaries-")
	if err != nil {
		panic(err)
	}
	binaries = directory
	for _, name := range []string{"api", "worker"} {
		build := exec.CommandContext(context.Background(), "go", "build", "-o", filepath.Join(binaries, name), "ovc/backend/cmd/"+name)
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			panic(err)
		}
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

func runBinary(t *testing.T, name string, env []string, args ...string) outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(binaries, name), args...)
	command.Env = env
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
		env     []string
		message string
	}{
		"test clock without the flag": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_CLOCK=test"},
			message: "OVC_CLOCK",
		},
		"non-loopback listen address": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1", "OVC_HTTP_ADDR=0.0.0.0:0"},
			message: "OVC_HTTP_ADDR",
		},
		"remote database host": {
			env:     []string{"OVC_DATABASE_URL=postgres://ovc:pw@10.255.255.1:5432/ovc_test", "OVC_TEST_CONTROL=1"},
			message: "database host",
		},
		"non-test database name": {
			env:     []string{"OVC_DATABASE_URL=" + production.URL.Reveal(), "OVC_TEST_CONTROL=1"},
			message: "_test or _dev",
		},
	}
	for name, testCase := range cases {
		for _, binary := range []string{"api", "worker"} {
			t.Run(name+"/"+binary, func(t *testing.T) {
				result := runBinary(t, binary, testCase.env)

				if result.exitCode == 0 || !strings.Contains(result.stderr, testCase.message) {
					t.Fatalf("expected a refusal mentioning %q, got %d: %s", testCase.message, result.exitCode, result.stderr)
				}
				if strings.Contains(result.stdout+result.stderr, "pw@") {
					t.Fatal("the database URL leaked")
				}
			})
		}
	}
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
	server.Env = env
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
	worker.Env = []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_LOG_LEVEL=debug"}
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

	ticked := waitForLine(t, ctx, lines, "worker.tick")
	signalErr := worker.Process.Signal(syscall.SIGTERM)
	stopped := waitForLine(t, ctx, lines, "process.stopped")
	waitErr := worker.Wait()

	if !ticked || signalErr != nil || !stopped || waitErr != nil {
		t.Fatalf("ticked %v, signal %v, stopped %v, exit %v", ticked, signalErr, stopped, waitErr)
	}
}

func waitForLine(t *testing.T, ctx context.Context, lines <-chan string, event string) bool {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("no %s before the deadline", event)
		case line, open := <-lines:
			if !open {
				return false
			}
			if strings.Contains(line, `"msg":"`+event+`"`) {
				return true
			}
		}
	}
}

package conventions_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const lintBinary = "../../../bin/golangci-lint"

var lintFixtureFiles = map[string]string{
	"internal/fixture/fixture.go": `package fixture

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"syscall"
	"time"
	"unsafe"
)

func Discard() {
	_ = os.Remove("x")
}

func Assert(value any) string {
	return value.(string)
}

func Read() string {
	return os.Getenv("A")
}

func Wait() {
	time.Sleep(time.Second)
}

func Now() time.Time {
	return time.Now()
}

func Say() {
	fmt.Println("x")
}

func Kind(value any) reflect.Kind {
	return reflect.TypeOf(value).Kind()
}

func Store() error {
	return os.Setenv("A", "b")
}

func Lookup() (string, bool) {
	return syscall.Getenv("A")
}

func Report() {
	log.Println("x")
}

func Size() uintptr {
	return unsafe.Sizeof(0)
}
`,
	"internal/fixture/fixture_test.go": `package fixture

import "testing"

func TestSettings(t *testing.T) {
	t.Setenv("A", "b")
}

func BenchmarkSettings(b *testing.B) {
	b.Setenv("A", "b")
}

func settingsHelper(tb testing.TB) {
	tb.Setenv("A", "b")
}
`,
}

var expectedLintFindings = []string{
	"Error return value of `os.Remove` is not checked",
	"Error return value is not checked",
	"use of `os.Getenv` forbidden",
	"use of `time.Sleep` forbidden",
	"use of `time.Now` forbidden",
	"use of `fmt.Println` forbidden",
	"use of `t.Setenv` forbidden",
	"use of `b.Setenv` forbidden",
	"use of `tb.Setenv` forbidden",
	"use of `os.Setenv` forbidden",
	"use of `syscall.Getenv` forbidden",
	"use of `log.Println` forbidden",
	"reflect is forbidden in hand-written code",
	"unsafe is forbidden in hand-written code",
}

func fixtureModule(t *testing.T) string {
	t.Helper()
	text, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(text), "\n") {
		if strings.HasPrefix(line, "go ") {
			return "module ovc/backend\n\n" + line + "\n"
		}
	}
	t.Fatal("backend/go.mod has no go directive")
	return ""
}

func writeLintFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"go.mod": fixtureModule(t)}
	for name, content := range lintFixtureFiles {
		files[name] = content
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runProjectLint(t *testing.T, directory string) string {
	t.Helper()
	binary, err := filepath.Abs(lintBinary)
	if err != nil {
		t.Fatal(err)
	}
	config, err := filepath.Abs("../../.golangci.yml")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), binary, "run", "--config", config, "--max-issues-per-linter", "0", "--max-same-issues", "0", "./...")
	command.Dir = directory
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("golangci-lint should report findings with exit code 1, got %v:\n%s", err, output)
	}
	return string(output)
}

func TestProjectLintRulesFire(t *testing.T) {
	fixture := writeLintFixture(t)

	output := runProjectLint(t, fixture)

	for _, expected := range expectedLintFindings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in the lint output:\n%s", expected, output)
		}
	}
}

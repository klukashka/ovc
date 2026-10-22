package startup_test

import (
	"encoding/base64"
	"strings"
	"testing"
)

const unreachableDatabase = "OVC_DATABASE_URL=postgres://ovc@127.0.0.1:1/ovc_test"

var shortFieldKey = base64.StdEncoding.EncodeToString([]byte("sentinel-short-field-key"))

func fieldKeyRefusals() map[string]string {
	return map[string]string{
		"missing key": unsetVariable("OVC_FIELD_ENCRYPTION_KEY"),
		"empty key":   "OVC_FIELD_ENCRYPTION_KEY=",
		"short key":   "OVC_FIELD_ENCRYPTION_KEY=" + shortFieldKey,
	}
}

func revealsAValue(result outcome) bool {
	output := result.stdout + result.stderr
	return strings.Contains(output, "sentinel") || strings.Contains(output, shortFieldKey) || strings.Contains(output, fieldKeyForTheRun)
}

func TestCheckSettingsPrintsOnlyOkOrTheSettingsError(t *testing.T) {
	for _, binary := range storageProcesses {
		t.Run(binary, func(t *testing.T) {
			accepted := runBinary(t, binary, []string{unreachableDatabase}, "check-settings")
			refused := map[string]outcome{}
			for name, variable := range fieldKeyRefusals() {
				refused[name] = runBinary(t, binary, []string{unreachableDatabase, variable}, "check-settings")
			}

			if accepted.exitCode != 0 || accepted.stdout != "ok\n" || accepted.stderr != "" {
				t.Fatalf("valid settings: %d %q %q", accepted.exitCode, accepted.stdout, accepted.stderr)
			}
			for name, result := range refused {
				if result.exitCode != 1 || result.stdout != "" || !strings.HasPrefix(result.stderr, "configuration: OVC_FIELD_ENCRYPTION_KEY: ") {
					t.Fatalf("%s: %d %q %q", name, result.exitCode, result.stdout, result.stderr)
				}
				if revealsAValue(result) || strings.Count(result.stderr, "\n") != 1 {
					t.Fatalf("%s: the refusal shows more than the variable and the problem: %q", name, result.stderr)
				}
			}
		})
	}
}

func TestCommandsRefuseUnknownArguments(t *testing.T) {
	cases := map[string]struct {
		binary string
		args   []string
		usage  string
	}{
		"worker with an unknown command":    {binary: "worker", args: []string{"serve"}, usage: "usage: worker"},
		"worker check-settings with extras": {binary: "worker", args: []string{"check-settings", "now"}, usage: "usage: worker"},
		"api check-settings with extras":    {binary: "api", args: []string{"check-settings", "now"}, usage: "usage: api"},
		"api with an unknown command":       {binary: "api", args: []string{"serve"}, usage: "usage: api"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			result := runBinary(t, testCase.binary, []string{unreachableDatabase}, testCase.args...)

			if result.exitCode != 2 || result.stdout != "" || !strings.HasPrefix(result.stderr, testCase.usage) {
				t.Fatalf("expected the usage and exit 2, got %d %q %q", result.exitCode, result.stdout, result.stderr)
			}
		})
	}
}

func TestProcessesRefuseToStartWithoutAValidFieldEncryptionKey(t *testing.T) {
	commands := map[string][]string{"api": nil, "api migrate": {"migrate"}, "worker": nil}
	for name, variable := range fieldKeyRefusals() {
		for command, args := range commands {
			t.Run(name+"/"+command, func(t *testing.T) {
				binary, _, _ := strings.Cut(command, " ")

				result := runBinary(t, binary, []string{unreachableDatabase, variable}, args...)

				if result.exitCode != 1 || !strings.Contains(result.stderr, "OVC_FIELD_ENCRYPTION_KEY") || strings.Contains(result.stdout, "process.started") {
					t.Fatalf("expected a start-up refusal naming the key, got %d %q %q", result.exitCode, result.stdout, result.stderr)
				}
				if revealsAValue(result) {
					t.Fatal("the refusal shows a value")
				}
			})
		}
	}
}

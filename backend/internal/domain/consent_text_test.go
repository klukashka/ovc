package domain_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

const russianConsentText = "../../../frontend/src/legal/ru/consent.md"

func consentTextVersion(t *testing.T) string {
	t.Helper()
	file, err := os.Open(russianConsentText)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	})
	scanner := bufio.NewScanner(file)
	inFrontMatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if inFrontMatter {
				break
			}
			inFrontMatter = true
			continue
		}
		if version, found := strings.CutPrefix(line, "version:"); inFrontMatter && found {
			return strings.TrimSpace(version)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("the consent text names no version in its front matter")
	return ""
}

func TestTheRecordedConsentVersionIsTheConsentTextsVersion(t *testing.T) {
	t.Parallel()

	version := consentTextVersion(t)

	if string(domain.CurrentConsentVersion) != version {
		t.Fatalf("CurrentConsentVersion is %q but the consent text's version is %q", domain.CurrentConsentVersion, version)
	}
}

package accounts

import (
	"bytes"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func TestRefreshCookieRoundTripsAndRefusesEveryMalformedForm(t *testing.T) {
	original := refreshCookie{session: domain.SessionID{7}, generation: 42, random: bytes.Repeat([]byte{1}, 32), mac: bytes.Repeat([]byte{2}, 32)}
	encoded := original.encode()
	parts := strings.Split(encoded, ".")
	malformed := []string{
		"",
		parts[0] + "." + parts[1] + "." + parts[2],
		encoded + ".extra",
		strings.Join([]string{parts[0], "0", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "042", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "-1", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "+1", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "1234567890123456789", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0] + "A", parts[1], parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], parts[1], parts[2][:40], parts[3]}, "."),
		strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + "="}, "."),
		strings.Join([]string{parts[0], parts[1], parts[2], strings.ReplaceAll(parts[3], "A", "+")}, "."),
	}

	parsed, ok := parseRefreshCookie(encoded)

	if !ok || parsed.session != original.session || parsed.generation != 42 || !bytes.Equal(parsed.random, original.random) || !bytes.Equal(parsed.mac, original.mac) {
		t.Fatalf("round trip %+v, %v", parsed, ok)
	}
	for _, value := range malformed {
		if _, accepted := parseRefreshCookie(value); accepted {
			t.Errorf("accepted %q", value)
		}
	}
}

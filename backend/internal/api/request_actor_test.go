package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type accessRecord struct {
	Message string `json:"msg"`
	Route   string `json:"route"`
	UserID  string `json:"user_id"`
	Role    string `json:"role"`
}

func accessRecordsFor(t *testing.T, logs string, route string) []accessRecord {
	t.Helper()
	var found []accessRecord
	for line := range strings.Lines(logs) {
		if strings.Count(line, `"user_id"`) > 1 || strings.Count(line, `"role"`) > 1 {
			t.Fatalf("a record repeats the actor fields: %s", line)
		}
		var record accessRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("not a JSON record: %s", line)
		}
		if record.Message == "http.request.completed" && record.Route == route {
			found = append(found, record)
		}
	}
	return found
}

func TestAuthenticatedRequestRecordsCarryTheUserAndRole(t *testing.T) {
	h := newAuthHarness(t)
	tutor := h.seedUserAs(t, domain.RoleTutor, 1, tutorEmail, "correct horse battery", cheapArgon2())
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	h.me(t, token)

	logs := h.logs.String()
	signedIn := accessRecordsFor(t, logs, "GET /api/v1/users/me")
	anonymous := accessRecordsFor(t, logs, "POST /api/v1/auth/login")
	if len(signedIn) != 1 || signedIn[0].UserID != tutor.String() || signedIn[0].Role != string(domain.RoleTutor) {
		t.Fatalf("the authenticated access record lacks the actor: %+v", signedIn)
	}
	if len(anonymous) != 1 || anonymous[0].UserID != "" || anonymous[0].Role != "" {
		t.Fatalf("the anonymous access record names an actor: %+v", anonymous)
	}
}

func TestRefusedTokensLeaveNoActorOnTheRecord(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUserAs(t, domain.RoleTutor, 1, tutorEmail, "correct horse battery", cheapArgon2())
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	h.advance(t, 16*time.Minute)

	h.me(t, token)

	refused := accessRecordsFor(t, h.logs.String(), "GET /api/v1/users/me")
	if len(refused) != 1 || refused[0].UserID != "" || refused[0].Role != "" {
		t.Fatalf("a refused request names an actor: %+v", refused)
	}
}

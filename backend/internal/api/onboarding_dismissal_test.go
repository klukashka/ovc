package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	dismissalBody             = `{"onboarding_dismissed":true}`
	dismissedRecord           = `"msg":"onboarding.dismissed"`
	settingsThrottledRecord   = `"msg":"tutor.settings.rate_limited"`
	settingsWritesPurpose     = "tutor_settings_writes_hour"
	settingsWritesPerHour     = 60
	unverifiedTutorEmail      = "unconfirmed-tutor@example.org"
	dismissalTimeoutCondition = time.Minute
)

type dismissalSettings struct {
	FileQuotaBytes      *int64 `json:"file_quota_bytes"`
	FileUsageBytes      *int64 `json:"file_usage_bytes"`
	OnboardingDismissed *bool  `json:"onboarding_dismissed"`
}

func (h *authHarness) dismissOnboarding(t *testing.T, token string, body string, headers map[string]string) response {
	t.Helper()
	if headers == nil {
		headers = jsonCall(token)
	}
	return h.send(t, http.MethodPatch, tutorSettingsRoute, body, headers)
}

func (h *authHarness) dismissed(t *testing.T, token string) {
	t.Helper()
	if result := h.dismissOnboarding(t, token, dismissalBody, nil); result.status != http.StatusOK {
		t.Fatalf("the dismissal answered %s", describe(result))
	}
}

func dismissalSettingsOf(t *testing.T, result response) dismissalSettings {
	t.Helper()
	var settings dismissalSettings
	if err := json.Unmarshal([]byte(result.body), &settings); err != nil {
		t.Fatalf("not tutor settings: %s", result.body)
	}
	return settings
}

func (h *authHarness) settingsDismissed(t *testing.T, token string) bool {
	t.Helper()
	result := h.send(t, http.MethodGet, tutorSettingsRoute, "", jsonCall(token))
	settings := dismissalSettingsOf(t, result)
	if result.status != http.StatusOK || settings.OnboardingDismissed == nil {
		t.Fatalf("the settings answered %s", describe(result))
	}
	return *settings.OnboardingDismissed
}

func (h *authHarness) storedDismissal(t *testing.T, tutor domain.UserID) *time.Time {
	t.Helper()
	var dismissedAt *time.Time
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT onboarding_dismissed_at FROM tutor_profiles WHERE user_id = $1::uuid", tutor.String()).
		Scan(&dismissedAt); err != nil {
		t.Fatal(err)
	}
	return dismissedAt
}

func (h *authHarness) settingsWriteSlots(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", settingsWritesPurpose)
}

func (h *authHarness) fillSettingsWrites(t *testing.T, tutor domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), time.Hour)
	subject := h.keys.ThrottleSubject(settingsWritesPurpose, tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)
ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = excluded.count`, settingsWritesPurpose, subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) profileRows(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(*) FROM tutor_profiles WHERE user_id = $1::uuid", tutor.String())
}

func reportsDismissal(result response, settings dismissalSettings) bool {
	return result.status == http.StatusOK && result.headers.Get("Cache-Control") == "no-store" &&
		settings.OnboardingDismissed != nil && *settings.OnboardingDismissed &&
		settings.FileQuotaBytes != nil && *settings.FileQuotaBytes == 5_000_000_000 && settings.FileUsageBytes != nil && *settings.FileUsageBytes == 0
}

func TestOnboardingDismissalFirstWrite(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	if h.settingsDismissed(t, token) {
		t.Fatal("a new tutor's checklist is already dismissed")
	}

	result := h.dismissOnboarding(t, token, dismissalBody, nil)

	if !reportsDismissal(result, dismissalSettingsOf(t, result)) {
		t.Fatalf("the dismissal answered %s", describe(result))
	}
	if stored := h.storedDismissal(t, tutor); stored == nil || !stored.Equal(h.now(t)) {
		t.Fatalf("the stored dismissal is %v", stored)
	}
	logs := h.logs.String()
	if strings.Count(logs, dismissedRecord) != 1 || !strings.Contains(logs, `"tutor_id":"`+tutor.String()+`"`) || strings.Contains(logs, tutorEmail) {
		t.Fatalf("the dismissal records are wrong: %s", logs)
	}
}

func TestOnboardingDismissalRepeatKeepsTheFirstInstant(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.dismissed(t, token)
	first := h.now(t)
	h.advance(t, time.Minute)

	result := h.dismissOnboarding(t, token, dismissalBody, nil)

	if !reportsDismissal(result, dismissalSettingsOf(t, result)) {
		t.Fatalf("the repeated dismissal answered %s", describe(result))
	}
	if stored := h.storedDismissal(t, tutor); stored == nil || !stored.Equal(first) {
		t.Fatalf("the repeat changed the stored dismissal to %v", stored)
	}
	if strings.Count(h.logs.String(), dismissedRecord) != 1 {
		t.Fatal("the repeat recorded another dismissal")
	}
}

func TestOnboardingDismissalAcceptsAnUnverifiedTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, unverifiedTutorEmail)

	result := h.dismissOnboarding(t, token, dismissalBody, nil)

	if !reportsDismissal(result, dismissalSettingsOf(t, result)) || !h.settingsDismissed(t, token) {
		t.Fatalf("the unconfirmed tutor's dismissal answered %s", describe(result))
	}
}

func TestOnboardingDismissalLeavesOtherTutorsUntouched(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)

	h.dismissed(t, token)

	if h.settingsDismissed(t, otherToken) || h.storedDismissal(t, other) != nil {
		t.Fatal("another tutor's checklist was dismissed")
	}
}

func TestOnboardingDismissalRefusesEveryOtherBody(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body        string
		contentType string
		status      int
		fields      []string
	}{
		"false":            {body: `{"onboarding_dismissed":false}`, status: http.StatusUnprocessableEntity, fields: []string{"body.onboarding_dismissed:invalid"}},
		"null":             {body: `{"onboarding_dismissed":null}`, status: http.StatusUnprocessableEntity, fields: []string{"body.onboarding_dismissed:invalid"}},
		"a string":         {body: `{"onboarding_dismissed":"yes"}`, status: http.StatusUnprocessableEntity, fields: []string{"body.onboarding_dismissed:invalid"}},
		"a number":         {body: `{"onboarding_dismissed":1}`, status: http.StatusUnprocessableEntity, fields: []string{"body.onboarding_dismissed:invalid"}},
		"no field":         {body: `{}`, status: http.StatusUnprocessableEntity, fields: []string{"body.onboarding_dismissed:required"}},
		"another field":    {body: `{"onboarding_dismissed":true,"recording_retention_days":30}`, status: http.StatusUnprocessableEntity, fields: []string{"body:unexpected_property"}},
		"plain text":       {body: dismissalBody, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		"malformed JSON":   {body: `{"onboarding_dismissed":`, status: http.StatusBadRequest},
		"an oversize body": {body: `{"onboarding_dismissed":true,"padding":"` + strings.Repeat("a", 2<<20) + `"}`, status: http.StatusRequestEntityTooLarge},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			headers := jsonCall(token)
			if testCase.contentType != "" {
				headers["Content-Type"] = testCase.contentType
			}

			result := h.dismissOnboarding(t, token, testCase.body, headers)

			if result.status != testCase.status {
				t.Fatalf("the body answered %s", describe(result))
			}
			if testCase.fields != nil && !slices.Equal(fields(t, result), testCase.fields) {
				t.Fatalf("the refusal names %v", fields(t, result))
			}
			if h.storedDismissal(t, tutor) != nil || h.settingsWriteSlots(t) != 0 {
				t.Fatal("a refused body dismissed the checklist or reserved a write slot")
			}
		})
	}
}

func TestOnboardingDismissalIsNotFoundForStudents(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		state domain.AccountState
		body  string
	}{
		"an active student with a valid body":        {state: domain.AccountActive, body: dismissalBody},
		"an active student with an invalid body":     {state: domain.AccountActive, body: `{"onboarding_dismissed":false,"other":1}`},
		"an unverified student with a valid body":    {state: domain.AccountUnverified, body: dismissalBody},
		"an unverified student with an invalid body": {state: domain.AccountUnverified, body: `{`},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			_, token := h.signedInAs(t, domain.RoleStudent, testCase.state, 2, studentEmail)
			unknown := h.unknownRouteRefusal(t)

			result := h.dismissOnboarding(t, token, testCase.body, nil)

			if result.status != http.StatusNotFound || refusal(t, result) != unknown {
				t.Fatalf("the student's dismissal answered %s", describe(result))
			}
		})
	}
}

func TestOnboardingDismissalRechecksTheSessionUnderTheProfileLock(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, h *authHarness, tutor domain.UserID, token string){
		"a session revoked meanwhile": func(t *testing.T, h *authHarness, _ domain.UserID, token string) {
			t.Helper()
			principal, err := h.accounts.Authenticate(context.Background(), token)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.accounts.Logout(context.Background(), principal); err != nil {
				t.Fatal(err)
			}
		},
		"a session that expired meanwhile": func(t *testing.T, h *authHarness, _ domain.UserID, _ string) {
			t.Helper()
			h.advance(t, 2*dismissalTimeoutCondition)
		},
		"an account blocked meanwhile": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) {
			t.Helper()
			if _, err := h.database.Pool.Exec(t.Context(), "UPDATE users SET state = 'blocked', state_before_block = 'active' WHERE id = $1::uuid",
				tutor.String()); err != nil {
				t.Fatal(err)
			}
		},
		"an account deleted meanwhile": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) {
			t.Helper()
			if _, err := h.database.Pool.Exec(t.Context(), "UPDATE users SET state = 'deleted', deleted_at = $2 WHERE id = $1::uuid",
				tutor.String(), h.now(t)); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			actor := h.authenticatedTutor(t, token)
			if _, err := h.database.Pool.Exec(t.Context(), "UPDATE sessions SET expires_at = $2 WHERE id = $1", actor.SessionID,
				h.now(t).Add(dismissalTimeoutCondition)); err != nil {
				t.Fatal(err)
			}
			release := h.holdTutorProfile(t, tutor)
			pending := h.sendInBackground(http.MethodPatch, tutorSettingsRoute, dismissalBody, jsonCall(token))
			h.waitForLockWaiters(t, 1)

			change(t, h, tutor, token)
			release()
			result := awaitAnswer(t, pending)

			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
				t.Fatalf("the dismissal answered %s", describe(result))
			}
			if h.storedDismissal(t, tutor) != nil || strings.Contains(h.logs.String(), dismissedRecord) || h.settingsWriteSlots(t) != 0 {
				t.Fatal("a refused actor dismissed the checklist or reserved a write slot")
			}
		})
	}
}

func TestOnboardingDismissalRefusesARevokedSessionBeforeTheLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillSettingsWrites(t, tutor, settingsWritesPerHour)
	release := h.holdTutorProfile(t, tutor)
	pending := h.sendInBackground(http.MethodPatch, tutorSettingsRoute, dismissalBody, jsonCall(token))
	h.waitForLockWaiters(t, 1)
	principal, err := h.accounts.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.accounts.Logout(context.Background(), principal); err != nil {
		t.Fatal(err)
	}

	release()
	result := awaitAnswer(t, pending)

	if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
		t.Fatalf("the revoked write over the limit answered %s", describe(result))
	}
	if strings.Contains(h.logs.String(), settingsThrottledRecord) || h.settingsWriteSlots(t) != settingsWritesPerHour {
		t.Fatal("the revoked write reached the limit")
	}
}

func TestOnboardingDismissalRefusesTheSixtyFirstWriteInAnHour(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillSettingsWrites(t, tutor, settingsWritesPerHour)

	result := h.dismissOnboarding(t, token, dismissalBody, nil)

	retryAfter, err := strconv.Atoi(result.headers.Get("Retry-After"))
	if result.status != http.StatusTooManyRequests || code(t, result) != "rate_limited" || err != nil || retryAfter <= 0 {
		t.Fatalf("the sixty-first write answered %s", describe(result))
	}
	logs := h.logs.String()
	if strings.Count(logs, settingsThrottledRecord) != 1 || !strings.Contains(logs, `"limit":"`+settingsWritesPurpose+`"`) {
		t.Fatalf("the throttled write was not recorded once: %s", logs)
	}
	if h.storedDismissal(t, tutor) != nil {
		t.Fatal("a throttled write dismissed the checklist")
	}
}

func TestOnboardingDismissalLimitIsPerTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.fillSettingsWrites(t, tutor, settingsWritesPerHour)

	result := h.dismissOnboarding(t, otherToken, dismissalBody, nil)

	if result.status != http.StatusOK {
		t.Fatalf("another tutor's write answered %s", describe(result))
	}
}

func TestOnboardingDismissalLimitResetsInTheNextHour(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillSettingsWrites(t, tutor, settingsWritesPerHour)
	h.advance(t, time.Hour)
	freshToken := accessToken(t, h.login(t, tutorEmail, accountPassword))

	result := h.dismissOnboarding(t, freshToken, dismissalBody, nil)

	if result.status != http.StatusOK {
		t.Fatalf("the write in the next hour answered %s", describe(result))
	}
}

func TestTutorPurgesRemoveTheOnboardingDismissal(t *testing.T) {
	t.Parallel()
	t.Run("a confirmed tutor after the retention", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		h.dismissed(t, token)
		if deleted := h.deleteMe(t, token, accountPassword); deleted.status != http.StatusNoContent {
			t.Fatalf("deletion answered %s", describe(deleted))
		}
		h.at(t, authStart.Add(domain.DeletedAccountRetention-time.Hour))
		h.tick(t)
		if h.storedDismissal(t, tutor) == nil {
			t.Fatal("the dismissal went before the retention ended")
		}
		h.at(t, authStart.Add(domain.DeletedAccountRetention))

		h.tick(t)

		if rows := h.profileRows(t, tutor); rows != 0 {
			t.Fatalf("%d profile rows of the purged tutor remain", rows)
		}
	})
	t.Run("an unconfirmed tutor at once", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, unverifiedTutorEmail)
		h.dismissed(t, token)
		if h.storedDismissal(t, tutor) == nil {
			t.Fatal("the seed holds no dismissal")
		}

		deleted := h.deleteMe(t, token, accountPassword)

		if deleted.status != http.StatusNoContent {
			t.Fatalf("deletion answered %s", describe(deleted))
		}
		if rows := h.profileRows(t, tutor); rows != 0 {
			t.Fatalf("%d profile rows of the purged tutor remain", rows)
		}
	})
}

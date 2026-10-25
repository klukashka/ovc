package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	preferencesPath = "/api/v1/users/me/email-preferences"
	unsubscribePath = "/api/v1/unsubscribe"
	previewPath     = "/api/v1/unsubscribe/preview"
	formType        = "application/x-www-form-urlencoded"
	oneClickForm    = "List-Unsubscribe=One-Click"
)

func (h *authHarness) preferences(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodGet, preferencesPath, "", map[string]string{"Authorization": "Bearer " + token})
}

func (h *authHarness) savePreferences(t *testing.T, token string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPut, preferencesPath, body, jsonCall(token))
}

func listed(t *testing.T, result response) []string {
	t.Helper()
	var decoded struct {
		Groups []struct {
			Group   string `json:"group"`
			Enabled bool   `json:"enabled"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK {
		t.Fatalf("not a preference list: %d %s", result.status, result.body)
	}
	entries := make([]string, 0, len(decoded.Groups))
	for _, group := range decoded.Groups {
		state := "on"
		if !group.Enabled {
			state = "off"
		}
		entries = append(entries, group.Group+":"+state)
	}
	return entries
}

func (h *authHarness) mintLink(t *testing.T, user domain.UserID, group domain.EmailGroup, sentAt time.Time) string {
	t.Helper()
	token, err := h.links.Mint(user, group, sentAt)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (h *authHarness) unsubscribe(t *testing.T, token string, contentType string, body string) response {
	t.Helper()
	headers := map[string]string{}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return h.send(t, http.MethodPost, unsubscribePath+"?token="+url.QueryEscape(token), body, headers)
}

func (h *authHarness) preview(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodGet, previewPath+"?token="+url.QueryEscape(token), "", nil)
}

func (h *authHarness) storedPreferences(t *testing.T, user domain.UserID) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT email_group, enabled FROM email_preferences WHERE user_id = $1::uuid ORDER BY email_group", user.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var stored []string
	for rows.Next() {
		var group string
		var enabled bool
		if err := rows.Scan(&group, &enabled); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, fmt.Sprintf("%s:%t", group, enabled))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return stored
}

func (h *authHarness) unsubscribeCounted(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", string(domain.PurposeUnsubscribe))
}

func multipartForm(t *testing.T, fields map[string]string) (string, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return form.FormDataContentType(), body.String()
}

type refusalRecord struct {
	Message string `json:"msg"`
	Level   string `json:"level"`
	Reason  string `json:"reason"`
}

func (h *authHarness) refusals(t *testing.T) map[string][]string {
	t.Helper()
	levels := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(h.logs.String()), "\n") {
		var record refusalRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("not a JSON record: %s", line)
		}
		if record.Message == "unsubscribe.refused" && !slices.Contains(levels[record.Reason], record.Level) {
			levels[record.Reason] = append(levels[record.Reason], record.Level)
		}
	}
	return levels
}

type identicalRefusal struct {
	status int
	code   string
	text   string
	fields string
}

func refusalOf(t *testing.T, result response) identicalRefusal {
	t.Helper()
	var decoded struct {
		Error struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Fields  json.RawMessage `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("not a structured error: %d %s", result.status, result.body)
	}
	return identicalRefusal{status: result.status, code: decoded.Error.Code, text: decoded.Error.Message, fields: string(decoded.Error.Fields)}
}

func TestEmailPreferencesDefaultToOn(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)

	tutorList := h.preferences(t, tutor)
	studentList := h.preferences(t, student)

	if got := listed(t, tutorList); !slices.Equal(got, []string{"lesson_reminders:on", "submissions:on", "message_digest:on"}) {
		t.Fatalf("the tutor's list is %v", got)
	}
	if got := listed(t, studentList); !slices.Equal(got, []string{"lesson_reminders:on", "lesson_changes:on", "homework:on", "results:on", "message_digest:on"}) {
		t.Fatalf("the student's list is %v", got)
	}
	for _, result := range []response{tutorList, studentList} {
		if result.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("the list is stored: %v", result.headers)
		}
	}
}

func TestPreferencesListOnlyTheRolesGroups(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)

	first := h.savePreferences(t, studentToken, `{"groups":[{"group":"homework","enabled":false},{"group":"lesson_reminders","enabled":false}]}`)
	second := h.savePreferences(t, studentToken, `{"groups":[{"group":"homework","enabled":true}]}`)
	reread := h.preferences(t, studentToken)
	tutorSaved := h.savePreferences(t, tutor, `{"groups":[{"group":"submissions","enabled":false}]}`)

	if got := listed(t, first); !slices.Equal(got, []string{"lesson_reminders:off", "lesson_changes:on", "homework:off", "results:on", "message_digest:on"}) {
		t.Fatalf("the first save answered %v", got)
	}
	if got := listed(t, second); !slices.Equal(got, listed(t, reread)) || !slices.Equal(got, []string{"lesson_reminders:off", "lesson_changes:on", "homework:on", "results:on", "message_digest:on"}) {
		t.Fatalf("the partial save answered %v and reads back as %v", got, listed(t, reread))
	}
	if got := listed(t, tutorSaved); !slices.Equal(got, []string{"lesson_reminders:on", "submissions:off", "message_digest:on"}) {
		t.Fatalf("the tutor's save answered %v", got)
	}
	if stored := h.storedPreferences(t, student); !slices.Equal(stored, []string{"homework:true", "lesson_reminders:false"}) {
		t.Fatalf("the student's saves stored %v", stored)
	}
	if second.headers.Get("Cache-Control") != "no-store" || !strings.Contains(h.logs.String(), `"groups":"homework,lesson_reminders"`) {
		t.Fatalf("the save is stored or unrecorded: %v", second.headers)
	}
}

func TestAGroupOfTheOtherRoleIsNotAllowed(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)

	studentOther := h.savePreferences(t, studentToken, `{"groups":[{"group":"homework","enabled":false},{"group":"submissions","enabled":false}]}`)
	tutorOther := h.savePreferences(t, tutorToken, `{"groups":[{"group":"homework","enabled":false}]}`)
	twice := h.savePreferences(t, studentToken, `{"groups":[{"group":"homework","enabled":false},{"group":"homework","enabled":true}]}`)
	unknown := h.savePreferences(t, studentToken, `{"groups":[{"group":"news","enabled":false}]}`)

	expected := map[string]struct {
		result response
		field  string
	}{
		"a student naming a tutor group": {studentOther, "body.groups[1].group:not_allowed"},
		"a tutor naming a student group": {tutorOther, "body.groups[0].group:not_allowed"},
		"a group named twice":            {twice, "body.groups[1].group:invalid"},
	}
	for name, outcome := range expected {
		if outcome.result.status != http.StatusUnprocessableEntity || code(t, outcome.result) != "validation_failed" || !slices.Equal(fields(t, outcome.result), []string{outcome.field}) {
			t.Fatalf("%s answered %d %s", name, outcome.result.status, outcome.result.body)
		}
	}
	if unknown.status != http.StatusUnprocessableEntity || code(t, unknown) != "validation_failed" {
		t.Fatalf("an unknown group answered %d %s", unknown.status, unknown.body)
	}
	if len(h.storedPreferences(t, student)) != 0 || len(h.storedPreferences(t, tutor)) != 0 {
		t.Fatal("a refused save stored a preference")
	}
}

func TestOneClickUnsubscribeTurnsOffOneGroupIdempotently(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	multipartType, multipartBody := multipartForm(t, map[string]string{"List-Unsubscribe": "One-Click"})
	variants := []struct {
		name        string
		user        byte
		contentType string
		body        string
	}{
		{"no body", 10, "", ""},
		{"a form", 11, formType, oneClickForm},
		{"a form with a charset", 12, "application/x-www-form-urlencoded; charset=utf-8", oneClickForm},
		{"a form in another letter case", 13, "Application/X-WWW-Form-URLEncoded", oneClickForm},
		{"a multipart form", 14, multipartType, multipartBody},
		{"a multipart form with a quoted boundary", 15, strings.Replace(multipartType, "boundary=", `boundary="`, 1) + `"`, multipartBody},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, variant.user, fmt.Sprintf("student%d@example.org", variant.user))
			link := h.mintLink(t, user, domain.EmailGroupHomework, authStart)

			first := h.unsubscribe(t, link, variant.contentType, variant.body)
			repeated := h.unsubscribe(t, link, variant.contentType, variant.body)
			after := h.preferences(t, token)

			if first.status != http.StatusNoContent || repeated.status != http.StatusNoContent || first.body != "" {
				t.Fatalf("the unsubscribe answered %d %s, then %d %s", first.status, first.body, repeated.status, repeated.body)
			}
			if got := listed(t, after); !slices.Equal(got, []string{"lesson_reminders:on", "lesson_changes:on", "homework:off", "results:on", "message_digest:on"}) {
				t.Fatalf("after the unsubscribe the list is %v", got)
			}
			if stored := h.storedPreferences(t, user); !slices.Equal(stored, []string{"homework:false"}) {
				t.Fatalf("the unsubscribe stored %v", stored)
			}
		})
	}
	if applied := strings.Count(h.logs.String(), `"msg":"unsubscribe.applied"`); applied != 2*len(variants) {
		t.Fatalf("%d unsubscribes were recorded", applied)
	}
}

func TestUnsubscribeRefusesOtherContentTypes(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	user, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	link := h.mintLink(t, user, domain.EmailGroupResults, authStart)
	cases := map[string]struct {
		contentType string
		body        string
	}{
		"JSON":                          {"application/json", `{"List-Unsubscribe":"One-Click"}`},
		"an empty JSON body":            {"application/json", ""},
		"plain text":                    {"text/plain", oneClickForm},
		"a body without a content type": {"", oneClickForm},
		"a malformed media type":        {"application/x-www-form-urlencoded; =", oneClickForm},
	}

	results := map[string]response{}
	for name, testCase := range cases {
		results[name] = h.unsubscribe(t, link, testCase.contentType, testCase.body)
	}

	for name, result := range results {
		if result.status != http.StatusUnsupportedMediaType || code(t, result) != "unsupported_media_type" {
			t.Fatalf("%s answered %d %s", name, result.status, result.body)
		}
	}
	if stored := h.storedPreferences(t, user); len(stored) != 0 {
		t.Fatalf("a refused unsubscribe stored %v", stored)
	}
}

func TestUnsubscribeRefusesAFormWithoutTheOneClickField(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	user, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	link := h.mintLink(t, user, domain.EmailGroupResults, authStart)
	withoutType, withoutBody := multipartForm(t, map[string]string{"unsubscribe": "One-Click"})
	extraType, extraBody := multipartForm(t, map[string]string{"List-Unsubscribe": "One-Click", "email": "someone@example.org"})
	var fileBody bytes.Buffer
	fileForm := multipart.NewWriter(&fileBody)
	part, err := fileForm.CreateFormFile("List-Unsubscribe", "one-click.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("One-Click")); err != nil {
		t.Fatal(err)
	}
	if err := fileForm.Close(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		contentType string
		body        string
	}{
		"an empty form":                   {formType, ""},
		"another value":                   {formType, "List-Unsubscribe=Yes"},
		"another field":                   {formType, oneClickForm + "&next=%2Fhome"},
		"the field twice":                 {formType, oneClickForm + "&" + oneClickForm},
		"a malformed form":                {formType, "List-Unsubscribe=%zz"},
		"a multipart form without it":     {withoutType, withoutBody},
		"a multipart form with more":      {extraType, extraBody},
		"a multipart file":                {fileForm.FormDataContentType(), fileBody.String()},
		"a multipart type with no bounds": {"multipart/form-data", oneClickForm},
	}

	results := map[string]response{}
	for name, testCase := range cases {
		results[name] = h.unsubscribe(t, link, testCase.contentType, testCase.body)
	}
	oversized := h.unsubscribe(t, link, formType, oneClickForm+"&padding="+strings.Repeat("a", 1024))

	for name, result := range results {
		if result.status != http.StatusUnprocessableEntity || code(t, result) != "validation_failed" || !slices.Equal(fields(t, result), []string{"body.List-Unsubscribe:invalid"}) {
			t.Fatalf("%s answered %d %s", name, result.status, result.body)
		}
	}
	if oversized.status != http.StatusRequestEntityTooLarge || code(t, oversized) != "payload_too_large" {
		t.Fatalf("a body over 1 KiB answered %d %s", oversized.status, oversized.body)
	}
	if stored := h.storedPreferences(t, user); len(stored) != 0 {
		t.Fatalf("a refused unsubscribe stored %v", stored)
	}
}

func TestUnsubscribePreviewChangesNothing(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := h.mintLink(t, user, domain.EmailGroupSubmissions, authStart)

	first := h.preview(t, link)
	repeated := h.preview(t, link)

	for _, result := range []response{first, repeated} {
		if result.status != http.StatusOK || strings.TrimSpace(result.body) != `{"group":"submissions"}` {
			t.Fatalf("the preview answered %d %s", result.status, result.body)
		}
	}
	if stored := h.storedPreferences(t, user); len(stored) != 0 {
		t.Fatalf("the preview stored %v", stored)
	}
	if got := listed(t, h.preferences(t, token)); !slices.Equal(got, []string{"lesson_reminders:on", "submissions:on", "message_digest:on"}) {
		t.Fatalf("after the preview the list is %v", got)
	}
}

func TestUnsubscribeNeverSignsIn(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	user := h.seedUserAs(t, domain.RoleStudent, 2, studentEmail, accountPassword, currentHash)
	link := h.mintLink(t, user, domain.EmailGroupLessonChanges, authStart)

	previewed := h.preview(t, link)
	unsubscribed := h.unsubscribe(t, link, formType, oneClickForm)

	var previewFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(previewed.body), &previewFields); err != nil || previewed.status != http.StatusOK {
		t.Fatalf("the preview answered %d %s", previewed.status, previewed.body)
	}
	if len(previewFields) != 1 || previewFields["group"] == nil {
		t.Fatalf("the preview answered more than the group: %s", previewed.body)
	}
	if unsubscribed.status != http.StatusNoContent || unsubscribed.body != "" {
		t.Fatalf("the unsubscribe answered %d %s", unsubscribed.status, unsubscribed.body)
	}
	for _, result := range []response{previewed, unsubscribed} {
		if result.headers.Get("Set-Cookie") != "" || strings.Contains(result.body, "token") {
			t.Fatalf("an unsubscribe response carries a credential: %v %s", result.headers, result.body)
		}
	}
	if sessions := h.countRows(t, "SELECT count(id) FROM sessions WHERE user_id = $1::uuid", user.String()); sessions != 0 {
		t.Fatalf("the unsubscribe started %d sessions", sessions)
	}
}

func TestInvalidExpiredForgedOrOrphanedTokensAnswerIdentically(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	valid := h.mintLink(t, student, domain.EmailGroupHomework, authStart)
	otherSigner, err := core.NewUnsubscribeTokens(core.NewSecret("another-unsubscribe-material-0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	forged, err := otherSigner.Mint(student, domain.EmailGroupHomework, authStart)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(valid, ".")
	tokens := map[string]string{
		"a missing token":         "",
		"a malformed token":       "not-a-token",
		"a token of another key":  forged,
		"a tampered token":        parts[0] + "." + parts[1] + "." + strings.Repeat("A", len(parts[2])),
		"an expired token":        h.mintLink(t, student, domain.EmailGroupHomework, authStart.Add(-90*24*time.Hour)),
		"an orphaned token":       h.mintLink(t, domain.UserID{0x42}, domain.EmailGroupHomework, authStart),
		"another role's group":    h.mintLink(t, student, domain.EmailGroupSubmissions, authStart),
		"a token with extra text": valid + ".x",
	}

	refusals := map[string]identicalRefusal{}
	for name, token := range tokens {
		refusals[name+" previewed"] = refusalOf(t, h.preview(t, token))
		refusals[name+" posted"] = refusalOf(t, h.unsubscribe(t, token, formType, oneClickForm))
	}

	expected := identicalRefusal{status: http.StatusUnprocessableEntity, code: "token_invalid", text: "The link is invalid or has expired.", fields: ""}
	for name, refusal := range refusals {
		if refusal != expected {
			t.Fatalf("%s answered %+v", name, refusal)
		}
	}
	if stored := h.storedPreferences(t, student); len(stored) != 0 {
		t.Fatalf("a refused token stored %v", stored)
	}
	levels := h.refusals(t)
	wanted := map[string][]string{"malformed": {"DEBUG"}, "signature": {"DEBUG"}, "expired": {"INFO"}, "account_state": {"INFO"}}
	if !maps.EqualFunc(levels, wanted, slices.Equal) {
		t.Fatalf("the refusals were recorded as %v", levels)
	}
}

func TestTokensOfDeletedOrBlockedAccountsAreInvalid(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	deleted, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	blocked, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, tutorEmail)
	deletedLink := h.mintLink(t, deleted, domain.EmailGroupResults, authStart)
	blockedLink := h.mintLink(t, blocked, domain.EmailGroupSubmissions, authStart)
	if marked, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil || !marked {
		t.Fatalf("the student was not deleted: %v", err)
	}
	if marked, err := h.store.Repositories().Users.Block(context.Background(), blocked, authStart); err != nil || !marked {
		t.Fatalf("the tutor was not blocked: %v", err)
	}

	results := map[string]response{
		"a deleted account previewed": h.preview(t, deletedLink),
		"a deleted account posted":    h.unsubscribe(t, deletedLink, "", ""),
		"a blocked account previewed": h.preview(t, blockedLink),
		"a blocked account posted":    h.unsubscribe(t, blockedLink, "", ""),
	}

	for name, result := range results {
		if result.status != http.StatusUnprocessableEntity || code(t, result) != "token_invalid" {
			t.Fatalf("%s answered %d %s", name, result.status, result.body)
		}
	}
	if len(h.storedPreferences(t, deleted)) != 0 || len(h.storedPreferences(t, blocked)) != 0 {
		t.Fatal("a refused token stored a preference")
	}
}

func TestUnsubscribeRoutesAreLimitedPerAccountAndGroupAfterTheSignatureCheck(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	neighbour, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, "neighbour@example.org")
	link := h.mintLink(t, student, domain.EmailGroupHomework, authStart)
	parts := strings.Split(link, ".")
	forged := parts[0] + "." + parts[1] + "." + strings.Repeat("A", len(parts[2]))
	var allowed []response
	for index := range domain.UnsubscribeLimitPerWindow {
		if index%2 == 0 {
			allowed = append(allowed, h.preview(t, link))
		} else {
			allowed = append(allowed, h.unsubscribe(t, link, "", ""))
		}
	}
	countedBeforeTheLimit := h.unsubscribeCounted(t)

	limitedPreview := h.preview(t, link)
	limitedPost := h.unsubscribe(t, link, "", "")
	forgedAfterTheLimit := h.unsubscribe(t, forged, "", "")
	countedAfterTheForgery := h.unsubscribeCounted(t)
	otherGroup := h.unsubscribe(t, h.mintLink(t, student, domain.EmailGroupResults, authStart), "", "")
	otherAccount := h.unsubscribe(t, h.mintLink(t, neighbour, domain.EmailGroupHomework, authStart), "", "")
	h.at(t, authStart.Add(time.Hour))
	nextHour := h.preview(t, link)

	for index, result := range allowed {
		if result.status != http.StatusOK && result.status != http.StatusNoContent {
			t.Fatalf("request %d within the limit answered %d %s", index+1, result.status, result.body)
		}
	}
	for name, result := range map[string]response{"the preview": limitedPreview, "the unsubscribe": limitedPost} {
		if result.status != http.StatusTooManyRequests || code(t, result) != "rate_limited" || result.headers.Get("Retry-After") != "3600" {
			t.Fatalf("%s over the limit answered %d %v %s", name, result.status, result.headers, result.body)
		}
		if result.headers.Get("Cache-Control") != "no-store" || result.headers.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s over the limit may be stored or referred: %v", name, result.headers)
		}
	}
	if forgedAfterTheLimit.status != http.StatusUnprocessableEntity || code(t, forgedAfterTheLimit) != "token_invalid" || countedAfterTheForgery != countedBeforeTheLimit+2 {
		t.Fatalf("a forged token answered %d %s and counted %d", forgedAfterTheLimit.status, forgedAfterTheLimit.body, countedAfterTheForgery-countedBeforeTheLimit-2)
	}
	if otherGroup.status != http.StatusNoContent || otherAccount.status != http.StatusNoContent || nextHour.status != http.StatusOK {
		t.Fatalf("another group answered %d, another account %d, the next hour %d", otherGroup.status, otherAccount.status, nextHour.status)
	}
	if levels := h.refusals(t); !slices.Equal(levels["throttled"], []string{"INFO"}) {
		t.Fatalf("the throttling was recorded as %v", levels)
	}
}

func (h *authHarness) holdUserDeletion(t *testing.T, user domain.UserID) func() {
	t.Helper()
	return h.holdRow(t, "DELETE FROM users WHERE id = $1::uuid RETURNING "+lockedRowPosition, user.String())
}

func TestUnsubscribeRacingThePurgeAnswersTokenInvalid(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	student := h.seedUserAs(t, domain.RoleStudent, 2, studentEmail, accountPassword, currentHash)
	link := h.mintLink(t, student, domain.EmailGroupHomework, authStart)
	purged := h.holdUserDeletion(t, student)

	answer := h.sendInBackground(http.MethodPost, unsubscribePath+"?token="+url.QueryEscape(link), "", nil)
	h.waitForLockWaiters(t, 1)
	purged()
	refused := awaitAnswer(t, answer)

	if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "token_invalid" {
		t.Fatalf("the unsubscribe racing the purge answered %d %s", refused.status, refused.body)
	}
	if h.countRows(t, "SELECT count(user_id) FROM email_preferences WHERE user_id = $1::uuid", student.String()) != 0 || h.userExists(t, studentEmail) {
		t.Fatal("the purged account kept a row")
	}
	if levels := h.refusals(t); !slices.Equal(levels["account_state"], []string{"INFO"}) {
		t.Fatalf("the refusal was recorded as %v", levels)
	}
}

func TestUnsubscribeTokensNeverReachAccessRecords(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	valid := h.mintLink(t, student, domain.EmailGroupHomework, authStart)
	expired := h.mintLink(t, student, domain.EmailGroupResults, authStart.Add(-91*24*time.Hour))
	malformed := "v1.sentinel-malformed-token-text"
	sent := []string{valid, expired, malformed}

	for _, token := range sent {
		h.preview(t, token)
		h.unsubscribe(t, token, formType, oneClickForm)
	}

	logs := h.logs.String()
	for _, token := range sent {
		for _, part := range strings.Split(token, ".")[1:] {
			if strings.Contains(logs, part) {
				t.Fatalf("a token part reached the records: %s", logs)
			}
		}
	}
	if strings.Contains(logs, "token=") || strings.Contains(logs, "sentinel") {
		t.Fatalf("a query string reached the records: %s", logs)
	}
	for _, route := range []string{`"route":"GET /api/v1/unsubscribe/preview"`, `"route":"POST /api/v1/unsubscribe"`} {
		if strings.Count(logs, route) != len(sent) {
			t.Fatalf("the access records do not name %s once per request: %s", route, logs)
		}
	}
}

func TestUnsubscribeResponsesAreNotStored(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	link := h.mintLink(t, student, domain.EmailGroupHomework, authStart)

	results := map[string]response{
		"a preview":             h.preview(t, link),
		"an unsubscribe":        h.unsubscribe(t, link, formType, oneClickForm),
		"an invalid token":      h.preview(t, "x"),
		"an unsupported body":   h.unsubscribe(t, link, "text/plain", oneClickForm),
		"a form without fields": h.unsubscribe(t, link, formType, ""),
	}

	for name, result := range results {
		if result.headers.Get("Cache-Control") != "no-store" || result.headers.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s (%d) may be stored or referred: %v", name, result.status, result.headers)
		}
	}
}

func TestPurgeDeletesEmailPreferences(t *testing.T) {
	t.Parallel()
	paths := map[string]struct {
		role  domain.Role
		state domain.AccountState
		purge func(t *testing.T, h *authHarness, token string)
	}{
		"the 30-day purge of a tutor": {role: domain.RoleTutor, state: domain.AccountActive, purge: func(t *testing.T, h *authHarness, token string) {
			t.Helper()
			if deleted := h.deleteMe(t, token, accountPassword); deleted.status != http.StatusNoContent {
				t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
			}
			h.at(t, authStart.Add(30*24*time.Hour))
			h.tick(t)
		}},
		"the 30-day purge": {role: domain.RoleStudent, state: domain.AccountActive, purge: func(t *testing.T, h *authHarness, token string) {
			t.Helper()
			if deleted := h.deleteMe(t, token, accountPassword); deleted.status != http.StatusNoContent {
				t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
			}
			h.at(t, authStart.Add(30*24*time.Hour))
			h.tick(t)
		}},
		"the 7-day cleanup": {role: domain.RoleStudent, state: domain.AccountUnverified, purge: func(t *testing.T, h *authHarness, _ string) {
			t.Helper()
			h.at(t, authStart.Add(7*24*time.Hour))
			h.tick(t)
		}},
		"the immediate purge of an unconfirmed account": {role: domain.RoleStudent, state: domain.AccountUnverified, purge: func(t *testing.T, h *authHarness, token string) {
			t.Helper()
			if deleted := h.deleteMe(t, token, accountPassword); deleted.status != http.StatusNoContent {
				t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
			}
		}},
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			student, token := h.signedInAs(t, path.role, path.state, 2, studentEmail)
			neighbour, neighbourToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, "neighbour@example.org")
			h.savePreferences(t, token, `{"groups":[{"group":"message_digest","enabled":false}]}`)
			h.savePreferences(t, neighbourToken, `{"groups":[{"group":"results","enabled":false}]}`)
			saved := h.storedPreferences(t, student)

			path.purge(t, h, token)

			if !slices.Equal(saved, []string{"message_digest:false"}) {
				t.Fatalf("the preference was not saved before the purge: %v", saved)
			}
			if owned := h.rowsOwnedBy(t, student); len(owned) != 0 {
				t.Fatalf("rows of the purged account remain: %v", owned)
			}
			if kept := h.storedPreferences(t, neighbour); !slices.Equal(kept, []string{"results:false"}) {
				t.Fatalf("the purge touched another account's preferences: %v", kept)
			}
		})
	}
}

package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	accountPassword  = "correct horse battery"
	studentEmail     = "student@example.org"
	codeRegenerated  = `"msg":"tutor_code.regenerated"`
	displayedCodeFmt = `^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$`
)

var displayedCode = regexp.MustCompile(displayedCodeFmt)

type tutorCode struct {
	Code   string `json:"code"`
	Active bool   `json:"active"`
}

func (h *authHarness) signedInAs(t *testing.T, role domain.Role, state domain.AccountState, id byte, email string) (domain.UserID, string) {
	t.Helper()
	encoded, err := h.hasher.Hash(context.Background(), accountPassword)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.UserID{id}
	var verifiedAt *time.Time
	if state == domain.AccountActive {
		verifiedAt = &authStart
	}
	err = h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: role, State: state, Email: email, EmailVerifiedAt: verifiedAt,
		PasswordHash: encoded, Name: "A language tutor", Language: domain.LanguageEnglish, Timezone: "Europe/Moscow",
		Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: authStart, CreatedAt: authStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if role == domain.RoleTutor {
		err = h.store.Repositories().TutorProfiles.Create(context.Background(), repositories.NewTutorProfile{
			UserID: user, FileQuotaBytes: 5_000_000_000, RecordingQuotaBytes: 50_000_000_000, CreatedAt: authStart,
		}, tutorCodeSource(user))
		if err != nil {
			t.Fatal(err)
		}
	}
	return user, accessToken(t, h.login(t, email, accountPassword))
}

func (h *authHarness) readCode(t *testing.T, token string) (response, tutorCode) {
	t.Helper()
	result := h.send(t, http.MethodGet, "/api/v1/tutor/code", "", map[string]string{"Authorization": "Bearer " + token})
	return result, decodeCode(t, result)
}

func (h *authHarness) regenerateCode(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodPost, "/api/v1/tutor/code/regenerate", "", map[string]string{"Authorization": "Bearer " + token})
}

func decodeCode(t *testing.T, result response) tutorCode {
	t.Helper()
	var decoded tutorCode
	if result.status != http.StatusOK {
		return decoded
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("not a tutor code: %s", result.body)
	}
	return decoded
}

func (h *authHarness) storedCode(t *testing.T, tutor domain.UserID) domain.TutorCode {
	t.Helper()
	profile, err := h.store.Repositories().TutorProfiles.ByUser(context.Background(), tutor)
	if err != nil {
		t.Fatal(err)
	}
	return profile.Code
}

func TestTutorReadsAndRegeneratesTheCodeWithoutLoggingIt(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	original := h.storedCode(t, tutor)

	readBefore, before := h.readCode(t, token)
	regenerated := h.regenerateCode(t, token)
	_, after := h.readCode(t, token)

	if readBefore.status != http.StatusOK || before.Code != original.Display() || !before.Active || readBefore.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("reading the code answered %d %s", readBefore.status, readBefore.body)
	}
	fresh := decodeCode(t, regenerated)
	if regenerated.status != http.StatusOK || !fresh.Active || !displayedCode.MatchString(fresh.Code) || fresh.Code == before.Code {
		t.Fatalf("regenerating answered %d %s", regenerated.status, regenerated.body)
	}
	if after.Code != fresh.Code || h.storedCode(t, tutor).Display() != fresh.Code {
		t.Fatalf("the regenerated code %s was not kept: %s", fresh.Code, after.Code)
	}
	logs := h.logs.String()
	for _, secret := range []string{string(original), original.Display(), fresh.Code, strings.ReplaceAll(fresh.Code, "-", "")} {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain the tutor code %q", secret)
		}
	}
	if !strings.Contains(logs, codeRegenerated) || !strings.Contains(logs, `"tutor_id":"`+tutor.String()+`"`) {
		t.Fatalf("no tutor_code.regenerated record naming the tutor: %s", logs)
	}
}

func TestUnverifiedTutorCannotRegenerateTheCode(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	original := h.storedCode(t, tutor)

	read, current := h.readCode(t, token)
	refused := h.regenerateCode(t, token)

	if read.status != http.StatusOK || current.Code != original.Display() || current.Active {
		t.Fatalf("an unverified tutor reading the code got %d %s", read.status, read.body)
	}
	if refused.status != http.StatusForbidden || code(t, refused) != "email_not_verified" {
		t.Fatalf("an unverified tutor regenerating got %d %s", refused.status, refused.body)
	}
	if h.storedCode(t, tutor) != original || strings.Contains(h.logs.String(), codeRegenerated) {
		t.Fatal("a refused regeneration changed the code")
	}
}

func TestCodeRegenerationLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	seen := map[string]bool{h.storedCode(t, tutor).Display(): true}
	for attempt := range 10 {
		result := h.regenerateCode(t, token)
		fresh := decodeCode(t, result)
		if result.status != http.StatusOK || seen[fresh.Code] {
			t.Fatalf("regeneration %d answered %d %s", attempt+1, result.status, result.body)
		}
		seen[fresh.Code] = true
	}
	tenth := h.storedCode(t, tutor)

	eleventh := h.regenerateCode(t, token)
	twelfth := h.regenerateCode(t, token)
	h.advance(t, 15*time.Hour)
	nextDay := h.regenerateCode(t, accessToken(t, h.login(t, tutorEmail, accountPassword)))

	for name, refused := range map[string]response{"eleventh": eleventh, "twelfth": twelfth} {
		if refused.status != http.StatusTooManyRequests || code(t, refused) != "rate_limited" {
			t.Fatalf("the %s regeneration answered %d %s", name, refused.status, refused.body)
		}
		if refused.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("the %s regeneration has Cache-Control %q", name, refused.headers.Get("Cache-Control"))
		}
		if retry, err := strconv.Atoi(refused.headers.Get("Retry-After")); err != nil || retry != int((15*time.Hour).Seconds()) {
			t.Fatalf("the %s regeneration has Retry-After %q", name, refused.headers.Get("Retry-After"))
		}
	}
	if nextDay.status != http.StatusOK || decodeCode(t, nextDay).Code == tenth.Display() {
		t.Fatalf("the first regeneration of the next day answered %d %s", nextDay.status, nextDay.body)
	}
	if strings.Count(h.logs.String(), codeRegenerated) != 11 {
		t.Fatalf("expected 11 regeneration records")
	}
}

func TestConcurrentRegenerationsStopAtTheLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	original := h.storedCode(t, tutor).Display()
	const attempts = 20
	start := make(chan struct{})
	results := make(chan response, attempts)
	failures := make(chan error, attempts)
	for range attempts {
		go func() {
			<-start
			result, err := h.trySend(http.MethodPost, "/api/v1/tutor/code/regenerate", "", map[string]string{"Authorization": "Bearer " + token})
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}

	close(start)
	statuses := map[int]int{}
	codes := map[string]bool{}
	for range attempts {
		select {
		case err := <-failures:
			t.Fatal(err)
		case result := <-results:
			statuses[result.status]++
			if result.status == http.StatusOK {
				codes[decodeCode(t, result).Code] = true
			}
		case <-time.After(pauseDeadline):
			t.Fatal("the concurrent regenerations did not finish")
		}
	}

	if statuses[http.StatusOK] != 10 || statuses[http.StatusTooManyRequests] != 10 || len(statuses) != 2 {
		t.Fatalf("statuses %v", statuses)
	}
	if len(codes) != 10 || codes[original] {
		t.Fatalf("expected 10 distinct new codes, got %d", len(codes))
	}
	if !codes[h.storedCode(t, tutor).Display()] {
		t.Fatal("the stored code is none of the answered codes")
	}
}

var ignoredRefusalHeaders = map[string]bool{"X-Request-Id": true, "Date": true, "Content-Length": true}

func refusal(t *testing.T, result response) string {
	t.Helper()
	var decoded map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded["error"] == nil {
		t.Fatalf("not a structured error: %d %s", result.status, result.body)
	}
	delete(decoded["error"], "request_id")
	normalised, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var headers []string
	for name, values := range result.headers {
		if !ignoredRefusalHeaders[http.CanonicalHeaderKey(name)] {
			headers = append(headers, http.CanonicalHeaderKey(name)+": "+strings.Join(values, ", "))
		}
	}
	slices.Sort(headers)
	return strconv.Itoa(result.status) + " " + string(normalised) + "\n" + strings.Join(headers, "\n")
}

func (h *authHarness) unknownRouteRefusal(t *testing.T) string {
	t.Helper()
	return refusal(t, h.send(t, http.MethodGet, "/api/v1/unknown-route", "", nil))
}

func requestFor(operation api.OperationAudience) (string, string) {
	segments := strings.Split(operation.Path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, "{") {
			segments[index] = "0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a"
		}
	}
	body := ""
	if operation.Method != http.MethodGet {
		body = "{}"
	}
	return strings.Join(segments, "/"), body
}

func jsonCall(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json", "Origin": publicOrigin}
}

func (h *authHarness) callAs(t *testing.T, token string, operation api.OperationAudience) response {
	t.Helper()
	path, body := requestFor(operation)
	return h.send(t, operation.Method, path, body, jsonCall(token))
}

func operationsFor(role domain.Role) []api.OperationAudience {
	var matching []api.OperationAudience
	for _, operation := range api.Operations() {
		if audienceRole, specific := operation.Audience.Role(); specific && audienceRole == role {
			matching = append(matching, operation)
		}
	}
	return matching
}

func assertRoleRefusals(t *testing.T, h *authHarness, operations []api.OperationAudience, tokens map[string]string) {
	t.Helper()
	unknown := h.unknownRouteRefusal(t)
	for _, operation := range operations {
		for who, token := range tokens {
			result := h.callAs(t, token, operation)

			if got := refusal(t, result); got != unknown {
				t.Errorf("%s %s called by %s answered\n%s\ninstead of\n%s", operation.Method, operation.Path, who, got, unknown)
			}
		}
	}
}

func TestStudentCallingTutorRoutesGetsNotFound(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	original := h.storedCode(t, tutor)
	_, active := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, unverified := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, "unconfirmed@example.org")
	operations := operationsFor(domain.RoleTutor)
	if len(operations) == 0 {
		t.Fatal("no tutor operations are registered")
	}

	assertRoleRefusals(t, h, operations, map[string]string{"an active student": active, "an unverified student": unverified})

	if h.storedCode(t, tutor) != original || strings.Contains(h.logs.String(), codeRegenerated) {
		t.Fatal("a student's call changed a tutor's data")
	}
}

var studentRoutesOfSliceThree = []string{
	"GET /api/v1/tutors",
	"POST /api/v1/tutors/{relationshipId}/end",
	"GET /api/v1/join-requests/sent",
}

func TestTutorCallingStudentRoutesGetsNotFound(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, active := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unverified := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 2, "unconfirmed@example.org")
	operations := operationsFor(domain.RoleStudent)
	registered := map[string]api.Audience{}
	for _, operation := range api.Operations() {
		registered[strings.ToUpper(operation.Method)+" "+operation.Path] = operation.Audience
	}
	for _, route := range studentRoutesOfSliceThree {
		audience, found := registered[route]
		if role, specific := audience.Role(); found && (!specific || role != domain.RoleStudent) {
			t.Fatalf("%s is registered for %q instead of a student audience", route, audience)
		}
	}
	if len(operations) == 0 {
		t.Fatal("no student operations are registered")
	}

	assertRoleRefusals(t, h, operations, map[string]string{"an active tutor": active, "an unverified tutor": unverified})
}

type denialCase func(t *testing.T, h *authHarness) (outOfScope response, random response)

var denialCases = map[string]denialCase{
	"GET /api/v1/avatars/{id}":                            anotherAccountsPendingUploadIsNoAvatar,
	"GET /api/v1/students/{id}":                           anotherTutorsCard(http.MethodGet, "", ""),
	"PATCH /api/v1/students/{id}":                         anotherTutorsCard(http.MethodPatch, "", `{"display_name":"Renamed","email":"renamed@example.org"}`),
	"DELETE /api/v1/students/{id}":                        anotherTutorsCard(http.MethodDelete, "", ""),
	"POST /api/v1/students/{id}/invitation":               anotherTutorsCard(http.MethodPost, "/invitation", ""),
	"POST /api/v1/students/{id}/invitation/send":          anotherTutorsCard(http.MethodPost, "/invitation/send", ""),
	"GET /api/v1/invitations/{token}":                     aBlockedTutorsLinkIsUnknown,
	"POST /api/v1/students/{id}/remove":                   anotherTutorsRemovableStudent,
	"POST /api/v1/students/{id}/end":                      anotherTutorsActiveStudent,
	"POST /api/v1/tutors/{relationshipId}/end":            anotherStudentsRelationship,
	"POST /api/v1/invitations/{token}/accept":             aBlockedTutorsLinkCannotBeAccepted,
	"POST /api/v1/join-requests/{id}/accept":              anotherTutorsJoinRequest("/accept", ""),
	"POST /api/v1/join-requests/{id}/decline":             anotherTutorsJoinRequest("/decline", `{"block":true}`),
	"DELETE /api/v1/tutor/blocked-requesters/{studentId}": anotherTutorsBlockedStudent,
	"GET /api/v1/groups/{id}":                             anotherTutorsGroup(http.MethodGet, "", ""),
	"PATCH /api/v1/groups/{id}":                           anotherTutorsGroup(http.MethodPatch, "", `{"name":"Taken over","colour":"ink"}`),
	"POST /api/v1/groups/{id}/archive":                    anotherTutorsGroup(http.MethodPost, "/archive", ""),
	"POST /api/v1/groups/{id}/unarchive":                  anotherTutorsGroup(http.MethodPost, "/unarchive", ""),
}

var bodyIdentifierDenialCases = map[string]denialCase{
	"POST /api/v1/notifications/read": anotherAccountsNotification,
}

var denialStatuses = map[string]int{
	"POST /api/v1/invitations/{token}/accept": http.StatusConflict,
}

const sharedStudentCard byte = 0x7f

func (h *authHarness) shareStudentWith(t *testing.T, intruder domain.UserID, student domain.UserID) {
	t.Helper()
	h.seedCard(t, intruder, sharedStudentCard, domain.RelationshipActive, &student)
}

func anotherTutorsRemovableStudent(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owner, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruderID, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	card := h.seedCard(t, owner, 1, domain.RelationshipReclaimPending, &student)
	h.shareStudentWith(t, intruderID, student)
	squatter, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 4, "squatter@example.org")
	provisional := h.seedCard(t, owner, 2, domain.RelationshipProvisional, &squatter)
	outOfScope := h.cardCall(t, intruder, http.MethodPost, card, "/remove", "")
	provisionalOutOfScope := h.cardCall(t, intruder, http.MethodPost, provisional, "/remove", "")
	random := h.cardCall(t, intruder, http.MethodPost, randomCardID, "/remove", "")
	if provisionalOutOfScope.status != random.status || refusal(t, provisionalOutOfScope) != refusal(t, random) {
		t.Fatalf("removing another tutor's provisional student answered %d %s", provisionalOutOfScope.status, provisionalOutOfScope.body)
	}
	if h.storedRelationship(t, card).State != string(domain.RelationshipReclaimPending) ||
		h.storedRelationship(t, provisional).State != string(domain.RelationshipProvisional) {
		t.Fatal("another tutor removed the student")
	}
	return outOfScope, random
}

func aBlockedTutorsLinkCannotBeAccepted(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := linkToken(t, h.newCard(t, token, cardName, "").Invitation)
	if _, err := h.store.Repositories().Users.Block(context.Background(), tutor, authStart); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	outOfScope := h.send(t, http.MethodPost, invitationPath+link+"/accept", studentSignUp{Email: newcomerEmail}.body(), headers)
	random := h.send(t, http.MethodPost, invitationPath+base64.RawURLEncoding.EncodeToString(make([]byte, 32))+"/accept",
		studentSignUp{Email: otherEmail}.body(), headers)
	return outOfScope, random
}

func anotherTutorsCard(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		intruderID, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
		invited := h.newCard(t, ownerToken, cardName, cardEmail)
		shared := h.linkedCard(t, owner, 10, domain.RelationshipActive)
		h.shareStudentWith(t, intruderID, domain.UserID{10})
		outOfScope := h.cardCall(t, intruder, method, invited.ID, suffix, body)
		sharedOutOfScope := h.cardCall(t, intruder, method, shared, suffix, body)
		random := h.cardCall(t, intruder, method, randomCardID, suffix, body)
		if refusal(t, sharedOutOfScope) != refusal(t, random) {
			t.Fatalf("the card of a shared student answered %d %s, a random id %d %s", sharedOutOfScope.status, sharedOutOfScope.body, random.status, random.body)
		}
		if h.storedRelationship(t, invited.ID).State != string(domain.RelationshipInvited) || h.storedRelationship(t, shared).State != string(domain.RelationshipActive) {
			t.Fatal("another tutor changed a card")
		}
		return outOfScope, random
	}
}

func aBlockedTutorsLinkIsUnknown(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	link := linkToken(t, h.newCard(t, token, cardName, cardEmail).Invitation)
	if _, err := h.store.Repositories().Users.Block(context.Background(), tutor, authStart); err != nil {
		t.Fatal(err)
	}
	outOfScope := h.invitationPage(t, link)
	random := h.invitationPage(t, base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	return outOfScope, random
}

func anotherAccountsPendingUploadIsNoAvatar(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, owner := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	pending := uploadedID(t, h.upload(t, owner, "me.png", pngBytes(t, 2, 2)))
	outOfScope := h.send(t, http.MethodGet, "/api/v1/avatars/"+pending, "", nil)
	random := h.send(t, http.MethodGet, "/api/v1/avatars/0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a", "", nil)
	return outOfScope, random
}

func TestEveryScopedRouteHasADenialCase(t *testing.T) {
	t.Parallel()
	scoped := map[string]bool{}
	for _, operation := range api.Operations() {
		if strings.Contains(operation.Path, "{") {
			scoped[strings.ToUpper(operation.Method)+" "+operation.Path] = true
		}
	}

	for key := range denialStatuses {
		if _, found := denialCases[key]; !found {
			t.Errorf("the denial status of %s names no denial case", key)
		}
	}
	for key := range denialCases {
		if !scoped[key] {
			t.Errorf("the denial case %s names no operation with a path identifier", key)
		}
	}
	for key := range scoped {
		denial, found := denialCases[key]
		if !found {
			t.Errorf("%s has a path identifier and no denial case", key)
			continue
		}
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)

			expected, overridden := denialStatuses[key]
			if !overridden {
				expected = http.StatusNotFound
			}

			outOfScope, random := denial(t, h)

			if outOfScope.status != expected || refusal(t, outOfScope) != refusal(t, random) {
				t.Fatalf("out of scope %d %s, random %d %s", outOfScope.status, outOfScope.body, random.status, random.body)
			}
		})
	}
}

func TestEveryBodyIdentifierRouteHasADenialCase(t *testing.T) {
	t.Parallel()
	operations := map[string]bool{}
	for _, operation := range api.Operations() {
		operations[strings.ToUpper(operation.Method)+" "+operation.Path] = true
	}
	for key, denial := range bodyIdentifierDenialCases {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)

			outOfScope, random := denial(t, h)

			if !operations[key] {
				t.Fatalf("the denial case %s names no operation", key)
			}
			if outOfScope.status != http.StatusNotFound || refusal(t, outOfScope) != refusal(t, random) {
				t.Fatalf("out of scope %d %s, random %d %s", outOfScope.status, outOfScope.body, random.status, random.body)
			}
		})
	}
}

type publishedProperty struct {
	Type json.RawMessage `json:"type"`
}

type publishedSchema struct {
	Ref        string                       `json:"$ref"`
	Required   []string                     `json:"required"`
	Properties map[string]publishedProperty `json:"properties"`
}

type publishedOperation struct {
	RequestBody *struct {
		Content map[string]struct {
			Schema publishedSchema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

type publishedDocument struct {
	Paths      map[string]map[string]publishedOperation `json:"paths"`
	Components struct {
		Schemas map[string]publishedSchema `json:"schemas"`
	} `json:"components"`
}

func (d publishedDocument) operation(t *testing.T, operation api.OperationAudience) publishedOperation {
	t.Helper()
	published, found := d.Paths[operation.Path][strings.ToLower(operation.Method)]
	if !found {
		t.Fatalf("%s %s is not published", operation.Method, operation.Path)
	}
	return published
}

func (d publishedDocument) bodySchema(t *testing.T, operation api.OperationAudience) (publishedSchema, bool) {
	t.Helper()
	published := d.operation(t, operation)
	if published.RequestBody == nil {
		return publishedSchema{}, false
	}
	schema := published.RequestBody.Content["application/json"].Schema
	if name, referenced := strings.CutPrefix(schema.Ref, "#/components/schemas/"); referenced {
		schema = d.Components.Schemas[name]
	}
	return schema, true
}

func (s publishedSchema) firstStringProperty() (string, bool) {
	return s.firstPropertyOf(func(kind string) bool { return kind == `"string"` })
}

var typesRefusingAString = map[string]bool{`"boolean"`: true, `"integer"`: true, `"number"`: true, `"array"`: true, `"object"`: true}

func (s publishedSchema) firstPropertyRefusingAString() (string, bool) {
	return s.firstPropertyOf(func(kind string) bool { return typesRefusingAString[kind] })
}

func (s publishedSchema) firstPropertyOf(matches func(kind string) bool) (string, bool) {
	names := make([]string, 0, len(s.Properties))
	for name, property := range s.Properties {
		if matches(string(property.Type)) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

func operationProbes(schema publishedSchema) map[string]bodyProbe {
	probes := map[string]bodyProbe{
		"malformed JSON":         {body: `{"`, contentType: "application/json"},
		"an unexpected property": {body: `{"unexpected_property":true}`, contentType: "application/json"},
		"a wrong content type":   {body: `{}`, contentType: "text/plain"},
	}
	if len(schema.Required) > 0 {
		probes["an empty object missing "+strings.Join(schema.Required, ", ")] = bodyProbe{body: "{}", contentType: "application/json"}
	}
	if name, found := schema.firstStringProperty(); found {
		probes["a number for "+name] = bodyProbe{body: fmt.Sprintf(`{%q:1}`, name), contentType: "application/json"}
	}
	if name, found := schema.firstPropertyRefusingAString(); found {
		probes["a string for "+name] = bodyProbe{body: fmt.Sprintf(`{%q:"text"}`, name), contentType: "application/json"}
	}
	return probes
}

func loadPublishedDocument(t *testing.T) publishedDocument {
	t.Helper()
	encoded, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var document publishedDocument
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

type bodyProbe struct {
	body        string
	contentType string
}

func bodyProbesFor(missing []string) map[string]bodyProbe {
	return map[string]bodyProbe{
		"malformed JSON": {body: `{"`, contentType: "application/json"},
		"an empty object missing " + strings.Join(missing, ", "): {body: "{}", contentType: "application/json"},
		"an unexpected property":                                 {body: `{"name":"a","unexpected_property":true}`, contentType: "application/json"},
		"a field of the wrong type":                              {body: `{"name":1,"count":"many"}`, contentType: "application/json"},
		"a wrong content type":                                   {body: `{"name":"a","count":1}`, contentType: "text/plain"},
	}
}

func (h *authHarness) assertWrongRoleProbes(t *testing.T, method string, path string, probes map[string]bodyProbe, tokens map[string]string) {
	t.Helper()
	unknown := h.unknownRouteRefusal(t)
	for who, token := range tokens {
		for probe, request := range probes {
			headers := jsonCall(token)
			headers["Content-Type"] = request.contentType

			result := h.send(t, method, path, request.body, headers)

			if got := refusal(t, result); got != unknown {
				t.Errorf("%s %s called by %s with %s answered\n%s\ninstead of\n%s", method, path, who, probe, got, unknown)
			}
		}
	}
}

func (h *authHarness) wrongRoleTokens(t *testing.T) map[domain.Role]map[string]string {
	t.Helper()
	_, activeTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unverifiedTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 2, "unconfirmed-tutor@example.org")
	_, activeStudent := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	_, unverifiedStudent := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 4, "unconfirmed-student@example.org")
	return map[domain.Role]map[string]string{
		domain.RoleTutor:   {"an active student": activeStudent, "an unverified student": unverifiedStudent},
		domain.RoleStudent: {"an active tutor": activeTutor, "an unverified tutor": unverifiedTutor},
	}
}

func TestUnconfirmedEmailIsRefusedBeforeTheBodyIsRead(t *testing.T) {
	t.Parallel()
	h := newAuthHarnessServing(t, nil, api.NewHandlerWithBodyProbe)
	_, unverified := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	_, active := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "confirmed-tutor@example.org")
	refusals := map[string]response{}
	for probe, request := range bodyProbesFor([]string{"name", "count"}) {
		headers := jsonCall(unverified)
		headers["Content-Type"] = request.contentType
		refusals[probe] = h.send(t, http.MethodPost, api.BodyProbePath, request.body, headers)
	}

	invalid := h.send(t, http.MethodPost, api.BodyProbePath, "{}", jsonCall(active))

	var first string
	for probe, result := range refusals {
		if result.status != http.StatusForbidden || code(t, result) != "email_not_verified" {
			t.Errorf("an unverified tutor sending %s answered %d %s", probe, result.status, result.body)
		}
		if first == "" {
			first = refusal(t, result)
		}
		if got := refusal(t, result); got != first {
			t.Errorf("an unverified tutor sending %s answered\n%s\ninstead of\n%s", probe, got, first)
		}
	}
	if invalid.status != http.StatusUnprocessableEntity || code(t, invalid) != "validation_failed" || invalid.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("a confirmed tutor sending an empty object answered %d %v %s", invalid.status, invalid.headers, invalid.body)
	}
}

func TestWrongRoleGetsNotFoundBeforeTheBodyIsRead(t *testing.T) {
	t.Parallel()
	h := newAuthHarnessServing(t, nil, api.NewHandlerWithBodyProbe)
	tokens := h.wrongRoleTokens(t)

	h.assertWrongRoleProbes(t, http.MethodPost, api.BodyProbePath, bodyProbesFor([]string{"name", "count"}), tokens[domain.RoleTutor])
}

func roleSpecificOperationsWithABody(document publishedDocument) map[string]bool {
	audiences := map[string]api.Audience{}
	for _, operation := range api.Operations() {
		audiences[strings.ToUpper(operation.Method)+" "+operation.Path] = operation.Audience
	}
	withBody := map[string]bool{}
	for path, methods := range document.Paths {
		for method, published := range methods {
			key := strings.ToUpper(method) + " " + path
			if _, specific := audiences[key].Role(); specific && published.RequestBody != nil {
				withBody[key] = true
			}
		}
	}
	return withBody
}

func TestWrongRoleGetsNotFoundBeforeBodyValidation(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tokens := h.wrongRoleTokens(t)
	_, confirmedTutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "confirmed-tutor@example.org")
	_, confirmedStudent := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, "confirmed-student@example.org")
	rightRole := map[domain.Role]string{domain.RoleTutor: confirmedTutor, domain.RoleStudent: confirmedStudent}
	document := loadPublishedDocument(t)
	exercised := map[string]bool{}

	for _, operation := range api.Operations() {
		role, specific := operation.Audience.Role()
		if !specific || operation.Method == http.MethodGet {
			continue
		}
		path, _ := requestFor(operation)
		schema, takesBody := document.bodySchema(t, operation)
		probes := operationProbes(schema)
		h.assertWrongRoleProbes(t, operation.Method, path, probes, tokens[role])
		if takesBody {
			h.assertProbesAreRefusedBeforeScope(t, operation, path, probes, rightRole[role])
		}
		exercised[strings.ToUpper(operation.Method)+" "+operation.Path] = true
	}

	for key := range roleSpecificOperationsWithABody(document) {
		if !exercised[key] {
			t.Errorf("%s takes a body and was not probed by the wrong role", key)
		}
	}
}

var bodyRefusals = map[int]bool{http.StatusBadRequest: true, http.StatusUnsupportedMediaType: true, http.StatusUnprocessableEntity: true}

func (h *authHarness) assertProbesAreRefusedBeforeScope(t *testing.T, operation api.OperationAudience, path string, probes map[string]bodyProbe, token string) {
	t.Helper()
	for probe, request := range probes {
		headers := jsonCall(token)
		headers["Content-Type"] = request.contentType

		result := h.send(t, operation.Method, path, request.body, headers)

		if !bodyRefusals[result.status] {
			t.Errorf("%s %s called by the right role with %s answered %d %s instead of a body refusal", operation.Method, path, probe, result.status, result.body)
		}
	}
}

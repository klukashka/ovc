package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
)

const (
	joinRequestsPath  = "/api/v1/join-requests"
	blockedPath       = "/api/v1/tutor/blocked-requesters"
	unknownCode       = "2222-2222"
	requestCreated    = `"msg":"join_request.created"`
	requestReleased   = `"msg":"join_request.released"`
	requestDecided    = `"msg":"join_request.decided"`
	requestFreed      = `"msg":"join_request.unsuppressed"`
	requestThrottled  = `"msg":"join_request.throttled"`
	requestingName    = "Anna Requesting Student"
	requestingEmail   = "anna@example.org"
	entrantEmail      = "entrant@example.org"
	visibleRequests   = "SELECT count(id) FROM join_requests WHERE tutor_id = $1::uuid AND state = 'pending' AND suppressed = 'none'"
	requestsOfStudent = "SELECT count(id) FROM join_requests WHERE student_id = $1::uuid"
)

type sentRequest struct {
	ID        string     `json:"id"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	DecidedAt *time.Time `json:"decided_at"`
}

type requestingStudent struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	AvatarID *string `json:"avatar_id"`
}

type tutorRequest struct {
	ID        string            `json:"id"`
	Student   requestingStudent `json:"student"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type blockedStudent struct {
	StudentID string    `json:"student_id"`
	Name      string    `json:"name"`
	AvatarID  *string   `json:"avatar_id"`
	BlockedAt time.Time `json:"blocked_at"`
}

func codeBody(code string) string {
	return fmt.Sprintf(`{"code":%s}`, jsonText(code))
}

func (h *authHarness) enterCode(t *testing.T, token string, code string) response {
	t.Helper()
	return h.send(t, http.MethodPost, joinRequestsPath, codeBody(code), jsonCall(token))
}

func (h *authHarness) tryEnterCode(token string, code string) <-chan pendingAnswer {
	return h.sendInBackground(http.MethodPost, joinRequestsPath, codeBody(code), jsonCall(token))
}

func sentRequestOf(t *testing.T, result response) sentRequest {
	t.Helper()
	var decoded struct {
		Request *sentRequest `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Request == nil {
		t.Fatalf("not a sent join request: %d %s", result.status, result.body)
	}
	return *decoded.Request
}

func requestIDOf(t *testing.T, request sentRequest) domain.JoinRequestID {
	t.Helper()
	id, valid := domain.ParseUUID[domain.JoinRequestID](request.ID)
	if !valid {
		t.Fatalf("the request identifier %q is not a UUID", request.ID)
	}
	return id
}

func (h *authHarness) enteredRequest(t *testing.T, token string, code string) domain.JoinRequestID {
	t.Helper()
	return requestIDOf(t, sentRequestOf(t, h.enterCode(t, token, code)))
}

func (h *authHarness) tutorRequests(t *testing.T, token string) []tutorRequest {
	t.Helper()
	result := h.send(t, http.MethodGet, joinRequestsPath+"?limit=100", "", jsonCall(token))
	var decoded struct {
		Items []tutorRequest `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Items == nil {
		t.Fatalf("not a list of join requests: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the list has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return decoded.Items
}

func (h *authHarness) sentRequests(t *testing.T, token string) (response, []sentRequest) {
	t.Helper()
	result := h.send(t, http.MethodGet, joinRequestsPath+"/sent", "", jsonCall(token))
	var decoded struct {
		Items []sentRequest `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Items == nil {
		t.Fatalf("not a list of sent requests: %d %s", result.status, result.body)
	}
	return result, decoded.Items
}

func (h *authHarness) acceptRequest(t *testing.T, token string, id string) response {
	t.Helper()
	return h.send(t, http.MethodPost, joinRequestsPath+"/"+id+"/accept", "", jsonCall(token))
}

func (h *authHarness) declineRequest(t *testing.T, token string, id string, block bool) response {
	t.Helper()
	return h.send(t, http.MethodPost, joinRequestsPath+"/"+id+"/decline", fmt.Sprintf(`{"block":%t}`, block), jsonCall(token))
}

func (h *authHarness) blockedStudents(t *testing.T, token string) []blockedStudent {
	t.Helper()
	result := h.send(t, http.MethodGet, blockedPath, "", jsonCall(token))
	var decoded struct {
		Items []blockedStudent `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Items == nil {
		t.Fatalf("not a list of blocked students: %d %s", result.status, result.body)
	}
	return decoded.Items
}

func (h *authHarness) unblock(t *testing.T, token string, student domain.UserID) response {
	t.Helper()
	return h.send(t, http.MethodDelete, blockedPath+"/"+student.String(), "", jsonCall(token))
}

func studentCodeSignUp(email string, code string) signUp {
	return signUp{
		Role: "student", Name: requestingName, Email: email, Password: newcomerPassword, Language: "en", Timezone: "Europe/Moscow",
		Consent: true, TutorCode: code,
	}
}

func (h *authHarness) confirmInOwnBrowser(t *testing.T, email string, token string) response {
	t.Helper()
	h.advance(t, 61*time.Second)
	h.resend(t, email)
	h.tick(t)
	confirmation := latestToken(t, h.mailTo(t, email), confirmSubject)
	return h.verify(t, fmt.Sprintf(`{"token":%q}`, confirmation), token)
}

func (h *authHarness) visibleCount(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, visibleRequests, tutor.String())
}

func (h *authHarness) holdTutorProfile(t *testing.T, tutor domain.UserID) func() {
	t.Helper()
	holder, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if released {
			return
		}
		if err := holder.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if _, err := holder.Exec(context.Background(), "SELECT 1 FROM tutor_profiles WHERE user_id = $1 FOR NO KEY UPDATE", tutor); err != nil {
		t.Fatal(err)
	}
	return func() {
		released = true
		if err := holder.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *authHarness) oldestFiller(t *testing.T, tutor domain.UserID, prefix string) string {
	t.Helper()
	var id string
	err := h.database.Pool.QueryRow(context.Background(), `
SELECT join_requests.id::text FROM join_requests JOIN users ON users.id = join_requests.student_id
WHERE join_requests.tutor_id = $1 AND users.email LIKE $2 AND join_requests.state = 'pending'
ORDER BY join_requests.created_at, join_requests.id LIMIT 1`, tutor, prefix+"%").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *authHarness) suppressedForLimit(t *testing.T, tutor domain.UserID, id byte, email string) domain.JoinRequestID {
	t.Helper()
	_, waiting := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, id, email)
	request := h.enteredRequest(t, waiting, h.storedCode(t, tutor).Display())
	if confirmed := h.confirmInOwnBrowser(t, email, waiting); outcomeOf(t, confirmed) != "request_sent" {
		t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
	}
	if got := h.storedRequest(t, request); got.Suppressed != "limit" {
		t.Fatalf("the request is %+v instead of suppressed for the limit", got)
	}
	return request
}

func TestRegeneratedCodeStopsWorkingAtOnce(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unconfirmed := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	old := h.storedCode(t, tutor).Display()
	held := h.enteredRequest(t, unconfirmed, old)

	regenerated := h.regenerateCode(t, tutorToken)
	stale := h.enterCode(t, student, old)
	signUp := h.register(t, studentCodeSignUp(requestingEmail, old))
	fresh := h.enterCode(t, student, decodeCode(t, regenerated).Code)

	if stale.status != http.StatusUnprocessableEntity || code(t, stale) != "tutor_code_invalid" {
		t.Fatalf("the old code answered %d %s", stale.status, stale.body)
	}
	if signUp.status != http.StatusUnprocessableEntity || code(t, signUp) != "tutor_code_invalid" || h.userExists(t, requestingEmail) {
		t.Fatalf("a sign-up with the old code answered %d %s", signUp.status, signUp.body)
	}
	if sentRequestOf(t, fresh).State != "pending" || h.storedRequest(t, held).State != "held" {
		t.Fatal("the new code or the request held before the regeneration does not work")
	}
}

func TestBlockedTutorsCodeIsInvalid(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	blockedCode := h.storedCode(t, tutor).Display()
	if blockedCode == unknownCode {
		t.Fatal("the seeded code collides with the unknown code")
	}
	if _, err := h.store.Repositories().Users.Block(context.Background(), tutor, authStart); err != nil {
		t.Fatal(err)
	}

	blocked := h.enterCode(t, token, blockedCode)
	unknown := h.enterCode(t, token, unknownCode)
	malformed := h.enterCode(t, token, "not a code")
	blockedSignUp := h.register(t, studentCodeSignUp(requestingEmail, blockedCode))
	unknownSignUp := h.register(t, studentCodeSignUp(requestingEmail, unknownCode))

	if blocked.status != http.StatusUnprocessableEntity || code(t, blocked) != "tutor_code_invalid" {
		t.Fatalf("the blocked tutor's code answered %d %s", blocked.status, blocked.body)
	}
	if refusal(t, blocked) != refusal(t, unknown) || refusal(t, malformed) != refusal(t, unknown) {
		t.Fatalf("a blocked tutor's code answered %s, an unknown one %s and a malformed one %s", blocked.body, unknown.body, malformed.body)
	}
	if refusal(t, blockedSignUp) != refusal(t, unknownSignUp) || code(t, blockedSignUp) != "tutor_code_invalid" {
		t.Fatalf("the sign-ups answered %s and %s", blockedSignUp.body, unknownSignUp.body)
	}
	if h.countRows(t, requestsOfStudent, student.String()) != 0 || h.userExists(t, requestingEmail) {
		t.Fatal("an invalid code created a request or an account")
	}
}

func TestTutorEnteringACodeGetsRoleMismatch(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	_, unverified := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 3, "unconfirmed-tutor@example.org")

	valid := h.enterCode(t, other, h.storedCode(t, tutor).Display())
	invalid := h.enterCode(t, unverified, unknownCode)

	for name, refused := range map[string]response{"a valid code": valid, "an unknown code from an unconfirmed tutor": invalid} {
		if refused.status != http.StatusConflict || code(t, refused) != "role_mismatch" {
			t.Fatalf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	if h.countRows(t, "SELECT count(id) FROM join_requests WHERE tutor_id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("a tutor's code entry created a request")
	}
}

func TestCodeEntryBodyIsRefusedBeforeAnyRequestIsCreated(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	entry := api.OperationAudience{Method: http.MethodPost, Path: joinRequestsPath, Audience: api.AudienceUnverifiedOK}
	schema, takesBody := loadPublishedDocument(t).bodySchema(t, entry)
	probes := operationProbes(schema)
	probes["a code that is too long"] = bodyProbe{body: codeBody(strings.Repeat(string(h.storedCode(t, tutor)), 9)), contentType: "application/json"}

	h.assertProbesAreRefusedBeforeScope(t, entry, joinRequestsPath, probes, token)

	if !takesBody || len(schema.Required) == 0 {
		t.Fatal("code entry publishes no required request body")
	}
	if h.countRows(t, requestsOfStudent, student.String()) != 0 {
		t.Fatal("a refused body created a request")
	}
}

func TestHeldRequestIsInvisibleUntilConfirmation(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	tutorCode := h.storedCode(t, tutor)
	registered := h.register(t, studentCodeSignUp(requestingEmail, strings.ToLower(string(tutorCode))))
	student := h.userID(t, requestingEmail)
	token := accessToken(t, h.login(t, requestingEmail, newcomerPassword))
	_, sentBefore := h.sentRequests(t, token)
	listedBefore := h.tutorRequests(t, tutorToken)

	confirmed := h.confirmInOwnBrowser(t, requestingEmail, token)
	tutorToken = h.relogin(t, tutorEmail)
	listedAfter := h.tutorRequests(t, tutorToken)
	_, sentAfter := h.sentRequests(t, token)

	if registered.status != http.StatusAccepted || len(sentBefore) != 1 || sentBefore[0].State != "pending" || len(listedBefore) != 0 {
		t.Fatalf("before the confirmation the student sees %+v and the tutor %+v", sentBefore, listedBefore)
	}
	if confirmed.status != http.StatusOK || outcomeOf(t, confirmed) != "request_sent" {
		t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
	}
	if len(listedAfter) != 1 || listedAfter[0].ID != sentBefore[0].ID || listedAfter[0].Student.Name != requestingName ||
		listedAfter[0].Student.Email != requestingEmail || !listedAfter[0].ExpiresAt.Equal(authStart.Add(domain.JoinRequestLifetime)) {
		t.Fatalf("after the confirmation the tutor sees %+v", listedAfter)
	}
	if len(sentAfter) != 1 || sentAfter[0].State != "pending" || h.storedRequest(t, requestIDOf(t, sentAfter[0])).State != "pending" {
		t.Fatalf("after the confirmation the student sees %+v", sentAfter)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, `"visibility":"held"`) || !strings.Contains(logs, requestReleased) || !strings.Contains(logs, `"user_id":"`+student.String()+`"`) {
		t.Fatal("no join_request.created record for the held request or no join_request.released record")
	}
	for _, private := range []string{string(tutorCode), tutorCode.Display(), requestingEmail, requestingName} {
		if strings.Contains(logs, private) {
			t.Fatalf("the logs contain %q", private)
		}
	}
}

func TestHeldRequestReleasedAtConfirmationRespectsBlockAndLimit(t *testing.T) {
	h := newFlowHarness(t)
	visibleTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blockingTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	fullTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "full-tutor@example.org")
	closedTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 4, "closed-tutor@example.org")
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 5, newcomerEmail)
	requests := map[string]domain.JoinRequestID{
		"visible": h.enteredRequest(t, token, h.storedCode(t, visibleTutor).Display()),
		"blocked": h.enteredRequest(t, token, h.storedCode(t, blockingTutor).Display()),
		"limit":   h.enteredRequest(t, token, h.storedCode(t, fullTutor).Display()),
		"closed":  h.enteredRequest(t, token, h.storedCode(t, closedTutor).Display()),
	}
	h.blockRequester(t, blockingTutor, student)
	h.fillPendingRequests(t, fullTutor)
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), closedTutor, authStart); err != nil {
		t.Fatal(err)
	}

	confirmed := h.confirmInOwnBrowser(t, newcomerEmail, token)

	if confirmed.status != http.StatusOK || outcomeOf(t, confirmed) != "request_sent" {
		t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
	}
	want := map[string]storedRequest{
		"visible": {State: "pending", Suppressed: "none"},
		"blocked": {State: "pending", Suppressed: "blocked"},
		"limit":   {State: "pending", Suppressed: "limit"},
		"closed":  {State: "expired", Suppressed: "none"},
	}
	for name, id := range requests {
		if got := h.storedRequest(t, id); got != want[name] {
			t.Errorf("the %s request is %+v", name, got)
		}
	}
	if h.visibleCount(t, fullTutor) != domain.VisiblePendingRequestCap {
		t.Fatal("the release exceeded the limit of visible requests")
	}
}

func TestSuppressedRequestBecomesVisibleWhenASlotFrees(t *testing.T) {
	frees := map[string]func(t *testing.T, h *authHarness, tutor domain.UserID){
		"a decline": func(t *testing.T, h *authHarness, tutor domain.UserID) {
			t.Helper()
			declined := h.declineRequest(t, h.relogin(t, tutorEmail), h.oldestFiller(t, tutor, "filler-"), false)
			if declined.status != http.StatusOK {
				t.Fatalf("the decline answered %d %s", declined.status, declined.body)
			}
		},
		"an accept": func(t *testing.T, h *authHarness, tutor domain.UserID) {
			t.Helper()
			accepted := h.acceptRequest(t, h.relogin(t, tutorEmail), h.oldestFiller(t, tutor, "filler-"))
			if accepted.status != http.StatusOK || decodeCard(t, accepted).State != "active" {
				t.Fatalf("the accept answered %d %s", accepted.status, accepted.body)
			}
		},
		"an expiry": func(t *testing.T, h *authHarness, _ domain.UserID) {
			t.Helper()
			h.at(t, authStart.Add(domain.JoinRequestLifetime))
			h.tick(t)
		},
	}
	for name, free := range frees {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.fillPendingRequests(t, tutor)
			h.advance(t, time.Hour)
			waiting := h.suppressedForLimit(t, tutor, 2, newcomerEmail)

			free(t, h, tutor)

			if got := h.storedRequest(t, waiting); got.State != "pending" || got.Suppressed != "none" {
				t.Fatalf("after %s the suppressed request is %+v", name, got)
			}
			if h.visibleCount(t, tutor) > domain.VisiblePendingRequestCap || !strings.Contains(h.logs.String(), requestFreed) {
				t.Fatal("the un-suppression exceeded the limit or was not recorded")
			}
			listed := h.tutorRequests(t, h.relogin(t, tutorEmail))
			if listed[len(listed)-1].ID != requestText(waiting) {
				t.Fatalf("the tutor's list does not end with the un-suppressed request: %+v", listed[len(listed)-1])
			}
		})
	}
}

func sentByID(t *testing.T, sent []sentRequest, id domain.JoinRequestID) sentRequest {
	t.Helper()
	for _, request := range sent {
		if request.ID == requestText(id) {
			return request
		}
	}
	t.Fatalf("the student's list has no request %s", requestText(id))
	return sentRequest{}
}

func sameLifecycle(first sentRequest, second sentRequest) bool {
	sameDecision := (first.DecidedAt == nil && second.DecidedAt == nil) ||
		(first.DecidedAt != nil && second.DecidedAt != nil && first.DecidedAt.Equal(*second.DecidedAt))
	return first.State == second.State && first.CreatedAt.Equal(second.CreatedAt) && sameDecision
}

func TestUnblockLeavesBlockedRequestsToExpireLikeAnyOther(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	h.setAccount(t, student, requestingName, domain.LanguageEnglish)
	tutorCode := h.storedCode(t, tutor).Display()
	first := h.enteredRequest(t, token, tutorCode)
	h.declineRequest(t, tutorToken, requestText(first), true)
	h.advance(t, time.Minute)
	suppressed := h.enteredRequest(t, token, tutorCode)
	ordinary := h.enteredRequest(t, token, h.storedCode(t, otherTutor).Display())
	blockedBefore := h.blockedStudents(t, tutorToken)
	_, sentBefore := h.sentRequests(t, token)
	expiry := authStart.Add(time.Minute + domain.JoinRequestLifetime)

	unblocked := h.unblock(t, tutorToken, student)
	repeated := h.unblock(t, tutorToken, student)
	_, sentAfterUnblock := h.sentRequests(t, token)
	random := h.acceptRequest(t, tutorToken, randomCardID)
	hidden := map[string]response{
		"accepting": h.acceptRequest(t, tutorToken, requestText(suppressed)),
		"declining": h.declineRequest(t, tutorToken, requestText(suppressed), false),
	}
	listed := h.tutorRequests(t, tutorToken)
	h.at(t, expiry)
	_, sentAtExpiry := h.sentRequests(t, accessToken(t, h.login(t, studentEmail, accountPassword)))
	h.tick(t)
	_, sentAfterTick := h.sentRequests(t, accessToken(t, h.login(t, studentEmail, accountPassword)))

	if len(blockedBefore) != 1 || blockedBefore[0].StudentID != student.String() || blockedBefore[0].Name != requestingName {
		t.Fatalf("the blocked students are %+v", blockedBefore)
	}
	if unblocked.status != http.StatusNoContent || repeated.status != http.StatusNotFound {
		t.Fatalf("unblocking answered %d %s and again %d %s", unblocked.status, unblocked.body, repeated.status, repeated.body)
	}
	moments := map[string][]sentRequest{
		"before the unblock": sentBefore, "after the unblock": sentAfterUnblock, "at the expiry before the job": sentAtExpiry, "after the job": sentAfterTick,
	}
	for when, sent := range moments {
		if got, want := sentByID(t, sent, suppressed), sentByID(t, sent, ordinary); !sameLifecycle(got, want) {
			t.Errorf("%s the blocked request shows %+v and an ordinary one %+v", when, got, want)
		}
	}
	atExpiry := sentByID(t, sentAtExpiry, suppressed)
	if sentByID(t, sentAfterUnblock, suppressed).State != "pending" || atExpiry.State != "expired" || atExpiry.DecidedAt == nil || !atExpiry.DecidedAt.Equal(expiry) {
		t.Fatalf("the blocked request did not show pending until its own expiry: %+v", atExpiry)
	}
	for action, answer := range hidden {
		if answer.status != http.StatusNotFound || refusal(t, answer) != refusal(t, random) {
			t.Errorf("%s the request the unblock closed answered %d %s", action, answer.status, answer.body)
		}
	}
	if len(listed) != 0 || len(h.blockedStudents(t, h.relogin(t, tutorEmail))) != 0 {
		t.Fatalf("after unblocking the tutor sees %+v", listed)
	}
	if !strings.Contains(h.logs.String(), `"msg":"blocked_requester.removed"`) || !strings.Contains(h.logs.String(), `"decision":"blocked"`) {
		t.Fatal("no join_request.decided record for the block or no blocked_requester.removed record")
	}
}

func TestReentryAfterUnblockReopensTheClosedRequest(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	tutorCode, otherCode := h.storedCode(t, tutor).Display(), h.storedCode(t, otherTutor).Display()
	h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, tutorCode)), true)
	h.advance(t, time.Minute)
	blockedEntry := h.enterCode(t, token, tutorCode)
	ordinaryEntry := h.enterCode(t, token, otherCode)
	h.unblock(t, tutorToken, student)
	_, sentBefore := h.sentRequests(t, token)
	h.advance(t, time.Minute)

	reentered := h.enterCode(t, token, tutorCode)
	ordinaryReentry := h.enterCode(t, token, otherCode)
	_, sentAfter := h.sentRequests(t, token)
	listed := h.tutorRequests(t, tutorToken)

	if reentered.status != http.StatusOK || reentered.body != blockedEntry.body || ordinaryReentry.status != http.StatusOK || ordinaryReentry.body != ordinaryEntry.body {
		t.Fatalf("the re-entries answered %d %s and %d %s", reentered.status, reentered.body, ordinaryReentry.status, ordinaryReentry.body)
	}
	if got, want := comparableHeaders(reentered.headers), comparableHeaders(ordinaryReentry.headers); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the re-entries' headers differ: %v and %v", got, want)
	}
	reopened := requestIDOf(t, sentRequestOf(t, reentered))
	if len(sentAfter) != len(sentBefore) || !sameLifecycle(sentByID(t, sentAfter, reopened), sentByID(t, sentBefore, reopened)) {
		t.Fatalf("the student's list changed from %+v to %+v", sentBefore, sentAfter)
	}
	if got := h.storedRequest(t, reopened); got != (storedRequest{State: "pending", Suppressed: "none"}) {
		t.Fatalf("the reopened request is %+v", got)
	}
	if len(listed) != 1 || listed[0].ID != requestText(reopened) || !listed[0].CreatedAt.Equal(authStart.Add(time.Minute)) {
		t.Fatalf("the tutor sees %+v", listed)
	}
	if !strings.Contains(h.logs.String(), `"msg":"join_request.reopened"`) || !strings.Contains(h.logs.String(), `"join_request_id":"`+requestText(reopened)+`","visibility":"visible"`) {
		t.Fatal("no join_request.reopened record")
	}
}

func TestReentryAfterUnblockIntoAFullTutorIsSuppressedNotRefused(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, tutorCode)), true)
	suppressed := h.enteredRequest(t, token, tutorCode)
	h.unblock(t, tutorToken, student)
	h.fillPendingRequests(t, tutor)

	reentered := h.enterCode(t, token, tutorCode)

	if reentered.status != http.StatusOK || sentRequestOf(t, reentered).ID != requestText(suppressed) {
		t.Fatalf("the re-entry into a full tutor answered %d %s", reentered.status, reentered.body)
	}
	if got := h.storedRequest(t, suppressed); got != (storedRequest{State: "pending", Suppressed: "limit"}) {
		t.Fatalf("the reopened request is %+v", got)
	}
}

func TestCodeEntryAfterTheRequestIsOverdueStartsANewOne(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	overdue := h.enteredRequest(t, token, tutorCode)
	h.at(t, authStart.Add(domain.JoinRequestLifetime))
	token = accessToken(t, h.login(t, studentEmail, accountPassword))

	entered := h.enterCode(t, token, tutorCode)
	_, sent := h.sentRequests(t, token)

	renewed := sentRequestOf(t, entered)
	if renewed.ID == requestText(overdue) || renewed.State != "pending" || h.storedRequest(t, requestIDOf(t, renewed)).State != "pending" {
		t.Fatalf("an entry after the old request was due answered %s", entered.body)
	}
	old := sentByID(t, sent, overdue)
	if old.State != "expired" || old.DecidedAt == nil || !old.DecidedAt.Equal(authStart.Add(domain.JoinRequestLifetime)) {
		t.Fatalf("the overdue request shows %+v", old)
	}
	if listed := h.tutorRequests(t, h.relogin(t, tutorEmail)); len(listed) != 1 || listed[0].ID != renewed.ID {
		t.Fatalf("the tutor sees %+v", listed)
	}
}

type hiddenRequest func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string) domain.JoinRequestID

var hiddenRequests = map[string]hiddenRequest{
	"a held request": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) domain.JoinRequestID {
		t.Helper()
		_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, newcomerEmail)
		return h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	},
	"a request suppressed by a block": func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string) domain.JoinRequestID {
		t.Helper()
		_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
		first := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
		h.declineRequest(t, tutorToken, requestText(first), true)
		return h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	},
	"a request suppressed by the limit": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) domain.JoinRequestID {
		t.Helper()
		h.fillPendingRequests(t, tutor)
		return h.suppressedForLimit(t, tutor, 3, newcomerEmail)
	},
	"a request of an operator-blocked student": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) domain.JoinRequestID {
		t.Helper()
		student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
		request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
		if _, err := h.store.Repositories().Users.Block(context.Background(), student, authStart); err != nil {
			t.Fatal(err)
		}
		return request
	},
}

func TestOwningTutorCannotDecideHiddenRequests(t *testing.T) {
	for name, hide := range hiddenRequests {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			request := hide(t, h, tutor, tutorToken)
			tutorToken = h.relogin(t, tutorEmail)
			before := h.storedRequest(t, request)
			blocks := h.countRows(t, "SELECT count(student_id) FROM blocked_requesters WHERE tutor_id = $1::uuid", tutor.String())
			random := h.acceptRequest(t, tutorToken, randomCardID)

			answers := map[string]response{
				"accepting":              h.acceptRequest(t, tutorToken, requestText(request)),
				"declining":              h.declineRequest(t, tutorToken, requestText(request), false),
				"declining with a block": h.declineRequest(t, tutorToken, requestText(request), true),
			}
			listed := h.tutorRequests(t, tutorToken)

			for action, answer := range answers {
				if answer.status != http.StatusNotFound || refusal(t, answer) != refusal(t, random) {
					t.Errorf("%s answered %d %s", action, answer.status, answer.body)
				}
			}
			if h.storedRequest(t, request) != before ||
				h.countRows(t, "SELECT count(student_id) FROM blocked_requesters WHERE tutor_id = $1::uuid", tutor.String()) != blocks {
				t.Fatal("a refused decision changed the request or the blocks")
			}
			for _, item := range listed {
				if item.ID == requestText(request) {
					t.Fatal("the tutor's list shows the hidden request")
				}
			}
		})
	}
}

func TestRequestsOfAnOperatorBlockedStudentReturnAfterTheUnblock(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	if _, err := h.store.Repositories().Users.Block(context.Background(), student, authStart); err != nil {
		t.Fatal(err)
	}
	whileBlocked := h.tutorRequests(t, tutorToken)

	if _, err := h.store.Repositories().Users.Unblock(context.Background(), student, authStart); err != nil {
		t.Fatal(err)
	}
	afterUnblock := h.tutorRequests(t, tutorToken)
	accepted := h.acceptRequest(t, tutorToken, requestText(request))

	if len(whileBlocked) != 0 || len(afterUnblock) != 1 || afterUnblock[0].ID != requestText(request) {
		t.Fatalf("the tutor sees %+v while blocked and %+v after the unblock", whileBlocked, afterUnblock)
	}
	if accepted.status != http.StatusOK || decodeCard(t, accepted).State != "active" {
		t.Fatalf("accepting after the unblock answered %d %s", accepted.status, accepted.body)
	}
}

func TestUnblockRacingACodeEntryHasOneOutcome(t *testing.T) {
	for _, unblockFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("unblock first %t", unblockFirst), func(t *testing.T) {
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
			tutorCode := h.storedCode(t, tutor).Display()
			h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, tutorCode)), true)
			suppressed := h.enteredRequest(t, token, tutorCode)
			unblock := func() <-chan pendingAnswer {
				return h.sendInBackground(http.MethodDelete, blockedPath+"/"+student.String(), "", jsonCall(tutorToken))
			}
			release := h.holdTutorProfile(t, tutor)

			var unblocked, entry <-chan pendingAnswer
			if unblockFirst {
				unblocked = unblock()
				h.waitForLockWaiters(t, 1)
				entry = h.tryEnterCode(token, tutorCode)
			} else {
				entry = h.tryEnterCode(token, tutorCode)
				h.waitForLockWaiters(t, 1)
				unblocked = unblock()
			}
			h.waitForLockWaiters(t, 2)
			release()
			unblockAnswer, entered := awaitAnswer(t, unblocked), awaitAnswer(t, entry)

			if unblockAnswer.status != http.StatusNoContent || entered.status != http.StatusOK {
				t.Fatalf("the unblock answered %d %s and the entry %d %s", unblockAnswer.status, unblockAnswer.body, entered.status, entered.body)
			}
			answered := sentRequestOf(t, entered)
			listed := h.tutorRequests(t, tutorToken)
			if answered.ID != requestText(suppressed) || answered.State != "pending" {
				t.Fatalf("the entry answered %s", entered.body)
			}
			if unblockFirst {
				if len(listed) != 1 || listed[0].ID != answered.ID {
					t.Fatalf("after the unblock the reopened request is not visible: %+v", listed)
				}
			} else if len(listed) != 0 {
				t.Fatalf("before the unblock the tutor sees %+v", listed)
			}
			if h.countRows(t, "SELECT count(id) FROM join_requests WHERE student_id = $1::uuid AND state IN ('held', 'pending')", student.String()) > 1 {
				t.Fatal("the race left two open requests")
			}
		})
	}
}

func TestStudentCodeSignUpAnswersIdenticallyForNewAndExistingEmail(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.seedUserAs(t, domain.RoleStudent, 2, studentEmail, "correct horse battery", currentHash)
	tutorCode := h.storedCode(t, tutor).Display()

	before := h.hasher.Hashes()
	fresh := h.register(t, studentCodeSignUp(requestingEmail, tutorCode))
	afterFresh := h.hasher.Hashes()
	taken := h.register(t, studentCodeSignUp(studentEmail, tutorCode))
	afterTaken := h.hasher.Hashes()
	h.tick(t)

	if fresh.status != http.StatusAccepted || taken.status != fresh.status || taken.body != fresh.body {
		t.Fatalf("new %d %q, existing %d %q", fresh.status, fresh.body, taken.status, taken.body)
	}
	if got, want := comparableHeaders(taken.headers), comparableHeaders(fresh.headers); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("headers differ: %v and %v", want, got)
	}
	if afterFresh-before != 1 || afterTaken-afterFresh != 1 {
		t.Fatalf("the sign-ups hashed %d and %d times", afterFresh-before, afterTaken-afterFresh)
	}
	created := h.userID(t, requestingEmail)
	if h.countRows(t, requestsOfStudent, created.String()) != 1 || h.countRows(t, requestsOfStudent, domain.UserID{2}.String()) != 0 {
		t.Fatal("the new account has no held request or the existing account got one")
	}
	if got := subjects(h.mailTo(t, studentEmail)); len(got) != 1 || got[0] != duplicateSubject {
		t.Fatalf("the existing account received %v", got)
	}
}

func TestStudentSignUpWithAnInvalidCodeCreatesNothing(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	regenerated := h.storedCode(t, tutor).Display()
	h.regenerateCode(t, tutorToken)
	missing := studentCodeSignUp(requestingEmail, "")
	shortPassword := studentCodeSignUp(requestingEmail, unknownCode)
	shortPassword.Password = "x"
	before := h.hasher.Hashes()

	answers := map[string]response{
		"an unknown code":     h.register(t, studentCodeSignUp(requestingEmail, unknownCode)),
		"a regenerated code":  h.register(t, studentCodeSignUp(requestingEmail, regenerated)),
		"a malformed code":    h.register(t, studentCodeSignUp(requestingEmail, "0000-0000")),
		"an invalid password": h.register(t, shortPassword),
	}
	withoutCode := h.register(t, missing)
	h.tick(t)

	for name, answer := range answers {
		if answer.status != http.StatusUnprocessableEntity || code(t, answer) != "tutor_code_invalid" {
			t.Errorf("%s answered %d %s", name, answer.status, answer.body)
		}
	}
	if withoutCode.status != http.StatusUnprocessableEntity || strings.Join(fields(t, withoutCode), ",") != "body.tutor_code:required" {
		t.Fatalf("a sign-up without a code answered %d %s", withoutCode.status, withoutCode.body)
	}
	if h.userExists(t, requestingEmail) || h.hasher.Hashes() != before || len(h.mailTo(t, requestingEmail)) != 0 {
		t.Fatal("an invalid code created an account, hashed a password or sent an email")
	}
}

func TestCodeSignUpReservesThePerAddressSlotFirst(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for attempt := range 10 {
		if refused := h.register(t, studentCodeSignUp(fmt.Sprintf("guess%d@example.org", attempt), unknownCode)); code(t, refused) != "tutor_code_invalid" {
			t.Fatalf("guess %d answered %d %s", attempt, refused.status, refused.body)
		}
	}
	before := h.hasher.Hashes()

	valid := h.register(t, studentCodeSignUp(requestingEmail, h.storedCode(t, tutor).Display()))
	invalid := h.register(t, studentCodeSignUp(requestingEmail, unknownCode))

	for name, refused := range map[string]response{"a valid code": valid, "an invalid code": invalid} {
		if refused.status != http.StatusTooManyRequests || code(t, refused) != "rate_limited" || refused.headers.Get("Retry-After") != "3600" {
			t.Fatalf("%s after ten guesses answered %d %s", name, refused.status, refused.body)
		}
	}
	if h.userExists(t, requestingEmail) || h.hasher.Hashes() != before {
		t.Fatal("a throttled sign-up created an account or hashed a password")
	}
}

func TestJoinRequestThrottleRefusesTheEleventhCodeInAnHour(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	h.enterCode(t, token, tutorCode)
	for attempt := range 9 {
		if guessed := h.enterCode(t, token, unknownCode); code(t, guessed) != "tutor_code_invalid" {
			t.Fatalf("guess %d answered %d %s", attempt, guessed.status, guessed.body)
		}
	}

	eleventh := h.enterCode(t, token, tutorCode)
	h.advance(t, time.Hour)
	nextHour := h.enterCode(t, accessToken(t, h.login(t, newcomerEmail, accountPassword)), tutorCode)

	if eleventh.status != http.StatusTooManyRequests || code(t, eleventh) != "rate_limited" || eleventh.headers.Get("Retry-After") != "3600" {
		t.Fatalf("the eleventh entry answered %d %v %s", eleventh.status, eleventh.headers, eleventh.body)
	}
	if nextHour.status != http.StatusOK || h.countRows(t, requestsOfStudent, student.String()) != 1 {
		t.Fatalf("the entry of the next hour answered %d %s", nextHour.status, nextHour.body)
	}
	if !strings.Contains(h.logs.String(), requestThrottled) {
		t.Fatal("no join_request.throttled record")
	}
}

func TestTutorPendingRequestLimitAnswersTutorUnavailable(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillPendingRequestsWith(t, tutor, "filler-", domain.VisiblePendingRequestCap-5)
	const entrants = 10
	tokens := make([]string, 0, entrants)
	for index := range entrants {
		_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(10+index), fmt.Sprintf("entrant%d@example.org", index))
		tokens = append(tokens, token)
	}
	tutorCode := h.storedCode(t, tutor).Display()

	answers := make([]<-chan pendingAnswer, 0, entrants)
	for _, token := range tokens {
		answers = append(answers, h.tryEnterCode(token, tutorCode))
	}
	statuses := map[int]int{}
	for _, answer := range answers {
		result := awaitAnswer(t, answer)
		statuses[result.status]++
		if result.status == http.StatusTooManyRequests && (code(t, result) != "tutor_unavailable" || result.headers.Get("Retry-After") != "3600") {
			t.Fatalf("a refused entry answered %v %s", result.headers, result.body)
		}
	}

	if statuses[http.StatusOK] != 5 || statuses[http.StatusTooManyRequests] != 5 || len(statuses) != 2 {
		t.Fatalf("the concurrent entries answered %v", statuses)
	}
	if h.visibleCount(t, tutor) != domain.VisiblePendingRequestCap {
		t.Fatalf("the tutor has %d visible requests", h.visibleCount(t, tutor))
	}
}

func TestUnconfirmedCodeEntryNeverAnswersTutorUnavailable(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillPendingRequests(t, tutor)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)

	entered := h.enterCode(t, token, h.storedCode(t, tutor).Display())

	request := sentRequestOf(t, entered)
	if request.State != "pending" || h.storedRequest(t, requestIDOf(t, request)).State != "held" || h.visibleCount(t, tutor) != domain.VisiblePendingRequestCap {
		t.Fatalf("the unconfirmed entry answered %s", entered.body)
	}
}

func TestDeclineAndBlockKeepsLaterRequestsPendingForTheStudent(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	first := h.enteredRequest(t, token, tutorCode)

	declined := h.declineRequest(t, tutorToken, requestText(first), true)
	repeated := h.declineRequest(t, tutorToken, requestText(first), true)
	h.advance(t, time.Minute)
	later := h.enterCode(t, token, tutorCode)
	_, sentWhileBlocked := h.sentRequests(t, token)
	listed := h.tutorRequests(t, tutorToken)
	h.at(t, authStart.Add(time.Minute+domain.JoinRequestLifetime))
	h.tick(t)
	_, sentAfterExpiry := h.sentRequests(t, accessToken(t, h.login(t, studentEmail, accountPassword)))

	if declined.status != http.StatusOK || strings.TrimSpace(declined.body) != fmt.Sprintf(`{"id":%q,"state":"declined"}`, requestText(first)) || repeated.body != declined.body {
		t.Fatalf("declining answered %d %s and again %s", declined.status, declined.body, repeated.body)
	}
	if later.status != http.StatusOK || sentRequestOf(t, later).State != "pending" || len(listed) != 0 {
		t.Fatalf("a later request answered %s and the tutor sees %+v", later.body, listed)
	}
	if len(sentWhileBlocked) != 2 || sentWhileBlocked[0].State != "pending" || sentWhileBlocked[1].State != "not_accepted" {
		t.Fatalf("while blocked the student sees %+v", sentWhileBlocked)
	}
	if got := h.storedRequest(t, requestIDOf(t, sentRequestOf(t, later))); got.State != "expired" {
		t.Fatalf("after 14 days the suppressed request is %+v", got)
	}
	if len(sentAfterExpiry) != 2 || sentAfterExpiry[0].State != "expired" || sentAfterExpiry[0].DecidedAt == nil {
		t.Fatalf("after 14 days the student sees %+v", sentAfterExpiry)
	}
	if strings.Count(h.logs.String(), requestDecided) != 1 {
		t.Fatal("the repeated decline recorded another decision")
	}
}

func TestAcceptLinksTheInvitedCardWithTheConfirmedEmail(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	card := h.newCard(t, tutorToken, cardName, strings.ToUpper(studentEmail))
	h.withNotes(t, tutorToken, card.ID, cardNotes)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())

	accepted := h.acceptRequest(t, tutorToken, requestText(request))
	repeated := h.acceptRequest(t, tutorToken, requestText(request))
	_, tutors := h.myTutors(t, token)

	linked := decodeCard(t, accepted)
	if accepted.status != http.StatusOK || linked.ID != card.ID || linked.State != "active" || linked.DisplayName != cardName ||
		linked.Notes == nil || *linked.Notes != cardNotes || linked.Account == nil || linked.Account.Email != studentEmail {
		t.Fatalf("accepting answered %d %s", accepted.status, accepted.body)
	}
	if repeated.status != http.StatusOK || decodeCard(t, repeated).ID != card.ID || h.storedRequest(t, request).State != "accepted" {
		t.Fatalf("accepting again answered %d %s", repeated.status, repeated.body)
	}
	if len(h.cardsOf(t, tutor, domain.RelationshipActive)) != 1 || h.linksOf(t, student) != 1 || len(tutors) != 1 || tutors[0].RelationshipID != card.ID {
		t.Fatalf("the student lists %+v", tutors)
	}
	if strings.Count(h.logs.String(), `"decision":"accepted"`) != 1 || !strings.Contains(h.logs.String(), `"effect":"linked"`) {
		t.Fatal("no single join_request.decided record for the accept")
	}
}

func TestReactivationByJoinRequest(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	ended := h.seedCard(t, tutor, 1, domain.RelationshipEnded, &student)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())

	accepted := h.acceptRequest(t, tutorToken, requestText(request))
	_, tutors := h.myTutors(t, token)

	card := decodeCard(t, accepted)
	if accepted.status != http.StatusOK || card.ID != ended || card.State != "active" || h.linksOf(t, student) != 1 {
		t.Fatalf("accepting answered %d %s", accepted.status, accepted.body)
	}
	if len(tutors) != 1 || tutors[0].RelationshipID != ended || tutors[0].State != "active" || tutors[0].EndedAt != nil || tutors[0].EndedBy != nil {
		t.Fatalf("the student lists %+v", tutors)
	}
	if !strings.Contains(h.logs.String(), relationshipReactivated) || !strings.Contains(h.logs.String(), `"effect":"reactivated"`) {
		t.Fatal("no relationship.reactivated record")
	}
}

func TestAcceptedRequestRevokesTheCardsOpenLink(t *testing.T) {
	cards := map[string]func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string, student domain.UserID) (string, string){
		"an invited card": func(t *testing.T, h *authHarness, _ domain.UserID, tutorToken string, _ domain.UserID) (string, string) {
			t.Helper()
			card := h.newCard(t, tutorToken, cardName, studentEmail)
			return card.ID, linkToken(t, card.Invitation)
		},
		"an ended card": func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string, student domain.UserID) (string, string) {
			t.Helper()
			card := h.seedCard(t, tutor, 1, domain.RelationshipEnded, &student)
			regenerated := h.cardCall(t, tutorToken, http.MethodPost, card, "/invitation", "")
			var issued struct {
				Invitation cardInvitation `json:"invitation"`
			}
			if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
				t.Fatalf("regenerating the ended card's link answered %d %s", regenerated.status, regenerated.body)
			}
			return card, linkToken(t, &issued.Invitation)
		},
	}
	for name, prepare := range cards {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
			card, link := prepare(t, h, tutor, tutorToken, student)
			request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())

			accepted := h.acceptRequest(t, tutorToken, requestText(request))
			page := h.invitationPage(t, link)
			random := h.invitationPage(t, randomInvitationToken())
			reused := h.acceptSignedIn(t, token, link)

			if accepted.status != http.StatusOK || decodeCard(t, accepted).ID != card || decodeCard(t, accepted).Invitation != nil {
				t.Fatalf("accepting answered %d %s", accepted.status, accepted.body)
			}
			if page.status != http.StatusNotFound || refusal(t, page) != refusal(t, random) {
				t.Fatalf("the old link answered %d %s", page.status, page.body)
			}
			if reused.status != http.StatusConflict || code(t, reused) != "invitation_invalid" {
				t.Fatalf("accepting the old link answered %d %s", reused.status, reused.body)
			}
			var reason *string
			var ciphertext []byte
			err := h.database.Pool.QueryRow(context.Background(),
				"SELECT revoke_reason, token_encrypted FROM invitations WHERE token_hash = $1", tokenHash(t, link)).Scan(&reason, &ciphertext)
			if err != nil {
				t.Fatal(err)
			}
			if reason == nil || *reason != string(domain.RevokeLinked) || ciphertext != nil {
				t.Fatalf("the old link has revoke reason %v and ciphertext %v", reason, ciphertext)
			}
		})
	}
}

func TestAcceptedRequestWithoutACardCopiesTheAccountName(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.setAccount(t, student, requestingName, domain.LanguageEnglish)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())

	accepted := h.acceptRequest(t, tutorToken, requestText(request))
	card := decodeCard(t, accepted)
	renamed := h.patchMe(t, token, `{"name":"A later account name"}`)
	afterRename := decodeCard(t, h.cardCall(t, tutorToken, http.MethodGet, card.ID, "", ""))
	edited := h.cardCall(t, tutorToken, http.MethodPatch, card.ID, "", `{"display_name":"Anna from the club"}`)

	if accepted.status != http.StatusOK || card.State != "active" || card.DisplayName != requestingName || card.Email != nil || card.Account == nil {
		t.Fatalf("accepting answered %d %s", accepted.status, accepted.body)
	}
	if renamed.status != http.StatusOK || afterRename.DisplayName != requestingName || afterRename.Account.Name != "A later account name" {
		t.Fatalf("after the account was renamed the card is %+v", afterRename)
	}
	if edited.status != http.StatusOK || decodeCard(t, edited).DisplayName != "Anna from the club" {
		t.Fatalf("editing the copied name answered %d %s", edited.status, edited.body)
	}
	if stored := h.storedRelationship(t, card.ID); stored.Email != nil || len(h.cardsOf(t, tutor, domain.RelationshipActive)) != 1 {
		t.Fatalf("the new card is %+v", stored)
	}
	if !strings.Contains(h.logs.String(), cardCreated) || !strings.Contains(h.logs.String(), `"effect":"created"`) {
		t.Fatal("no card.created record for the card the accept created")
	}
}

func TestJoinRequestsExpireAfter14Days(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())

	h.at(t, authStart.Add(domain.JoinRequestLifetime-time.Hour))
	h.tick(t)
	beforeExpiry := h.storedRequest(t, request)
	h.at(t, authStart.Add(domain.JoinRequestLifetime))
	h.tick(t)
	_, sent := h.sentRequests(t, accessToken(t, h.login(t, studentEmail, accountPassword)))
	listed := h.tutorRequests(t, h.relogin(t, tutorEmail))

	if beforeExpiry.State != "pending" {
		t.Fatalf("an hour before the expiry the request is %+v", beforeExpiry)
	}
	if len(sent) != 1 || sent[0].State != "expired" || sent[0].DecidedAt == nil || !sent[0].DecidedAt.Equal(authStart.Add(domain.JoinRequestLifetime)) {
		t.Fatalf("after 14 days the student sees %+v", sent)
	}
	if len(listed) != 0 || !strings.Contains(h.logs.String(), `"job":"join_requests.expire","result":"ok","batch_size":1`) {
		t.Fatalf("after 14 days the tutor sees %+v", listed)
	}
	if renewed := h.enterCode(t, accessToken(t, h.login(t, studentEmail, accountPassword)), h.storedCode(t, tutor).Display()); renewed.status != http.StatusOK {
		t.Fatalf("a new request after the expiry answered %d %s", renewed.status, renewed.body)
	}
}

func anotherTutorsJoinRequest(suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		_, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
		_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
		request := h.enteredRequest(t, student, h.storedCode(t, owner).Display())
		outOfScope := h.send(t, http.MethodPost, joinRequestsPath+"/"+requestText(request)+suffix, body, jsonCall(intruder))
		random := h.send(t, http.MethodPost, joinRequestsPath+"/"+randomCardID+suffix, body, jsonCall(intruder))
		if got := h.storedRequest(t, request); got.State != "pending" || got.Suppressed != "none" {
			t.Fatalf("another tutor decided the request: %+v", got)
		}
		return outOfScope, random
	}
}

func anotherTutorsBlockedStudent(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owner, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	h.blockRequester(t, owner, student)
	outOfScope := h.unblock(t, intruderToken, student)
	random := h.send(t, http.MethodDelete, blockedPath+"/"+randomCardID, "", jsonCall(intruderToken))
	if h.countRows(t, "SELECT count(student_id) FROM blocked_requesters WHERE tutor_id = $1::uuid", owner.String()) != 1 {
		t.Fatal("another tutor unblocked the student")
	}
	return outOfScope, random
}

func TestTutorCannotDecideAnotherTutorsJoinRequest(t *testing.T) {
	h := newFlowHarness(t)
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	request := requestText(h.enteredRequest(t, token, h.storedCode(t, owner).Display()))
	random := h.acceptRequest(t, intruderToken, randomCardID)

	answers := map[string]response{
		"accepting":              h.acceptRequest(t, intruderToken, request),
		"declining":              h.declineRequest(t, intruderToken, request, false),
		"declining with a block": h.declineRequest(t, intruderToken, request, true),
	}
	listed := h.tutorRequests(t, intruderToken)

	for name, answer := range answers {
		if answer.status != http.StatusNotFound || refusal(t, answer) != refusal(t, random) {
			t.Errorf("%s another tutor's request answered %d %s", name, answer.status, answer.body)
		}
	}
	if len(listed) != 0 || len(h.tutorRequests(t, ownerToken)) != 1 {
		t.Fatalf("the other tutor sees %+v", listed)
	}
	if h.countRows(t, "SELECT count(student_id) FROM blocked_requesters WHERE student_id = $1::uuid", student.String()) != 0 ||
		h.countRows(t, "SELECT count(id) FROM relationships WHERE tutor_id = $1::uuid", intruder.String()) != 0 {
		t.Fatal("another tutor's decision changed something")
	}
}

func TestStudentSeesOnlyTheirOwnRequests(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, first := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, second := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, newcomerEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	own := h.enteredRequest(t, first, tutorCode)
	theirs := h.enteredRequest(t, second, tutorCode)

	firstList, firstSent := h.sentRequests(t, first)
	secondList, secondSent := h.sentRequests(t, second)

	if len(firstSent) != 1 || firstSent[0].ID != requestText(own) || len(secondSent) != 1 || secondSent[0].ID != requestText(theirs) {
		t.Fatalf("the students see %+v and %+v", firstSent, secondSent)
	}
	for _, listed := range []response{firstList, secondList} {
		if listed.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("the list has Cache-Control %q", listed.headers.Get("Cache-Control"))
		}
		for _, private := range []string{"tutor", "suppressed", "held", tutorEmail, studentEmail, newcomerEmail, "A language tutor"} {
			if strings.Contains(listed.body, private) {
				t.Fatalf("a student's list contains %q: %s", private, listed.body)
			}
		}
	}
}

func TestCodeEntryDuringAPendingChoiceKeepsTheRequest(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, squatter := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	request := h.enteredRequest(t, squatter, tutorCode)
	h.reset(t, h.resetToken(t, newcomerEmail))
	owner := h.ownerSignIn(t, newcomerEmail)
	pendingBefore := h.meReclaimPending(t, owner)

	kept := h.enterCode(t, owner, tutorCode)

	answered := sentRequestOf(t, kept)
	if !pendingBefore || answered.ID != requestText(request) || answered.State != "pending" {
		t.Fatalf("the entry during the choice answered %d %s", kept.status, kept.body)
	}
	if got := h.storedRequest(t, request); got != (storedRequest{State: "pending", Suppressed: "none"}) {
		t.Fatalf("the kept request is %+v", got)
	}
	if h.meReclaimPending(t, owner) || h.reclaimDeadline(t, student) != nil {
		t.Fatal("the choice stays pending after the last request was kept")
	}
	if listed := h.tutorRequests(t, h.relogin(t, tutorEmail)); len(listed) != 1 || listed[0].ID != requestText(request) {
		t.Fatalf("the tutor sees %+v", listed)
	}
	if !strings.Contains(h.logs.String(), `"kept":1,"dropped":0,"cause":"owner"`) {
		t.Fatal("no relationship.reclaim_decided record for the keep")
	}
}

func TestCodeEntryWithAReclaimPendingLinkAnswersAlreadyLinked(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	_, active := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	h.linkedSignedIn(t, tutorToken, active, "An active student", studentEmail)
	tutorCode := h.storedCode(t, tutor).Display()

	pending := h.enterCode(t, owner, tutorCode)
	linked := h.enterCode(t, active, tutorCode)

	for name, refused := range map[string]response{"a reclaim-pending link": pending, "an active link": linked} {
		if refused.status != http.StatusConflict || code(t, refused) != "already_linked" {
			t.Fatalf("an entry with %s answered %d %s", name, refused.status, refused.body)
		}
	}
	if h.storedRelationship(t, card.ID).State != "reclaim_pending" || !h.meReclaimPending(t, owner) {
		t.Fatal("the refused entry changed the reclaim-pending link")
	}
	if h.countRows(t, requestsOfStudent, student.String()) != 0 {
		t.Fatal("the refused entry created a request")
	}
}

type lockOrderRace struct {
	contested func(t *testing.T, h *authHarness) domain.UserID
	free      func(t *testing.T, h *authHarness) <-chan pendingAnswer
	waiters   int64
	entryWant int
	check     func(t *testing.T, h *authHarness, freed response)
}

func (h *authHarness) racingEntrant(t *testing.T) string {
	t.Helper()
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 99, entrantEmail)
	return token
}

func (h *authHarness) tickAnswer(t *testing.T) <-chan pendingAnswer {
	t.Helper()
	done := h.tickInBackground(t)
	answer := make(chan pendingAnswer, 1)
	go func() {
		err := <-done
		answer <- pendingAnswer{result: response{status: http.StatusOK}, err: err}
	}()
	return answer
}

func lockOrderRaces() map[string]lockOrderRace {
	var secondLink string
	return map[string]lockOrderRace{
		"a decline that un-suppresses the next request": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				h.fillPendingRequests(t, tutor)
				h.suppressedForLimit(t, tutor, 2, newcomerEmail)
				return tutor
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				tutor := h.userID(t, tutorEmail)
				return h.sendInBackground(http.MethodPost, joinRequestsPath+"/"+h.oldestFiller(t, tutor, "filler-")+"/decline", `{}`,
					jsonCall(h.relogin(t, tutorEmail)))
			},
			waiters: 1, entryWant: http.StatusTooManyRequests,
			check: func(t *testing.T, h *authHarness, _ response) {
				t.Helper()
				if !strings.Contains(h.logs.String(), requestFreed) {
					t.Fatal("the decline did not un-suppress the waiting request")
				}
			},
		},
		"an accept": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				h.fillPendingRequests(t, tutor)
				return tutor
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				tutor := h.userID(t, tutorEmail)
				return h.sendInBackground(http.MethodPost, joinRequestsPath+"/"+h.oldestFiller(t, tutor, "filler-")+"/accept", "",
					jsonCall(h.relogin(t, tutorEmail)))
			},
			waiters: 1, entryWant: http.StatusOK,
		},
		"an expiry": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				h.fillPendingRequests(t, tutor)
				h.at(t, authStart.Add(domain.JoinRequestLifetime))
				return tutor
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				return h.tickAnswer(t)
			},
			waiters: 1, entryWant: http.StatusOK,
		},
		"a student deletion that touches two tutors": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				first, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				second, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
				_, leaving := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
				h.fillPendingRequestsWith(t, first, "first-filler-", domain.VisiblePendingRequestCap-1)
				h.fillPendingRequestsWith(t, second, "second-filler-", domain.VisiblePendingRequestCap-1)
				h.enterCode(t, leaving, h.storedCode(t, first).Display())
				h.enterCode(t, leaving, h.storedCode(t, second).Display())
				return second
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				token := accessToken(t, h.login(t, studentEmail, accountPassword))
				if h.countRows(t, requestsOfStudent, domain.UserID{3}.String()) != 2 {
					t.Fatal("the leaving student has not two requests")
				}
				return h.sendInBackground(http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, accountPassword), jsonCall(token))
			},
			waiters: 1, entryWant: http.StatusOK,
			check: func(t *testing.T, h *authHarness, freed response) {
				t.Helper()
				first := h.userID(t, tutorEmail)
				if freed.status != http.StatusNoContent || h.visibleCount(t, first) != domain.VisiblePendingRequestCap-1 {
					t.Fatalf("the deletion answered %d %s", freed.status, freed.body)
				}
			},
		},
		"a release at confirmation that touches two tutors": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				first, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				second, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
				_, confirming := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, newcomerEmail)
				h.fillPendingRequestsWith(t, first, "first-filler-", domain.VisiblePendingRequestCap-1)
				h.fillPendingRequestsWith(t, second, "second-filler-", domain.VisiblePendingRequestCap-1)
				h.enterCode(t, confirming, h.storedCode(t, first).Display())
				h.enterCode(t, confirming, h.storedCode(t, second).Display())
				return second
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				token := accessToken(t, h.login(t, newcomerEmail, accountPassword))
				h.advance(t, 61*time.Second)
				h.resend(t, newcomerEmail)
				h.tick(t)
				confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
				headers := signUpHeaders()
				headers["Authorization"] = "Bearer " + token
				return h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email", fmt.Sprintf(`{"token":%q}`, confirmation), headers)
			},
			waiters: 1, entryWant: http.StatusTooManyRequests,
			check: func(t *testing.T, h *authHarness, freed response) {
				t.Helper()
				if outcomeOf(t, freed) != "request_sent" || h.visibleCount(t, h.userID(t, secondTutorEmail)) != domain.VisiblePendingRequestCap {
					t.Fatalf("the confirmation answered %d %s", freed.status, freed.body)
				}
			},
		},
		"a signed-in acceptance that un-suppresses the next request": {
			contested: func(t *testing.T, h *authHarness) domain.UserID {
				t.Helper()
				tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				_, accepting := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
				h.fillPendingRequestsWith(t, tutor, "filler-", domain.VisiblePendingRequestCap-1)
				h.enterCode(t, accepting, h.storedCode(t, tutor).Display())
				h.suppressedForLimit(t, tutor, 2, newcomerEmail)
				secondLink = linkToken(t, h.newCard(t, h.relogin(t, tutorEmail), "A second card", "").Invitation)
				return tutor
			},
			free: func(t *testing.T, h *authHarness) <-chan pendingAnswer {
				t.Helper()
				return h.trySignedInAcceptance(accessToken(t, h.login(t, studentEmail, accountPassword)), secondLink)
			},
			waiters: 1, entryWant: http.StatusTooManyRequests,
			check: func(t *testing.T, h *authHarness, freed response) {
				t.Helper()
				if acceptedTutor(t, freed).State != "active" || !strings.Contains(h.logs.String(), requestFreed) {
					t.Fatal("the acceptance did not accept the request or un-suppress the waiting one")
				}
			},
		},
	}
}

func TestSlotFreeingPathsFollowTheLockOrder(t *testing.T) {
	for name, race := range lockOrderRaces() {
		t.Run(name, func(t *testing.T) {
			h := newFlowHarness(t)
			contested := race.contested(t, h)
			entrant := h.racingEntrant(t)
			contestedCode := h.storedCode(t, contested).Display()
			release := h.holdTutorProfile(t, contested)

			freeing := race.free(t, h)
			h.waitForLockWaiters(t, race.waiters)
			entry := h.tryEnterCode(entrant, contestedCode)
			h.waitForLockWaiters(t, race.waiters+1)
			release()
			freed, entered := awaitAnswer(t, freeing), awaitAnswer(t, entry)

			if freed.status >= http.StatusInternalServerError || entered.status != race.entryWant {
				t.Fatalf("the freeing path answered %d %s and the entry %d %s", freed.status, freed.body, entered.status, entered.body)
			}
			if h.visibleCount(t, contested) > domain.VisiblePendingRequestCap {
				t.Fatalf("the tutor has %d visible requests", h.visibleCount(t, contested))
			}
			if race.check != nil {
				race.check(t, h, freed)
			}
			if strings.Contains(h.logs.String(), `"level":"ERROR"`) {
				t.Fatal("the race logged an error")
			}
		})
	}
}

func TestJoinRequestListsPageWithoutGapsOrRepeats(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillPendingRequestsWith(t, tutor, "filler-", 5)
	seen := map[string]bool{}
	cursor := ""

	for range 3 {
		path := joinRequestsPath + "?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		result := h.send(t, http.MethodGet, path, "", jsonCall(tutorToken))
		var page struct {
			Items      []tutorRequest `json:"items"`
			NextCursor *string        `json:"next_cursor"`
		}
		if err := json.Unmarshal([]byte(result.body), &page); err != nil || result.status != http.StatusOK {
			t.Fatalf("a page answered %d %s", result.status, result.body)
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("the request %s appeared twice", item.ID)
			}
			seen[item.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	invalid := h.send(t, http.MethodGet, joinRequestsPath+"?cursor=not-a-cursor", "", jsonCall(tutorToken))

	if len(seen) != 5 {
		t.Fatalf("the pages listed %d of 5 requests", len(seen))
	}
	if invalid.status != http.StatusUnprocessableEntity || strings.Join(fields(t, invalid), ",") != "query.cursor:invalid" {
		t.Fatalf("an invalid cursor answered %d %s", invalid.status, invalid.body)
	}
}

func TestOverdueRequestLeavesTheTutorsViewBeforeTheJob(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	expiry := authStart.Add(domain.JoinRequestLifetime)
	h.at(t, expiry)
	tutorToken := h.relogin(t, tutorEmail)

	listed := h.tutorRequests(t, tutorToken)
	accepted := h.acceptRequest(t, tutorToken, requestText(request))
	random := h.acceptRequest(t, tutorToken, randomCardID)
	_, sent := h.sentRequests(t, accessToken(t, h.login(t, studentEmail, accountPassword)))

	if h.storedRequest(t, request).State != "pending" {
		t.Fatal("the job ran before the assertions")
	}
	if len(listed) != 0 || accepted.status != http.StatusNotFound || refusal(t, accepted) != refusal(t, random) {
		t.Fatalf("before the job the tutor sees %+v and accepting answered %d %s", listed, accepted.status, accepted.body)
	}
	if len(sent) != 1 || sent[0].State != "expired" || sent[0].DecidedAt == nil || !sent[0].DecidedAt.Equal(expiry) {
		t.Fatalf("before the job the student sees %+v", sent)
	}
}

func TestOperatorBlockedStudentsHiddenRequestStillCountsTowardTheLimit(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blocked, blockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, entrant := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, entrantEmail)
	h.fillPendingRequestsWith(t, tutor, "filler-", domain.VisiblePendingRequestCap-1)
	tutorCode := h.storedCode(t, tutor).Display()
	h.enteredRequest(t, blockedToken, tutorCode)
	if _, err := h.store.Repositories().Users.Block(context.Background(), blocked, authStart); err != nil {
		t.Fatal(err)
	}

	listed := h.tutorRequests(t, tutorToken)
	refused := h.enterCode(t, entrant, tutorCode)

	if len(listed) != domain.VisiblePendingRequestCap-1 {
		t.Fatalf("the tutor sees %d requests", len(listed))
	}
	if refused.status != http.StatusTooManyRequests || code(t, refused) != "tutor_unavailable" {
		t.Fatalf("an entry against the full limit answered %d %s", refused.status, refused.body)
	}
}

func TestBlockedStudentsListPagesThroughTiesWithoutGapsOrRepeats(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for index := range 5 {
		student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(10+index), fmt.Sprintf("blocked%d@example.org", index))
		h.blockRequester(t, tutor, student)
	}
	_, err := h.database.Pool.Exec(context.Background(), "UPDATE blocked_requesters SET created_at = $1 WHERE student_id = $2", authStart.Add(time.Hour), domain.UserID{14})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	cursor := ""

	for range 4 {
		path := blockedPath + "?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		result := h.send(t, http.MethodGet, path, "", jsonCall(tutorToken))
		var page struct {
			Items      []blockedStudent `json:"items"`
			NextCursor *string          `json:"next_cursor"`
		}
		if err := json.Unmarshal([]byte(result.body), &page); err != nil || result.status != http.StatusOK {
			t.Fatalf("a page answered %d %s", result.status, result.body)
		}
		for _, item := range page.Items {
			order = append(order, item.StudentID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	want := []string{domain.UserID{14}.String(), domain.UserID{10}.String(), domain.UserID{11}.String(), domain.UserID{12}.String(), domain.UserID{13}.String()}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("the pages listed %v instead of %v", order, want)
	}
}

func (h *authHarness) seedDueRequests(t *testing.T, tutor domain.UserID, prefix string, count int, expiresAt time.Time) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at,
    unverified_since, created_at, updated_at)
SELECT gen_random_uuid(), 'student', 'active', $3 || n || '@example.org', $1, '$argon2id$unused', 'A requester', 'en', 'UTC', 'system', '2026-10', $1, $1, $1, $1
FROM generate_series(1, $2::int) AS n`, authStart, count, prefix)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.database.Pool.Exec(context.Background(), `
INSERT INTO join_requests (id, tutor_id, student_id, state, suppressed, owner_choice_pending, expires_at, created_at)
SELECT gen_random_uuid(), $1, users.id, 'pending', 'none', false, $2, $3 FROM users WHERE users.email LIKE $4`,
		tutor, expiresAt, expiresAt.Add(-domain.JoinRequestLifetime), prefix+"%")
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) failRequestExpiriesOf(t *testing.T, prefix string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), fmt.Sprintf(`
CREATE FUNCTION refuse_request_expiry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE users.id = NEW.student_id AND users.email::text LIKE '%s%%') THEN
        RAISE EXCEPTION 'the test refuses to expire this request';
    END IF;
    RETURN NEW;
END
$$`, prefix))
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.database.Pool.Exec(context.Background(), `
CREATE TRIGGER refuse_request_expiry BEFORE UPDATE OF state ON join_requests FOR EACH ROW
WHEN (NEW.state = 'expired') EXECUTE FUNCTION refuse_request_expiry()`)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) openRequestsOf(t *testing.T, pattern string) int64 {
	t.Helper()
	return h.countRows(t, `SELECT count(join_requests.id) FROM join_requests JOIN users ON users.id = join_requests.student_id
WHERE join_requests.state IN ('held', 'pending') AND users.email::text LIKE $1`, pattern)
}

func TestJoinRequestExpirySkipsFailingRequestsAcrossTheBatchBoundary(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	due := authStart.Add(-time.Hour)
	h.seedDueRequests(t, tutor, "failing-", 100, due)
	h.seedDueRequests(t, tutor, "due-", 150, due)
	h.failRequestExpiriesOf(t, "failing-")

	h.tick(t)

	if h.openRequestsOf(t, "due-%") != 0 || h.openRequestsOf(t, "failing-%") != 100 {
		t.Fatalf("the run left %d due and %d failing requests open", h.openRequestsOf(t, "due-%"), h.openRequestsOf(t, "failing-%"))
	}
	logs := h.logs.String()
	if strings.Count(logs, `"msg":"join_requests.expire_failed"`) != 100 || !strings.Contains(logs, `"job":"join_requests.expire","result":"failed","batch_size":150`) {
		t.Fatal("the failing requests or the failed run were not recorded")
	}
}

func TestJoinRequestExpiryResumesPastPersistentFailures(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.seedDueRequests(t, tutor, "failing-", 1000, authStart.Add(-2*time.Hour))
	h.seedDueRequests(t, tutor, "due-", 5, authStart.Add(-time.Hour))
	h.failRequestExpiriesOf(t, "failing-")

	h.tick(t)
	afterFirstRun := h.openRequestsOf(t, "due-%")
	h.advance(t, time.Hour)
	h.tick(t)

	if afterFirstRun != 5 {
		t.Fatalf("the first run, spent on failing requests, expired %d later ones", 5-afterFirstRun)
	}
	if h.openRequestsOf(t, "due-%") != 0 || h.openRequestsOf(t, "failing-%") != 1000 {
		t.Fatal("the next run did not resume past the failing requests")
	}
	if !strings.Contains(h.logs.String(), `"job":"join_requests.expire","result":"ok","batch_size":5`) {
		t.Fatal("the resumed run did not record the five expiries")
	}
}

func (h *authHarness) closedByUnblock(t *testing.T, tutor domain.UserID, tutorToken string, student domain.UserID, token string) domain.JoinRequestID {
	t.Helper()
	tutorCode := h.storedCode(t, tutor).Display()
	h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, tutorCode)), true)
	closed := h.enteredRequest(t, token, tutorCode)
	if unblocked := h.unblock(t, tutorToken, student); unblocked.status != http.StatusNoContent {
		t.Fatalf("unblocking answered %d %s", unblocked.status, unblocked.body)
	}
	return closed
}

func (h *authHarness) endRelationship(t *testing.T, id string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(),
		"UPDATE relationships SET state = 'ended', ended_at = $1, ended_by = 'tutor' WHERE id = $2::uuid", authStart, id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignedInAcceptanceAfterAnUnblockAcceptsTheClosedRequest(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherTutor, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	closed := h.closedByUnblock(t, tutor, tutorToken, student, token)
	ordinary := h.enteredRequest(t, token, h.storedCode(t, otherTutor).Display())
	h.advance(t, time.Minute)

	first := h.linkedSignedIn(t, tutorToken, token, cardName, "")
	h.linkedSignedIn(t, otherToken, token, cardName, "")
	_, sent := h.sentRequests(t, token)
	h.endRelationship(t, first.ID)
	h.advance(t, time.Minute)
	renewed := h.enterCode(t, token, h.storedCode(t, tutor).Display())

	if got, want := sentByID(t, sent, closed), sentByID(t, sent, ordinary); got.State != "accepted" || !sameLifecycle(got, want) {
		t.Fatalf("the closed request shows %+v and an ordinary one %+v", got, want)
	}
	if h.storedRequest(t, closed).State != "accepted" {
		t.Fatal("the acceptance left the closed request unaccepted")
	}
	if renewed.status != http.StatusOK || sentRequestOf(t, renewed).ID == requestText(closed) {
		t.Fatalf("a code entry after the link ended answered %d %s", renewed.status, renewed.body)
	}
}

func TestSignedInAcceptanceAfterTheExpiryLeavesTheRequestExpired(t *testing.T) {
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	expiry := authStart.Add(domain.JoinRequestLifetime)
	h.at(t, expiry)
	tutorToken, token := h.relogin(t, tutorEmail), accessToken(t, h.login(t, studentEmail, accountPassword))

	h.linkedSignedIn(t, tutorToken, token, cardName, "")
	_, beforeJob := h.sentRequests(t, token)
	h.tick(t)
	_, afterJob := h.sentRequests(t, token)

	for when, sent := range map[string][]sentRequest{"before the job": beforeJob, "after the job": afterJob} {
		if got := sentByID(t, sent, request); got.State != "expired" || got.DecidedAt == nil || !got.DecidedAt.Equal(expiry) {
			t.Fatalf("%s the overdue request shows %+v", when, got)
		}
	}
	if h.storedRequest(t, request).State != "expired" {
		t.Fatal("the job did not expire the overdue request the acceptance left alone")
	}
}

func TestStudentDeletionClosesTheRequestsAnUnblockClosed(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	closed := h.closedByUnblock(t, tutor, tutorToken, student, token)
	h.advance(t, time.Minute)

	deleted := h.deleteMe(t, token, accountPassword)

	var decidedAt time.Time
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT decided_at FROM join_requests WHERE id = $1", closed).Scan(&decidedAt); err != nil {
		t.Fatal(err)
	}
	if deleted.status != http.StatusNoContent || !decidedAt.Equal(authStart.Add(time.Minute)) {
		t.Fatalf("the deletion answered %d and the closed request was decided at %v", deleted.status, decidedAt)
	}
}

func TestStudentDeletionClosingAnUnblockClosedRowLeavesThatTutorAlone(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	tutorCode := h.storedCode(t, tutor).Display()
	h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, tutorCode)), true)
	h.fillPendingRequests(t, tutor)
	waiting := h.suppressedForLimit(t, tutor, 3, newcomerEmail)
	closed := h.enteredRequest(t, token, tutorCode)
	h.unblock(t, tutorToken, student)
	release := h.holdTutorProfile(t, tutor)

	deleted := awaitAnswer(t, h.sendInBackground(http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, accountPassword), jsonCall(token)))
	release()

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s while the tutor's profile was held", deleted.status, deleted.body)
	}
	if got := h.storedRequest(t, waiting); got != (storedRequest{State: "pending", Suppressed: "limit"}) {
		t.Fatalf("the deletion un-suppressed %+v", got)
	}
	if h.visibleCount(t, tutor) != domain.VisiblePendingRequestCap || strings.Contains(h.logs.String(), requestFreed) {
		t.Fatal("the deletion changed the tutor's visible requests")
	}
	if h.storedRequest(t, closed).State != "expired" {
		t.Fatal("the closed request changed state")
	}
}

func TestConcurrentStudentDeletionsSharingTutorsFinishWithoutADeadlock(t *testing.T) {
	h := newFlowHarness(t)
	first, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	second, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	closing, closingToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, "closing-tutor@example.org")
	var tokens []string
	for index, email := range []string{studentEmail, entrantEmail} {
		student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(10+index), email)
		h.enterCode(t, token, h.storedCode(t, first).Display())
		h.enterCode(t, token, h.storedCode(t, second).Display())
		h.closedByUnblock(t, closing, closingToken, student, token)
		tokens = append(tokens, token)
	}
	release := h.holdTutorProfile(t, second)
	deleteBody := fmt.Sprintf(`{"password":%q}`, accountPassword)

	firstDeletion := h.sendInBackground(http.MethodDelete, "/api/v1/users/me", deleteBody, jsonCall(tokens[0]))
	h.waitForLockWaiters(t, 1)
	secondDeletion := h.sendInBackground(http.MethodDelete, "/api/v1/users/me", deleteBody, jsonCall(tokens[1]))
	h.waitForLockWaiters(t, 2)
	release()
	answers := []response{awaitAnswer(t, firstDeletion), awaitAnswer(t, secondDeletion)}

	for index, answer := range answers {
		if answer.status != http.StatusNoContent {
			t.Fatalf("deletion %d answered %d %s", index+1, answer.status, answer.body)
		}
	}
	if h.visibleCount(t, first) != 0 || h.visibleCount(t, second) != 0 || strings.Contains(h.logs.String(), `"level":"ERROR"`) {
		t.Fatal("the deletions left visible requests or logged an error")
	}
	var futureDecisions int64
	err := h.database.Pool.QueryRow(context.Background(), "SELECT count(id) FROM join_requests WHERE tutor_id = $1 AND decided_at > $2",
		closing, authStart.Add(24*time.Hour)).Scan(&futureDecisions)
	if err != nil {
		t.Fatal(err)
	}
	if futureDecisions != 0 {
		t.Fatal("the deletions left a closed request with its future decision time")
	}
}

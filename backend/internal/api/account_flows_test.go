package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

const (
	newcomerEmail    = "newcomer@example.org"
	newcomerPassword = "newcomer horse battery"
	homeAddress      = "198.51.100.20"
	confirmSubject   = "Confirm your email for OVC"
	resetSubject     = "Reset your OVC password"
	duplicateSubject = "Someone tried to sign up with your email"
)

func newFlowHarness(t *testing.T) *authHarness {
	t.Helper()
	return newAuthHarnessBehind(t, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
}

type signUp struct {
	Role      string
	Name      string
	Email     string
	Password  string
	Language  string
	Timezone  string
	Consent   bool
	TutorCode string
}

func (form signUp) body() string {
	code := ""
	if form.TutorCode != "" {
		code = `,"tutor_code":` + jsonText(form.TutorCode)
	}
	return fmt.Sprintf(`{"role":%s,"name":%s,"email":%s,"password":%s,"language":%s,"timezone":%s,"consent":%t%s}`,
		jsonText(form.Role), jsonText(form.Name), jsonText(form.Email), jsonText(form.Password), jsonText(form.Language),
		jsonText(form.Timezone), form.Consent, code)
}

func jsonText(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic("a string could not be encoded as JSON")
	}
	return string(encoded)
}

func tutorSignUp(email string) signUp {
	return signUp{Role: "tutor", Name: "A language tutor", Email: email, Password: newcomerPassword, Language: "en", Timezone: "Europe/Moscow", Consent: true}
}

func (h *authHarness) postJSON(t *testing.T, path string, body string, headers map[string]string) response {
	t.Helper()
	all := map[string]string{"Content-Type": "application/json", "Origin": publicOrigin, "X-Forwarded-For": homeAddress}
	for name, value := range headers {
		all[name] = value
	}
	return h.send(t, http.MethodPost, path, body, all)
}

func (h *authHarness) register(t *testing.T, form signUp) response {
	t.Helper()
	return h.postJSON(t, "/api/v1/auth/register", form.body(), nil)
}

func (h *authHarness) verify(t *testing.T, body string, bearer string) response {
	t.Helper()
	headers := map[string]string{}
	if bearer != "" {
		headers["Authorization"] = "Bearer " + bearer
	}
	return h.postJSON(t, "/api/v1/auth/verify-email", body, headers)
}

func (h *authHarness) forgot(t *testing.T, email string) response {
	t.Helper()
	return h.postJSON(t, "/api/v1/auth/password/forgot", fmt.Sprintf(`{"email":%q}`, email), nil)
}

func (h *authHarness) resend(t *testing.T, email string) response {
	t.Helper()
	return h.postJSON(t, "/api/v1/auth/verify-email/resend", fmt.Sprintf(`{"email":%q}`, email), nil)
}

func (h *authHarness) tick(t *testing.T) {
	t.Helper()
	now, err := h.clock.Instant(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.worker.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) at(t *testing.T, instant time.Time) {
	t.Helper()
	if _, err := h.clock.Set(context.Background(), instant); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) mailTo(t *testing.T, recipient string) []mail.SinkMessage {
	t.Helper()
	messages, err := h.sink.Messages(context.Background(), recipient)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func (h *authHarness) userID(t *testing.T, email string) domain.UserID {
	t.Helper()
	user, err := h.store.Repositories().Users.ByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func subjects(messages []mail.SinkMessage) []string {
	found := make([]string, 0, len(messages))
	for _, message := range messages {
		found = append(found, message.Subject)
	}
	slices.Sort(found)
	return found
}

func tokenOf(t *testing.T, message mail.SinkMessage) string {
	t.Helper()
	for _, link := range message.Links {
		parsed, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		if token := parsed.Query().Get("token"); token != "" {
			return token
		}
	}
	t.Fatalf("no token link in %q", message.Subject)
	return ""
}

func latestToken(t *testing.T, messages []mail.SinkMessage, subject string) string {
	t.Helper()
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Subject == subject {
			return tokenOf(t, messages[index])
		}
	}
	t.Fatalf("no %q email among %v", subject, subjects(messages))
	return ""
}

func fields(t *testing.T, result response) []string {
	t.Helper()
	var decoded struct {
		Error struct {
			Fields []struct {
				Path string `json:"path"`
				Code string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("%v in %s", err, result.body)
	}
	found := make([]string, 0, len(decoded.Error.Fields))
	for _, field := range decoded.Error.Fields {
		found = append(found, field.Path+":"+field.Code)
	}
	return found
}

func comparableHeaders(headers http.Header) map[string]string {
	kept := map[string]string{}
	for name, values := range headers {
		if name == "Date" || name == "X-Request-Id" {
			continue
		}
		kept[name] = strings.Join(values, ",")
	}
	return kept
}

func TestRegisterAnswersIdenticallyForNewAndExistingEmail(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)

	fresh := h.register(t, tutorSignUp(newcomerEmail))
	taken := h.register(t, tutorSignUp(tutorEmail))
	h.tick(t)

	if fresh.status != http.StatusAccepted || taken.status != fresh.status || taken.body != fresh.body {
		t.Fatalf("new %d %q, existing %d %q", fresh.status, fresh.body, taken.status, taken.body)
	}
	if got, want := comparableHeaders(taken.headers), comparableHeaders(fresh.headers); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("headers differ: %v and %v", want, got)
	}
	if got := subjects(h.mailTo(t, newcomerEmail)); !slices.Equal(got, []string{confirmSubject}) {
		t.Fatalf("the newcomer received %v", got)
	}
	if got := subjects(h.mailTo(t, tutorEmail)); !slices.Equal(got, []string{duplicateSubject}) {
		t.Fatalf("the existing account received %v", got)
	}
}

func TestDuplicateSignUpSendsOneNoticePerDayAndNoneForADeletedAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	deleted := h.seedUser(t, 2, "gone@example.org", "correct horse battery", currentHash)
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil {
		t.Fatal(err)
	}

	h.register(t, tutorSignUp(tutorEmail))
	h.register(t, tutorSignUp(tutorEmail))
	h.register(t, tutorSignUp("gone@example.org"))
	h.tick(t)
	h.at(t, authStart.Add(24*time.Hour))
	h.register(t, tutorSignUp(tutorEmail))
	h.tick(t)

	if got := subjects(h.mailTo(t, tutorEmail)); !slices.Equal(got, []string{duplicateSubject, duplicateSubject}) {
		t.Fatalf("the existing account received %v", got)
	}
	if got := h.mailTo(t, "gone@example.org"); len(got) != 0 {
		t.Fatalf("the deleted account received %v", subjects(got))
	}
}

func TestRegisterRefusesInvalidInput(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	cases := map[string]struct {
		change func(*signUp)
		field  string
	}{
		"student":         {change: func(form *signUp) { form.Role = "student" }, field: "body.tutor_code:required"},
		"tutor with code": {change: func(form *signUp) { form.TutorCode = "2222-2222" }, field: "body.tutor_code:not_allowed"},
		"short password":  {change: func(form *signUp) { form.Password = "ninechars" }, field: "body.password:too_short"},
		"long password":   {change: func(form *signUp) { form.Password = strings.Repeat("п", 129) }, field: "body.password:too_long"},
		"no consent":      {change: func(form *signUp) { form.Consent = false }, field: "body.consent:required"},
		"display name":    {change: func(form *signUp) { form.Email = "Tutor <tutor@example.org>" }, field: "body.email:invalid"},
		"line break":      {change: func(form *signUp) { form.Email = "tutor@example.org\r\nBcc: x@example.org" }, field: "body.email:invalid"},
		"empty name":      {change: func(form *signUp) { form.Name = "   " }, field: "body.name:invalid"},
		"control in name": {change: func(form *signUp) { form.Name = "Tutor\u0007" }, field: "body.name:invalid"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			form := tutorSignUp(newcomerEmail)
			testCase.change(&form)

			result := h.register(t, form)

			if result.status != http.StatusUnprocessableEntity || code(t, result) != "validation_failed" || !slices.Contains(fields(t, result), testCase.field) {
				t.Fatalf("answered %d %s", result.status, result.body)
			}
		})
	}
}

func TestRegisterAcceptsALongCyrillicPasswordAndFallsBackToUTCForAnUnknownZone(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	form := tutorSignUp(newcomerEmail)
	form.Password = strings.Repeat("п", 128)
	form.Timezone = "Mars/Olympus_Mons"

	result := h.register(t, form)

	user, err := h.store.Repositories().Users.ByEmail(context.Background(), newcomerEmail)
	if result.status != http.StatusAccepted || err != nil || user.Timezone != "UTC" || user.State != domain.AccountUnverified || user.ConsentVersion != string(domain.CurrentConsentVersion) {
		t.Fatalf("answered %d %s; stored %+v, %v", result.status, result.body, user, err)
	}
}

func TestSignUpAndResetRequestsShareTheLimitOfTenPerHourPerAddress(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	for index := range 9 {
		if result := h.register(t, tutorSignUp(fmt.Sprintf("person%d@example.org", index))); result.status != http.StatusAccepted {
			t.Fatalf("sign-up %d answered %d %s", index, result.status, result.body)
		}
	}
	tenth := h.forgot(t, "someone@example.org")

	eleventh := h.register(t, tutorSignUp(newcomerEmail))
	elsewhere := h.postJSON(t, "/api/v1/auth/register", tutorSignUp(newcomerEmail).body(), map[string]string{"X-Forwarded-For": "203.0.113.9"})

	if tenth.status != http.StatusAccepted {
		t.Fatalf("the tenth request answered %d %s", tenth.status, tenth.body)
	}
	if eleventh.status != http.StatusTooManyRequests || code(t, eleventh) != "rate_limited" || eleventh.headers.Get("Retry-After") != "3600" {
		t.Fatalf("the eleventh request answered %d %s %v", eleventh.status, eleventh.body, eleventh.headers)
	}
	if elsewhere.status != http.StatusAccepted {
		t.Fatalf("another address answered %d %s", elsewhere.status, elsewhere.body)
	}
}

func TestConfirmFromAnotherBrowserRequiresPasswordAndRevokesSessions(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	squatter := h.login(t, newcomerEmail, newcomerPassword)
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)

	withPassword := fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, token)
	foreignOrigin := h.postJSON(t, "/api/v1/auth/verify-email", withPassword, map[string]string{"Origin": "https://meet.ovc.example"})
	twoOrigins := h.sendHeaders(t, http.MethodPost, "/api/v1/auth/verify-email", withPassword, http.Header{
		"Origin": []string{publicOrigin, publicOrigin}, "Content-Type": []string{"application/json"}, "X-Forwarded-For": []string{homeAddress},
	})
	withoutPassword := h.verify(t, fmt.Sprintf(`{"token":%q}`, token), "")
	confirmed := h.verify(t, withPassword, "")
	squatterRefresh := h.refresh(t, refreshValue(t, squatter))
	oldPassword := h.login(t, newcomerEmail, newcomerPassword)
	me := h.me(t, accessToken(t, confirmed))

	if foreignOrigin.status != http.StatusForbidden || code(t, foreignOrigin) != "forbidden" ||
		twoOrigins.status != http.StatusForbidden || code(t, twoOrigins) != "forbidden" {
		t.Fatalf("a foreign origin answered %d %s, two origins %d", foreignOrigin.status, foreignOrigin.body, twoOrigins.status)
	}
	if withoutPassword.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, withoutPassword), []string{"body.new_password:required"}) {
		t.Fatalf("without a password %d %s", withoutPassword.status, withoutPassword.body)
	}
	if confirmed.status != http.StatusOK || refreshValue(t, confirmed) == "" || confirmed.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("confirmation %d %s %v", confirmed.status, confirmed.body, confirmed.headers)
	}
	if squatterRefresh.status != http.StatusUnauthorized || oldPassword.status != http.StatusUnauthorized {
		t.Fatalf("the squatter's session refreshed with %d and the old password answered %d", squatterRefresh.status, oldPassword.status)
	}
	if me.status != http.StatusOK || !strings.Contains(me.body, `"state":"active"`) {
		t.Fatalf("me %d %s", me.status, me.body)
	}
	for _, event := range []string{"auth.signup.accepted", "auth.email.confirmed"} {
		if !strings.Contains(h.logs.String(), `"msg":"`+event+`"`) {
			t.Fatalf("no %s record", event)
		}
	}
	for _, secret := range []string{newcomerEmail, newcomerPassword, "mailbox owner password", token} {
		if strings.Contains(h.logs.String(), secret) {
			t.Fatalf("the log contains %q", secret)
		}
	}
}

func TestConfirmFromTheSignedInBrowserKeepsOnlyThatSession(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	here := h.login(t, newcomerEmail, newcomerPassword)
	elsewhere := h.login(t, newcomerEmail, newcomerPassword)
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)

	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q}`, token), accessToken(t, here))
	hereRefresh := h.refresh(t, refreshValue(t, here))
	elsewhereRefresh := h.refresh(t, refreshValue(t, elsewhere))
	me := h.me(t, accessToken(t, here))

	if confirmed.status != http.StatusOK || confirmed.headers.Get("Set-Cookie") != "" || strings.TrimSpace(confirmed.body) != `{"outcome":null}` {
		t.Fatalf("confirmation %d %s %v", confirmed.status, confirmed.body, confirmed.headers)
	}
	if hereRefresh.status != http.StatusOK || elsewhereRefresh.status != http.StatusUnauthorized {
		t.Fatalf("this session refreshed with %d, the other with %d", hereRefresh.status, elsewhereRefresh.status)
	}
	if !strings.Contains(me.body, `"state":"active"`) || !strings.Contains(me.body, `"email_verified":true`) {
		t.Fatalf("me %s", me.body)
	}
}

func TestConfirmationTokenIsMintedAtDispatchAndSupersedesEarlierOnes(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	minted := h.countRows(t, "SELECT count(id) FROM email_tokens WHERE purpose <> $1", "")
	h.tick(t)
	first := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	h.at(t, authStart.Add(61*time.Second))
	h.resend(t, newcomerEmail)
	h.tick(t)
	second := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)

	superseded := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, first), "")
	current := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, second), "")
	reused := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, second), "")

	if minted != 0 || first == second {
		t.Fatalf("%d tokens before dispatch; tokens %q and %q", minted, first, second)
	}
	if superseded.status != http.StatusUnprocessableEntity || code(t, superseded) != "token_invalid" {
		t.Fatalf("the superseded token answered %d %s", superseded.status, superseded.body)
	}
	if current.status != http.StatusOK || reused.status != http.StatusUnprocessableEntity || code(t, reused) != "token_invalid" {
		t.Fatalf("the current token answered %d, reused %d %s", current.status, reused.status, reused.body)
	}
	for _, token := range []string{first, second} {
		if leaked := h.countRows(t, "SELECT count(id) FROM outbox WHERE strpos(payload::text, $1) > 0", token); leaked != 0 {
			t.Fatalf("an outbox payload holds a token")
		}
	}
}

func (h *authHarness) countRows(t *testing.T, query string, argument string) int64 {
	t.Helper()
	var count int64
	if err := h.database.Pool.QueryRow(context.Background(), query, argument).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestConfirmationTokenExpiresAfter24Hours(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	h.at(t, authStart.Add(24*time.Hour))

	expired := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, token), "")

	if expired.status != http.StatusUnprocessableEntity || code(t, expired) != "token_invalid" {
		t.Fatalf("the expired token answered %d %s", expired.status, expired.body)
	}
}

func TestConfirmationAppliesOnlyToAnUnverifiedAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	h.register(t, tutorSignUp("blocked@example.org"))
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	if _, err := h.store.Repositories().Users.Block(context.Background(), h.userID(t, newcomerEmail), authStart); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Repositories().Users.Block(context.Background(), h.userID(t, "blocked@example.org"), authStart); err != nil {
		t.Fatal(err)
	}
	h.at(t, authStart.Add(61*time.Second))
	h.resend(t, "blocked@example.org")

	refused := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, token), "")
	h.tick(t)

	if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "token_invalid" {
		t.Fatalf("a blocked account's token answered %d %s", refused.status, refused.body)
	}
	if got := subjects(h.mailTo(t, "blocked@example.org")); len(got) != 1 {
		t.Fatalf("the blocked account received %v", got)
	}
}

func TestConfirmationAndResetEmailsShareTheAccountSendingLimits(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	moments := []time.Duration{0, 61 * time.Second, 122 * time.Second, 183 * time.Second, 244 * time.Second, 305 * time.Second,
		time.Hour, time.Hour + 61*time.Second, time.Hour + 122*time.Second, time.Hour + 183*time.Second, time.Hour + 244*time.Second,
		24 * time.Hour}
	var statuses []int
	for index, moment := range moments {
		h.at(t, authStart.Add(moment))
		request := h.forgot
		if index%2 == 1 {
			request = h.resend
		}
		statuses = append(statuses, request(t, newcomerEmail).status)
		h.tick(t)
	}

	sent := h.mailTo(t, newcomerEmail)
	for _, status := range statuses {
		if status != http.StatusAccepted {
			t.Fatalf("a limited request was not silent: %v", statuses)
		}
	}
	if len(sent) != 11 {
		t.Fatalf("%d emails were sent: %v", len(sent), subjects(sent))
	}
}

func TestPublicResendAndForgotAnswerIdenticallyForUnknownEmails(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	h.at(t, authStart.Add(61*time.Second))

	knownResend := h.resend(t, newcomerEmail)
	unknownResend := h.resend(t, "nobody@example.org")
	h.at(t, authStart.Add(122*time.Second))
	knownForgot := h.forgot(t, newcomerEmail)
	unknownForgot := h.forgot(t, "nobody@example.org")
	h.tick(t)

	for _, pair := range [][2]response{{knownResend, unknownResend}, {knownForgot, unknownForgot}} {
		if pair[0].status != http.StatusAccepted || pair[1].status != pair[0].status || pair[1].body != pair[0].body ||
			fmt.Sprint(comparableHeaders(pair[0].headers)) != fmt.Sprint(comparableHeaders(pair[1].headers)) {
			t.Fatalf("known %d %q, unknown %d %q", pair[0].status, pair[0].body, pair[1].status, pair[1].body)
		}
	}
	if got := h.mailTo(t, "nobody@example.org"); len(got) != 0 {
		t.Fatalf("an unknown email received %v", subjects(got))
	}
	if got := subjects(h.mailTo(t, newcomerEmail)); !slices.Equal(got, []string{confirmSubject, resetSubject}) {
		t.Fatalf("the newcomer received %v", got)
	}
}

func TestSignedInResendIsRateLimitedAndSendsNothingToAnActiveAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.register(t, tutorSignUp(newcomerEmail))
	unverified := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	active := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	limited := h.postJSON(t, "/api/v1/auth/verify-email/resend", `{}`, map[string]string{"Authorization": "Bearer " + unverified})
	confirmed := h.postJSON(t, "/api/v1/auth/verify-email/resend", `{}`, map[string]string{"Authorization": "Bearer " + active})
	anonymous := h.postJSON(t, "/api/v1/auth/verify-email/resend", `{}`, nil)
	h.tick(t)

	if limited.status != http.StatusTooManyRequests || limited.headers.Get("Retry-After") != "60" {
		t.Fatalf("a resend within a minute answered %d %s %v", limited.status, limited.body, limited.headers)
	}
	if confirmed.status != http.StatusAccepted || len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatalf("an active account's resend answered %d and sent %d emails", confirmed.status, len(h.mailTo(t, tutorEmail)))
	}
	if anonymous.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, anonymous), []string{"body.email:required"}) {
		t.Fatalf("an anonymous resend without an email answered %d %s", anonymous.status, anonymous.body)
	}
}

func TestPasswordResetRevokesSessionsAndConfirmsTheEmail(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	signedIn := h.login(t, newcomerEmail, newcomerPassword)
	h.at(t, authStart.Add(61*time.Second))
	h.forgot(t, newcomerEmail)
	h.tick(t)
	messages := h.mailTo(t, newcomerEmail)
	confirmation, reset := latestToken(t, messages, confirmSubject), latestToken(t, messages, resetSubject)

	short := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"short"}`, reset), nil)
	done := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"a brand new password"}`, reset), nil)
	again := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"a brand new password"}`, reset), nil)
	staleConfirmation := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, confirmation), "")
	oldSession := h.refresh(t, refreshValue(t, signedIn))
	newSignIn := h.login(t, newcomerEmail, "a brand new password")

	if short.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, short), []string{"body.new_password:too_short"}) {
		t.Fatalf("a short password answered %d %s", short.status, short.body)
	}
	if done.status != http.StatusOK || strings.TrimSpace(done.body) != `{"outcome":null}` || again.status != http.StatusUnprocessableEntity || code(t, again) != "token_invalid" {
		t.Fatalf("reset %d %s, reuse %d %s", done.status, done.body, again.status, again.body)
	}
	if staleConfirmation.status != http.StatusUnprocessableEntity || oldSession.status != http.StatusUnauthorized || newSignIn.status != http.StatusOK {
		t.Fatalf("confirmation %d, old session %d, new sign-in %d", staleConfirmation.status, oldSession.status, newSignIn.status)
	}
	if me := h.me(t, accessToken(t, newSignIn)); !strings.Contains(me.body, `"state":"active"`) {
		t.Fatalf("me %s", me.body)
	}
}

func TestResetTokenExpiresAfterOneHour(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.forgot(t, tutorEmail)
	h.tick(t)
	expiring := latestToken(t, h.mailTo(t, tutorEmail), resetSubject)
	h.at(t, authStart.Add(time.Hour))

	expired := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"a brand new password"}`, expiring), nil)

	if expired.status != http.StatusUnprocessableEntity || code(t, expired) != "token_invalid" {
		t.Fatalf("an expired reset answered %d %s", expired.status, expired.body)
	}
}

func TestResetIsRefusedForADeletedAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	deleted := h.seedUser(t, 2, "gone@example.org", "correct horse battery", currentHash)
	h.forgot(t, "gone@example.org")
	h.tick(t)
	orphaned := latestToken(t, h.mailTo(t, "gone@example.org"), resetSubject)
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil {
		t.Fatal(err)
	}

	refused := h.postJSON(t, "/api/v1/auth/password/reset", fmt.Sprintf(`{"token":%q,"new_password":"a brand new password"}`, orphaned), nil)

	if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "token_invalid" {
		t.Fatalf("a deleted account's reset answered %d %s", refused.status, refused.body)
	}
}

func TestChangePasswordRevokesOtherSessionsAndKeepsThisOne(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	here := h.login(t, tutorEmail, "correct horse battery")
	elsewhere := h.login(t, tutorEmail, "correct horse battery")

	changed := h.postJSON(t, "/api/v1/auth/password/change", `{"current_password":"correct horse battery","new_password":"a brand new password"}`,
		map[string]string{"Authorization": "Bearer " + accessToken(t, here)})
	hereRefresh := h.refresh(t, refreshValue(t, here))
	elsewhereRefresh := h.refresh(t, refreshValue(t, elsewhere))
	oldPassword := h.login(t, tutorEmail, "correct horse battery")
	newPassword := h.login(t, tutorEmail, "a brand new password")

	if changed.status != http.StatusNoContent {
		t.Fatalf("change answered %d %s", changed.status, changed.body)
	}
	if hereRefresh.status != http.StatusOK || elsewhereRefresh.status != http.StatusUnauthorized {
		t.Fatalf("this session refreshed with %d, the other with %d", hereRefresh.status, elsewhereRefresh.status)
	}
	if oldPassword.status != http.StatusUnauthorized || newPassword.status != http.StatusOK {
		t.Fatalf("old password %d, new password %d", oldPassword.status, newPassword.status)
	}
}

func TestWrongCurrentPasswordIsRefusedAndThrottledAfterFiveAttempts(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	attempt := func(current string) response {
		body := fmt.Sprintf(`{"current_password":%q,"new_password":"a brand new password"}`, current)
		return h.postJSON(t, "/api/v1/auth/password/change", body, map[string]string{"Authorization": "Bearer " + token})
	}
	var refusals []response
	for range 5 {
		refusals = append(refusals, attempt("wrong horse battery"))
	}

	throttled := attempt("correct horse battery")

	for _, refusal := range refusals {
		if refusal.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, refusal), []string{"body.current_password:incorrect"}) {
			t.Fatalf("a wrong current password answered %d %s", refusal.status, refusal.body)
		}
	}
	if throttled.status != http.StatusTooManyRequests || throttled.headers.Get("Retry-After") == "" {
		t.Fatalf("the sixth attempt answered %d %s", throttled.status, throttled.body)
	}
	if counted := h.countRows(t, "SELECT coalesce(max(count), 0) FROM rate_limit_counters WHERE purpose = $1", "current_password"); counted != 5 {
		t.Fatalf("the refused attempt was counted: %d", counted)
	}
}

func TestAccountEmailsFollowTheLanguageAndCarryNoUserText(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	form := tutorSignUp(newcomerEmail)
	form.Name = "Zelda <b>Bold</b>"
	form.Language = "ru"
	h.register(t, form)
	h.register(t, form)
	h.at(t, authStart.Add(61*time.Second))
	h.forgot(t, newcomerEmail)
	h.tick(t)

	messages := h.mailTo(t, newcomerEmail)

	if got := subjects(messages); !slices.Equal(got, []string{"Подтвердите email в OVC", "Попытка регистрации с вашим email", "Сброс пароля в OVC"}) {
		t.Fatalf("subjects %v", got)
	}
	for _, message := range messages {
		for _, part := range []string{message.Subject, message.Text, message.HTML} {
			if strings.Contains(part, "Zelda") || strings.Contains(part, "Bold") {
				t.Fatalf("%q carries the user's name", message.Subject)
			}
		}
		for _, link := range message.Links {
			if !strings.HasPrefix(link, publicOrigin+"/") {
				t.Fatalf("%q links outside OVC: %s", message.Subject, link)
			}
		}
	}
}

func TestTheResetEmailFollowsTheLanguageSavedInTheSettings(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	saved := h.send(t, http.MethodPatch, "/api/v1/users/me", `{"language":"ru"}`, jsonCall(token))
	h.forgot(t, tutorEmail)
	h.tick(t)

	if saved.status != http.StatusOK {
		t.Fatalf("saving the language answered %d %s", saved.status, saved.body)
	}
	if got := subjects(h.mailTo(t, tutorEmail)); !slices.Equal(got, []string{"Сброс пароля в OVC"}) {
		t.Fatalf("subjects %v", got)
	}
}

func (h *authHarness) obsoleteRows(t *testing.T, kind domain.OutboxKind) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1 AND status = 'done' AND done_reason = 'obsolete'", string(kind))
}

func TestQueuedAccountEmailsAreDroppedWhenTheirSubjectChangedBeforeDispatch(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.seedUser(t, 2, "owner@example.org", "correct horse battery", currentHash)
	h.register(t, tutorSignUp(newcomerEmail))
	h.register(t, tutorSignUp(tutorEmail))
	h.forgot(t, "owner@example.org")
	owner := accessToken(t, h.login(t, "owner@example.org", "correct horse battery"))
	for _, email := range []string{newcomerEmail, tutorEmail} {
		if _, err := h.store.Repositories().Users.Block(context.Background(), h.userID(t, email), authStart); err != nil {
			t.Fatal(err)
		}
	}
	changed := h.postJSON(t, "/api/v1/auth/password/change", `{"current_password":"correct horse battery","new_password":"a brand new password"}`,
		map[string]string{"Authorization": "Bearer " + owner})

	h.tick(t)

	if changed.status != http.StatusNoContent {
		t.Fatalf("change answered %d %s", changed.status, changed.body)
	}
	for _, email := range []string{newcomerEmail, tutorEmail, "owner@example.org"} {
		if got := h.mailTo(t, email); len(got) != 0 {
			t.Fatalf("%s received %v", email, subjects(got))
		}
	}
	for _, kind := range []domain.OutboxKind{domain.EventEmailConfirmation, domain.EventSignUpDuplicate, domain.EventPasswordReset} {
		if dropped := h.obsoleteRows(t, kind); dropped != 1 {
			t.Fatalf("%d %s rows were dropped as obsolete", dropped, kind)
		}
	}
}

func TestAnUnverifiedPasswordChangeKeepsTheMailboxOwnersLinks(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	changed := h.postJSON(t, "/api/v1/auth/password/change", fmt.Sprintf(`{"current_password":%q,"new_password":"squatter password two"}`, newcomerPassword),
		map[string]string{"Authorization": "Bearer " + squatter})

	taken := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, token), "")

	if changed.status != http.StatusNoContent {
		t.Fatalf("change answered %d %s", changed.status, changed.body)
	}
	if taken.status != http.StatusOK || h.login(t, newcomerEmail, "squatter password two").status != http.StatusUnauthorized {
		t.Fatalf("the mailbox owner's confirmation answered %d %s", taken.status, taken.body)
	}
}

func TestSignUpRightAfterAResetRequestAnswersAlikeForANewAndATakenEmail(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.forgot(t, newcomerEmail)
	h.forgot(t, tutorEmail)
	sendsBefore := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose LIKE $1", "email_send%")

	fresh := h.register(t, tutorSignUp(newcomerEmail))
	taken := h.register(t, tutorSignUp(tutorEmail))
	h.tick(t)

	if sendsAfter := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose LIKE $1", "email_send%"); sendsAfter != sendsBefore {
		t.Fatalf("the limited sign-up left the sending counters at %d instead of %d", sendsAfter, sendsBefore)
	}

	if fresh.status != http.StatusAccepted || taken.status != fresh.status || taken.body != fresh.body ||
		fmt.Sprint(comparableHeaders(fresh.headers)) != fmt.Sprint(comparableHeaders(taken.headers)) {
		t.Fatalf("new %d %q, taken %d %q", fresh.status, fresh.body, taken.status, taken.body)
	}
	if user, err := h.store.Repositories().Users.ByEmail(context.Background(), newcomerEmail); err != nil || user.State != domain.AccountUnverified {
		t.Fatalf("the new account was not created: %+v %v", user, err)
	}
	if got := h.mailTo(t, newcomerEmail); len(got) != 0 {
		t.Fatalf("the limited sign-up still sent %v", subjects(got))
	}
}

func TestAnUnverifiedPasswordChangeKeepsAQueuedConfirmation(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.postJSON(t, "/api/v1/auth/password/change", fmt.Sprintf(`{"current_password":%q,"new_password":"squatter password two"}`, newcomerPassword),
		map[string]string{"Authorization": "Bearer " + squatter})
	h.tick(t)
	token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)

	taken := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":"mailbox owner password"}`, token), "")

	if taken.status != http.StatusOK {
		t.Fatalf("the mailbox owner's confirmation answered %d %s", taken.status, taken.body)
	}
}

func TestRegisterStoresUTCForAZoneNameThatIsNotCanonical(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	names := map[string]string{
		"dot segments":   "Europe/./Moscow",
		"over 64 bytes":  "Europe/" + strings.Repeat("./", 40) + "Moscow",
		"parent segment": "Europe/../Europe/Moscow",
		"posix tree":     "posix/Europe/Moscow",
		"leap tree":      "right/Europe/Moscow",
		"factory zone":   "Factory",
	}
	for label, zone := range names {
		t.Run(label, func(t *testing.T) {
			email := strings.ReplaceAll(label, " ", "-") + "@example.org"
			form := tutorSignUp(email)
			form.Timezone = zone

			result := h.register(t, form)

			user, err := h.store.Repositories().Users.ByEmail(context.Background(), email)
			if result.status != http.StatusAccepted || err != nil || user.Timezone != "UTC" {
				t.Fatalf("answered %d %s; stored %+v, %v", result.status, result.body, user.Timezone, err)
			}
		})
	}
}

package api_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testhasher"
	"ovc/backend/internal/testmail"
	"ovc/backend/internal/teststorage"
	"ovc/backend/internal/worker"
)

const (
	publicOrigin = "https://ovc.example"
	tutorEmail   = "tutor@example.org"
)

var authStart = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

type countingRandom struct {
	mutex   sync.Mutex
	counter uint64
	pending []byte
}

func (r *countingRandom) Read(buffer []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for len(r.pending) < len(buffer) {
		r.counter++
		block := sha256.Sum256(binary.BigEndian.AppendUint64(nil, r.counter))
		r.pending = append(r.pending, block[:]...)
	}
	copy(buffer, r.pending)
	r.pending = r.pending[len(buffer):]
	return len(buffer), nil
}

var processSeed = sync.OnceValues(func() ([]byte, error) {
	seed := make([]byte, 16)
	_, err := rand.Read(seed)
	return seed, err
})

func randomPerTest(t *testing.T) *countingRandom {
	t.Helper()
	process, err := processSeed()
	if err != nil {
		t.Fatal(err)
	}
	seed := sha256.Sum256(append(append([]byte{}, process...), t.Name()...))
	return &countingRandom{counter: binary.BigEndian.Uint64(seed[:8])}
}

func tutorCodeSource(user domain.UserID) *countingRandom {
	return &countingRandom{counter: binary.BigEndian.Uint64(user[:8])}
}

type passwordAge int

const (
	currentHash passwordAge = iota
	staleHash
)

const (
	currentHashGeneration = 2
	staleHashGeneration   = 1
	pauseDeadline         = 10 * time.Second
)

type authHarness struct {
	server    *httptest.Server
	store     *repositories.Store
	clock     *clock.Shared
	hasher    *testhasher.Fake
	logs      *safeBuffer
	database  *testdb.Database
	sink      *mail.Sink
	worker    *worker.Worker
	objects   *storage.S3
	contested *contestedRow
	links     core.UnsubscribeTokens
	keys      core.Keys
	mailer    *testmail.Sender
	schedule  *schedule.Service
	jobs      *lessonJobs
	files     *files.Service
	purger    *accounts.Purger
	accounts  *accounts.Service
	tutoring  *tutoring.Service
	chat      *chat.Service
}

type lessonJobs struct {
	mutex   sync.Mutex
	current *schedule.Jobs
}

func (j *lessonJobs) use(jobs *schedule.Jobs) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.current = jobs
}

func (j *lessonJobs) run(job func(*schedule.Jobs, context.Context, time.Time) (int64, error)) func(context.Context, time.Time) (int64, error) {
	return func(ctx context.Context, now time.Time) (int64, error) {
		j.mutex.Lock()
		current := j.current
		j.mutex.Unlock()
		return job(current, ctx, now)
	}
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	return newAuthHarnessBehind(t, nil)
}

func newAuthHarnessBehind(t *testing.T, trusted []netip.Prefix) *authHarness {
	t.Helper()
	return newAuthHarnessServing(t, trusted, api.NewHandler)
}

func newAuthHarnessServing(t *testing.T, trusted []netip.Prefix, newHandler func(api.Options) (http.Handler, error)) *authHarness {
	t.Helper()
	return newAuthHarnessStoring(t, trusted, newHandler, func(local *teststorage.Storage) storage.Store { return local.Store })
}

type optionsHook func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database)

func newAuthHarnessStoring(
	t *testing.T, trusted []netip.Prefix, newHandler func(api.Options) (http.Handler, error), objectsFor func(*teststorage.Storage) storage.Store,
	hooks ...optionsHook,
) *authHarness {
	t.Helper()
	return newAuthHarnessWithDatabase(t, testdb.NewForLockContention(t), trusted, newHandler, objectsFor, hooks...)
}

func newAuthHarnessWithDatabase(
	t *testing.T, database *testdb.Database, trusted []netip.Prefix, newHandler func(api.Options) (http.Handler, error),
	objectsFor func(*teststorage.Storage) storage.Store, hooks ...optionsHook,
) *authHarness {
	t.Helper()
	random := randomPerTest(t)
	logs := &safeBuffer{}
	logger := slog.New(core.NewContextHandler(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), authStart); err != nil {
		t.Fatal(err)
	}
	hasher := testhasher.New(currentHashGeneration)
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store := repositories.NewStore(database.Pool)
	local := teststorage.New(t)
	objects := local.Store
	fileService := files.New(files.Dependencies{Store: store, Objects: objectsFor(local), Clock: shared, Random: random, Keys: keys, Logger: logger})
	fieldKey := sha256.Sum256([]byte("test-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), random)
	if err != nil {
		t.Fatal(err)
	}
	tutoringService := tutoring.New(tutoring.Dependencies{
		Store: store, Clock: shared, Random: random, Keys: keys, Cipher: cipher, PublicURL: publicOrigin, Logger: logger,
	})
	scheduleService := schedule.New(schedule.Dependencies{Store: store, Clock: shared, Random: random, Keys: keys, Logger: logger})
	service := accounts.New(accounts.Dependencies{
		Store: store, Clock: shared, Random: random, Hasher: hasher, Keys: keys, Logger: logger,
		Tokens: core.NewAccessTokens(keys, publicOrigin, 15*time.Minute), RefreshTTL: 30 * 24 * time.Hour,
		TutorQuotas: core.TutorQuotas{FileBytes: 5_000_000_000, RecordingBytes: 50_000_000_000}, Avatars: fileService, Uploads: fileService,
		Tutoring: tutoringService,
	})
	_, unsubscribeSecret, err := core.RandomToken(random, 32)
	if err != nil {
		t.Fatal(err)
	}
	links, err := core.NewUnsubscribeTokens(core.NewSecret(unsubscribeSecret))
	if err != nil {
		t.Fatal(err)
	}
	preferences := notifications.NewPreferences(notifications.PreferenceDependencies{Store: store, Clock: shared, Tokens: links, Keys: keys, Logger: logger})
	inbox := notifications.NewInbox(notifications.InboxDependencies{Store: store, Clock: shared, Logger: logger})
	chatService := chat.New(chat.Dependencies{
		Store: store, Clock: shared, Random: random, Addresses: fileService, Erasure: chat.DefaultErasureSettings(), Logger: logger,
	})
	options := api.Options{
		Logger: logger, Now: clock.System{}.Now, NewRequestID: core.NewRandomID,
		Readiness:   func(context.Context) error { return nil },
		Auth:        api.Auth{Accounts: service, PublicOrigin: publicOrigin, TrustedProxies: trusted},
		Files:       fileService,
		Tutoring:    tutoringService,
		Schedule:    scheduleService,
		Preferences: preferences,
		Inbox:       inbox,
		Chat:        chatService,
	}
	for _, hook := range hooks {
		hook(&options, shared, logger, database)
	}
	handler, err := newHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	sink := mail.NewSink(database.Pool, random)
	mailer := testmail.New(sink)
	accountEmails, err := notifications.NewAccountEmails(store, random, publicOrigin)
	if err != nil {
		t.Fatal(err)
	}
	relationshipEmails, err := notifications.NewRelationshipEmails(store, cipher, publicOrigin)
	if err != nil {
		t.Fatal(err)
	}
	lessonEmails, err := notifications.NewLessonEmails(store, links, publicOrigin)
	if err != nil {
		t.Fatal(err)
	}
	digestEmails, err := notifications.NewDigestEmails(store, links, publicOrigin)
	if err != nil {
		t.Fatal(err)
	}
	builders, err := notifications.Combine(accountEmails.Builders(), relationshipEmails.Builders(), lessonEmails.Builders(), digestEmails.Builders())
	if err != nil {
		t.Fatal(err)
	}
	notifier, err := notifications.New(mailer, publicOrigin, builders, notifications.Dispatch{Store: store, Keys: keys, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &lessonJobs{current: schedule.NewJobs(scheduleService, schedule.DefaultJobSettings())}
	lanes := []worker.Lane{
		{Name: domain.LaneEmail, Concurrency: 10, Perform: notifier.Deliver},
		{Name: domain.LaneStorageDelete, Concurrency: 4, Perform: fileService.DeleteObject},
	}
	purger := accounts.NewPurger(store, fileService, tutoringService, logger)
	scheduled := []worker.Job{
		{Name: domain.JobUploadsExpire, Every: time.Hour, Run: fileService.ExpirePending},
		{Name: domain.JobDailyUploadsCleanup, Every: time.Minute, Run: fileService.CleanupDailyUploads},
		{Name: domain.JobAccountsPurge, Every: time.Hour, Run: purger.PurgeDeleted},
		{Name: domain.JobAccountsCleanupUnverified, Every: time.Hour, Run: purger.CleanupUnverified},
		{Name: domain.JobSessionsCleanup, Every: time.Hour, Run: accounts.CleanupSessions(store)},
		{Name: domain.JobEmailRequestsCleanup, Every: time.Hour, Run: accounts.CleanupEmailRequests(store)},
		{Name: domain.JobInvitationsExpire, Every: time.Hour, Run: tutoringService.ExpireInvitations},
		{Name: domain.JobReclaimExpire, Every: time.Hour, Run: tutoringService.ExpireReclaims},
		{Name: domain.JobJoinRequestsExpire, Every: time.Hour, Run: tutoringService.ExpireJoinRequests},
		{Name: domain.JobNotificationsCleanup, Every: time.Hour, Run: inbox.Cleanup},
		{Name: domain.JobMessagesDigest, Every: time.Minute, Run: chatService.QueueDigests},
		{Name: domain.JobEraseChannels, Every: time.Minute, Run: chatService.EraseChannels},
		{Name: domain.JobLessonStatus, Every: time.Minute, Run: jobs.run((*schedule.Jobs).EndUnjoined)},
		{Name: domain.JobLessonReminders, Every: time.Minute, Run: jobs.run((*schedule.Jobs).QueueReminders)},
		{Name: domain.JobSeriesExtension, Every: time.Hour, OwnLoop: true, Run: jobs.run((*schedule.Jobs).ExtendHorizons)},
	}
	background := worker.New(logger, shared, outbox.New(store, random, logger), lanes, scheduled, worker.DefaultTiming())
	return &authHarness{
		server: server, store: store, clock: shared, hasher: hasher, logs: logs,
		database: database, sink: sink, worker: background, objects: objects, links: links,
		accounts: options.Auth.Accounts, tutoring: options.Tutoring, chat: options.Chat, keys: keys, mailer: mailer, schedule: scheduleService, jobs: jobs, files: fileService, purger: purger,
	}
}

func (h *authHarness) seedUser(t *testing.T, id byte, email string, password string, age passwordAge) domain.UserID {
	t.Helper()
	return h.seedUserAs(t, domain.RoleTutor, id, email, password, age)
}

func (h *authHarness) seedUserAs(t *testing.T, role domain.Role, id byte, email string, password string, age passwordAge) domain.UserID {
	t.Helper()
	hasher := h.hasher
	if age == staleHash {
		hasher = testhasher.New(staleHashGeneration)
	}
	encoded, err := hasher.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.UserID{id}
	err = h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: role, State: domain.AccountActive, Email: email, EmailVerifiedAt: &authStart,
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
	return user
}

type response struct {
	status  int
	body    string
	headers http.Header
}

func (h *authHarness) send(t *testing.T, method string, path string, body string, headers map[string]string) response {
	t.Helper()
	result, err := h.trySend(method, path, body, headers)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h *authHarness) sendHeaders(t *testing.T, method string, path string, body string, headers http.Header) response {
	t.Helper()
	result, err := h.trySendHeaders(method, path, body, headers)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h *authHarness) trySend(method string, path string, body string, headers map[string]string) (response, error) {
	all := http.Header{}
	for name, value := range headers {
		all.Set(name, value)
	}
	return h.trySendHeaders(method, path, body, all)
}

func (h *authHarness) trySendHeaders(method string, path string, body string, headers http.Header) (response, error) {
	request, err := http.NewRequestWithContext(context.Background(), method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	request.Header = headers
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return response{}, err
	}
	var text strings.Builder
	buffer := make([]byte, 4096)
	for {
		read, readErr := result.Body.Read(buffer)
		text.Write(buffer[:read])
		if readErr != nil {
			break
		}
	}
	return response{status: result.StatusCode, body: text.String(), headers: result.Header}, result.Body.Close()
}

func (h *authHarness) loginWhileVerifying(t *testing.T, email string, password string, during func()) response {
	t.Helper()
	pause := h.hasher.PauseAfterNextVerification()
	t.Cleanup(pause.Resume)
	type outcome struct {
		result response
		err    error
	}
	answered := make(chan outcome, 1)
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	go func() {
		result, err := h.trySend(http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": publicOrigin, "Content-Type": "application/json"})
		answered <- outcome{result: result, err: err}
	}()
	deadline := time.NewTimer(pauseDeadline)
	defer deadline.Stop()
	select {
	case <-pause.Reached():
	case <-deadline.C:
		t.Fatal("the sign-in never reached its password verification")
	}
	during()
	pause.Resume()
	select {
	case got := <-answered:
		if got.err != nil {
			t.Fatal(got.err)
		}
		return got.result
	case <-deadline.C:
		t.Fatal("the sign-in did not answer after its verification resumed")
	}
	return response{}
}

func (h *authHarness) replacePassword(t *testing.T, user domain.UserID, password string) {
	t.Helper()
	replacement, err := h.hasher.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	current, err := h.store.Repositories().Users.ByID(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Repositories().Users.UpdatePasswordHash(context.Background(), repositories.PasswordRehash{
		PasswordHash: replacement, Now: authStart, ID: user, CurrentPasswordHash: current.PasswordHash,
	}); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) storedHashIsCurrent(t *testing.T, stored string, password string) bool {
	t.Helper()
	verification, err := testhasher.New(currentHashGeneration).Verify(context.Background(), password, stored)
	return err == nil && verification.Matches && !verification.Stale
}

func (h *authHarness) quietLogin(email string, password string) (int, error) {
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+"/api/v1/auth/login", strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Origin", publicOrigin)
	request.Header.Set("Content-Type", "application/json")
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	return result.StatusCode, result.Body.Close()
}

func (h *authHarness) quietRefresh(cookie string) (int, string, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+"/api/v1/auth/refresh", nil)
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Origin", publicOrigin)
	request.Header.Set("Cookie", "__Host-ovc_refresh="+cookie)
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "", err
	}
	value := ""
	for _, set := range result.Cookies() {
		if set.Name == "__Host-ovc_refresh" {
			value = set.Value
		}
	}
	return result.StatusCode, value, result.Body.Close()
}

func (h *authHarness) login(t *testing.T, email string, password string) response {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	return h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": publicOrigin, "Content-Type": "application/json"})
}

func (h *authHarness) refresh(t *testing.T, cookie string) response {
	t.Helper()
	headers := map[string]string{"Origin": publicOrigin}
	if cookie != "" {
		headers["Cookie"] = "__Host-ovc_refresh=" + cookie
	}
	return h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", headers)
}

func (h *authHarness) me(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodGet, "/api/v1/users/me", "", map[string]string{"Authorization": "Bearer " + token})
}

func (h *authHarness) advance(t *testing.T, by time.Duration) {
	t.Helper()
	if _, err := h.clock.Advance(context.Background(), by); err != nil {
		t.Fatal(err)
	}
}

func accessToken(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.AccessToken == "" || decoded.ExpiresIn != 900 {
		t.Fatalf("no access token in %d %s", result.status, result.body)
	}
	return decoded.AccessToken
}

func refreshValue(t *testing.T, result response) string {
	t.Helper()
	for _, cookie := range (&http.Response{Header: result.headers}).Cookies() {
		if cookie.Name == "__Host-ovc_refresh" {
			return cookie.Value
		}
	}
	t.Fatalf("no refresh cookie in %v", result.headers)
	return ""
}

func code(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("not a structured error: %d %s", result.status, result.body)
	}
	return decoded.Error.Code
}

func TestLoginSetsExactlyTheHostOnlyRefreshCookie(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)

	result := h.login(t, "Tutor@Example.org ", "correct horse battery")

	cookies := result.headers.Values("Set-Cookie")
	if result.status != http.StatusOK || len(cookies) != 1 || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("login %d, cookies %v, cache %q", result.status, cookies, result.headers.Get("Cache-Control"))
	}
	for _, attribute := range []string{"__Host-ovc_refresh=", "Path=/", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(cookies[0], attribute) {
			t.Fatalf("cookie %q lacks %s", cookies[0], attribute)
		}
	}
	if strings.Contains(strings.ToLower(cookies[0]), "domain=") {
		t.Fatalf("cookie %q sets a domain", cookies[0])
	}
	if me := h.me(t, accessToken(t, result)); me.status != http.StatusOK || !strings.Contains(me.body, `"email":"tutor@example.org"`) {
		t.Fatalf("me %d %s", me.status, me.body)
	}
}

func TestLoginAnswersIdenticallyForAnUnknownEmailAWrongPasswordAndADeletedAccount(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	deleted := h.seedUser(t, 2, "deleted@example.org", "correct horse battery", currentHash)
	if marked, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil || !marked {
		t.Fatalf("mark deleted: %v %v", marked, err)
	}

	unknown := h.login(t, "nobody@example.org", "correct horse battery")
	wrong := h.login(t, tutorEmail, "wrong horse battery")
	gone := h.login(t, "deleted@example.org", "correct horse battery")

	if verified := h.hasher.Verifications(); verified != 3 {
		t.Fatalf("%d verifications; every path must hash once", verified)
	}
	for _, result := range []response{unknown, wrong, gone} {
		if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
			t.Fatalf("got %d %s", result.status, result.body)
		}
	}
	if strings.Contains(h.logs.String(), tutorEmail) || strings.Contains(h.logs.String(), "nobody@example.org") {
		t.Fatal("an email reached the logs")
	}
}

func TestLoginThrottlesTheSixthFailureWithin15Minutes(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.advance(t, 5*time.Minute+30*time.Second)
	for range 5 {
		if result := h.login(t, tutorEmail, "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("failure answered %d", result.status)
		}
	}

	sixth := h.login(t, tutorEmail, "wrong horse battery")
	correct := h.login(t, tutorEmail, "correct horse battery")
	h.advance(t, 9*time.Minute+30*time.Second)
	later := h.login(t, tutorEmail, "correct horse battery")

	if sixth.status != http.StatusTooManyRequests || code(t, sixth) != "rate_limited" || sixth.headers.Get("Retry-After") != "570" {
		t.Fatalf("sixth %d %s retry %q", sixth.status, sixth.body, sixth.headers.Get("Retry-After"))
	}
	if correct.status != http.StatusTooManyRequests || later.status != http.StatusOK {
		t.Fatalf("correct while throttled %d, after the window %d", correct.status, later.status)
	}
}

func TestParallelWrongPasswordsVerifyAtMostFivePasswords(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	statuses := make([]int, 20)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, err := h.quietLogin(tutorEmail, "wrong horse battery")
			if err != nil {
				t.Error(err)
			}
			statuses[index] = status
		})
	}
	wait.Wait()

	throttled := 0
	for _, status := range statuses {
		if status == http.StatusTooManyRequests {
			throttled++
		}
	}
	if verified := h.hasher.Verifications(); verified > 5 || throttled != 15 {
		t.Fatalf("%d password verifications and %d throttled of 20 attempts: %v", verified, throttled, statuses)
	}
}

func TestTheTwentyFirstFailureFromOneAddressIsThrottled(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	for attempt := range 20 {
		if result := h.login(t, fmt.Sprintf("person%d@example.org", attempt), "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d answered %d", attempt, result.status)
		}
	}

	result := h.login(t, "another@example.org", "wrong horse battery")

	if result.status != http.StatusTooManyRequests || !strings.Contains(h.logs.String(), `"scope":"address"`) {
		t.Fatalf("the 21st failure answered %d", result.status)
	}
	if verified := h.hasher.Verifications(); verified != 20 {
		t.Fatalf("%d verifications; the throttled attempt must not hash", verified)
	}
}

func TestBlockedAccountIsRevealedOnlyAfterTheCorrectPassword(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	if blocked, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil || !blocked {
		t.Fatalf("block: %v %v", blocked, err)
	}

	wrong := h.login(t, tutorEmail, "wrong horse battery")
	correct := h.login(t, tutorEmail, "correct horse battery")

	if wrong.status != http.StatusUnauthorized || code(t, wrong) != "invalid_credentials" {
		t.Fatalf("wrong password %d %s", wrong.status, wrong.body)
	}
	if correct.status != http.StatusForbidden || code(t, correct) != "account_blocked" {
		t.Fatalf("correct password %d %s", correct.status, correct.body)
	}
}

func TestLoginRehashesAPasswordWithStaleParameters(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", staleHash)

	result := h.login(t, tutorEmail, "correct horse battery")

	stored, err := h.store.Repositories().Users.ByID(context.Background(), user)
	if result.status != http.StatusOK || err != nil || !h.storedHashIsCurrent(t, stored.PasswordHash, "correct horse battery") {
		t.Fatalf("login %d, stored %q, %v", result.status, stored.PasswordHash, err)
	}
}

func TestParallelSignInsWithAStaleHashAllSucceed(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", staleHash)
	statuses := make([]int, 4)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, err := h.quietLogin(tutorEmail, "correct horse battery")
			if err != nil {
				t.Error(err)
			}
			statuses[index] = status
		})
	}
	wait.Wait()

	stored, err := h.store.Repositories().Users.ByID(context.Background(), user)
	for _, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("parallel sign-ins with a stale hash answered %v", statuses)
		}
	}
	if err != nil || !h.storedHashIsCurrent(t, stored.PasswordHash, "correct horse battery") {
		t.Fatalf("stored %q, %v", stored.PasswordHash, err)
	}
}

func TestRefreshReuseRevokesTheSession(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	first := h.login(t, tutorEmail, "correct horse battery")
	t0 := refreshValue(t, first)
	t1 := refreshValue(t, h.refresh(t, t0))
	second := h.refresh(t, t1)
	t2 := refreshValue(t, second)

	replay := h.refresh(t, t0)
	afterReplay := h.refresh(t, t2)
	me := h.me(t, accessToken(t, second))

	if replay.status != http.StatusUnauthorized || code(t, replay) != "unauthenticated" || !strings.Contains(replay.headers.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("replay %d %s %v", replay.status, replay.body, replay.headers)
	}
	if afterReplay.status != http.StatusUnauthorized || me.status != http.StatusUnauthorized {
		t.Fatalf("after reuse: refresh %d, me %d", afterReplay.status, me.status)
	}
	if !strings.Contains(h.logs.String(), `"msg":"auth.refresh.reuse_detected"`) {
		t.Fatal("no reuse record")
	}
}

func TestForgedStaleGenerationIsRefusedWithoutRevoking(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	current := refreshValue(t, h.refresh(t, refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))))
	parts := strings.Split(current, ".")
	forgeries := []string{
		strings.Join([]string{parts[0], "1", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "1", parts[2], strings.Repeat("A", 43)}, "."),
		parts[0] + ".1",
		"not-a-cookie",
	}

	var statuses []int
	for _, forged := range forgeries {
		statuses = append(statuses, h.refresh(t, forged).status)
	}
	still := h.refresh(t, current)

	for _, status := range statuses {
		if status != http.StatusUnauthorized {
			t.Fatalf("forgeries answered %v", statuses)
		}
	}
	if still.status != http.StatusOK || strings.Contains(h.logs.String(), "reuse_detected") {
		t.Fatalf("the session was revoked by a forgery: %d", still.status)
	}
}

func TestRefreshRejectsForeignOrigin(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))

	results := []response{
		h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"Origin": "https://video.ovc.example", "Cookie": "__Host-ovc_refresh=" + cookie}),
		h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"Cookie": "__Host-ovc_refresh=" + cookie}),
		h.sendHeaders(t, http.MethodPost, "/api/v1/auth/refresh", "", http.Header{
			"Origin": []string{publicOrigin, "https://video.ovc.example"}, "Cookie": []string{"__Host-ovc_refresh=" + cookie},
		}),
		h.sendHeaders(t, http.MethodPost, "/api/v1/auth/refresh", "", http.Header{
			"Origin": []string{publicOrigin, publicOrigin}, "Cookie": []string{"__Host-ovc_refresh=" + cookie},
		}),
	}

	for _, result := range results {
		if result.status != http.StatusForbidden || code(t, result) != "forbidden" {
			t.Fatalf("got %d %s", result.status, result.body)
		}
	}
	if h.refresh(t, cookie).status != http.StatusOK {
		t.Fatal("a refused origin consumed the cookie")
	}
}

func TestLoginRejectsForeignOriginAndMissingContentType(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	body := `{"email":"tutor@example.org","password":"correct horse battery"}`

	foreign := h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": "https://attacker.example", "Content-Type": "application/json"})
	foreignInvalid := h.send(t, http.MethodPost, "/api/v1/auth/login", "{", map[string]string{"Origin": "https://attacker.example", "Content-Type": "application/json"})
	untyped := h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": publicOrigin})
	twoOrigins := h.sendHeaders(t, http.MethodPost, "/api/v1/auth/login", body, http.Header{
		"Origin": []string{publicOrigin, "https://attacker.example"}, "Content-Type": []string{"application/json"},
	})

	if foreign.status != http.StatusForbidden || foreignInvalid.status != http.StatusForbidden || untyped.status != http.StatusUnsupportedMediaType {
		t.Fatalf("foreign %d, untyped %d %s", foreign.status, untyped.status, untyped.body)
	}
	if twoOrigins.status != http.StatusForbidden || code(t, twoOrigins) != "forbidden" {
		t.Fatalf("two origins answered %d %s", twoOrigins.status, twoOrigins.body)
	}
}

func TestRefreshWithoutCookieAnswers204(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)

	result := h.refresh(t, "")

	if result.status != http.StatusNoContent || result.body != "" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("got %d %q %v", result.status, result.body, result.headers)
	}
}

func TestAccessTokenExpiresByTheSharedClock(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	h.advance(t, 14*time.Minute)
	before := h.me(t, token)
	h.advance(t, 2*time.Minute)
	after := h.me(t, token)

	if before.status != http.StatusOK || after.status != http.StatusUnauthorized || code(t, after) != "token_expired" {
		t.Fatalf("before %d, after %d %s", before.status, after.status, after.body)
	}
}

func TestLogoutRevokesTheSessionAndLogoutAllEverySession(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	first := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	second := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	third := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	logout := h.send(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + first})
	afterLogout := []int{h.me(t, first).status, h.me(t, second).status}
	logoutAll := h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", map[string]string{"Authorization": "Bearer " + second})
	afterAll := []int{h.me(t, second).status, h.me(t, third).status}

	if logout.status != http.StatusNoContent || !strings.Contains(logout.headers.Get("Set-Cookie"), "Max-Age=0") || logoutAll.status != http.StatusNoContent {
		t.Fatalf("logout %d %v, logout-all %d", logout.status, logout.headers, logoutAll.status)
	}
	if afterLogout[0] != http.StatusUnauthorized || afterLogout[1] != http.StatusOK || afterAll[0] != http.StatusUnauthorized || afterAll[1] != http.StatusUnauthorized {
		t.Fatalf("after logout %v, after logout-all %v", afterLogout, afterAll)
	}
}

func TestEveryOperationDeclaresAnAudience(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	document, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(document, &published); err != nil {
		t.Fatal(err)
	}

	operations := api.Operations()

	declared := map[string]api.Audience{}
	for _, operation := range operations {
		declared[strings.ToUpper(operation.Method)+" "+operation.Path] = operation.Audience
	}
	for path, methods := range published.Paths {
		for method := range methods {
			if _, found := declared[strings.ToUpper(method)+" "+path]; !found {
				t.Fatalf("%s %s is published without a declared audience", method, path)
			}
		}
	}
	for _, operation := range operations {
		switch operation.Audience {
		case api.AudiencePublic, api.AudienceRefreshCookie:
		case api.AudienceOptionalUser:
			result := h.send(t, operation.Method, operation.Path, "{}", map[string]string{
				"Authorization": "Bearer forged", "Content-Type": "application/json", "Origin": publicOrigin,
			})
			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
				t.Fatalf("%s %s with a forged token answered %d %s", operation.Method, operation.Path, result.status, result.body)
			}
		case api.AudienceFirstFrame:
			result := h.send(t, operation.Method, operation.Path, "", nil)
			if result.status != http.StatusForbidden || code(t, result) != "forbidden" {
				t.Fatalf("%s %s without an origin answered %d %s", operation.Method, operation.Path, result.status, result.body)
			}
		case api.AudienceUser, api.AudienceUnverifiedOK, api.AudienceTutor, api.AudienceTutorUnverifiedOK, api.AudienceStudent, api.AudienceStudentUnverifiedOK:
			result := h.send(t, operation.Method, operation.Path, "", nil)
			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" || result.headers.Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("%s %s without a token answered %d %s", operation.Method, operation.Path, result.status, result.body)
			}
		default:
			t.Fatalf("%s declares no known audience: %q", operation.ID, operation.Audience)
		}
	}
}

func TestRefreshExpiresThirtyDaysAfterSignInWhateverTheRotations(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))
	h.advance(t, 29*24*time.Hour)
	rotated := h.refresh(t, cookie)

	h.advance(t, 24*time.Hour)
	expired := h.refresh(t, refreshValue(t, rotated))

	if rotated.status != http.StatusOK || !strings.Contains(rotated.headers.Get("Set-Cookie"), "Max-Age=86400") {
		t.Fatalf("rotation on day 29: %d %v", rotated.status, rotated.headers.Values("Set-Cookie"))
	}
	if expired.status != http.StatusUnauthorized {
		t.Fatalf("refresh on day 30 answered %d", expired.status)
	}
}

func TestBlockedAccountLosesRefreshAndAccess(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	signedIn := h.login(t, tutorEmail, "correct horse battery")
	if _, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil {
		t.Fatal(err)
	}

	refresh := h.refresh(t, refreshValue(t, signedIn))
	me := h.me(t, accessToken(t, signedIn))

	if refresh.status != http.StatusForbidden || code(t, refresh) != "account_blocked" || !strings.Contains(refresh.headers.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("refresh %d %s %v", refresh.status, refresh.body, refresh.headers)
	}
	if me.status != http.StatusForbidden || code(t, me) != "account_blocked" {
		t.Fatalf("me %d %s", me.status, me.body)
	}
}

func TestConcurrentRefreshesWithOneCookieRotateOnceAndRevokeAsReuse(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))
	statuses := make([]int, 2)
	cookies := make([]string, 2)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, value, err := h.quietRefresh(cookie)
			if err != nil {
				t.Error(err)
			}
			statuses[index], cookies[index] = status, value
		})
	}
	wait.Wait()

	winner := -1
	for index, status := range statuses {
		if status == http.StatusOK {
			winner = index
		}
	}
	if winner < 0 || statuses[1-winner] != http.StatusUnauthorized {
		t.Fatalf("concurrent refreshes answered %v", statuses)
	}
	if after := h.refresh(t, cookies[winner]); after.status != http.StatusUnauthorized {
		t.Fatalf("the session survived a reused cookie: %d", after.status)
	}
}

func TestAccountFlowsLogNoSecretsOrEmails(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	signedIn := h.login(t, tutorEmail, "correct horse battery")
	token := accessToken(t, signedIn)
	cookie := refreshValue(t, signedIn)

	rotated := refreshValue(t, h.refresh(t, cookie))
	h.refresh(t, cookie)
	h.login(t, tutorEmail, "wrong horse battery")
	h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", map[string]string{"Authorization": "Bearer " + token})

	logs := h.logs.String()
	for _, secret := range []string{tutorEmail, "correct horse battery", "wrong horse battery", token, cookie, rotated, strings.Split(cookie, ".")[2]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain %q", secret)
		}
	}
	for _, event := range []string{"auth.login.succeeded", "auth.refresh.reuse_detected", "auth.login.failed"} {
		if !strings.Contains(logs, `"msg":"`+event+`"`) {
			t.Fatalf("no %s record", event)
		}
	}
}

func TestClientAddressWalksForwardedHeadersFromTheRightPastTrustedProxies(t *testing.T) {
	t.Parallel()
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32")}
	cases := map[string]struct {
		remote    string
		forwarded []string
		want      string
	}{
		"direct client":               {remote: "203.0.113.5:4000", forwarded: []string{"198.51.100.1"}, want: "203.0.113.5"},
		"one trusted proxy":           {remote: "10.0.0.2:4000", forwarded: []string{"198.51.100.1"}, want: "198.51.100.1"},
		"spoofed left entry":          {remote: "10.0.0.2:4000", forwarded: []string{"1.2.3.4, 198.51.100.1"}, want: "198.51.100.1"},
		"chain of trusted proxies":    {remote: "127.0.0.1:4000", forwarded: []string{"198.51.100.1, 10.0.0.9"}, want: "198.51.100.1"},
		"several header lines":        {remote: "10.0.0.2:4000", forwarded: []string{"1.2.3.4", "198.51.100.1, 10.0.0.3"}, want: "198.51.100.1"},
		"unparsable entry":            {remote: "10.0.0.2:4000", forwarded: []string{"198.51.100.1, garbage"}, want: "10.0.0.2"},
		"trusted peer without header": {remote: "10.0.0.2:4000", want: "10.0.0.2"},
		"mapped IPv4 peer":            {remote: "[::ffff:203.0.113.5]:4000", want: "203.0.113.5"},
		"unparsable peer":             {remote: "not-an-address", want: "invalid IP"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got, known := api.ClientAddress(testCase.remote, testCase.forwarded, trusted)

			if got.String() != testCase.want || known != (testCase.want != "invalid IP") {
				t.Fatalf("got %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestEveryPublishedOperationDescribesTheErrorEnvelope(t *testing.T) {
	t.Parallel()
	document, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]json.RawMessage `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}

	decodeErr := json.Unmarshal(document, &published)

	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	for path, methods := range published.Paths {
		for method, operation := range methods {
			described := false
			for status, response := range operation.Responses {
				if (status == "default" || strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5")) && response.Content["application/json"] != nil {
					described = true
				}
			}
			if !described {
				t.Errorf("%s %s publishes no error response", method, path)
			}
		}
	}
}

func TestAThrottledAddressDoesNotLockTheVictimOutElsewhere(t *testing.T) {
	t.Parallel()
	h := newAuthHarnessBehind(t, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	from := func(address string, email string, password string) response {
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		return h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{
			"Origin": publicOrigin, "Content-Type": "application/json", "X-Forwarded-For": address,
		})
	}
	for attempt := range 20 {
		from("198.51.100.7", fmt.Sprintf("person%d@example.org", attempt), "wrong horse battery")
	}

	var attackerStatuses []int
	for range 6 {
		attackerStatuses = append(attackerStatuses, from("198.51.100.7", tutorEmail, "wrong horse battery").status)
	}
	victim := from("203.0.113.9", tutorEmail, "correct horse battery")

	for _, status := range attackerStatuses {
		if status != http.StatusTooManyRequests {
			t.Fatalf("the exhausted address answered %v", attackerStatuses)
		}
	}
	if victim.status != http.StatusOK {
		t.Fatalf("the victim from another address answered %d %s", victim.status, victim.body)
	}
}

func TestAPasswordChangedDuringVerificationStartsNoSession(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)

	result := h.loginWhileVerifying(t, tutorEmail, "correct horse battery", func() {
		h.replacePassword(t, user, "a brand new password")
	})

	if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a sign-in raced by a password change answered %d %s", result.status, result.body)
	}
	if !strings.Contains(h.logs.String(), `"reason":"changed"`) {
		t.Fatal("no changed record")
	}
}

func TestAStaleHashRehashLostToAPasswordChangeStartsNoSession(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", staleHash)

	result := h.loginWhileVerifying(t, tutorEmail, "correct horse battery", func() {
		h.replacePassword(t, user, "a brand new password")
	})

	if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a stale sign-in raced by a password change answered %d %s", result.status, result.body)
	}
	if verified := h.hasher.Verifications(); verified != 2 {
		t.Fatalf("%d password verifications, want the second against the replaced hash", verified)
	}
	if !strings.Contains(h.logs.String(), `"reason":"changed"`) {
		t.Fatal("no changed record")
	}
}

func TestABlockDuringVerificationStartsNoSession(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)

	result := h.loginWhileVerifying(t, tutorEmail, "correct horse battery", func() {
		if _, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil {
			t.Fatal(err)
		}
	})

	if result.status != http.StatusForbidden || code(t, result) != "account_blocked" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a sign-in raced by a block answered %d %s", result.status, result.body)
	}
	if !strings.Contains(h.logs.String(), `"reason":"blocked"`) {
		t.Fatal("no blocked record")
	}
}

func TestBusyHashingDoesNotSpendSignInAttempts(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	h.hasher.FailVerifications(core.ErrHashingBusy)
	var statuses []int
	for range 6 {
		statuses = append(statuses, h.login(t, tutorEmail, "correct horse battery").status)
	}

	h.hasher.FailVerifications(nil)
	result := h.login(t, tutorEmail, "correct horse battery")

	for _, status := range statuses {
		if status != http.StatusServiceUnavailable {
			t.Fatalf("busy hashing answered %v", statuses)
		}
	}
	if result.status != http.StatusOK {
		t.Fatalf("after busy hashing the sign-in answered %d %s", result.status, result.body)
	}
}

func TestAttemptsRefusedByALimitDoNotSpendTheAddressBudget(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	for range 5 {
		if result := h.login(t, "guessed@example.org", "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("a failure answered %d", result.status)
		}
	}
	for range 15 {
		if result := h.login(t, "guessed@example.org", "wrong horse battery"); result.status != http.StatusTooManyRequests {
			t.Fatalf("an attempt over the email limit answered %d", result.status)
		}
	}

	result := h.login(t, tutorEmail, "correct horse battery")

	if result.status != http.StatusOK {
		t.Fatalf("a correct sign-in from the same address answered %d after only 5 failures", result.status)
	}
}

func TestAnAttemptRefusedByTheAddressLimitIsNotCounted(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	for attempt := range 20 {
		if result := h.login(t, fmt.Sprintf("person%d@example.org", attempt), "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d answered %d", attempt, result.status)
		}
	}

	refused := h.login(t, "another@example.org", "wrong horse battery")

	if refused.status != http.StatusTooManyRequests {
		t.Fatalf("the 21st attempt answered %d", refused.status)
	}
	var counted int64
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT max(count) FROM rate_limit_counters WHERE purpose = 'sign_in_address'").Scan(&counted); err != nil {
		t.Fatal(err)
	}
	if counted != 20 {
		t.Fatalf("the address counter holds %d, want the 20 failures only", counted)
	}
}

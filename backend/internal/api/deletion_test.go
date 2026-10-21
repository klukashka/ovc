package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (h *authHarness) deleteMe(t *testing.T, token string, password string) response {
	t.Helper()
	return h.send(t, http.MethodDelete, "/api/v1/users/me", fmt.Sprintf(`{"password":%q}`, password), map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": "application/json",
	})
}

func (h *authHarness) rowsOwnedBy(t *testing.T, user domain.UserID) map[string]int64 {
	t.Helper()
	queries := map[string]string{
		"users":                 "SELECT count(id) FROM users WHERE id = $1::uuid",
		"sessions":              "SELECT count(id) FROM sessions WHERE user_id = $1::uuid",
		"email_requests":        "SELECT count(id) FROM email_requests WHERE user_id = $1::uuid",
		"email links":           "SELECT count(id) FROM email_tokens WHERE user_id = $1::uuid",
		"tutor_profiles":        "SELECT count(user_id) FROM tutor_profiles WHERE user_id = $1::uuid",
		"attachments":           "SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid",
		"attachment_references": "SELECT count(attachment_id) FROM attachment_references WHERE ref_id = $1::uuid",
	}
	owned := map[string]int64{}
	for table, query := range queries {
		if count := h.countRows(t, query, user.String()); count > 0 {
			owned[table] = count
		}
	}
	return owned
}

func TestDeleteAccountRequiresThePasswordAndSignsOutEverywhere(t *testing.T) {
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	here := h.login(t, tutorEmail, "correct horse battery")
	elsewhere := h.login(t, tutorEmail, "correct horse battery")

	wrong := h.deleteMe(t, accessToken(t, here), "wrong horse battery")
	deleted := h.deleteMe(t, accessToken(t, here), "correct horse battery")
	elsewhereRefresh := h.refresh(t, refreshValue(t, elsewhere))
	signIn := h.login(t, tutorEmail, "correct horse battery")
	me := h.me(t, accessToken(t, here))

	if wrong.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, wrong), ","), "body.password:incorrect") {
		t.Fatalf("a wrong password answered %d %s", wrong.status, wrong.body)
	}
	if deleted.status != http.StatusNoContent || !strings.Contains(deleted.headers.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("deletion answered %d %v", deleted.status, deleted.headers)
	}
	if elsewhereRefresh.status != http.StatusUnauthorized || signIn.status != http.StatusUnauthorized || code(t, signIn) != "invalid_credentials" || me.status != http.StatusUnauthorized {
		t.Fatalf("after deletion: refresh %d, sign-in %d %s, me %d", elsewhereRefresh.status, signIn.status, signIn.body, me.status)
	}
}

func TestDeletingAnUnverifiedAccountPurgesItAtOnceAndFreesTheEmail(t *testing.T) {
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	user := h.userID(t, newcomerEmail)
	token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))

	deleted := h.deleteMe(t, token, newcomerPassword)
	h.at(t, authStart.Add(61*time.Second))
	again := h.register(t, tutorSignUp(newcomerEmail))
	h.tick(t)

	if deleted.status != http.StatusNoContent || len(h.rowsOwnedBy(t, user)) != 0 {
		t.Fatalf("deletion answered %d and left %v", deleted.status, h.rowsOwnedBy(t, user))
	}
	if again.status != http.StatusAccepted || h.userID(t, newcomerEmail) == user {
		t.Fatalf("signing up again answered %d", again.status)
	}
	if got := subjects(h.mailTo(t, newcomerEmail)); len(got) != 1 || got[0] != confirmSubject {
		t.Fatalf("the email received %v", got)
	}
}

func TestPurgeRemovesAccountRowsAndAvatarAfter30Days(t *testing.T) {
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	user := h.userID(t, tutorEmail)
	neighbour := h.seedUser(t, 2, "neighbour@example.org", "correct horse battery", cheapArgon2())
	neighbourToken := accessToken(t, h.login(t, "neighbour@example.org", "correct horse battery"))
	neighbourAvatar := uploadedID(t, h.upload(t, neighbourToken, "n.png", pngBytes(t, 2, 2)))
	h.patchMe(t, neighbourToken, fmt.Sprintf(`{"avatar_id":%q}`, neighbourAvatar))
	avatar := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
	pending := uploadedID(t, h.upload(t, token, "next.png", pngBytes(t, 2, 2)))
	keys := []string{h.storageKey(t, avatar), h.storageKey(t, pending)}
	h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, avatar))
	h.forgot(t, tutorEmail)
	if deleted := h.deleteMe(t, token, "correct horse battery"); deleted.status != http.StatusNoContent {
		t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
	}

	h.at(t, authStart.Add(30*24*time.Hour-time.Hour))
	h.tick(t)
	keptBeforeThirtyDays := h.rowsOwnedBy(t, user)
	h.at(t, authStart.Add(30*24*time.Hour))
	h.tick(t)

	if keptBeforeThirtyDays["users"] != 1 {
		t.Fatalf("the account was purged before 30 days: %v", keptBeforeThirtyDays)
	}
	if owned := h.rowsOwnedBy(t, user); len(owned) != 0 {
		t.Fatalf("rows of the purged account remain: %v", owned)
	}
	for _, key := range keys {
		if h.objectExists(t, key) {
			t.Fatal("an object of the purged account remains")
		}
	}
	if served := h.send(t, http.MethodGet, "/api/v1/avatars/"+avatar, "", nil); served.status != http.StatusNotFound {
		t.Fatalf("the purged avatar answered %d", served.status)
	}
	for _, message := range h.mailTo(t, tutorEmail) {
		if message.Subject == resetSubject {
			t.Fatal("a reset email was sent after the account was deleted")
		}
	}
	if owned := h.rowsOwnedBy(t, neighbour); owned["users"] != 1 || owned["attachments"] != 1 || owned["tutor_profiles"] != 1 {
		t.Fatalf("the purge touched another account: %v", owned)
	}
	if served := h.send(t, http.MethodGet, "/api/v1/avatars/"+neighbourAvatar, "", nil); served.status != http.StatusOK {
		t.Fatalf("another account's avatar answered %d", served.status)
	}
}

func TestUnverifiedAccountIsRemovedAfter7Days(t *testing.T) {
	h := newFlowHarness(t)
	h.register(t, tutorSignUp(newcomerEmail))
	user := h.userID(t, newcomerEmail)
	h.tick(t)

	h.at(t, authStart.Add(7*24*time.Hour-time.Hour))
	h.tick(t)
	keptBeforeSevenDays := h.rowsOwnedBy(t, user)
	h.at(t, authStart.Add(7*24*time.Hour))
	h.tick(t)

	if keptBeforeSevenDays["users"] != 1 {
		t.Fatalf("the account was removed before 7 days: %v", keptBeforeSevenDays)
	}
	if owned := h.rowsOwnedBy(t, user); len(owned) != 0 {
		t.Fatalf("rows of the removed account remain: %v", owned)
	}
}

func TestAConfirmedAccountIsNotRemovedAsUnverified(t *testing.T) {
	h := newFlowHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())

	h.at(t, authStart.Add(8*24*time.Hour))
	h.tick(t)

	if user, err := h.store.Repositories().Users.ByEmail(context.Background(), tutorEmail); err != nil || user.State != domain.AccountActive {
		t.Fatalf("the active account was touched: %+v %v", user, err)
	}
}

func TestDeletionIsThrottledAfterFiveWrongPasswords(t *testing.T) {
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	for range 5 {
		h.deleteMe(t, token, "wrong horse battery")
	}

	throttled := h.deleteMe(t, token, "correct horse battery")

	if throttled.status != http.StatusTooManyRequests || throttled.headers.Get("Retry-After") == "" {
		t.Fatalf("the sixth attempt answered %d %s", throttled.status, throttled.body)
	}
	if user, err := h.store.Repositories().Users.ByEmail(context.Background(), tutorEmail); err != nil || user.State != domain.AccountActive {
		t.Fatalf("the throttled deletion changed the account: %+v %v", user, err)
	}
}

func (h *authHarness) seedUnverifiedTutor(t *testing.T, user domain.UserID, email string, passwordHash string, created time.Time) {
	t.Helper()
	err := h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleTutor, State: domain.AccountUnverified, Email: email,
		PasswordHash: passwordHash, Name: "A language tutor", Language: domain.LanguageEnglish, Timezone: "Europe/Moscow",
		Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: created, CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = h.store.Repositories().TutorProfiles.Create(context.Background(), repositories.NewTutorProfile{
		UserID: user, FileQuotaBytes: 5_000_000_000, RecordingQuotaBytes: 50_000_000_000, CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) chargeAnUploadTo(t *testing.T, uploader domain.UserID, tutor domain.UserID, index int) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(),
		`INSERT INTO attachments (id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type, size_bytes, sha256, state, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1::uuid, 'chat', $2::uuid, $3, 'notes.png', 'image/png', 1, $4, 'bound', $5, $5)`,
		uploader.String(), tutor.String(), fmt.Sprintf("chat/charged-%d", index), make([]byte, 32), authStart)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAPurgeThatCannotRemoveSomeAccountsStillRemovesTheOnesBehindThem(t *testing.T) {
	h := newFlowHarness(t)
	uploader := h.seedUser(t, 250, "uploader@example.org", "correct horse battery", cheapArgon2())
	passwordHash, err := h.hasher.Hash(context.Background(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	var stuck []domain.UserID
	for index := range 100 {
		user := domain.UserID{1, byte(index)}
		h.seedUnverifiedTutor(t, user, fmt.Sprintf("stuck-%d@example.org", index), passwordHash, authStart)
		h.chargeAnUploadTo(t, uploader, user, index)
		stuck = append(stuck, user)
	}
	behind := domain.UserID{2}
	h.seedUnverifiedTutor(t, behind, "behind@example.org", passwordHash, authStart.Add(time.Hour))

	h.at(t, authStart.Add(7*24*time.Hour+2*time.Hour))
	h.tick(t)

	if owned := h.rowsOwnedBy(t, behind); len(owned) != 0 {
		t.Fatalf("the account behind the stuck ones was not removed: %v", owned)
	}
	if owned := h.rowsOwnedBy(t, stuck[0]); owned["users"] != 1 {
		t.Fatalf("a stuck account vanished: %v", owned)
	}
	if strings.Count(h.logs.String(), `"msg":"account.purge_failed"`) != len(stuck) {
		t.Fatalf("each stuck account should be reported once")
	}
}

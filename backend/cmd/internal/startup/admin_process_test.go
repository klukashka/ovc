package startup_test

import (
	"context"
	"errors"
	"os/user"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

var adminStart = time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)

type adminFixture struct {
	database *testdb.Database
	store    *repositories.Store
	hash     string
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	database := testdb.New(t)
	hasher, err := core.NewPasswordHasher(core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, core.CryptoRandom(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := hasher.Hash(context.Background(), "operator test password")
	if err != nil {
		t.Fatal(err)
	}
	return &adminFixture{database: database, store: repositories.NewStore(database.Pool), hash: encoded}
}

func (f *adminFixture) seed(t *testing.T, id byte, role domain.Role, state domain.AccountState, email string) domain.UserID {
	t.Helper()
	user := domain.UserID{id}
	var verified *time.Time
	if state == domain.AccountActive {
		verified = &adminStart
	}
	err := f.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: role, State: state, Email: email, EmailVerifiedAt: verified, PasswordHash: f.hash, Name: "An account",
		Language: domain.LanguageEnglish, Timezone: "UTC", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: adminStart, CreatedAt: adminStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if role == domain.RoleTutor {
		err = f.store.Repositories().TutorProfiles.Create(context.Background(), repositories.NewTutorProfile{
			UserID: user, FileQuotaBytes: 1, RecordingQuotaBytes: 1, CreatedAt: adminStart,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return user
}

func (f *adminFixture) admin(t *testing.T, args ...string) outcome {
	t.Helper()
	return runBinary(t, "ovc-admin", []string{"OVC_DATABASE_URL=" + f.database.URL.Reveal()}, args...)
}

func (f *adminFixture) user(t *testing.T, id domain.UserID) repositories.User {
	t.Helper()
	found, err := f.store.Repositories().Users.ByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestOvcAdminList(t *testing.T) {
	f := newAdminFixture(t)
	f.seed(t, 1, domain.RoleTutor, domain.AccountActive, "a_b@example.org")
	f.seed(t, 2, domain.RoleTutor, domain.AccountActive, "axb@example.org")
	f.seed(t, 3, domain.RoleStudent, domain.AccountUnverified, "student@example.org")
	f.seed(t, 4, domain.RoleTutor, domain.AccountActive, "100%sure@example.org")
	f.seed(t, 5, domain.RoleTutor, domain.AccountActive, "zab@example.org")

	underscore := f.admin(t, "users", "list", "--email", "A_B")
	percent := f.admin(t, "users", "list", "--email", "0%s")
	backslash := f.admin(t, "users", "list", "--email", `a\b`)
	students := f.admin(t, "users", "list", "--role", "student", "--state", "unverified")
	unknownState := f.admin(t, "users", "list", "--state", "sleeping")

	if underscore.exitCode != 0 || strings.Count(underscore.stdout, "\n") != 1 || !strings.Contains(underscore.stdout, "a_b@example.org") {
		t.Fatalf("an email filter with an underscore printed %q (exit %d, %s)", underscore.stdout, underscore.exitCode, underscore.stderr)
	}
	if students.exitCode != 0 || strings.Count(students.stdout, "\n") != 1 || !strings.Contains(students.stdout, "student\tunverified\tstudent@example.org") {
		t.Fatalf("the role and state filters printed %q", students.stdout)
	}
	if percent.exitCode != 0 || strings.Count(percent.stdout, "\n") != 1 || !strings.Contains(percent.stdout, "100%sure@example.org") {
		t.Fatalf("an email filter with a percent sign printed %q", percent.stdout)
	}
	if backslash.exitCode != 0 || backslash.stdout != "" {
		t.Fatalf("an email filter with a backslash printed %q", backslash.stdout)
	}
	if unknownState.exitCode != 2 {
		t.Fatalf("an unknown state exited %d", unknownState.exitCode)
	}
}

func TestOvcAdminBlock(t *testing.T) {
	f := newAdminFixture(t)
	tutor := f.seed(t, 1, domain.RoleTutor, domain.AccountActive, "tutor@example.org")
	deleted := f.seed(t, 2, domain.RoleTutor, domain.AccountActive, "gone@example.org")
	if _, err := f.store.Repositories().Users.MarkDeleted(context.Background(), deleted, adminStart); err != nil {
		t.Fatal(err)
	}
	session := domain.SessionID{9}
	err := f.store.Repositories().Sessions.Create(context.Background(), repositories.NewSession{
		ID: session, UserID: tutor, RefreshHash: make([]byte, 32), ExpiresAt: adminStart.Add(time.Hour), CreatedAt: adminStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	repos := f.store.Repositories()
	request := domain.EmailRequestID{3}
	if err := repos.EmailTokens.CreateRequest(context.Background(), repositories.NewEmailRequest{ID: request, UserID: tutor, Purpose: domain.EmailPurposeReset, CreatedAt: adminStart}); err != nil {
		t.Fatal(err)
	}
	err = repos.EmailTokens.Create(context.Background(), repositories.NewEmailToken{
		ID: domain.EmailTokenID{4}, RequestID: request, UserID: tutor, Purpose: domain.EmailPurposeReset, TokenHash: make([]byte, 32),
		ExpiresAt: adminStart.Add(time.Hour), CreatedAt: adminStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	blocked := f.admin(t, "users", "block", "Tutor@Example.org")
	refused := f.admin(t, "users", "block", "gone@example.org")

	stored, err := f.store.Repositories().Sessions.ByID(context.Background(), session)
	if blocked.exitCode != 0 || f.user(t, tutor).State != domain.AccountBlocked || err != nil || stored.RevokedAt == nil {
		t.Fatalf("block exited %d (%s); session %+v %v", blocked.exitCode, blocked.stderr, stored, err)
	}
	if _, err := repos.EmailTokens.Request(context.Background(), request); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("the reset request survived the block: %v", err)
	}
	if !strings.Contains(blocked.stderr, `"msg":"admin.command"`) || !strings.Contains(blocked.stderr, `"operator":"`+operator.Username+`"`) ||
		!strings.Contains(blocked.stderr, `"operator_uid":"`+operator.Uid+`"`) || strings.Contains(blocked.stderr, "tutor@example.org") {
		t.Fatalf("the audit record is wrong: %s", blocked.stderr)
	}
	if refused.exitCode != 1 || f.user(t, deleted).State != domain.AccountDeleted || !strings.Contains(refused.stderr, `"outcome":"refused"`) {
		t.Fatalf("blocking a deleted account exited %d: %s", refused.exitCode, refused.stderr)
	}
}

func TestOvcAdminUnblock(t *testing.T) {
	f := newAdminFixture(t)
	unverified := f.seed(t, 1, domain.RoleTutor, domain.AccountUnverified, "new@example.org")
	active := f.seed(t, 2, domain.RoleStudent, domain.AccountActive, "student@example.org")
	f.admin(t, "users", "block", "new@example.org")
	f.admin(t, "users", "block", "student@example.org")
	before := clock.System{}.Now()

	first := f.admin(t, "users", "unblock", "new@example.org")
	second := f.admin(t, "users", "unblock", "student@example.org")
	again := f.admin(t, "users", "unblock", "student@example.org")

	restoredUnverified, restoredActive := f.user(t, unverified), f.user(t, active)
	if first.exitCode != 0 || restoredUnverified.State != domain.AccountUnverified || restoredUnverified.UnverifiedSince.Before(before) {
		t.Fatalf("unblocking an unverified account: exit %d, %+v", first.exitCode, restoredUnverified)
	}
	if second.exitCode != 0 || restoredActive.State != domain.AccountActive || restoredActive.StateBeforeBlock != nil {
		t.Fatalf("unblocking an active account: exit %d, %+v", second.exitCode, restoredActive)
	}
	if again.exitCode != 1 {
		t.Fatalf("unblocking an account that is not blocked exited %d", again.exitCode)
	}
}

func TestOvcAdminRestore(t *testing.T) {
	f := newAdminFixture(t)
	deleted := f.seed(t, 1, domain.RoleTutor, domain.AccountActive, "tutor@example.org")
	neverConfirmed := f.seed(t, 2, domain.RoleTutor, domain.AccountUnverified, "new@example.org")
	for _, id := range []domain.UserID{deleted, neverConfirmed} {
		if _, err := f.store.Repositories().Users.MarkDeleted(context.Background(), id, adminStart); err != nil {
			t.Fatal(err)
		}
	}

	restored := f.admin(t, "users", "restore", "tutor@example.org")
	again := f.admin(t, "users", "restore", "tutor@example.org")
	unconfirmed := f.admin(t, "users", "restore", "new@example.org")

	account := f.user(t, deleted)
	if restored.exitCode != 0 || account.State != domain.AccountActive || account.DeletedAt != nil {
		t.Fatalf("restore exited %d (%s), account %+v", restored.exitCode, restored.stderr, account)
	}
	if again.exitCode != 1 || unconfirmed.exitCode != 1 || f.user(t, neverConfirmed).State != domain.AccountDeleted {
		t.Fatalf("restoring an active account exited %d and a never-confirmed one %d", again.exitCode, unconfirmed.exitCode)
	}
}

func TestOvcAdminSetQuota(t *testing.T) {
	f := newAdminFixture(t)
	tutor := f.seed(t, 1, domain.RoleTutor, domain.AccountActive, "tutor@example.org")
	f.seed(t, 2, domain.RoleStudent, domain.AccountActive, "student@example.org")

	set := f.admin(t, "tutors", "set-quota", "tutor@example.org", "--files", "7000000000", "--recordings", "80000000000")
	student := f.admin(t, "tutors", "set-quota", "student@example.org", "--files", "1", "--recordings", "1")
	negative := f.admin(t, "tutors", "set-quota", "tutor@example.org", "--files", "-1", "--recordings", "1")

	profile, err := f.store.Repositories().TutorProfiles.ByUser(context.Background(), tutor)
	if set.exitCode != 0 || err != nil || profile.FileQuotaBytes != 7_000_000_000 || profile.RecordingQuotaBytes != 80_000_000_000 {
		t.Fatalf("set-quota exited %d (%s), profile %+v %v", set.exitCode, set.stderr, profile, err)
	}
	if student.exitCode != 1 || negative.exitCode != 1 {
		t.Fatalf("a student exited %d and a negative quota %d", student.exitCode, negative.exitCode)
	}
}

func TestOvcAdminUnknownEmail(t *testing.T) {
	f := newAdminFixture(t)

	unknown := f.admin(t, "users", "block", "nobody@example.org")
	usage := f.admin(t, "users", "erase", "nobody@example.org")

	if unknown.exitCode != 1 || !strings.Contains(unknown.stderr, "refused: no account has this email") || strings.Contains(unknown.stderr, "nobody@example.org") {
		t.Fatalf("an unknown email exited %d: %s", unknown.exitCode, unknown.stderr)
	}
	if usage.exitCode != 2 || !strings.Contains(usage.stderr, "usage:") {
		t.Fatalf("an unknown subcommand exited %d: %s", usage.exitCode, usage.stderr)
	}
}

func TestOvcAdminAuditsAFailureBeforeTheCommandRuns(t *testing.T) {
	f := newAdminFixture(t)
	pending := strings.Replace(f.database.URL.Reveal(), f.database.Name, "ovc_missing_"+core.NewRandomID()[:8]+"_test", 1)

	failed := runBinary(t, "ovc-admin", []string{"OVC_DATABASE_URL=" + pending}, "users", "block", "tutor@example.org")

	if failed.exitCode != 1 || !strings.Contains(failed.stderr, `"level":"INFO","msg":"admin.command"`) || !strings.Contains(failed.stderr, `"outcome":"failed"`) ||
		!strings.Contains(failed.stderr, `"subcommand":"users block"`) || strings.Contains(failed.stderr, "tutor@example.org") {
		t.Fatalf("a failure before the command ran exited %d: %s", failed.exitCode, failed.stderr)
	}
}

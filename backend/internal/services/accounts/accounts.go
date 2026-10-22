package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	signInWindow       = 15 * time.Minute
	signInEmailLimit   = 5
	signInAddressLimit = 20
	ipv6BucketBits     = 64

	purposeSignInEmail   domain.RateLimitPurpose = "sign_in_email"
	purposeSignInAddress domain.RateLimitPurpose = "sign_in_address"
)

var (
	ErrInvalidCredentials = errors.New("the email or password is incorrect")
	ErrAccountBlocked     = errors.New("the account is blocked")
	ErrUnauthenticated    = errors.New("authentication is required")
	ErrTokenExpired       = errors.New("the access token has expired")
)

var errCredentialsChanged = errors.New("the credentials changed during sign-in")

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "too many sign-in attempts"
}

type SignIn struct {
	AccessToken     string
	ExpiresIn       time.Duration
	RefreshCookie   string
	RefreshLifetime time.Duration
}

type Principal struct {
	UserID    domain.UserID
	SessionID domain.SessionID
	Role      domain.Role
	State     domain.AccountState
}

type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password string, encoded string) (core.Verification, error)
	VerifyNothing(ctx context.Context, password string) error
}

type Profile struct {
	ID            domain.UserID
	Role          domain.Role
	State         domain.AccountState
	Email         string
	EmailVerified bool
	Name          string
	Language      domain.Language
	Timezone      string
	Theme         domain.Theme
	AvatarID      *domain.AttachmentID
	Tutor         *TutorText
}

type TutorText struct {
	Bio      string
	Subjects []string
}

type Dependencies struct {
	Store       *repositories.Store
	Clock       clock.Source
	Random      core.RandomSource
	Hasher      PasswordHasher
	Tokens      *core.AccessTokens
	Keys        core.Keys
	RefreshTTL  time.Duration
	TutorQuotas core.TutorQuotas
	Avatars     AvatarBinder
	Uploads     UploadPurger
	Logger      *slog.Logger
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	if dependencies.Avatars == nil || dependencies.Uploads == nil {
		panic("the accounts service needs an avatar binder and an upload purger")
	}
	return &Service{deps: dependencies}
}

func (s *Service) Login(ctx context.Context, email string, password string, address netip.Addr) (SignIn, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return SignIn{}, err
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	emailSlot, addressSlot := s.signInSlots(normalized, address, now)
	if err := s.reserve(ctx, now, emailSlot, addressSlot); err != nil {
		return SignIn{}, err
	}
	repos := s.deps.Store.Repositories()
	user, err := repos.Users.ByEmail(ctx, normalized)
	if errors.Is(err, repositories.ErrNotFound) {
		if err := s.deps.Hasher.VerifyNothing(ctx, password); err != nil {
			return SignIn{}, errors.Join(err, s.release(context.WithoutCancel(ctx), emailSlot, addressSlot))
		}
		s.loginFailed(ctx, "unknown_account")
		return SignIn{}, ErrInvalidCredentials
	}
	if err != nil {
		return SignIn{}, err
	}
	verification, err := s.deps.Hasher.Verify(ctx, password, user.PasswordHash)
	if err != nil {
		return SignIn{}, errors.Join(err, s.release(context.WithoutCancel(ctx), emailSlot, addressSlot))
	}
	if !verification.Matches || user.State == domain.AccountDeleted {
		reason := "password"
		if verification.Matches {
			reason = "deleted"
		}
		s.loginFailed(ctx, reason)
		return SignIn{}, ErrInvalidCredentials
	}
	if user.State == domain.AccountBlocked {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.login.failed", slog.String("reason", "blocked"), userAttribute(user.ID))
		return SignIn{}, ErrAccountBlocked
	}
	if err := s.release(ctx, emailSlot, addressSlot); err != nil {
		return SignIn{}, err
	}
	verifiedHash := user.PasswordHash
	if verification.Stale {
		rehashed, err := s.rehash(ctx, user, password, now)
		if errors.Is(err, errCredentialsChanged) {
			s.loginFailed(ctx, "changed")
			return SignIn{}, ErrInvalidCredentials
		}
		if err != nil {
			return SignIn{}, err
		}
		verifiedHash = rehashed
	}
	signIn, err := s.startSession(ctx, user.ID, verifiedHash, now)
	if errors.Is(err, errCredentialsChanged) {
		s.loginFailed(ctx, "changed")
		return SignIn{}, ErrInvalidCredentials
	}
	if errors.Is(err, ErrAccountBlocked) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.login.failed", slog.String("reason", "blocked"), userAttribute(user.ID))
		return SignIn{}, ErrAccountBlocked
	}
	if err != nil {
		return SignIn{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.login.succeeded", userAttribute(user.ID))
	return signIn, nil
}

func (s *Service) Refresh(ctx context.Context, value string) (SignIn, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return SignIn{}, err
	}
	cookie, ok := parseRefreshCookie(value)
	if !ok || !hmac.Equal(cookie.mac, s.deps.Keys.RefreshMAC(cookie.session, cookie.generation)) {
		return SignIn{}, ErrUnauthenticated
	}
	repos := s.deps.Store.Repositories()
	session, err := repos.Sessions.ByID(ctx, cookie.session)
	if errors.Is(err, repositories.ErrNotFound) {
		return SignIn{}, ErrUnauthenticated
	}
	if err != nil {
		return SignIn{}, err
	}
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
		return SignIn{}, ErrUnauthenticated
	}
	if cookie.generation < session.Generation {
		return SignIn{}, s.reuseDetected(ctx, session.ID, session.UserID, now)
	}
	presented := sha256.Sum256(cookie.random)
	if cookie.generation > session.Generation || subtle.ConstantTimeCompare(presented[:], session.RefreshHash) != 1 {
		return SignIn{}, ErrUnauthenticated
	}
	user, err := repos.Users.ByID(ctx, session.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return SignIn{}, ErrUnauthenticated
	}
	if err != nil {
		return SignIn{}, err
	}
	if user.State == domain.AccountBlocked {
		return SignIn{}, ErrAccountBlocked
	}
	if user.State == domain.AccountDeleted {
		return SignIn{}, ErrUnauthenticated
	}
	return s.rotate(ctx, session, cookie, presented[:], now)
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Principal{}, err
	}
	claims, err := s.deps.Tokens.Verify(token, now)
	if errors.Is(err, core.ErrAccessTokenExpired) {
		return Principal{}, ErrTokenExpired
	}
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	found, err := s.deps.Store.Repositories().Sessions.Principal(ctx, claims.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	if found.UserID != claims.UserID || found.RevokedAt != nil || !now.Before(found.ExpiresAt) {
		return Principal{}, ErrUnauthenticated
	}
	switch found.State {
	case domain.AccountBlocked:
		return Principal{}, ErrAccountBlocked
	case domain.AccountActive, domain.AccountUnverified:
		return Principal{UserID: found.UserID, SessionID: found.ID, Role: found.Role, State: found.State}, nil
	case domain.AccountDeleted:
		return Principal{}, ErrUnauthenticated
	}
	return Principal{}, fmt.Errorf("unknown account state %q", found.State)
}

func (s *Service) Logout(ctx context.Context, principal Principal) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	if _, err := s.deps.Store.Repositories().Sessions.Revoke(ctx, principal.SessionID, now); err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.logout", userAttribute(principal.UserID))
	return nil
}

func (s *Service) LogoutAll(ctx context.Context, principal Principal) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	revoked, err := s.deps.Store.Repositories().Sessions.RevokeAllOf(ctx, principal.UserID, now)
	if err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.logout_all", userAttribute(principal.UserID), slog.Int64("sessions", revoked))
	return nil
}

func (s *Service) Me(ctx context.Context, principal Principal) (Profile, error) {
	repos := s.deps.Store.Repositories()
	user, err := repos.Users.ByID(ctx, principal.UserID)
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{
		ID: user.ID, Role: user.Role, State: user.State, Email: user.Email, EmailVerified: user.EmailVerifiedAt != nil,
		Name: user.Name, Language: user.Language, Timezone: user.Timezone, Theme: user.Theme,
	}
	avatar, err := repos.Attachments.AvatarOf(ctx, user.ID)
	if err == nil {
		profile.AvatarID = &avatar
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return Profile{}, err
	}
	if user.Role == domain.RoleTutor {
		tutor, err := repos.TutorProfiles.ByUser(ctx, user.ID)
		if err != nil {
			return Profile{}, err
		}
		profile.Tutor = &TutorText{Bio: tutor.Bio, Subjects: tutor.Subjects}
	}
	return profile, nil
}

func (s *Service) signInSlots(email string, address netip.Addr, now time.Time) (repositories.RateLimitSlot, repositories.RateLimitSlot) {
	window, _ := domain.FixedWindow(now, signInWindow)
	emailSlot := repositories.RateLimitSlot{Purpose: purposeSignInEmail, Subject: s.deps.Keys.ThrottleSubject(string(purposeSignInEmail), email), WindowStart: window}
	addressSlot := repositories.RateLimitSlot{Purpose: purposeSignInAddress, Subject: s.deps.Keys.ThrottleSubject(string(purposeSignInAddress), addressBucket(address)), WindowStart: window}
	return emailSlot, addressSlot
}

func (s *Service) reserve(ctx context.Context, now time.Time, emailSlot repositories.RateLimitSlot, addressSlot repositories.RateLimitSlot) error {
	limits := s.deps.Store.Repositories().RateLimits
	addressCount, err := limits.Reserve(ctx, addressSlot)
	if err != nil {
		return err
	}
	if addressCount > signInAddressLimit {
		if err := s.release(context.WithoutCancel(ctx), addressSlot); err != nil {
			return err
		}
		return s.throttled(ctx, "address", now)
	}
	emailCount, err := limits.Reserve(ctx, emailSlot)
	if err != nil {
		return errors.Join(err, s.release(context.WithoutCancel(ctx), addressSlot))
	}
	if emailCount > signInEmailLimit {
		if err := s.release(context.WithoutCancel(ctx), emailSlot, addressSlot); err != nil {
			return err
		}
		return s.throttled(ctx, "email", now)
	}
	return nil
}

func (s *Service) throttled(ctx context.Context, scope string, now time.Time) error {
	s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "auth.login.throttled", slog.String("scope", scope))
	_, retryAfter := domain.FixedWindow(now, signInWindow)
	return &ThrottledError{RetryAfter: retryAfter}
}

func addressBucket(address netip.Addr) string {
	bucket := address.Unmap()
	if bucket.Is6() {
		bucket = netip.PrefixFrom(bucket, ipv6BucketBits).Masked().Addr()
	}
	return bucket.String()
}

func (s *Service) release(ctx context.Context, slots ...repositories.RateLimitSlot) error {
	for _, slot := range slots {
		if _, err := s.deps.Store.Repositories().RateLimits.Release(ctx, slot); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) rehash(ctx context.Context, user repositories.User, password string, now time.Time) (string, error) {
	encoded, err := s.deps.Hasher.Hash(ctx, password)
	if err != nil {
		return "", err
	}
	replaced, err := s.deps.Store.Repositories().Users.UpdatePasswordHash(ctx, repositories.PasswordRehash{
		PasswordHash: encoded, Now: now, ID: user.ID, CurrentPasswordHash: user.PasswordHash,
	})
	if err != nil {
		return "", err
	}
	if replaced {
		return encoded, nil
	}
	current, err := s.deps.Store.Repositories().Users.ByID(ctx, user.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return "", errCredentialsChanged
	}
	if err != nil {
		return "", err
	}
	verification, err := s.deps.Hasher.Verify(ctx, password, current.PasswordHash)
	if err != nil {
		return "", err
	}
	if !verification.Matches {
		return "", errCredentialsChanged
	}
	return current.PasswordHash, nil
}

type pendingSession struct {
	row    repositories.NewSession
	random []byte
}

func (s *Service) newSession(user domain.UserID, now time.Time) (pendingSession, error) {
	session, err := core.NewID[domain.SessionID](s.deps.Random)
	if err != nil {
		return pendingSession{}, err
	}
	random, _, err := core.RandomToken(s.deps.Random, refreshRandomBytes)
	if err != nil {
		return pendingSession{}, err
	}
	hash := sha256.Sum256(random)
	row := repositories.NewSession{ID: session, UserID: user, RefreshHash: hash[:], ExpiresAt: now.Add(s.deps.RefreshTTL), CreatedAt: now}
	return pendingSession{row: row, random: random}, nil
}

func (s *Service) startSession(ctx context.Context, user domain.UserID, verifiedHash string, now time.Time) (SignIn, error) {
	session, err := s.newSession(user, now)
	if err != nil {
		return SignIn{}, err
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Users.LockCredentials(ctx, user)
		if errors.Is(err, repositories.ErrNotFound) {
			return errCredentialsChanged
		}
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(locked.PasswordHash), []byte(verifiedHash)) != 1 || locked.State == domain.AccountDeleted {
			return errCredentialsChanged
		}
		if locked.State == domain.AccountBlocked {
			return ErrAccountBlocked
		}
		return repos.Sessions.Create(ctx, session.row)
	})
	if err != nil {
		return SignIn{}, err
	}
	return s.signIn(user, session.row.ID, 1, session.random, session.row.ExpiresAt, now)
}

func (s *Service) rotate(ctx context.Context, session repositories.Session, cookie refreshCookie, presented []byte, now time.Time) (SignIn, error) {
	random, _, err := core.RandomToken(s.deps.Random, refreshRandomBytes)
	if err != nil {
		return SignIn{}, err
	}
	next := sha256.Sum256(random)
	generation, err := s.deps.Store.Repositories().Sessions.Rotate(ctx, repositories.SessionRotation{
		ID: session.ID, Generation: cookie.generation, RefreshHash: presented, NewRefreshHash: next[:], Now: now,
	})
	if errors.Is(err, repositories.ErrNotFound) {
		return SignIn{}, s.lostRotation(ctx, session, cookie.generation, now)
	}
	if err != nil {
		return SignIn{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "auth.refresh.rotated", userAttribute(session.UserID))
	return s.signIn(session.UserID, session.ID, generation, random, session.ExpiresAt, now)
}

func (s *Service) lostRotation(ctx context.Context, session repositories.Session, generation int64, now time.Time) error {
	current, err := s.deps.Store.Repositories().Sessions.ByID(ctx, session.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if current.RevokedAt == nil && current.Generation > generation {
		return s.reuseDetected(ctx, session.ID, session.UserID, now)
	}
	return ErrUnauthenticated
}

func (s *Service) reuseDetected(ctx context.Context, session domain.SessionID, user domain.UserID, now time.Time) error {
	if _, err := s.deps.Store.Repositories().Sessions.Revoke(ctx, session, now); err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "auth.refresh.reuse_detected", userAttribute(user))
	return ErrUnauthenticated
}

func (s *Service) signIn(user domain.UserID, session domain.SessionID, generation int64, random []byte, expires time.Time, now time.Time) (SignIn, error) {
	access, err := s.deps.Tokens.Issue(core.AccessClaims{UserID: user, SessionID: session}, now)
	if err != nil {
		return SignIn{}, err
	}
	cookie := refreshCookie{session: session, generation: generation, random: random, mac: s.deps.Keys.RefreshMAC(session, generation)}
	return SignIn{AccessToken: access, ExpiresIn: s.deps.Tokens.TTL(), RefreshCookie: cookie.encode(), RefreshLifetime: expires.Sub(now)}, nil
}

func (s *Service) loginFailed(ctx context.Context, reason string) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.login.failed", slog.String("reason", reason))
}

func userAttribute(user domain.UserID) slog.Attr {
	return slog.String("user_id", user.String())
}

const (
	rateLimitRetention  = 25 * time.Hour
	rateLimitBatchSize  = 1000
	rateLimitMaxBatches = 20

	finishedSessionRetention = 24 * time.Hour
	emailRequestRetention    = 7 * 24 * time.Hour
	housekeepingBatchSize    = 1000
	housekeepingMaxBatches   = 20
)

func CleanupRateLimits(store *repositories.Store) func(context.Context, time.Time) (int64, error) {
	return func(ctx context.Context, now time.Time) (int64, error) {
		return store.Repositories().RateLimits.DeleteBefore(ctx, now.Add(-rateLimitRetention), rateLimitBatchSize, rateLimitMaxBatches)
	}
}

func CleanupSessions(store *repositories.Store) func(context.Context, time.Time) (int64, error) {
	return func(ctx context.Context, now time.Time) (int64, error) {
		return store.Repositories().Sessions.DeleteFinishedBefore(ctx, now.Add(-finishedSessionRetention), housekeepingBatchSize, housekeepingMaxBatches)
	}
}

func CleanupEmailRequests(store *repositories.Store) func(context.Context, time.Time) (int64, error) {
	return func(ctx context.Context, now time.Time) (int64, error) {
		return store.Repositories().EmailTokens.DeleteRequestsBefore(ctx, now.Add(-emailRequestRetention), housekeepingBatchSize, housekeepingMaxBatches)
	}
}

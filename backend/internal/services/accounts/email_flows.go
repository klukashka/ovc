package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	maxZoneNameLength = 64
	maxNameRunes      = 100
	fallbackZone      = "UTC"

	purposeRequestAddress  domain.RateLimitPurpose = "email_request_address"
	purposeSendMinute      domain.RateLimitPurpose = "email_send_minute"
	purposeSendHour        domain.RateLimitPurpose = "email_send_hour"
	purposeSendDay         domain.RateLimitPurpose = "email_send_day"
	purposeCurrentPassword domain.RateLimitPurpose = "current_password"

	fieldRole            = "body.role"
	fieldName            = "body.name"
	fieldEmail           = "body.email"
	fieldPassword        = "body.password"
	fieldConsent         = "body.consent"
	fieldNewPassword     = "body.new_password"
	fieldCurrentPassword = "body.current_password"
)

var (
	ErrTokenInvalid = errors.New("the email token is invalid")

	errSendLimited = errors.New("the account's email sending limit is reached")
)

type rateLimit struct {
	purpose domain.RateLimitPurpose
	window  time.Duration
	limit   int32
}

var (
	requestAddressLimit  = rateLimit{purpose: purposeRequestAddress, window: time.Hour, limit: 10}
	currentPasswordLimit = rateLimit{purpose: purposeCurrentPassword, window: signInWindow, limit: signInEmailLimit}
	sendLimits           = []rateLimit{
		{purpose: purposeSendMinute, window: time.Minute, limit: 1},
		{purpose: purposeSendHour, window: time.Hour, limit: 5},
		{purpose: purposeSendDay, window: 24 * time.Hour, limit: 10},
	}
)

type FieldProblem struct {
	Path string
	Code domain.FieldCode
}

type ValidationError struct {
	Fields []FieldProblem
}

func (e *ValidationError) Error() string {
	return "the request is invalid"
}

func invalid(path string, code domain.FieldCode) error {
	return &ValidationError{Fields: []FieldProblem{{Path: path, Code: code}}}
}

type Registration struct {
	Role     domain.Role
	Name     string
	Email    string
	Password string
	Language domain.Language
	Timezone string
	Consent  bool
}

type Confirmation struct {
	SignIn  *SignIn
	Outcome *domain.LinkOutcome
}

type InvitationSignUp struct {
	Name     *string
	Email    *string
	Password *string
	Language *domain.Language
	Timezone *string
	Consent  *bool
}

func (s *Service) Register(ctx context.Context, registration Registration, address netip.Addr) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	email := domain.NormalizeEmail(registration.Email)
	name := strings.TrimSpace(registration.Name)
	if problems := registrationProblems(registration, name, email); len(problems) > 0 {
		return &ValidationError{Fields: problems}
	}
	if err := s.reserveRequestAddress(ctx, address, now); err != nil {
		return err
	}
	encoded, err := s.deps.Hasher.Hash(ctx, registration.Password)
	if err != nil {
		return err
	}
	user, err := core.NewID[domain.UserID](s.deps.Random)
	if err != nil {
		return err
	}
	created := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		inserted, err := repos.Users.CreateIfEmailFree(ctx, repositories.SignUp{
			ID: user, Role: domain.RoleTutor, Email: email, PasswordHash: encoded, Name: name, Language: registration.Language,
			Timezone: resolveZone(registration.Timezone), Theme: domain.ThemeSystem, ConsentVersion: string(domain.CurrentConsentVersion), CreatedAt: now,
		})
		if err != nil || !inserted {
			return err
		}
		created = true
		err = repos.TutorProfiles.Create(ctx, repositories.NewTutorProfile{
			UserID: user, FileQuotaBytes: s.deps.TutorQuotas.FileBytes, RecordingQuotaBytes: s.deps.TutorQuotas.RecordingBytes, CreatedAt: now,
		}, s.deps.Random)
		if err != nil {
			return err
		}
		err = s.queueEmailRequest(ctx, repos, email, &user, domain.EmailPurposeVerify, now)
		var limited *sendLimitError
		if errors.As(err, &limited) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.email.limited", slog.String("purpose", string(domain.EmailPurposeVerify)), userAttribute(user))
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	if created {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.signup.accepted", userAttribute(user))
		return nil
	}
	return s.signUpDuplicate(ctx, email, now)
}

func (s *Service) signUpDuplicate(ctx context.Context, email string, now time.Time) error {
	existing, err := s.deps.Store.Repositories().Users.ByEmail(ctx, email)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.State == domain.AccountDeleted {
		return nil
	}
	key := fmt.Sprintf("%s:%s:%s", domain.EventSignUpDuplicate, hex.EncodeToString(existing.ID[:]), now.UTC().Format(time.DateOnly))
	payload := domain.AccountPayload{User: hex.EncodeToString(existing.ID[:])}
	if err := queueEmail(ctx, s.deps.Random, s.deps.Store.Repositories(), domain.EventSignUpDuplicate, key, payload, now); err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.signup.duplicate", userAttribute(existing.ID))
	return nil
}

func (s *Service) ResendConfirmationTo(ctx context.Context, email string, address netip.Addr) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	normalized := domain.NormalizeEmail(email)
	if !domain.ValidEmail(normalized) {
		return invalid(fieldEmail, domain.FieldInvalid)
	}
	if err := s.reserveRequestAddress(ctx, address, now); err != nil {
		return err
	}
	recipient, err := s.recipient(ctx, normalized, domain.AccountUnverified)
	if err != nil {
		return err
	}
	return s.requestEmailSilently(ctx, normalized, recipient, domain.EmailPurposeVerify, now)
}

func (s *Service) recipient(ctx context.Context, email string, states ...domain.AccountState) (*domain.UserID, error) {
	user, err := s.deps.Store.Repositories().Users.ByEmail(ctx, email)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(states, user.State) {
		return nil, nil
	}
	return &user.ID, nil
}

func (s *Service) ResendConfirmationFor(ctx context.Context, principal Principal, address netip.Addr) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	if err := s.reserveRequestAddress(ctx, address, now); err != nil {
		return err
	}
	if principal.State != domain.AccountUnverified {
		return nil
	}
	user, err := s.deps.Store.Repositories().Users.ByID(ctx, principal.UserID)
	if err != nil {
		return err
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		return s.queueEmailRequest(ctx, repos, user.Email, &user.ID, domain.EmailPurposeVerify, now)
	})
	var limited *sendLimitError
	if errors.As(err, &limited) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.email.limited", slog.String("purpose", string(domain.EmailPurposeVerify)), userAttribute(principal.UserID))
		return &ThrottledError{RetryAfter: limited.retryAfter}
	}
	return err
}

func (s *Service) ForgotPassword(ctx context.Context, email string, address netip.Addr) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	normalized := domain.NormalizeEmail(email)
	if !domain.ValidEmail(normalized) {
		return invalid(fieldEmail, domain.FieldInvalid)
	}
	if err := s.reserveRequestAddress(ctx, address, now); err != nil {
		return err
	}
	recipient, err := s.recipient(ctx, normalized, domain.AccountUnverified, domain.AccountActive)
	if err != nil {
		return err
	}
	return s.requestEmailSilently(ctx, normalized, recipient, domain.EmailPurposeReset, now)
}

func (s *Service) ConfirmEmail(ctx context.Context, caller *Principal, token string, newPassword *string) (Confirmation, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Confirmation{}, err
	}
	stored, err := s.usableToken(ctx, token, domain.EmailPurposeVerify, now)
	if err != nil {
		return Confirmation{}, err
	}
	if caller != nil && caller.UserID == stored.UserID {
		return s.confirmSignedIn(ctx, stored, *caller, now)
	}
	if newPassword == nil {
		return Confirmation{}, invalid(fieldNewPassword, domain.FieldRequired)
	}
	if problem, fits := domain.CheckPasswordLength(*newPassword); !fits {
		return Confirmation{}, invalid(fieldNewPassword, problem)
	}
	encoded, err := s.deps.Hasher.Hash(ctx, *newPassword)
	if err != nil {
		return Confirmation{}, err
	}
	session, err := s.newSession(stored.UserID, now)
	if err != nil {
		return Confirmation{}, err
	}
	var outcome domain.LinkOutcome
	announce := Announce(Silent)
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if _, err := consumeFor(ctx, repos, stored, now, domain.AccountUnverified); err != nil {
			return err
		}
		if err := replacePassword(ctx, repos, stored.UserID, encoded, now); err != nil {
			return err
		}
		if _, err := repos.Sessions.RevokeAllOf(ctx, stored.UserID, now); err != nil {
			return err
		}
		if err := repos.Sessions.Create(ctx, session.row); err != nil {
			return err
		}
		outcome, announce, err = s.deps.Tutoring.OnEmailConfirmed(ctx, repos, stored.UserID, true, now)
		return err
	})
	if err != nil {
		return Confirmation{}, err
	}
	signIn, err := s.signIn(stored.UserID, session.row.ID, 1, session.random, session.row.ExpiresAt, now)
	if err != nil {
		return Confirmation{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.email.confirmed", userAttribute(stored.UserID), slog.String("path", "new_password"))
	announce(ctx)
	return Confirmation{SignIn: &signIn, Outcome: reported(outcome)}, nil
}

func reported(outcome domain.LinkOutcome) *domain.LinkOutcome {
	if outcome == "" {
		return nil
	}
	return &outcome
}

func (s *Service) confirmSignedIn(ctx context.Context, stored repositories.EmailToken, caller Principal, now time.Time) (Confirmation, error) {
	var outcome domain.LinkOutcome
	announce := Announce(Silent)
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := LockActingAccount(ctx, repos, caller, now, domain.AccountUnverified, domain.AccountActive); err != nil {
			return err
		}
		if _, err := consumeFor(ctx, repos, stored, now, domain.AccountUnverified); err != nil {
			return err
		}
		confirmed, err := repos.Users.ConfirmEmail(ctx, stored.UserID, now)
		if err != nil {
			return err
		}
		if !confirmed {
			return ErrTokenInvalid
		}
		if err := closeEmailRequests(ctx, repos, stored.UserID); err != nil {
			return err
		}
		if _, err := repos.Sessions.RevokeOthersOf(ctx, stored.UserID, caller.SessionID, now); err != nil {
			return err
		}
		outcome, announce, err = s.deps.Tutoring.OnEmailConfirmed(ctx, repos, stored.UserID, false, now)
		return err
	})
	if err != nil {
		return Confirmation{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.email.confirmed", userAttribute(stored.UserID), slog.String("path", "signed_in"))
	announce(ctx)
	return Confirmation{Outcome: reported(outcome)}, nil
}

func LockActingAccount(ctx context.Context, repos repositories.Repositories, principal Principal, now time.Time, authorised ...domain.AccountState) error {
	locked, err := repos.Users.LockCredentials(ctx, principal.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	session, err := repos.Sessions.ByID(ctx, principal.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if session.UserID != principal.UserID || session.RevokedAt != nil || !now.Before(session.ExpiresAt) || !slices.Contains(authorised, locked.State) {
		return ErrUnauthenticated
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token string, newPassword string) (*domain.LinkOutcome, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := s.usableToken(ctx, token, domain.EmailPurposeReset, now)
	if err != nil {
		return nil, err
	}
	if problem, fits := domain.CheckPasswordLength(newPassword); !fits {
		return nil, invalid(fieldNewPassword, problem)
	}
	encoded, err := s.deps.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return nil, err
	}
	var outcome domain.LinkOutcome
	announce := Announce(Silent)
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		before, err := consumeFor(ctx, repos, stored, now, domain.AccountUnverified, domain.AccountActive)
		if err != nil {
			return err
		}
		if err := replacePassword(ctx, repos, stored.UserID, encoded, now); err != nil {
			return err
		}
		if _, err := repos.Sessions.RevokeAllOf(ctx, stored.UserID, now); err != nil {
			return err
		}
		if before != domain.AccountUnverified {
			return nil
		}
		outcome, announce, err = s.deps.Tutoring.OnEmailConfirmed(ctx, repos, stored.UserID, true, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.password.reset", userAttribute(stored.UserID))
	announce(ctx)
	return reported(outcome), nil
}

func (s *Service) ChangePassword(ctx context.Context, principal Principal, current string, next string) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	if problem, fits := domain.CheckPasswordLength(next); !fits {
		return invalid(fieldNewPassword, problem)
	}
	user, err := s.verifyCurrentPassword(ctx, principal.UserID, current, fieldCurrentPassword, now)
	if err != nil {
		return err
	}
	encoded, err := s.deps.Hasher.Hash(ctx, next)
	if err != nil {
		return err
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Users.LockCredentials(ctx, principal.UserID)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(locked.PasswordHash), []byte(user.PasswordHash)) != 1 {
			return invalid(fieldCurrentPassword, domain.FieldIncorrect)
		}
		if locked.State != domain.AccountUnverified && locked.State != domain.AccountActive {
			return ErrUnauthenticated
		}
		replaced, err := repos.Users.UpdatePasswordHash(ctx, repositories.PasswordRehash{
			PasswordHash: encoded, Now: now, ID: principal.UserID, CurrentPasswordHash: user.PasswordHash,
		})
		if err != nil {
			return err
		}
		if !replaced {
			return invalid(fieldCurrentPassword, domain.FieldIncorrect)
		}
		if locked.State == domain.AccountActive {
			if err := closeEmailRequests(ctx, repos, principal.UserID); err != nil {
				return err
			}
		}
		_, err = repos.Sessions.RevokeOthersOf(ctx, principal.UserID, principal.SessionID, now)
		return err
	})
	if err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.password.changed", userAttribute(principal.UserID))
	return nil
}

func (s *Service) usableToken(ctx context.Context, token string, purpose domain.EmailPurpose, now time.Time) (repositories.EmailToken, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != domain.EmailTokenBytes {
		return repositories.EmailToken{}, ErrTokenInvalid
	}
	hash := sha256.Sum256(raw)
	stored, err := s.deps.Store.Repositories().EmailTokens.Usable(ctx, repositories.EmailTokenLookup{TokenHash: hash[:], Purpose: purpose, Now: now})
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.EmailToken{}, ErrTokenInvalid
	}
	return stored, err
}

func consumeFor(
	ctx context.Context, repos repositories.Repositories, stored repositories.EmailToken, now time.Time, states ...domain.AccountState,
) (domain.AccountState, error) {
	locked, err := repos.Users.LockCredentials(ctx, stored.UserID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(states, locked.State) {
		return "", ErrTokenInvalid
	}
	_, err = repos.EmailTokens.Consume(ctx, repositories.EmailTokenLookup{TokenHash: stored.TokenHash, Purpose: stored.Purpose, Now: now})
	if errors.Is(err, repositories.ErrNotFound) {
		return "", ErrTokenInvalid
	}
	return locked.State, err
}

func replacePassword(ctx context.Context, repos repositories.Repositories, user domain.UserID, encoded string, now time.Time) error {
	replaced, err := repos.Users.ReplacePassword(ctx, user, encoded, now)
	if err != nil {
		return err
	}
	if !replaced {
		return ErrTokenInvalid
	}
	return closeEmailRequests(ctx, repos, user)
}

func releaseIn(ctx context.Context, repos repositories.Repositories, slots []repositories.RateLimitSlot) error {
	for _, slot := range slots {
		if _, err := repos.RateLimits.Release(ctx, slot); err != nil {
			return err
		}
	}
	return nil
}

func closeEmailRequests(ctx context.Context, repos repositories.Repositories, user domain.UserID) error {
	if _, err := repos.EmailTokens.DeleteOpenRequests(ctx, user); err != nil {
		return err
	}
	_, err := repos.EmailTokens.DiscardAllUnused(ctx, user)
	return err
}

type sendLimitError struct {
	retryAfter time.Duration
}

func (e *sendLimitError) Error() string {
	return errSendLimited.Error()
}

func (s *Service) requestEmailSilently(ctx context.Context, email string, recipient *domain.UserID, purpose domain.EmailPurpose, now time.Time) error {
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		return s.queueEmailRequest(ctx, repos, email, recipient, purpose, now)
	})
	var limited *sendLimitError
	if errors.As(err, &limited) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.email.limited", slog.String("purpose", string(purpose)))
		return nil
	}
	if err != nil || recipient == nil {
		return err
	}
	if purpose == domain.EmailPurposeReset {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.password.reset_requested", userAttribute(*recipient))
	}
	return nil
}

func (s *Service) queueEmailRequest(ctx context.Context, repos repositories.Repositories, email string, recipient *domain.UserID, purpose domain.EmailPurpose, now time.Time) error {
	reserved := make([]repositories.RateLimitSlot, 0, len(sendLimits))
	for _, limit := range sendLimits {
		slot := s.limitSlot(limit, email, now)
		count, err := repos.RateLimits.Reserve(ctx, slot)
		if err != nil {
			return err
		}
		reserved = append(reserved, slot)
		if count > limit.limit {
			_, retryAfter := domain.FixedWindow(now, limit.window)
			if err := releaseIn(ctx, repos, reserved); err != nil {
				return fmt.Errorf("return the sending limit slots: %w", err)
			}
			return &sendLimitError{retryAfter: retryAfter}
		}
	}
	if recipient == nil {
		return nil
	}
	request, err := core.NewID[domain.EmailRequestID](s.deps.Random)
	if err != nil {
		return err
	}
	err = repos.EmailTokens.CreateRequest(ctx, repositories.NewEmailRequest{ID: request, UserID: *recipient, Purpose: purpose, CreatedAt: now})
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%s", purpose.Event(), hex.EncodeToString(request[:]))
	return queueEmail(ctx, s.deps.Random, repos, purpose.Event(), key, domain.EmailRequestPayload{Request: hex.EncodeToString(request[:])}, now)
}

func queueEmail[Payload domain.EmailRequestPayload | domain.AccountPayload](ctx context.Context, random core.RandomSource, repos repositories.Repositories, kind domain.OutboxKind, key string, payload Payload, now time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode the outbox payload: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityHigh, Kind: kind, Payload: encoded,
		IdempotencyKey: key, NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

func (s *Service) reserveRequestAddress(ctx context.Context, address netip.Addr, now time.Time) error {
	count, err := s.deps.Store.Repositories().RateLimits.Reserve(ctx, s.limitSlot(requestAddressLimit, addressBucket(address), now))
	if err != nil {
		return err
	}
	if count > requestAddressLimit.limit {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "auth.request.throttled", slog.String("scope", "address"))
		_, retryAfter := domain.FixedWindow(now, requestAddressLimit.window)
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (s *Service) limitSlot(limit rateLimit, subject string, now time.Time) repositories.RateLimitSlot {
	window, _ := domain.FixedWindow(now, limit.window)
	return repositories.RateLimitSlot{Purpose: limit.purpose, Subject: s.deps.Keys.ThrottleSubject(string(limit.purpose), subject), WindowStart: window}
}

func registrationProblems(registration Registration, name string, email string) []FieldProblem {
	var problems []FieldProblem
	if registration.Role != domain.RoleTutor {
		problems = append(problems, FieldProblem{Path: fieldRole, Code: domain.FieldNotAllowed})
	}
	return append(problems, signUpProblems(name, email, registration.Password, registration.Consent)...)
}

func signUpProblems(name string, email string, password string, consent bool) []FieldProblem {
	var problems []FieldProblem
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes || strings.ContainsFunc(name, unicode.IsControl) {
		problems = append(problems, FieldProblem{Path: fieldName, Code: domain.FieldInvalid})
	}
	if !domain.ValidEmail(email) {
		problems = append(problems, FieldProblem{Path: fieldEmail, Code: domain.FieldInvalid})
	}
	if problem, fits := domain.CheckPasswordLength(password); !fits {
		problems = append(problems, FieldProblem{Path: fieldPassword, Code: problem})
	}
	if !consent {
		problems = append(problems, FieldProblem{Path: fieldConsent, Code: domain.FieldRequired})
	}
	return problems
}

var canonicalZoneName = regexp.MustCompile(`^[A-Za-z0-9_+-]+(/[A-Za-z0-9_+-]+){0,3}$`)

func notAZone(name string) bool {
	switch name {
	case "Local", "Factory", "posixrules", "localtime":
		return true
	}
	return strings.HasPrefix(name, "posix/") || strings.HasPrefix(name, "right/")
}

func resolveZone(name string) string {
	if len(name) > maxZoneNameLength || !canonicalZoneName.MatchString(name) || notAZone(name) {
		return fallbackZone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return fallbackZone
	}
	return location.String()
}

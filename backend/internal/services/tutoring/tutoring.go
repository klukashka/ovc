package tutoring

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
)

const (
	dailyWindow  = 24 * time.Hour
	hourlyWindow = time.Hour

	purposeCodeRegeneration domain.RateLimitPurpose = "tutor_code_regeneration_day"
	purposeCardCreation     domain.RateLimitPurpose = "card_creation_day"
	purposeCardEmailChange  domain.RateLimitPurpose = "card_email_change_day"

	purposeInvitationRegeneration domain.RateLimitPurpose = "invitation_regeneration_day"
	purposeSettingsWrites         domain.RateLimitPurpose = "tutor_settings_writes_hour"
)

type rateLimit struct {
	purpose domain.RateLimitPurpose
	window  time.Duration
	limit   int32
}

func daily(purpose domain.RateLimitPurpose, limit int32) rateLimit {
	return rateLimit{purpose: purpose, window: dailyWindow, limit: limit}
}

var (
	codeRegenerationLimit = daily(purposeCodeRegeneration, 10)
	cardCreationLimit     = daily(purposeCardCreation, 50)
	cardEmailChangeLimit  = daily(purposeCardEmailChange, 50)
	regenerationLimit     = daily(purposeInvitationRegeneration, 50)
	settingsWriteLimit    = rateLimit{purpose: purposeSettingsWrites, window: hourlyWindow, limit: 60}

	settingsWriters = []domain.AccountState{domain.AccountActive, domain.AccountUnverified}
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "a rate limit is reached"
}

type TutorCode struct {
	Code   domain.TutorCode
	Active bool
}

type TutorSettings struct {
	FileQuotaBytes      int64
	FileUsageBytes      int64
	OnboardingDismissed bool
}

type Dependencies struct {
	Store     *repositories.Store
	Clock     clock.Source
	Random    core.RandomSource
	Keys      core.Keys
	Cipher    core.FieldCipher
	PublicURL string
	Logger    *slog.Logger
	Exports   ExportInvalidator

	PurgeBatchRows int32
}

type ExportInvalidator interface {
	Apply(ctx context.Context, repos repositories.Repositories, now time.Time) error
}

type Service struct {
	deps   Dependencies
	expiry *expiryResume
}

func New(dependencies Dependencies) *Service {
	return &Service{deps: dependencies, expiry: &expiryResume{}}
}

func (s *Service) withExports(ctx context.Context, now time.Time, work func(repositories.Repositories) error) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := work(repos); err != nil {
			return err
		}
		return s.deps.Exports.Apply(ctx, repos, now)
	})
}

func (s *Service) mirror(
	ctx context.Context, repos repositories.Repositories, path domain.MembershipPath, audiences chat.Audiences, now time.Time,
) error {
	return s.channels().Apply(ctx, repos, path, audiences, now)
}

func (s *Service) channels() chat.Mirror {
	return chat.Mirror{Random: s.deps.Random, Logger: s.deps.Logger}
}

func directAudience(links ...domain.RelationshipID) chat.Audiences {
	return chat.Audiences{Relationships: links}
}

func (s *Service) Code(ctx context.Context, tutor domain.UserID) (TutorCode, error) {
	current, err := s.deps.Store.Repositories().TutorProfiles.Code(ctx, tutor)
	if err != nil {
		return TutorCode{}, err
	}
	return TutorCode{Code: current.Code, Active: current.State == domain.AccountActive}, nil
}

func (s *Service) Settings(ctx context.Context, tutor domain.UserID) (TutorSettings, error) {
	return settingsOf(ctx, s.deps.Store.Repositories(), tutor)
}

func settingsOf(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) (TutorSettings, error) {
	profile, err := repos.TutorProfiles.ByUser(ctx, tutor)
	if err != nil {
		return TutorSettings{}, err
	}
	return TutorSettings{
		FileQuotaBytes: profile.FileQuotaBytes, FileUsageBytes: profile.FileUsageBytes, OnboardingDismissed: profile.OnboardingDismissedAt != nil,
	}, nil
}

func (s *Service) DismissOnboarding(ctx context.Context, actor domain.TutorActor) (TutorSettings, error) {
	tutor := actor.UserID
	var settings TutorSettings
	dismissed := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockActingTutorIn(ctx, repos, actor, settingsWriters)
		if err != nil {
			return err
		}
		if err := s.reserve(ctx, repos, settingsWriteLimit, tutor, now); err != nil {
			s.logSettingsThrottled(ctx, err)
			return err
		}
		dismissed, err = repos.TutorProfiles.DismissOnboarding(ctx, tutor, now)
		if err != nil {
			return err
		}
		settings, err = settingsOf(ctx, repos, tutor)
		return err
	})
	if err != nil {
		return TutorSettings{}, err
	}
	if dismissed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "onboarding.dismissed", slog.String("tutor_id", tutor.String()))
	}
	return settings, nil
}

func (s *Service) logSettingsThrottled(ctx context.Context, err error) {
	var throttled *ThrottledError
	if errors.As(err, &throttled) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "tutor.settings.rate_limited", slog.String("limit", string(purposeSettingsWrites)))
	}
}

func (s *Service) RegenerateCode(ctx context.Context, actor domain.TutorActor) (TutorCode, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return TutorCode{}, err
	}
	var regenerated domain.TutorCode
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		if err := s.reserve(ctx, repos, codeRegenerationLimit, tutor, now); err != nil {
			return err
		}
		code, err := repos.TutorProfiles.ReplaceCode(ctx, tutor, s.deps.Random, now)
		regenerated = code
		return err
	})
	if err != nil {
		return TutorCode{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "tutor_code.regenerated", slog.String("tutor_id", tutor.String()))
	return TutorCode{Code: regenerated, Active: true}, nil
}

func (s *Service) reserve(ctx context.Context, repos repositories.Repositories, limit rateLimit, tutor domain.UserID, now time.Time) error {
	return s.reserveFor(ctx, repos, limit, tutor.String(), now)
}

func (s *Service) reserveFor(ctx context.Context, repos repositories.Repositories, limit rateLimit, subject string, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, limit.window)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: limit.purpose, Subject: s.deps.Keys.ThrottleSubject(string(limit.purpose), subject), WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > limit.limit {
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

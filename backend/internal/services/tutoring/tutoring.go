package tutoring

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	dailyWindow = 24 * time.Hour

	purposeCodeRegeneration domain.RateLimitPurpose = "tutor_code_regeneration_day"
	purposeCardCreation     domain.RateLimitPurpose = "card_creation_day"
	purposeCardEmailChange  domain.RateLimitPurpose = "card_email_change_day"

	purposeInvitationRegeneration domain.RateLimitPurpose = "invitation_regeneration_day"
)

type dailyLimit struct {
	purpose domain.RateLimitPurpose
	limit   int32
}

var (
	codeRegenerationLimit = dailyLimit{purpose: purposeCodeRegeneration, limit: 10}
	cardCreationLimit     = dailyLimit{purpose: purposeCardCreation, limit: 50}
	cardEmailChangeLimit  = dailyLimit{purpose: purposeCardEmailChange, limit: 50}
	regenerationLimit     = dailyLimit{purpose: purposeInvitationRegeneration, limit: 50}
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "a daily limit of the tutor is reached"
}

type TutorCode struct {
	Code   domain.TutorCode
	Active bool
}

type Dependencies struct {
	Store     *repositories.Store
	Clock     clock.Source
	Random    core.RandomSource
	Keys      core.Keys
	Cipher    core.FieldCipher
	PublicURL string
	Logger    *slog.Logger
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	return &Service{deps: dependencies}
}

func (s *Service) Code(ctx context.Context, tutor domain.UserID) (TutorCode, error) {
	current, err := s.deps.Store.Repositories().TutorProfiles.Code(ctx, tutor)
	if err != nil {
		return TutorCode{}, err
	}
	return TutorCode{Code: current.Code, Active: current.State == domain.AccountActive}, nil
}

func (s *Service) RegenerateCode(ctx context.Context, tutor domain.UserID) (TutorCode, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return TutorCode{}, err
	}
	var regenerated domain.TutorCode
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
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

func (s *Service) reserve(ctx context.Context, repos repositories.Repositories, limit dailyLimit, tutor domain.UserID, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, dailyWindow)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: limit.purpose, Subject: s.deps.Keys.ThrottleSubject(string(limit.purpose), tutor.String()), WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > limit.limit {
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

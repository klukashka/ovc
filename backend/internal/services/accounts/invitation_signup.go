package accounts

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) RegisterThroughInvitation(ctx context.Context, token string, registration InvitationSignUp, address netip.Addr) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	if err := s.reserveRequestAddress(ctx, address, now); err != nil {
		return err
	}
	check, err := s.deps.Tutoring.PrecheckInvitation(ctx, token, now)
	if err != nil {
		return err
	}
	if problems := invitationSignUpProblems(registration); len(problems) > 0 {
		return &ValidationError{Fields: problems}
	}
	email := domain.NormalizeEmail(*registration.Email)
	name := strings.TrimSpace(*registration.Name)
	if err := s.deps.Tutoring.BindNewAccount(ctx, check, email, now); err != nil {
		return err
	}
	encoded, err := s.deps.Hasher.Hash(ctx, *registration.Password)
	if err != nil {
		return err
	}
	user, err := core.NewID[domain.UserID](s.deps.Random)
	if err != nil {
		return err
	}
	created := false
	announce := Announce(Silent)
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		inserted, err := repos.Users.CreateIfEmailFree(ctx, repositories.SignUp{
			ID: user, Role: domain.RoleStudent, Email: email, PasswordHash: encoded, Name: name, Language: *registration.Language,
			Timezone: resolveZone(*registration.Timezone), Theme: domain.ThemeSystem, ConsentVersion: string(domain.CurrentConsentVersion), CreatedAt: now,
		})
		if err != nil || !inserted {
			return err
		}
		created = true
		announce, err = s.deps.Tutoring.LinkAtSignUp(ctx, repos, user, check, now)
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
		announce(ctx)
		return nil
	}
	return s.signUpDuplicate(ctx, email, now)
}

func invitationSignUpProblems(registration InvitationSignUp) []FieldProblem {
	var problems []FieldProblem
	missing := func(path string, absent bool) bool {
		if absent {
			problems = append(problems, FieldProblem{Path: path, Code: domain.FieldRequired})
		}
		return absent
	}
	if !missing(fieldName, registration.Name == nil) && !validName(strings.TrimSpace(*registration.Name)) {
		problems = append(problems, FieldProblem{Path: fieldName, Code: domain.FieldInvalid})
	}
	if !missing(fieldEmail, registration.Email == nil) && !domain.ValidEmail(domain.NormalizeEmail(*registration.Email)) {
		problems = append(problems, FieldProblem{Path: fieldEmail, Code: domain.FieldInvalid})
	}
	if !missing(fieldPassword, registration.Password == nil) {
		if problem, fits := domain.CheckPasswordLength(*registration.Password); !fits {
			problems = append(problems, FieldProblem{Path: fieldPassword, Code: problem})
		}
	}
	missing(fieldLanguage, registration.Language == nil)
	missing(fieldTimezone, registration.Timezone == nil)
	missing(fieldConsent, registration.Consent == nil || !*registration.Consent)
	return problems
}

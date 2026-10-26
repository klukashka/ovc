package accounts

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrSeedEmailTaken = errors.New("the email already belongs to an account")
	ErrNotSeeded      = errors.New("the account was not created by the seeder or its password changed")
)

type SeedAccount struct {
	Role     domain.Role
	Name     string
	Email    string
	Language domain.Language
	Timezone string
}

type SeededAccount struct {
	User   domain.UserID
	SignIn SignIn
}

type Seeder struct {
	service  *Service
	password string
	mutex    sync.Mutex
	hash     string
}

const seededPasswordBytes = 18

func NewSeeder(service *Service) (*Seeder, error) {
	_, password, err := core.RandomToken(service.deps.Random, seededPasswordBytes)
	if err != nil {
		return nil, err
	}
	if _, fits := domain.CheckPasswordLength(password); !fits {
		panic("the seeded account password does not fit the password rules")
	}
	return &Seeder{service: service, password: password}, nil
}

func (s *Seeder) Password() string {
	return s.password
}

func (s *Seeder) seededHash(ctx context.Context) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.hash != "" {
		return s.hash, nil
	}
	encoded, err := s.service.deps.Hasher.Hash(ctx, s.password)
	if err != nil {
		return "", err
	}
	s.hash = encoded
	return encoded, nil
}

func (s *Seeder) Create(ctx context.Context, account SeedAccount) (SeededAccount, error) {
	deps := s.service.deps
	now, err := deps.Clock.Instant(ctx)
	if err != nil {
		return SeededAccount{}, err
	}
	email := domain.NormalizeEmail(account.Email)
	name := strings.TrimSpace(account.Name)
	if problems := seedProblems(account, name, email, s.password); len(problems) > 0 {
		return SeededAccount{}, &ValidationError{Fields: problems}
	}
	encoded, err := s.seededHash(ctx)
	if err != nil {
		return SeededAccount{}, err
	}
	user, err := core.NewID[domain.UserID](deps.Random)
	if err != nil {
		return SeededAccount{}, err
	}
	err = deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		inserted, err := repos.Users.CreateIfEmailFree(ctx, repositories.SignUp{
			ID: user, Role: account.Role, Email: email, PasswordHash: encoded, Name: name, Language: account.Language,
			Timezone: resolveZone(account.Timezone), Theme: domain.ThemeSystem, ConsentVersion: string(domain.CurrentConsentVersion), CreatedAt: now,
		})
		if err != nil {
			return err
		}
		if !inserted {
			return ErrSeedEmailTaken
		}
		if account.Role == domain.RoleTutor {
			if err := repos.TutorProfiles.Create(ctx, repositories.NewTutorProfile{
				UserID: user, FileQuotaBytes: deps.TutorQuotas.FileBytes, RecordingQuotaBytes: deps.TutorQuotas.RecordingBytes, CreatedAt: now,
			}, deps.Random); err != nil {
				return err
			}
		}
		confirmed, err := repos.Users.ConfirmEmail(ctx, user, now)
		if err != nil {
			return err
		}
		if !confirmed {
			return errCredentialsChanged
		}
		return nil
	})
	if err != nil {
		return SeededAccount{}, err
	}
	deps.Logger.LogAttrs(ctx, slog.LevelInfo, "testcontrol.account.seeded", userAttribute(user), slog.String("role", string(account.Role)))
	signIn, err := s.startSeededSession(ctx, user, encoded)
	if err != nil {
		return SeededAccount{}, err
	}
	return SeededAccount{User: user, SignIn: signIn}, nil
}

func (s *Seeder) SignIn(ctx context.Context, email string) (SignIn, error) {
	encoded, err := s.seededHash(ctx)
	if err != nil {
		return SignIn{}, err
	}
	user, err := s.service.deps.Store.Repositories().Users.ByEmail(ctx, domain.NormalizeEmail(email))
	if errors.Is(err, repositories.ErrNotFound) {
		return SignIn{}, ErrNotSeeded
	}
	if err != nil {
		return SignIn{}, err
	}
	if subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(encoded)) != 1 {
		return SignIn{}, ErrNotSeeded
	}
	return s.startSeededSession(ctx, user.ID, encoded)
}

func (s *Seeder) startSeededSession(ctx context.Context, user domain.UserID, encoded string) (SignIn, error) {
	now, err := s.service.deps.Clock.Instant(ctx)
	if err != nil {
		return SignIn{}, err
	}
	signIn, err := s.service.startSession(ctx, user, encoded, now)
	if errors.Is(err, errCredentialsChanged) {
		return SignIn{}, ErrNotSeeded
	}
	if err != nil {
		return SignIn{}, err
	}
	s.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "testcontrol.session.seeded", userAttribute(user))
	return signIn, nil
}

func seedProblems(account SeedAccount, name string, email string, password string) []FieldProblem {
	var problems []FieldProblem
	if account.Role != domain.RoleTutor && account.Role != domain.RoleStudent {
		problems = append(problems, FieldProblem{Path: fieldRole, Code: domain.FieldInvalid})
	}
	if account.Language != domain.LanguageRussian && account.Language != domain.LanguageEnglish {
		problems = append(problems, FieldProblem{Path: fieldLanguage, Code: domain.FieldInvalid})
	}
	return append(problems, signUpProblems(name, email, password, true)...)
}

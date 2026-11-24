package operators

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	listLimit      = 1000
	maxQuotaBytes  = 1 << 50
	likeEscape     = `\`
	likeWildcards  = `%_`
	likeAnySubtext = "%"
)

var (
	ErrUnknownAccount  = errors.New("no account has this email")
	ErrImpossibleState = errors.New("the account's state does not allow this change")
	ErrWrongRole       = errors.New("the account does not have the role this command needs")
	ErrInvalidQuota    = errors.New("a quota must be between 0 and 2^50 bytes")
)

type Filter struct {
	Role  *domain.Role
	State *domain.AccountState
	Email *string
}

type Account struct {
	ID        domain.UserID
	Role      domain.Role
	State     domain.AccountState
	Email     string
	CreatedAt time.Time
}

type Service struct {
	store   *repositories.Store
	clock   clock.Source
	logger  *slog.Logger
	exports ExportWithdrawer
}

type ExportWithdrawer interface {
	WithdrawOwn(ctx context.Context, repos repositories.Repositories, user domain.UserID, cause domain.ExportInvalidationCause, now time.Time) error
}

func New(store *repositories.Store, source clock.Source, logger *slog.Logger, exports ExportWithdrawer) *Service {
	if exports == nil {
		panic("the operator service needs the export effects")
	}
	return &Service{store: store, clock: source, logger: logger, exports: exports}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Account, error) {
	query := repositories.OperatorFilter{RowLimit: listLimit}
	if filter.Role != nil {
		role := string(*filter.Role)
		query.Role = &role
	}
	if filter.State != nil {
		state := string(*filter.State)
		query.State = &state
	}
	if filter.Email != nil {
		pattern := likeAnySubtext + escapeLike(strings.ToLower(strings.TrimSpace(*filter.Email))) + likeAnySubtext
		query.EmailPattern = &pattern
	}
	rows, err := s.store.Repositories().Users.ForOperator(ctx, query)
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, Account{ID: row.ID, Role: row.Role, State: row.State, Email: row.Email, CreatedAt: row.CreatedAt})
	}
	return accounts, nil
}

func escapeLike(text string) string {
	var escaped strings.Builder
	for _, character := range text {
		if strings.ContainsRune(likeEscape+likeWildcards, character) {
			escaped.WriteString(likeEscape)
		}
		escaped.WriteRune(character)
	}
	return escaped.String()
}

func (s *Service) Block(ctx context.Context, email string) (domain.UserID, error) {
	return s.change(ctx, email, domain.ActionBlock, func(repos repositories.Repositories, target repositories.OperatorTarget, now time.Time) error {
		blocked, err := repos.Users.Block(ctx, target.ID, now)
		if err != nil {
			return err
		}
		if !blocked {
			return ErrImpossibleState
		}
		if err := lockTutorProfile(ctx, repos, target); err != nil {
			return err
		}
		if _, err := repos.Sessions.RevokeAllOf(ctx, target.ID, now); err != nil {
			return err
		}
		if _, err := repos.EmailTokens.DeleteOpenRequests(ctx, target.ID); err != nil {
			return err
		}
		if _, err := repos.EmailTokens.DiscardAllUnused(ctx, target.ID); err != nil {
			return err
		}
		return s.exports.WithdrawOwn(ctx, repos, target.ID, domain.ExportCauseAccountBlocked, now)
	})
}

func (s *Service) Unblock(ctx context.Context, email string) (domain.UserID, error) {
	return s.change(ctx, email, domain.ActionUnblock, func(repos repositories.Repositories, target repositories.OperatorTarget, now time.Time) error {
		if target.StateBeforeBlock == nil {
			return ErrImpossibleState
		}
		if _, ok := domain.UnblockTarget(*target.StateBeforeBlock); !ok {
			return ErrImpossibleState
		}
		unblocked, err := repos.Users.Unblock(ctx, target.ID, now)
		if err != nil {
			return err
		}
		if !unblocked {
			return ErrImpossibleState
		}
		return lockTutorProfile(ctx, repos, target)
	})
}

func (s *Service) Restore(ctx context.Context, email string) (domain.UserID, error) {
	return s.change(ctx, email, domain.ActionRestore, func(repos repositories.Repositories, target repositories.OperatorTarget, now time.Time) error {
		restored, err := repos.Users.Restore(ctx, target.ID, now, now.Add(-domain.DeletedAccountRetention))
		if err != nil {
			return err
		}
		if !restored {
			return ErrImpossibleState
		}
		return lockTutorProfile(ctx, repos, target)
	})
}

func lockTutorProfile(ctx context.Context, repos repositories.Repositories, target repositories.OperatorTarget) error {
	if target.Role != domain.RoleTutor {
		return nil
	}
	return repos.TutorProfiles.Lock(ctx, target.ID)
}

func (s *Service) SetQuota(ctx context.Context, email string, files int64, recordings int64) (domain.UserID, error) {
	if files < 0 || recordings < 0 || files > maxQuotaBytes || recordings > maxQuotaBytes {
		return domain.UserID{}, ErrInvalidQuota
	}
	now, err := s.clock.Instant(ctx)
	if err != nil {
		return domain.UserID{}, err
	}
	var user domain.UserID
	err = s.store.WithTx(ctx, func(repos repositories.Repositories) error {
		target, err := s.lock(ctx, repos, email)
		if err != nil {
			return err
		}
		user = target.ID
		if target.Role != domain.RoleTutor {
			return ErrWrongRole
		}
		updated, err := repos.TutorProfiles.SetQuotas(ctx, target.ID, files, recordings, now)
		if err != nil {
			return err
		}
		if !updated {
			return ErrWrongRole
		}
		return nil
	})
	if err != nil {
		return user, err
	}
	s.logger.LogAttrs(ctx, slog.LevelInfo, "account.quota_changed", userAttribute(user), slog.Int64("file_quota_bytes", files), slog.Int64("recording_quota_bytes", recordings))
	return user, nil
}

func (s *Service) change(ctx context.Context, email string, action domain.OperatorAction, apply func(repositories.Repositories, repositories.OperatorTarget, time.Time) error) (domain.UserID, error) {
	now, err := s.clock.Instant(ctx)
	if err != nil {
		return domain.UserID{}, err
	}
	var user domain.UserID
	err = s.store.WithTx(ctx, func(repos repositories.Repositories) error {
		target, err := s.lock(ctx, repos, email)
		if err != nil {
			return err
		}
		user = target.ID
		if !domain.AllowedFrom(action, target.State) {
			return ErrImpossibleState
		}
		return apply(repos, target, now)
	})
	if err != nil {
		return user, err
	}
	s.logger.LogAttrs(ctx, slog.LevelInfo, "account."+actionEvent(action), userAttribute(user))
	return user, nil
}

func actionEvent(action domain.OperatorAction) string {
	switch action {
	case domain.ActionBlock:
		return "blocked"
	case domain.ActionUnblock:
		return "unblocked"
	case domain.ActionRestore:
		return "restored"
	}
	return string(action)
}

func (s *Service) lock(ctx context.Context, repos repositories.Repositories, email string) (repositories.OperatorTarget, error) {
	target, err := repos.Users.LockByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, repositories.ErrNotFound) {
		return target, ErrUnknownAccount
	}
	return target, err
}

func userAttribute(user domain.UserID) slog.Attr {
	return slog.String("user_id", user.String())
}

package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

type refusalReason string

const (
	refusedMalformed    refusalReason = "malformed"
	refusedSignature    refusalReason = "signature"
	refusedExpired      refusalReason = "expired"
	refusedAccountState refusalReason = "account_state"
	refusedThrottled    refusalReason = "throttled"
)

type PreferenceDependencies struct {
	Store  *repositories.Store
	Clock  clock.Source
	Tokens core.UnsubscribeTokens
	Keys   core.Keys
	Logger *slog.Logger
}

type Preferences struct {
	deps PreferenceDependencies
}

func NewPreferences(dependencies PreferenceDependencies) *Preferences {
	return &Preferences{deps: dependencies}
}

type EmailPreference struct {
	Group   domain.EmailGroup
	Enabled bool
}

func (p *Preferences) Of(ctx context.Context, principal accounts.Principal) ([]EmailPreference, error) {
	stored, err := p.deps.Store.Repositories().Preferences.Of(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	return listedFor(principal.Role, stored), nil
}

func (p *Preferences) Save(ctx context.Context, principal accounts.Principal, changes []EmailPreference) ([]EmailPreference, error) {
	if problems := changeProblems(principal.Role, changes); len(problems) > 0 {
		return nil, &accounts.ValidationError{Fields: problems}
	}
	now, err := p.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	var saved []EmailPreference
	var changed []string
	err = p.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		before, err := repos.Preferences.Of(ctx, principal.UserID)
		if err != nil {
			return err
		}
		current := enabledByGroup(before)
		for _, change := range changes {
			if current[change.Group] != change.Enabled {
				changed = append(changed, string(change.Group))
			}
			write := repositories.EmailPreferenceWrite{UserID: principal.UserID, EmailGroup: change.Group, Enabled: change.Enabled, Now: now}
			if err := repos.Preferences.Save(ctx, write); err != nil {
				return err
			}
		}
		after, err := repos.Preferences.Of(ctx, principal.UserID)
		saved = listedFor(principal.Role, after)
		return err
	})
	if err != nil {
		return nil, err
	}
	p.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "email_preferences.updated", slog.String("groups", strings.Join(changed, ",")))
	return saved, nil
}

func (p *Preferences) PreviewUnsubscribe(ctx context.Context, token string) (domain.EmailGroup, error) {
	claims, err := p.admit(ctx, token)
	if err != nil {
		return "", err
	}
	user, err := p.deps.Store.Repositories().Users.ByID(ctx, claims.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return "", p.refuse(ctx, refusedAccountState)
	}
	if err != nil {
		return "", err
	}
	if !usableForUnsubscribe(user.State) || !claims.Group.AllowedFor(user.Role) {
		return "", p.refuse(ctx, refusedAccountState)
	}
	return claims.Group, nil
}

func (p *Preferences) Unsubscribe(ctx context.Context, token string) error {
	claims, err := p.admit(ctx, token)
	if err != nil {
		return err
	}
	now, err := p.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	disabled, err := p.deps.Store.Repositories().Preferences.DisableForUsableAccount(ctx, claims.UserID, claims.Group, now)
	if err != nil {
		return err
	}
	if !disabled {
		return p.refuse(ctx, refusedAccountState)
	}
	p.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "unsubscribe.applied",
		slog.String("target_user_id", claims.UserID.String()), slog.String("group", string(claims.Group)))
	return nil
}

func (p *Preferences) admit(ctx context.Context, token string) (core.UnsubscribeClaims, error) {
	now, err := p.deps.Clock.Instant(ctx)
	if err != nil {
		return core.UnsubscribeClaims{}, err
	}
	claims, verifyErr := p.deps.Tokens.Verify(token, now)
	switch {
	case errors.Is(verifyErr, core.ErrUnsubscribeTokenMalformed):
		return claims, p.refuse(ctx, refusedMalformed)
	case errors.Is(verifyErr, core.ErrUnsubscribeTokenSignature):
		return claims, p.refuse(ctx, refusedSignature)
	case verifyErr != nil && !errors.Is(verifyErr, core.ErrUnsubscribeTokenExpired):
		return claims, verifyErr
	}
	if err := p.reserve(ctx, claims, now); err != nil {
		return claims, err
	}
	if verifyErr != nil {
		return claims, p.refuse(ctx, refusedExpired)
	}
	return claims, nil
}

func (p *Preferences) reserve(ctx context.Context, claims core.UnsubscribeClaims, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, domain.UnsubscribeLimitWindow)
	subject := p.deps.Keys.ThrottleSubject(string(domain.PurposeUnsubscribe), claims.UserID.String()+"/"+string(claims.Group))
	count, err := p.deps.Store.Repositories().RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: domain.PurposeUnsubscribe, Subject: subject, WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > domain.UnsubscribeLimitPerWindow {
		p.logRefusal(ctx, refusedThrottled)
		return &accounts.ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (p *Preferences) refuse(ctx context.Context, reason refusalReason) error {
	p.logRefusal(ctx, reason)
	return fmt.Errorf("unsubscribe refused (%s): %w", reason, accounts.ErrTokenInvalid)
}

func (p *Preferences) logRefusal(ctx context.Context, reason refusalReason) {
	level := slog.LevelInfo
	if reason == refusedMalformed || reason == refusedSignature {
		level = slog.LevelDebug
	}
	p.deps.Logger.LogAttrs(ctx, level, "unsubscribe.refused", slog.String("reason", string(reason)))
}

func usableForUnsubscribe(state domain.AccountState) bool {
	return state == domain.AccountActive || state == domain.AccountUnverified
}

func listedFor(role domain.Role, stored []repositories.EmailPreference) []EmailPreference {
	enabled := enabledByGroup(stored)
	groups := domain.EmailGroupsOf(role)
	listed := make([]EmailPreference, 0, len(groups))
	for _, group := range groups {
		listed = append(listed, EmailPreference{Group: group, Enabled: enabled[group]})
	}
	return listed
}

func enabledByGroup(stored []repositories.EmailPreference) map[domain.EmailGroup]bool {
	enabled := map[domain.EmailGroup]bool{}
	for _, role := range []domain.Role{domain.RoleTutor, domain.RoleStudent} {
		for _, group := range domain.EmailGroupsOf(role) {
			enabled[group] = true
		}
	}
	for _, row := range stored {
		enabled[row.EmailGroup] = row.Enabled
	}
	return enabled
}

func changeProblems(role domain.Role, changes []EmailPreference) []accounts.FieldProblem {
	var problems []accounts.FieldProblem
	seen := map[domain.EmailGroup]bool{}
	for index, change := range changes {
		path := fmt.Sprintf("body.groups[%d].group", index)
		switch {
		case !change.Group.AllowedFor(role) && change.Group.Known():
			problems = append(problems, accounts.FieldProblem{Path: path, Code: domain.FieldNotAllowed})
		case !change.Group.Known() || seen[change.Group]:
			problems = append(problems, accounts.FieldProblem{Path: path, Code: domain.FieldInvalid})
		}
		seen[change.Group] = true
	}
	return problems
}

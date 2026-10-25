package accounts

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

type Access struct {
	UserID    domain.UserID
	SessionID domain.SessionID
	ExpiresAt time.Time
}

type SocketRefusal string

const (
	SocketAllowed        SocketRefusal = ""
	SocketSessionEnded   SocketRefusal = "session"
	SocketAccountRefused SocketRefusal = "account_state"
)

type SessionStanding struct {
	UserID  domain.UserID
	Role    domain.Role
	Refusal SocketRefusal
}

func (s *Service) VerifyAccess(ctx context.Context, token string) (Access, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Access{}, err
	}
	claims, err := s.deps.Tokens.Verify(token, now)
	if errors.Is(err, core.ErrAccessTokenExpired) {
		return Access{}, ErrTokenExpired
	}
	if err != nil {
		return Access{}, ErrUnauthenticated
	}
	return Access{UserID: claims.UserID, SessionID: claims.SessionID, ExpiresAt: claims.ExpiresAt}, nil
}

func (s *Service) SessionStandings(ctx context.Context, sessions []domain.SessionID) (map[domain.SessionID]SessionStanding, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Store.Repositories().Sessions.Standings(ctx, sessions)
	if err != nil {
		return nil, err
	}
	standings := make(map[domain.SessionID]SessionStanding, len(sessions))
	for _, session := range sessions {
		standings[session] = SessionStanding{Refusal: SocketSessionEnded}
	}
	for _, row := range rows {
		standing := SessionStanding{UserID: row.UserID, Role: row.Role}
		switch {
		case row.State != domain.AccountActive && row.State != domain.AccountUnverified:
			standing.Refusal = SocketAccountRefused
		case row.RevokedAt != nil || !now.Before(row.ExpiresAt):
			standing.Refusal = SocketSessionEnded
		}
		standings[row.ID] = standing
	}
	return standings, nil
}

package library

import (
	"context"

	"ovc/backend/internal/domain"
)

func (s *Service) HasTemplate(ctx context.Context, tutor domain.UserID) (bool, error) {
	counts, err := s.deps.Store.Repositories().Library.CountTemplates(ctx, tutor)
	if err != nil {
		return false, err
	}
	return counts.Live > 0, nil
}

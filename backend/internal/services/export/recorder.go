package export

import (
	"bytes"
	"context"
	"slices"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type purgeDueRecorder struct {
	earliest *time.Time
}

func (r *purgeDueRecorder) record(ctx context.Context, repos repositories.Repositories, referenced []domain.UserID) error {
	if len(referenced) == 0 {
		return nil
	}
	accounts := slices.Clone(referenced)
	slices.SortFunc(accounts, func(a, b domain.UserID) int { return bytes.Compare(a[:], b[:]) })
	due, err := repos.Users.EarliestPurgeDue(ctx, slices.Compact(accounts), domain.DeletedAccountRetention)
	if err != nil {
		return err
	}
	if due != nil && (r.earliest == nil || due.Before(*r.earliest)) {
		r.earliest = due
	}
	return nil
}

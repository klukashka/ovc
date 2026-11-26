package export

import (
	"context"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type pageReader struct {
	store    *repositories.Store
	clock    clock.Source
	sink     *partSink
	recorder *purgeDueRecorder
}

func (r pageReader) page(ctx context.Context, read func(repositories.Repositories, time.Time) ([]domain.UserID, error)) error {
	if err := r.sink.checkpoint(false); err != nil {
		return err
	}
	now, err := r.clock.Instant(ctx)
	if err != nil {
		return err
	}
	return r.store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		referenced, err := read(repos, now)
		if err != nil {
			return err
		}
		return r.recorder.record(ctx, repos, referenced)
	})
}

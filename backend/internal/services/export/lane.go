package export

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
)

var ErrNothingToClaim = errors.New("no export can start now")

type Claim struct {
	Export  repositories.Export
	Build   domain.ExportBuildID
	Object  string
	Started time.Time
}

func (s *Service) ClaimBuild(ctx context.Context, now time.Time, slot int) (func(context.Context) error, error) {
	claim, err := s.Claim(ctx, now, slot)
	if errors.Is(err, ErrNothingToClaim) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { return s.Build(ctx, claim) }, nil
}

func (s *Service) Claim(ctx context.Context, now time.Time, slot int) (Claim, error) {
	for {
		claim, retry, err := s.claimOnce(ctx, now)
		if err != nil || !retry {
			if err == nil {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.build.started", exportAttribute(claim.Export.ID), buildAttribute(&claim.Build), slog.Int("slot", slot))
			}
			return claim, err
		}
	}
}

func (s *Service) claimOnce(ctx context.Context, now time.Time) (Claim, bool, error) {
	claim := Claim{}
	retry := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Exports.ClaimQueued(ctx)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNothingToClaim
		}
		if err != nil {
			return err
		}
		owner, err := repos.Users.ByID(ctx, row.UserID)
		if err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		if err != nil || owner.State != domain.AccountActive || owner.EmailVerifiedAt == nil {
			retry = true
			_, err := repos.Exports.Delete(ctx, row.ID)
			return err
		}
		build, err := core.NewID[domain.ExportBuildID](s.deps.Random)
		if err != nil {
			return err
		}
		storageKey := domain.ExportStorageKey(build)
		if _, err := repos.Exports.StartBuild(ctx, row.ID, build, storageKey, now); err != nil {
			return err
		}
		if err := repos.UploadIntents.Create(ctx, build.Identity(), storageKey, domain.UploadCapacityExport, now); err != nil {
			if errors.Is(err, repositories.ErrUploadCapacity) {
				return ErrNothingToClaim
			}
			return err
		}
		claim = Claim{Export: row, Build: build, Object: storageKey, Started: now}
		return nil
	})
	if errors.Is(err, ErrNothingToClaim) {
		return Claim{}, false, ErrNothingToClaim
	}
	return claim, retry, err
}

type outcome struct {
	failure domain.ExportFailure
	stopped bool
}

func (s *Service) Build(ctx context.Context, claim Claim) error {
	started := s.deps.Wall()
	bounded, cancel := context.WithTimeout(ctx, s.deps.Timing.Budget)
	defer cancel()
	size, upload, err := s.writeArchive(bounded, claim, started)
	if err == nil {
		err = s.complete(bounded, claim, upload)
	}
	if err == nil {
		return s.publish(ctx, claim, size, started)
	}
	result := classify(err)
	if bounded.Err() != nil && ctx.Err() == nil && !result.stopped {
		result.failure = domain.ExportFailureTooLong
	}
	if result.stopped {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.build.stopped", exportAttribute(claim.Export.ID), buildAttribute(&claim.Build))
		return nil
	}
	return s.fail(ctx, claim, result.failure, err, started)
}

func classify(err error) outcome {
	switch {
	case errors.Is(err, errBuildStopped):
		return outcome{stopped: true}
	case errors.Is(err, exportformat.ErrTooLarge):
		return outcome{failure: domain.ExportFailureTooLarge}
	case errors.Is(err, errBudgetPassed):
		return outcome{failure: domain.ExportFailureTooLong}
	}
	return outcome{failure: domain.ExportFailureError}
}

func (s *Service) settlement(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), domain.UploadSettlementBudget)
}

func (s *Service) recordUpload(ctx context.Context, claim Claim, upload storage.MultipartUploadID) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(ctx, claim.Export.ID)
		if err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		if _, err := repos.UploadIntents.Lock(ctx, claim.Build.Identity()); err != nil {
			return err
		}
		if err := repos.UploadIntents.RecordMultipart(ctx, claim.Build.Identity(), claim.Object, string(upload), now); err != nil {
			return err
		}
		if !stillBuilding(row, err == nil, claim.Build) {
			return errBuildStopped
		}
		return nil
	})
}

func stillBuilding(row repositories.Export, found bool, build domain.ExportBuildID) bool {
	return found && row.State == domain.ExportBuilding && row.BuildID != nil && *row.BuildID == build
}

func (s *Service) complete(ctx context.Context, claim Claim, upload uploadState) error {
	if err := s.authorizeComplete(ctx, claim, upload.id); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, s.deps.Timing.Complete)
	defer cancel()
	return s.deps.Objects.CompleteMultipartUpload(bounded, claim.Object, upload.id, upload.parts)
}

func (s *Service) authorizeComplete(ctx context.Context, claim Claim, upload storage.MultipartUploadID) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(ctx, claim.Export.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errBuildStopped
		}
		if err != nil {
			return err
		}
		intent, err := repos.UploadIntents.Lock(ctx, claim.Build.Identity())
		if err != nil {
			return err
		}
		if !stillBuilding(row, true, claim.Build) || intent.CleanupRequested {
			return errBuildStopped
		}
		return repos.UploadIntents.AuthorizeComplete(ctx, claim.Build.Identity(), claim.Object, string(upload), now)
	})
}

func (s *Service) publish(ctx context.Context, claim Claim, size int64, started time.Time) error {
	settled, cancel := s.settlement(ctx)
	defer cancel()
	now, err := s.deps.Clock.Instant(settled)
	if err != nil {
		return err
	}
	published := false
	err = s.deps.Store.WithTx(settled, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(settled, claim.Export.ID)
		found := err == nil
		if err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		if _, err := repos.Exports.LockLimit(settled, claim.Export.UserID); err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		intent, err := repos.UploadIntents.Lock(settled, claim.Build.Identity())
		if err != nil {
			return err
		}
		if !stillBuilding(row, found, claim.Build) || intent.CleanupRequested {
			return s.deps.Cleanup.ConfirmAndCleanExport(settled, repos, claim.Build, now)
		}
		closing, err := repos.Exports.EarliestDeletedTutorClosing(settled, row.UserID)
		if err != nil {
			return err
		}
		if closing != nil && !now.Before(*closing) {
			return s.withdrawAtTutorClosing(settled, repos, row, claim.Build, now)
		}
		expires := now.Add(domain.ExportRetention)
		if closing != nil && closing.Before(expires) {
			expires = *closing
		}
		if err := repos.UploadIntents.ConfirmComplete(settled, claim.Build.Identity(), claim.Object, now); err != nil {
			return err
		}
		if _, err := repos.Exports.Publish(settled, row.ID, claim.Build, size, expires, now); err != nil {
			return err
		}
		if err := repos.Exports.RecordLimit(settled, row.UserID, row.ID, row.RequestedAt.Add(domain.ExportStartInterval)); err != nil {
			return err
		}
		if err := repos.UploadIntents.Remove(settled, claim.Build.Identity(), claim.Object, now); err != nil {
			return err
		}
		published = true
		return s.notifyOwner(settled, repos, row, claim.Build, domain.NotificationExportReady, now)
	})
	if err != nil {
		return err
	}
	if published {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.built", exportAttribute(claim.Export.ID), buildAttribute(&claim.Build),
			userAttribute(claim.Export.UserID), slog.Int64("size_bytes", size), slog.Int("files", 0), slog.Int("files_unreadable", 0),
			slog.Int64("duration_ms", s.deps.Wall().Sub(started).Milliseconds()))
	}
	return nil
}

func (s *Service) withdrawAtTutorClosing(ctx context.Context, repos repositories.Repositories, row repositories.Export, build domain.ExportBuildID, now time.Time) error {
	if _, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureWithdrawn, now); err != nil {
		return err
	}
	if err := s.deps.Cleanup.ConfirmAndCleanExport(ctx, repos, build, now); err != nil {
		return err
	}
	if err := s.notifyOwner(ctx, repos, row, build, domain.NotificationExportFailed, now); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.invalidated", exportAttribute(row.ID), buildAttribute(&build),
			userAttribute(row.UserID), slog.String("state", string(row.State)), slog.String("cause", string(domain.ExportCauseTutorDeleted)))
	})
	return nil
}

func (s *Service) fail(ctx context.Context, claim Claim, failure domain.ExportFailure, cause error, started time.Time) error {
	settled, cancel := s.settlement(ctx)
	defer cancel()
	now, err := s.deps.Clock.Instant(settled)
	if err != nil {
		return errors.Join(cause, err)
	}
	failed := false
	err = s.deps.Store.WithTx(settled, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(settled, claim.Export.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !stillBuilding(row, true, claim.Build) {
			return nil
		}
		if _, err := repos.Exports.Fail(settled, row.ID, failure, now); err != nil {
			return err
		}
		if err := s.requestCleanupIfOpen(settled, repos, claim.Build, now); err != nil {
			return err
		}
		failed = true
		return s.notifyOwner(settled, repos, row, claim.Build, domain.NotificationExportFailed, now)
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	if failed {
		level := slog.LevelWarn
		attributes := []slog.Attr{exportAttribute(claim.Export.ID), buildAttribute(&claim.Build), userAttribute(claim.Export.UserID),
			slog.String("failure", string(failure)), slog.Int64("duration_ms", s.deps.Wall().Sub(started).Milliseconds())}
		if failure == domain.ExportFailureError {
			level = slog.LevelError
			attributes = append(attributes, slog.String("kind", core.ErrorKind(cause)))
		}
		s.deps.Logger.LogAttrs(ctx, level, "export.failed", attributes...)
	}
	return nil
}

func (s *Service) notifyOwner(ctx context.Context, repos repositories.Repositories, row repositories.Export, build domain.ExportBuildID, event domain.NotificationEvent, now time.Time) error {
	owner, err := repos.Users.ByID(ctx, row.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner.State != domain.AccountActive {
		return nil
	}
	id, err := core.NewID[domain.NotificationID](s.deps.Random)
	if err != nil {
		return err
	}
	inserted, err := repos.Notifications.Insert(ctx, repositories.NewNotification{
		ID: id, UserID: row.UserID, Event: event, SubjectID: domain.EntityID(row.ID),
		IdempotencyKey: domain.ExportNotificationKey(event, row.ID, build), CreatedAt: now,
	})
	if err != nil || !inserted {
		return err
	}
	if err := repos.Signals.Notification(ctx, row.UserID, id); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "notification.created", slog.String("event", string(event)),
			slog.String("target_user_id", row.UserID.String()), slog.String("notification_id", id.String()))
	})
	return nil
}

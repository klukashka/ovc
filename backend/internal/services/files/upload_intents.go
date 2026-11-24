package files

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type UploadTiming struct {
	Settlement time.Duration
	Recovery   time.Duration
	Candidate  time.Duration
	Effect     time.Duration
}

func (t UploadTiming) withDefaults() UploadTiming {
	if t.Settlement <= 0 {
		t.Settlement = domain.UploadSettlementBudget
	}
	if t.Recovery <= 0 {
		t.Recovery = domain.UploadRecoveryBudget
	}
	if t.Candidate <= 0 {
		t.Candidate = domain.UploadRecoveryCandidateBudget
	}
	if t.Effect <= 0 {
		t.Effect = domain.UploadSettlementBudget
	}
	return t
}

func (s *Service) settlementContext(ctx context.Context, id domain.AttachmentID) (context.Context, context.CancelFunc) {
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.deps.Uploads.Settlement)
	stop := context.AfterFunc(s.deps.Lifetime, cancel)
	return bounded, func() {
		if errors.Is(bounded.Err(), context.DeadlineExceeded) {
			s.deps.Logger.LogAttrs(bounded, slog.LevelWarn, "upload.settlement.deadline", attachmentAttribute(id))
		}
		stop()
		cancel()
	}
}

func lockUpload(ctx context.Context, repos repositories.Repositories, id domain.AttachmentID) (repositories.Attachment, bool, error) {
	row, err := repos.Attachments.Lock(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return row, false, nil
	}
	return row, err == nil, err
}

func (s *Service) recordUploadID(ctx context.Context, opened openUpload, now time.Time) (bool, error) {
	allowed := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, found, err := lockUpload(ctx, repos, opened.id)
		if err != nil {
			return err
		}
		intent, err := repos.UploadIntents.Lock(ctx, opened.id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := repos.UploadIntents.RecordMultipart(ctx, opened.id, opened.key, string(opened.upload), now); err != nil {
			return err
		}
		if !found || row.State != domain.AttachmentUploading || intent.CleanupRequested {
			return nil
		}
		allowed, err = repos.Attachments.RecordMultipartUpload(ctx, opened.id, string(opened.upload), now)
		return err
	})
	return allowed, err
}

func (s *Service) authorizeComplete(ctx context.Context, opened openUpload) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, found, err := lockUpload(ctx, repos, opened.id)
		if err != nil {
			return err
		}
		if !found || row.State != domain.AttachmentUploading || row.StorageKey != opened.key {
			return ErrOutOfScope
		}
		policy, known := uploadPolicyOf(row.Context)
		if !known {
			return fmt.Errorf("%w: %s", errUnregisteredContext, row.Context)
		}
		opened.created = row.CreatedAt
		if err := s.checkDailyActivation(ctx, repos, policy, opened, now); err != nil {
			return err
		}
		if _, err := repos.UploadIntents.Lock(ctx, opened.id); err != nil {
			return err
		}
		return repos.UploadIntents.AuthorizeComplete(ctx, opened.id, opened.key, string(opened.upload), now)
	})
}

func (s *Service) reconcileUpload(ctx context.Context, repos repositories.Repositories, opened openUpload, completed bool, now time.Time) error {
	row, found, err := lockUpload(ctx, repos, opened.id)
	if err != nil {
		return err
	}
	intent, err := repos.UploadIntents.Lock(ctx, opened.id)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if intent.StorageKey != opened.key {
		return repositories.ErrUploadTransition
	}
	if found && (row.State == domain.AttachmentPending || row.State == domain.AttachmentBound) {
		return repositories.ErrUploadTransition
	}
	if completed {
		if err := repos.UploadIntents.ConfirmComplete(ctx, opened.id, opened.key, now); err != nil {
			return err
		}
	}
	if found && row.State != domain.AttachmentDeleted {
		if _, err := repos.Attachments.MarkDeleted(ctx, opened.id, now); err != nil {
			return err
		}
	}
	if err := repos.UploadIntents.Revoke(ctx, opened.id, now); err != nil {
		return err
	}
	intent, err = repos.UploadIntents.Lock(ctx, opened.id)
	if err != nil {
		return err
	}
	return s.queueIntentCleanup(ctx, repos, intent, now)
}

func (s *Service) queueIntentCleanup(ctx context.Context, repos repositories.Repositories, intent repositories.UploadIntent, now time.Time) error {
	if !intent.CleanupRequested {
		return nil
	}
	generation := domain.UploadInitialCleanup
	if intent.CleanupGeneration != nil {
		if *intent.CleanupGeneration != domain.UploadInitialCleanup || !intent.FinalCleanupRequired {
			return nil
		}
		generation = domain.UploadFinalCleanup
	}
	active, err := s.activeManualCleanup(ctx, repos, intent.AttachmentID, now)
	if err != nil || active != nil {
		return err
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	effect := intentEffect{subject: intent.SubjectKind, id: intent.AttachmentID, key: intent.StorageKey, generation: &generation}
	if err := repos.UploadIntents.Queue(ctx, intent.AttachmentID, generation, id, now); err != nil {
		return err
	}
	if err := s.insertIntentEffect(ctx, repos, id, effect, effect.cleanupKey(generation), now); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.cleanup.requested", intentAttributes(intent.SubjectKind, intent.AttachmentID, slog.Int("generation", int(generation)))...)
	})
	return nil
}

func (s *Service) insertIntentEffect(ctx context.Context, repos repositories.Repositories, id domain.OutboxID, effect intentEffect, key string, now time.Time) error {
	kind, encoded, err := effect.outboxPayload()
	if err != nil {
		return err
	}
	inserted, err := repos.Outbox.Insert(ctx, repositories.NewOutboxRow{ID: id, Lane: domain.LaneStorageDelete, Priority: domain.PriorityNormal, Kind: kind, Payload: encoded, IdempotencyKey: key, NextAttemptAt: now, CreatedAt: now})
	if err != nil {
		return err
	}
	if !inserted {
		return repositories.ErrUploadTransition
	}
	return nil
}

func (s *Service) activeManualCleanup(ctx context.Context, repos repositories.Repositories, id domain.AttachmentID, now time.Time) (*repositories.UploadCleanupReceipt, error) {
	receipt, err := repos.UploadIntents.ActiveReceipt(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row, err := repos.Outbox.ByID(ctx, receipt.OutboxID)
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	if err == nil && (row.Status == domain.OutboxPending || row.Status == domain.OutboxInProgress) {
		return &receipt, nil
	}
	status := domain.OutboxDead
	if err == nil {
		status = row.Status
	}
	return nil, repos.UploadIntents.UpdateReceipt(ctx, id, receipt.OutboxID, status, now)
}

package files

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var ErrUploadCleanupActive = errors.New("an upload cleanup cycle is already active")

type UploadCleanupCycle struct {
	Attempt domain.UploadAttemptID
	Outbox  domain.OutboxID
	Status  domain.OutboxStatus
}

type UploadInspection struct {
	Attachment           domain.AttachmentID
	Subject              domain.UploadSubjectKind
	Phase                domain.UploadPhase
	CompletionStarted    bool
	CompletionConfirmed  bool
	CleanupRequested     bool
	FinalCleanupRequired bool
	Generation           *domain.UploadCleanupGeneration
	Outbox               *domain.OutboxID
	Result               domain.UploadCleanupResult
	ErrorCode            *string
	ReceiptCount         int64
	Receipts             []UploadCleanupCycle
	Next                 *domain.UploadAttemptID
}

func (s *Service) InspectUpload(ctx context.Context, id domain.AttachmentID, after domain.UploadAttemptID) (UploadInspection, error) {
	repos := s.deps.Store.Repositories()
	intent, err := repos.UploadIntents.ByID(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return UploadInspection{}, ErrNotFound
	}
	if err != nil {
		return UploadInspection{}, err
	}
	receipts, err := repos.UploadIntents.Receipts(ctx, id, after)
	if err != nil {
		return UploadInspection{}, err
	}
	count, err := repos.UploadIntents.ReceiptCount(ctx, id)
	if err != nil {
		return UploadInspection{}, err
	}
	result := UploadInspection{Attachment: id, Subject: intent.SubjectKind, Phase: intent.Phase, CompletionStarted: intent.CompletionStarted, CompletionConfirmed: intent.CompletionConfirmed, CleanupRequested: intent.CleanupRequested, FinalCleanupRequired: intent.FinalCleanupRequired, Generation: intent.CleanupGeneration, Outbox: intent.CleanupOutboxID, Result: intent.CleanupResult, ErrorCode: intent.ErrorCode, ReceiptCount: count}
	for _, receipt := range receipts {
		result.Receipts = append(result.Receipts, cycleOf(receipt))
	}
	if len(receipts) == 100 {
		last := receipts[len(receipts)-1].AttemptID
		result.Next = &last
	}
	return result, nil
}

func cycleOf(receipt repositories.UploadCleanupReceipt) UploadCleanupCycle {
	return UploadCleanupCycle{Attempt: receipt.AttemptID, Outbox: receipt.OutboxID, Status: receipt.Status}
}

func (s *Service) RetryUploadCleanup(ctx context.Context, id domain.AttachmentID, attempt domain.UploadAttemptID) (UploadCleanupCycle, error) {
	bounded, cancel := context.WithTimeout(ctx, s.deps.Uploads.Settlement)
	defer cancel()
	now, err := s.deps.Clock.Instant(bounded)
	if err != nil {
		return UploadCleanupCycle{}, err
	}
	result := UploadCleanupCycle{}
	refused := false
	err = s.deps.Store.WithTx(bounded, func(repos repositories.Repositories) error {
		known, err := repos.UploadIntents.ByID(bounded, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := lockSubject(bounded, repos, known.SubjectKind, id, known.StorageKey, now); err != nil {
			return err
		}
		intent, err := repos.UploadIntents.Lock(bounded, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		previous, err := repos.UploadIntents.Receipt(bounded, id, attempt)
		if err == nil {
			if previous.Status == domain.OutboxPending || previous.Status == domain.OutboxInProgress {
				current, lookupErr := repos.Outbox.ByID(bounded, previous.OutboxID)
				if lookupErr != nil && !errors.Is(lookupErr, repositories.ErrNotFound) {
					return lookupErr
				}
				previous.Status = domain.OutboxDead
				if lookupErr == nil {
					previous.Status = current.Status
				}
				if err := repos.UploadIntents.UpdateReceipt(bounded, id, previous.OutboxID, previous.Status, now); err != nil {
					return err
				}
			}
			result = cycleOf(previous)
			return nil
		}
		if !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		if !intent.CleanupRequested {
			return repositories.ErrUploadTransition
		}
		active, err := s.activeUploadCycle(bounded, repos, intent, now)
		if err != nil {
			return err
		}
		if active != nil {
			result = *active
			refused = true
			return nil
		}
		outbox, err := core.NewID[domain.OutboxID](s.deps.Random)
		if err != nil {
			return err
		}
		if err := repos.UploadIntents.CreateReceipt(bounded, intent, attempt, outbox, now); err != nil {
			return err
		}
		effect := intentEffect{subject: intent.SubjectKind, id: id, key: intent.StorageKey, attempt: &attempt}
		if err := s.insertIntentEffect(bounded, repos, outbox, effect, effect.manualKey(attempt), now); err != nil {
			return err
		}
		result = UploadCleanupCycle{Attempt: attempt, Outbox: outbox, Status: domain.OutboxPending}
		repos.AfterCommit(bounded, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.cleanup.operator_retry", intentAttributes(intent.SubjectKind, id, slog.String("attempt_id", attempt.String()))...)
		})
		return nil
	})
	if err == nil && refused {
		err = ErrUploadCleanupActive
	}
	return result, err
}

func (s *Service) activeUploadCycle(ctx context.Context, repos repositories.Repositories, intent repositories.UploadIntent, now time.Time) (*UploadCleanupCycle, error) {
	manual, err := s.activeManualCleanup(ctx, repos, intent.AttachmentID, now)
	if err != nil {
		return nil, err
	}
	if manual != nil {
		result := cycleOf(*manual)
		return &result, nil
	}
	for _, generation := range []domain.UploadCleanupGeneration{domain.UploadInitialCleanup, domain.UploadFinalCleanup} {
		row, err := repos.Outbox.ByKey(ctx, intentEffect{subject: intent.SubjectKind, id: intent.AttachmentID}.cleanupKey(generation))
		if errors.Is(err, repositories.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if row.Status == domain.OutboxPending || row.Status == domain.OutboxInProgress {
			return &UploadCleanupCycle{Outbox: row.ID, Status: row.Status}, nil
		}
	}
	return nil, nil
}

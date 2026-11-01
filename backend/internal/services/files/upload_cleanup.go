package files

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type intentDeletion struct {
	intent repositories.UploadIntent
	found  bool
}

func (s *Service) deleteUploadIntent(ctx context.Context, row repositories.ClaimedOutboxRow, payload domain.AttachmentPayload, id domain.AttachmentID, now time.Time) (domain.OutboxDoneReason, error) {
	bounded, cancel := context.WithTimeout(ctx, s.deps.Uploads.Effect)
	defer cancel()
	for range 2 {
		deletion, err := s.readIntentDeletion(bounded, row, payload, id)
		if err != nil {
			return "", err
		}
		if !deletion.found {
			return domain.DoneObsolete, nil
		}
		if deletion.intent.MultipartUploadID != nil {
			if err := s.deps.Objects.AbortMultipartUpload(bounded, payload.Key, storage.MultipartUploadID(*deletion.intent.MultipartUploadID)); err != nil {
				return "", err
			}
		}
		if err := s.deps.Objects.Delete(bounded, payload.Key); err != nil {
			return "", err
		}
		repeated, err := s.finishIntentDeletion(bounded, row, payload, deletion.intent, now)
		if err != nil {
			return "", err
		}
		if !repeated {
			return domain.DoneSent, nil
		}
	}
	return "", repositories.ErrUploadTransition
}

func (s *Service) readIntentDeletion(ctx context.Context, row repositories.ClaimedOutboxRow, payload domain.AttachmentPayload, id domain.AttachmentID) (intentDeletion, error) {
	result := intentDeletion{}
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		attachment, found, err := lockUpload(ctx, repos, id)
		if err != nil {
			return err
		}
		if found && (attachment.StorageKey != payload.Key || attachment.State != domain.AttachmentDeleted) {
			return nil
		}
		intent, err := repos.UploadIntents.Lock(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateIntentEffect(ctx, repos, intent, row, payload); err != nil {
			return err
		}
		result = intentDeletion{intent: intent, found: true}
		return nil
	})
	return result, err
}

func validateIntentEffect(ctx context.Context, repos repositories.Repositories, intent repositories.UploadIntent, row repositories.ClaimedOutboxRow, payload domain.AttachmentPayload) error {
	if intent.StorageKey != payload.Key || !intent.CleanupRequested || payload.Upload != "" {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	if (payload.Generation == nil) == (payload.Attempt == nil) {
		return domain.PermanentEffect(errMalformedPayload)
	}
	if payload.Attempt != nil {
		receipt, err := repos.UploadIntents.Receipt(ctx, intent.AttachmentID, *payload.Attempt)
		if err != nil {
			return err
		}
		if receipt.OutboxID != row.ID {
			return domain.PermanentEffect(repositories.ErrUploadTransition)
		}
		return nil
	}
	if *payload.Generation != domain.UploadInitialCleanup && *payload.Generation != domain.UploadFinalCleanup {
		return domain.PermanentEffect(errMalformedPayload)
	}
	if intent.CleanupGeneration == nil || *payload.Generation > *intent.CleanupGeneration {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	expected, err := repos.Outbox.ByKey(ctx, fmt.Sprintf("storage_delete:%s:intent:%d", intent.AttachmentID, *payload.Generation))
	if err != nil {
		return err
	}
	if expected.ID != row.ID {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	return nil
}

func (s *Service) finishIntentDeletion(ctx context.Context, row repositories.ClaimedOutboxRow, payload domain.AttachmentPayload, observed repositories.UploadIntent, now time.Time) (bool, error) {
	repeated := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		attachment, found, err := lockUpload(ctx, repos, observed.AttachmentID)
		if err != nil {
			return err
		}
		if found && (attachment.State != domain.AttachmentDeleted || attachment.StorageKey != payload.Key) {
			return repositories.ErrUploadTransition
		}
		intent, err := repos.UploadIntents.Lock(ctx, observed.AttachmentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateIntentEffect(ctx, repos, intent, row, payload); err != nil {
			return err
		}
		if observed.MultipartUploadID == nil && intent.MultipartUploadID != nil {
			repeated = true
			return nil
		}
		if observed.MultipartUploadID != nil && (intent.MultipartUploadID == nil || *observed.MultipartUploadID != *intent.MultipartUploadID) {
			return domain.PermanentEffect(repositories.ErrUploadTransition)
		}
		if payload.Attempt == nil {
			if err := repos.UploadIntents.Finish(ctx, intent.AttachmentID, row.ID, domain.UploadCleanupDeleted, nil, now); err != nil {
				return err
			}
		}
		current := payload.Attempt != nil || (intent.CleanupOutboxID != nil && *intent.CleanupOutboxID == row.ID)
		terminal := current && !intent.FinalCleanupRequired && (!intent.CompletionStarted || (observed.CompletionConfirmed && intent.CompletionConfirmed))
		if terminal {
			if found {
				if _, err := repos.Attachments.DeleteRow(ctx, intent.AttachmentID); err != nil {
					return err
				}
			}
			if err := repos.UploadIntents.Remove(ctx, intent.AttachmentID, intent.StorageKey, now); err != nil {
				return err
			}
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.cleanup.completed", attachmentAttribute(intent.AttachmentID))
			})
		} else if intent.CompletionStarted && !intent.CompletionConfirmed && intent.CleanupResult != domain.UploadCleanupDeleted {
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "upload.completion.uncertain", attachmentAttribute(intent.AttachmentID))
			})
		}
		return nil
	})
	return repeated, err
}

package files

import (
	"context"
	"errors"
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

func (s *Service) deleteUploadIntent(ctx context.Context, row repositories.ClaimedOutboxRow, effect intentEffect, now time.Time) (domain.OutboxDoneReason, error) {
	bounded, cancel := context.WithTimeout(ctx, s.deps.Uploads.Effect)
	defer cancel()
	for range 2 {
		deletion, err := s.readIntentDeletion(bounded, row, effect)
		if err != nil {
			return "", err
		}
		if !deletion.found {
			return domain.DoneObsolete, nil
		}
		if deletion.intent.MultipartUploadID != nil {
			if err := s.deps.Objects.AbortMultipartUpload(bounded, effect.key, storage.MultipartUploadID(*deletion.intent.MultipartUploadID)); err != nil {
				return "", err
			}
		}
		if err := s.deps.Objects.Delete(bounded, effect.key); err != nil {
			return "", err
		}
		repeated, err := s.finishIntentDeletion(bounded, row, effect, deletion.intent, now)
		if err != nil {
			return "", err
		}
		if !repeated {
			return domain.DoneSent, nil
		}
	}
	return "", repositories.ErrUploadTransition
}

func (s *Service) readIntentDeletion(ctx context.Context, row repositories.ClaimedOutboxRow, effect intentEffect) (intentDeletion, error) {
	result := intentDeletion{}
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := lockSubject(ctx, repos, effect.subject, effect.id, effect.key, time.Time{})
		if err != nil {
			return err
		}
		if subject.keeps {
			return nil
		}
		intent, err := repos.UploadIntents.Lock(ctx, effect.id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateIntentEffect(ctx, repos, intent, row, effect); err != nil {
			return err
		}
		result = intentDeletion{intent: intent, found: true}
		return nil
	})
	return result, err
}

func validateIntentEffect(ctx context.Context, repos repositories.Repositories, intent repositories.UploadIntent, row repositories.ClaimedOutboxRow, effect intentEffect) error {
	if intent.SubjectKind != effect.subject || intent.StorageKey != effect.key || !intent.CleanupRequested || effect.upload != "" {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	if (effect.generation == nil) == (effect.attempt == nil) {
		return domain.PermanentEffect(errMalformedPayload)
	}
	if effect.attempt != nil {
		receipt, err := repos.UploadIntents.Receipt(ctx, intent.AttachmentID, *effect.attempt)
		if err != nil {
			return err
		}
		if receipt.OutboxID != row.ID || receipt.SubjectKind != effect.subject {
			return domain.PermanentEffect(repositories.ErrUploadTransition)
		}
		return nil
	}
	if *effect.generation != domain.UploadInitialCleanup && *effect.generation != domain.UploadFinalCleanup {
		return domain.PermanentEffect(errMalformedPayload)
	}
	if intent.CleanupGeneration == nil || *effect.generation > *intent.CleanupGeneration {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	expected, err := repos.Outbox.ByKey(ctx, effect.cleanupKey(*effect.generation))
	if err != nil {
		return err
	}
	if expected.ID != row.ID {
		return domain.PermanentEffect(repositories.ErrUploadTransition)
	}
	return nil
}

func (s *Service) finishIntentDeletion(ctx context.Context, row repositories.ClaimedOutboxRow, effect intentEffect, observed repositories.UploadIntent, now time.Time) (bool, error) {
	repeated := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := lockSubject(ctx, repos, effect.subject, observed.AttachmentID, effect.key, time.Time{})
		if err != nil {
			return err
		}
		if subject.keeps {
			return repositories.ErrUploadTransition
		}
		intent, err := repos.UploadIntents.Lock(ctx, observed.AttachmentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateIntentEffect(ctx, repos, intent, row, effect); err != nil {
			return err
		}
		if observed.MultipartUploadID == nil && intent.MultipartUploadID != nil {
			repeated = true
			return nil
		}
		if observed.MultipartUploadID != nil && (intent.MultipartUploadID == nil || *observed.MultipartUploadID != *intent.MultipartUploadID) {
			return domain.PermanentEffect(repositories.ErrUploadTransition)
		}
		if effect.attempt == nil {
			if err := repos.UploadIntents.Finish(ctx, intent.AttachmentID, row.ID, domain.UploadCleanupDeleted, nil, now); err != nil {
				return err
			}
		}
		current := effect.attempt != nil || (intent.CleanupOutboxID != nil && *intent.CleanupOutboxID == row.ID)
		terminal := current && !intent.FinalCleanupRequired && (!intent.CompletionStarted || (observed.CompletionConfirmed && intent.CompletionConfirmed))
		attributes := intentAttributes(intent.SubjectKind, intent.AttachmentID)
		if terminal {
			if subject.found && effect.subject == domain.UploadSubjectAttachment {
				if _, err := repos.Attachments.DeleteRow(ctx, intent.AttachmentID); err != nil {
					return err
				}
			}
			if err := repos.UploadIntents.Remove(ctx, intent.AttachmentID, intent.StorageKey, now); err != nil {
				return err
			}
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.cleanup.completed", attributes...)
			})
		} else if intent.CompletionStarted && !intent.CompletionConfirmed && intent.CleanupResult != domain.UploadCleanupDeleted {
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "upload.completion.uncertain", attributes...)
			})
		}
		return nil
	})
	return repeated, err
}

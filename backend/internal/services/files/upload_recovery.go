package files

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) RecoverUploads(ctx context.Context, now time.Time) (int64, error) {
	bounded, cancel := context.WithTimeout(ctx, s.deps.Uploads.Recovery)
	defer cancel()
	if !s.recoveryMutex.TryLock() {
		return 0, nil
	}
	defer s.recoveryMutex.Unlock()
	var processed int64
	var failures error
	for range expireRounds {
		ids, err := s.deps.Store.Repositories().UploadIntents.Candidates(bounded, s.recoveryCursor, uploadingSince(now), expireBatch)
		if err != nil {
			return processed, errors.Join(failures, err)
		}
		if len(ids) == 0 {
			s.recoveryCursor = domain.AttachmentID{}
			break
		}
		for _, found := range ids {
			candidate, done := context.WithTimeout(bounded, s.deps.Uploads.Candidate)
			err := s.recoverUpload(candidate, found.SubjectKind, found.AttachmentID, now)
			done()
			s.recoveryCursor = found.AttachmentID
			if err != nil {
				failures = errors.Join(failures, err)
				s.deps.Logger.LogAttrs(bounded, slog.LevelError, "upload.recovery.failed", intentAttributes(found.SubjectKind, found.AttachmentID, slog.String("kind", "candidate_failed"))...)
			} else {
				processed++
			}
			if bounded.Err() != nil {
				return processed, errors.Join(failures, bounded.Err())
			}
		}
		if len(ids) < expireBatch {
			s.recoveryCursor = domain.AttachmentID{}
			break
		}
	}
	return processed, failures
}

func (s *Service) recoverUpload(ctx context.Context, kind domain.UploadSubjectKind, id domain.AttachmentID, now time.Time) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		subject, err := lockSubject(ctx, repos, kind, id, "", uploadingSince(now))
		if err != nil {
			return err
		}
		intent, err := repos.UploadIntents.Lock(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if intent.SubjectKind != kind {
			return repositories.ErrUploadTransition
		}
		if subject.protected {
			return repositories.ErrUploadTransition
		}
		before := intent
		if !intent.CleanupRequested {
			if subject.live {
				return nil
			}
			if kind == domain.UploadSubjectAttachment && subject.found && !subject.deleted {
				if _, err := repos.Attachments.MarkDeleted(ctx, id, now); err != nil {
					return err
				}
			}
			if err := repos.UploadIntents.Revoke(ctx, id, now); err != nil {
				return err
			}
			intent.CleanupRequested = true
		}
		if err := reconcileIntentResult(ctx, repos, intent, now); err != nil {
			return err
		}
		if _, err := s.activeManualCleanup(ctx, repos, id, now); err != nil {
			return err
		}
		if err := s.queueIntentCleanup(ctx, repos, intent, now); err != nil {
			return err
		}
		after, err := repos.UploadIntents.Lock(ctx, id)
		if err != nil {
			return err
		}
		changed := before.CleanupRequested != after.CleanupRequested || before.CleanupResult != after.CleanupResult || before.FinalCleanupRequired != after.FinalCleanupRequired
		if changed {
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.recovery.completed", intentAttributes(kind, id)...)
			})
		}
		return nil
	})
}

func reconcileIntentResult(ctx context.Context, repos repositories.Repositories, intent repositories.UploadIntent, now time.Time) error {
	if intent.CleanupOutboxID == nil || intent.CleanupResult != domain.UploadCleanupQueued {
		return nil
	}
	row, err := repos.Outbox.ByID(ctx, *intent.CleanupOutboxID)
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return err
	}
	if err == nil && (row.Status == domain.OutboxPending || row.Status == domain.OutboxInProgress) {
		return nil
	}
	result, code := domain.UploadCleanupFailed, "effect_failed"
	if errors.Is(err, repositories.ErrNotFound) {
		code = "outbox_missing"
	}
	if err == nil && row.Status == domain.OutboxDone && row.DoneReason != nil && *row.DoneReason == domain.DoneSent {
		return repos.UploadIntents.Finish(ctx, intent.AttachmentID, row.ID, domain.UploadCleanupDeleted, nil, now)
	}
	if err == nil && row.LastErrorCode != nil && (*row.LastErrorCode == "permanent" || *row.LastErrorCode == "lease_expired") {
		code = *row.LastErrorCode
	}
	return repos.UploadIntents.Finish(ctx, intent.AttachmentID, *intent.CleanupOutboxID, result, &code, now)
}

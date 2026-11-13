package homework

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

type studentPurgeBatch struct {
	limit                int64
	referencesRemoved    int64
	revisionsRemoved     int64
	answersRemoved       int64
	notificationsRemoved int64
	submissionsRemoved   int64
	revisions            map[domain.SubmissionID][]domain.ReviewRevisionID
	answerAttachments    []domain.AttachmentID
	reviewAttachments    []domain.AttachmentID
}

func (b *studentPurgeBatch) full() bool {
	return b.referencesRemoved == b.limit || b.revisionsRemoved == b.limit || b.answersRemoved == b.limit || b.notificationsRemoved == b.limit || b.submissionsRemoved == b.limit
}

func (b *studentPurgeBatch) remaining(removed int64) int32 {
	left := b.limit - removed
	if left < 0 || left > int64(domain.HomeworkCleanupBatch) {
		panic("student purge exceeded its row budget")
	}
	return int32(left)
}

func (b *studentPurgeBatch) references(ctx context.Context, repos repositories.Repositories, kind domain.ReferenceKind, id domain.EntityID) error {
	removed, err := repos.Homework.RemoveCleanupReferences(ctx, kind, id, nil, b.remaining(b.referencesRemoved))
	if err != nil {
		return err
	}
	b.referencesRemoved += int64(len(removed))
	if kind == domain.ReferenceAnswer {
		b.answerAttachments = append(b.answerAttachments, removed...)
	} else {
		b.reviewAttachments = append(b.reviewAttachments, removed...)
	}
	return nil
}

func (b *studentPurgeBatch) erase(ctx context.Context, repos repositories.Repositories, id domain.SubmissionID) error {
	for _, revision := range b.revisions[id] {
		if err := b.references(ctx, repos, domain.ReferenceReviewItem, domain.EntityID(revision)); err != nil {
			return err
		}
		if b.full() {
			return nil
		}
		deleted, err := repos.Homework.DeleteCleanupRevision(ctx, revision)
		if err != nil {
			return err
		}
		b.revisionsRemoved += deleted
		if b.full() {
			return nil
		}
	}
	if err := b.references(ctx, repos, domain.ReferenceAnswer, domain.EntityID(id)); err != nil {
		return err
	}
	if _, err := repos.Homework.RefreshFileBytes(ctx, id); err != nil {
		return err
	}
	if b.full() {
		return nil
	}
	deleted, err := repos.Homework.DeleteCleanupAnswers(ctx, id, b.remaining(b.answersRemoved))
	if err != nil {
		return err
	}
	b.answersRemoved += deleted
	if b.full() {
		return nil
	}
	deleted, err = repos.Homework.DeleteCleanupNotifications(ctx, id, b.remaining(b.notificationsRemoved))
	if err != nil {
		return err
	}
	b.notificationsRemoved += deleted
	if b.full() {
		return nil
	}
	deleted, err = repos.Homework.DeleteCleanupSubmission(ctx, id)
	b.submissionsRemoved += deleted
	return err
}

func (b *studentPurgeBatch) release(ctx context.Context, repos repositories.Repositories, purge Purge, now time.Time) error {
	attachments := append(append([]domain.AttachmentID{}, b.answerAttachments...), b.reviewAttachments...)
	for _, id := range distinctAttachments(attachments) {
		if _, err := repos.Attachments.Lock(ctx, id); err != nil {
			return err
		}
	}
	if err := library.ReleaseReferences(ctx, repos, purge.Random, purge.Logger, domain.ReferenceReviewItem, distinctAttachments(b.reviewAttachments), now); err != nil {
		return err
	}
	return library.ReleaseReferences(ctx, repos, purge.Random, purge.Logger, domain.ReferenceAnswer, distinctAttachments(b.answerAttachments), now)
}

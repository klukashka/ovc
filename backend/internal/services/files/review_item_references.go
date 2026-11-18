package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type reviewItemAuthorization struct{}

func (reviewItemAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceReviewItem
}

func (reviewItemAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID, now time.Time,
) ([]visibleReference, error) {
	revisions, err := repos.Homework.VisibleReviewItems(ctx, viewer, attachment, now)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(revisions))
	for _, revision := range revisions {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceReviewItem, Item: domain.EntityID(revision.ID)}, width: revision.Width,
		})
	}
	return visible, nil
}

func (reviewItemAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID, now time.Time,
) (repositories.ServableFile, error) {
	return repos.Homework.ServableReviewItemFile(ctx, attachment, item, now)
}

func (s *Service) ReviewItemAddress(
	id domain.AttachmentID, revision domain.ReviewRevisionID, disposition domain.Disposition, now time.Time,
) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceReviewItem, Item: domain.EntityID(revision)}, disposition, now)
	if err != nil {
		panic("a corrected file download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

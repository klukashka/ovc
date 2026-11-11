package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type issueSnapshotAuthorization struct{}

func (issueSnapshotAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceIssueSnapshot
}

func (issueSnapshotAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID,
) ([]visibleReference, error) {
	issues, err := repos.Homework.VisibleSnapshots(ctx, viewer, attachment)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(issues))
	for _, issue := range issues {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceIssueSnapshot, Item: domain.EntityID(issue.ID)}, width: issue.Width,
		})
	}
	return visible, nil
}

func (issueSnapshotAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID,
) (repositories.ServableFile, error) {
	return repos.Homework.ServableSnapshotFile(ctx, attachment, item)
}

func (s *Service) IssueSnapshotAddress(id domain.AttachmentID, issue domain.IssueID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceIssueSnapshot, Item: domain.EntityID(issue)}, disposition, now)
	if err != nil {
		panic("an issue snapshot download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type answerAuthorization struct{}

func (answerAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceAnswer
}

func (answerAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID,
) ([]visibleReference, error) {
	submissions, err := repos.Homework.VisibleAnswers(ctx, viewer, attachment)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(submissions))
	for _, submission := range submissions {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceAnswer, Item: domain.EntityID(submission.ID)}, width: submission.Width,
		})
	}
	return visible, nil
}

func (answerAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID,
) (repositories.ServableFile, error) {
	return repos.Homework.ServableAnswerFile(ctx, attachment, item)
}

func (s *Service) AnswerAddress(id domain.AttachmentID, submission domain.SubmissionID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceAnswer, Item: domain.EntityID(submission)}, disposition, now)
	if err != nil {
		panic("an answer download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

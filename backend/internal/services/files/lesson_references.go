package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type lessonAuthorization struct{}

func (lessonAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceLesson
}

func (lessonAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID,
) ([]visibleReference, error) {
	lessons, err := repos.LessonMaterials.Visible(ctx, viewer, attachment)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(lessons))
	for _, lesson := range lessons {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceLesson, Item: domain.EntityID(lesson.ID)}, width: lesson.Width,
		})
	}
	return visible, nil
}

func (lessonAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID,
) (repositories.ServableFile, error) {
	return repos.LessonMaterials.ServableFile(ctx, attachment, item)
}

func (s *Service) LessonAddress(id domain.AttachmentID, lesson domain.LessonID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceLesson, Item: domain.EntityID(lesson)}, disposition, now)
	if err != nil {
		panic("a lesson download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

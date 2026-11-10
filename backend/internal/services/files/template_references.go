package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type templateAuthorization struct{}

func (templateAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceTemplate
}

func (templateAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID,
) ([]visibleReference, error) {
	templates, err := repos.Library.VisibleTemplates(ctx, viewer, attachment)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(templates))
	for _, template := range templates {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceTemplate, Item: domain.EntityID(template.ID)}, width: template.Width,
		})
	}
	return visible, nil
}

func (templateAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID,
) (repositories.ServableFile, error) {
	return repos.Library.ServableTemplateFile(ctx, attachment, item)
}

func (s *Service) TemplateAddress(id domain.AttachmentID, template domain.TemplateID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceTemplate, Item: domain.EntityID(template)}, disposition, now)
	if err != nil {
		panic("a template download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

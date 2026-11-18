package files

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type materialAuthorization struct{}

func (materialAuthorization) kind() domain.ReferenceKind {
	return domain.ReferenceMaterial
}

func (materialAuthorization) visible(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, attachment domain.AttachmentID, _ time.Time,
) ([]visibleReference, error) {
	materials, err := repos.Library.VisibleMaterials(ctx, viewer, attachment)
	if err != nil {
		return nil, err
	}
	visible := make([]visibleReference, 0, len(materials))
	for _, material := range materials {
		visible = append(visible, visibleReference{
			reference: DownloadReference{Kind: domain.ReferenceMaterial, Item: domain.EntityID(material.ID)}, width: material.Width,
		})
	}
	return visible, nil
}

func (materialAuthorization) servable(
	ctx context.Context, repos repositories.Repositories, attachment domain.AttachmentID, item domain.EntityID, _ time.Time,
) (repositories.ServableFile, error) {
	return repos.Library.ServableMaterialFile(ctx, attachment, item)
}

func (s *Service) MaterialAddress(id domain.AttachmentID, material domain.MaterialID, disposition domain.Disposition, now time.Time) (string, time.Time) {
	address, err := s.ReferenceAddress(id, DownloadReference{Kind: domain.ReferenceMaterial, Item: domain.EntityID(material)}, disposition, now)
	if err != nil {
		panic("a material download token could not be minted")
	}
	return address.URL, address.ExpiresAt
}

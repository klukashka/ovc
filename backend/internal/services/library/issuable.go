package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type IssuableMaterial struct {
	Title      string
	Attachment domain.AttachmentID
}

type IssuableTemplate struct {
	ID        domain.TemplateID
	Title     string
	Blocks    []domain.TemplateBlock
	Ready     bool
	MaxPoints int32
	Materials map[domain.MaterialID]IssuableMaterial
}

func (s *Service) TemplateForIssue(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.TemplateID,
) (IssuableTemplate, error) {
	row, err := repos.Library.LiveTemplate(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return IssuableTemplate{}, ErrNotFound
	}
	if err != nil {
		return IssuableTemplate{}, err
	}
	var blocks []domain.TemplateBlock
	if err := json.Unmarshal(row.Blocks, &blocks); err != nil {
		return IssuableTemplate{}, fmt.Errorf("decode the stored blocks of template %s: %w", id, err)
	}
	template := IssuableTemplate{
		ID: row.ID, Title: row.Title, Blocks: blocks, Ready: row.Ready, MaxPoints: row.MaxPoints,
		Materials: map[domain.MaterialID]IssuableMaterial{},
	}
	var materials []domain.MaterialID
	for _, block := range blocks {
		if block.MaterialID == nil {
			continue
		}
		material, valid := domain.ParseUUID[domain.MaterialID](*block.MaterialID)
		if !valid {
			return IssuableTemplate{}, fmt.Errorf("template %s: %w", id, errStoredMaterialGone)
		}
		materials = append(materials, material)
	}
	if len(materials) == 0 {
		return template, nil
	}
	files, err := repos.Library.TemplateMaterialFiles(ctx, tutor, materials)
	if err != nil {
		return IssuableTemplate{}, err
	}
	for _, file := range files {
		template.Materials[file.ID] = IssuableMaterial{Title: file.Title, Attachment: file.AttachmentID}
	}
	return template, nil
}

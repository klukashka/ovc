package homework

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/library"
)

var (
	errSnapshotMaterialGone  = errors.New("a material block of the issued template names no material")
	errSnapshotImageInvalid  = errors.New("a prompt image of the issued template is not an identifier")
	errSnapshotUnreferenced  = errors.New("a file of the snapshot is not referenced by the issued template")
	errStoredSnapshotInvalid = errors.New("a stored snapshot cannot be decoded")
)

type snapshot struct {
	title       string
	encoded     []byte
	attachments []domain.AttachmentID
	maxPoints   int32
	manual      bool
}

func (s *Service) snapshotOf(template library.IssuableTemplate) (snapshot, error) {
	blocks := make([]domain.IssueBlock, 0, len(template.Blocks))
	seen := map[domain.AttachmentID]bool{}
	result := snapshot{title: template.Title}
	for _, block := range template.Blocks {
		issued := domain.IssueBlock{TemplateBlock: block}
		if block.Options != nil {
			options := make([]domain.TemplateOption, 0, len(*block.Options))
			for _, option := range *block.Options {
				fresh, err := core.NewID[domain.TemplateID](s.deps.Random)
				if err != nil {
					return snapshot{}, err
				}
				options = append(options, domain.TemplateOption{ID: fresh.String(), Text: option.Text, Correct: option.Correct})
			}
			issued.Options = &options
		}
		if block.ImageIDs != nil {
			for _, text := range *block.ImageIDs {
				image, valid := domain.ParseUUID[domain.AttachmentID](text)
				if !valid {
					return snapshot{}, errSnapshotImageInvalid
				}
				seen[image] = true
			}
		}
		if block.MaterialID != nil {
			material, valid := domain.ParseUUID[domain.MaterialID](*block.MaterialID)
			copied, found := template.Materials[material]
			if !valid || !found {
				return snapshot{}, errSnapshotMaterialGone
			}
			issued.Material = &domain.IssueMaterial{Title: copied.Title, AttachmentID: copied.Attachment.String()}
			seen[copied.Attachment] = true
		}
		if block.Type.Question() && block.Points != nil {
			result.maxPoints += *block.Points
		}
		if block.Type.Manual() {
			result.manual = true
		}
		blocks = append(blocks, issued)
	}
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return snapshot{}, fmt.Errorf("encode the snapshot: %w", err)
	}
	result.encoded = encoded
	for attachment := range seen {
		result.attachments = append(result.attachments, attachment)
	}
	slices.SortFunc(result.attachments, func(a, b domain.AttachmentID) int { return bytes.Compare(a[:], b[:]) })
	return result, nil
}

func requireReferenced(attachments []domain.AttachmentID, referenced []domain.AttachmentID) error {
	for _, attachment := range attachments {
		if !slices.Contains(referenced, attachment) {
			return errSnapshotUnreferenced
		}
	}
	return nil
}

func decodeSnapshot(encoded []byte) ([]domain.IssueBlock, error) {
	var blocks []domain.IssueBlock
	if err := json.Unmarshal(encoded, &blocks); err != nil {
		return nil, fmt.Errorf("%w: %w", errStoredSnapshotInvalid, err)
	}
	return blocks, nil
}

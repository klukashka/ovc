package export

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/files"
)

var errUnexpectedFileContext = errors.New("an exportable file has an upload context the archive has no folder for")

type archivedFile struct {
	attachment domain.AttachmentID
	kind       exportformat.FileKind
	entry      string
	filename   string
	mediaType  string
	size       int64
	storageKey string
	references []string
	digest     string
	stored     bool
}

type inventory struct {
	files map[domain.AttachmentID]*archivedFile
	items int32
}

func newInventory() *inventory {
	return &inventory{files: map[domain.AttachmentID]*archivedFile{}}
}

func referenceText(at files.DownloadReference) string {
	return string(at.Kind) + ":" + at.Item.String()
}

func fileKindOf(context domain.AttachmentContext) (exportformat.FileKind, error) {
	switch context {
	case domain.ContextLibrary:
		return exportformat.FileKindLibrary, nil
	case domain.ContextPromptImage:
		return exportformat.FileKindPromptImages, nil
	case domain.ContextChat:
		return exportformat.FileKindChat, nil
	case domain.ContextAnswer:
		return exportformat.FileKindAnswers, nil
	case domain.ContextCorrected:
		return exportformat.FileKindCorrected, nil
	case domain.ContextAvatar:
	}
	return "", errUnexpectedFileContext
}

func (i *inventory) admit(
	ctx context.Context, repos repositories.Repositories, authorizer FileAuthorizer, viewer domain.UserID, attachment domain.AttachmentID,
	at files.DownloadReference, now time.Time,
) error {
	file, err := authorizer.ExportableFile(ctx, repos, viewer, attachment, at, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	entry := i.files[attachment]
	if entry == nil {
		kind, err := fileKindOf(file.Context)
		if err != nil {
			return err
		}
		name, err := exportformat.FileEntryName(kind, attachment, file.Filename)
		if err != nil {
			return err
		}
		entry = &archivedFile{
			attachment: attachment, kind: kind, entry: name, filename: file.Filename, mediaType: file.MediaType, size: file.SizeBytes,
			storageKey: file.StorageKey,
		}
		i.files[attachment] = entry
	}
	if text := referenceText(at); !slices.Contains(entry.references, text) {
		entry.references = append(entry.references, text)
	}
	return nil
}

func (i *inventory) fileBytes() int64 {
	var total int64
	for _, file := range i.files {
		total += file.size
	}
	return total
}

func (i *inventory) ordered() []*archivedFile {
	ordered := make([]*archivedFile, 0, len(i.files))
	for _, file := range i.files {
		ordered = append(ordered, file)
	}
	slices.SortFunc(ordered, func(a, b *archivedFile) int { return bytes.Compare(a.attachment[:], b.attachment[:]) })
	return ordered
}

func (i *inventory) path(attachment domain.AttachmentID, at files.DownloadReference) *string {
	file := i.files[attachment]
	if file == nil || !file.stored || !slices.Contains(file.references, referenceText(at)) {
		return nil
	}
	entry := file.entry
	return &entry
}

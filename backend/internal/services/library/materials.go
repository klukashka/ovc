package library

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type PathEntry struct {
	ID   domain.FolderID
	Name string
}

type Material struct {
	ID         domain.MaterialID
	Title      string
	Folder     *domain.FolderID
	Path       []PathEntry
	UpdatedAt  time.Time
	Attachment domain.AttachmentID
	Filename   string
	MediaType  string
	SizeBytes  int64
	Width      *int32
	Height     *int32
	URL        *string
	ExpiresAt  *time.Time
}

type NewMaterial struct {
	Attachment domain.AttachmentID
	Title      string
	Folder     *domain.FolderID
}

type MaterialChange struct {
	Title     *string
	Folder    *domain.FolderID
	FolderSet bool
}

func (s *Service) materialOf(row repositories.MaterialRow, now time.Time) Material {
	material := Material{
		ID: row.ID, Title: row.Title, Folder: row.FolderID, UpdatedAt: row.UpdatedAt, Attachment: row.AttachmentID, Filename: row.Filename,
		MediaType: row.MediaType, SizeBytes: row.SizeBytes, Width: row.Width, Height: row.Height,
	}
	if row.Width != nil {
		url, expires := s.deps.Addresses.MaterialAddress(row.AttachmentID, row.ID, domain.DispositionInline, now)
		material.URL, material.ExpiresAt = &url, &expires
	}
	return material
}

func (s *Service) CreateMaterial(ctx context.Context, actor domain.TutorActor, request NewMaterial) (Material, error) {
	tutor := actor.UserID
	var created Material
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		_, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if err := requireFolder(tree, request.Folder); err != nil {
			return err
		}
		if _, err := repos.Library.LockBindableUpload(ctx, tutor, request.Attachment, now); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		normalised, code, valid := domain.LibraryTitleLimits.Check(request.Title)
		if !valid {
			return invalid(fieldTitle, code)
		}
		if err := s.refuseBeyondMaterialCaps(ctx, repos, tutor); err != nil {
			return err
		}
		id, err := core.NewID[domain.MaterialID](s.deps.Random)
		if err != nil {
			return err
		}
		if err := repos.Library.CreateMaterial(ctx, repositories.NewMaterial{
			ID: id, Tutor: tutor, Folder: request.Folder, Title: request.Title, NormalisedTitle: normalised, Attachment: request.Attachment, Now: now,
		}); err != nil {
			return err
		}
		if err := repos.Library.BindMaterialFile(ctx, id, request.Attachment, now); err != nil {
			return err
		}
		charged, err := repos.Library.Charge(ctx, tutor, request.Attachment, now)
		if err != nil {
			return err
		}
		if !charged {
			return ErrQuotaExceeded
		}
		row, err := repos.Library.LiveMaterial(ctx, tutor, id)
		if err != nil {
			return err
		}
		created = s.materialOf(row, now)
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.material.created", tutorAttribute(tutor), materialAttribute(id),
				attachmentAttribute(request.Attachment))
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", attachmentAttribute(request.Attachment),
				slog.String(recordRefKind, string(domain.ReferenceMaterial)))
		})
		return nil
	})
	if errors.Is(err, ErrQuotaExceeded) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", tutorAttribute(tutor), attachmentAttribute(request.Attachment),
			slog.String("context", string(domain.ContextLibrary)))
	}
	return created, err
}

func (s *Service) refuseBeyondMaterialCaps(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) error {
	counts, err := repos.Library.CountMaterials(ctx, tutor)
	if err != nil {
		return err
	}
	templates, err := repos.Library.CountTemplates(ctx, tutor)
	if err != nil {
		return err
	}
	if counts.Live >= s.deps.Caps.Materials || counts.Total+templates.Total >= s.deps.Caps.Rows {
		return invalid(fieldBody, domain.FieldTooMany)
	}
	return nil
}

func (s *Service) UpdateMaterial(ctx context.Context, actor domain.TutorActor, id domain.MaterialID, change MaterialChange) (Material, error) {
	tutor := actor.UserID
	var updated Material
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		current, err := repos.Library.LiveMaterial(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if change.FolderSet {
			_, tree, err := s.folderTree(ctx, repos, tutor)
			if err != nil {
				return err
			}
			if err := requireFolder(tree, change.Folder); err != nil {
				return err
			}
		}
		next := repositories.NewMaterial{
			ID: id, Tutor: tutor, Folder: current.FolderID, Title: current.Title, NormalisedTitle: domain.NormalisedText(current.NormalisedTitle), Now: now,
		}
		renamed, moved := false, false
		if change.Title != nil && *change.Title != current.Title {
			normalised, code, valid := domain.LibraryTitleLimits.Check(*change.Title)
			if !valid {
				return invalid(fieldTitle, code)
			}
			next.Title, next.NormalisedTitle, renamed = *change.Title, normalised, true
		}
		if change.FolderSet && !sameFolder(change.Folder, current.FolderID) {
			next.Folder, moved = change.Folder, true
		}
		if renamed || moved {
			if _, err := repos.Library.UpdateMaterial(ctx, next); err != nil {
				return err
			}
			if current, err = repos.Library.LiveMaterial(ctx, tutor, id); err != nil {
				return err
			}
			repos.AfterCommit(ctx, func(ctx context.Context) {
				for _, change := range changes(renamed, moved) {
					s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.material.updated", tutorAttribute(tutor), materialAttribute(id), slog.String("change", change))
				}
			})
		}
		updated = s.materialOf(current, now)
		return nil
	})
	return updated, err
}

func (s *Service) DeleteMaterial(ctx context.Context, actor domain.TutorActor, id domain.MaterialID) error {
	tutor := actor.UserID
	return s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		state, err := repos.Library.MaterialState(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if state.DeletedAt != nil {
			return nil
		}
		if _, err := repos.Library.SoftDeleteMaterial(ctx, tutor, id, now); err != nil {
			return err
		}
		removed, err := repos.Library.RemoveMaterialReference(ctx, id)
		if err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.material.deleted", tutorAttribute(tutor), materialAttribute(id))
		})
		return s.release(domain.ReferenceMaterial).attachments(ctx, repos, removed, now)
	})
}

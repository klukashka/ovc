package library

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Folder struct {
	ID     domain.FolderID
	Name   string
	Parent *domain.FolderID
}

type NewFolder struct {
	Name   string
	Parent *domain.FolderID
}

type FolderChange struct {
	Name      *string
	Parent    *domain.FolderID
	ParentSet bool
}

func folderOf(row repositories.FolderRow) Folder {
	return Folder{ID: row.ID, Name: row.Name, Parent: row.ParentID}
}

func (s *Service) Folders(ctx context.Context, tutor domain.UserID) ([]Folder, error) {
	rows, err := s.deps.Store.Repositories().Library.Folders(ctx, tutor)
	if err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, folderOf(row))
	}
	return folders, nil
}

func (s *Service) CreateFolder(ctx context.Context, actor domain.TutorActor, request NewFolder) (Folder, error) {
	tutor := actor.UserID
	var created Folder
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		rows, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if err := requireFolder(tree, request.Parent); err != nil {
			return err
		}
		normalised, code, valid := domain.FolderNameLimits.Check(request.Name)
		if !valid {
			return invalid(fieldName, code)
		}
		if !tree.CanCreateUnder(request.Parent) {
			return invalid(fieldParent, domain.FieldInvalid)
		}
		if err := s.refuseTakenName(ctx, repos, tutor, request.Parent, normalised, domain.FolderID{}); err != nil {
			return err
		}
		if int64(len(rows)) >= s.deps.Caps.Folders {
			return invalid(fieldBody, domain.FieldTooMany)
		}
		id, err := core.NewID[domain.FolderID](s.deps.Random)
		if err != nil {
			return err
		}
		if err := repos.Library.CreateFolder(ctx, repositories.NewFolder{
			ID: id, Tutor: tutor, Parent: request.Parent, Name: request.Name, NormalisedName: normalised, Now: now,
		}); err != nil {
			return err
		}
		created = Folder{ID: id, Name: request.Name, Parent: request.Parent}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.folder.created", tutorAttribute(tutor), folderAttribute(id))
		})
		return nil
	})
	return created, err
}

func (s *Service) refuseTakenName(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, parent *domain.FolderID, name domain.NormalisedText, excluded domain.FolderID,
) error {
	taken, err := repos.Library.FolderNameTaken(ctx, tutor, parent, name, excluded)
	if err != nil {
		return err
	}
	if taken {
		return invalid(fieldName, domain.FieldTaken)
	}
	return nil
}

func (s *Service) UpdateFolder(ctx context.Context, actor domain.TutorActor, id domain.FolderID, change FolderChange) (Folder, error) {
	tutor := actor.UserID
	var updated Folder
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		rows, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		current, found := findFolder(rows, id)
		if !found {
			return ErrNotFound
		}
		if change.ParentSet {
			if err := requireFolder(tree, change.Parent); err != nil {
				return err
			}
		}
		next := repositories.NewFolder{ID: id, Tutor: tutor, Parent: current.ParentID, Name: current.Name, NormalisedName: domain.NormalisedText(current.NormalisedName), Now: now}
		renamed, moved := false, false
		if change.Name != nil && *change.Name != current.Name {
			normalised, code, valid := domain.FolderNameLimits.Check(*change.Name)
			if !valid {
				return invalid(fieldName, code)
			}
			next.Name, next.NormalisedName, renamed = *change.Name, normalised, true
		}
		if change.ParentSet && !sameFolder(change.Parent, current.ParentID) {
			if !tree.CanMove(id, change.Parent) {
				return invalid(fieldParent, domain.FieldInvalid)
			}
			next.Parent, moved = change.Parent, true
		}
		updated = Folder{ID: id, Name: next.Name, Parent: next.Parent}
		if !renamed && !moved {
			return nil
		}
		if err := s.refuseTakenName(ctx, repos, tutor, next.Parent, next.NormalisedName, id); err != nil {
			return err
		}
		if _, err := repos.Library.UpdateFolder(ctx, next); err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			for _, change := range changes(renamed, moved) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.folder.updated", tutorAttribute(tutor), folderAttribute(id), slog.String("change", change))
			}
		})
		return nil
	})
	return updated, err
}

func (s *Service) DeleteFolder(ctx context.Context, actor domain.TutorActor, id domain.FolderID) error {
	tutor := actor.UserID
	return s.write(ctx, actor, func(repos repositories.Repositories, _ time.Time) error {
		rows, _, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if _, found := findFolder(rows, id); !found {
			return ErrNotFound
		}
		contents, err := repos.Library.FolderHasContents(ctx, tutor, id)
		if err != nil {
			return err
		}
		if contents {
			return ErrNotEditable
		}
		if _, err := repos.Library.DeleteFolder(ctx, tutor, id); err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.folder.deleted", tutorAttribute(tutor), folderAttribute(id))
		})
		return nil
	})
}

func findFolder(rows []repositories.FolderRow, id domain.FolderID) (repositories.FolderRow, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return repositories.FolderRow{}, false
}

func changes(renamed bool, moved bool) []string {
	var happened []string
	if renamed {
		happened = append(happened, changeRename)
	}
	if moved {
		happened = append(happened, changeMove)
	}
	return happened
}

func sameFolder(a *domain.FolderID, b *domain.FolderID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

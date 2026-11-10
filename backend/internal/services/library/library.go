package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrUnauthenticated = errors.New("the tutor can no longer write to the library")
	ErrNotFound        = errors.New("the item is not in the caller's library")
	ErrNotEditable     = errors.New("the folder is not empty")
	ErrQuotaExceeded   = errors.New("the tutor's file quota has no room for the file")
)

const (
	fieldBody     = "body"
	fieldName     = "body.name"
	fieldTitle    = "body.title"
	fieldParent   = "body.parent_id"
	fieldQuery    = "query.q"
	fieldFolder   = "query.folder"
	fieldCursor   = "query.cursor"
	limitWrites   = "library_writes"
	changeRename  = "rename"
	changeMove    = "move"
	recordRefKind = "ref_kind"
)

type FieldProblem struct {
	Path string
	Code domain.FieldCode
}

type ValidationError struct {
	Fields []FieldProblem
}

func (e *ValidationError) Error() string {
	return "the request is invalid"
}

func invalid(path string, code domain.FieldCode) *ValidationError {
	return &ValidationError{Fields: []FieldProblem{{Path: path, Code: code}}}
}

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "the library write limit is reached"
}

type Addresses interface {
	MaterialAddress(id domain.AttachmentID, material domain.MaterialID, disposition domain.Disposition, now time.Time) (string, time.Time)
}

type Dependencies struct {
	Store     *repositories.Store
	Clock     clock.Source
	Random    core.RandomSource
	Keys      core.Keys
	Addresses Addresses
	Logger    *slog.Logger
	Caps      domain.LibraryCaps
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	if dependencies.Caps == (domain.LibraryCaps{}) {
		dependencies.Caps = domain.DefaultLibraryCaps
	}
	return &Service{deps: dependencies}
}

func (s *Service) lockTutor(ctx context.Context, repos repositories.Repositories, actor domain.TutorActor) (time.Time, error) {
	account, err := repos.Users.LockCredentials(ctx, actor.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	err = repos.TutorProfiles.Lock(ctx, actor.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return time.Time{}, err
	}
	session, err := repos.Sessions.ByID(ctx, actor.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	if session.UserID != actor.UserID || session.RevokedAt != nil || !checkedAt.Before(session.ExpiresAt) {
		return time.Time{}, ErrUnauthenticated
	}
	if account.State != domain.AccountActive && account.State != domain.AccountUnverified {
		return time.Time{}, ErrUnauthenticated
	}
	return checkedAt, nil
}

func (s *Service) write(ctx context.Context, actor domain.TutorActor, work func(repos repositories.Repositories, now time.Time) error) error {
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		return s.reserveWrite(ctx, repos, actor.UserID, now)
	})
	if err != nil {
		return err
	}
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		return work(repos, now)
	})
}

func (s *Service) reserveWrite(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, domain.LibraryWriteWindow)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose:     domain.PurposeLibraryWrites,
		Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeLibraryWrites), tutor.String()),
		WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > domain.LibraryWritesPerHour {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "library.rate_limited", tutorAttribute(tutor), slog.String("limit", limitWrites))
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (s *Service) folderTree(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) ([]repositories.FolderRow, domain.FolderTree, error) {
	rows, err := repos.Library.Folders(ctx, tutor)
	if err != nil {
		return nil, nil, err
	}
	tree := make(domain.FolderTree, len(rows))
	for _, row := range rows {
		tree[row.ID] = row.ParentID
	}
	return rows, tree, nil
}

func requireFolder(tree domain.FolderTree, folder *domain.FolderID) error {
	if folder == nil {
		return nil
	}
	if _, found := tree[*folder]; !found {
		return ErrNotFound
	}
	return nil
}

type release struct {
	random core.RandomSource
	logger *slog.Logger
}

func (r release) attachments(ctx context.Context, repos repositories.Repositories, attachments []domain.AttachmentID, now time.Time) error {
	if len(attachments) == 0 {
		return nil
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, attachment := range attachments {
			r.logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.removed", attachmentAttribute(attachment),
				slog.String(recordRefKind, string(domain.ReferenceMaterial)))
		}
	})
	if _, err := repos.Library.ReleaseCharges(ctx, attachments, now); err != nil {
		return err
	}
	deleted, err := repos.Library.MarkUnreferencedDeleted(ctx, attachments, now)
	if err != nil {
		return err
	}
	return r.queueStorageDeletions(ctx, repos, deleted, now)
}

func (r release) queueStorageDeletions(ctx context.Context, repos repositories.Repositories, uploads []repositories.UnreferencedUpload, now time.Time) error {
	if len(uploads) == 0 {
		return nil
	}
	rows := make([]repositories.QueuedOutboxRow, 0, len(uploads))
	for _, upload := range uploads {
		payload, err := json.Marshal(domain.AttachmentPayload{Attachment: upload.ID.String(), Key: upload.StorageKey})
		if err != nil {
			return fmt.Errorf("encode the storage deletion payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](r.random)
		if err != nil {
			return err
		}
		rows = append(rows, repositories.QueuedOutboxRow{
			ID: id, Payload: payload, IdempotencyKey: string(domain.KindStorageDelete) + ":" + upload.ID.String(),
		})
	}
	_, err := repos.Outbox.InsertAll(ctx, repositories.NewOutboxRows{
		Lane: domain.LaneStorageDelete, Priority: domain.PriorityNormal, Kind: domain.KindStorageDelete, NextAttemptAt: now, CreatedAt: now,
		Rows: rows,
	})
	return err
}

func tutorAttribute(tutor domain.UserID) slog.Attr {
	return slog.String("tutor_id", tutor.String())
}

func attachmentAttribute(attachment domain.AttachmentID) slog.Attr {
	return slog.String("attachment_id", attachment.String())
}

func folderAttribute(folder domain.FolderID) slog.Attr {
	return slog.String("folder_id", folder.String())
}

func materialAttribute(material domain.MaterialID) slog.Attr {
	return slog.String("material_id", material.String())
}

package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const fieldMaterialIDs = "body.material_ids"

type Addresses interface {
	LessonAddress(id domain.AttachmentID, lesson domain.LessonID, disposition domain.Disposition, now time.Time) (string, time.Time)
}

type LessonMaterial struct {
	MaterialID   *domain.MaterialID
	AttachmentID domain.AttachmentID
	Title        string
	Filename     string
	MediaType    string
	SizeBytes    int64
	Width        *int32
	Height       *int32
	URL          string
	ExpiresAt    time.Time
}

type MaterialsChange struct {
	MaterialIDs []domain.MaterialID
	Version     int32
}

type materialsOutcome struct {
	lesson  Lesson
	changed bool
	count   int
}

func materialIDProblem(ids []domain.MaterialID) *FieldProblem {
	if len(ids) > domain.LessonMaterialsMax {
		return &FieldProblem{Path: fieldMaterialIDs, Code: domain.FieldTooLong}
	}
	seen := make(map[domain.MaterialID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return &FieldProblem{Path: fieldMaterialIDs, Code: domain.FieldInvalid}
		}
		seen[id] = true
	}
	return nil
}

func distinctMaterials(ids []domain.MaterialID) []domain.MaterialID {
	seen := make(map[domain.MaterialID]bool, len(ids))
	distinct := make([]domain.MaterialID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			distinct = append(distinct, id)
		}
	}
	return distinct
}

func resolveMaterials(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, ids []domain.MaterialID, kept []domain.MaterialID) error {
	if len(ids) == 0 {
		return nil
	}
	distinct := distinctMaterials(ids)
	rows, err := repos.LessonMaterials.TutorMaterials(ctx, tutor, distinct)
	if err != nil {
		return err
	}
	usable := make(map[domain.MaterialID]bool, len(rows))
	for _, row := range rows {
		usable[row.ID] = row.DeletedAt == nil || slices.Contains(kept, row.ID)
	}
	for _, id := range distinct {
		if !usable[id] {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Service) withMaterials(ctx context.Context, repos repositories.Repositories, lessons []Lesson, forTutor bool) error {
	ids := make([]domain.LessonID, 0, len(lessons))
	for _, lesson := range lessons {
		ids = append(ids, lesson.ID)
	}
	rows, err := repos.LessonMaterials.Views(ctx, ids)
	if err != nil {
		return err
	}
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return err
	}
	byLesson := map[domain.LessonID][]LessonMaterial{}
	for _, row := range rows {
		disposition := domain.DispositionAttachment
		if row.Width != nil {
			disposition = domain.DispositionInline
		}
		url, expires := s.deps.Addresses.LessonAddress(row.AttachmentID, row.LessonID, disposition, now)
		material := LessonMaterial{
			AttachmentID: row.AttachmentID, Title: row.Title, Filename: row.Filename, MediaType: row.MediaType, SizeBytes: row.SizeBytes,
			Width: row.Width, Height: row.Height, URL: url, ExpiresAt: expires,
		}
		if forTutor {
			id := row.MaterialID
			material.MaterialID = &id
		}
		byLesson[row.LessonID] = append(byLesson[row.LessonID], material)
	}
	for index := range lessons {
		lessons[index].Materials = append([]LessonMaterial{}, byLesson[lessons[index].ID]...)
	}
	return nil
}

func (s *Service) SetLessonMaterials(ctx context.Context, actor domain.TutorActor, id domain.LessonID, change MaterialsChange) (Lesson, error) {
	tutor := actor.UserID
	if err := s.admitLessonWrite(ctx, actor, id); err != nil {
		return Lesson{}, err
	}
	var outcome materialsOutcome
	err := s.writeWithAudienceRetry(ctx, func(repos repositories.Repositories) error {
		var err error
		outcome, err = s.setMaterials(ctx, repos, actor, id, change)
		return err
	})
	if err != nil {
		return Lesson{}, err
	}
	if outcome.changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.materials.changed", tutorAttribute(tutor), lessonAttributes(id),
			slog.Int("count", outcome.count), slog.String("scope", string(domain.ScopeThis)))
	}
	return outcome.lesson, nil
}

func (s *Service) setMaterials(
	ctx context.Context, repos repositories.Repositories, actor domain.TutorActor, id domain.LessonID, change MaterialsChange,
) (materialsOutcome, error) {
	tutor := actor.UserID
	account, err := s.lockActingTutor(ctx, repos, actor)
	if err != nil {
		return materialsOutcome{}, err
	}
	target, err := repos.Lessons.TutorTarget(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return materialsOutcome{}, ErrNotFound
	}
	if err != nil {
		return materialsOutcome{}, err
	}
	audience, membersChanged, err := lockAudienceOf(ctx, repos, tutor, target.RelationshipID, target.GroupID)
	if err != nil {
		return materialsOutcome{}, err
	}
	if target.SeriesID != nil {
		if err := repos.Lessons.LockSeriesShared(ctx, *target.SeriesID); err != nil {
			return materialsOutcome{}, err
		}
	}
	locked, err := repos.LessonMaterials.LockLesson(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return materialsOutcome{}, ErrNotFound
	}
	if err != nil {
		return materialsOutcome{}, err
	}
	if !sameSeries(locked.SeriesID, target.SeriesID) {
		return materialsOutcome{}, ErrStale
	}
	attached, err := repos.LessonMaterials.OfLessons(ctx, []domain.LessonID{id})
	if err != nil {
		return materialsOutcome{}, err
	}
	current := attached[id]
	if err := resolveMaterials(ctx, repos, tutor, change.MaterialIDs, current); err != nil {
		return materialsOutcome{}, err
	}
	if locked.Version != change.Version {
		return materialsOutcome{}, ErrStale
	}
	if problem := materialIDProblem(change.MaterialIDs); problem != nil {
		return materialsOutcome{}, &ValidationError{Fields: []FieldProblem{*problem}}
	}
	if slices.Equal(current, change.MaterialIDs) {
		views, err := s.tutorViews(ctx, repos, account, []domain.LessonID{id})
		return materialsOutcome{lesson: views[0]}, err
	}
	scheduled := locked.Status == domain.LessonScheduled
	if scheduled && membersChanged {
		return materialsOutcome{}, errMembersChanged
	}
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return materialsOutcome{}, err
	}
	references, err := repos.LessonMaterials.ReplaceForLessons(ctx, tutor, []domain.LessonID{id}, change.MaterialIDs, now)
	if err != nil {
		return materialsOutcome{}, err
	}
	overridden := locked.OverriddenFields
	if locked.SeriesID != nil && !slices.Contains(overridden, domain.LessonFieldMaterials) {
		overridden = append(slices.Clone(overridden), domain.LessonFieldMaterials)
	}
	recorded, err := repos.LessonMaterials.RecordChange(ctx, id, change.Version, overridden, now)
	if err != nil {
		return materialsOutcome{}, err
	}
	if !recorded {
		return materialsOutcome{}, ErrStale
	}
	if err := s.releaseReferences(ctx, repos, domain.ReferenceLesson, references, now); err != nil {
		return materialsOutcome{}, err
	}
	students := audience.students()
	if !scheduled {
		if students, err = repos.LessonMaterials.RosterStudents(ctx, id); err != nil {
			return materialsOutcome{}, err
		}
	}
	if err := signalLessons(ctx, repos, tutor, students, []domain.LessonID{id}); err != nil {
		return materialsOutcome{}, err
	}
	views, err := s.tutorViews(ctx, repos, account, []domain.LessonID{id})
	if err != nil {
		return materialsOutcome{}, err
	}
	return materialsOutcome{lesson: views[0], changed: true, count: len(change.MaterialIDs)}, nil
}

func (s *Service) retireSeriesMaterials(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, series domain.SeriesID, now time.Time) error {
	retired, err := repos.LessonMaterials.ReplaceForSeries(ctx, tutor, series, nil, now)
	if err != nil {
		return err
	}
	return s.releaseReferences(ctx, repos, domain.ReferenceSeries, retired, now)
}

func difference(from []domain.AttachmentID, without []domain.AttachmentID) []domain.AttachmentID {
	var kept []domain.AttachmentID
	for _, attachment := range from {
		if !slices.Contains(without, attachment) && !slices.Contains(kept, attachment) {
			kept = append(kept, attachment)
		}
	}
	return kept
}

func (s *Service) releaseReferences(
	ctx context.Context, repos repositories.Repositories, kind domain.ReferenceKind, change repositories.ReferenceChange, now time.Time,
) error {
	added, removed := difference(change.Added, change.Removed), difference(change.Removed, change.Added)
	s.logReferences(ctx, repos, kind, added, removed)
	if len(removed) == 0 {
		return nil
	}
	if _, err := repos.Library.ReleaseCharges(ctx, removed, now); err != nil {
		return err
	}
	deleted, err := repos.Library.MarkUnreferencedDeleted(ctx, removed, now)
	if err != nil {
		return err
	}
	return s.queueStorageDeletions(ctx, repos, deleted, now)
}

func (s *Service) logReferences(ctx context.Context, repos repositories.Repositories, kind domain.ReferenceKind, added []domain.AttachmentID, removed []domain.AttachmentID) {
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, attachment := range added {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", attachmentAttribute(attachment), slog.String("ref_kind", string(kind)))
		}
		for _, attachment := range removed {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.removed", attachmentAttribute(attachment), slog.String("ref_kind", string(kind)))
		}
	})
}

func (s *Service) queueStorageDeletions(ctx context.Context, repos repositories.Repositories, uploads []repositories.UnreferencedUpload, now time.Time) error {
	if len(uploads) == 0 {
		return nil
	}
	rows := make([]repositories.QueuedOutboxRow, 0, len(uploads))
	for _, upload := range uploads {
		payload, err := json.Marshal(domain.AttachmentPayload{Attachment: upload.ID.String(), Key: upload.StorageKey})
		if err != nil {
			return fmt.Errorf("encode the storage deletion payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](s.deps.Random)
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

func attachmentAttribute(attachment domain.AttachmentID) slog.Attr {
	return slog.String("attachment_id", attachment.String())
}

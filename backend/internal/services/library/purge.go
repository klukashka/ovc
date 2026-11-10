package library

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Purge struct {
	Random core.RandomSource
	Logger *slog.Logger
}

type PurgeStep func(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time) (int64, error)

func (p Purge) TutorSteps() []PurgeStep {
	return []PurgeStep{
		p.lessonMaterialsBatch, p.seriesMaterialsBatch, p.templateReferencesBatch, p.materialReferencesBatch, templatesBatch, materialsBatch, foldersBatch,
	}
}

func (p Purge) lessonMaterialsBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time) (int64, error) {
	rows, err := repos.LessonMaterials.LessonBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	removed, err := repos.LessonMaterials.RemoveLessonRows(ctx, tutor, rows)
	if err != nil {
		return 0, err
	}
	if err := (release{random: p.Random, logger: p.Logger, kind: domain.ReferenceLesson}).attachments(ctx, repos, removed, now); err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func (p Purge) seriesMaterialsBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time) (int64, error) {
	rows, err := repos.LessonMaterials.SeriesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	removed, err := repos.LessonMaterials.RemoveSeriesRows(ctx, tutor, rows)
	if err != nil {
		return 0, err
	}
	if err := (release{random: p.Random, logger: p.Logger, kind: domain.ReferenceSeries}).attachments(ctx, repos, removed, now); err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func (p Purge) materialReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Library.MaterialReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	removed, err := repos.Library.RemoveMaterialReferencesOfTutor(ctx, tutor, references)
	if err != nil {
		return 0, err
	}
	if err := (release{random: p.Random, logger: p.Logger, kind: domain.ReferenceMaterial}).attachments(ctx, repos, removed, now); err != nil {
		return 0, err
	}
	return int64(len(removed)), nil
}

func materialsBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Library.DeleteMaterialsBatchOfTutor(ctx, tutor, batch)
}

func foldersBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Library.DeleteLeafFoldersBatchOfTutor(ctx, tutor, batch)
}

func (p Purge) templateReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Library.TemplateReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	removed, err := repos.Library.RemoveTemplateReferencesOfTutor(ctx, tutor, references)
	if err != nil {
		return 0, err
	}
	if err := (release{random: p.Random, logger: p.Logger, kind: domain.ReferenceTemplate}).attachments(ctx, repos, removed, now); err != nil {
		return 0, err
	}
	return int64(len(removed)), nil
}

func templatesBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Library.DeleteTemplatesBatchOfTutor(ctx, tutor, batch)
}

package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type LessonMaterials struct {
	queries *dbgen.Queries
}

type (
	TutorMaterial       = dbgen.ListTutorMaterialsByIDRow
	LessonMaterialView  = dbgen.ListLessonMaterialViewsRow
	VisibleLessonFile   = dbgen.VisibleLessonReferencesRow
	LessonMaterialRow   = dbgen.ListTutorLessonMaterialsBatchRow
	SeriesMaterialRow   = dbgen.ListTutorSeriesMaterialsBatchRow
	LessonPresentation  = dbgen.FirstLessonPresentationRow
	LockedMaterialsItem struct {
		ID               domain.LessonID
		SeriesID         *domain.SeriesID
		Status           domain.LessonStatus
		Version          int32
		OverriddenFields []domain.LessonField
	}
)

type ReferenceChange struct {
	Removed []domain.AttachmentID
	Added   []domain.AttachmentID
}

func (r LessonMaterials) TutorMaterials(ctx context.Context, tutor domain.UserID, ids []domain.MaterialID) ([]TutorMaterial, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListTutorMaterialsByID(ctx, dbgen.ListTutorMaterialsByIDParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "materials of a tutor by identifier")
}

func (r LessonMaterials) OfLessons(ctx context.Context, lessons []domain.LessonID) (map[domain.LessonID][]domain.MaterialID, error) {
	byLesson := map[domain.LessonID][]domain.MaterialID{}
	if len(lessons) == 0 {
		return byLesson, nil
	}
	rows, err := r.queries.ListMaterialIDsOfLessons(ctx, uuids(lessons))
	if err != nil {
		return nil, done(err, "materials of lessons")
	}
	for _, row := range rows {
		byLesson[row.LessonID] = append(byLesson[row.LessonID], row.MaterialID)
	}
	return byLesson, nil
}

func (r LessonMaterials) OfSeries(ctx context.Context, series domain.SeriesID) ([]domain.MaterialID, error) {
	ids, err := r.queries.ListMaterialIDsOfSeries(ctx, series)
	return one(ids, err, "materials of a series")
}

func (r LessonMaterials) FirstPresentation(ctx context.Context, lesson domain.LessonID) (LessonPresentation, error) {
	row, err := r.queries.FirstLessonPresentation(ctx, lesson)
	return one(row, err, "first presentation of a lesson")
}

func (r LessonMaterials) Views(ctx context.Context, lessons []domain.LessonID) ([]LessonMaterialView, error) {
	if len(lessons) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListLessonMaterialViews(ctx, uuids(lessons))
	return one(rows, err, "material views of lessons")
}

func (r LessonMaterials) ReplaceForLessons(
	ctx context.Context, tutor domain.UserID, lessons []domain.LessonID, materials []domain.MaterialID, now time.Time,
) (ReferenceChange, error) {
	if len(lessons) == 0 {
		return ReferenceChange{}, nil
	}
	removed, err := r.queries.RemoveLessonMaterialReferences(ctx, uuids(lessons))
	if err != nil {
		return ReferenceChange{}, done(err, "remove the references of lesson materials")
	}
	if _, err := r.queries.DeleteLessonMaterials(ctx, uuids(lessons)); err != nil {
		return ReferenceChange{}, done(err, "delete lesson materials")
	}
	if len(materials) == 0 {
		return ReferenceChange{Removed: removed}, nil
	}
	if _, err := r.queries.InsertLessonMaterials(ctx, dbgen.InsertLessonMaterialsParams{
		TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, Now: now, LessonIds: uuids(lessons), MaterialIds: uuids(materials),
	}); err != nil {
		return ReferenceChange{}, done(err, "insert lesson materials")
	}
	added, err := r.queries.InsertLessonMaterialReferences(ctx, dbgen.InsertLessonMaterialReferencesParams{Now: now, LessonIds: uuids(lessons)})
	if err != nil {
		return ReferenceChange{}, done(err, "insert the references of lesson materials")
	}
	return ReferenceChange{Removed: removed, Added: added}, nil
}

func (r LessonMaterials) ReplaceForSeries(
	ctx context.Context, tutor domain.UserID, series domain.SeriesID, materials []domain.MaterialID, now time.Time,
) (ReferenceChange, error) {
	removed, err := r.queries.RemoveSeriesMaterialReferences(ctx, pgtype.UUID{Bytes: series, Valid: true})
	if err != nil {
		return ReferenceChange{}, done(err, "remove the references of series materials")
	}
	if _, err := r.queries.DeleteSeriesMaterials(ctx, series); err != nil {
		return ReferenceChange{}, done(err, "delete series materials")
	}
	if len(materials) == 0 {
		return ReferenceChange{Removed: removed}, nil
	}
	if _, err := r.queries.InsertSeriesMaterials(ctx, dbgen.InsertSeriesMaterialsParams{
		SeriesID: pgtype.UUID{Bytes: series, Valid: true}, TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, Now: now, MaterialIds: uuids(materials),
	}); err != nil {
		return ReferenceChange{}, done(err, "insert series materials")
	}
	added, err := r.queries.InsertSeriesMaterialReferences(ctx, dbgen.InsertSeriesMaterialReferencesParams{
		Now: now, SeriesID: pgtype.UUID{Bytes: series, Valid: true},
	})
	if err != nil {
		return ReferenceChange{}, done(err, "insert the references of series materials")
	}
	return ReferenceChange{Removed: removed, Added: added}, nil
}

func (r LessonMaterials) CopySeries(ctx context.Context, from domain.SeriesID, to domain.SeriesID, now time.Time) ([]domain.AttachmentID, error) {
	copied, err := r.queries.CopySeriesMaterials(ctx, dbgen.CopySeriesMaterialsParams{
		ToSeriesID: pgtype.UUID{Bytes: to, Valid: true}, Now: now, FromSeriesID: pgtype.UUID{Bytes: from, Valid: true},
	})
	if err != nil {
		return nil, done(err, "copy the materials of a series")
	}
	if copied == 0 {
		return nil, nil
	}
	added, err := r.queries.InsertSeriesMaterialReferences(ctx, dbgen.InsertSeriesMaterialReferencesParams{
		Now: now, SeriesID: pgtype.UUID{Bytes: to, Valid: true},
	})
	return one(added, err, "insert the references of copied series materials")
}

func (r LessonMaterials) CopyToLessons(ctx context.Context, series domain.SeriesID, lessons []domain.LessonID, now time.Time) ([]domain.AttachmentID, error) {
	if len(lessons) == 0 {
		return nil, nil
	}
	copied, err := r.queries.CopySeriesMaterialsToLessons(ctx, dbgen.CopySeriesMaterialsToLessonsParams{
		Now: now, LessonIds: uuids(lessons), SeriesID: pgtype.UUID{Bytes: series, Valid: true},
	})
	if err != nil {
		return nil, done(err, "copy series materials to lessons")
	}
	if copied == 0 {
		return nil, nil
	}
	added, err := r.queries.InsertLessonMaterialReferences(ctx, dbgen.InsertLessonMaterialReferencesParams{Now: now, LessonIds: uuids(lessons)})
	return one(added, err, "insert the references of copied lesson materials")
}

func (r LessonMaterials) RosterStudents(ctx context.Context, lesson domain.LessonID) ([]domain.UserID, error) {
	students, err := r.queries.ListLessonRosterStudents(ctx, lesson)
	return one(students, err, "students on a lesson's roster")
}

func (r LessonMaterials) LockLesson(ctx context.Context, tutor domain.UserID, id domain.LessonID) (LockedMaterialsItem, error) {
	row, err := r.queries.LockLessonForMaterials(ctx, dbgen.LockLessonForMaterialsParams{ID: id, TutorID: tutor})
	if _, err := one(row, err, "lock a lesson for its materials"); err != nil {
		return LockedMaterialsItem{}, err
	}
	fields := make([]domain.LessonField, 0, len(row.OverriddenFields))
	for _, field := range row.OverriddenFields {
		fields = append(fields, domain.LessonField(field))
	}
	return LockedMaterialsItem{ID: row.ID, SeriesID: row.SeriesID, Status: row.Status, Version: row.Version, OverriddenFields: fields}, nil
}

func (r LessonMaterials) RecordChange(
	ctx context.Context, id domain.LessonID, version int32, overridden []domain.LessonField, now time.Time,
) (bool, error) {
	fields := make([]string, 0, len(overridden))
	for _, field := range overridden {
		fields = append(fields, string(field))
	}
	rows, err := r.queries.RecordLessonMaterialsChange(ctx, dbgen.RecordLessonMaterialsChangeParams{
		OverriddenFields: fields, Now: now, ID: id, Version: version,
	})
	return applied(rows, err, "record a change of lesson materials")
}

func (r LessonMaterials) Visible(ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID) ([]VisibleLessonFile, error) {
	rows, err := r.queries.VisibleLessonReferences(ctx, dbgen.VisibleLessonReferencesParams{
		AttachmentID: attachment, Viewer: pgtype.UUID{Bytes: viewer, Valid: true},
	})
	return one(rows, err, "visible lesson references")
}

func (r LessonMaterials) ServableFile(ctx context.Context, attachment domain.AttachmentID, lesson domain.EntityID) (ServableFile, error) {
	row, err := r.queries.GetServableLessonFile(ctx, dbgen.GetServableLessonFileParams{
		LessonID: pgtype.UUID{Bytes: lesson, Valid: true}, ID: attachment,
	})
	return one(ServableFile(row), err, "servable lesson file")
}

func (r LessonMaterials) LessonBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]LessonMaterialRow, error) {
	rows, err := r.queries.ListTutorLessonMaterialsBatch(ctx, dbgen.ListTutorLessonMaterialsBatchParams{TutorID: tutor, BatchSize: batch})
	return one(rows, err, "a batch of the tutor's lesson materials")
}

func (r LessonMaterials) RemoveLessonRows(ctx context.Context, tutor domain.UserID, rows []LessonMaterialRow) ([]domain.AttachmentID, error) {
	lessons, materials, attachments, items := make([]domain.LessonID, 0, len(rows)), make([]domain.MaterialID, 0, len(rows)),
		make([]domain.AttachmentID, 0, len(rows)), make([]domain.EntityID, 0, len(rows))
	for _, row := range rows {
		lessons, materials = append(lessons, row.LessonID), append(materials, row.MaterialID)
		attachments, items = append(attachments, row.AttachmentID), append(items, domain.EntityID(row.LessonID))
	}
	if _, err := r.queries.DeleteLessonMaterialPairs(ctx, dbgen.DeleteLessonMaterialPairsParams{
		LessonIds: uuids(lessons), MaterialIds: uuids(materials), TutorID: pgtype.UUID{Bytes: tutor, Valid: true},
	}); err != nil {
		return nil, done(err, "delete a batch of lesson materials")
	}
	return r.removeReferencePairs(ctx, domain.ReferenceLesson, attachments, items)
}

func (r LessonMaterials) SeriesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]SeriesMaterialRow, error) {
	rows, err := r.queries.ListTutorSeriesMaterialsBatch(ctx, dbgen.ListTutorSeriesMaterialsBatchParams{TutorID: tutor, BatchSize: batch})
	return one(rows, err, "a batch of the tutor's series materials")
}

func (r LessonMaterials) RemoveSeriesRows(ctx context.Context, tutor domain.UserID, rows []SeriesMaterialRow) ([]domain.AttachmentID, error) {
	series, materials, attachments, items := make([]domain.SeriesID, 0, len(rows)), make([]domain.MaterialID, 0, len(rows)),
		make([]domain.AttachmentID, 0, len(rows)), make([]domain.EntityID, 0, len(rows))
	for _, row := range rows {
		series, materials = append(series, row.SeriesID), append(materials, row.MaterialID)
		attachments, items = append(attachments, row.AttachmentID), append(items, domain.EntityID(row.SeriesID))
	}
	if _, err := r.queries.DeleteSeriesMaterialPairs(ctx, dbgen.DeleteSeriesMaterialPairsParams{
		SeriesIds: uuids(series), MaterialIds: uuids(materials), TutorID: pgtype.UUID{Bytes: tutor, Valid: true},
	}); err != nil {
		return nil, done(err, "delete a batch of series materials")
	}
	return r.removeReferencePairs(ctx, domain.ReferenceSeries, attachments, items)
}

func (r LessonMaterials) removeReferencePairs(
	ctx context.Context, kind domain.ReferenceKind, attachments []domain.AttachmentID, items []domain.EntityID,
) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveReferencePairs(ctx, dbgen.RemoveReferencePairsParams{
		AttachmentIds: uuids(attachments), RefIds: uuids(items), RefKind: string(kind),
	})
	return one(removed, err, "remove a batch of references")
}

type RetiredSeries = dbgen.ListRetiredSeriesWithMaterialsRow

func (r LessonMaterials) RetiredSeries(ctx context.Context, now time.Time, limit int32) ([]RetiredSeries, error) {
	rows, err := r.queries.ListRetiredSeriesWithMaterials(ctx, dbgen.ListRetiredSeriesWithMaterialsParams{Now: now, RowLimit: limit})
	return one(rows, err, "retired series that still hold materials")
}

func (r LessonMaterials) LockRetiredSeries(ctx context.Context, id domain.SeriesID, now time.Time) error {
	_, err := r.queries.LockRetiredSeries(ctx, dbgen.LockRetiredSeriesParams{ID: id, Now: now})
	_, err = one(id, err, "lock a retired series")
	return err
}

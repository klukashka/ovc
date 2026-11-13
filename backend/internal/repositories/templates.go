package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	TemplateRow           = dbgen.GetLiveTemplateRow
	TemplateState         = dbgen.LockTemplateStateRow
	TemplateCounts        = dbgen.CountTemplatesOfTutorRow
	TemplateByKey         = dbgen.GetTemplateByCreateKeyRow
	TemplateListRow       = dbgen.ListRootTemplatesRow
	TemplateBlockMaterial = dbgen.ListTemplateBlockMaterialsRow
	TemplateMaterialFile  = dbgen.DescribeTemplateMaterialsRow
	TemplateImageFile     = dbgen.DescribeTemplateImagesRow
	VisibleTemplate       = dbgen.VisibleTemplateReferencesRow
)

type TemplateContent struct {
	Title           string
	NormalisedTitle domain.NormalisedText
	Blocks          []byte
	BlocksBytes     int64
	Summary         domain.TemplateSummary
}

type NewTemplate struct {
	ID        domain.TemplateID
	Tutor     domain.UserID
	Folder    *domain.FolderID
	CreateKey *string
	Content   TemplateContent
	Now       time.Time
}

type TemplateSave struct {
	ID      domain.TemplateID
	Tutor   domain.UserID
	SaveID  domain.TemplateSaveID
	Content TemplateContent
	Now     time.Time
}

func (r Library) CountTemplates(ctx context.Context, tutor domain.UserID) (TemplateCounts, error) {
	counts, err := r.queries.CountTemplatesOfTutor(ctx, tutor)
	return one(counts, err, "count the tutor's templates")
}

func (r Library) LiveTemplateCount(ctx context.Context, tutor domain.UserID) (int64, error) {
	count, err := r.queries.CountLiveTemplatesOfTutor(ctx, tutor)
	return one(count, err, "count the tutor's live templates")
}

func (r Library) TemplateBlockBytes(ctx context.Context, tutor domain.UserID, excluded *domain.TemplateID) (int64, error) {
	used, err := r.queries.TemplateBlockBytesOfTutor(ctx, dbgen.TemplateBlockBytesOfTutorParams{TutorID: tutor, ExcludedID: excludedUUID(excluded)})
	return one(used, err, "the tutor's template block bytes")
}

func excludedUUID(id *domain.TemplateID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: true}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func (r Library) TemplateByCreateKey(ctx context.Context, tutor domain.UserID, key string) (TemplateByKey, error) {
	row, err := r.queries.GetTemplateByCreateKey(ctx, dbgen.GetTemplateByCreateKeyParams{TutorID: tutor, CreateKey: key})
	return one(row, err, "template by its create key")
}

func (r Library) CreateTemplate(ctx context.Context, template NewTemplate) error {
	content := template.Content
	return done(r.queries.CreateTemplate(ctx, dbgen.CreateTemplateParams{
		ID: template.ID, TutorID: template.Tutor, FolderID: optionalUUID(template.Folder), Title: content.Title,
		NormalisedTitle: string(content.NormalisedTitle), Blocks: content.Blocks, SchemaVersion: domain.TemplateSchemaVersion,
		CreateKey: template.CreateKey, QuestionCount: content.Summary.QuestionCount, MaxPoints: content.Summary.MaxPoints,
		Ready: content.Summary.Ready, BlocksBytes: content.BlocksBytes, Now: template.Now,
	}), "create a template")
}

func (r Library) LiveTemplate(ctx context.Context, tutor domain.UserID, id domain.TemplateID) (TemplateRow, error) {
	row, err := r.queries.GetLiveTemplate(ctx, dbgen.GetLiveTemplateParams{ID: id, TutorID: tutor})
	return one(row, err, "live template")
}

func (r Library) LockTemplate(ctx context.Context, tutor domain.UserID, id domain.TemplateID) (TemplateState, error) {
	row, err := r.queries.LockTemplateState(ctx, dbgen.LockTemplateStateParams{ID: id, TutorID: tutor})
	return one(row, err, "lock a template")
}

func (r Library) TemplateLastSave(ctx context.Context, tutor domain.UserID, id domain.TemplateID) (*domain.TemplateSaveID, error) {
	last, err := r.queries.GetTemplateLastSave(ctx, dbgen.GetTemplateLastSaveParams{ID: id, TutorID: tutor})
	return one(last, err, "the template's last save")
}

func (r Library) SaveTemplate(ctx context.Context, save TemplateSave) (bool, error) {
	content := save.Content
	rows, err := r.queries.SaveTemplate(ctx, dbgen.SaveTemplateParams{
		Title: content.Title, NormalisedTitle: string(content.NormalisedTitle), Blocks: content.Blocks,
		SaveID: pgtype.UUID{Bytes: save.SaveID, Valid: true}, QuestionCount: content.Summary.QuestionCount, MaxPoints: content.Summary.MaxPoints,
		Ready: content.Summary.Ready, BlocksBytes: content.BlocksBytes, Now: save.Now, ID: save.ID, TutorID: save.Tutor,
	})
	return applied(rows, err, "save a template")
}

func (r Library) MoveTemplate(ctx context.Context, tutor domain.UserID, id domain.TemplateID, folder *domain.FolderID, now time.Time) (bool, error) {
	rows, err := r.queries.MoveTemplate(ctx, dbgen.MoveTemplateParams{FolderID: optionalUUID(folder), Now: now, ID: id, TutorID: tutor})
	return applied(rows, err, "move a template")
}

func (r Library) SoftDeleteTemplate(ctx context.Context, tutor domain.UserID, id domain.TemplateID, now time.Time) (bool, error) {
	rows, err := r.queries.SoftDeleteTemplate(ctx, dbgen.SoftDeleteTemplateParams{ID: id, TutorID: tutor, Now: now})
	return applied(rows, err, "soft-delete a template")
}

func (r Library) TemplateReferences(ctx context.Context, id domain.TemplateID) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ListTemplateReferences(ctx, pgtype.UUID{Bytes: id, Valid: true})
	return one(ids, err, "template references")
}

func (r Library) AddTemplateReferences(ctx context.Context, id domain.TemplateID, attachments []domain.AttachmentID, now time.Time) error {
	if len(attachments) == 0 {
		return nil
	}
	return done(r.queries.AddTemplateReferences(ctx, dbgen.AddTemplateReferencesParams{
		TemplateID: pgtype.UUID{Bytes: id, Valid: true}, Now: now, AttachmentIds: uuids(attachments),
	}), "add template references")
}

func (r Library) RemoveTemplateReferences(ctx context.Context, id domain.TemplateID, attachments []domain.AttachmentID) ([]domain.AttachmentID, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	removed, err := r.queries.RemoveTemplateReferences(ctx, dbgen.RemoveTemplateReferencesParams{
		TemplateID: pgtype.UUID{Bytes: id, Valid: true}, AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove template references")
}

func (r Library) RemoveAllTemplateReferences(ctx context.Context, id domain.TemplateID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveAllTemplateReferences(ctx, pgtype.UUID{Bytes: id, Valid: true})
	return one(removed, err, "remove every template reference")
}

func (r Library) LockBindablePromptImages(
	ctx context.Context, tutor domain.UserID, ids []domain.AttachmentID, now time.Time,
) ([]domain.AttachmentID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	locked, err := r.queries.LockBindablePromptImages(ctx, dbgen.LockBindablePromptImagesParams{Ids: uuids(ids), UploaderID: &tutor, Now: now})
	return one(locked, err, "lock bindable prompt images")
}

func (r Library) PromptPictures(ctx context.Context, ids []domain.AttachmentID) ([]domain.AttachmentID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	pictures, err := r.queries.ListPromptPictures(ctx, uuids(ids))
	return one(pictures, err, "prompt pictures")
}

func (r Library) BindUploads(ctx context.Context, ids []domain.AttachmentID, now time.Time) error {
	for _, id := range ids {
		bound, err := r.queries.BindAttachment(ctx, dbgen.BindAttachmentParams{Now: now, ID: id})
		if err != nil {
			return done(err, "bind a prompt image")
		}
		if bound != 1 {
			return done(ErrNotFound, "bind a prompt image that is no longer pending")
		}
	}
	return nil
}

func (r Library) ChargeAll(ctx context.Context, tutor domain.UserID, attachments []domain.AttachmentID, now time.Time) (bool, error) {
	if len(attachments) == 0 {
		return true, nil
	}
	_, err := r.queries.ChargeAttachments(ctx, dbgen.ChargeAttachmentsParams{
		Now: now, TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, AttachmentIds: uuids(attachments),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("charge template files: %w", err)
	}
	return true, nil
}

func (r Library) TemplateBlockMaterials(ctx context.Context, tutor domain.UserID, ids []domain.MaterialID) ([]TemplateBlockMaterial, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListTemplateBlockMaterials(ctx, dbgen.ListTemplateBlockMaterialsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "materials of template blocks")
}

func (r Library) TemplateMaterialFiles(ctx context.Context, tutor domain.UserID, ids []domain.MaterialID) ([]TemplateMaterialFile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.DescribeTemplateMaterials(ctx, dbgen.DescribeTemplateMaterialsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "files of template materials")
}

func (r Library) TemplateImageFiles(ctx context.Context, ids []domain.AttachmentID) ([]TemplateImageFile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.DescribeTemplateImages(ctx, uuids(ids))
	return one(rows, err, "files of template prompt images")
}

func (r Library) Templates(ctx context.Context, page MaterialPage) ([]TemplateListRow, error) {
	afterID := pgtype.UUID{Bytes: page.After.ID, Valid: true}
	kind := string(page.After.Kind)
	switch {
	case page.Pattern != nil:
		rows, err := r.queries.SearchTemplates(ctx, dbgen.SearchTemplatesParams{
			TutorID: page.Tutor, Pattern: *page.Pattern, AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID, PageSize: page.PageSize,
		})
		return convertRows(rows, err, "search templates", func(row dbgen.SearchTemplatesRow) TemplateListRow { return TemplateListRow(row) })
	case page.Folder != nil:
		rows, err := r.queries.ListFolderTemplates(ctx, dbgen.ListFolderTemplatesParams{
			TutorID: page.Tutor, FolderID: optionalUUID(page.Folder), AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID,
			PageSize: page.PageSize,
		})
		return convertRows(rows, err, "templates of a folder", func(row dbgen.ListFolderTemplatesRow) TemplateListRow { return TemplateListRow(row) })
	}
	rows, err := r.queries.ListRootTemplates(ctx, dbgen.ListRootTemplatesParams{
		TutorID: page.Tutor, AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID, PageSize: page.PageSize,
	})
	return convertRows(rows, err, "templates at the root", func(row dbgen.ListRootTemplatesRow) TemplateListRow { return row })
}

func (r Library) VisibleTemplates(ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID) ([]VisibleTemplate, error) {
	rows, err := r.queries.VisibleTemplateReferences(ctx, dbgen.VisibleTemplateReferencesParams{
		Viewer: pgtype.UUID{Bytes: viewer, Valid: true}, AttachmentID: attachment,
	})
	return one(rows, err, "visible template references")
}

func (r Library) ServableTemplateFile(ctx context.Context, attachment domain.AttachmentID, template domain.EntityID) (ServableFile, error) {
	row, err := r.queries.GetServableTemplateFile(ctx, dbgen.GetServableTemplateFileParams{
		TemplateID: pgtype.UUID{Bytes: template, Valid: true}, ID: attachment,
	})
	return one(ServableFile(row), err, "servable template file")
}

func (r Library) TemplateReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ListTutorTemplateReferencesBatch(ctx, dbgen.ListTutorTemplateReferencesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(ids, err, "a batch of the tutor's template references")
}

func (r Library) RemoveTemplateReferencesOfTutor(ctx context.Context, tutor domain.UserID, attachments []domain.AttachmentID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveTutorTemplateReferences(ctx, dbgen.RemoveTutorTemplateReferencesParams{
		TutorID: tutor, AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove the tutor's template references")
}

func (r Library) DeleteTemplatesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorTemplatesBatch(ctx, dbgen.DeleteTutorTemplatesBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's templates")
}

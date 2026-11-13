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

type Library struct {
	queries *dbgen.Queries
}

type (
	FolderRow         = dbgen.ListFoldersOfTutorRow
	MaterialRow       = dbgen.GetLiveMaterialRow
	MaterialState     = dbgen.GetMaterialStateRow
	MaterialCounts    = dbgen.CountMaterialsOfTutorRow
	BindableLibrary   = dbgen.LockBindableLibraryUploadRow
	VisibleMaterial   = dbgen.VisibleMaterialReferencesRow
	MaterialListAfter struct {
		Title string
		Kind  domain.LibraryItemKind
		ID    domain.EntityID
	}
)

type NewFolder struct {
	ID             domain.FolderID
	Tutor          domain.UserID
	Parent         *domain.FolderID
	Name           string
	NormalisedName domain.NormalisedText
	Now            time.Time
}

type NewMaterial struct {
	ID              domain.MaterialID
	Tutor           domain.UserID
	Folder          *domain.FolderID
	Title           string
	NormalisedTitle domain.NormalisedText
	Attachment      domain.AttachmentID
	Now             time.Time
}

type MaterialPage struct {
	Tutor    domain.UserID
	Folder   *domain.FolderID
	Pattern  *string
	After    MaterialListAfter
	PageSize int32
}

func (r Library) Folders(ctx context.Context, tutor domain.UserID) ([]FolderRow, error) {
	rows, err := r.queries.ListFoldersOfTutor(ctx, tutor)
	return one(rows, err, "folders of the tutor")
}

func (r Library) CountFolders(ctx context.Context, tutor domain.UserID) (int64, error) {
	count, err := r.queries.CountFoldersOfTutor(ctx, tutor)
	return one(count, err, "count the tutor's folders")
}

func (r Library) FolderNameTaken(
	ctx context.Context, tutor domain.UserID, parent *domain.FolderID, name domain.NormalisedText, excluded domain.FolderID,
) (bool, error) {
	taken, err := r.queries.FolderNameTaken(ctx, dbgen.FolderNameTakenParams{
		TutorID: tutor, ParentID: optionalUUID(parent), NormalisedName: string(name), ExcludedID: excluded,
	})
	return one(taken, err, "whether a sibling folder has the name")
}

func (r Library) CreateFolder(ctx context.Context, folder NewFolder) error {
	return done(r.queries.CreateFolder(ctx, dbgen.CreateFolderParams{
		ID: folder.ID, TutorID: folder.Tutor, ParentID: optionalUUID(folder.Parent), Name: folder.Name,
		NormalisedName: string(folder.NormalisedName), Now: folder.Now,
	}), "create a folder")
}

func (r Library) UpdateFolder(ctx context.Context, folder NewFolder) (bool, error) {
	rows, err := r.queries.UpdateFolder(ctx, dbgen.UpdateFolderParams{
		ID: folder.ID, TutorID: folder.Tutor, ParentID: optionalUUID(folder.Parent), Name: folder.Name,
		NormalisedName: string(folder.NormalisedName), Now: folder.Now,
	})
	return applied(rows, err, "update a folder")
}

func (r Library) FolderHasContents(ctx context.Context, tutor domain.UserID, folder domain.FolderID) (bool, error) {
	contents, err := r.queries.FolderHasContents(ctx, dbgen.FolderHasContentsParams{
		FolderID: pgtype.UUID{Bytes: folder, Valid: true}, TutorID: pgtype.UUID{Bytes: tutor, Valid: true},
	})
	return one(contents, err, "whether a folder has contents")
}

func (r Library) DeleteFolder(ctx context.Context, tutor domain.UserID, folder domain.FolderID) (bool, error) {
	rows, err := r.queries.DeleteFolder(ctx, dbgen.DeleteFolderParams{ID: folder, TutorID: tutor})
	return applied(rows, err, "delete a folder")
}

func (r Library) CountMaterials(ctx context.Context, tutor domain.UserID) (MaterialCounts, error) {
	counts, err := r.queries.CountMaterialsOfTutor(ctx, tutor)
	return one(counts, err, "count the tutor's materials")
}

func (r Library) LockBindableUpload(ctx context.Context, tutor domain.UserID, id domain.AttachmentID, now time.Time) (BindableLibrary, error) {
	upload, err := r.queries.LockBindableLibraryUpload(ctx, dbgen.LockBindableLibraryUploadParams{ID: id, UploaderID: &tutor, Now: now})
	return one(upload, err, "lock a bindable library upload")
}

func (r Library) BindMaterialFile(ctx context.Context, material domain.MaterialID, attachment domain.AttachmentID, now time.Time) error {
	bound, err := r.queries.BindAttachment(ctx, dbgen.BindAttachmentParams{Now: now, ID: attachment})
	if err != nil {
		return done(err, "bind a library upload")
	}
	if bound != 1 {
		return errors.New("bind a library upload: the locked upload was no longer pending")
	}
	return done(r.queries.CreateAttachmentReference(ctx, dbgen.CreateAttachmentReferenceParams{
		AttachmentID: attachment, RefKind: domain.ReferenceMaterial, RefID: domain.EntityID(material), CreatedAt: now,
	}), "create a material reference")
}

func (r Library) Charge(ctx context.Context, tutor domain.UserID, attachment domain.AttachmentID, now time.Time) (bool, error) {
	_, err := r.queries.ChargeAttachments(ctx, dbgen.ChargeAttachmentsParams{
		Now: now, TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, AttachmentIds: uuids([]domain.AttachmentID{attachment}),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("charge a library file: %w", err)
	}
	return true, nil
}

func (r Library) CreateMaterial(ctx context.Context, material NewMaterial) error {
	return done(r.queries.CreateMaterial(ctx, dbgen.CreateMaterialParams{
		ID: material.ID, TutorID: material.Tutor, FolderID: optionalUUID(material.Folder), Title: material.Title,
		NormalisedTitle: string(material.NormalisedTitle), AttachmentID: material.Attachment, Now: material.Now,
	}), "create a material")
}

func (r Library) MaterialState(ctx context.Context, tutor domain.UserID, id domain.MaterialID) (MaterialState, error) {
	state, err := r.queries.GetMaterialState(ctx, dbgen.GetMaterialStateParams{ID: id, TutorID: tutor})
	return one(state, err, "material state")
}

func (r Library) LiveMaterial(ctx context.Context, tutor domain.UserID, id domain.MaterialID) (MaterialRow, error) {
	row, err := r.queries.GetLiveMaterial(ctx, dbgen.GetLiveMaterialParams{ID: id, TutorID: tutor})
	return one(row, err, "live material")
}

func (r Library) UpdateMaterial(ctx context.Context, material NewMaterial) (bool, error) {
	rows, err := r.queries.UpdateMaterial(ctx, dbgen.UpdateMaterialParams{
		ID: material.ID, TutorID: material.Tutor, FolderID: optionalUUID(material.Folder), Title: material.Title,
		NormalisedTitle: string(material.NormalisedTitle), Now: material.Now,
	})
	return applied(rows, err, "update a material")
}

func (r Library) SoftDeleteMaterial(ctx context.Context, tutor domain.UserID, id domain.MaterialID, now time.Time) (bool, error) {
	rows, err := r.queries.SoftDeleteMaterial(ctx, dbgen.SoftDeleteMaterialParams{ID: id, TutorID: tutor, Now: now})
	return applied(rows, err, "soft-delete a material")
}

func (r Library) RemoveMaterialReference(ctx context.Context, id domain.MaterialID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveMaterialReference(ctx, pgtype.UUID{Bytes: id, Valid: true})
	return one(removed, err, "remove a material reference")
}

func (r Library) ReleaseCharges(ctx context.Context, attachments []domain.AttachmentID, now time.Time) ([]ReleasedCharge, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	released, err := r.queries.ReleaseUnreferencedCharges(ctx, dbgen.ReleaseUnreferencedChargesParams{Now: now, AttachmentIds: uuids(attachments)})
	return one(released, err, "release unreferenced library charges")
}

func (r Library) MarkUnreferencedDeleted(ctx context.Context, attachments []domain.AttachmentID, now time.Time) ([]UnreferencedUpload, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	rows, err := r.queries.MarkUnreferencedAttachmentsDeleted(ctx, dbgen.MarkUnreferencedAttachmentsDeletedParams{Now: now, AttachmentIds: uuids(attachments)})
	if err != nil {
		return nil, done(err, "mark unreferenced library files deleted")
	}
	deleted := make([]UnreferencedUpload, 0, len(rows))
	for _, row := range rows {
		deleted = append(deleted, UnreferencedUpload(row))
	}
	return deleted, nil
}

func (r Library) Materials(ctx context.Context, page MaterialPage) ([]MaterialRow, error) {
	afterID := pgtype.UUID{Bytes: page.After.ID, Valid: true}
	kind := string(page.After.Kind)
	switch {
	case page.Pattern != nil:
		rows, err := r.queries.SearchMaterials(ctx, dbgen.SearchMaterialsParams{
			TutorID: page.Tutor, Pattern: *page.Pattern, AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID, PageSize: page.PageSize,
		})
		return convertRows(rows, err, "search materials", func(row dbgen.SearchMaterialsRow) MaterialRow { return MaterialRow(row) })
	case page.Folder != nil:
		rows, err := r.queries.ListFolderMaterials(ctx, dbgen.ListFolderMaterialsParams{
			TutorID: page.Tutor, FolderID: optionalUUID(page.Folder), AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID,
			PageSize: page.PageSize,
		})
		return convertRows(rows, err, "materials of a folder", func(row dbgen.ListFolderMaterialsRow) MaterialRow { return MaterialRow(row) })
	}
	rows, err := r.queries.ListRootMaterials(ctx, dbgen.ListRootMaterialsParams{
		TutorID: page.Tutor, AfterTitle: page.After.Title, AfterKind: kind, AfterID: afterID, PageSize: page.PageSize,
	})
	return convertRows(rows, err, "materials at the root", func(row dbgen.ListRootMaterialsRow) MaterialRow { return MaterialRow(row) })
}

func convertRows[Row any, Converted any](rows []Row, err error, what string, convert func(Row) Converted) ([]Converted, error) {
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	converted := make([]Converted, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, convert(row))
	}
	return converted, nil
}

func (r Library) VisibleMaterials(ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID) ([]VisibleMaterial, error) {
	rows, err := r.queries.VisibleMaterialReferences(ctx, dbgen.VisibleMaterialReferencesParams{
		Viewer: pgtype.UUID{Bytes: viewer, Valid: true}, AttachmentID: attachment,
	})
	return one(rows, err, "visible material references")
}

func (r Library) ServableMaterialFile(ctx context.Context, attachment domain.AttachmentID, material domain.EntityID) (ServableFile, error) {
	row, err := r.queries.GetServableMaterialFile(ctx, dbgen.GetServableMaterialFileParams{
		MaterialID: pgtype.UUID{Bytes: material, Valid: true}, ID: attachment,
	})
	return one(ServableFile(row), err, "servable material file")
}

func (r Library) MaterialReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ListTutorMaterialReferencesBatch(ctx, dbgen.ListTutorMaterialReferencesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(ids, err, "a batch of the tutor's material references")
}

func (r Library) RemoveMaterialReferencesOfTutor(ctx context.Context, tutor domain.UserID, attachments []domain.AttachmentID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveTutorMaterialReferences(ctx, dbgen.RemoveTutorMaterialReferencesParams{
		TutorID: tutor, AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove the tutor's material references")
}

func (r Library) DeleteMaterialsBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorMaterialsBatch(ctx, dbgen.DeleteTutorMaterialsBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's materials")
}

func (r Library) DeleteLeafFoldersBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorLeafFoldersBatch(ctx, dbgen.DeleteTutorLeafFoldersBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's leaf folders")
}

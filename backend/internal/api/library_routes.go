package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/library"
)

const (
	libraryPath          = "/api/v1/library"
	libraryFoldersPath   = libraryPath + "/folders"
	libraryMaterialsPath = libraryPath + "/materials"
)

var errLibraryIDUnparsed = errors.New("a library identifier passed the schema but is not a UUID")

type libraryInput struct {
	Folder string `query:"folder" maxLength:"64"`
	Q      string `query:"q" maxLength:"200"`
	Limit  int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string `query:"cursor" maxLength:"1200"`
}

type libraryOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Library
}

type libraryFolderListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.LibraryFolderList
}

type createLibraryFolderInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.NewLibraryFolder
}

type libraryItemInput struct {
	ID string `path:"id"`
}

type updateLibraryFolderInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.LibraryFolderPatch
}

type libraryFolderOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.LibraryFolder
}

type createLibraryMaterialInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.NewLibraryMaterial
}

type updateLibraryMaterialInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.LibraryMaterialPatch
}

type libraryItemOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.LibraryItem
}

type libraryDeletedOutput struct {
	CacheControl string `header:"Cache-Control"`
}

func (r *router) registerLibrary() {
	listing := operation("library-get", http.MethodGet, libraryPath)
	listing.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, listing, AudienceTutorUnverifiedOK, r.libraryListing)

	folders := operation("library-folders-list", http.MethodGet, libraryFoldersPath)
	folders.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, folders, AudienceTutorUnverifiedOK, r.libraryFolders)

	createFolder := operation("library-folders-create", http.MethodPost, libraryFoldersPath)
	createFolder.DefaultStatus = http.StatusCreated
	createFolder.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, createFolder, AudienceTutorUnverifiedOK, r.createLibraryFolder)

	updateFolder := operation("library-folders-update", http.MethodPatch, libraryFoldersPath+"/{id}")
	updateFolder.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, updateFolder, AudienceTutorUnverifiedOK, r.updateLibraryFolder)

	deleteFolder := operation("library-folders-delete", http.MethodDelete, libraryFoldersPath+"/{id}")
	deleteFolder.DefaultStatus = http.StatusNoContent
	deleteFolder.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, deleteFolder, AudienceTutorUnverifiedOK, r.deleteLibraryFolder)

	createMaterial := operation("library-materials-create", http.MethodPost, libraryMaterialsPath)
	createMaterial.DefaultStatus = http.StatusCreated
	createMaterial.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, createMaterial, AudienceTutorUnverifiedOK, r.createLibraryMaterial)

	updateMaterial := operation("library-materials-update", http.MethodPatch, libraryMaterialsPath+"/{id}")
	updateMaterial.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, updateMaterial, AudienceTutorUnverifiedOK, r.updateLibraryMaterial)

	deleteMaterial := operation("library-materials-delete", http.MethodDelete, libraryMaterialsPath+"/{id}")
	deleteMaterial.DefaultStatus = http.StatusNoContent
	deleteMaterial.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, deleteMaterial, AudienceTutorUnverifiedOK, r.deleteLibraryMaterial)
}

func (r *router) libraryListing(ctx context.Context, input *libraryInput) (*libraryOutput, error) {
	query := library.ListingQuery{Limit: input.Limit, Cursor: input.Cursor}
	if input.Folder != "" && input.Q != "" {
		return nil, validationRefusal(ctx, "query.folder", domain.FieldNotAllowed)
	}
	if input.Folder != "" {
		folder, valid := domain.ParseUUID[domain.FolderID](input.Folder)
		if !valid {
			return nil, NewError(ctx, domain.ErrorNotFound)
		}
		query.Folder = &folder
	}
	if input.Q != "" {
		query.Query = &input.Q
	}
	listing, err := r.library.Listing(ctx, principalFrom(ctx).UserID, query)
	if err != nil {
		return nil, r.libraryError(ctx, "library.get.failed", err)
	}
	body := schemas.Library{
		Path: pathBody(listing.Path), Folders: foldersBody(listing.Folders), Items: make([]schemas.LibraryItem, 0, len(listing.Items)),
		NextCursor: listing.NextCursor, TemplateCount: listing.TemplateCount,
	}
	if listing.Folder != nil {
		folder := folderBody(*listing.Folder)
		body.Folder = &folder
	}
	for _, material := range listing.Items {
		body.Items = append(body.Items, materialBody(material))
	}
	return &libraryOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) libraryFolders(ctx context.Context, _ *struct{}) (*libraryFolderListOutput, error) {
	folders, err := r.library.Folders(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.libraryError(ctx, "library.folders.list.failed", err)
	}
	return &libraryFolderListOutput{CacheControl: noStore, Body: schemas.LibraryFolderList{Folders: foldersBody(folders)}}, nil
}

func (r *router) createLibraryFolder(ctx context.Context, input *createLibraryFolderInput) (*libraryFolderOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	parent, err := optionalLibraryID[domain.FolderID](input.Body.ParentID)
	if err != nil {
		return nil, r.internal(ctx, "library.folders.create.failed", err)
	}
	folder, err := r.library.CreateFolder(ctx, tutorActorFrom(ctx), library.NewFolder{Name: input.Body.Name, Parent: parent})
	if err != nil {
		return nil, r.libraryError(ctx, "library.folders.create.failed", err)
	}
	return &libraryFolderOutput{CacheControl: noStore, Body: folderBody(folder)}, nil
}

func (r *router) updateLibraryFolder(ctx context.Context, input *updateLibraryFolderInput) (*libraryFolderOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.FolderID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if input.Body.Name == nil && !input.Body.ParentID.Set {
		return nil, validationRefusal(ctx, "body", domain.FieldRequired)
	}
	parent, err := optionalLibraryID[domain.FolderID](input.Body.ParentID.Value)
	if err != nil {
		return nil, r.internal(ctx, "library.folders.update.failed", err)
	}
	change := library.FolderChange{Name: input.Body.Name, Parent: parent, ParentSet: input.Body.ParentID.Set}
	folder, err := r.library.UpdateFolder(ctx, tutorActorFrom(ctx), id, change)
	if err != nil {
		return nil, r.libraryError(ctx, "library.folders.update.failed", err)
	}
	return &libraryFolderOutput{CacheControl: noStore, Body: folderBody(folder)}, nil
}

func (r *router) deleteLibraryFolder(ctx context.Context, input *libraryItemInput) (*libraryDeletedOutput, error) {
	id, valid := domain.ParseUUID[domain.FolderID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.library.DeleteFolder(ctx, tutorActorFrom(ctx), id); err != nil {
		return nil, r.libraryError(ctx, "library.folders.delete.failed", err)
	}
	return &libraryDeletedOutput{CacheControl: noStore}, nil
}

func (r *router) createLibraryMaterial(ctx context.Context, input *createLibraryMaterialInput) (*libraryItemOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	attachment, valid := domain.ParseUUID[domain.AttachmentID](input.Body.AttachmentID)
	if !valid {
		return nil, r.internal(ctx, "library.materials.create.failed", errLibraryIDUnparsed)
	}
	folder, err := optionalLibraryID[domain.FolderID](input.Body.FolderID)
	if err != nil {
		return nil, r.internal(ctx, "library.materials.create.failed", err)
	}
	request := library.NewMaterial{Attachment: attachment, Title: input.Body.Title, Folder: folder}
	material, err := r.library.CreateMaterial(ctx, tutorActorFrom(ctx), request)
	if err != nil {
		return nil, r.libraryError(ctx, "library.materials.create.failed", err)
	}
	return &libraryItemOutput{CacheControl: noStore, Body: materialBody(material)}, nil
}

func (r *router) updateLibraryMaterial(ctx context.Context, input *updateLibraryMaterialInput) (*libraryItemOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.MaterialID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if input.Body.Title == nil && !input.Body.FolderID.Set {
		return nil, validationRefusal(ctx, "body", domain.FieldRequired)
	}
	folder, err := optionalLibraryID[domain.FolderID](input.Body.FolderID.Value)
	if err != nil {
		return nil, r.internal(ctx, "library.materials.update.failed", err)
	}
	change := library.MaterialChange{Title: input.Body.Title, Folder: folder, FolderSet: input.Body.FolderID.Set}
	material, err := r.library.UpdateMaterial(ctx, tutorActorFrom(ctx), id, change)
	if err != nil {
		return nil, r.libraryError(ctx, "library.materials.update.failed", err)
	}
	return &libraryItemOutput{CacheControl: noStore, Body: materialBody(material)}, nil
}

func (r *router) deleteLibraryMaterial(ctx context.Context, input *libraryItemInput) (*libraryDeletedOutput, error) {
	id, valid := domain.ParseUUID[domain.MaterialID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.library.DeleteMaterial(ctx, tutorActorFrom(ctx), id); err != nil {
		return nil, r.libraryError(ctx, "library.materials.delete.failed", err)
	}
	return &libraryDeletedOutput{CacheControl: noStore}, nil
}

func optionalLibraryID[ID ~[16]byte](text *string) (*ID, error) {
	if text == nil {
		return nil, nil
	}
	id, valid := domain.ParseUUID[ID](*text)
	if !valid {
		return nil, errLibraryIDUnparsed
	}
	return &id, nil
}

func validationRefusal(ctx context.Context, path string, code domain.FieldCode) *Error {
	fields := []schemas.FieldError{{Path: path, Code: string(code)}}
	return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
}

func folderBody(folder library.Folder) schemas.LibraryFolder {
	body := schemas.LibraryFolder{ID: folder.ID.String(), Name: folder.Name}
	if folder.Parent != nil {
		parent := folder.Parent.String()
		body.ParentID = &parent
	}
	return body
}

func foldersBody(folders []library.Folder) []schemas.LibraryFolder {
	body := make([]schemas.LibraryFolder, 0, len(folders))
	for _, folder := range folders {
		body = append(body, folderBody(folder))
	}
	return body
}

func pathBody(path []library.PathEntry) []schemas.LibraryPathEntry {
	body := make([]schemas.LibraryPathEntry, 0, len(path))
	for _, entry := range path {
		body = append(body, schemas.LibraryPathEntry{ID: entry.ID.String(), Name: entry.Name})
	}
	return body
}

func materialBody(material library.Material) schemas.LibraryItem {
	attachment, filename, mediaType, size := material.Attachment.String(), material.Filename, material.MediaType, material.SizeBytes
	body := schemas.LibraryItem{
		Kind: domain.LibraryItemMaterial, ID: material.ID.String(), Title: material.Title, UpdatedAt: material.UpdatedAt.UTC(),
		AttachmentID: &attachment, Filename: &filename, MediaType: &mediaType, SizeBytes: &size, Width: material.Width, Height: material.Height,
		URL: material.URL, ExpiresAt: utc(material.ExpiresAt),
	}
	if material.Folder != nil {
		folder := material.Folder.String()
		body.FolderID = &folder
	}
	if material.Path != nil {
		body.Path = pathBody(material.Path)
	}
	return body
}

func (r *router) libraryError(ctx context.Context, event string, err error) error {
	var invalid *library.ValidationError
	var throttled *library.ThrottledError
	switch {
	case errors.Is(err, library.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, library.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, library.ErrNotEditable):
		return NewError(ctx, domain.ErrorNotEditable)
	case errors.Is(err, library.ErrQuotaExceeded):
		return NewError(ctx, domain.ErrorQuotaExceeded)
	case errors.As(err, &invalid):
		fields := make([]schemas.FieldError, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			fields = append(fields, schemas.FieldError{Path: field.Path, Code: string(field.Code)})
		}
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}

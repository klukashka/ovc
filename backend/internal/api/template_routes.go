package api

import (
	"context"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/library"
)

const libraryTemplatesPath = libraryPath + "/templates"

type createTemplateInput struct {
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"64" pattern:"^[A-Za-z0-9._-]{1,64}$"`
	Body           schemas.NewTemplate
}

type saveTemplateInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.TemplateSave
}

type moveTemplateInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.TemplateMove
}

type duplicateTemplateInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.TemplateDuplicate
}

type templateOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Template
}

type studentTemplateOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentTemplate
}

func (r *router) registerTemplates() {
	create := operation("library-templates-create", http.MethodPost, libraryTemplatesPath)
	create.DefaultStatus = http.StatusCreated
	create.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, create, AudienceTutorUnverifiedOK, r.createTemplate)

	get := operation("library-templates-get", http.MethodGet, libraryTemplatesPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, get, AudienceTutorUnverifiedOK, r.template)

	save := operation("library-templates-save", http.MethodPut, libraryTemplatesPath+"/{id}")
	save.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, save, AudienceTutorUnverifiedOK, r.saveTemplate)

	move := operation("library-templates-move", http.MethodPatch, libraryTemplatesPath+"/{id}")
	move.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, move, AudienceTutorUnverifiedOK, r.moveTemplate)

	studentView := operation("library-templates-student-view", http.MethodGet, libraryTemplatesPath+"/{id}/student-view")
	studentView.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, studentView, AudienceTutorUnverifiedOK, r.templateStudentView)

	duplicate := operation("library-templates-duplicate", http.MethodPost, libraryTemplatesPath+"/{id}/duplicate")
	duplicate.DefaultStatus = http.StatusCreated
	duplicate.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, duplicate, AudienceTutorUnverifiedOK, r.duplicateTemplate)

	remove := operation("library-templates-delete", http.MethodDelete, libraryTemplatesPath+"/{id}")
	remove.DefaultStatus = http.StatusNoContent
	remove.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, remove, AudienceTutorUnverifiedOK, r.deleteTemplate)
}

func (r *router) createTemplate(ctx context.Context, input *createTemplateInput) (*templateOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	folder, err := optionalLibraryID[domain.FolderID](input.Body.FolderID)
	if err != nil {
		return nil, r.internal(ctx, "library.templates.create.failed", err)
	}
	request := library.NewTemplate{Title: input.Body.Title, Folder: folder, Blocks: blocksOf(input.Body.Blocks)}
	if input.IdempotencyKey != "" {
		request.CreateKey = &input.IdempotencyKey
	}
	template, err := r.library.CreateTemplate(ctx, tutorActorFrom(ctx), request)
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.create.failed", err)
	}
	return &templateOutput{CacheControl: noStore, Body: templateBody(template)}, nil
}

func (r *router) template(ctx context.Context, input *libraryItemInput) (*templateOutput, error) {
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	template, err := r.library.Template(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.get.failed", err)
	}
	return &templateOutput{CacheControl: noStore, Body: templateBody(template)}, nil
}

func (r *router) saveTemplate(ctx context.Context, input *saveTemplateInput) (*templateOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	saveID, valid := domain.ParseUUID[domain.TemplateSaveID](input.Body.SaveID)
	if !valid {
		return nil, r.internal(ctx, "library.templates.save.failed", errLibraryIDUnparsed)
	}
	save := library.TemplateSave{Title: input.Body.Title, Blocks: blocksOf(input.Body.Blocks), Version: input.Body.Version, SaveID: saveID}
	template, err := r.library.SaveTemplate(ctx, tutorActorFrom(ctx), id, save)
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.save.failed", err)
	}
	return &templateOutput{CacheControl: noStore, Body: templateBody(template)}, nil
}

func (r *router) moveTemplate(ctx context.Context, input *moveTemplateInput) (*templateOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	folder, err := optionalLibraryID[domain.FolderID](input.Body.FolderID)
	if err != nil {
		return nil, r.internal(ctx, "library.templates.move.failed", err)
	}
	template, err := r.library.MoveTemplate(ctx, tutorActorFrom(ctx), id, library.TemplateMove{Folder: folder, Version: input.Body.Version})
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.move.failed", err)
	}
	return &templateOutput{CacheControl: noStore, Body: templateBody(template)}, nil
}

func (r *router) templateStudentView(ctx context.Context, input *libraryItemInput) (*studentTemplateOutput, error) {
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	view, err := r.library.StudentView(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.student_view.failed", err)
	}
	return &studentTemplateOutput{CacheControl: noStore, Body: studentTemplateBody(view)}, nil
}

func (r *router) duplicateTemplate(ctx context.Context, input *duplicateTemplateInput) (*templateOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	folder, err := optionalLibraryID[domain.FolderID](input.Body.FolderID)
	if err != nil {
		return nil, r.internal(ctx, "library.templates.duplicate.failed", err)
	}
	template, err := r.library.DuplicateTemplate(ctx, tutorActorFrom(ctx), id, library.TemplateCopy{Title: input.Body.Title, Folder: folder})
	if err != nil {
		return nil, r.libraryError(ctx, "library.templates.duplicate.failed", err)
	}
	return &templateOutput{CacheControl: noStore, Body: templateBody(template)}, nil
}

func (r *router) deleteTemplate(ctx context.Context, input *libraryItemInput) (*libraryDeletedOutput, error) {
	id, valid := domain.ParseUUID[domain.TemplateID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.library.DeleteTemplate(ctx, tutorActorFrom(ctx), id); err != nil {
		return nil, r.libraryError(ctx, "library.templates.delete.failed", err)
	}
	return &libraryDeletedOutput{CacheControl: noStore}, nil
}

func blocksOf(inputs []schemas.TemplateBlockInput) []domain.TemplateBlock {
	blocks := make([]domain.TemplateBlock, 0, len(inputs))
	for _, input := range inputs {
		block := domain.TemplateBlock{
			ID: input.ID, Type: input.Type, Text: input.Text, MaterialID: input.MaterialID, Prompt: input.Prompt, ImageIDs: input.ImageIDs,
			Scoring: input.Scoring, AcceptedAnswers: input.AcceptedAnswers, Value: input.Value, Tolerance: input.Tolerance,
			MaxLength: input.MaxLength, AllowedKinds: input.AllowedKinds, MaxFiles: input.MaxFiles, Points: input.Points,
			Explanation: input.Explanation,
		}
		if input.Options != nil {
			options := make([]domain.TemplateOption, 0, len(*input.Options))
			for _, option := range *input.Options {
				options = append(options, domain.TemplateOption{ID: option.ID, Text: option.Text, Correct: option.Correct})
			}
			block.Options = &options
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func templateBody(template library.Template) schemas.Template {
	body := schemas.Template{
		ID: template.ID.String(), Title: template.Title, Blocks: make([]schemas.TemplateBlock, 0, len(template.Blocks)), Version: template.Version,
		Ready: template.Ready, Problems: make([]schemas.TemplateProblem, 0, len(template.Problems)), QuestionCount: template.QuestionCount,
		MaxPoints: template.MaxPoints, CreatedAt: template.CreatedAt.UTC(), UpdatedAt: template.UpdatedAt.UTC(),
	}
	if template.Folder != nil {
		folder := template.Folder.String()
		body.FolderID = &folder
	}
	for _, block := range template.Blocks {
		body.Blocks = append(body.Blocks, templateBlockBody(block))
	}
	for _, problem := range template.Problems {
		body.Problems = append(body.Problems, schemas.TemplateProblem{BlockID: problem.Block, Code: problem.Code})
	}
	return body
}

func templateBlockBody(block library.TemplateBlock) schemas.TemplateBlock {
	body := schemas.TemplateBlock{
		ID: block.ID, Type: block.Type, Text: block.Text, MaterialID: block.MaterialID, Prompt: block.Prompt, ImageIDs: block.ImageIDs,
		Scoring: block.Scoring, AcceptedAnswers: block.AcceptedAnswers, Value: block.Value, Tolerance: block.Tolerance, MaxLength: block.MaxLength,
		AllowedKinds: block.AllowedKinds, MaxFiles: block.MaxFiles, Points: block.Points, Explanation: block.Explanation,
	}
	if block.Options != nil {
		options := make([]schemas.TemplateOption, 0, len(*block.Options))
		for _, option := range *block.Options {
			options = append(options, schemas.TemplateOption{ID: option.ID, Text: option.Text, Correct: option.Correct})
		}
		body.Options = &options
	}
	if block.Material != nil {
		file := block.Material.File
		body.Material = &schemas.TemplateMaterial{
			MaterialID: block.Material.Material.String(), AttachmentID: file.Attachment.String(), Title: block.Material.Title, Filename: file.Filename,
			MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height, Deleted: block.Material.Deleted,
			URL: file.URL, ExpiresAt: file.ExpiresAt.UTC(),
		}
	}
	if block.ImageIDs != nil {
		images := make([]schemas.TemplateFile, 0, len(block.Images))
		for _, image := range block.Images {
			images = append(images, schemas.TemplateFile{
				AttachmentID: image.Attachment.String(), Filename: image.Filename, MediaType: image.MediaType, SizeBytes: image.SizeBytes,
				Width: image.Width, Height: image.Height, URL: image.URL, ExpiresAt: image.ExpiresAt.UTC(),
			})
		}
		body.Images = &images
	}
	return body
}

func studentTemplateBody(view library.StudentTemplate) schemas.StudentTemplate {
	body := schemas.StudentTemplate{Title: view.Title, Blocks: make([]schemas.StudentTemplateBlock, 0, len(view.Blocks))}
	for _, block := range view.Blocks {
		student := schemas.StudentTemplateBlock{ID: block.ID, Type: block.Type, Text: block.Text, Prompt: block.Prompt, MaxLength: block.MaxLength, MaxFiles: block.MaxFiles}
		if block.Options != nil {
			options := make([]schemas.StudentTemplateOption, 0, len(block.Options))
			for _, option := range block.Options {
				options = append(options, schemas.StudentTemplateOption{ID: option.ID, Text: option.Text})
			}
			student.Options = &options
		}
		if block.AllowedKinds != nil {
			kinds := block.AllowedKinds
			student.AllowedKinds = &kinds
		}
		if block.Material != nil {
			file := block.Material.File
			student.Material = &schemas.StudentTemplateMaterial{
				Title: block.Material.Title, Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width,
				Height: file.Height, URL: file.URL, ExpiresAt: file.ExpiresAt.UTC(),
			}
		}
		if block.Images != nil {
			images := make([]schemas.StudentTemplateFile, 0, len(block.Images))
			for _, image := range block.Images {
				images = append(images, schemas.StudentTemplateFile{
					Filename: image.Filename, MediaType: image.MediaType, SizeBytes: image.SizeBytes, Width: image.Width, Height: image.Height,
					URL: image.URL, ExpiresAt: image.ExpiresAt.UTC(),
				})
			}
			student.Images = &images
		}
		body.Blocks = append(body.Blocks, student)
	}
	return body
}

func templateItemBody(template library.TemplateListItem) schemas.LibraryItem {
	questionCount, maxPoints, ready, version := template.QuestionCount, template.MaxPoints, template.Ready, template.Version
	body := schemas.LibraryItem{
		Kind: domain.LibraryItemTemplate, ID: template.ID.String(), Title: template.Title, UpdatedAt: template.UpdatedAt.UTC(),
		QuestionCount: &questionCount, MaxPoints: &maxPoints, Ready: &ready, Version: &version,
	}
	if template.Folder != nil {
		folder := template.Folder.String()
		body.FolderID = &folder
	}
	if template.Path != nil {
		body.Path = pathBody(template.Path)
	}
	return body
}

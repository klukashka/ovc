package library

import (
	"bytes"
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

const limitTemplateSave = "template_saves"

var (
	errStoredMaterialGone = errors.New("a stored material block names a material that is gone")
	errStoredImageGone    = errors.New("a stored prompt image is gone")
)

type TemplateFile struct {
	Attachment domain.AttachmentID
	Filename   string
	MediaType  string
	SizeBytes  int64
	Width      *int32
	Height     *int32
	URL        string
	ExpiresAt  time.Time
}

type TemplateMaterial struct {
	Material domain.MaterialID
	Title    string
	Deleted  bool
	File     TemplateFile
}

type TemplateBlock struct {
	domain.TemplateBlock
	Material *TemplateMaterial
	Images   []TemplateFile
}

type Template struct {
	ID            domain.TemplateID
	Title         string
	Folder        *domain.FolderID
	Blocks        []TemplateBlock
	Version       int64
	Ready         bool
	Problems      []domain.TemplateProblem
	QuestionCount int32
	MaxPoints     int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NewTemplate struct {
	Title     string
	Folder    *domain.FolderID
	Blocks    []domain.TemplateBlock
	CreateKey *string
}

type TemplateSave struct {
	Title   string
	Blocks  []domain.TemplateBlock
	Version int64
	SaveID  domain.TemplateSaveID
}

type TemplateMove struct {
	Folder  *domain.FolderID
	Version int64
}

type TemplateCopy struct {
	Title  string
	Folder *domain.FolderID
}

type TemplateListItem struct {
	ID            domain.TemplateID
	Title         string
	Folder        *domain.FolderID
	Path          []PathEntry
	UpdatedAt     time.Time
	QuestionCount int32
	MaxPoints     int32
	Ready         bool
	Version       int64
}

type preparedContent struct {
	content repositories.TemplateContent
	checked domain.CheckedTemplate
}

type referencePlan struct {
	added   []domain.AttachmentID
	removed []domain.AttachmentID
	pending []domain.AttachmentID
}

func prepareContent(title string, blocks []domain.TemplateBlock) (preparedContent, error) {
	normalised, code, valid := domain.LibraryTitleLimits.Check(title)
	issues := []domain.FieldIssue{}
	if !valid {
		issues = append(issues, domain.FieldIssue{Path: fieldTitle, Code: code})
	}
	checked, blockIssues := domain.CheckTemplateBlocks(blocks)
	issues = append(issues, blockIssues...)
	if len(issues) > 0 {
		problems := make([]FieldProblem, 0, len(issues))
		for _, issue := range issues {
			problems = append(problems, FieldProblem{Path: issue.Path, Code: issue.Code})
		}
		return preparedContent{}, &ValidationError{Fields: problems}
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(checked.Blocks); err != nil {
		return preparedContent{}, fmt.Errorf("encode template blocks: %w", err)
	}
	stored := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
	return preparedContent{
		content: repositories.TemplateContent{
			Title: title, NormalisedTitle: normalised, Blocks: stored, BlocksBytes: int64(len(stored)), Summary: checked.Summary,
		},
		checked: checked,
	}, nil
}

func (s *Service) CreateTemplate(ctx context.Context, actor domain.TutorActor, request NewTemplate) (Template, error) {
	tutor := actor.UserID
	if request.CreateKey != nil {
		replayed, found, err := s.replayCreate(ctx, s.deps.Store.Repositories(), tutor, *request.CreateKey)
		if err != nil || found {
			return replayed, err
		}
	}
	var created Template
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		if request.CreateKey != nil {
			replayed, found, err := s.replayCreate(ctx, repos, tutor, *request.CreateKey)
			if err != nil || found {
				created = replayed
				return err
			}
		}
		_, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if err := requireFolder(tree, request.Folder); err != nil {
			return err
		}
		prepared, err := prepareContent(request.Title, request.Blocks)
		if err != nil {
			return err
		}
		id, err := core.NewID[domain.TemplateID](s.deps.Random)
		if err != nil {
			return err
		}
		plan, err := s.planReferences(ctx, repos, tutor, nil, prepared.checked, now)
		if err != nil {
			return err
		}
		if err := s.refuseBeyondTemplateCaps(ctx, repos, tutor); err != nil {
			return err
		}
		if err := s.refuseBeyondBudget(ctx, repos, tutor, nil, prepared.content.BlocksBytes); err != nil {
			return err
		}
		if err := repos.Library.CreateTemplate(ctx, repositories.NewTemplate{
			ID: id, Tutor: tutor, Folder: request.Folder, CreateKey: request.CreateKey, Content: prepared.content, Now: now,
		}); err != nil {
			return err
		}
		if err := s.applyReferences(ctx, repos, tutor, id, plan, now); err != nil {
			return err
		}
		created, err = s.templateOf(ctx, repos, tutor, id, now)
		if err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.template.created", tutorAttribute(tutor), templateAttribute(id))
		})
		return nil
	})
	s.logQuota(ctx, tutor, err)
	return created, err
}

func (s *Service) replayCreate(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, key string) (Template, bool, error) {
	existing, err := repos.Library.TemplateByCreateKey(ctx, tutor, key)
	if errors.Is(err, repositories.ErrNotFound) {
		return Template{}, false, nil
	}
	if err != nil {
		return Template{}, false, err
	}
	if existing.DeletedAt != nil {
		return Template{}, true, ErrNotFound
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Template{}, false, err
	}
	template, err := s.templateOf(ctx, repos, tutor, existing.ID, now)
	return template, true, err
}

func (s *Service) Template(ctx context.Context, tutor domain.UserID, id domain.TemplateID) (Template, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Template{}, err
	}
	var template Template
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		template, err = s.templateOf(ctx, repos, tutor, id, now)
		return err
	})
	return template, err
}

func (s *Service) SaveTemplate(ctx context.Context, actor domain.TutorActor, id domain.TemplateID, save TemplateSave) (Template, error) {
	tutor := actor.UserID
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		last, err := repos.Library.TemplateLastSave(ctx, tutor, id)
		if err == nil && last != nil && *last == save.SaveID {
			return nil
		}
		if err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		return s.reserveSave(ctx, repos, tutor, id, now)
	})
	if err != nil {
		return Template{}, err
	}
	var saved Template
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		state, err := repos.Library.LockTemplate(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if state.DeletedAt != nil {
			return ErrNotFound
		}
		if state.LastSaveID != nil && *state.LastSaveID == save.SaveID {
			saved, err = s.templateOf(ctx, repos, tutor, id, now)
			return err
		}
		if state.Version != save.Version {
			s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "library.save_conflict", tutorAttribute(tutor), templateAttribute(id))
			return ErrConflict
		}
		prepared, err := prepareContent(save.Title, save.Blocks)
		if err != nil {
			return err
		}
		plan, err := s.planReferences(ctx, repos, tutor, &id, prepared.checked, now)
		if err != nil {
			return err
		}
		if err := s.refuseBeyondBudget(ctx, repos, tutor, &id, prepared.content.BlocksBytes); err != nil {
			return err
		}
		if _, err := repos.Library.SaveTemplate(ctx, repositories.TemplateSave{
			ID: id, Tutor: tutor, SaveID: save.SaveID, Content: prepared.content, Now: now,
		}); err != nil {
			return err
		}
		if err := s.applyReferences(ctx, repos, tutor, id, plan, now); err != nil {
			return err
		}
		saved, err = s.templateOf(ctx, repos, tutor, id, now)
		if err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "library.template.saved", tutorAttribute(tutor), templateAttribute(id),
				slog.Int("block_count", len(prepared.checked.Blocks)), slog.Bool("ready", prepared.checked.Summary.Ready),
				slog.Int("references_added", len(plan.added)), slog.Int("references_removed", len(plan.removed)))
		})
		return nil
	})
	s.logQuota(ctx, tutor, err)
	return saved, err
}

func (s *Service) reserveSave(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, template domain.TemplateID, now time.Time) error {
	second, secondRetry := domain.FixedWindow(now, domain.TemplateSaveSecondWindow)
	perTemplate, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose:     domain.PurposeTemplateSavesSecond,
		Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeTemplateSavesSecond), tutor.String()+":"+template.String()),
		WindowStart: second,
	})
	if err != nil {
		return err
	}
	hour, hourRetry := domain.FixedWindow(now, domain.TemplateSaveHourWindow)
	perTutor, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose:     domain.PurposeTemplateSavesHour,
		Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeTemplateSavesHour), tutor.String()),
		WindowStart: hour,
	})
	if err != nil {
		return err
	}
	retryAfter := time.Duration(0)
	if perTemplate > domain.TemplateSavesPerSecond {
		retryAfter = secondRetry
	}
	if perTutor > domain.TemplateSavesPerHour {
		retryAfter = max(retryAfter, hourRetry)
	}
	if retryAfter == 0 {
		return nil
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "library.rate_limited", tutorAttribute(tutor), slog.String("limit", limitTemplateSave))
	return &ThrottledError{RetryAfter: retryAfter}
}

func (s *Service) MoveTemplate(ctx context.Context, actor domain.TutorActor, id domain.TemplateID, move TemplateMove) (Template, error) {
	tutor := actor.UserID
	var moved Template
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		state, err := repos.Library.LockTemplate(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) || (err == nil && state.DeletedAt != nil) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if err := requireFolder(tree, move.Folder); err != nil {
			return err
		}
		if state.Version != move.Version {
			return ErrConflict
		}
		if !sameFolder(move.Folder, state.FolderID) {
			if _, err := repos.Library.MoveTemplate(ctx, tutor, id, move.Folder, now); err != nil {
				return err
			}
			repos.AfterCommit(ctx, func(ctx context.Context) {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.template.moved", tutorAttribute(tutor), templateAttribute(id))
			})
		}
		moved, err = s.templateOf(ctx, repos, tutor, id, now)
		return err
	})
	return moved, err
}

func (s *Service) DuplicateTemplate(ctx context.Context, actor domain.TutorActor, id domain.TemplateID, copyRequest TemplateCopy) (Template, error) {
	tutor := actor.UserID
	var duplicate Template
	err := s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		source, err := repos.Library.LiveTemplate(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, tree, err := s.folderTree(ctx, repos, tutor)
		if err != nil {
			return err
		}
		if err := requireFolder(tree, copyRequest.Folder); err != nil {
			return err
		}
		normalised, code, valid := domain.LibraryTitleLimits.Check(copyRequest.Title)
		if !valid {
			return invalid(fieldTitle, code)
		}
		if err := s.refuseBeyondTemplateCaps(ctx, repos, tutor); err != nil {
			return err
		}
		if err := s.refuseBeyondBudget(ctx, repos, tutor, nil, source.BlocksBytes); err != nil {
			return err
		}
		references, err := repos.Library.TemplateReferences(ctx, id)
		if err != nil {
			return err
		}
		copyID, err := core.NewID[domain.TemplateID](s.deps.Random)
		if err != nil {
			return err
		}
		content := repositories.TemplateContent{
			Title: copyRequest.Title, NormalisedTitle: normalised, Blocks: source.Blocks, BlocksBytes: source.BlocksBytes,
			Summary: domain.TemplateSummary{QuestionCount: source.QuestionCount, MaxPoints: source.MaxPoints, Ready: source.Ready},
		}
		if err := repos.Library.CreateTemplate(ctx, repositories.NewTemplate{
			ID: copyID, Tutor: tutor, Folder: copyRequest.Folder, Content: content, Now: now,
		}); err != nil {
			return err
		}
		if err := repos.Library.AddTemplateReferences(ctx, copyID, references, now); err != nil {
			return err
		}
		duplicate, err = s.templateOf(ctx, repos, tutor, copyID, now)
		if err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.template.duplicated", tutorAttribute(tutor), templateAttribute(copyID),
				slog.String("source_template_id", id.String()))
			s.logReferencesAdded(ctx, references)
		})
		return nil
	})
	return duplicate, err
}

func (s *Service) DeleteTemplate(ctx context.Context, actor domain.TutorActor, id domain.TemplateID) error {
	tutor := actor.UserID
	return s.write(ctx, actor, func(repos repositories.Repositories, now time.Time) error {
		state, err := repos.Library.LockTemplate(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if state.DeletedAt != nil {
			return nil
		}
		if _, err := repos.Library.SoftDeleteTemplate(ctx, tutor, id, now); err != nil {
			return err
		}
		removed, err := repos.Library.RemoveAllTemplateReferences(ctx, id)
		if err != nil {
			return err
		}
		repos.AfterCommit(ctx, func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "library.template.deleted", tutorAttribute(tutor), templateAttribute(id))
		})
		return s.release(domain.ReferenceTemplate).attachments(ctx, repos, removed, now)
	})
}

func (s *Service) refuseBeyondTemplateCaps(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) error {
	templates, err := repos.Library.CountTemplates(ctx, tutor)
	if err != nil {
		return err
	}
	materials, err := repos.Library.CountMaterials(ctx, tutor)
	if err != nil {
		return err
	}
	if templates.Live >= s.deps.Caps.Templates || templates.Total+materials.Total >= s.deps.Caps.Rows {
		return invalid(fieldBody, domain.FieldTooMany)
	}
	return nil
}

func (s *Service) refuseBeyondBudget(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, excluded *domain.TemplateID, size int64) error {
	used, err := repos.Library.TemplateBlockBytes(ctx, tutor, excluded)
	if err != nil {
		return err
	}
	if used+size > s.deps.Caps.TemplateBlockBytes {
		return invalid(fieldBody, domain.FieldTooMany)
	}
	return nil
}

func (s *Service) planReferences(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, template *domain.TemplateID, checked domain.CheckedTemplate,
	now time.Time,
) (referencePlan, error) {
	var existing []domain.AttachmentID
	if template != nil {
		var err error
		if existing, err = repos.Library.TemplateReferences(ctx, *template); err != nil {
			return referencePlan{}, err
		}
	}
	wanted, pending, err := s.wantedImages(ctx, repos, tutor, existing, checked, now)
	if err != nil {
		return referencePlan{}, err
	}
	materialFiles, err := s.wantedMaterialFiles(ctx, repos, tutor, existing, checked)
	if err != nil {
		return referencePlan{}, err
	}
	for _, file := range materialFiles {
		if !slices.Contains(wanted, file) {
			wanted = append(wanted, file)
		}
	}
	plan := referencePlan{pending: pending}
	for _, attachment := range wanted {
		if !slices.Contains(existing, attachment) {
			plan.added = append(plan.added, attachment)
		}
	}
	for _, attachment := range existing {
		if !slices.Contains(wanted, attachment) {
			plan.removed = append(plan.removed, attachment)
		}
	}
	return plan, nil
}

func (s *Service) wantedImages(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, existing []domain.AttachmentID, checked domain.CheckedTemplate,
	now time.Time,
) ([]domain.AttachmentID, []domain.AttachmentID, error) {
	var images, fresh []domain.AttachmentID
	for _, block := range checked.Images {
		for _, image := range block.Images {
			if slices.Contains(images, image) {
				continue
			}
			images = append(images, image)
			if !slices.Contains(existing, image) {
				fresh = append(fresh, image)
			}
		}
	}
	pictures, err := repos.Library.PromptPictures(ctx, images)
	if err != nil {
		return nil, nil, err
	}
	pending, err := repos.Library.LockBindablePromptImages(ctx, tutor, fresh, now)
	if err != nil {
		return nil, nil, err
	}
	for _, image := range images {
		allowed := slices.Contains(pending, image) || (slices.Contains(existing, image) && slices.Contains(pictures, image))
		if !allowed {
			return nil, nil, ErrNotFound
		}
	}
	return images, pending, nil
}

func (s *Service) wantedMaterialFiles(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, existing []domain.AttachmentID, checked domain.CheckedTemplate,
) ([]domain.AttachmentID, error) {
	ids := make([]domain.MaterialID, 0, len(checked.Materials))
	for _, block := range checked.Materials {
		if !slices.Contains(ids, block.Material) {
			ids = append(ids, block.Material)
		}
	}
	rows, err := repos.Library.TemplateBlockMaterials(ctx, tutor, ids)
	if err != nil {
		return nil, err
	}
	files := make([]domain.AttachmentID, 0, len(rows))
	for _, id := range ids {
		index := slices.IndexFunc(rows, func(row repositories.TemplateBlockMaterial) bool { return row.ID == id })
		if index < 0 {
			return nil, ErrNotFound
		}
		row := rows[index]
		if row.Deleted && !slices.Contains(existing, row.AttachmentID) {
			return nil, ErrNotFound
		}
		files = append(files, row.AttachmentID)
	}
	return files, nil
}

func (s *Service) applyReferences(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, template domain.TemplateID, plan referencePlan, now time.Time,
) error {
	removed, err := repos.Library.RemoveTemplateReferences(ctx, template, plan.removed)
	if err != nil {
		return err
	}
	if err := s.release(domain.ReferenceTemplate).attachments(ctx, repos, removed, now); err != nil {
		return err
	}
	if err := repos.Library.BindUploads(ctx, plan.pending, now); err != nil {
		return err
	}
	if err := repos.Library.AddTemplateReferences(ctx, template, plan.added, now); err != nil {
		return err
	}
	charged, err := repos.Library.ChargeAll(ctx, tutor, plan.pending, now)
	if err != nil {
		return err
	}
	if !charged {
		return ErrQuotaExceeded
	}
	repos.AfterCommit(ctx, func(ctx context.Context) { s.logReferencesAdded(ctx, plan.added) })
	return nil
}

func (s *Service) logReferencesAdded(ctx context.Context, attachments []domain.AttachmentID) {
	for _, attachment := range attachments {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", attachmentAttribute(attachment),
			slog.String(recordRefKind, string(domain.ReferenceTemplate)))
	}
}

func (s *Service) logQuota(ctx context.Context, tutor domain.UserID, err error) {
	if errors.Is(err, ErrQuotaExceeded) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", tutorAttribute(tutor), slog.String("context", string(domain.ContextPromptImage)))
	}
}

func (s *Service) templateOf(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.TemplateID, now time.Time) (Template, error) {
	row, err := repos.Library.LiveTemplate(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	var stored []domain.TemplateBlock
	if err := json.Unmarshal(row.Blocks, &stored); err != nil {
		return Template{}, fmt.Errorf("decode stored template blocks: %w", err)
	}
	blocks, err := s.describeBlocks(ctx, repos, tutor, id, stored, now)
	if err != nil {
		return Template{}, err
	}
	return Template{
		ID: row.ID, Title: row.Title, Folder: row.FolderID, Blocks: blocks, Version: row.Version, Ready: row.Ready,
		Problems: domain.TemplateProblems(stored), QuestionCount: row.QuestionCount, MaxPoints: row.MaxPoints, CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *Service) describeBlocks(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, template domain.TemplateID, stored []domain.TemplateBlock,
	now time.Time,
) ([]TemplateBlock, error) {
	var materialIDs []domain.MaterialID
	var imageIDs []domain.AttachmentID
	for _, block := range stored {
		if block.MaterialID != nil {
			if material, valid := domain.ParseUUID[domain.MaterialID](*block.MaterialID); valid {
				materialIDs = append(materialIDs, material)
			}
		}
		if block.ImageIDs != nil {
			for _, text := range *block.ImageIDs {
				if image, valid := domain.ParseUUID[domain.AttachmentID](text); valid {
					imageIDs = append(imageIDs, image)
				}
			}
		}
	}
	materials, err := repos.Library.TemplateMaterialFiles(ctx, tutor, materialIDs)
	if err != nil {
		return nil, err
	}
	images, err := repos.Library.TemplateImageFiles(ctx, imageIDs)
	if err != nil {
		return nil, err
	}
	described := make([]TemplateBlock, 0, len(stored))
	for _, block := range stored {
		view := TemplateBlock{TemplateBlock: block}
		if block.MaterialID != nil {
			material, err := s.describeMaterial(materials, template, *block.MaterialID, now)
			if err != nil {
				return nil, err
			}
			view.Material = &material
		}
		if block.ImageIDs != nil {
			view.Images = make([]TemplateFile, 0, len(*block.ImageIDs))
			for _, text := range *block.ImageIDs {
				image, err := s.describeImage(images, template, text, now)
				if err != nil {
					return nil, err
				}
				view.Images = append(view.Images, image)
			}
		}
		described = append(described, view)
	}
	return described, nil
}

func (s *Service) describeMaterial(
	rows []repositories.TemplateMaterialFile, template domain.TemplateID, text string, now time.Time,
) (TemplateMaterial, error) {
	index := slices.IndexFunc(rows, func(row repositories.TemplateMaterialFile) bool { return row.ID.String() == text })
	if index < 0 {
		return TemplateMaterial{}, errStoredMaterialGone
	}
	row := rows[index]
	file := TemplateFile{
		Attachment: row.AttachmentID, Filename: row.Filename, MediaType: row.MediaType, SizeBytes: row.SizeBytes, Width: row.Width, Height: row.Height,
	}
	file.URL, file.ExpiresAt = s.deps.Addresses.TemplateAddress(row.AttachmentID, template, dispositionOf(row.Width), now)
	return TemplateMaterial{Material: row.ID, Title: row.Title, Deleted: row.Deleted, File: file}, nil
}

func (s *Service) describeImage(rows []repositories.TemplateImageFile, template domain.TemplateID, text string, now time.Time) (TemplateFile, error) {
	index := slices.IndexFunc(rows, func(row repositories.TemplateImageFile) bool { return row.ID.String() == text })
	if index < 0 {
		return TemplateFile{}, errStoredImageGone
	}
	row := rows[index]
	file := TemplateFile{
		Attachment: row.ID, Filename: row.Filename, MediaType: row.MediaType, SizeBytes: row.SizeBytes, Width: row.Width, Height: row.Height,
	}
	file.URL, file.ExpiresAt = s.deps.Addresses.TemplateAddress(row.ID, template, dispositionOf(row.Width), now)
	return file, nil
}

func dispositionOf(width *int32) domain.Disposition {
	if width != nil {
		return domain.DispositionInline
	}
	return domain.DispositionAttachment
}

func templateItemOf(row repositories.TemplateListRow) TemplateListItem {
	return TemplateListItem{
		ID: row.ID, Title: row.Title, Folder: row.FolderID, UpdatedAt: row.UpdatedAt, QuestionCount: row.QuestionCount, MaxPoints: row.MaxPoints,
		Ready: row.Ready, Version: row.Version,
	}
}

func templateAttribute(template domain.TemplateID) slog.Attr {
	return slog.String("template_id", template.String())
}

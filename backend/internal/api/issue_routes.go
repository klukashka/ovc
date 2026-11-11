package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/homework"
)

const (
	issuesPath          = "/api/v1/issues"
	fieldAudience       = "body.audience"
	fieldDeadlineBefore = "query.deadline_before"
)

var errIssueIDUnparsed = errors.New("a schema-checked identifier did not parse")

type createIssueInput struct {
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"64" pattern:"^[A-Za-z0-9._-]{1,64}$"`
	Body           schemas.NewIssue
}

type issuesInput struct {
	Status         domain.IssueStatus `query:"status" default:"active" enum:"active,withdrawn"`
	Group          string             `query:"group" maxLength:"64"`
	Student        string             `query:"student" maxLength:"64"`
	NeedsChecking  bool               `query:"needs_checking"`
	DeadlineBefore string             `query:"deadline_before" maxLength:"64"`
	Limit          int32              `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor         string             `query:"cursor" maxLength:"200"`
}

type issueInput struct {
	ID string `path:"id"`
}

type updateIssueInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.IssuePatch
}

type issueItemOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.IssueItem
}

type issuePageOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.IssuePage
}

type issueOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Issue
}

func (r *router) registerIssues() {
	create := operation("issues-create", http.MethodPost, issuesPath)
	create.DefaultStatus = http.StatusCreated
	create.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, create, AudienceTutor, r.createIssue)

	list := operation("issues-list", http.MethodGet, issuesPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, list, AudienceTutor, r.issues)

	get := operation("issues-get", http.MethodGet, issuesPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, get, AudienceTutor, r.issue)

	update := operation("issues-update", http.MethodPatch, issuesPath+"/{id}")
	update.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, update, AudienceTutor, r.updateIssue)

	withdraw := operation("issues-withdraw", http.MethodPost, issuesPath+"/{id}/withdraw")
	withdraw.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, withdraw, AudienceTutor, r.withdrawIssue)
}

func (r *router) createIssue(ctx context.Context, input *createIssueInput) (*issueItemOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	body := input.Body
	template, valid := domain.ParseUUID[domain.TemplateID](body.TemplateID)
	if !valid {
		return nil, r.internal(ctx, "homework.issues.create.failed", errIssueIDUnparsed)
	}
	audience, err := audienceOf(ctx, body.Audience)
	if err != nil {
		return nil, err
	}
	lesson, err := optionalLibraryID[domain.LessonID](body.LessonID)
	if err != nil {
		return nil, r.internal(ctx, "homework.issues.create.failed", err)
	}
	request := homework.NewIssue{Template: template, Audience: audience, Lesson: lesson, Deadline: utcInstant(body.DeadlineAt), Policy: body.AnswerPolicy}
	if input.IdempotencyKey != "" {
		request.CreateKey = &input.IdempotencyKey
	}
	item, err := r.homework.CreateIssue(ctx, tutorActorFrom(ctx), request)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.issues.create.failed", err)
	}
	return &issueItemOutput{CacheControl: noStore, Body: issueItemBody(item)}, nil
}

func audienceOf(ctx context.Context, input schemas.IssueAudienceInput) (homework.IssueAudienceRequest, error) {
	if (input.RelationshipID == nil) == (input.GroupID == nil) {
		return homework.IssueAudienceRequest{}, validationRefusal(ctx, fieldAudience, domain.FieldInvalid)
	}
	if input.RelationshipID != nil {
		relationship, valid := domain.ParseUUID[domain.RelationshipID](*input.RelationshipID)
		if !valid {
			return homework.IssueAudienceRequest{}, validationRefusal(ctx, fieldAudience, domain.FieldInvalid)
		}
		return homework.IssueAudienceRequest{Relationship: &relationship}, nil
	}
	group, valid := domain.ParseUUID[domain.GroupID](*input.GroupID)
	if !valid {
		return homework.IssueAudienceRequest{}, validationRefusal(ctx, fieldAudience, domain.FieldInvalid)
	}
	return homework.IssueAudienceRequest{Group: &group}, nil
}

func utcInstant(instant *time.Time) *time.Time {
	if instant == nil {
		return nil
	}
	value := instant.UTC()
	return &value
}

func (r *router) issues(ctx context.Context, input *issuesInput) (*issuePageOutput, error) {
	query := homework.IssueListQuery{Status: input.Status, NeedsChecking: input.NeedsChecking, Cursor: input.Cursor, Limit: input.Limit}
	if input.Group != "" {
		group, valid := domain.ParseUUID[domain.GroupID](input.Group)
		if !valid {
			return nil, NewError(ctx, domain.ErrorNotFound)
		}
		query.Group = &group
	}
	if input.Student != "" {
		student, valid := domain.ParseUUID[domain.RelationshipID](input.Student)
		if !valid {
			return nil, NewError(ctx, domain.ErrorNotFound)
		}
		query.Student = &student
	}
	if input.DeadlineBefore != "" {
		before, err := time.Parse(time.RFC3339Nano, input.DeadlineBefore)
		if err != nil {
			return nil, validationRefusal(ctx, fieldDeadlineBefore, domain.FieldInvalid)
		}
		before = before.UTC()
		query.DeadlineBefore = &before
	}
	page, err := r.homework.Issues(ctx, principalFrom(ctx).UserID, query)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.issues.list.failed", err)
	}
	body := schemas.IssuePage{Items: make([]schemas.IssueItem, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		body.Items = append(body.Items, issueItemBody(item))
	}
	return &issuePageOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) issue(ctx context.Context, input *issueInput) (*issueOutput, error) {
	id, valid := domain.ParseUUID[domain.IssueID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	issue, err := r.homework.Issue(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.issues.get.failed", err)
	}
	return &issueOutput{CacheControl: noStore, Body: issueBody(issue)}, nil
}

func (r *router) updateIssue(ctx context.Context, input *updateIssueInput) (*issueItemOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.IssueID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	change := homework.IssueChange{
		Deadline: homework.OptionalDeadline{Set: input.Body.DeadlineAt.Set, Value: utcInstant(input.Body.DeadlineAt.Value)},
		Version:  input.Body.Version,
	}
	if input.Body.LessonID.Set {
		lesson, err := optionalLibraryID[domain.LessonID](input.Body.LessonID.Value)
		if err != nil {
			return nil, r.internal(ctx, "homework.issues.update.failed", err)
		}
		change.Lesson = homework.OptionalLesson{Set: true, Value: lesson}
	}
	item, err := r.homework.UpdateIssue(ctx, tutorActorFrom(ctx), id, change)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.issues.update.failed", err)
	}
	return &issueItemOutput{CacheControl: noStore, Body: issueItemBody(item)}, nil
}

func (r *router) withdrawIssue(ctx context.Context, input *issueInput) (*issueItemOutput, error) {
	id, valid := domain.ParseUUID[domain.IssueID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	item, err := r.homework.WithdrawIssue(ctx, tutorActorFrom(ctx), id)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.issues.withdraw.failed", err)
	}
	return &issueItemOutput{CacheControl: noStore, Body: issueItemBody(item)}, nil
}

func (r *router) homeworkError(ctx context.Context, event string, err error) error {
	var invalid *homework.ValidationError
	var throttled *homework.ThrottledError
	switch {
	case errors.Is(err, homework.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, homework.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, homework.ErrNotEditable):
		return NewError(ctx, domain.ErrorNotEditable)
	case errors.Is(err, homework.ErrRelationshipNotActive):
		return NewError(ctx, domain.ErrorRelationshipNotActive)
	case errors.Is(err, homework.ErrAudienceNotActive):
		return NewError(ctx, domain.ErrorAudienceNotActive)
	case errors.Is(err, homework.ErrConflict):
		return NewError(ctx, domain.ErrorConflict)
	case errors.Is(err, homework.ErrIssueHasSubmissions):
		return NewError(ctx, domain.ErrorIssueHasSubmissions)
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

func issueItemBody(item homework.IssueItem) schemas.IssueItem {
	body := schemas.IssueItem{
		ID: item.ID.String(), Title: item.Title, DeadlineAt: utcInstant(item.Deadline), AnswerPolicy: item.Policy, Status: item.Status,
		IssuedAt: item.IssuedAt.UTC(), WithdrawnAt: utcInstant(item.WithdrawnAt), HasManualQuestions: item.HasManualQuestions,
		MaxPoints: item.MaxPoints, Version: item.Version,
		Progress: schemas.IssueProgress{
			Students: item.Progress.Students, Submitted: item.Progress.Submitted, Checked: item.Progress.Checked,
			Returned: item.Progress.Returned, Late: item.Progress.Late, NeedsChecking: item.Progress.NeedsChecking,
		},
	}
	if item.Lesson != nil {
		lesson := item.Lesson.String()
		body.LessonID = &lesson
	}
	if item.Audience.Relationship != nil {
		relationship, name := item.Audience.Relationship.String(), item.Audience.DisplayName
		body.Audience = schemas.IssueAudience{RelationshipID: &relationship, DisplayName: &name}
	}
	if item.Audience.Group != nil {
		group, name := item.Audience.Group.String(), item.Audience.Name
		body.Audience = schemas.IssueAudience{GroupID: &group, Name: &name}
	}
	if item.Submission != nil {
		body.Submission = &schemas.IssueStudentSubmission{
			ID: item.Submission.ID.String(), Status: item.Submission.Status, Late: item.Submission.Late, FinalPoints: item.Submission.FinalPoints,
			IssuedBeforeReclaim: item.Submission.IssuedBeforeReclaim,
		}
	}
	return body
}

func issueBody(issue homework.Issue) schemas.Issue {
	body := schemas.Issue{
		IssueItem: issueItemBody(issue.IssueItem), Blocks: make([]schemas.IssueBlock, 0, len(issue.Blocks)),
		Submissions: make([]schemas.IssueSubmission, 0, len(issue.Submissions)),
	}
	for _, block := range issue.Blocks {
		body.Blocks = append(body.Blocks, issueBlockBody(block))
	}
	for _, submission := range issue.Submissions {
		body.Submissions = append(body.Submissions, schemas.IssueSubmission{
			ID: submission.ID.String(), Student: schemas.IssueSubmissionStudent{RelationshipID: submission.Relationship.String(), DisplayName: submission.DisplayName},
			Status: submission.Status, Late: submission.Late, FirstSubmittedAt: utcInstant(submission.FirstSubmittedAt),
			SubmittedAt: utcInstant(submission.SubmittedAt), CheckedAt: utcInstant(submission.CheckedAt), FinalPoints: submission.FinalPoints,
			Percentage: submission.Percentage, IssuedBeforeReclaim: submission.IssuedBeforeReclaim, HasDraft: submission.HasDraft,
		})
	}
	return body
}

func issueBlockBody(block homework.IssueBlock) schemas.IssueBlock {
	body := schemas.IssueBlock{
		ID: block.ID, Type: block.Type, Text: block.Text, Prompt: block.Prompt, Scoring: block.Scoring, AcceptedAnswers: block.AcceptedAnswers,
		Value: block.Value, Tolerance: block.Tolerance, MaxLength: block.MaxLength, AllowedKinds: block.AllowedKinds, MaxFiles: block.MaxFiles,
		Points: block.Points, Explanation: block.Explanation,
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
		body.Material = &schemas.IssueMaterial{
			AttachmentID: file.Attachment.String(), Title: block.Material.Title, Filename: file.Filename, MediaType: file.MediaType,
			SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height, URL: file.URL, ExpiresAt: file.ExpiresAt.UTC(),
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

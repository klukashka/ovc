package api

import (
	"context"
	"errors"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
)

var errSaveIDUnparsed = errors.New("a schema-checked save identifier did not parse")

const (
	assignmentsPath = "/api/v1/assignments"
	submissionsPath = "/api/v1/submissions"
)

type assignmentsInput struct {
	Status domain.AssignmentGroup `query:"status" default:"todo" enum:"todo,submitted,checked,closed"`
	Tutor  string                 `query:"tutor" maxLength:"64"`
	Limit  int32                  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string                 `query:"cursor" maxLength:"200"`
}

type assignmentPageOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentAssignmentPage
}

type submissionInput struct {
	ID string `path:"id"`
}

type saveAnswersInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.StudentAnswersSave
}

type submitInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.StudentSubmit
}

type studentSubmissionOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentSubmission
}

type submissionDetailOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.SubmissionDetail
}

func (r *router) registerSubmissions() {
	list := operation("assignments-list", http.MethodGet, assignmentsPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, list, AudienceStudentUnverifiedOK, r.assignments)

	save := operation("submissions-answers-save", http.MethodPut, submissionsPath+"/{id}/answers")
	save.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge,
		http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, save, AudienceStudentUnverifiedOK, r.saveAnswers)

	submit := operation("submissions-submit", http.MethodPost, submissionsPath+"/{id}/submit")
	submit.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, submit, AudienceStudentUnverifiedOK, r.submit)

	get := operation("submissions-get", http.MethodGet, submissionsPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, get, AudienceUnverifiedOK, r.submission)
}

func (r *router) assignments(ctx context.Context, input *assignmentsInput) (*assignmentPageOutput, error) {
	query := homework.AssignmentQuery{Group: input.Status, Cursor: input.Cursor, Limit: input.Limit}
	if input.Tutor != "" {
		tutor, valid := domain.ParseUUID[domain.RelationshipID](input.Tutor)
		if !valid {
			return nil, NewError(ctx, domain.ErrorNotFound)
		}
		query.Tutor = &tutor
	}
	page, err := r.homework.Assignments(ctx, principalFrom(ctx).UserID, query)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.assignments.list.failed", err)
	}
	body := schemas.StudentAssignmentPage{
		Items: assignmentItemsBody(page.Items), NextCursor: page.NextCursor,
		Counts: schemas.StudentAssignmentCounts{Todo: page.Counts.Todo, Closed: page.Counts.Closed},
	}
	return &assignmentPageOutput{CacheControl: noStore, Body: body}, nil
}

func assignmentItemsBody(items []homework.AssignmentItem) []schemas.StudentAssignmentItem {
	body := make([]schemas.StudentAssignmentItem, 0, len(items))
	for _, item := range items {
		body = append(body, schemas.StudentAssignmentItem{
			SubmissionID: item.Submission.String(), Title: item.Title, Tutor: assignmentTutorBody(item.Tutor), Status: item.Status,
			Late: item.Late, IssuedBeforeReclaim: item.IssuedBeforeReclaim, DeadlineAt: utcInstant(item.Deadline),
			IssuedAt: item.IssuedAt.UTC(), SubmittedAt: utcInstant(item.SubmittedAt), CheckedAt: utcInstant(item.CheckedAt),
			FeedbackExcerpt: item.FeedbackExcerpt, HasCorrectedFiles: item.HasCorrectedFiles,
		})
	}
	return body
}

func (r *router) submission(ctx context.Context, input *submissionInput) (*submissionDetailOutput, error) {
	principal := principalFrom(ctx)
	if principal.Role == domain.RoleTutor && principal.State != domain.AccountActive {
		return nil, NewError(ctx, domain.ErrorEmailNotVerified)
	}
	id, valid := domain.ParseUUID[domain.SubmissionID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if principal.Role == domain.RoleTutor {
		work, err := r.homework.TutorSubmission(ctx, principal.UserID, id)
		if err != nil {
			return nil, r.homeworkError(ctx, "homework.submissions.get.failed", err)
		}
		body := tutorSubmissionBody(work)
		return &submissionDetailOutput{CacheControl: noStore, Body: schemas.SubmissionDetail{Tutor: &body}}, nil
	}
	work, err := r.homework.StudentSubmission(ctx, principal.UserID, id)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.submissions.get.failed", err)
	}
	body := studentSubmissionBody(work)
	return &submissionDetailOutput{CacheControl: noStore, Body: schemas.SubmissionDetail{Student: &body}}, nil
}

func (r *router) saveAnswers(ctx context.Context, input *saveAnswersInput) (*studentSubmissionOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.SubmissionID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	saveID, valid := domain.ParseUUID[domain.EntityID](input.Body.SaveID)
	if !valid {
		return nil, r.internal(ctx, "homework.submissions.save.failed", errSaveIDUnparsed)
	}
	save := homework.AnswersSave{Version: input.Body.Version, SaveID: saveID, Answers: make([]homework.AnswerInput, 0, len(input.Body.Answers))}
	for _, answer := range input.Body.Answers {
		save.Answers = append(save.Answers, homework.AnswerInput{
			Question: answer.QuestionID,
			Value:    domain.AnswerValue{OptionIDs: answer.Value.OptionIDs, Text: answer.Value.Text, AttachmentIDs: answer.Value.AttachmentIDs},
		})
	}
	work, err := r.homework.SaveAnswers(ctx, principalFrom(ctx), id, save)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.submissions.save.failed", err)
	}
	return &studentSubmissionOutput{CacheControl: noStore, Body: studentSubmissionBody(work)}, nil
}

func (r *router) submit(ctx context.Context, input *submitInput) (*studentSubmissionOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.SubmissionID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	work, err := r.homework.Submit(ctx, principalFrom(ctx), id, input.Body.Version)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.submissions.submit.failed", err)
	}
	return &studentSubmissionOutput{CacheControl: noStore, Body: studentSubmissionBody(work)}, nil
}

func assignmentTutorBody(tutor homework.TutorRef) schemas.StudentAssignmentTutor {
	return schemas.StudentAssignmentTutor{RelationshipID: tutor.Relationship.String(), Name: tutor.Name, Colour: tutor.Colour}
}

func studentSubmissionBody(work homework.StudentWork) schemas.StudentSubmission {
	blocks := studentTemplateBody(library.StudentTemplate{Title: work.Title, Blocks: work.Blocks}).Blocks
	body := schemas.StudentSubmission{
		ID: work.ID.String(), Title: work.Title, Tutor: assignmentTutorBody(work.Tutor), Status: work.Status, Late: work.Late,
		IssuedBeforeReclaim: work.IssuedBeforeReclaim, DeadlineAt: utcInstant(work.Deadline), Version: work.Version, Blocks: blocks,
		Answers: make([]schemas.StudentAnswer, 0, len(work.Answers)), EditableQuestionIDs: work.Editable,
	}
	for _, answer := range work.Answers {
		body.Answers = append(body.Answers, schemas.StudentAnswer{
			QuestionID: answer.Question, Value: answerValueBody(answer.Value), Files: submissionFilesBody(answer.Files),
		})
	}
	if work.Review != nil {
		review := schemas.StudentReview{Feedback: work.Review.Feedback, Items: make([]schemas.StudentReviewItem, 0, len(work.Review.Items))}
		for _, item := range work.Review.Items {
			review.Items = append(review.Items, schemas.StudentReviewItem{
				QuestionID: item.Question, Comment: item.Comment, CorrectedFiles: submissionFilesBody(item.CorrectedFiles),
			})
		}
		body.Review = &review
	}
	if work.Revealed != nil {
		revealed := schemas.StudentRevealed{Questions: make([]schemas.StudentRevealedQuestion, 0, len(*work.Revealed))}
		for _, question := range *work.Revealed {
			revealed.Questions = append(revealed.Questions, schemas.StudentRevealedQuestion{
				QuestionID: question.Question, CorrectOptionIDs: question.CorrectOptionIDs, AcceptedAnswers: question.AcceptedAnswers,
				CorrectValue: question.CorrectValue, Tolerance: question.Tolerance, Explanation: question.Explanation,
			})
		}
		body.Revealed = &revealed
	}
	return body
}

func tutorSubmissionBody(work homework.TutorWork) schemas.TutorSubmission {
	body := schemas.TutorSubmission{
		ID: work.ID.String(), IssueID: work.Issue.String(), Title: work.Title,
		Student: schemas.TutorSubmissionStudent{RelationshipID: work.Relationship.String(), DisplayName: work.DisplayName},
		Status:  work.Status, Late: work.Late, IssuedBeforeReclaim: work.IssuedBeforeReclaim, DeadlineAt: utcInstant(work.Deadline),
		FirstSubmittedAt: utcInstant(work.FirstSubmittedAt), SubmittedAt: utcInstant(work.SubmittedAt), CheckedAt: utcInstant(work.CheckedAt),
		Version: work.Version, AnswerPolicy: work.Policy, Blocks: make([]schemas.IssueBlock, 0, len(work.Blocks)),
		Answers: make([]schemas.TutorAnswer, 0, len(work.Answers)), Revisions: make([]schemas.TutorReviewRevision, 0, len(work.Revisions)),
		AutoPoints: work.AutoPoints, FinalPoints: work.FinalPoints, MaxPoints: work.MaxPoints, Percentage: work.Percentage,
	}
	for _, block := range work.Blocks {
		body.Blocks = append(body.Blocks, issueBlockBody(block))
	}
	for _, answer := range work.Answers {
		body.Answers = append(body.Answers, schemas.TutorAnswer{
			QuestionID: answer.Question, Value: answerValueBody(answer.Value), Normalised: answer.Normalised,
			NormaliserVersion: answer.NormaliserVersion, AutoPoints: answer.AutoPoints, AutoCorrect: answer.AutoCorrect,
			FrozenAt: utcInstant(answer.FrozenAt), Files: submissionFilesBody(answer.Files),
		})
	}
	for _, revision := range work.Revisions {
		var action *string
		if revision.Action != nil {
			text := string(*revision.Action)
			action = &text
		}
		view := schemas.TutorReviewRevision{
			Revision: revision.Revision, State: string(revision.State), Action: action, Feedback: revision.Feedback,
			Items: make([]schemas.TutorReviewItem, 0, len(revision.Items)), Version: revision.Version, PublishedAt: utcInstant(revision.PublishedAt),
		}
		for _, item := range revision.Items {
			view.Items = append(view.Items, schemas.TutorReviewItem{
				QuestionID: item.Question, Points: item.Points, OverridePoints: item.OverridePoints, OverrideReason: item.OverrideReason,
				Comment: item.Comment, CorrectedFiles: submissionFilesBody(item.CorrectedFiles),
			})
		}
		body.Revisions = append(body.Revisions, view)
	}
	return body
}

func answerValueBody(value domain.AnswerValue) schemas.StudentAnswerValue {
	return schemas.StudentAnswerValue{OptionIDs: value.OptionIDs, Text: value.Text, AttachmentIDs: value.AttachmentIDs}
}

func submissionFilesBody(files []homework.AnswerFileView) []schemas.SubmissionFile {
	body := make([]schemas.SubmissionFile, 0, len(files))
	for _, file := range files {
		body = append(body, schemas.SubmissionFile{
			AttachmentID: file.Attachment.String(), Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes,
			Width: file.Width, Height: file.Height, URL: file.URL, ExpiresAt: file.ExpiresAt.UTC(),
		})
	}
	return body
}

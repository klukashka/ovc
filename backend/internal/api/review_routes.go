package api

import (
	"context"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/homework"
)

const reviewQueuePath = "/api/v1/review-queue"

type reviewQueueInput struct {
	Issue   string `query:"issue" maxLength:"64"`
	Group   string `query:"group" maxLength:"64"`
	Student string `query:"student" maxLength:"64"`
	Limit   int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor  string `query:"cursor" maxLength:"200"`
}

type reviewQueueOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ReviewQueuePage
}

type saveReviewInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.ReviewSave
}

type publishReviewInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.ReviewPublish
}

type tutorSubmissionOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.TutorSubmission
}

func (r *router) registerReviews() {
	queue := operation("review-queue-list", http.MethodGet, reviewQueuePath)
	queue.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, queue, AudienceTutor, r.reviewQueue)

	save := operation("submissions-review-save", http.MethodPut, submissionsPath+"/{id}/review")
	save.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
		http.StatusRequestEntityTooLarge, http.StatusTooManyRequests, http.StatusInsufficientStorage)
	register(r, save, AudienceTutor, r.saveReview)

	check := operation("submissions-check", http.MethodPost, submissionsPath+"/{id}/check")
	check.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, check, AudienceTutor, r.checkSubmission)

	giveBack := operation("submissions-return", http.MethodPost, submissionsPath+"/{id}/return")
	giveBack.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, giveBack, AudienceTutor, r.returnSubmission)
}

func (r *router) reviewQueue(ctx context.Context, input *reviewQueueInput) (*reviewQueueOutput, error) {
	issue, issueValid := queueFilter[domain.IssueID](input.Issue)
	group, groupValid := queueFilter[domain.GroupID](input.Group)
	student, studentValid := queueFilter[domain.RelationshipID](input.Student)
	if !issueValid || !groupValid || !studentValid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	query := homework.ReviewQueueQuery{Issue: issue, Group: group, Student: student, Cursor: input.Cursor, Limit: input.Limit}
	page, err := r.homework.ReviewQueue(ctx, principalFrom(ctx).UserID, query)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.review_queue.list.failed", err)
	}
	body := schemas.ReviewQueuePage{Items: make([]schemas.ReviewQueueItem, 0, len(page.Items)), NextCursor: page.NextCursor, Count: page.Count}
	for _, item := range page.Items {
		body.Items = append(body.Items, reviewQueueItemBody(item))
	}
	return &reviewQueueOutput{CacheControl: noStore, Body: body}, nil
}

func reviewQueueItemBody(item homework.ReviewQueueItem) schemas.ReviewQueueItem {
	return schemas.ReviewQueueItem{
		SubmissionID: item.Submission.String(), IssueID: item.Issue.String(), Title: item.Title,
		Student:     schemas.TutorSubmissionStudent{RelationshipID: item.Relationship.String(), DisplayName: item.DisplayName},
		SubmittedAt: utcInstant(item.SubmittedAt), FirstSubmittedAt: utcInstant(item.FirstSubmittedAt), Late: item.Late,
		HasDraft: item.HasDraft, IssuedBeforeReclaim: item.IssuedBeforeReclaim,
	}
}

func (r *router) saveReview(ctx context.Context, input *saveReviewInput) (*tutorSubmissionOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.SubmissionID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	saveID, valid := domain.ParseUUID[domain.EntityID](input.Body.SaveID)
	if !valid {
		return nil, r.internal(ctx, "homework.reviews.save.failed", errSaveIDUnparsed)
	}
	save := homework.ReviewSave{
		Revision: input.Body.Revision, Version: input.Body.Version, SaveID: saveID, Feedback: input.Body.Feedback,
		Items: make([]homework.ReviewItemInput, 0, len(input.Body.Items)),
	}
	for _, item := range input.Body.Items {
		save.Items = append(save.Items, homework.ReviewItemInput{
			Question: item.QuestionID, Points: item.Points, OverridePoints: item.OverridePoints, OverrideReason: item.OverrideReason,
			Comment: item.Comment, Corrected: item.CorrectedAttachmentIDs,
		})
	}
	work, err := r.homework.SaveReview(ctx, tutorActorFrom(ctx), id, save)
	if err != nil {
		return nil, r.homeworkError(ctx, "homework.reviews.save.failed", err)
	}
	return &tutorSubmissionOutput{CacheControl: noStore, Body: tutorSubmissionBody(work)}, nil
}

func (r *router) checkSubmission(ctx context.Context, input *publishReviewInput) (*tutorSubmissionOutput, error) {
	return r.publishReview(ctx, input, domain.ReviewCheck, "homework.reviews.check.failed")
}

func (r *router) returnSubmission(ctx context.Context, input *publishReviewInput) (*tutorSubmissionOutput, error) {
	return r.publishReview(ctx, input, domain.ReviewReturn, "homework.reviews.return.failed")
}

func (r *router) publishReview(
	ctx context.Context, input *publishReviewInput, action domain.ReviewAction, event string,
) (*tutorSubmissionOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.SubmissionID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	work, err := r.homework.Publish(ctx, tutorActorFrom(ctx), id, action, input.Body.Revision)
	if err != nil {
		return nil, r.homeworkError(ctx, event, err)
	}
	return &tutorSubmissionOutput{CacheControl: noStore, Body: tutorSubmissionBody(work)}, nil
}

func queueFilter[ID ~[16]byte](text string) (*ID, bool) {
	if text == "" {
		return nil, true
	}
	id, valid := domain.ParseUUID[ID](text)
	return &id, valid
}

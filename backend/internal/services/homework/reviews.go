package homework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

const (
	limitReviewSaveSecond = "review_saves_second"
	limitReviewSaveHour   = "review_saves_hour"
	recordRevision        = "revision"
	fieldFeedback         = "body.feedback"
	fieldItems            = "body.items"
	fieldRevision         = "body.revision"
	newDraftVersion       = 0
)

var errStoredReviewInvalid = errors.New("a stored review revision cannot be decoded")

type ReviewItemInput struct {
	Question       string
	Points         *int32
	OverridePoints *int32
	OverrideReason *string
	Comment        string
	Corrected      []string
}

type ReviewSave struct {
	Revision int32
	Version  int64
	SaveID   domain.EntityID
	Feedback string
	Items    []ReviewItemInput
}

type tutorLocks struct {
	tutor        domain.UserID
	parties      repositories.TutorSubmissionParties
	relationship domain.RelationshipState
	issue        repositories.LockedSubmissionIssue
	submission   repositories.LockedSubmission
	blocks       []domain.IssueBlock
	now          time.Time
}

type correctedPlan struct {
	bound    []repositories.CorrectedFile
	released []domain.AttachmentID
}

type draftState struct {
	exists    bool
	locked    repositories.LockedReviewDraft
	published *repositories.PublishedRevision
}

func (s *Service) reserveReviewSlots(ctx context.Context, actor domain.TutorActor, id domain.SubmissionID) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		tutor := actor.UserID
		second, secondRetry := domain.FixedWindow(now, domain.ReviewSaveSecondWindow)
		perSubmission, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
			Purpose:     domain.PurposeReviewSavesSecond,
			Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeReviewSavesSecond), tutor.String()+":"+id.String()),
			WindowStart: second,
		})
		if err != nil {
			return err
		}
		hour, hourRetry := domain.FixedWindow(now, domain.ReviewSaveHourWindow)
		perTutor, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
			Purpose:     domain.PurposeReviewSavesHour,
			Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeReviewSavesHour), tutor.String()),
			WindowStart: hour,
		})
		if err != nil {
			return err
		}
		retryAfter, limit := time.Duration(0), ""
		if perSubmission > domain.ReviewSavesPerSecond {
			retryAfter, limit = secondRetry, limitReviewSaveSecond
		}
		if perTutor > domain.ReviewSavesPerHour {
			retryAfter, limit = max(retryAfter, hourRetry), limitReviewSaveHour
		}
		if retryAfter == 0 {
			return nil
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "homework.rate_limited", tutorAttribute(tutor),
			slog.String(recordSubmission, id.String()), slog.String("limit", limit))
		return &ThrottledError{RetryAfter: retryAfter}
	})
}

func (s *Service) lockTutorWork(ctx context.Context, repos repositories.Repositories, actor domain.TutorActor, id domain.SubmissionID) (tutorLocks, error) {
	now, err := s.lockTutor(ctx, repos, actor)
	if err != nil {
		return tutorLocks{}, err
	}
	tutor := actor.UserID
	parties, err := repos.Homework.TutorSubmissionParties(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorLocks{}, ErrNotFound
	}
	if err != nil {
		return tutorLocks{}, err
	}
	relationship, err := repos.Homework.LockSubmissionRelationship(ctx, tutor, parties.RelationshipID)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorLocks{}, ErrNotFound
	}
	if err != nil {
		return tutorLocks{}, err
	}
	issue, err := repos.Homework.LockSubmissionIssue(ctx, tutor, parties.IssueID)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorLocks{}, ErrNotFound
	}
	if err != nil {
		return tutorLocks{}, err
	}
	if issue.MarkedForErasure {
		return tutorLocks{}, ErrNotFound
	}
	submission, err := repos.Homework.LockSubmission(ctx, parties.IssueID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return tutorLocks{}, ErrNotFound
	}
	if err != nil {
		return tutorLocks{}, err
	}
	blocks, err := decodeSnapshot(issue.Snapshot)
	if err != nil {
		return tutorLocks{}, err
	}
	return tutorLocks{tutor: tutor, parties: parties, relationship: relationship, issue: issue, submission: submission, blocks: blocks, now: now}, nil
}

func (s *Service) draftState(ctx context.Context, repos repositories.Repositories, id domain.SubmissionID) (draftState, error) {
	var state draftState
	locked, err := repos.Homework.LockReviewDraft(ctx, id)
	switch {
	case err == nil:
		state.exists, state.locked = true, locked
	case !errors.Is(err, repositories.ErrNotFound):
		return draftState{}, err
	}
	published, err := repos.Homework.LatestPublishedRevision(ctx, id)
	switch {
	case err == nil:
		state.published = &published
	case !errors.Is(err, repositories.ErrNotFound):
		return draftState{}, err
	}
	return state, nil
}

func (d draftState) publishedNumber() int32 {
	if d.published == nil {
		return domain.AutomaticReviewRevision
	}
	return d.published.Revision
}

func (s *Service) SaveReview(ctx context.Context, actor domain.TutorActor, id domain.SubmissionID, save ReviewSave) (TutorWork, error) {
	if err := s.reserveReviewSlots(ctx, actor, id); err != nil {
		return TutorWork{}, err
	}
	var work TutorWork
	var plan correctedPlan
	replayed := false
	revision := save.Revision
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := s.lockTutorWork(ctx, repos, actor, id)
		if err != nil {
			return err
		}
		state, err := s.draftState(ctx, repos, id)
		if err != nil {
			return err
		}
		if state.exists && state.locked.LastSaveID.Valid && domain.EntityID(state.locked.LastSaveID.Bytes) == save.SaveID {
			replayed = true
			work, err = s.tutorWork(ctx, repos, actor.UserID, id, locked.now)
			return err
		}
		if !domain.ReviewableSubmission(locked.issue.Status, locked.issue.MarkedForErasure, locked.submission.Status) {
			return ErrNotEditable
		}
		if !draftMatches(state, save) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "homework.save_conflict", tutorAttribute(actor.UserID), slog.String(recordSubmission, id.String()),
				slog.String("kind", "review"))
			return ErrConflict
		}
		items, err := checkReview(save, locked.blocks)
		if err != nil {
			return err
		}
		held, err := s.heldCorrected(ctx, repos, state)
		if err != nil {
			return err
		}
		plan, err = s.planCorrected(ctx, repos, actor.UserID, id, items, held, locked.now)
		if err != nil {
			return err
		}
		draft, err := s.writeDraft(ctx, repos, actor.UserID, id, state, save, items, locked.now)
		if err != nil {
			return err
		}
		if err := s.applyCorrected(ctx, repos, actor.UserID, draft, plan, locked.now); err != nil {
			return err
		}
		work, err = s.tutorWork(ctx, repos, actor.UserID, id, locked.now)
		return err
	})
	if errors.Is(err, ErrQuotaExceeded) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", slog.String(recordSubmission, id.String()),
			slog.String("context", string(domain.ContextCorrected)))
	}
	if err != nil || replayed {
		return work, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "homework.review.saved", tutorAttribute(actor.UserID), slog.String(recordSubmission, id.String()),
		slog.Int(recordRevision, int(revision)), slog.Int("files_bound", len(plan.bound)), slog.Int("files_released", len(plan.released)))
	return work, nil
}

func draftMatches(state draftState, save ReviewSave) bool {
	if state.exists {
		return save.Revision == state.locked.Revision && save.Version == state.locked.Version
	}
	return save.Revision == state.publishedNumber()+1 && save.Version == newDraftVersion
}

func checkReview(save ReviewSave, blocks []domain.IssueBlock) ([]domain.ReviewItem, error) {
	if problem := reviewText(save.Feedback, domain.ReviewFeedbackMaxRunes, fieldFeedback); problem != nil {
		return nil, problem
	}
	questions := map[string]domain.IssueBlock{}
	for _, block := range blocks {
		if block.Type.Question() && block.Points != nil {
			questions[block.ID] = block
		}
	}
	items := make([]domain.ReviewItem, 0, len(save.Items))
	var seenQuestions, seenFiles []string
	for index, input := range save.Items {
		path := fieldItems + "[" + strconv.Itoa(index) + "]"
		block, found := questions[input.Question]
		if _, valid := domain.ParseUUID[domain.QuestionID](input.Question); !found || !valid || slices.Contains(seenQuestions, input.Question) {
			return nil, invalid(path+".question_id", domain.FieldInvalid)
		}
		seenQuestions = append(seenQuestions, input.Question)
		if problem := checkReviewItem(input, block, path); problem != nil {
			return nil, problem
		}
		corrected := make([]string, 0, len(input.Corrected))
		for _, text := range input.Corrected {
			attachment, valid := domain.ParseUUID[domain.AttachmentID](text)
			if !valid || slices.Contains(seenFiles, attachment.String()) {
				return nil, invalid(path+".corrected_attachment_ids", domain.FieldInvalid)
			}
			seenFiles = append(seenFiles, attachment.String())
			corrected = append(corrected, attachment.String())
		}
		items = append(items, domain.ReviewItem{
			QuestionID: input.Question, Points: input.Points, OverridePoints: input.OverridePoints, OverrideReason: input.OverrideReason,
			Comment: input.Comment, CorrectedAttachmentIDs: corrected,
		})
	}
	return items, nil
}

func checkReviewItem(input ReviewItemInput, block domain.IssueBlock, path string) *ValidationError {
	maximum := *block.Points
	if input.Points != nil {
		if !block.Type.Manual() {
			return invalid(path+".points", domain.FieldNotAllowed)
		}
		if *input.Points < 0 || *input.Points > maximum {
			return invalid(path+".points", domain.FieldInvalid)
		}
	}
	if input.OverridePoints != nil || input.OverrideReason != nil {
		if !block.Type.Automatic() {
			field := ".override_points"
			if input.OverridePoints == nil {
				field = ".override_reason"
			}
			return invalid(path+field, domain.FieldNotAllowed)
		}
		if input.OverridePoints == nil {
			return invalid(path+".override_reason", domain.FieldNotAllowed)
		}
		if *input.OverridePoints < 0 || *input.OverridePoints > maximum {
			return invalid(path+".override_points", domain.FieldInvalid)
		}
		if input.OverrideReason == nil || *input.OverrideReason == "" {
			return invalid(path+".override_reason", domain.FieldRequired)
		}
		if problem := reviewText(*input.OverrideReason, domain.OverrideReasonMaxRunes, path+".override_reason"); problem != nil {
			return problem
		}
	}
	if problem := reviewText(input.Comment, domain.ReviewCommentMaxRunes, path+".comment"); problem != nil {
		return problem
	}
	if len(input.Corrected) > 0 && block.Type != domain.BlockFile {
		return invalid(path+".corrected_attachment_ids", domain.FieldNotAllowed)
	}
	if len(input.Corrected) > domain.CorrectedFilesPerAnswer {
		return invalid(path+".corrected_attachment_ids", domain.FieldTooMany)
	}
	return nil
}

func reviewText(text string, limit int, path string) *ValidationError {
	if !utf8.ValidString(text) || containsNull(text) {
		return invalid(path, domain.FieldInvalid)
	}
	if utf8.RuneCountInString(text) > limit {
		return invalid(path, domain.FieldTooLong)
	}
	return nil
}

func (s *Service) heldCorrected(ctx context.Context, repos repositories.Repositories, state draftState) ([]repositories.ReviewItemReference, error) {
	switch {
	case state.exists:
		return repos.Homework.ReviewItemReferences(ctx, state.locked.ID)
	case state.published != nil:
		return repos.Homework.ReviewItemReferences(ctx, state.published.ID)
	}
	return nil, nil
}

func (s *Service) planCorrected(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.SubmissionID, items []domain.ReviewItem,
	held []repositories.ReviewItemReference, now time.Time,
) (correctedPlan, error) {
	var plan correctedPlan
	var fresh []domain.AttachmentID
	wantedBy := map[domain.AttachmentID]domain.QuestionID{}
	isHeld := func(attachment domain.AttachmentID, question domain.QuestionID) bool {
		return slices.ContainsFunc(held, func(reference repositories.ReviewItemReference) bool {
			return reference.AttachmentID == attachment && reference.QuestionID != nil && domain.QuestionID(*reference.QuestionID) == question
		})
	}
	var kept []domain.AttachmentID
	for _, item := range items {
		question, _ := domain.ParseUUID[domain.QuestionID](item.QuestionID)
		for _, text := range item.CorrectedAttachmentIDs {
			attachment, _ := domain.ParseUUID[domain.AttachmentID](text)
			if isHeld(attachment, question) {
				kept = append(kept, attachment)
				continue
			}
			fresh = append(fresh, attachment)
			wantedBy[attachment] = question
		}
	}
	for _, reference := range held {
		if !slices.Contains(kept, reference.AttachmentID) && !slices.Contains(plan.released, reference.AttachmentID) {
			plan.released = append(plan.released, reference.AttachmentID)
		}
	}
	bindable, err := repos.Homework.LockBindableCorrectedUploads(ctx, tutor, id, fresh, now)
	if err != nil {
		return correctedPlan{}, err
	}
	for _, attachment := range fresh {
		index := slices.IndexFunc(bindable, func(upload repositories.BindableCorrectedUpload) bool { return upload.ID == attachment })
		question := wantedBy[attachment]
		if index < 0 || bindable[index].ScopeQuestionID == nil || domain.QuestionID(*bindable[index].ScopeQuestionID) != question {
			return correctedPlan{}, ErrNotFound
		}
		plan.bound = append(plan.bound, repositories.CorrectedFile{Attachment: attachment, Question: question})
	}
	return plan, nil
}

func (s *Service) writeDraft(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.SubmissionID, state draftState, save ReviewSave,
	items []domain.ReviewItem, now time.Time,
) (domain.ReviewRevisionID, error) {
	encoded, err := json.Marshal(items)
	if err != nil {
		return domain.ReviewRevisionID{}, fmt.Errorf("encode review items: %w", err)
	}
	if state.exists {
		return state.locked.ID, repos.Homework.UpdateReviewDraft(ctx, repositories.ReviewDraftChange{
			ID: state.locked.ID, Feedback: save.Feedback, Items: encoded, SaveID: save.SaveID, Now: now,
		})
	}
	count, err := repos.Homework.RevisionCount(ctx, id)
	if err != nil {
		return domain.ReviewRevisionID{}, err
	}
	if count >= s.deps.Caps.RevisionsPerReview {
		return domain.ReviewRevisionID{}, invalid(fieldBody, domain.FieldTooMany)
	}
	draft, err := core.NewID[domain.ReviewRevisionID](s.deps.Random)
	if err != nil {
		return domain.ReviewRevisionID{}, err
	}
	if err := repos.Homework.InsertReviewDraft(ctx, repositories.NewReviewDraft{
		ID: draft, Submission: id, Tutor: tutor, Revision: save.Revision, Feedback: save.Feedback, Items: encoded, SaveID: save.SaveID,
		Now: now,
	}); err != nil {
		return domain.ReviewRevisionID{}, err
	}
	if state.published != nil {
		if err := repos.Homework.CopyReviewItemReferences(ctx, state.published.ID, draft); err != nil {
			return domain.ReviewRevisionID{}, err
		}
	}
	return draft, nil
}

func (s *Service) applyCorrected(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, draft domain.ReviewRevisionID, plan correctedPlan, now time.Time,
) error {
	removed, err := repos.Homework.RemoveReviewItemReferences(ctx, draft, plan.released)
	if err != nil {
		return err
	}
	if err := library.ReleaseReferences(ctx, repos, s.deps.Random, s.deps.Logger, domain.ReferenceReviewItem, distinctAttachments(removed), now); err != nil {
		return err
	}
	if len(plan.bound) == 0 {
		return nil
	}
	bound := make([]domain.AttachmentID, 0, len(plan.bound))
	for _, file := range plan.bound {
		bound = append(bound, file.Attachment)
	}
	if err := repos.Library.BindUploads(ctx, bound, now); err != nil {
		return err
	}
	if err := repos.Homework.AddReviewItemReferences(ctx, draft, plan.bound, now); err != nil {
		return err
	}
	charged, err := repos.Library.ChargeAll(ctx, tutor, bound, now)
	if err != nil {
		return err
	}
	if !charged {
		return ErrQuotaExceeded
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, attachment := range bound {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", slog.String(recordAttached, attachment.String()),
				slog.String(recordRefKind, string(domain.ReferenceReviewItem)))
		}
	})
	return nil
}

func decodeReviewItems(encoded []byte) ([]domain.ReviewItem, error) {
	var items []domain.ReviewItem
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil, fmt.Errorf("%w: %w", errStoredReviewInvalid, err)
	}
	return items, nil
}

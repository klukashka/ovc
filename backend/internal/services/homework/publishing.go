package homework

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type reviewEvent struct {
	notification domain.NotificationEvent
	outbox       domain.OutboxKind
	idempotency  func(domain.SubmissionID, int32) string
}

var (
	checkedEvent  = reviewEvent{domain.NotificationSubmissionChecked, domain.EventSubmissionChecked, domain.SubmissionCheckedKey}
	returnedEvent = reviewEvent{domain.NotificationSubmissionReturned, domain.EventSubmissionReturned, domain.SubmissionReturnedKey}
	updatedEvent  = reviewEvent{domain.NotificationReviewUpdated, domain.EventReviewUpdated, domain.ReviewUpdatedKey}
)

type publishOutcome struct {
	changed bool
	visible bool
}

func (s *Service) Publish(
	ctx context.Context, actor domain.TutorActor, id domain.SubmissionID, action domain.ReviewAction, revision int32,
) (TutorWork, error) {
	if err := s.reserveSlot(ctx, actor, s.writeSlot()); err != nil {
		return TutorWork{}, err
	}
	var work TutorWork
	var outcome publishOutcome
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := s.lockTutorWork(ctx, repos, actor, id)
		if err != nil {
			return err
		}
		state, err := s.draftState(ctx, repos, id)
		if err != nil {
			return err
		}
		if state.published != nil && state.published.Revision == revision {
			work, err = s.tutorWork(ctx, repos, actor.UserID, id, locked.now)
			return err
		}
		status := locked.submission.Status
		if !domain.ReviewableSubmission(locked.issue.Status, locked.issue.MarkedForErasure, status) ||
			(action == domain.ReviewReturn && status != domain.SubmissionSubmitted) {
			return ErrNotEditable
		}
		if !state.exists || state.locked.Revision != revision {
			return ErrConflict
		}
		if action == domain.ReviewReturn &&
			!domain.ReturnAllowed(status, locked.issue.AnswerPolicy, locked.issue.HasManualQuestions, locked.relationship) {
			return ErrReturnNotAllowed
		}
		items, err := decodeReviewItems(state.locked.Items)
		if err != nil {
			return err
		}
		draft := domain.VisibleReview{Feedback: state.locked.Feedback, Items: items}
		if !publishable(action, draft, locked.blocks) {
			return invalid(fieldRevision, domain.FieldRequired)
		}
		outcome, err = s.publishLocked(ctx, repos, locked, state, action, draft)
		if err != nil {
			return err
		}
		work, err = s.tutorWork(ctx, repos, actor.UserID, id, locked.now)
		return err
	})
	if err != nil {
		return TutorWork{}, err
	}
	if outcome.changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "homework.review.published", tutorAttribute(actor.UserID), slog.String(recordSubmission, id.String()),
			slog.Int(recordRevision, int(revision)), slog.String("action", string(action)), slog.Bool("visible_changed", outcome.visible))
	}
	return work, nil
}

func publishable(action domain.ReviewAction, draft domain.VisibleReview, blocks []domain.IssueBlock) bool {
	if action == domain.ReviewReturn {
		return draft.Feedback != "" || slicesContainComment(draft.Items)
	}
	scored := map[string]bool{}
	for _, item := range draft.Items {
		if item.Points != nil {
			scored[item.QuestionID] = true
		}
	}
	for _, block := range blocks {
		if block.Type.Manual() && block.Points != nil && !scored[block.ID] {
			return false
		}
	}
	return true
}

func slicesContainComment(items []domain.ReviewItem) bool {
	for _, item := range items {
		if item.Comment != "" {
			return true
		}
	}
	return false
}

func (s *Service) publishLocked(
	ctx context.Context, repos repositories.Repositories, locked tutorLocks, state draftState, action domain.ReviewAction,
	draft domain.VisibleReview,
) (publishOutcome, error) {
	id := locked.parties.ID
	finalPoints, err := s.reviewedPoints(ctx, repos, id, locked.blocks, draft.Items)
	if err != nil {
		return publishOutcome{}, err
	}
	next := domain.SubmissionChecked
	if action == domain.ReviewReturn {
		next = domain.SubmissionReturned
	}
	if !domain.SubmissionTransitionAllowed(locked.submission.Status, next) {
		return publishOutcome{}, ErrNotEditable
	}
	event := checkedEvent
	visible := true
	switch {
	case action == domain.ReviewReturn:
		event = returnedEvent
	case locked.submission.Status == domain.SubmissionChecked:
		event = updatedEvent
		previous := domain.VisibleReview{}
		if state.published != nil {
			items, err := decodeReviewItems(state.published.Items)
			if err != nil {
				return publishOutcome{}, err
			}
			previous = domain.VisibleReview{Feedback: state.published.Feedback, Items: items}
		}
		visible = domain.ReviewVisiblyChanged(previous, draft)
	}
	published, err := repos.Homework.PublishRevision(ctx, state.locked.ID, action, locked.now)
	if err != nil {
		return publishOutcome{}, err
	}
	if !published {
		return publishOutcome{}, ErrConflict
	}
	if err := repos.Homework.PublishSubmissionReview(ctx, repositories.ReviewPublication{
		Submission: id, Status: next, Checked: next == domain.SubmissionChecked, FinalPoints: finalPoints, Revision: state.locked.ID,
		Visible: visible, Now: locked.now,
	}); err != nil {
		return publishOutcome{}, err
	}
	if err := s.announcePublished(ctx, repos, locked, event, state.locked.Revision, visible); err != nil {
		return publishOutcome{}, err
	}
	return publishOutcome{changed: true, visible: visible}, nil
}

func (s *Service) reviewedPoints(
	ctx context.Context, repos repositories.Repositories, id domain.SubmissionID, blocks []domain.IssueBlock, items []domain.ReviewItem,
) (int32, error) {
	stored, err := repos.Homework.Answers(ctx, id)
	if err != nil {
		return 0, err
	}
	automatic := map[string]int32{}
	for _, answer := range stored {
		if answer.AutoPoints != nil {
			automatic[answer.QuestionID.String()] = *answer.AutoPoints
		}
	}
	reviewed := map[string]domain.ReviewItem{}
	for _, item := range items {
		reviewed[item.QuestionID] = item
	}
	scores := make([]domain.QuestionScore, 0, len(blocks))
	for _, block := range blocks {
		if !block.Type.Question() || block.Points == nil {
			continue
		}
		item := reviewed[block.ID]
		score := domain.QuestionScore{Type: block.Type, MaxPoints: *block.Points}
		if block.Type.Automatic() {
			score.AutoPoints, score.OverridePoints = automatic[block.ID], item.OverridePoints
		} else {
			score.ManualPoints = item.Points
		}
		scores = append(scores, score)
	}
	total, err := domain.FinalPoints(scores)
	if err != nil {
		return 0, err
	}
	if total < 0 || total > math.MaxInt32 {
		return 0, fmt.Errorf("submission %s: %w", id, domain.ErrPointsInvalid)
	}
	return int32(total), nil
}

func (s *Service) announcePublished(
	ctx context.Context, repos repositories.Repositories, locked tutorLocks, event reviewEvent, revision int32, visible bool,
) error {
	id := locked.parties.ID
	tutor, student, relationship := locked.tutor, locked.parties.StudentID, locked.parties.RelationshipID
	readable := domain.ReadableStudentSubmission(locked.issue.Status, locked.issue.MarkedForErasure, locked.relationship)
	users := []domain.UserID{tutor}
	if visible && readable {
		users = append(users, student)
		payload, err := json.Marshal(domain.SubmissionCheckedPayload{Submission: hex.EncodeToString(id[:]), Revision: revision})
		if err != nil {
			return fmt.Errorf("encode the review payload: %w", err)
		}
		idempotency := event.idempotency(id, revision)
		if err := s.notify(ctx, repos, student, tutor, student, event.notification, id, relationship, idempotency, locked.now); err != nil {
			return err
		}
		if err := s.queueEmail(ctx, repos, event.outbox, payload, idempotency, locked.now); err != nil {
			return err
		}
	}
	return repos.Signals.Entities(ctx, []domain.EntitySignal{{Users: users, Entity: domain.EntitySubmission, ID: domain.EntityID(id)}})
}

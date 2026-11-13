package homework

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

var errUnknownNormaliser = errors.New("a score names a normaliser version the submission cannot store")

type submitOutcome struct {
	changed      bool
	late         bool
	autoChecked  bool
	resubmission bool
}

func (s *Service) Submit(ctx context.Context, principal accounts.Principal, id domain.SubmissionID, version int64) (StudentWork, error) {
	if err := s.reserveAnswerSlots(ctx, principal, id); err != nil {
		return StudentWork{}, err
	}
	var work StudentWork
	var outcome submitOutcome
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := s.lockStudentWork(ctx, repos, principal, id, false)
		if err != nil {
			return err
		}
		student := principal.UserID
		status := locked.submission.Status
		if status == domain.SubmissionSubmitted || status == domain.SubmissionChecked {
			work, err = s.studentWork(ctx, repos, student, id, locked.now)
			return err
		}
		if !domain.WritableStudentSubmission(status, locked.relationship) {
			return ErrNotEditable
		}
		if locked.submission.Version != version {
			s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "homework.save_conflict", slog.String(recordSubmission, id.String()),
				slog.String("kind", "answers"))
			return ErrConflict
		}
		outcome, err = s.submitLocked(ctx, repos, locked, id, student)
		if err != nil {
			return err
		}
		work, err = s.studentWork(ctx, repos, student, id, locked.now)
		return err
	})
	if err != nil {
		return StudentWork{}, err
	}
	if outcome.changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "homework.submission.submitted", slog.String(recordSubmission, id.String()),
			slog.Bool("late", outcome.late), slog.Bool("auto_checked", outcome.autoChecked), slog.Bool("resubmission", outcome.resubmission))
	}
	return work, nil
}

func (s *Service) submitLocked(
	ctx context.Context, repos repositories.Repositories, locked studentLocks, id domain.SubmissionID, student domain.UserID,
) (submitOutcome, error) {
	stored, err := repos.Homework.Answers(ctx, id)
	if err != nil {
		return submitOutcome{}, err
	}
	answers := map[domain.QuestionID]repositories.StoredAnswer{}
	for _, answer := range stored {
		answers[answer.QuestionID] = answer
	}
	reviewed, err := publishedItems(ctx, repos, id)
	if err != nil {
		return submitOutcome{}, err
	}
	scores := make([]domain.QuestionScore, 0, len(locked.blocks))
	var autoPoints int32
	for _, block := range locked.blocks {
		if !block.Type.Question() || block.Points == nil {
			continue
		}
		question, valid := domain.ParseUUID[domain.QuestionID](block.ID)
		if !valid {
			return submitOutcome{}, fmt.Errorf("issue %s: %w", locked.parties.IssueID, errStoredSnapshotInvalid)
		}
		score := domain.QuestionScore{Type: block.Type, MaxPoints: *block.Points}
		if item, found := reviewed[block.ID]; found {
			score.ManualPoints, score.OverridePoints = item.Points, item.OverridePoints
		}
		if block.Type.Automatic() {
			points, err := s.scoreAutomatic(ctx, repos, id, question, block, answers[question], locked.now)
			if err != nil {
				return submitOutcome{}, err
			}
			score.AutoPoints = points
			autoPoints += points
		}
		scores = append(scores, score)
	}
	total, err := domain.FinalPoints(scores)
	if err != nil {
		return submitOutcome{}, err
	}
	if total < 0 || total > math.MaxInt32 {
		return submitOutcome{}, fmt.Errorf("submission %s: %w", id, domain.ErrPointsInvalid)
	}
	finalPoints := int32(total)
	next := domain.SubmissionSubmitted
	if !locked.issue.HasManualQuestions {
		next = domain.SubmissionChecked
	}
	if !domain.SubmissionTransitionAllowed(locked.submission.Status, domain.SubmissionSubmitted) {
		return submitOutcome{}, ErrNotEditable
	}
	result, err := repos.Homework.Submit(ctx, repositories.SubmitChange{
		ID: id, Status: next, Checked: next == domain.SubmissionChecked, AutoPoints: autoPoints, FinalPoints: finalPoints,
		Now: locked.now,
	})
	if err != nil {
		return submitOutcome{}, err
	}
	if err := s.announceSubmitted(ctx, repos, locked, id, student, next); err != nil {
		return submitOutcome{}, err
	}
	return submitOutcome{changed: true, late: result.Late, autoChecked: next == domain.SubmissionChecked, resubmission: result.Resubmission}, nil
}

func publishedItems(ctx context.Context, repos repositories.Repositories, id domain.SubmissionID) (map[string]domain.ReviewItem, error) {
	published, err := repos.Homework.LatestPublishedRevision(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return map[string]domain.ReviewItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := decodeReviewItems(published.Items)
	if err != nil {
		return nil, err
	}
	reviewed := make(map[string]domain.ReviewItem, len(items))
	for _, item := range items {
		reviewed[item.QuestionID] = item
	}
	return reviewed, nil
}

func (s *Service) scoreAutomatic(
	ctx context.Context, repos repositories.Repositories, id domain.SubmissionID, question domain.QuestionID, block domain.IssueBlock,
	stored repositories.StoredAnswer, now time.Time,
) (int32, error) {
	if stored.FrozenAt != nil && stored.AutoPoints != nil {
		return *stored.AutoPoints, nil
	}
	value := emptyValue(block.Type)
	if stored.Value != nil {
		if err := json.Unmarshal(stored.Value, &value); err != nil {
			return 0, fmt.Errorf("%w: %w", errStoredAnswerInvalid, err)
		}
	}
	score, err := domain.ScoreAnswerV1(block, value)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("encode an answer: %w", err)
	}
	var version *int32
	if score.Version != nil {
		if *score.Version != domain.NormalisationVersion1 {
			return 0, fmt.Errorf("submission %s: %w", id, errUnknownNormaliser)
		}
		number := int32(domain.NormalisationVersion1)
		version = &number
	}
	if _, err := repos.Homework.FreezeAnswer(ctx, repositories.FrozenAnswer{
		Submission: id, Question: question, Value: encoded, Normalised: score.Normalised, Version: version, Points: score.Points,
		Correct: score.Correct, Now: now,
	}); err != nil {
		return 0, err
	}
	return score.Points, nil
}

func emptyValue(question domain.BlockType) domain.AnswerValue {
	switch question {
	case domain.BlockSingleChoice, domain.BlockMultipleChoice:
		return domain.AnswerValue{OptionIDs: &[]string{}}
	case domain.BlockFile:
		return domain.AnswerValue{AttachmentIDs: &[]string{}}
	case domain.BlockShortText, domain.BlockShortNumber, domain.BlockLongText, domain.BlockInstruction, domain.BlockMaterial:
	}
	empty := ""
	return domain.AnswerValue{Text: &empty}
}

func (s *Service) announceSubmitted(
	ctx context.Context, repos repositories.Repositories, locked studentLocks, id domain.SubmissionID, student domain.UserID,
	next domain.SubmissionStatus,
) error {
	tutor := locked.parties.TutorID
	relationship := locked.parties.RelationshipID
	if next == domain.SubmissionChecked {
		payload, err := json.Marshal(domain.SubmissionCheckedPayload{Submission: hex.EncodeToString(id[:]), Revision: domain.AutomaticReviewRevision})
		if err != nil {
			return fmt.Errorf("encode the submission checked payload: %w", err)
		}
		key := domain.SubmissionCheckedKey(id, domain.AutomaticReviewRevision)
		if err := s.notify(ctx, repos, student, tutor, student, domain.NotificationSubmissionChecked, id, relationship, key, locked.now); err != nil {
			return err
		}
		if err := s.queueEmail(ctx, repos, domain.EventSubmissionChecked, payload, key, locked.now); err != nil {
			return err
		}
	} else {
		submitted := locked.now
		payload, err := json.Marshal(domain.SubmittedPayload{Submission: hex.EncodeToString(id[:]), SubmittedAtMicros: submitted.UnixMicro()})
		if err != nil {
			return fmt.Errorf("encode the submission submitted payload: %w", err)
		}
		key := domain.SubmittedKey(id, submitted)
		if err := s.notify(ctx, repos, tutor, tutor, student, domain.NotificationSubmitted, id, relationship, key, locked.now); err != nil {
			return err
		}
		if err := s.queueEmail(ctx, repos, domain.EventSubmissionSubmitted, payload, key, locked.now); err != nil {
			return err
		}
	}
	return s.signal(ctx, repos, tutor, locked.parties.IssueID, []domain.SubmissionID{id},
		[]recipient{{submission: id, relationship: relationship, student: student}})
}

func (s *Service) notify(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, tutor domain.UserID, student domain.UserID,
	event domain.NotificationEvent, submission domain.SubmissionID, relationship domain.RelationshipID, key string, now time.Time,
) error {
	id, err := core.NewID[domain.NotificationID](s.deps.Random)
	if err != nil {
		return err
	}
	inserted, err := repos.Notifications.Insert(ctx, repositories.NewNotification{
		ID: id, UserID: user, TutorID: tutor, StudentID: &student, Event: event, SubjectID: domain.EntityID(submission),
		RelationshipID: &relationship, IdempotencyKey: key, CreatedAt: now,
	})
	if err != nil || !inserted {
		return err
	}
	s.logNotificationAfterCommit(ctx, repos, event, user, id)
	return repos.Signals.Notification(ctx, user, id)
}

func (s *Service) logNotificationAfterCommit(
	ctx context.Context, repos repositories.Repositories, event domain.NotificationEvent, user domain.UserID, id domain.NotificationID,
) {
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "notification.created", slog.String("event", string(event)),
			slog.String("target_user_id", user.String()), slog.String("notification_id", id.String()))
	})
}

func (s *Service) queueEmail(
	ctx context.Context, repos repositories.Repositories, kind domain.OutboxKind, payload []byte, key string, now time.Time,
) error {
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: kind, Payload: payload, IdempotencyKey: key,
		NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

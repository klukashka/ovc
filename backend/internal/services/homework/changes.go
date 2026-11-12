package homework

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

type lockedIssue struct {
	row    repositories.LockedIssue
	states []repositories.IssueSubmissionState
}

func (s *Service) lockRecipients(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.IssueID) error {
	if _, err := repos.Homework.Issue(ctx, tutor, id); errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	relationships, err := repos.Homework.SubmissionRelationships(ctx, id)
	if err != nil {
		return err
	}
	_, err = repos.Homework.LockRelationships(ctx, tutor, relationships)
	return err
}

func (s *Service) lockIssueRow(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.IssueID) (lockedIssue, error) {
	row, err := repos.Homework.LockIssue(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return lockedIssue{}, ErrNotFound
	}
	if err != nil {
		return lockedIssue{}, err
	}
	states, err := repos.Homework.SubmissionStates(ctx, id)
	if err != nil {
		return lockedIssue{}, err
	}
	return lockedIssue{row: row, states: states}, nil
}

func (s *Service) UpdateIssue(ctx context.Context, actor domain.TutorActor, id domain.IssueID, change IssueChange) (IssueItem, error) {
	tutor := actor.UserID
	if err := s.reserveSlot(ctx, actor, s.writeSlot()); err != nil {
		return IssueItem{}, err
	}
	var updated IssueItem
	var changes []domain.IssueChange
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		if err := s.lockRecipients(ctx, repos, tutor, id); err != nil {
			return err
		}
		current, err := repos.Homework.Issue(ctx, tutor, id)
		if err != nil {
			return err
		}
		if change.Lesson.Set && change.Lesson.Value != nil && !sameLesson(current.LessonID, change.Lesson.Value) {
			if err := s.lockLesson(ctx, repos, tutor, *change.Lesson.Value, current.RelationshipID, current.GroupID); err != nil {
				return err
			}
		}
		locked, err := s.lockIssueRow(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		if locked.row.Status == domain.IssueWithdrawn {
			return ErrNotEditable
		}
		if locked.row.Version != change.Version {
			return ErrConflict
		}
		deadline, lesson := locked.row.DeadlineAt, locked.row.LessonID
		if change.Deadline.Set && !sameInstant(deadline, change.Deadline.Value) {
			if change.Deadline.Value != nil && !change.Deadline.Value.After(now) {
				return invalid(fieldDeadline, domain.FieldInvalid)
			}
			deadline = change.Deadline.Value
			changes = append(changes, domain.IssueChangeDeadline)
		}
		if change.Lesson.Set && !sameLesson(lesson, change.Lesson.Value) {
			if err := s.refuseLessonFull(ctx, repos, tutor, change.Lesson.Value); err != nil {
				return err
			}
			lesson = change.Lesson.Value
			changes = append(changes, domain.IssueChangeLesson)
		}
		if len(changes) > 0 {
			if err := s.applyChange(ctx, repos, tutor, locked, deadline, lesson, changes, now); err != nil {
				return err
			}
		}
		updated, err = s.itemOf(ctx, repos, tutor, id)
		return err
	})
	if err != nil {
		return IssueItem{}, err
	}
	for _, kind := range changes {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "homework.issue.updated", tutorAttribute(tutor), issueAttribute(id),
			slog.String("change", string(kind)))
	}
	return updated, nil
}

func (s *Service) applyChange(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, locked lockedIssue, deadline *time.Time,
	lesson *domain.LessonID, changes []domain.IssueChange, now time.Time,
) error {
	if err := repos.Homework.UpdateIssue(ctx, tutor, locked.row.ID, deadline, lesson, now); err != nil {
		return err
	}
	for _, kind := range changes {
		if kind != domain.IssueChangeDeadline {
			continue
		}
		if err := repos.Homework.SetSubmissionDeadlines(ctx, locked.row.ID, deadline, now); err != nil {
			return err
		}
	}
	readers, all := readableRecipients(locked.row.Status, locked.states)
	return s.signal(ctx, repos, tutor, locked.row.ID, all, readers)
}

func (s *Service) WithdrawIssue(ctx context.Context, actor domain.TutorActor, id domain.IssueID) (IssueItem, error) {
	tutor := actor.UserID
	if err := s.reserveSlot(ctx, actor, s.writeSlot()); err != nil {
		return IssueItem{}, err
	}
	var withdrawn IssueItem
	changed := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		if err := s.lockRecipients(ctx, repos, tutor, id); err != nil {
			return err
		}
		locked, err := s.lockIssueRow(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		if locked.row.Status == domain.IssueWithdrawn {
			withdrawn, err = s.itemOf(ctx, repos, tutor, id)
			return err
		}
		for _, state := range locked.states {
			if state.FirstSubmittedAt != nil {
				return ErrIssueHasSubmissions
			}
		}
		readers, all := readableRecipients(locked.row.Status, locked.states)
		if err := repos.Homework.WithdrawIssue(ctx, tutor, id, now); err != nil {
			return err
		}
		released, err := repos.Homework.RemoveIssueAnswerReferences(ctx, id)
		if err != nil {
			return err
		}
		if err := library.ReleaseReferences(ctx, repos, s.deps.Random, s.deps.Logger, domain.ReferenceAnswer, released, now); err != nil {
			return err
		}
		if err := s.notifyStudents(ctx, repos, tutor, domain.NotificationIssueWithdrawn, readers, now); err != nil {
			return err
		}
		if err := s.signal(ctx, repos, tutor, id, all, readers); err != nil {
			return err
		}
		changed = true
		withdrawn, err = s.itemOf(ctx, repos, tutor, id)
		return err
	})
	if err != nil {
		return IssueItem{}, err
	}
	if changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "homework.issue.withdrawn", tutorAttribute(tutor), issueAttribute(id))
	}
	return withdrawn, nil
}

func sameInstant(a *time.Time, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func sameLesson(a *domain.LessonID, b *domain.LessonID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

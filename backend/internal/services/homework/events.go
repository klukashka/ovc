package homework

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type recipient struct {
	submission   domain.SubmissionID
	relationship domain.RelationshipID
	student      domain.UserID
}

func (s *Service) announceCreated(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, issue domain.IssueID, submissions []repositories.NewSubmission,
	now time.Time,
) error {
	recipients := make([]recipient, 0, len(submissions))
	emails := make([]repositories.QueuedOutboxRow, 0, len(submissions))
	for _, submission := range submissions {
		recipients = append(recipients, recipient{submission: submission.ID, relationship: submission.Relationship, student: submission.Student})
		payload, err := json.Marshal(domain.IssueCreatedPayload{Submission: hex.EncodeToString(submission.ID[:])})
		if err != nil {
			return fmt.Errorf("encode the issue created payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](s.deps.Random)
		if err != nil {
			return err
		}
		emails = append(emails, repositories.QueuedOutboxRow{ID: id, Payload: payload, IdempotencyKey: domain.IssueCreatedKey(submission.ID)})
	}
	if err := s.notifyStudents(ctx, repos, tutor, domain.NotificationIssueCreated, recipients, now); err != nil {
		return err
	}
	if _, err := repos.Outbox.InsertAll(ctx, repositories.NewOutboxRows{
		Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: domain.EventIssueCreated, NextAttemptAt: now, CreatedAt: now, Rows: emails,
	}); err != nil {
		return err
	}
	all := make([]domain.SubmissionID, 0, len(recipients))
	for _, recipient := range recipients {
		all = append(all, recipient.submission)
	}
	return s.signal(ctx, repos, tutor, issue, all, recipients)
}

func (s *Service) notifyStudents(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, event domain.NotificationEvent, recipients []recipient, now time.Time,
) error {
	for _, recipient := range recipients {
		id, err := core.NewID[domain.NotificationID](s.deps.Random)
		if err != nil {
			return err
		}
		key := domain.IssueCreatedKey(recipient.submission)
		if event == domain.NotificationIssueWithdrawn {
			key = domain.IssueWithdrawnKey(recipient.submission)
		}
		student, relationship := recipient.student, recipient.relationship
		inserted, err := repos.Notifications.Insert(ctx, repositories.NewNotification{
			ID: id, UserID: student, TutorID: tutor, StudentID: &student, Event: event, SubjectID: domain.EntityID(recipient.submission),
			RelationshipID: &relationship, IdempotencyKey: key, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		if !inserted {
			continue
		}
		if err := repos.Signals.Notification(ctx, student, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) signal(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, issue domain.IssueID, submissions []domain.SubmissionID,
	readers []recipient,
) error {
	signals := []domain.EntitySignal{{Users: []domain.UserID{tutor}, Entity: domain.EntityIssue, ID: domain.EntityID(issue)}}
	students := make(map[domain.SubmissionID]domain.UserID, len(readers))
	for _, reader := range readers {
		students[reader.submission] = reader.student
	}
	for _, submission := range submissions {
		users := []domain.UserID{tutor}
		if student, readable := students[submission]; readable {
			users = append(users, student)
		}
		signals = append(signals, domain.EntitySignal{Users: users, Entity: domain.EntitySubmission, ID: domain.EntityID(submission)})
	}
	return repos.Signals.Entities(ctx, signals)
}

func readableRecipients(status domain.IssueStatus, states []repositories.IssueSubmissionState) ([]recipient, []domain.SubmissionID) {
	readers := make([]recipient, 0, len(states))
	all := make([]domain.SubmissionID, 0, len(states))
	for _, state := range states {
		all = append(all, state.ID)
		if domain.ReadableStudentSubmission(status, false, state.RelationshipState) {
			readers = append(readers, recipient{submission: state.ID, relationship: state.RelationshipID, student: state.StudentID})
		}
	}
	return readers, all
}

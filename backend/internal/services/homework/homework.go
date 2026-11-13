package homework

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

var (
	ErrUnauthenticated       = errors.New("the tutor can no longer write homework")
	ErrNotFound              = errors.New("the item is not in the caller's scope")
	ErrNotEditable           = errors.New("the item can no longer be changed")
	ErrRelationshipNotActive = errors.New("the relationship cannot receive work")
	ErrAudienceNotActive     = errors.New("the group cannot receive work")
	ErrConflict              = errors.New("the item changed since the request was prepared")
	ErrIssueHasSubmissions   = errors.New("the issue already has submitted work")
	ErrAnswerFrozen          = errors.New("an automatic answer is frozen once submitted")
	ErrQuotaExceeded         = errors.New("the tutor's file quota is exceeded")
	ErrReturnNotAllowed      = errors.New("the work cannot be returned for revision")
)

const (
	fieldBody      = "body"
	fieldDeadline  = "body.deadline_at"
	fieldLesson    = "body.lesson_id"
	fieldCursor    = "query.cursor"
	limitIssues    = "issues_hour"
	limitWrites    = "homework_writes_hour"
	recordRefKind  = "ref_kind"
	recordTutor    = "tutor_id"
	recordIssue    = "issue_id"
	recordAttached = "attachment_id"
)

type FieldProblem struct {
	Path string
	Code domain.FieldCode
}

type ValidationError struct {
	Fields []FieldProblem
}

func (e *ValidationError) Error() string {
	return "the request is invalid"
}

func invalid(path string, code domain.FieldCode) *ValidationError {
	return &ValidationError{Fields: []FieldProblem{{Path: path, Code: code}}}
}

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "a homework rate limit is reached"
}

type Addresses interface {
	IssueSnapshotAddress(id domain.AttachmentID, issue domain.IssueID, disposition domain.Disposition, now time.Time) (string, time.Time)
	AnswerAddress(id domain.AttachmentID, submission domain.SubmissionID, disposition domain.Disposition, now time.Time) (string, time.Time)
	ReviewItemAddress(id domain.AttachmentID, revision domain.ReviewRevisionID, disposition domain.Disposition, now time.Time) (string, time.Time)
}

type Templates interface {
	TemplateForIssue(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.TemplateID) (library.IssuableTemplate, error)
}

type Dependencies struct {
	Store     *repositories.Store
	Clock     clock.Source
	Random    core.RandomSource
	Keys      core.Keys
	Addresses Addresses
	Templates Templates
	Logger    *slog.Logger
	Caps      domain.HomeworkCaps
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	dependencies.Caps = dependencies.Caps.WithDefaults()
	return &Service{deps: dependencies}
}

func (s *Service) lockTutor(ctx context.Context, repos repositories.Repositories, actor domain.TutorActor) (time.Time, error) {
	err := repos.TutorProfiles.Lock(ctx, actor.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return time.Time{}, err
	}
	session, err := repos.Sessions.ByID(ctx, actor.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	if session.UserID != actor.UserID || session.RevokedAt != nil || !checkedAt.Before(session.ExpiresAt) {
		return time.Time{}, ErrUnauthenticated
	}
	account, err := repos.Users.ByID(ctx, actor.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	if account.State != domain.AccountActive {
		return time.Time{}, ErrUnauthenticated
	}
	return checkedAt, nil
}

type slotLimit struct {
	purpose domain.RateLimitPurpose
	limit   int64
	name    string
}

func (s *Service) issueSlot() slotLimit {
	return slotLimit{purpose: domain.PurposeIssuesHour, limit: s.deps.Caps.IssuesPerHour, name: limitIssues}
}

func (s *Service) writeSlot() slotLimit {
	return slotLimit{purpose: domain.PurposeHomeworkWritesHour, limit: s.deps.Caps.WritesPerHour, name: limitWrites}
}

func (s *Service) reserve(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, slot slotLimit, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, domain.IssueWindow)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: slot.purpose, Subject: s.deps.Keys.ThrottleSubject(string(slot.purpose), tutor.String()), WindowStart: window,
	})
	if err != nil {
		return err
	}
	if int64(count) > slot.limit {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "homework.rate_limited", tutorAttribute(tutor), slog.String("limit", slot.name))
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (s *Service) reserveSlot(ctx context.Context, actor domain.TutorActor, slot slotLimit) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		return s.reserve(ctx, repos, actor.UserID, slot, now)
	})
}

func tutorAttribute(tutor domain.UserID) slog.Attr {
	return slog.String(recordTutor, tutor.String())
}

func issueAttribute(issue domain.IssueID) slog.Attr {
	return slog.String(recordIssue, issue.String())
}

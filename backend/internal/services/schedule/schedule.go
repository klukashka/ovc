package schedule

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrUnauthenticated   = errors.New("the tutor is no longer active")
	ErrNotFound          = errors.New("the resource is not in the caller's scope")
	ErrAudienceNotActive = errors.New("the audience is not active")
	ErrLessonInPast      = errors.New("the lesson would start in the past")
	ErrConflict          = errors.New("the audience changed while the request held its locks")
	errMembersChanged    = errors.New("a group's members changed under the lock")
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
	return "the lesson write limit is reached"
}

type Conflict struct {
	Title    string
	StartsAt time.Time
	EndsAt   time.Time
}

type OverlapError struct {
	Lessons []Conflict
	More    int
}

func (e *OverlapError) Error() string {
	return "the lesson overlaps another lesson"
}

type Dependencies struct {
	Store  *repositories.Store
	Clock  clock.Source
	Random core.RandomSource
	Keys   core.Keys
	Logger *slog.Logger
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	return &Service{deps: dependencies}
}

func (s *Service) lockActingTutor(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) (repositories.User, error) {
	err := repos.TutorProfiles.Lock(ctx, tutor)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.User{}, ErrUnauthenticated
	}
	if err != nil {
		return repositories.User{}, err
	}
	account, err := repos.Users.ByID(ctx, tutor)
	if err != nil {
		return repositories.User{}, err
	}
	if account.State != domain.AccountActive {
		return repositories.User{}, ErrUnauthenticated
	}
	return account, nil
}

func (s *Service) reserveWrite(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) error {
	window, retryAfter := domain.FixedWindow(now, domain.LessonWriteWindow)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose:     domain.PurposeLessonWrites,
		Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeLessonWrites), tutor.String()),
		WindowStart: window,
	})
	if err != nil {
		return err
	}
	if count > domain.LessonWritesPerHour {
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func loadZone(name string) (*time.Location, error) {
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, errors.Join(errors.New("a stored timezone cannot be loaded"), err)
	}
	return location, nil
}

func tutorAttribute(tutor domain.UserID) slog.Attr {
	return slog.String("tutor_id", tutor.String())
}

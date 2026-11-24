package export

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	ErrUnauthenticated  = errors.New("the session or the account no longer allows this request")
	ErrEmailNotVerified = errors.New("the account's email is not confirmed")
	ErrInProgress       = errors.New("an export of the caller is queued or building")
	ErrCapacityFull     = errors.New("the export intent capacity is full")
	ErrNotFound         = errors.New("no export may be downloaded")
	errBuildStopped     = errors.New("the export row changed under the build")
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("too many export requests; retry after %s", e.RetryAfter)
}

type PasswordChecker interface {
	VerifyCurrentPassword(ctx context.Context, id domain.UserID, password string, now time.Time) (repositories.User, error)
}

type CleanupRequester interface {
	RequestExportCleanup(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, now time.Time) error
	ConfirmAndCleanExport(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, now time.Time) error
}

type DownloadSigner interface {
	PresignDownload(ctx context.Context, key string, contentType string, disposition string) (core.Secret, error)
}

type Dependencies struct {
	Store      *repositories.Store
	Clock      clock.Source
	Wall       func() time.Time
	Random     core.RandomSource
	Keys       core.Keys
	Logger     *slog.Logger
	Objects    storage.Store
	Signer     DownloadSigner
	Passwords  PasswordChecker
	Cleanup    CleanupRequester
	PublicHost string
	Timing     Timing
}

type Timing struct {
	Complete time.Duration
	Budget   time.Duration
}

func (t Timing) withDefaults() Timing {
	if t.Complete <= 0 {
		t.Complete = domain.UploadSettlementBudget
	}
	if t.Budget <= 0 {
		t.Budget = domain.ExportBuildBudget
	}
	return t
}

type Service struct {
	deps          Dependencies
	recoveryMutex sync.Mutex
}

func New(dependencies Dependencies) *Service {
	if dependencies.Passwords == nil || dependencies.Signer == nil {
		panic("the export service of the API needs the password check and the download signer")
	}
	return NewBuilder(dependencies)
}

func NewBuilder(dependencies Dependencies) *Service {
	if dependencies.Store == nil || dependencies.Clock == nil || dependencies.Wall == nil || dependencies.Random == nil ||
		dependencies.Logger == nil || dependencies.Objects == nil || dependencies.Cleanup == nil {
		panic("the export service needs its store, clocks, randomness, logger, storage and cleanup")
	}
	dependencies.Timing = dependencies.Timing.withDefaults()
	return &Service{deps: dependencies}
}

func exportAttribute(id domain.ExportID) slog.Attr {
	return slog.String("export_id", id.String())
}

func userAttribute(id domain.UserID) slog.Attr {
	return slog.String("user_id", id.String())
}

func buildAttribute(build *domain.ExportBuildID) slog.Attr {
	if build == nil {
		return slog.String("build_id", "")
	}
	return slog.String("build_id", build.String())
}

func (s *Service) newID() (domain.ExportID, error) {
	return core.NewID[domain.ExportID](s.deps.Random)
}

type Effects struct {
	service *Service
}

func NewEffects(dependencies Dependencies) Effects {
	if dependencies.Store == nil || dependencies.Random == nil || dependencies.Logger == nil || dependencies.Cleanup == nil {
		panic("the export effects need their store, randomness, logger and cleanup")
	}
	return Effects{service: &Service{deps: dependencies}}
}

func (e Effects) WithdrawOwn(ctx context.Context, repos repositories.Repositories, user domain.UserID, cause domain.ExportInvalidationCause, now time.Time) error {
	return e.service.WithdrawOwn(ctx, repos, user, cause, now)
}

func (e Effects) DeleteForPurge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	return e.service.DeleteForPurge(ctx, repos, user, now)
}

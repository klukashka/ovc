package export

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	purposeExportRequests domain.RateLimitPurpose = "export_requests"
	purposeExportMints    domain.RateLimitPurpose = "export_downloads"
	downloadContentType                           = "application/zip"
)

type View struct {
	Export        *Summary
	NextAllowedAt *time.Time
}

type Summary struct {
	ID            domain.ExportID
	State         domain.ExportState
	Failure       *domain.ExportFailure
	RequestedAt   time.Time
	BuiltAt       *time.Time
	ExpiresAt     *time.Time
	SizeBytes     *int64
	ProgressDone  int32
	ProgressTotal int32
}

func summaryOf(row repositories.Export) *Summary {
	return &Summary{
		ID: row.ID, State: row.State, Failure: row.Failure, RequestedAt: row.RequestedAt, BuiltAt: row.BuiltAt, ExpiresAt: row.ExpiresAt,
		SizeBytes: row.SizeBytes, ProgressDone: row.ProgressDone, ProgressTotal: row.ProgressTotal,
	}
}

type Download struct {
	URL       core.Secret
	ExpiresAt time.Time
}

func (s *Service) Request(ctx context.Context, principal accounts.Principal, password string) (View, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return View{}, err
	}
	used, err := s.deps.Store.Repositories().UploadIntents.CapacityUsed(ctx, domain.UploadCapacityExport)
	if err != nil {
		return View{}, err
	}
	if used >= domain.ExportUploadIntentCapacity {
		return View{}, ErrCapacityFull
	}
	user, err := s.deps.Passwords.VerifyCurrentPassword(ctx, principal.UserID, password, now)
	if err != nil {
		return View{}, err
	}
	var created repositories.Export
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		account, err := s.lockCaller(ctx, repos, principal, now)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(account.PasswordHash), []byte(user.PasswordHash)) != 1 {
			return ErrUnauthenticated
		}
		if err := repos.Exports.LockUsersForChange(ctx, principal.UserID); err != nil {
			return err
		}
		previous, found, err := lockOwnExport(ctx, repos, principal.UserID)
		if err != nil {
			return err
		}
		if found && (previous.State == domain.ExportQueued || previous.State == domain.ExportBuilding) {
			return ErrInProgress
		}
		if err := checkStartLimit(ctx, repos, principal.UserID, now); err != nil {
			return err
		}
		if err := s.reserve(ctx, repos, purposeExportRequests, domain.ExportRequestWindow, domain.ExportRequestLimit, principal.UserID, now); err != nil {
			return err
		}
		if found {
			if err := s.deleteLocked(ctx, repos, previous, now); err != nil {
				return err
			}
		}
		id, err := s.newID()
		if err != nil {
			return err
		}
		created, err = repos.Exports.Insert(ctx, repositories.NewExport{ID: id, User: principal.UserID, Language: account.Language, Timezone: account.Timezone, RequestedAt: now})
		if err != nil {
			return err
		}
		return s.queueRequestedEmail(ctx, repos, created, now)
	})
	if err != nil {
		return View{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.requested", userAttribute(principal.UserID), slog.String("role", string(principal.Role)), exportAttribute(created.ID))
	return View{Export: summaryOf(created)}, nil
}

type callerAccount struct {
	PasswordHash string
	Language     domain.Language
	Timezone     string
}

func (s *Service) lockCaller(ctx context.Context, repos repositories.Repositories, principal accounts.Principal, now time.Time) (callerAccount, error) {
	err := accounts.LockActingAccount(ctx, repos, principal, now, domain.AccountActive)
	if errors.Is(err, accounts.ErrUnauthenticated) {
		return callerAccount{}, ErrUnauthenticated
	}
	if err != nil {
		return callerAccount{}, err
	}
	credentials, err := repos.Users.LockCredentials(ctx, principal.UserID)
	if err != nil {
		return callerAccount{}, err
	}
	if principal.Role == domain.RoleTutor {
		err := repos.TutorProfiles.Lock(ctx, principal.UserID)
		if errors.Is(err, repositories.ErrNotFound) {
			return callerAccount{}, ErrUnauthenticated
		}
		if err != nil {
			return callerAccount{}, err
		}
	}
	user, err := repos.Users.ByID(ctx, principal.UserID)
	if err != nil {
		return callerAccount{}, err
	}
	if user.EmailVerifiedAt == nil {
		return callerAccount{}, ErrEmailNotVerified
	}
	return callerAccount{PasswordHash: credentials.PasswordHash, Language: user.Language, Timezone: user.Timezone}, nil
}

func lockOwnExport(ctx context.Context, repos repositories.Repositories, user domain.UserID) (repositories.Export, bool, error) {
	row, err := repos.Exports.LockOfUser(ctx, user)
	if errors.Is(err, repositories.ErrNotFound) {
		return row, false, nil
	}
	return row, err == nil, err
}

func checkStartLimit(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	limit, err := repos.Exports.LockLimit(ctx, user)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if now.Before(limit.AllowedFrom) {
		return &ThrottledError{RetryAfter: limit.AllowedFrom.Sub(now)}
	}
	return nil
}

func (s *Service) reserve(ctx context.Context, repos repositories.Repositories, purpose domain.RateLimitPurpose, window time.Duration, limit int32, user domain.UserID, now time.Time) error {
	start, retryAfter := domain.FixedWindow(now, window)
	count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
		Purpose: purpose, Subject: s.deps.Keys.ThrottleSubject(string(purpose), user.String()), WindowStart: start,
	})
	if err != nil {
		return err
	}
	if count > limit {
		return &ThrottledError{RetryAfter: retryAfter}
	}
	return nil
}

func (s *Service) queueRequestedEmail(ctx context.Context, repos repositories.Repositories, export repositories.Export, now time.Time) error {
	payload, err := json.Marshal(domain.ExportRequestedPayload{
		Export: hex.EncodeToString(export.ID[:]), User: hex.EncodeToString(export.UserID[:]), RequestedAtMicros: export.RequestedAt.UnixMicro(),
	})
	if err != nil {
		return fmt.Errorf("encode the export request email: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityHigh, Kind: domain.EventExportRequested, Payload: payload,
		IdempotencyKey: domain.ExportRequestedKey(export.ID), NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

func (s *Service) Current(ctx context.Context, principal accounts.Principal) (View, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return View{}, err
	}
	repos := s.deps.Store.Repositories()
	view := View{}
	row, err := repos.Exports.OfUser(ctx, principal.UserID)
	if err == nil {
		view.Export = summaryOf(row)
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return View{}, err
	}
	limit, err := repos.Exports.Limit(ctx, principal.UserID)
	if err == nil && now.Before(limit.AllowedFrom) {
		view.NextAllowedAt = &limit.AllowedFrom
	} else if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return View{}, err
	}
	return view, nil
}

func (s *Service) Delete(ctx context.Context, principal accounts.Principal) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	var deleted *repositories.Export
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if _, err := s.lockCaller(ctx, repos, principal, now); err != nil {
			return err
		}
		row, found, err := lockOwnExport(ctx, repos, principal.UserID)
		if err != nil || !found {
			return err
		}
		deleted = &row
		return s.deleteLocked(ctx, repos, row, now)
	})
	if err != nil {
		return err
	}
	if deleted != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.deleted", exportAttribute(deleted.ID), slog.String("state", string(deleted.State)))
	}
	return nil
}

func (s *Service) deleteLocked(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) error {
	if err := s.releaseStorage(ctx, repos, row, now); err != nil {
		return err
	}
	_, err := repos.Exports.Delete(ctx, row.ID)
	return err
}

func (s *Service) releaseStorage(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) error {
	if row.BuildID == nil {
		return nil
	}
	switch row.State {
	case domain.ExportReady:
		return s.queueObjectDeletion(ctx, repos, row, now)
	case domain.ExportBuilding, domain.ExportFailed:
		return s.requestCleanupIfOpen(ctx, repos, *row.BuildID, now)
	case domain.ExportQueued, domain.ExportExpired:
	}
	return nil
}

func (s *Service) requestCleanupIfOpen(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, now time.Time) error {
	intent, err := repos.UploadIntents.Lock(ctx, build.Identity())
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if intent.CleanupRequested {
		return nil
	}
	return s.deps.Cleanup.RequestExportCleanup(ctx, repos, build, now)
}

func (s *Service) queueObjectDeletion(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) error {
	if row.BuildID == nil || row.StorageKey == nil {
		return nil
	}
	payload, err := json.Marshal(domain.ExportObjectPayload{Export: row.ID.String(), Build: row.BuildID.String(), Key: *row.StorageKey})
	if err != nil {
		return fmt.Errorf("encode the export object deletion: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneStorageDelete, Priority: domain.PriorityNormal, Kind: domain.KindExportObject, Payload: payload,
		IdempotencyKey: domain.ExportObjectDeletionKey(*row.BuildID), NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

func (s *Service) MintDownload(ctx context.Context, principal accounts.Principal) (Download, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Download{}, err
	}
	var row repositories.Export
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		found, err := repos.Exports.OfUser(ctx, principal.UserID)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if found.State != domain.ExportReady || found.StorageKey == nil || found.ExpiresAt == nil || found.BuiltAt == nil || !now.Before(*found.ExpiresAt) {
			return ErrNotFound
		}
		row = found
		return s.reserve(ctx, repos, purposeExportMints, domain.ExportDownloadWindow, domain.ExportDownloadLimit, principal.UserID, now)
	})
	if err != nil {
		return Download{}, err
	}
	disposition := fmt.Sprintf(`attachment; filename="ovc-export-%s.zip"`, row.BuiltAt.UTC().Format(time.DateOnly))
	signed, err := s.deps.Signer.PresignDownload(ctx, *row.StorageKey, downloadContentType, disposition)
	if err != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "export.download.failed", exportAttribute(row.ID), slog.String("kind", "presign_failed"))
		return Download{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.download.minted", exportAttribute(row.ID))
	return Download{URL: signed, ExpiresAt: now.Add(storage.PresignedReadLifetime)}, nil
}

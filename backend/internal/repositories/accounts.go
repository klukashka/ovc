package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/core"
	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	User              = dbgen.User
	NewUser           = dbgen.CreateUserParams
	TutorProfile      = dbgen.TutorProfile
	Session           = dbgen.Session
	NewSession        = dbgen.CreateSessionParams
	SessionRotation   = dbgen.RotateSessionParams
	EmailToken        = dbgen.EmailToken
	NewEmailRequest   = dbgen.CreateEmailRequestParams
	NewEmailToken     = dbgen.CreateEmailTokenParams
	ConsumedToken     = dbgen.ConsumeEmailTokenRow
	RateLimitSlot     = dbgen.ReserveRateLimitSlotParams
	JobRun            = dbgen.JobRun
	JobRunRecord      = dbgen.RecordJobRunParams
	EmailTokenLookup  = dbgen.GetUsableEmailTokenParams
	SessionPrincipal  = dbgen.GetSessionPrincipalRow
	SessionStanding   = dbgen.GetSessionStandingsRow
	PasswordRehash    = dbgen.UpdateUserPasswordHashParams
	LockedCredentials = dbgen.LockUserCredentialsRow
	SignUp            = dbgen.CreateUserIfEmailFreeParams
	EmailRequest      = dbgen.EmailRequest
)

type Users struct {
	queries *dbgen.Queries
}

func (r Users) Create(ctx context.Context, user NewUser) error {
	return done(r.queries.CreateUser(ctx, user), "create user")
}

func (r Users) CreateIfEmailFree(ctx context.Context, user SignUp) (bool, error) {
	rows, err := r.queries.CreateUserIfEmailFree(ctx, user)
	return applied(rows, err, "create user if the email is free")
}

func (r Users) ConfirmEmail(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.ConfirmUserEmail(ctx, dbgen.ConfirmUserEmailParams{Now: now, ID: id})
	return applied(rows, err, "confirm user email")
}

func (r Users) ReplacePassword(ctx context.Context, id domain.UserID, passwordHash string, now time.Time) (bool, error) {
	rows, err := r.queries.ReplaceUserPassword(ctx, dbgen.ReplaceUserPasswordParams{PasswordHash: passwordHash, Now: now, ID: id})
	return applied(rows, err, "replace user password")
}

func (r Users) TutorStanding(ctx context.Context, tutor domain.UserID) (domain.TutorStanding, error) {
	row, err := r.queries.GetTutorStanding(ctx, tutor)
	return one(domain.TutorStanding{State: row.State, DeletedAt: row.DeletedAt}, err, "a tutor's standing")
}

func (r Users) ByID(ctx context.Context, id domain.UserID) (User, error) {
	user, err := r.queries.GetUserByID(ctx, id)
	return one(user, err, "user by id")
}

func (r Users) UpdatePasswordHash(ctx context.Context, rehash PasswordRehash) (bool, error) {
	rows, err := r.queries.UpdateUserPasswordHash(ctx, rehash)
	return applied(rows, err, "update password hash")
}

func (r Users) LockCredentials(ctx context.Context, id domain.UserID) (LockedCredentials, error) {
	locked, err := r.queries.LockUserCredentials(ctx, id)
	return one(locked, err, "lock user credentials")
}

func (r Users) MarkDeleted(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.MarkUserDeleted(ctx, dbgen.MarkUserDeletedParams{Now: &now, ID: id})
	return applied(rows, err, "mark user deleted")
}

func (r Users) Block(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.BlockUser(ctx, dbgen.BlockUserParams{Now: now, ID: id})
	return applied(rows, err, "block user")
}

func (r Users) ByEmail(ctx context.Context, email string) (User, error) {
	user, err := r.queries.GetUserByEmail(ctx, email)
	return one(user, err, "user by email")
}

type TutorProfiles struct {
	queries *dbgen.Queries
}

type NewTutorProfile struct {
	UserID              domain.UserID
	FileQuotaBytes      int64
	RecordingQuotaBytes int64
	CreatedAt           time.Time
}

const tutorCodeAttempts = 5

var ErrTutorCodesExhausted = errors.New("every minted tutor code was already taken")

func (r TutorProfiles) Create(ctx context.Context, profile NewTutorProfile, random core.RandomSource) error {
	for range tutorCodeAttempts {
		code, err := core.NewTutorCode(random)
		if err != nil {
			return fmt.Errorf("create tutor profile: %w", err)
		}
		rows, err := r.queries.CreateTutorProfileIfCodeFree(ctx, dbgen.CreateTutorProfileIfCodeFreeParams{
			UserID: profile.UserID, Code: code, FileQuotaBytes: profile.FileQuotaBytes, RecordingQuotaBytes: profile.RecordingQuotaBytes,
			CreatedAt: profile.CreatedAt,
		})
		inserted, err := applied(rows, err, "create tutor profile")
		if err != nil || inserted {
			return err
		}
	}
	return fmt.Errorf("create tutor profile: %w", ErrTutorCodesExhausted)
}

func (r TutorProfiles) ByUser(ctx context.Context, id domain.UserID) (TutorProfile, error) {
	profile, err := r.queries.GetTutorProfile(ctx, id)
	return one(profile, err, "tutor profile")
}

type TutorCode = dbgen.GetTutorCodeRow

func (r TutorProfiles) Code(ctx context.Context, tutor domain.UserID) (TutorCode, error) {
	code, err := r.queries.GetTutorCode(ctx, tutor)
	return one(code, err, "tutor code")
}

func (r TutorProfiles) Lock(ctx context.Context, tutor domain.UserID) error {
	_, err := r.LockCode(ctx, tutor)
	return err
}

func (r TutorProfiles) DismissOnboarding(ctx context.Context, tutor domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.DismissTutorOnboarding(ctx, dbgen.DismissTutorOnboardingParams{Now: now, UserID: tutor})
	return applied(rows, err, "dismiss the tutor's onboarding")
}

func (r TutorProfiles) LockCode(ctx context.Context, tutor domain.UserID) (domain.TutorCode, error) {
	code, err := r.queries.LockTutorProfile(ctx, tutor)
	return one(code, err, "lock tutor profile")
}

func (r TutorProfiles) ReplaceCode(ctx context.Context, tutor domain.UserID, random core.RandomSource, now time.Time) (domain.TutorCode, error) {
	for range tutorCodeAttempts {
		code, err := core.NewTutorCode(random)
		if err != nil {
			return "", fmt.Errorf("replace tutor code: %w", err)
		}
		rows, err := r.queries.ReplaceTutorCodeIfFree(ctx, dbgen.ReplaceTutorCodeIfFreeParams{Code: code, Now: now, UserID: tutor})
		replaced, err := applied(rows, err, "replace tutor code")
		if err != nil {
			return "", err
		}
		if replaced {
			return code, nil
		}
	}
	return "", fmt.Errorf("replace tutor code: %w", ErrTutorCodesExhausted)
}

type Sessions struct {
	queries *dbgen.Queries
}

func (r Sessions) Create(ctx context.Context, session NewSession) error {
	return done(r.queries.CreateSession(ctx, session), "create session")
}

func (r Sessions) ByID(ctx context.Context, id domain.SessionID) (Session, error) {
	session, err := r.queries.GetSession(ctx, id)
	return one(session, err, "session by id")
}

func (r Sessions) Principal(ctx context.Context, id domain.SessionID) (SessionPrincipal, error) {
	principal, err := r.queries.GetSessionPrincipal(ctx, id)
	return one(principal, err, "session principal")
}

func (r Sessions) Standings(ctx context.Context, ids []domain.SessionID) ([]SessionStanding, error) {
	standings, err := r.queries.GetSessionStandings(ctx, uuids(ids))
	return standings, done(err, "session standings")
}

func (r Sessions) Rotate(ctx context.Context, rotation SessionRotation) (int64, error) {
	generation, err := r.queries.RotateSession(ctx, rotation)
	return one(generation, err, "rotate session")
}

func (r Sessions) Revoke(ctx context.Context, id domain.SessionID, now time.Time) (bool, error) {
	rows, err := r.queries.RevokeSession(ctx, dbgen.RevokeSessionParams{ID: id, Now: &now})
	return applied(rows, err, "revoke session")
}

func (r Sessions) RevokeAllOf(ctx context.Context, user domain.UserID, now time.Time) (int64, error) {
	rows, err := r.queries.RevokeUserSessions(ctx, dbgen.RevokeUserSessionsParams{UserID: user, Now: &now})
	return counted(rows, err, "revoke user sessions")
}

func (r Sessions) DeleteFinishedBefore(ctx context.Context, before time.Time, batchSize int32, maxBatches int) (int64, error) {
	expired, err := deleteInBatches(maxBatches, batchSize, "delete expired sessions", func() (int64, error) {
		return r.queries.DeleteExpiredSessions(ctx, dbgen.DeleteExpiredSessionsParams{Before: before, BatchSize: batchSize})
	})
	if err != nil {
		return expired, err
	}
	revoked, err := deleteInBatches(maxBatches, batchSize, "delete revoked sessions", func() (int64, error) {
		return r.queries.DeleteRevokedSessions(ctx, dbgen.DeleteRevokedSessionsParams{Before: before, BatchSize: batchSize})
	})
	return expired + revoked, err
}

func (r Sessions) RevokeOthersOf(ctx context.Context, user domain.UserID, kept domain.SessionID, now time.Time) (int64, error) {
	rows, err := r.queries.RevokeOtherUserSessions(ctx, dbgen.RevokeOtherUserSessionsParams{Now: now, UserID: user, Kept: kept})
	return counted(rows, err, "revoke other user sessions")
}

type EmailTokens struct {
	queries *dbgen.Queries
}

func (r EmailTokens) CreateRequest(ctx context.Context, request NewEmailRequest) error {
	return done(r.queries.CreateEmailRequest(ctx, request), "create email request")
}

func (r EmailTokens) Request(ctx context.Context, id domain.EmailRequestID) (EmailRequest, error) {
	request, err := r.queries.GetEmailRequest(ctx, id)
	return one(request, err, "email request")
}

func (r EmailTokens) Superseded(ctx context.Context, request EmailRequest) (bool, error) {
	newer, err := r.queries.HasNewerEmailRequest(ctx, dbgen.HasNewerEmailRequestParams{
		UserID: request.UserID, Purpose: request.Purpose, CreatedAt: request.CreatedAt, ID: request.ID,
	})
	return one(newer, err, "newer email request")
}

func (r EmailTokens) DeleteRequestsBefore(ctx context.Context, before time.Time, batchSize int32, maxBatches int) (int64, error) {
	return deleteInBatches(maxBatches, batchSize, "delete old email requests", func() (int64, error) {
		return r.queries.DeleteOldEmailRequests(ctx, dbgen.DeleteOldEmailRequestsParams{Before: before, BatchSize: batchSize})
	})
}

func (r EmailTokens) DeleteOpenRequests(ctx context.Context, user domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteOpenEmailRequests(ctx, user)
	return counted(rows, err, "delete open email requests")
}

func (r EmailTokens) RequestUsed(ctx context.Context, request domain.EmailRequestID) (bool, error) {
	used, err := r.queries.EmailRequestHasUsedToken(ctx, request)
	return one(used, err, "email request has a used token")
}

func (r EmailTokens) Create(ctx context.Context, token NewEmailToken) error {
	return done(r.queries.CreateEmailToken(ctx, token), "create email token")
}

func (r EmailTokens) DiscardUnusedFor(ctx context.Context, user domain.UserID, purpose domain.EmailPurpose) (int64, error) {
	rows, err := r.queries.DeleteUnusedEmailTokensForPurpose(ctx, dbgen.DeleteUnusedEmailTokensForPurposeParams{UserID: user, Purpose: purpose})
	return counted(rows, err, "discard unused email tokens of a purpose")
}

func (r EmailTokens) DiscardAllUnused(ctx context.Context, user domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteUnusedEmailTokens(ctx, user)
	return counted(rows, err, "discard unused email tokens")
}

func (r EmailTokens) Usable(ctx context.Context, lookup EmailTokenLookup) (EmailToken, error) {
	token, err := r.queries.GetUsableEmailToken(ctx, lookup)
	return one(token, err, "usable email token")
}

func (r EmailTokens) Consume(ctx context.Context, lookup EmailTokenLookup) (ConsumedToken, error) {
	consumed, err := r.queries.ConsumeEmailToken(ctx, dbgen.ConsumeEmailTokenParams{Now: &lookup.Now, TokenHash: lookup.TokenHash, Purpose: lookup.Purpose})
	return one(consumed, err, "consume email token")
}

type RateLimits struct {
	queries *dbgen.Queries
}

func (r RateLimits) Reserve(ctx context.Context, slot RateLimitSlot) (int32, error) {
	count, err := r.queries.ReserveRateLimitSlot(ctx, slot)
	return one(count, err, "reserve rate limit slot")
}

func (r RateLimits) Release(ctx context.Context, slot RateLimitSlot) (bool, error) {
	rows, err := r.queries.ReleaseRateLimitSlot(ctx, dbgen.ReleaseRateLimitSlotParams(slot))
	return applied(rows, err, "release rate limit slot")
}

func (r RateLimits) DeleteBefore(ctx context.Context, before time.Time, batchSize int32, maxBatches int) (int64, error) {
	return deleteInBatches(maxBatches, batchSize, "delete expired rate limit counters", func() (int64, error) {
		return r.queries.DeleteExpiredRateLimitCounters(ctx, dbgen.DeleteExpiredRateLimitCountersParams{Before: before, BatchSize: batchSize})
	})
}

func deleteInBatches(maxBatches int, batchSize int32, action string, batch func() (int64, error)) (int64, error) {
	var deleted int64
	for range maxBatches {
		rows, err := batch()
		if err != nil {
			return deleted, fmt.Errorf("%s: %w", action, err)
		}
		deleted += rows
		if rows < int64(batchSize) {
			break
		}
	}
	return deleted, nil
}

type JobRuns struct {
	queries *dbgen.Queries
}

func (r JobRuns) Record(ctx context.Context, run JobRunRecord) error {
	return done(r.queries.RecordJobRun(ctx, run), "record job run")
}

func (r JobRuns) ByJob(ctx context.Context, job domain.JobName) (JobRun, error) {
	run, err := r.queries.GetJobRun(ctx, job)
	return one(run, err, "job run")
}

type LockedReclaim = dbgen.LockUserReclaimRow

func (r Users) SetReclaimDeadline(ctx context.Context, id domain.UserID, until time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.SetUserReclaimDeadline(ctx, dbgen.SetUserReclaimDeadlineParams{ReclaimUntil: until, Now: now, ID: id})
	return applied(rows, err, "set reclaim deadline")
}

func (r Users) ClearReclaimDeadline(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.ClearUserReclaimDeadline(ctx, dbgen.ClearUserReclaimDeadlineParams{Now: now, ID: id})
	return applied(rows, err, "clear reclaim deadline")
}

type ReclaimDue = dbgen.ListReclaimsDueRow

func (r Users) ReclaimsDue(ctx context.Context, now time.Time, after *ReclaimDue, batchSize int32) ([]ReclaimDue, error) {
	params := dbgen.ListReclaimsDueParams{Now: now, BatchSize: batchSize}
	if after != nil {
		params.AfterUntil = &after.ReclaimUntil
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	due, err := r.queries.ListReclaimsDue(ctx, params)
	return one(due, err, "list reclaims due")
}

func (r Users) LockReclaim(ctx context.Context, id domain.UserID) (LockedReclaim, error) {
	locked, err := r.queries.LockUserReclaim(ctx, id)
	return one(locked, err, "lock user reclaim deadline")
}

func (r TutorProfiles) LockAll(ctx context.Context, tutors []domain.UserID) error {
	_, err := r.queries.LockTutorProfiles(ctx, uuids(tutors))
	return done(err, "lock tutor profiles")
}

func (r Users) EarliestPurgeDue(ctx context.Context, ids []domain.UserID, retention time.Duration) (*time.Time, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deletedAt, err := r.queries.EarliestDeletionAmong(ctx, uuids(ids))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("earliest purge due among accounts: %w", err)
	}
	if deletedAt == nil {
		return nil, nil
	}
	due := deletedAt.Add(retention)
	return &due, nil
}

func (r Users) ConfirmedAmong(ctx context.Context, ids []domain.UserID) ([]domain.UserID, error) {
	confirmed, err := r.queries.ListConfirmedUsers(ctx, uuids(ids))
	return one(confirmed, err, "confirmed accounts")
}

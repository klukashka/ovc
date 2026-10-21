package repositories

import (
	"context"
	"fmt"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	User              = dbgen.User
	NewUser           = dbgen.CreateUserParams
	TutorProfile      = dbgen.TutorProfile
	NewTutorProfile   = dbgen.CreateTutorProfileParams
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
	PasswordRehash    = dbgen.UpdateUserPasswordHashParams
	LockedCredentials = dbgen.LockUserCredentialsRow
)

type Users struct {
	queries *dbgen.Queries
}

func (r Users) Create(ctx context.Context, user NewUser) error {
	return done(r.queries.CreateUser(ctx, user), "create user")
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

func (r TutorProfiles) Create(ctx context.Context, profile NewTutorProfile) error {
	return done(r.queries.CreateTutorProfile(ctx, profile), "create tutor profile")
}

func (r TutorProfiles) ByUser(ctx context.Context, id domain.UserID) (TutorProfile, error) {
	profile, err := r.queries.GetTutorProfile(ctx, id)
	return one(profile, err, "tutor profile")
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

type EmailTokens struct {
	queries *dbgen.Queries
}

func (r EmailTokens) CreateRequest(ctx context.Context, request NewEmailRequest) error {
	return done(r.queries.CreateEmailRequest(ctx, request), "create email request")
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
	var deleted int64
	for range maxBatches {
		rows, err := r.queries.DeleteExpiredRateLimitCounters(ctx, dbgen.DeleteExpiredRateLimitCountersParams{Before: before, BatchSize: batchSize})
		if err != nil {
			return deleted, fmt.Errorf("delete expired rate limit counters: %w", err)
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

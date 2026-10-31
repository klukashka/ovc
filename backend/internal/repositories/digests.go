package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type DigestCandidate = dbgen.DigestPending
type DigestBounds = dbgen.ListDigestBoundsRow

type DigestCursor struct {
	Since time.Time
	User  domain.UserID
}

type DigestRange struct {
	Channel domain.ChannelID
	After   int64
	Through int64
}

type DigestDispatchChannel = dbgen.ListDigestChannelsAtDispatchRow

type Digests struct {
	queries *dbgen.Queries
}

func (r Digests) Candidates(ctx context.Context, now time.Time, after *DigestCursor, batch int32) ([]DigestCandidate, error) {
	params := dbgen.ListDigestCandidatesParams{Now: now, BatchSize: batch}
	if after != nil {
		params.AfterSince = &after.Since
		params.AfterUser = pgtype.UUID{Bytes: after.User, Valid: true}
	}
	rows, err := r.queries.ListDigestCandidates(ctx, params)
	return one(rows, err, "list digest candidates")
}

func (r Digests) Revision(ctx context.Context, user domain.UserID) (domain.DigestRevision, error) {
	revision, err := r.queries.GetDigestRevision(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return one(revision, err, "read digest revision")
}

func (r Digests) Channels(ctx context.Context, user domain.UserID) ([]domain.ChannelID, error) {
	rows, err := r.queries.ListDigestChannels(ctx, user)
	return one(rows, err, "list digest channels")
}

func (r Digests) LockChannels(ctx context.Context, channels []domain.ChannelID) error {
	_, err := r.queries.ShareDigestChannels(ctx, uuids(channels))
	return done(err, "share digest channels")
}

func (r Digests) LockMembers(ctx context.Context, user domain.UserID, channels []domain.ChannelID) error {
	_, err := r.queries.LockDigestMembers(ctx, dbgen.LockDigestMembersParams{UserID: pgtype.UUID{Bytes: user, Valid: true}, ChannelIds: uuids(channels)})
	return done(err, "lock digest members")
}

func (r Digests) LockState(ctx context.Context, user domain.UserID) error {
	if err := r.queries.EnsureDigestState(ctx, user); err != nil {
		return done(err, "initialize digest state")
	}
	_, err := r.queries.LockDigestState(ctx, user)
	return done(err, "lock digest state")
}

func (r Digests) LockPending(ctx context.Context, user domain.UserID) (domain.DigestRevision, error) {
	row, err := r.queries.LockDigestPending(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return one(row.Revision, err, "lock digest pending")
}

func (r Digests) Eligible(ctx context.Context, user domain.UserID, now time.Time) (bool, error) {
	eligible, err := r.queries.DigestEligible(ctx, dbgen.DigestEligibleParams{UserID: user, Now: now})
	return one(eligible, err, "check digest eligibility")
}

func (r Digests) LastSeen(ctx context.Context, user domain.UserID) (*time.Time, error) {
	seen, err := r.queries.GetDigestLastSeen(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, done(err, "read digest last seen")
	}
	return &seen, nil
}

func (r Digests) Bounds(ctx context.Context, user domain.UserID, channels []domain.ChannelID, seen *time.Time) ([]DigestBounds, error) {
	rows, err := r.queries.ListDigestBounds(ctx, dbgen.ListDigestBoundsParams{UserID: user, ChannelIds: uuids(channels), AfterSeen: seen})
	return one(rows, err, "read digest bounds")
}

func (r Digests) Advance(ctx context.Context, user domain.UserID, bound DigestBounds) error {
	return done(r.queries.AdvanceDigestedMarker(ctx, dbgen.AdvanceDigestedMarkerParams{UserID: pgtype.UUID{Bytes: user, Valid: true}, ChannelID: pgtype.UUID{Bytes: bound.ChannelID, Valid: true}, ThroughSeq: bound.ThroughSeq}), "advance digest marker")
}

func (r Digests) Record(ctx context.Context, user domain.UserID, now time.Time) error {
	return done(r.queries.RecordDigestTime(ctx, dbgen.RecordDigestTimeParams{UserID: user, Now: now}), "record digest time")
}

func (r Digests) Clear(ctx context.Context, user domain.UserID, revision domain.DigestRevision) error {
	_, err := r.queries.DeleteDigestRevision(ctx, dbgen.DeleteDigestRevisionParams{UserID: pgtype.UUID{Bytes: user, Valid: true}, Revision: revision})
	return done(err, "clear digest revision")
}

func (r Digests) ClearRead(ctx context.Context, user domain.UserID, revision domain.DigestRevision) error {
	_, err := r.queries.ClearReadDigestRevision(ctx, dbgen.ClearReadDigestRevisionParams{UserID: pgtype.UUID{Bytes: user, Valid: true}, Revision: revision})
	return done(err, "clear read digest revision")
}

func (r Digests) AtDispatch(ctx context.Context, user domain.UserID, bounds []DigestRange, seen *time.Time) ([]DigestDispatchChannel, error) {
	params := dbgen.ListDigestChannelsAtDispatchParams{
		UserID: pgtype.UUID{Bytes: user, Valid: true}, AfterSeen: seen, ChannelLimit: domain.DigestChannelLimit,
		ChannelIds: make([]pgtype.UUID, 0, len(bounds)), AfterSeqs: make([]int64, 0, len(bounds)), ThroughSeqs: make([]int64, 0, len(bounds)),
	}
	for _, bound := range bounds {
		params.ChannelIds = append(params.ChannelIds, pgtype.UUID{Bytes: bound.Channel, Valid: true})
		params.AfterSeqs = append(params.AfterSeqs, bound.After)
		params.ThroughSeqs = append(params.ThroughSeqs, bound.Through)
	}
	rows, err := r.queries.ListDigestChannelsAtDispatch(ctx, params)
	return one(rows, err, "read digest channels at dispatch")
}

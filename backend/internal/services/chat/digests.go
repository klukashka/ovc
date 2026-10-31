package chat

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var errDigestChanged = errors.New("digest state changed while channels were locked")

type DigestSettings struct {
	Wall   func() time.Time
	Budget time.Duration
	Batch  int32
}

func DefaultDigestSettings() DigestSettings {
	return DigestSettings{Wall: clock.System{}.Now, Budget: 20 * time.Second, Batch: 200}
}

func (s *Service) QueueDigests(ctx context.Context, now time.Time) (int64, error) {
	settings := s.deps.Digest
	deadline := settings.Wall().Add(settings.Budget)
	bounded, cancel := context.WithTimeout(ctx, settings.Budget)
	defer cancel()
	var processed int64
	var queuedMembers, queuedChannels int64
	defer func() {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "digest.queued", slog.Int64("members", queuedMembers), slog.Int64("channels", queuedChannels))
	}()
	var failures []error
	var cursor *repositories.DigestCursor
	for settings.Wall().Before(deadline) {
		candidates, err := s.deps.Store.Repositories().Digests.Candidates(bounded, now, cursor, settings.Batch)
		if err != nil {
			return processed, errors.Join(append(failures, err)...)
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			if !settings.Wall().Before(deadline) {
				return processed, errors.Join(failures...)
			}
			done, channels, err := s.queueDigest(bounded, candidate, now)
			if err != nil {
				failures = append(failures, err)
			}
			if done {
				processed++
				if channels > 0 {
					queuedMembers++
					queuedChannels += channels
				}
			}
			cursor = &repositories.DigestCursor{Since: candidate.PendingSince, User: candidate.UserID}
			if bounded.Err() != nil {
				return processed, errors.Join(append(failures, bounded.Err())...)
			}
		}
	}
	return processed, errors.Join(failures...)
}

func (s *Service) queueDigest(ctx context.Context, candidate repositories.DigestCandidate, now time.Time) (bool, int64, error) {
	processed := false
	var queuedChannels int64
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		found, err := repos.Presence.LockUsers(ctx, []domain.UserID{candidate.UserID})
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return nil
		}
		channels, err := repos.Digests.Channels(ctx, candidate.UserID)
		if err != nil {
			return err
		}
		if err := repos.Digests.LockChannels(ctx, channels); err != nil {
			return err
		}
		if err := repos.Digests.LockMembers(ctx, candidate.UserID, channels); err != nil {
			return err
		}
		if err := repos.Digests.LockState(ctx, candidate.UserID); err != nil {
			return err
		}
		revision, err := repos.Digests.LockPending(ctx, candidate.UserID)
		if err != nil {
			return err
		}
		if revision == 0 || revision != candidate.Revision {
			return errDigestChanged
		}
		current, err := repos.Digests.Channels(ctx, candidate.UserID)
		if err != nil {
			return err
		}
		if !slices.Equal(channels, current) {
			return errDigestChanged
		}
		eligible, err := repos.Digests.Eligible(ctx, candidate.UserID, now)
		if err != nil {
			return err
		}
		if !eligible {
			return nil
		}
		seen, err := repos.Digests.LastSeen(ctx, candidate.UserID)
		if err != nil {
			return err
		}
		bounds, err := repos.Digests.Bounds(ctx, candidate.UserID, channels, seen)
		if err != nil {
			return err
		}
		if err := s.enqueueDigest(ctx, repos, candidate.UserID, bounds, seen, now); err != nil {
			return err
		}
		if err := repos.Digests.Clear(ctx, candidate.UserID, revision); err != nil {
			return err
		}
		for _, bound := range bounds {
			if bound.MessageCount > 0 {
				queuedChannels++
			}
		}
		processed = true
		return nil
	})
	if errors.Is(err, errDigestChanged) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return processed, queuedChannels, nil
}

func (s *Service) enqueueDigest(ctx context.Context, repos repositories.Repositories, user domain.UserID, bounds []repositories.DigestBounds, seen *time.Time, now time.Time) error {
	payload := domain.DigestPayload{Recipient: hex.EncodeToString(user[:]), AfterSeen: seen}
	for _, bound := range bounds {
		if bound.MessageCount > 0 {
			payload.Channels = append(payload.Channels, domain.DigestChannelBounds{Channel: hex.EncodeToString(bound.ChannelID[:]), AfterSeq: bound.AfterSeq, ThroughSeq: bound.ThroughSeq})
		}
	}
	if len(payload.Channels) == 0 {
		return nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode message digest: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: domain.EventMessagesDigest, Payload: encoded,
		IdempotencyKey: fmt.Sprintf("%s:%s:%d", domain.EventMessagesDigest, payload.Recipient, now.UnixMicro()), NextAttemptAt: now, CreatedAt: now,
	})
	if err != nil {
		return err
	}
	for _, bound := range bounds {
		if err := repos.Digests.Advance(ctx, user, bound); err != nil {
			return err
		}
	}
	return repos.Digests.Record(ctx, user, now)
}

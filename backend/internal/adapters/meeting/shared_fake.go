package meeting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/domain"
)

type SharedFake struct {
	pool         *pgxpool.Pool
	clock        Clock
	random       io.Reader
	entropyMutex sync.Mutex
	logger       *slog.Logger
	limits       FakeLimits
}

type sharedState struct {
	namespace       []byte
	limits          FakeLimits
	generations     int64
	lastCreateTime  domain.ProviderCreateTime
	linkSequence    int64
	unavailable     bool
	generationLimit int64
}

const sharedStateColumns = `namespace, max_rooms, max_links, max_participants, max_generations, generations, last_create_time, link_sequence, unavailable`

func NewSharedFake(ctx context.Context, pool *pgxpool.Pool, clock Clock, random io.Reader, logger *slog.Logger, limits FakeLimits) (*SharedFake, error) {
	if pool == nil || clock == nil || random == nil || logger == nil || limits.Meetings <= 0 || limits.Links <= 0 || limits.Participants <= 0 || limits.Generations == 0 || limits.Generations > math.MaxInt64 {
		return nil, FailureInvalidInput
	}
	maximumGenerations := int64(limits.Generations)
	provider := &SharedFake{pool: pool, clock: clock, random: random, logger: logger, limits: limits}
	existing, err := scanSharedState(pool.QueryRow(ctx, `SELECT `+sharedStateColumns+` FROM fake_meeting_state WHERE id`))
	if err == nil {
		if existing.limits != limits {
			return nil, FailureConflict
		}
		return provider, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, sharedFailure(ctx, err)
	}
	namespace, entropyErr := provider.entropy()
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if entropyErr == nil {
			if _, err := tx.Exec(ctx, `INSERT INTO fake_meeting_state (namespace, max_rooms, max_links, max_participants, max_generations) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`, namespace[:], limits.Meetings, limits.Links, limits.Participants, maximumGenerations); err != nil {
				return err
			}
		}
		state, err := scanSharedState(tx.QueryRow(ctx, `SELECT `+sharedStateColumns+` FROM fake_meeting_state WHERE id FOR UPDATE`))
		if errors.Is(err, pgx.ErrNoRows) && entropyErr != nil {
			return FailureRandomness
		}
		if err != nil {
			return err
		}
		if state.limits != limits {
			return FailureConflict
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, sharedFailure(ctx, err)
	}
	return provider, nil
}

func scanSharedState(row pgx.Row) (sharedState, error) {
	var state sharedState
	var generations int64
	err := row.Scan(&state.namespace, &state.limits.Meetings, &state.limits.Links, &state.limits.Participants, &generations, &state.generations, &state.lastCreateTime, &state.linkSequence, &state.unavailable)
	if err != nil {
		return state, err
	}
	if generations <= 0 {
		return state, FailureDatabase
	}
	state.generationLimit = generations
	state.limits.Generations = uint64(generations)
	return state, nil
}

func sharedFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var failure Failure
	if errors.As(err, &failure) {
		return failure
	}
	return FailureDatabase
}

func (f *SharedFake) entropy() ([32]byte, error) {
	f.entropyMutex.Lock()
	defer f.entropyMutex.Unlock()
	var value [32]byte
	_, err := io.ReadFull(f.random, value[:])
	return value, err
}

func (f *SharedFake) transaction(ctx context.Context, op operation, work func(pgx.Tx, sharedState) error) (err error) {
	defer reportProvider(ctx, f.logger, op, &err)
	err = pgx.BeginFunc(ctx, f.pool, func(tx pgx.Tx) error {
		state, err := scanSharedState(tx.QueryRow(ctx, `SELECT `+sharedStateColumns+` FROM fake_meeting_state WHERE id FOR UPDATE`))
		if err != nil {
			return err
		}
		if state.unavailable && op != operationAvailability {
			return FailureUnavailable
		}
		if err := work(tx, state); err != nil {
			return err
		}
		return ctx.Err()
	})
	return sharedFailure(ctx, err)
}

func sharedRoom(ctx context.Context, tx pgx.Tx, requested domain.Meeting) (domain.Meeting, error) {
	if requested.ID == (domain.ProviderMeetingID{}) {
		return domain.Meeting{}, FailureInvalidInput
	}
	room := domain.Meeting{ID: requested.ID}
	err := tx.QueryRow(ctx, `SELECT name, create_time FROM fake_meeting_rooms WHERE id = $1`, [16]byte(requested.ID)).Scan(&room.Name, &room.CreateTime)
	if errors.Is(err, pgx.ErrNoRows) {
		return room, FailureNotFound
	}
	if err != nil {
		return room, err
	}
	return room, nil
}

func currentSharedRoom(ctx context.Context, tx pgx.Tx, requested domain.Meeting) error {
	room, err := sharedRoom(ctx, tx, requested)
	if err != nil {
		return err
	}
	if room.CreateTime != requested.CreateTime {
		return FailureNotFound
	}
	return nil
}

func (f *SharedFake) CreateMeeting(ctx context.Context, requested domain.Meeting) (domain.ProviderCreateTime, error) {
	instant, clockErr := f.clock.Instant(ctx)
	var created domain.ProviderCreateTime
	err := f.transaction(ctx, operationCreate, func(tx pgx.Tx, state sharedState) error {
		if requested.ID == (domain.ProviderMeetingID{}) || !validName(requested.Name, domain.LessonTitleMaxRunes) {
			return FailureInvalidInput
		}
		room, err := sharedRoom(ctx, tx, requested)
		if err == nil {
			if room.Name != requested.Name || (requested.CreateTime != 0 && requested.CreateTime != room.CreateTime) {
				return FailureConflict
			}
			created = room.CreateTime
			return nil
		}
		if !errors.Is(err, FailureNotFound) {
			return err
		}
		if requested.CreateTime != 0 {
			return FailureInvalidInput
		}
		var rooms int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fake_meeting_rooms`).Scan(&rooms); err != nil {
			return err
		}
		if rooms >= state.limits.Meetings || state.generations >= state.generationLimit || state.lastCreateTime == math.MaxInt64 {
			return FailureCapacity
		}
		if clockErr != nil || instant.Year() < 1970 || instant.Year() > 9999 {
			return FailureClock
		}
		created = max(domain.ProviderCreateTime(instant.UnixMilli()), state.lastCreateTime+1)
		if _, err := tx.Exec(ctx, `INSERT INTO fake_meeting_rooms (id, name, create_time) VALUES ($1,$2,$3)`, [16]byte(requested.ID), requested.Name, created); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE fake_meeting_state SET generations = generations + 1, last_create_time = $1 WHERE id`, created)
		return err
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

func (f *SharedFake) MeetingInfo(ctx context.Context, requested domain.Meeting) (domain.MeetingInfo, error) {
	var info domain.MeetingInfo
	err := f.transaction(ctx, operationInfo, func(tx pgx.Tx, _ sharedState) error {
		err := currentSharedRoom(ctx, tx, requested)
		if errors.Is(err, FailureNotFound) {
			info.Status = domain.MeetingNotFound
			return nil
		}
		if err != nil {
			return err
		}
		info.Status, info.CreateTime = domain.MeetingCreated, requested.CreateTime
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fake_meeting_participants WHERE room_id = $1 AND present`, [16]byte(requested.ID)).Scan(&info.Participants); err != nil {
			return err
		}
		if info.Participants > 0 {
			info.Status = domain.MeetingRunning
		}
		return nil
	})
	if err != nil {
		return domain.MeetingInfo{}, err
	}
	return info, nil
}

func (f *SharedFake) InspectMeeting(ctx context.Context, id domain.ProviderMeetingID) (domain.MeetingInfo, error) {
	var info domain.MeetingInfo
	err := f.transaction(ctx, operationInspect, func(tx pgx.Tx, _ sharedState) error {
		room, err := sharedRoom(ctx, tx, domain.Meeting{ID: id})
		if errors.Is(err, FailureNotFound) {
			info.Status = domain.MeetingNotFound
			return nil
		}
		if err != nil {
			return err
		}
		info.Status, info.CreateTime = domain.MeetingCreated, room.CreateTime
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fake_meeting_participants WHERE room_id = $1 AND present`, [16]byte(id)).Scan(&info.Participants); err != nil {
			return err
		}
		if info.Participants > 0 {
			info.Status = domain.MeetingRunning
		}
		return nil
	})
	if err != nil {
		return domain.MeetingInfo{}, err
	}
	return info, nil
}

func (f *SharedFake) EndMeeting(ctx context.Context, requested domain.Meeting) error {
	return f.transaction(ctx, operationEnd, func(tx pgx.Tx, _ sharedState) error {
		room, err := sharedRoom(ctx, tx, requested)
		if errors.Is(err, FailureNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if room.CreateTime != requested.CreateTime {
			return FailureNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM fake_meeting_rooms WHERE id = $1`, [16]byte(requested.ID))
		return err
	})
}

func (f *SharedFake) SimulateUnavailable(ctx context.Context, unavailable bool) error {
	return f.transaction(ctx, operationAvailability, func(tx pgx.Tx, _ sharedState) error {
		_, err := tx.Exec(ctx, `UPDATE fake_meeting_state SET unavailable = $1 WHERE id`, unavailable)
		return err
	})
}

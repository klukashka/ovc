package meeting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/domain"
)

const sharedLinkPrefix = "ovc-shared-fake://join/"

func (f *SharedFake) JoinLink(ctx context.Context, requested domain.Meeting, participant domain.Participant) (JoinLink, error) {
	nonce, entropyErr := f.entropy()
	var link JoinLink
	err := f.transaction(ctx, operationJoin, func(tx pgx.Tx, state sharedState) error {
		if err := currentSharedRoom(ctx, tx, requested); err != nil {
			return err
		}
		if !validParticipant(participant) {
			return FailureInvalidInput
		}
		var role domain.ParticipantRole
		err := tx.QueryRow(ctx, `SELECT role FROM fake_meeting_participants WHERE room_id = $1 AND participant_id = $2`, [16]byte(requested.ID), [16]byte(participant.ID)).Scan(&role)
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if exists && role != participant.Role {
			return FailureConflict
		}
		var links, participants int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fake_meeting_links`).Scan(&links); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fake_meeting_participants WHERE room_id = $1`, [16]byte(requested.ID)).Scan(&participants); err != nil {
			return err
		}
		if links >= state.limits.Links || state.linkSequence == math.MaxInt64 || (!exists && participants >= state.limits.Participants) {
			return FailureCapacity
		}
		if entropyErr != nil {
			return FailureRandomness
		}
		address := sharedLinkPrefix + hex.EncodeToString(state.namespace) + "/" + strconv.FormatInt(state.linkSequence+1, 10) + "/" + hex.EncodeToString(nonce[:])
		digest := sha256.Sum256([]byte(address))
		if !exists {
			if _, err := tx.Exec(ctx, `INSERT INTO fake_meeting_participants (room_id, create_time, participant_id, role) VALUES ($1,$2,$3,$4)`, [16]byte(requested.ID), requested.CreateTime, [16]byte(participant.ID), participant.Role); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fake_meeting_links (digest, room_id, create_time, participant_id, role) VALUES ($1,$2,$3,$4,$5)`, digest[:], [16]byte(requested.ID), requested.CreateTime, [16]byte(participant.ID), participant.Role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE fake_meeting_state SET link_sequence = link_sequence + 1 WHERE id`); err != nil {
			return err
		}
		link = NewJoinLink(address)
		return nil
	})
	if err != nil {
		return JoinLink{}, err
	}
	return link, nil
}

func canonicalSharedLink(address string, namespace []byte) bool {
	if len(address) < len(sharedLinkPrefix)+131 || len(address) > len(sharedLinkPrefix)+149 || !strings.HasPrefix(address, sharedLinkPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(address, sharedLinkPrefix), "/")
	if len(parts) != 3 || parts[0] != hex.EncodeToString(namespace) || len(parts[2]) != 64 {
		return false
	}
	sequence, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || sequence <= 0 || strconv.FormatInt(sequence, 10) != parts[1] {
		return false
	}
	nonce, err := hex.DecodeString(parts[2])
	return err == nil && hex.EncodeToString(nonce) == parts[2]
}

func (f *SharedFake) Enter(ctx context.Context, link JoinLink, requested domain.Meeting, participant domain.Participant) error {
	return f.transaction(ctx, operationEnter, func(tx pgx.Tx, state sharedState) error {
		if err := currentSharedRoom(ctx, tx, requested); err != nil {
			return err
		}
		if !validParticipant(participant) || !canonicalSharedLink(link.Reveal(), state.namespace) {
			return FailureInvalidInput
		}
		digest := sha256.Sum256([]byte(link.Reveal()))
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fake_meeting_links WHERE digest = $1 AND room_id = $2 AND create_time = $3 AND participant_id = $4 AND role = $5)`, digest[:], [16]byte(requested.ID), requested.CreateTime, [16]byte(participant.ID), participant.Role).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return FailureInvalidInput
		}
		_, err := tx.Exec(ctx, `UPDATE fake_meeting_participants SET present = true WHERE room_id = $1 AND participant_id = $2`, [16]byte(requested.ID), [16]byte(participant.ID))
		return err
	})
}

func (f *SharedFake) Leave(ctx context.Context, requested domain.Meeting, participant domain.ParticipantID) error {
	return f.transaction(ctx, operationLeave, func(tx pgx.Tx, _ sharedState) error {
		if err := currentSharedRoom(ctx, tx, requested); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE fake_meeting_participants SET present = false WHERE room_id = $1 AND participant_id = $2`, [16]byte(requested.ID), [16]byte(participant))
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return FailureNotFound
		}
		return nil
	})
}

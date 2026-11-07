package meeting

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const FakeRoomPath = "/__test__/fake-room"

type FakeRoomStatus string

const (
	FakeRoomAbsent      FakeRoomStatus = "absent"
	FakeRoomUnavailable FakeRoomStatus = "unavailable"
	FakeRoomEntered     FakeRoomStatus = "entered"
	FakeRoomLeft        FakeRoomStatus = "left"
)

type FakeRoomView struct {
	Status          FakeRoomStatus
	RoomName        string
	ParticipantName string
	Role            domain.ParticipantRole
}

type fakeRoomBinding struct {
	room        domain.ProviderMeetingID
	participant domain.ParticipantID
	view        FakeRoomView
}

type NavigableFake struct {
	*SharedFake
	origin string
}

func NewNavigableFake(fake *SharedFake, origin string) (*NavigableFake, error) {
	if fake == nil || !core.IsFakeRoomOrigin(origin) {
		return nil, FailureInvalidInput
	}
	return &NavigableFake{SharedFake: fake, origin: origin}, nil
}

func (n *NavigableFake) JoinLink(ctx context.Context, requested domain.Meeting, participant domain.Participant) (JoinLink, error) {
	link, err := n.SharedFake.JoinLink(ctx, requested, participant)
	if err != nil {
		return JoinLink{}, err
	}
	return NewJoinLink(n.origin + FakeRoomPath + "#" + strings.TrimPrefix(link.Reveal(), sharedLinkPrefix)), nil
}

func (f *SharedFake) ViewRoom(ctx context.Context, capability string) (FakeRoomView, error) {
	return f.roomOperation(ctx, operationRoomView, capability, func(pgx.Tx, *fakeRoomBinding) error { return nil })
}

func (f *SharedFake) EnterRoom(ctx context.Context, capability string) (FakeRoomView, error) {
	return f.roomOperation(ctx, operationEnter, capability, func(tx pgx.Tx, binding *fakeRoomBinding) error {
		binding.view.Status = FakeRoomEntered
		return setRoomPresence(ctx, tx, binding, true)
	})
}

func (f *SharedFake) LeaveRoom(ctx context.Context, capability string) (FakeRoomView, error) {
	return f.roomOperation(ctx, operationLeave, capability, func(tx pgx.Tx, binding *fakeRoomBinding) error {
		binding.view.Status = FakeRoomLeft
		return setRoomPresence(ctx, tx, binding, false)
	})
}

func (f *SharedFake) EndRoom(ctx context.Context, capability string) (FakeRoomView, error) {
	return f.roomOperation(ctx, operationEnd, capability, func(tx pgx.Tx, binding *fakeRoomBinding) error {
		binding.view = FakeRoomView{Status: FakeRoomAbsent}
		_, err := tx.Exec(ctx, `DELETE FROM fake_meeting_rooms WHERE id = $1`, [16]byte(binding.room))
		return err
	})
}

func setRoomPresence(ctx context.Context, tx pgx.Tx, binding *fakeRoomBinding, present bool) error {
	_, err := tx.Exec(ctx, `UPDATE fake_meeting_participants SET present = $3 WHERE room_id = $1 AND participant_id = $2`, [16]byte(binding.room), [16]byte(binding.participant), present)
	return err
}

func (f *SharedFake) roomOperation(ctx context.Context, op operation, capability string, change func(pgx.Tx, *fakeRoomBinding) error) (FakeRoomView, error) {
	view := FakeRoomView{Status: FakeRoomAbsent}
	err := f.transaction(ctx, op, func(tx pgx.Tx, state sharedState) error {
		address := sharedLinkPrefix + capability
		if !canonicalSharedLink(address, state.namespace) {
			return nil
		}
		digest := sha256.Sum256([]byte(address))
		binding, found, err := findRoomBinding(ctx, tx, digest)
		if err != nil || !found {
			return err
		}
		if err := change(tx, &binding); err != nil {
			return err
		}
		view = binding.view
		return nil
	})
	if errors.Is(err, FailureUnavailable) {
		return FakeRoomView{Status: FakeRoomUnavailable}, nil
	}
	if err != nil {
		return FakeRoomView{}, err
	}
	return view, nil
}

func findRoomBinding(ctx context.Context, tx pgx.Tx, digest [32]byte) (fakeRoomBinding, bool, error) {
	var binding fakeRoomBinding
	var room, participant [16]byte
	var present bool
	err := tx.QueryRow(ctx, `SELECT rooms.id, participants.participant_id, rooms.name, participants.display_name, participants.role, participants.present
		FROM fake_meeting_links links
		JOIN fake_meeting_participants participants ON participants.room_id = links.room_id AND participants.create_time = links.create_time AND participants.participant_id = links.participant_id AND participants.role = links.role
		JOIN fake_meeting_rooms rooms ON rooms.id = links.room_id AND rooms.create_time = links.create_time
		WHERE links.digest = $1`, digest[:]).Scan(&room, &participant, &binding.view.RoomName, &binding.view.ParticipantName, &binding.view.Role, &present)
	if errors.Is(err, pgx.ErrNoRows) {
		return binding, false, nil
	}
	if err != nil {
		return binding, false, err
	}
	binding.room, binding.participant = domain.ProviderMeetingID(room), domain.ParticipantID(participant)
	binding.view.Status = FakeRoomLeft
	if present {
		binding.view.Status = FakeRoomEntered
	}
	return binding, true, nil
}

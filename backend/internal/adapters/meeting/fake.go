package meeting

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
)

type FakeLimits struct {
	Meetings     int
	Links        int
	Participants int
	Generations  uint64
}

type fakeParticipant struct {
	role    domain.ParticipantRole
	present bool
}

type fakeLink struct {
	participant domain.ParticipantID
	role        domain.ParticipantRole
	created     domain.ProviderCreateTime
}

type fakeMeeting struct {
	meeting      domain.Meeting
	participants map[domain.ParticipantID]fakeParticipant
	links        map[string]fakeLink
}

type Fake struct {
	issuer         *linkIssuer
	mutex          sync.Mutex
	clock          Clock
	random         io.Reader
	logger         *slog.Logger
	limits         FakeLimits
	meetings       map[domain.ProviderMeetingID]*fakeMeeting
	generations    uint64
	lastCreateTime domain.ProviderCreateTime
	linkSequence   uint64
	links          int
	unavailable    bool
}

func NewFake(clock Clock, random io.Reader, logger *slog.Logger, limits FakeLimits) (*Fake, error) {
	if clock == nil || random == nil || logger == nil || limits.Meetings <= 0 || limits.Links <= 0 || limits.Participants <= 0 || limits.Generations == 0 {
		return nil, FailureInvalidInput
	}
	return &Fake{issuer: &linkIssuer{marker: 1}, clock: clock, random: random, logger: logger, limits: limits, meetings: make(map[domain.ProviderMeetingID]*fakeMeeting)}, nil
}

type operation string

const (
	operationCreate       operation = "create"
	operationInfo         operation = "info"
	operationInspect      operation = "inspect"
	operationJoin         operation = "join_link"
	operationEnd          operation = "end"
	operationEnter        operation = "enter"
	operationLeave        operation = "leave"
	operationAvailability operation = "availability"
)

func (f *Fake) report(ctx context.Context, op operation, err *error) {
	reportProvider(ctx, f.logger, op, err)
}

func reportProvider(ctx context.Context, logger *slog.Logger, op operation, err *error) {
	if *err == nil {
		logger.LogAttrs(ctx, slog.LevelDebug, "meeting.provider.completed", slog.String("operation", string(op)), slog.String("outcome", "ok"))
		return
	}
	level := slog.LevelWarn
	kind := "failure"
	var failure Failure
	if errors.As(*err, &failure) {
		kind = string(failure)
	}
	if errors.Is(*err, context.Canceled) {
		level, kind = slog.LevelDebug, "cancelled"
	}
	if errors.Is(*err, context.DeadlineExceeded) {
		level, kind = slog.LevelDebug, "deadline"
	}
	logger.LogAttrs(ctx, level, "meeting.provider.failed", slog.String("operation", string(op)), slog.String("kind", kind))
}

func (f *Fake) available(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.unavailable {
		return FailureUnavailable
	}
	return nil
}

func validName(name string, maximum int) bool {
	return utf8.ValidString(name) && strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maximum && strings.IndexFunc(name, unicode.IsControl) < 0
}

func (f *Fake) CreateMeeting(ctx context.Context, requested domain.Meeting) (created domain.ProviderCreateTime, err error) {
	defer f.report(ctx, operationCreate, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return 0, err
	}
	if requested.ID == (domain.ProviderMeetingID{}) || !validName(requested.Name, domain.LessonTitleMaxRunes) {
		return 0, FailureInvalidInput
	}
	if existing, found := f.meetings[requested.ID]; found {
		if requested.Name != existing.meeting.Name || (requested.CreateTime != 0 && requested.CreateTime != existing.meeting.CreateTime) {
			return 0, FailureConflict
		}
		return existing.meeting.CreateTime, nil
	}
	if requested.CreateTime != 0 {
		return 0, FailureInvalidInput
	}
	if len(f.meetings) >= f.limits.Meetings || f.generations >= f.limits.Generations || f.lastCreateTime == math.MaxInt64 {
		return 0, FailureCapacity
	}
	instant, clockErr := f.clock.Instant(ctx)
	if clockErr != nil {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return 0, FailureClock
	}
	if instant.Year() < 1970 || instant.Year() > 9999 {
		return 0, FailureClock
	}
	created = max(domain.ProviderCreateTime(instant.UnixMilli()), f.lastCreateTime+1)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	requested.CreateTime = created
	f.meetings[requested.ID] = &fakeMeeting{meeting: requested, participants: make(map[domain.ParticipantID]fakeParticipant), links: make(map[string]fakeLink)}
	f.generations++
	f.lastCreateTime = created
	return created, nil
}

func (f *Fake) current(requested domain.Meeting) (*fakeMeeting, error) {
	if requested.ID == (domain.ProviderMeetingID{}) {
		return nil, FailureInvalidInput
	}
	room, found := f.meetings[requested.ID]
	if !found || room.meeting.CreateTime != requested.CreateTime {
		return nil, FailureNotFound
	}
	return room, nil
}

func (f *Fake) MeetingInfo(ctx context.Context, requested domain.Meeting) (info domain.MeetingInfo, err error) {
	defer f.report(ctx, operationInfo, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return info, err
	}
	room, err := f.current(requested)
	if errors.Is(err, FailureNotFound) {
		return domain.MeetingInfo{Status: domain.MeetingNotFound}, nil
	}
	if err != nil {
		return info, err
	}
	return fakeRoomInfo(room), nil
}

func (f *Fake) InspectMeeting(ctx context.Context, id domain.ProviderMeetingID) (info domain.MeetingInfo, err error) {
	defer f.report(ctx, operationInspect, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return info, err
	}
	if id == (domain.ProviderMeetingID{}) {
		return info, FailureInvalidInput
	}
	room, found := f.meetings[id]
	if !found {
		return domain.MeetingInfo{Status: domain.MeetingNotFound}, nil
	}
	return fakeRoomInfo(room), nil
}

func fakeRoomInfo(room *fakeMeeting) domain.MeetingInfo {
	info := domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: room.meeting.CreateTime}
	for _, participant := range room.participants {
		if participant.present {
			info.Participants++
		}
	}
	if info.Participants > 0 {
		info.Status = domain.MeetingRunning
	}
	return info
}

func validParticipant(participant domain.Participant) bool {
	return participant.ID != (domain.ParticipantID{}) && validName(participant.Name, domain.ParticipantNameMaxRunes) &&
		(participant.Role == domain.ParticipantModerator || participant.Role == domain.ParticipantViewer)
}

func (f *Fake) JoinLink(ctx context.Context, requested domain.Meeting, participant domain.Participant) (link JoinLink, err error) {
	defer f.report(ctx, operationJoin, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return link, err
	}
	room, err := f.current(requested)
	if err != nil {
		return link, err
	}
	if !validParticipant(participant) {
		return link, FailureInvalidInput
	}
	bound, exists := room.participants[participant.ID]
	if exists && bound.role != participant.Role {
		return link, FailureConflict
	}
	if f.links >= f.limits.Links || f.linkSequence == math.MaxUint64 || (!exists && len(room.participants) >= f.limits.Participants) {
		return link, FailureCapacity
	}
	var nonce [32]byte
	if _, randomErr := io.ReadFull(f.random, nonce[:]); randomErr != nil {
		if err := ctx.Err(); err != nil {
			return link, err
		}
		return link, FailureRandomness
	}
	address := "ovc-fake://join/" + strconv.FormatUint(f.linkSequence+1, 10) + "/" + hex.EncodeToString(nonce[:])
	if err := ctx.Err(); err != nil {
		return link, err
	}
	room.links[address] = fakeLink{participant: participant.ID, role: participant.Role, created: requested.CreateTime}
	if !exists {
		room.participants[participant.ID] = fakeParticipant{role: participant.Role}
	}
	f.linkSequence++
	f.links++
	link = NewJoinLink(address)
	link.issuer = f.issuer
	return link, nil
}

func (f *Fake) EndMeeting(ctx context.Context, requested domain.Meeting) (err error) {
	defer f.report(ctx, operationEnd, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return err
	}
	if requested.ID == (domain.ProviderMeetingID{}) {
		return FailureInvalidInput
	}
	room, found := f.meetings[requested.ID]
	if !found {
		return nil
	}
	if room.meeting.CreateTime != requested.CreateTime {
		return FailureNotFound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f.links -= len(room.links)
	delete(f.meetings, requested.ID)
	return nil
}

func (f *Fake) Enter(ctx context.Context, link JoinLink, requested domain.Meeting, participant domain.Participant) (err error) {
	defer f.report(ctx, operationEnter, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return err
	}
	room, err := f.current(requested)
	if err != nil {
		return err
	}
	binding, exists := room.links[link.Reveal()]
	if link.issuer != f.issuer || !exists || !validParticipant(participant) || binding.participant != participant.ID || binding.role != participant.Role || binding.created != requested.CreateTime {
		return FailureInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	room.participants[participant.ID] = fakeParticipant{role: participant.Role, present: true}
	return nil
}

func (f *Fake) Leave(ctx context.Context, requested domain.Meeting, participant domain.ParticipantID) (err error) {
	defer f.report(ctx, operationLeave, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := f.available(ctx); err != nil {
		return err
	}
	room, err := f.current(requested)
	if err != nil {
		return err
	}
	bound, found := room.participants[participant]
	if !found {
		return FailureNotFound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bound.present = false
	room.participants[participant] = bound
	return nil
}

func (f *Fake) SimulateUnavailable(ctx context.Context, unavailable bool) (err error) {
	defer f.report(ctx, operationAvailability, &err)
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.unavailable = unavailable
	return nil
}

package meeting

import (
	"context"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

type Provider interface {
	CreateMeeting(context.Context, domain.Meeting) (domain.ProviderCreateTime, error)
	InspectMeeting(context.Context, domain.ProviderMeetingID) (domain.MeetingInfo, error)
	MeetingInfo(context.Context, domain.Meeting) (domain.MeetingInfo, error)
	JoinLink(context.Context, domain.Meeting, domain.Participant) (JoinLink, error)
	EndMeeting(context.Context, domain.Meeting) error
}

type linkIssuer struct {
	marker byte
}

type JoinLink struct {
	core.Secret
	issuer *linkIssuer
}

func NewJoinLink(address string) JoinLink {
	return JoinLink{Secret: core.NewSecret(address)}
}

type Clock interface {
	Instant(context.Context) (time.Time, error)
}

type Failure string

const (
	FailureInvalidInput Failure = "invalid_input"
	FailureNotFound     Failure = "not_found"
	FailureConflict     Failure = "conflict"
	FailureCapacity     Failure = "capacity"
	FailureUnavailable  Failure = "unavailable"
	FailureDatabase     Failure = "database"
	FailureClock        Failure = "clock"
	FailureRandomness   Failure = "randomness"
)

func (failure Failure) Error() string {
	return "meeting provider: " + string(failure)
}

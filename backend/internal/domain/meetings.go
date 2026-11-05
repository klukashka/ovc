package domain

type ProviderMeetingID [16]byte

type ProviderCreateTime int64

type ParticipantID [16]byte

const ParticipantNameMaxRunes = 100

type ParticipantRole string

const (
	ParticipantModerator ParticipantRole = "moderator"
	ParticipantViewer    ParticipantRole = "viewer"
)

type MeetingStatus string

const (
	MeetingNotFound MeetingStatus = "not_found"
	MeetingCreated  MeetingStatus = "created"
	MeetingRunning  MeetingStatus = "running"
)

type Meeting struct {
	ID         ProviderMeetingID
	Name       string
	CreateTime ProviderCreateTime
}

type Participant struct {
	ID   ParticipantID
	Name string
	Role ParticipantRole
}

type MeetingInfo struct {
	Status       MeetingStatus
	CreateTime   ProviderCreateTime
	Participants int
}

type MeetingID [16]byte

type MeetingRoutingKey [16]byte

type MeetingProvider string

const (
	MeetingProviderFake MeetingProvider = "fake"
	MeetingProviderBBB  MeetingProvider = "bbb"
)

type MeetingState string

const (
	MeetingCreating MeetingState = "creating"
	MeetingOpen     MeetingState = "open"
	MeetingClosed   MeetingState = "closed"
)

type MeetingCloseReason string

const (
	MeetingCloseTutorEnd         MeetingCloseReason = "tutor_end"
	MeetingCloseTutorRestart     MeetingCloseReason = "tutor_restart"
	MeetingCloseStatusJob        MeetingCloseReason = "status_job"
	MeetingCloseProviderNotFound MeetingCloseReason = "provider_not_found"
	MeetingCloseProviderError    MeetingCloseReason = "provider_error"
)

type MeetingMutation string

const (
	MeetingMutationApplied MeetingMutation = "applied"
	MeetingMutationRefused MeetingMutation = "refused"
	MeetingAlreadyJoined   MeetingMutation = "already_joined"
	MeetingAlreadyClosed   MeetingMutation = "already_closed"
)

type MeetingIdentity struct {
	LessonID          LessonID
	ID                MeetingID
	ProviderMeetingID ProviderMeetingID
}

type MeetingExpectation struct {
	Identity   MeetingIdentity
	State      MeetingState
	CreateTime *ProviderCreateTime
}

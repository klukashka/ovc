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

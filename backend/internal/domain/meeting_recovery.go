package domain

type MeetingCreateAttemptID [16]byte

type MeetingRecoveryRevision int64

type MeetingRecoveryOrigin string

const (
	MeetingRecoveryAttempt MeetingRecoveryOrigin = "attempt"
	MeetingRecoveryLegacy  MeetingRecoveryOrigin = "legacy"
)

type MeetingCreateState string

const (
	MeetingCreatePrepared  MeetingCreateState = "prepared"
	MeetingCreateStarted   MeetingCreateState = "started"
	MeetingCreateReturned  MeetingCreateState = "returned"
	MeetingCreateUncertain MeetingCreateState = "uncertain"
)

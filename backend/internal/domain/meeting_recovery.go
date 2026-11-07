package domain

import "time"

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

type MeetingRecoveryCycleNumber int64

type MeetingRecoveryCycleMode string

const (
	MeetingRecoveryAutomatic MeetingRecoveryCycleMode = "automatic"
	MeetingRecoveryOperator  MeetingRecoveryCycleMode = "operator"
)

type MeetingRecoveryCycleResult string

const (
	MeetingRecoveryCycleNone       MeetingRecoveryCycleResult = "none"
	MeetingRecoveryCyclePending    MeetingRecoveryCycleResult = "pending"
	MeetingRecoveryCycleSettled    MeetingRecoveryCycleResult = "settled"
	MeetingRecoveryCycleUnresolved MeetingRecoveryCycleResult = "unresolved"
	MeetingRecoveryCycleConflict   MeetingRecoveryCycleResult = "conflict"
	MeetingRecoveryCycleExhausted  MeetingRecoveryCycleResult = "exhausted"
)

const KindMeetingCleanup OutboxKind = "meeting_cleanup"
const MeetingRecoveryCapacity = 4096

type MeetingRecoverySettlement string

const (
	MeetingRecoverySettledRevoked      MeetingRecoverySettlement = "revoked"
	MeetingRecoverySettledConfirmedEnd MeetingRecoverySettlement = "confirmed_end"
	MeetingRecoverySettledQuietAbsence MeetingRecoverySettlement = "quiet_absence"
)

const MeetingCreateQuietWindow = time.Hour

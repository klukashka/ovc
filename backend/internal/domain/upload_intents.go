package domain

import "time"

type UploadAttemptID [16]byte

func (id UploadAttemptID) String() string {
	return formatUUID(id)
}

type UploadPhase string

const (
	UploadOpening    UploadPhase = "opening"
	UploadStreaming  UploadPhase = "streaming"
	UploadCompleting UploadPhase = "completing"
	UploadCleanup    UploadPhase = "cleanup"
	UploadUncertain  UploadPhase = "uncertain"
)

type UploadCapacityClass string

const (
	UploadCapacityChat    UploadCapacityClass = "chat"
	UploadCapacityLibrary UploadCapacityClass = "library"
)

type UploadCleanupGeneration int16

const (
	UploadInitialCleanup UploadCleanupGeneration = 0
	UploadFinalCleanup   UploadCleanupGeneration = 1
)

type UploadCleanupResult string

const (
	UploadCleanupNone    UploadCleanupResult = "none"
	UploadCleanupQueued  UploadCleanupResult = "queued"
	UploadCleanupDeleted UploadCleanupResult = "deleted"
	UploadCleanupFailed  UploadCleanupResult = "failed"
)

const (
	UploadSettlementBudget                = 30 * time.Second
	UploadRecoveryBudget                  = 30 * time.Second
	UploadRecoveryCandidateBudget         = time.Second
	UploadIntentCapacity                  = 4096
	LibraryUploadIntentCapacity           = 1024
	JobUploadsRecover             JobName = "uploads.recover"
)

func (id OutboxID) String() string { return formatUUID(id) }

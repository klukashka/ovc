package domain

import (
	"fmt"
	"time"
)

type ExportID [16]byte

func (id ExportID) String() string { return formatUUID(id) }

type ExportBuildID [16]byte

func (id ExportBuildID) String() string { return formatUUID(id) }

func (id ExportBuildID) Identity() AttachmentID { return AttachmentID(id) }

type ExportState string

const (
	ExportQueued   ExportState = "queued"
	ExportBuilding ExportState = "building"
	ExportReady    ExportState = "ready"
	ExportFailed   ExportState = "failed"
	ExportExpired  ExportState = "expired"
)

type ExportFailure string

const (
	ExportFailureError     ExportFailure = "error"
	ExportFailureTooLong   ExportFailure = "too_long"
	ExportFailureTooLarge  ExportFailure = "too_large"
	ExportFailureWithdrawn ExportFailure = "withdrawn"
)

type ExportInvalidationCause string

const (
	ExportCauseAccountDeleted ExportInvalidationCause = "account_deleted"
	ExportCauseAccountBlocked ExportInvalidationCause = "account_blocked"
)

type UploadSubjectKind string

const (
	UploadSubjectAttachment  UploadSubjectKind = "attachment"
	UploadSubjectExportBuild UploadSubjectKind = "export_build"
)

const (
	UploadCapacityExport UploadCapacityClass = "export"

	ExportUploadIntentCapacity = 256
	ExportKeyPrefix            = "exports/"

	ExportStartInterval       = 24 * time.Hour
	ExportRequestWindow       = 24 * time.Hour
	ExportRequestLimit        = 5
	ExportRetention           = 3 * 24 * time.Hour
	ExportRowRetention        = 3 * 24 * time.Hour
	ExportBuildBudget         = 2 * time.Hour
	ExportRecoveryAge         = 2*time.Hour + 15*time.Minute
	ExportTableBytesAllowance = 200 << 20
	ExportPartBytes           = 16 << 20
	ExportProgressInterval    = 10 * time.Second
	ExportLaneSlots           = 2
	ExportLanePoll            = 5 * time.Second
	ExportCapacityRetryAfter  = 60 * time.Second
	ExportDownloadWindow      = time.Hour
	ExportDownloadLimit       = 30
	ExportRecoveryBatch       = 100
	ExportRecoveryBudget      = 20 * time.Second
	ExportExpireBatch         = 100
	ExportExpireRounds        = 10
	ExportFormatVersion       = 1

	JobExportsRecover JobName = "exports.recover"
	JobExportsExpire  JobName = "exports.expire"

	NotificationExportReady  NotificationEvent    = "export.ready"
	NotificationExportFailed NotificationEvent    = "export.failed"
	LinkMyData               NotificationLinkKind = "my_data"

	EventExportRequested OutboxKind = "export.requested"
	KindExportIntent     OutboxKind = "export_intent"
	KindExportObject     OutboxKind = "export_object"
)

type ExportIntentPayload struct {
	Build      string                   `json:"build_id"`
	Key        string                   `json:"storage_key"`
	Generation *UploadCleanupGeneration `json:"generation,omitempty"`
	Attempt    *UploadAttemptID         `json:"attempt,omitempty"`
}

type ExportObjectPayload struct {
	Export string `json:"export_id"`
	Build  string `json:"build_id"`
	Key    string `json:"storage_key"`
}

type ExportRequestedPayload struct {
	Export            string `json:"export"`
	User              string `json:"user"`
	RequestedAtMicros int64  `json:"requested_at_micros"`
}

func ExportIntentCleanupKey(build ExportBuildID, generation UploadCleanupGeneration) string {
	return fmt.Sprintf("storage_delete:export:%s:intent:%d", build, generation)
}

func ExportObjectDeletionKey(build ExportBuildID) string {
	return fmt.Sprintf("storage_delete:export:%s", build)
}

func ExportRequestedKey(export ExportID) string {
	return fmt.Sprintf("%s:%s", EventExportRequested, export)
}

func ExportNotificationKey(event NotificationEvent, export ExportID, build ExportBuildID) string {
	return fmt.Sprintf("%s:%s:%s", event, export, build)
}

func ExportStorageKey(build ExportBuildID) string {
	return ExportKeyPrefix + build.String() + ".zip"
}

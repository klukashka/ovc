package domain

type OutboxID [16]byte

type LeaseToken [16]byte

const OutboxMaxAttempts = 8

type OutboxKind string

type JobName string

type OutboxLane string

const (
	LaneEmail         OutboxLane = "email"
	LaneProviderCall  OutboxLane = "provider_call"
	LaneRecordingCopy OutboxLane = "recording_copy"
	LaneStorageDelete OutboxLane = "storage_delete"
)

type OutboxPriority int16

const (
	PriorityHigh   OutboxPriority = 0
	PriorityNormal OutboxPriority = 1
)

type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxInProgress OutboxStatus = "in_progress"
	OutboxDone       OutboxStatus = "done"
	OutboxDead       OutboxStatus = "dead"
)

type OutboxDoneReason string

const (
	DoneSent     OutboxDoneReason = "sent"
	DoneObsolete OutboxDoneReason = "obsolete"
)

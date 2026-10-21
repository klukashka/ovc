package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrPermanentEffect = errors.New("the effect can never succeed")

func PermanentEffect(cause error) error {
	return fmt.Errorf("%w: %w", ErrPermanentEffect, cause)
}

type OutboxID [16]byte

type LeaseToken [16]byte

const (
	OutboxMaxAttempts     = 8
	OutboxFirstRetryDelay = 30 * time.Second
	OutboxMaxRetryDelay   = time.Hour
)

func OutboxRetryDelay(attempts int32, fraction float64) time.Duration {
	delay := OutboxFirstRetryDelay
	for step := int32(1); step < attempts && delay < OutboxMaxRetryDelay; step++ {
		delay *= 2
	}
	jittered := delay + time.Duration(min(max(fraction, 0), 1)*float64(delay)/2)
	return min(jittered, OutboxMaxRetryDelay)
}

type OutboxKind string

type JobName string

const JobOutboxCleanup JobName = "outbox.cleanup"

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

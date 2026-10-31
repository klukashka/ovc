package domain

import "time"

type DigestRevision int64

const (
	EventMessagesDigest OutboxKind      = "messages.digest"
	JobMessagesDigest   JobName         = "messages.digest"
	DropObsoleteDigest  EmailDropReason = "obsolete_digest"
	DigestChannelLimit                  = 20
)

type DigestChannelBounds struct {
	Channel    string `json:"channel"`
	AfterSeq   int64  `json:"after_seq"`
	ThroughSeq int64  `json:"through_seq"`
}

type DigestPayload struct {
	Recipient string                `json:"recipient"`
	AfterSeen *time.Time            `json:"after_seen"`
	Channels  []DigestChannelBounds `json:"channels"`
}

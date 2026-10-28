package domain

import (
	"encoding/hex"
	"fmt"
)

const EventGroupArchived OutboxKind = "group.archived"

func GroupArchivedKey(group GroupID, archivedAtMicros int64, recipient UserID) string {
	return fmt.Sprintf("%s:%s:%d:%s", EventGroupArchived, hex.EncodeToString(group[:]), archivedAtMicros, hex.EncodeToString(recipient[:]))
}

type GroupArchivedPayload struct {
	Group            string `json:"group"`
	ArchivedAtMicros int64  `json:"archived_at_micros"`
	Recipient        string `json:"recipient"`
}

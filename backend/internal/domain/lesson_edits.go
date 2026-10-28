package domain

import (
	"encoding/hex"
	"fmt"
)

type EditScope string

const (
	ScopeThis      EditScope = "this"
	ScopeFollowing EditScope = "following"
)

type OperationID [16]byte

func (id OperationID) String() string {
	return formatUUID(id)
}

const (
	EventLessonMoved         OutboxKind = "lesson.moved"
	EventLessonCancelled     OutboxKind = "lesson.cancelled"
	EventLessonSeriesChanged OutboxKind = "lesson.series_changed"
)

const (
	SeriesChangeLessonLimit = 100
	ChangeEmailLessonLimit  = 20
)

func LessonCancelledKey(lesson LessonID, recipient UserID) string {
	return fmt.Sprintf("%s:%s:%s", EventLessonCancelled, hex.EncodeToString(lesson[:]), hex.EncodeToString(recipient[:]))
}

func SeriesChangedKey(operation OperationID, recipient UserID) string {
	return fmt.Sprintf("%s:%s:%s", EventLessonSeriesChanged, hex.EncodeToString(operation[:]), hex.EncodeToString(recipient[:]))
}

type LessonCancelledPayload struct {
	Lesson    string `json:"lesson"`
	Recipient string `json:"recipient"`
}

type SeriesChangedPayload struct {
	Operation string   `json:"operation"`
	Tutor     string   `json:"tutor"`
	Subject   string   `json:"subject"`
	Lessons   []string `json:"lessons"`
	Recipient string   `json:"recipient"`
}

package domain

import (
	"encoding/hex"
	"fmt"
	"time"
)

type NotificationEvent string

const (
	NotificationInvitationAccepted  NotificationEvent = "invitation.accepted"
	NotificationJoinRequestCreated  NotificationEvent = "join_request.created"
	NotificationJoinRequestAccepted NotificationEvent = "join_request.accepted"
	NotificationJoinRequestDeclined NotificationEvent = "join_request.declined"
	NotificationRelationshipEnded   NotificationEvent = "relationship.ended"
	NotificationLessonReminder1h    NotificationEvent = "lesson.reminder_1h"
	NotificationLessonMoved         NotificationEvent = "lesson.moved"
	NotificationLessonCancelled     NotificationEvent = "lesson.cancelled"
	NotificationSeriesChanged       NotificationEvent = "lesson.series_changed"
	NotificationGroupArchived       NotificationEvent = "group.archived"
)

func NotificationEvents() []NotificationEvent {
	return []NotificationEvent{
		NotificationInvitationAccepted, NotificationJoinRequestCreated, NotificationJoinRequestAccepted,
		NotificationJoinRequestDeclined, NotificationRelationshipEnded, NotificationLessonReminder1h, NotificationLessonMoved,
		NotificationLessonCancelled, NotificationSeriesChanged, NotificationGroupArchived,
	}
}

type NotificationLinkKind string

const (
	LinkStudentCard  NotificationLinkKind = "student_card"
	LinkJoinRequests NotificationLinkKind = "join_requests"
	LinkMyTutors     NotificationLinkKind = "my_tutors"
)

const (
	NotificationPageDefault = 30
	NotificationPageMaximum = 100
	NotificationUnreadCap   = 100
	NotificationReadMaximum = 100
	NotificationRetention   = 90 * 24 * time.Hour
)

func InvitationAcceptedKey(invitation InvitationID, tutor UserID) string {
	return fmt.Sprintf("%s:%s:%s", NotificationInvitationAccepted, invitation, tutor)
}

func JoinRequestCreatedKey(request JoinRequestID, tutor UserID) string {
	return fmt.Sprintf("%s:%s:%s", NotificationJoinRequestCreated, request, tutor)
}

func JoinRequestAcceptedKey(request JoinRequestID, student UserID) string {
	return fmt.Sprintf("%s:%s:%s", NotificationJoinRequestAccepted, request, student)
}

func JoinRequestDeclinedKey(request JoinRequestID, student UserID) string {
	return fmt.Sprintf("%s:%s:%s", NotificationJoinRequestDeclined, request, student)
}

func RelationshipEndedKey(relationship RelationshipID, endedAtMicros int64) string {
	return fmt.Sprintf("%s:%s:%d", EventRelationshipEnded, hex.EncodeToString(relationship[:]), endedAtMicros)
}

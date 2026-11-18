package schemas

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

var errHomeEmpty = errors.New("a home answer holds neither role's schema")

type TutorHomeNeedsChecking struct {
	Count int64             `json:"count"`
	Items []ReviewQueueItem `json:"items" nullable:"false"`
}

type TutorHomeDeadlines struct {
	Count int64       `json:"count"`
	Items []IssueItem `json:"items" nullable:"false"`
}

type TutorHomeChannel struct {
	ChannelID   string            `json:"channel_id" format:"uuid"`
	Label       *string           `json:"label" nullable:"true"`
	LabelKind   *domain.LabelKind `json:"label_kind" enum:"deleted_user" nullable:"true"`
	Colour      *domain.Colour    `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
	UnreadCount int64             `json:"unread_count"`
}

type TutorHomeUnreadChannels struct {
	Items       []TutorHomeChannel `json:"items" nullable:"false"`
	UnreadTotal int64              `json:"unread_total"`
}

type TutorHomeJoinRequests struct {
	Count      int64              `json:"count"`
	Items      []TutorJoinRequest `json:"items" nullable:"false"`
	NextCursor *string            `json:"next_cursor" nullable:"true"`
}

type TutorHomeSteps struct {
	Profile    bool  `json:"profile"`
	Student    *bool `json:"student" nullable:"true"`
	Lesson     bool  `json:"lesson"`
	Assignment bool  `json:"assignment"`
}

type TutorHomeChecklist struct {
	OnboardingDismissed bool           `json:"onboarding_dismissed"`
	Steps               TutorHomeSteps `json:"steps"`
}

type TutorHome struct {
	TodayLessons      []CalendarItem          `json:"today_lessons" nullable:"false"`
	TodayLessonsCount int64                   `json:"today_lessons_count"`
	NeedsChecking     *TutorHomeNeedsChecking `json:"needs_checking"`
	UpcomingDeadlines *TutorHomeDeadlines     `json:"upcoming_deadlines"`
	UnreadChannels    TutorHomeUnreadChannels `json:"unread_channels"`
	JoinRequests      *TutorHomeJoinRequests  `json:"join_requests"`
	TutorCode         TutorCode               `json:"tutor_code"`
	Checklist         TutorHomeChecklist      `json:"checklist"`
}

func (TutorHome) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "needs_checking", "upcoming_deadlines", "join_requests")
}

type StudentHomeTodo struct {
	Count int64                   `json:"count"`
	Items []StudentAssignmentItem `json:"items" nullable:"false"`
}

type StudentHomeTutor struct {
	RelationshipID string                   `json:"relationship_id" format:"uuid"`
	State          domain.RelationshipState `json:"state" enum:"active,provisional"`
	Name           string                   `json:"name"`
	AvatarID       *string                  `json:"avatar_id" format:"uuid" nullable:"true"`
	Colour         domain.Colour            `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	Subjects       []string                 `json:"subjects" nullable:"false"`
	ChannelID      *string                  `json:"channel_id" format:"uuid" nullable:"true"`
}

type StudentHome struct {
	NextLesson         *CalendarItem           `json:"next_lesson"`
	TodayLessonsCount  int64                   `json:"today_lessons_count"`
	Todo               StudentHomeTodo         `json:"todo"`
	RecentlyChecked    []StudentAssignmentItem `json:"recently_checked" nullable:"false"`
	NextSevenDays      []CalendarItem          `json:"next_seven_days" nullable:"false"`
	NextSevenDaysCount int64                   `json:"next_seven_days_count"`
	Tutors             []StudentHomeTutor      `json:"tutors" nullable:"false"`
	TutorsCount        int64                   `json:"tutors_count"`
}

func (StudentHome) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "next_lesson")
}

type Home struct {
	Student *StudentHome `json:"student,omitempty"`
	Tutor   *TutorHome   `json:"tutor,omitempty"`
}

func (h Home) MarshalJSON() ([]byte, error) {
	var encoded []byte
	var err error
	switch {
	case h.Student != nil:
		encoded, err = json.Marshal(h.Student)
	case h.Tutor != nil:
		encoded, err = json.Marshal(h.Tutor)
	default:
		return nil, errHomeEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("encode a home answer: %w", err)
	}
	return encoded, nil
}

func (Home) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	student, tutor := schema.Properties["student"], schema.Properties["tutor"]
	if student == nil || tutor == nil || student.Ref == "" || tutor.Ref == "" {
		panic("a home schema lost its references")
	}
	return &huma.Schema{OneOf: []*huma.Schema{{Ref: student.Ref}, {Ref: tutor.Ref}}}
}

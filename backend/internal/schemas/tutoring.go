package schemas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type TutorCode struct {
	Code   string `json:"code" pattern:"^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$"`
	Active bool   `json:"active"`
}

type CardAccount struct {
	Name           string  `json:"name"`
	Email          string  `json:"email"`
	AvatarID       *string `json:"avatar_id" format:"uuid" nullable:"true"`
	EmailConfirmed bool    `json:"email_confirmed"`
}

type CardGroup struct {
	ID     string        `json:"id" format:"uuid"`
	Name   string        `json:"name"`
	Colour domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
}

type CardInvitation struct {
	URL       *string                 `json:"url" nullable:"true"`
	ExpiresAt time.Time               `json:"expires_at"`
	Status    domain.InvitationStatus `json:"status" enum:"open,expired,revoked_after_mismatches"`
}

type IssuedInvitation struct {
	URL       string                  `json:"url"`
	ExpiresAt time.Time               `json:"expires_at"`
	Status    domain.InvitationStatus `json:"status" enum:"open"`
}

type CardFields struct {
	ID          string                   `json:"id" format:"uuid"`
	State       domain.RelationshipState `json:"state" enum:"invited,provisional,reclaim_pending,active,ended"`
	DisplayName string                   `json:"display_name"`
	Email       *string                  `json:"email" nullable:"true"`
	Account     *CardAccount             `json:"account"`
	Groups      []CardGroup              `json:"groups" nullable:"false"`
	LinkedAt    *time.Time               `json:"linked_at" nullable:"true"`
	EndedAt     *time.Time               `json:"ended_at" nullable:"true"`
	EndedBy     *domain.EndedBy          `json:"ended_by" enum:"tutor,student,deletion" nullable:"true"`
	CreatedAt   time.Time                `json:"created_at"`
}

type StudentCardSummary struct {
	CardFields
	NextLesson *CalendarItem `json:"next_lesson"`
}

func (StudentCardSummary) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "account", "next_lesson")
}

type StudentCard struct {
	CardFields
	Notes      string          `json:"notes"`
	Invitation *CardInvitation `json:"invitation"`
}

func (StudentCard) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "account", "invitation")
}

type StudentCardDetail struct {
	StudentCard
	NextLessons []CalendarItem `json:"next_lessons" nullable:"false"`
}

func (StudentCardDetail) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "account", "invitation")
}

type CardLessonPage struct {
	Items      []CalendarItem `json:"items" nullable:"false"`
	NextCursor *string        `json:"next_cursor" nullable:"true"`
}

func nullableReferences(schema *huma.Schema, properties ...string) *huma.Schema {
	for _, name := range properties {
		property, found := schema.Properties[name]
		if !found || property.Ref == "" {
			panic("a nullable object property is missing or not a reference: " + name)
		}
		schema.Properties[name] = &huma.Schema{AnyOf: []*huma.Schema{{Ref: property.Ref}, {Type: "null"}}}
	}
	return schema
}

type StudentCardPage struct {
	Items      []StudentCardSummary `json:"items" nullable:"false"`
	NextCursor *string              `json:"next_cursor" nullable:"true"`
}

type NewStudentCard struct {
	DisplayName string  `json:"display_name" minLength:"1" maxLength:"100"`
	Email       *string `json:"email,omitempty" maxLength:"254"`
}

type NullableEmail struct {
	Set   bool
	Value *string
}

func (n *NullableEmail) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(data, []byte("null")) {
		n.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode an email: %w", err)
	}
	n.Value = &value
	return nil
}

func (NullableEmail) Schema(huma.Registry) *huma.Schema {
	maxLength := domain.EmailMaxBytes
	return &huma.Schema{Type: huma.TypeString, MaxLength: &maxLength, Nullable: true}
}

type StudentCardPatch struct {
	DisplayName *string       `json:"display_name,omitempty" minLength:"1" maxLength:"100"`
	Email       NullableEmail `json:"email,omitempty"`
	Notes       *string       `json:"notes,omitempty" maxLength:"10000"`
}

type RegeneratedInvitation struct {
	Invitation IssuedInvitation `json:"invitation"`
}

type PublicTutor struct {
	Name     string   `json:"name"`
	AvatarID *string  `json:"avatar_id" format:"uuid" nullable:"true"`
	Subjects []string `json:"subjects" nullable:"false"`
}

type InvitationPage struct {
	Tutor PublicTutor `json:"tutor"`
}

type InvitationAcceptRequest struct {
	Name     *string          `json:"name,omitempty" maxLength:"400"`
	Email    *string          `json:"email,omitempty" maxLength:"254"`
	Password *string          `json:"password,omitempty" maxLength:"512"`
	Language *domain.Language `json:"language,omitempty" enum:"ru,en"`
	Timezone *string          `json:"timezone,omitempty"`
	Consent  *bool            `json:"consent,omitempty"`
}

type MyTutorProfile struct {
	Name     string   `json:"name"`
	AvatarID *string  `json:"avatar_id" format:"uuid" nullable:"true"`
	Subjects []string `json:"subjects" nullable:"false"`
	Bio      string   `json:"bio"`
}

type MyTutor struct {
	RelationshipID     string                   `json:"relationship_id" format:"uuid"`
	State              domain.RelationshipState `json:"state" enum:"active,provisional,ended"`
	Colour             domain.Colour            `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	Tutor              MyTutorProfile           `json:"tutor"`
	EndedAt            *time.Time               `json:"ended_at" nullable:"true"`
	EndedBy            *domain.EndedBy          `json:"ended_by" enum:"tutor,student,deletion" nullable:"true"`
	AccountClosedUntil *time.Time               `json:"account_closed_until" nullable:"true"`
}

type MyTutors struct {
	Tutors []MyTutor `json:"tutors" nullable:"false"`
}

type InvitationAccepted struct {
	Tutor *MyTutor `json:"tutor,omitempty"`
}

type RemovedStudent struct {
	NewCard StudentCard `json:"new_card"`
}

package schemas

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type NullableID struct {
	Set   bool
	Value *string
}

func (n *NullableID) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(data, []byte("null")) {
		n.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode an identifier: %w", err)
	}
	n.Value = &value
	return nil
}

func (NullableID) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Format: "uuid", Nullable: true}
}

type ProfilePatch struct {
	Name     *string          `json:"name,omitempty" maxLength:"400"`
	Language *domain.Language `json:"language,omitempty" enum:"ru,en"`
	Timezone *string          `json:"timezone,omitempty" maxLength:"64"`
	Theme    *domain.Theme    `json:"theme,omitempty" enum:"light,dark,system"`
	Bio      *string          `json:"bio,omitempty" maxLength:"4000"`
	Subjects *[]string        `json:"subjects,omitempty" maxItems:"10"`
	AvatarID NullableID       `json:"avatar_id,omitempty"`
}

type UploadedFile struct {
	ID        string `json:"id" format:"uuid"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
}

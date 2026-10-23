package schemas

import (
	"time"

	"ovc/backend/internal/domain"
)

type ReclaimTutor struct {
	Name     string   `json:"name"`
	AvatarID *string  `json:"avatar_id" format:"uuid" nullable:"true"`
	Subjects []string `json:"subjects" nullable:"false"`
}

type ReclaimLink struct {
	RelationshipID string       `json:"relationship_id" format:"uuid"`
	Tutor          ReclaimTutor `json:"tutor"`
}

type ReclaimRequest struct {
	RequestID string       `json:"request_id" format:"uuid"`
	Tutor     ReclaimTutor `json:"tutor"`
}

type ReclaimList struct {
	Links    []ReclaimLink    `json:"links" nullable:"false"`
	Requests []ReclaimRequest `json:"requests" nullable:"false"`
	ChooseBy *time.Time       `json:"choose_by" nullable:"true"`
}

type ReclaimChoice struct {
	KeepLinks    []string `json:"keep_links,omitempty" maxItems:"2000" nullable:"false"`
	DropLinks    []string `json:"drop_links,omitempty" maxItems:"2000" nullable:"false"`
	KeepRequests []string `json:"keep_requests,omitempty" maxItems:"2000" nullable:"false"`
	DropRequests []string `json:"drop_requests,omitempty" maxItems:"2000" nullable:"false"`
}

type ReclaimDecided struct {
	Outcome *domain.LinkOutcome `json:"outcome" enum:"linked,request_sent,code_invalid" nullable:"true"`
}

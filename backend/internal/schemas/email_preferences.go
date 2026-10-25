package schemas

import "ovc/backend/internal/domain"

type EmailPreference struct {
	Group   domain.EmailGroup `json:"group" enum:"lesson_reminders,lesson_changes,homework,results,submissions,message_digest"`
	Enabled bool              `json:"enabled"`
}

type EmailPreferences struct {
	Groups []EmailPreference `json:"groups" maxItems:"6" nullable:"false"`
}

type UnsubscribePreview struct {
	Group domain.EmailGroup `json:"group" enum:"lesson_reminders,lesson_changes,homework,results,submissions,message_digest"`
}

package domain

import "time"

type EmailGroup string

const (
	EmailGroupLessonReminders EmailGroup = "lesson_reminders"
	EmailGroupLessonChanges   EmailGroup = "lesson_changes"
	EmailGroupHomework        EmailGroup = "homework"
	EmailGroupResults         EmailGroup = "results"
	EmailGroupSubmissions     EmailGroup = "submissions"
	EmailGroupMessageDigest   EmailGroup = "message_digest"
)

const (
	UnsubscribeTokenLifetime   = 90 * 24 * time.Hour
	UnsubscribeLimitWindow     = time.Hour
	UnsubscribeLimitPerWindow  = 60
	UnsubscribeFormField       = "List-Unsubscribe"
	UnsubscribeFormValue       = "One-Click"
	UnsubscribeFormMaxBytes    = 1024
	PurposeUnsubscribe         = RateLimitPurpose("unsubscribe_hour")
	emailGroupTokenCodeOfFirst = 1
)

var emailGroupsByRole = map[Role][]EmailGroup{
	RoleTutor:   {EmailGroupLessonReminders, EmailGroupSubmissions, EmailGroupMessageDigest},
	RoleStudent: {EmailGroupLessonReminders, EmailGroupLessonChanges, EmailGroupHomework, EmailGroupResults, EmailGroupMessageDigest},
}

var emailGroupTokenOrder = []EmailGroup{
	EmailGroupLessonReminders, EmailGroupLessonChanges, EmailGroupHomework, EmailGroupResults, EmailGroupSubmissions, EmailGroupMessageDigest,
}

func EmailGroupsOf(role Role) []EmailGroup {
	return append([]EmailGroup(nil), emailGroupsByRole[role]...)
}

func (g EmailGroup) AllowedFor(role Role) bool {
	for _, allowed := range emailGroupsByRole[role] {
		if allowed == g {
			return true
		}
	}
	return false
}

func (g EmailGroup) RolesAllowed() []Role {
	var roles []Role
	for _, role := range []Role{RoleTutor, RoleStudent} {
		if g.AllowedFor(role) {
			roles = append(roles, role)
		}
	}
	return roles
}

func (g EmailGroup) Known() bool {
	_, known := g.TokenCode()
	return known
}

func (g EmailGroup) TokenCode() (byte, bool) {
	for index, group := range emailGroupTokenOrder {
		if group == g {
			return byte(index + emailGroupTokenCodeOfFirst), true
		}
	}
	return 0, false
}

func EmailGroupFromTokenCode(code byte) (EmailGroup, bool) {
	index := int(code) - emailGroupTokenCodeOfFirst
	if index < 0 || index >= len(emailGroupTokenOrder) {
		return "", false
	}
	return emailGroupTokenOrder[index], true
}

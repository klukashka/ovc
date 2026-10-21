package accounts

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	maxBioRunes     = 1000
	maxSubjects     = 10
	maxSubjectRunes = 60

	fieldLanguage = "body.language"
	fieldTimezone = "body.timezone"
	fieldTheme    = "body.theme"
	fieldBio      = "body.bio"
	fieldSubjects = "body.subjects"
	fieldAvatar   = "body.avatar_id"
)

type AvatarBinder interface {
	BindAvatar(ctx context.Context, repos repositories.Repositories, user domain.UserID, target *domain.AttachmentID, now time.Time) error
}

type AvatarChange struct {
	Set bool
	ID  *domain.AttachmentID
}

type ProfileChange struct {
	Name     *string
	Language *domain.Language
	Timezone *string
	Theme    *domain.Theme
	Bio      *string
	Subjects *[]string
	Avatar   AvatarChange
}

func (s *Service) UpdateProfile(ctx context.Context, principal Principal, change ProfileChange) (Profile, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Profile{}, err
	}
	if problems := profileProblems(principal.Role, change); len(problems) > 0 {
		return Profile{}, &ValidationError{Fields: problems}
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Users.LockCredentials(ctx, principal.UserID)
		if err != nil {
			return err
		}
		if locked.State != domain.AccountUnverified && locked.State != domain.AccountActive {
			return ErrUnauthenticated
		}
		user, err := repos.Users.ByID(ctx, principal.UserID)
		if err != nil {
			return err
		}
		if _, err := repos.Users.UpdateProfile(ctx, mergeProfile(user, change, now)); err != nil {
			return err
		}
		if change.Bio != nil || change.Subjects != nil {
			if err := updateTutorText(ctx, repos, principal.UserID, change, now); err != nil {
				return err
			}
		}
		if change.Avatar.Set {
			return s.bindAvatar(ctx, repos, principal.UserID, change.Avatar.ID, now)
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "account.profile.updated", userAttribute(principal.UserID))
	return s.Me(ctx, principal)
}

func (s *Service) bindAvatar(ctx context.Context, repos repositories.Repositories, user domain.UserID, target *domain.AttachmentID, now time.Time) error {
	err := s.deps.Avatars.BindAvatar(ctx, repos, user, target, now)
	if errors.Is(err, domain.ErrInvalidAvatar) {
		return invalid(fieldAvatar, domain.FieldInvalid)
	}
	return err
}

func mergeProfile(user repositories.User, change ProfileChange, now time.Time) repositories.ProfileUpdate {
	update := repositories.ProfileUpdate{Name: user.Name, Language: user.Language, Timezone: user.Timezone, Theme: user.Theme, Now: now, ID: user.ID}
	if change.Name != nil {
		update.Name = strings.TrimSpace(*change.Name)
	}
	if change.Language != nil {
		update.Language = *change.Language
	}
	if change.Timezone != nil {
		update.Timezone = *change.Timezone
	}
	if change.Theme != nil {
		update.Theme = *change.Theme
	}
	return update
}

func updateTutorText(ctx context.Context, repos repositories.Repositories, user domain.UserID, change ProfileChange, now time.Time) error {
	profile, err := repos.TutorProfiles.ByUser(ctx, user)
	if err != nil {
		return err
	}
	text := repositories.TutorText{Bio: profile.Bio, Subjects: profile.Subjects, Now: now, UserID: user}
	if change.Bio != nil {
		text.Bio = strings.TrimSpace(*change.Bio)
	}
	if change.Subjects != nil {
		text.Subjects = trimmedSubjects(*change.Subjects)
	}
	_, err = repos.TutorProfiles.UpdateText(ctx, text)
	return err
}

func trimmedSubjects(subjects []string) []string {
	trimmed := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		trimmed = append(trimmed, strings.TrimSpace(subject))
	}
	return trimmed
}

func profileProblems(role domain.Role, change ProfileChange) []FieldProblem {
	var problems []FieldProblem
	if change.Name != nil && !validName(strings.TrimSpace(*change.Name)) {
		problems = append(problems, FieldProblem{Path: fieldName, Code: domain.FieldInvalid})
	}
	if change.Language != nil && *change.Language != domain.LanguageRussian && *change.Language != domain.LanguageEnglish {
		problems = append(problems, FieldProblem{Path: fieldLanguage, Code: domain.FieldInvalid})
	}
	if change.Timezone != nil && !knownZone(*change.Timezone) {
		problems = append(problems, FieldProblem{Path: fieldTimezone, Code: domain.FieldInvalid})
	}
	if change.Theme != nil && *change.Theme != domain.ThemeLight && *change.Theme != domain.ThemeDark && *change.Theme != domain.ThemeSystem {
		problems = append(problems, FieldProblem{Path: fieldTheme, Code: domain.FieldInvalid})
	}
	problems = append(problems, tutorTextProblems(role, change)...)
	return problems
}

func tutorTextProblems(role domain.Role, change ProfileChange) []FieldProblem {
	var problems []FieldProblem
	if change.Bio != nil {
		bio := strings.TrimSpace(*change.Bio)
		switch {
		case role != domain.RoleTutor:
			problems = append(problems, FieldProblem{Path: fieldBio, Code: domain.FieldNotAllowed})
		case utf8.RuneCountInString(bio) > maxBioRunes || strings.ContainsFunc(bio, controlOtherThanLineBreak):
			problems = append(problems, FieldProblem{Path: fieldBio, Code: domain.FieldInvalid})
		}
	}
	if change.Subjects != nil {
		switch {
		case role != domain.RoleTutor:
			problems = append(problems, FieldProblem{Path: fieldSubjects, Code: domain.FieldNotAllowed})
		case !validSubjects(trimmedSubjects(*change.Subjects)):
			problems = append(problems, FieldProblem{Path: fieldSubjects, Code: domain.FieldInvalid})
		}
	}
	return problems
}

func validName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= maxNameRunes && !strings.ContainsFunc(name, unicode.IsControl)
}

func validSubjects(subjects []string) bool {
	if len(subjects) > maxSubjects {
		return false
	}
	for index, subject := range subjects {
		if subject == "" || utf8.RuneCountInString(subject) > maxSubjectRunes || strings.ContainsFunc(subject, unicode.IsControl) {
			return false
		}
		if slices.Contains(subjects[:index], subject) {
			return false
		}
	}
	return true
}

func controlOtherThanLineBreak(character rune) bool {
	return unicode.IsControl(character) && character != '\n'
}

func knownZone(name string) bool {
	return name != "" && name != "Local" && resolveZone(name) == name
}

package notifications

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/core/timezonecities"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

//go:embed templates/lesson.html
var lessonHTML string

//go:embed templates/lesson.txt
var lessonText string

const (
	lessonPagePath      = "/lessons/"
	unsubscribePagePath = "/unsubscribe"
	unsubscribeAPIPath  = "/api/v1/unsubscribe"
)

type lessonDetail struct {
	Label string
	Value string
}

type lessonView struct {
	Language        domain.Language
	Subject         string
	Intro           string
	Title           string
	Details         []lessonDetail
	LessonsHeading  string
	Lessons         []string
	More            string
	Action          string
	ActionURL       string
	Note            string
	Unsubscribe     string
	UnsubscribeURL  string
	UnsubscribeNote string
}

type LessonEmails struct {
	store     *repositories.Store
	tokens    core.UnsubscribeTokens
	cities    timezonecities.Table
	publicURL string
	html      *htmltemplate.Template
	text      *texttemplate.Template
}

func NewLessonEmails(store *repositories.Store, tokens core.UnsubscribeTokens, publicURL string) (*LessonEmails, error) {
	html, err := htmltemplate.New("lesson.html").Option("missingkey=error").Parse(lessonHTML)
	if err != nil {
		return nil, fmt.Errorf("parse the lesson email HTML template: %w", err)
	}
	text, err := texttemplate.New("lesson.txt").Option("missingkey=error").Parse(lessonText)
	if err != nil {
		return nil, fmt.Errorf("parse the lesson email text template: %w", err)
	}
	cities, err := timezonecities.Load()
	if err != nil {
		return nil, err
	}
	return &LessonEmails{
		store: store, tokens: tokens, cities: cities, publicURL: strings.TrimRight(publicURL, "/"), html: html, text: text,
	}, nil
}

func (l *LessonEmails) Builders() map[domain.OutboxKind]Builder {
	builders := map[domain.OutboxKind]Builder{}
	for _, kind := range domain.ReminderKinds() {
		builders[kind.Event] = lessonReminderEmail{emails: l, kind: kind}
	}
	builders[domain.EventLessonMoved] = lessonMovedEmail{emails: l}
	builders[domain.EventLessonCancelled] = lessonCancelledEmail{emails: l}
	builders[domain.EventLessonSeriesChanged] = seriesChangedEmail{emails: l}
	builders[domain.EventGroupArchived] = groupArchivedEmail{emails: l}
	return builders
}

type lessonReminderEmail struct {
	emails *LessonEmails
	kind   domain.ReminderKind
}

func (b lessonReminderEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.LessonReminderPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	lessonID, err := decodeID[domain.LessonID](payload.Lesson)
	if err != nil {
		return Composition{}, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	lesson, err := repos.Lessons.ForEmail(ctx, lessonID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if lesson.Status != domain.LessonScheduled || lesson.ScheduleRevision != payload.Revision || b.kind.Late(lesson.StartsAt, now) {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	toTutor := recipient.ID == lesson.TutorID
	optional := &Optional{Recipient: recipient.ID, Group: domain.EmailGroupLessonReminders, Capped: !toTutor}
	composition := Composition{Recipient: AccountRecipient(recipient.State), Optional: optional}
	if !toTutor {
		inAudience, err := b.emails.inAudience(ctx, repos, lesson, recipient.ID)
		if err != nil || !inAudience {
			return Composition{Obsolete: !inAudience}, err
		}
		if lesson.TutorState != domain.AccountActive {
			composition.Drop = domain.DropTutorInactive
			return composition, nil
		}
	}
	text, found := core.LessonReminderText(recipient.Language, b.kind.Offset)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	composition.Email, err = b.emails.render(lessonRender{lesson: lesson, recipient: recipient, text: text, toTutor: toTutor, group: optional.Group}, now)
	return composition, err
}

func (l *LessonEmails) inAudience(ctx context.Context, repos repositories.Repositories, lesson repositories.LessonForEmail, student domain.UserID) (bool, error) {
	if lesson.RelationshipID != nil {
		state := domain.RelationshipState(lesson.CardState)
		active := state == domain.RelationshipActive || state == domain.RelationshipProvisional
		return active && lesson.CardStudentID != nil && *lesson.CardStudentID == student, nil
	}
	if lesson.GroupID == nil {
		return false, nil
	}
	return repos.Lessons.IsCurrentMember(ctx, *lesson.GroupID, student)
}

type lessonRender struct {
	lesson    repositories.LessonForEmail
	recipient repositories.User
	text      core.LessonEmailText
	toTutor   bool
	group     domain.EmailGroup
	lines     []string
	more      string
	former    bool
}

func (l *LessonEmails) render(source lessonRender, now time.Time) (Email, error) {
	when, err := l.when(source.recipient, source.lesson.StartsAt, source.lesson.Timezone)
	if err != nil {
		return Email{}, err
	}
	title := core.EmailSafeText(source.lesson.Title)
	details := append([]lessonDetail{{Label: source.text.When, Value: when}}, l.audienceDetails(source)...)
	if source.lines != nil {
		details = l.audienceDetails(source)
	}
	view := lessonView{
		Language: source.recipient.Language, Subject: source.text.SubjectPrefix + ": " + title, Intro: source.text.Intro, Title: title,
		Details: details, Lessons: source.lines, More: source.more,
		Action: source.text.Action, ActionURL: l.publicURL + lessonPagePath + source.lesson.ID.String(), Note: source.text.Note,
		Unsubscribe: source.text.Unsubscribe, UnsubscribeNote: source.text.UnsubscribeNote,
	}
	return l.execute(view, source.recipient, source.group, now)
}

func (l *LessonEmails) execute(view lessonView, recipient repositories.User, group domain.EmailGroup, now time.Time) (Email, error) {
	token, err := l.tokens.Mint(recipient.ID, group, now)
	if err != nil {
		return Email{}, err
	}
	view.UnsubscribeURL = l.publicURL + unsubscribePagePath + "?token=" + url.QueryEscape(token)
	var html, plain bytes.Buffer
	if err := l.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the lesson email: %w", err))
	}
	if err := l.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the lesson email: %w", err))
	}
	return Email{
		To: recipient.Email, Subject: view.Subject, Text: plain.String(), HTML: html.String(),
		Unsubscribe: l.publicURL + unsubscribeAPIPath + "?token=" + url.QueryEscape(token),
	}, nil
}

func (l *LessonEmails) when(recipient repositories.User, startsAt time.Time, lessonTimezone string) (string, error) {
	return lessonStart(l.cities, recipient, startsAt, lessonTimezone)
}

func lessonStart(cities timezonecities.Table, recipient repositories.User, startsAt time.Time, lessonTimezone string) (string, error) {
	zone, err := time.LoadLocation(recipient.Timezone)
	if err != nil {
		return "", fmt.Errorf("load the recipient's timezone: %w", err)
	}
	var label *core.ZoneLabel
	if recipient.Timezone != lessonTimezone {
		place := cities.Describe(zone, startsAt)
		label = &core.ZoneLabel{Offset: place.Offset}
		if place.City != nil {
			label.City = place.City.English
			if recipient.Language == domain.LanguageRussian {
				label.City = place.City.Russian
			}
		}
	}
	return core.LessonStartText(recipient.Language, startsAt, zone, label), nil
}

func (l *LessonEmails) audienceDetails(source lessonRender) []lessonDetail {
	lesson, text := source.lesson, source.text
	if source.toTutor {
		if lesson.RelationshipID == nil {
			return []lessonDetail{{Label: text.Group, Value: safeName(lesson.GroupName)}}
		}
		return []lessonDetail{{Label: text.Student, Value: tutorCardLabel(lesson, text)}}
	}
	audience := lessonDetail{Label: text.Lesson, Value: text.Individual}
	if lesson.RelationshipID == nil {
		audience = lessonDetail{Label: text.Group, Value: safeName(lesson.GroupName)}
	}
	if lesson.RelationshipID == nil && source.former {
		audience = lessonDetail{Label: text.Lesson, Value: text.GroupLesson}
	}
	return []lessonDetail{audience, {Label: text.Tutor, Value: core.EmailSafeText(lesson.TutorName)}}
}

func tutorCardLabel(lesson repositories.LessonForEmail, text core.LessonEmailText) string {
	removed := lesson.CardDiscardReason != nil && (*lesson.CardDiscardReason == domain.DiscardRemoved || *lesson.CardDiscardReason == domain.DiscardTutorDeleted)
	if lesson.CardName == nil || (domain.RelationshipState(lesson.CardState) == domain.RelationshipDiscarded && removed) {
		return text.RemovedStudent
	}
	return safeName(lesson.CardName)
}

func safeName(name *string) string {
	if name == nil {
		return ""
	}
	return core.EmailSafeText(*name)
}

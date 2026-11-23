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
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

//go:embed templates/homework.html
var homeworkHTML string

//go:embed templates/homework.txt
var homeworkText string

type homeworkView struct {
	Language        domain.Language
	Subject         string
	Intro           string
	Title           string
	Details         []lessonDetail
	Action          string
	ActionURL       string
	Note            string
	Unsubscribe     string
	UnsubscribeURL  string
	UnsubscribeNote string
}

type HomeworkEmails struct {
	store     *repositories.Store
	tokens    core.UnsubscribeTokens
	publicURL string
	html      *htmltemplate.Template
	text      *texttemplate.Template
}

func NewHomeworkEmails(store *repositories.Store, tokens core.UnsubscribeTokens, publicURL string) (*HomeworkEmails, error) {
	html, err := htmltemplate.New("homework.html").Option("missingkey=error").Parse(homeworkHTML)
	if err != nil {
		return nil, fmt.Errorf("parse the homework email HTML template: %w", err)
	}
	text, err := texttemplate.New("homework.txt").Option("missingkey=error").Parse(homeworkText)
	if err != nil {
		return nil, fmt.Errorf("parse the homework email text template: %w", err)
	}
	return &HomeworkEmails{store: store, tokens: tokens, publicURL: strings.TrimRight(publicURL, "/"), html: html, text: text}, nil
}

func (h *HomeworkEmails) Builders() map[domain.OutboxKind]Builder {
	return map[domain.OutboxKind]Builder{
		domain.EventIssueCreated:        issueCreatedEmail{emails: h},
		domain.EventSubmissionSubmitted: submittedEmail{emails: h},
		domain.EventSubmissionChecked:   resultEmail{emails: h, text: core.SubmissionCheckedText},
		domain.EventSubmissionReturned:  resultEmail{emails: h, text: core.SubmissionReturnedText},
		domain.EventReviewUpdated:       resultEmail{emails: h, text: core.ReviewUpdatedText},
	}
}

type issueCreatedEmail struct {
	emails *HomeworkEmails
}

func (b issueCreatedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.IssueCreatedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	id, err := decodeID[domain.SubmissionID](payload.Submission)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	submission, err := repos.Homework.SubmissionForEmail(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	readable := domain.ReadableStudentSubmission(submission.IssueStatus, submission.MarkedForErasure, submission.RelationshipState)
	if !readable || submission.Status == domain.SubmissionClosed {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, submission.StudentID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	if submission.TutorState != domain.AccountActive {
		return tutorInactive(recipient, domain.EmailGroupHomework), nil
	}
	text, found := core.IssueCreatedText(recipient.Language)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	return b.emails.compose(recipient, homeworkMessage{
		text: text, person: submission.TutorName, deadline: submission.DeadlineAt, title: submission.Title,
		path: domain.HomeworkAssignmentPagePath + id.String(), group: domain.EmailGroupHomework,
	}, now)
}

func tutorInactive(recipient repositories.User, group domain.EmailGroup) Composition {
	return Composition{
		Recipient: AccountRecipient(recipient.State),
		Optional:  &Optional{Recipient: recipient.ID, Group: group, Capped: true},
		Drop:      domain.DropTutorInactive,
	}
}

type homeworkMessage struct {
	text     core.HomeworkEmailText
	person   string
	deadline *time.Time
	title    string
	path     string
	group    domain.EmailGroup
}

func (h *HomeworkEmails) compose(recipient repositories.User, message homeworkMessage, now time.Time) (Composition, error) {
	optional := &Optional{Recipient: recipient.ID, Group: message.group, Capped: true}
	composition := Composition{Recipient: AccountRecipient(recipient.State), Optional: optional}
	text := message.text
	details := []lessonDetail{{Label: text.Tutor, Value: core.EmailSafeText(message.person)}}
	if message.deadline != nil {
		zone, err := time.LoadLocation(recipient.Timezone)
		if err != nil {
			return Composition{}, fmt.Errorf("load the recipient's timezone: %w", err)
		}
		details = append(details, lessonDetail{Label: text.Deadline, Value: core.LessonStartText(recipient.Language, *message.deadline, zone, nil)})
	}
	title := core.EmailSafeText(message.title)
	view := homeworkView{
		Language: recipient.Language, Subject: text.Subject + ": " + title, Intro: text.Intro, Title: title, Details: details,
		Action: text.Action, ActionURL: h.publicURL + message.path, Note: text.Note,
		Unsubscribe: text.Unsubscribe, UnsubscribeNote: text.UnsubscribeNote,
	}
	var err error
	composition.Email, err = h.execute(view, recipient, optional.Group, now)
	return composition, err
}

type submittedEmail struct {
	emails *HomeworkEmails
}

func (b submittedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.SubmittedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	id, err := decodeID[domain.SubmissionID](payload.Submission)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	submission, err := repos.Homework.SubmissionForTutorEmail(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	stillSubmitted := submission.Status == domain.SubmissionSubmitted && submission.SubmittedAt != nil &&
		submission.SubmittedAt.UnixMicro() == payload.SubmittedAtMicros
	if !stillSubmitted || submission.MarkedForErasure || submission.CardState == domain.RelationshipDiscarded {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, submission.TutorID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	text, found := core.SubmittedText(recipient.Language)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	person, err := domain.CardName(submission.DisplayName)
	if err != nil {
		return Composition{}, domain.PermanentEffect(err)
	}
	return b.emails.compose(recipient, homeworkMessage{
		text: text, person: person, deadline: submission.DeadlineAt, title: submission.Title,
		path: domain.HomeworkReviewPagePath + id.String(), group: domain.EmailGroupSubmissions,
	}, now)
}

type resultEmail struct {
	emails *HomeworkEmails
	text   func(domain.Language) (core.HomeworkEmailText, bool)
}

func (b resultEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.SubmissionCheckedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	id, err := decodeID[domain.SubmissionID](payload.Submission)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	submission, err := repos.Homework.SubmissionForEmail(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if !domain.ReadableStudentSubmission(submission.IssueStatus, submission.MarkedForErasure, submission.RelationshipState) {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, submission.StudentID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	if submission.TutorState != domain.AccountActive {
		return tutorInactive(recipient, domain.EmailGroupResults), nil
	}
	text, found := b.text(recipient.Language)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	return b.emails.compose(recipient, homeworkMessage{
		text: text, person: submission.TutorName, deadline: submission.DeadlineAt, title: submission.Title,
		path: domain.HomeworkAssignmentPagePath + id.String(), group: domain.EmailGroupResults,
	}, now)
}

func (h *HomeworkEmails) execute(view homeworkView, recipient repositories.User, group domain.EmailGroup, now time.Time) (Email, error) {
	token, err := h.tokens.Mint(recipient.ID, group, now)
	if err != nil {
		return Email{}, err
	}
	view.UnsubscribeURL = h.publicURL + unsubscribePagePath + "?token=" + url.QueryEscape(token)
	var html, plain bytes.Buffer
	if err := h.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the homework email: %w", err))
	}
	if err := h.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the homework email: %w", err))
	}
	return Email{
		To: recipient.Email, Subject: view.Subject, Text: plain.String(), HTML: html.String(),
		Unsubscribe: h.publicURL + unsubscribeAPIPath + "?token=" + url.QueryEscape(token),
	}, nil
}

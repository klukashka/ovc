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
	return map[domain.OutboxKind]Builder{domain.EventIssueCreated: issueCreatedEmail{emails: h}}
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
	optional := &Optional{Recipient: recipient.ID, Group: domain.EmailGroupHomework, Capped: true}
	composition := Composition{Recipient: AccountRecipient(recipient.State), Optional: optional}
	text, found := core.IssueCreatedText(recipient.Language)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	details := []lessonDetail{{Label: text.Tutor, Value: core.EmailSafeText(submission.TutorName)}}
	if submission.DeadlineAt != nil {
		zone, err := time.LoadLocation(recipient.Timezone)
		if err != nil {
			return Composition{}, fmt.Errorf("load the recipient's timezone: %w", err)
		}
		details = append(details, lessonDetail{Label: text.Deadline, Value: core.LessonStartText(recipient.Language, *submission.DeadlineAt, zone, nil)})
	}
	title := core.EmailSafeText(submission.Title)
	view := homeworkView{
		Language: recipient.Language, Subject: text.Subject + ": " + title, Intro: text.Intro, Title: title, Details: details,
		Action: text.Action, ActionURL: b.emails.publicURL + domain.HomeworkAssignmentPagePath + id.String(), Note: text.Note,
		Unsubscribe: text.Unsubscribe, UnsubscribeNote: text.UnsubscribeNote,
	}
	composition.Email, err = b.emails.execute(view, recipient, optional.Group, now)
	return composition, err
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

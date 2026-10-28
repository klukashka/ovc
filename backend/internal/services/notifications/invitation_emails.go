package notifications

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/core/timezonecities"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

//go:embed templates/relationship.html
var relationshipHTML string

//go:embed templates/relationship.txt
var relationshipText string

const invitationPathPrefix = "/invite/"

type relationshipView struct {
	Language       domain.Language
	Subject        string
	Intro          string
	Name           string
	Lines          []string
	LessonsHeading string
	Lessons        []string
	More           string
	Action         string
	ActionURL      string
	Note           string
}

type RelationshipEmails struct {
	store     *repositories.Store
	cipher    core.FieldCipher
	cities    timezonecities.Table
	publicURL string
	html      *htmltemplate.Template
	text      *texttemplate.Template
}

func NewRelationshipEmails(store *repositories.Store, cipher core.FieldCipher, publicURL string) (*RelationshipEmails, error) {
	html, err := htmltemplate.New("relationship.html").Option("missingkey=error").Parse(relationshipHTML)
	if err != nil {
		return nil, fmt.Errorf("parse the relationship email HTML template: %w", err)
	}
	text, err := texttemplate.New("relationship.txt").Option("missingkey=error").Parse(relationshipText)
	if err != nil {
		return nil, fmt.Errorf("parse the relationship email text template: %w", err)
	}
	cities, err := timezonecities.Load()
	if err != nil {
		return nil, err
	}
	return &RelationshipEmails{
		store: store, cipher: cipher, cities: cities, publicURL: strings.TrimRight(publicURL, "/"), html: html, text: text,
	}, nil
}

func (r *RelationshipEmails) Builders() map[domain.OutboxKind]Builder {
	return map[domain.OutboxKind]Builder{
		domain.EventInvitationCreated: invitationEmail{emails: r},
		domain.EventRelationshipEnded: relationshipEndedEmail{emails: r},
		domain.EventTutorDeleted:      tutorDeletedEmail{emails: r},
	}
}

type invitationEmail struct {
	emails *RelationshipEmails
}

type invitationRecipient struct {
	language domain.Language
	state    Recipient
}

func (b invitationEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.InvitationEmailPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	invitationID, err := decodeID[domain.InvitationID](payload.Invitation)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	invitation, err := repos.Invitations.ForEmail(ctx, invitationID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if !sendable(invitation, payload.Send, now) {
		return Composition{Obsolete: true}, nil
	}
	tutor, err := repos.Users.ByID(ctx, invitation.TutorID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if tutor.State != domain.AccountActive {
		return Composition{Obsolete: true}, nil
	}
	recipient, err := b.recipient(ctx, repos, *invitation.Email, tutor.Language)
	if err != nil {
		return Composition{}, err
	}
	link, err := b.link(invitation)
	if err != nil {
		return Composition{}, err
	}
	text, found := core.RelationshipEmailText(recipient.language, row.Kind)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.language))
	}
	email, err := b.emails.render(recipient.language, text, *invitation.Email, tutor.Name, link, cancelledList{})
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Identity: invitation.ID.String(), Recipient: recipient.state}, nil
}

func sendable(invitation repositories.InvitationEmail, send int16, now time.Time) bool {
	return invitation.UsedAt == nil && invitation.RevokedAt == nil && invitation.ExpiresAt.After(now) &&
		invitation.TokenEncrypted != nil && invitation.SendCount == send && invitation.Email != nil &&
		domain.CardTakesInvitations(invitation.State)
}

func (b invitationEmail) recipient(ctx context.Context, repos repositories.Repositories, email string, fallback domain.Language) (invitationRecipient, error) {
	account, err := repos.Users.ByEmail(ctx, email)
	if errors.Is(err, repositories.ErrNotFound) {
		return invitationRecipient{language: fallback, state: NoAccountRecipient()}, nil
	}
	if err != nil {
		return invitationRecipient{}, err
	}
	return invitationRecipient{language: account.Language, state: AccountRecipient(account.State)}, nil
}

func (b invitationEmail) link(invitation repositories.InvitationEmail) (string, error) {
	raw, err := b.emails.cipher.OpenForInvitation(invitation.ID, invitation.TokenEncrypted)
	if err != nil {
		return "", domain.PermanentEffect(err)
	}
	token, valid := core.InvitationTokenText(raw)
	if !valid {
		return "", domain.PermanentEffect(core.ErrFieldCiphertextInvalid)
	}
	return b.emails.publicURL + invitationPathPrefix + token, nil
}

func (r *RelationshipEmails) render(
	language domain.Language, text core.RelationshipText, to string, name string, actionURL string, cancelled cancelledList,
) (Email, error) {
	view := relationshipView{
		Language: language, Subject: text.Subject, Intro: text.Intro, Name: strings.Join(strings.Fields(name), " "),
		Lines: text.Lines, LessonsHeading: cancelled.heading, Lessons: cancelled.lines, More: cancelled.more,
		Action: text.Action, ActionURL: actionURL, Note: text.Note,
	}
	var html, plain bytes.Buffer
	if err := r.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the relationship email: %w", err))
	}
	if err := r.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the relationship email: %w", err))
	}
	return Email{To: to, Subject: text.Subject, Text: plain.String(), HTML: html.String()}, nil
}

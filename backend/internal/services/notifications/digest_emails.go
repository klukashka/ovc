package notifications

import (
	"bytes"
	"context"
	_ "embed"
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

//go:embed templates/digest.html
var digestHTML string

//go:embed templates/digest.txt
var digestText string

type digestLine struct {
	Label string
	Count int64
}

type digestView struct {
	Language       domain.Language
	Subject        string
	Intro          string
	Channels       []digestLine
	More           string
	Action         string
	ActionURL      string
	Unsubscribe    string
	UnsubscribeURL string
}

type DigestEmails struct {
	store     *repositories.Store
	tokens    core.UnsubscribeTokens
	publicURL string
	html      *htmltemplate.Template
	text      *texttemplate.Template
}

func NewDigestEmails(store *repositories.Store, tokens core.UnsubscribeTokens, publicURL string) (*DigestEmails, error) {
	html, err := htmltemplate.New("digest.html").Option("missingkey=error").Parse(digestHTML)
	if err != nil {
		return nil, fmt.Errorf("parse digest HTML template: %w", err)
	}
	text, err := texttemplate.New("digest.txt").Option("missingkey=error").Parse(digestText)
	if err != nil {
		return nil, fmt.Errorf("parse digest text template: %w", err)
	}
	return &DigestEmails{store: store, tokens: tokens, publicURL: strings.TrimRight(publicURL, "/"), html: html, text: text}, nil
}

func (d *DigestEmails) Builders() map[domain.OutboxKind]Builder {
	return map[domain.OutboxKind]Builder{domain.EventMessagesDigest: d}
}

func (d *DigestEmails) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.DigestPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	user, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return Composition{}, err
	}
	var composition Composition
	err = d.store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		recipient, found, err := existingUser(ctx, repos, user)
		if err != nil {
			return err
		}
		if !found {
			composition.Obsolete = true
			return nil
		}
		composition.Recipient = AccountRecipient(recipient.State)
		composition.Optional = &Optional{Recipient: user, Group: domain.EmailGroupMessageDigest}
		lines, total, err := d.lines(ctx, repos, user, payload)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			composition.Drop = domain.DropObsoleteDigest
			return nil
		}
		composition.Email, err = d.render(recipient, lines, total, now)
		return err
	})
	return composition, err
}

func (d *DigestEmails) lines(ctx context.Context, repos repositories.Repositories, user domain.UserID, payload domain.DigestPayload) ([]digestLine, int64, error) {
	bounds := make([]repositories.DigestRange, 0, len(payload.Channels))
	for _, bound := range payload.Channels {
		channel, err := decodeID[domain.ChannelID](bound.Channel)
		if err != nil {
			return nil, 0, err
		}
		if bound.AfterSeq < 0 || bound.ThroughSeq < bound.AfterSeq {
			return nil, 0, domain.PermanentEffect(errMalformedPayload)
		}
		bounds = append(bounds, repositories.DigestRange{Channel: channel, After: bound.AfterSeq, Through: bound.ThroughSeq})
	}
	rows, err := repos.Digests.AtDispatch(ctx, user, bounds, payload.AfterSeen)
	if err != nil {
		return nil, 0, err
	}
	lines := make([]digestLine, 0, len(rows))
	var total int64
	for _, row := range rows {
		lines = append(lines, digestLine{Label: core.EmailSafeText(row.Label), Count: row.MessageCount})
		total = row.TotalChannels
	}
	return lines, total, nil
}

func (d *DigestEmails) render(recipient repositories.User, lines []digestLine, total int64, now time.Time) (Email, error) {
	text, found := core.DigestEmailText(recipient.Language)
	if !found {
		return Email{}, domain.PermanentEffect(fmt.Errorf("%w: digest language %s", errUnknownKind, recipient.Language))
	}
	view := digestView{Language: recipient.Language, Subject: text.Subject, Intro: text.Intro, Action: text.Action, ActionURL: d.publicURL + "/messages", Unsubscribe: text.Unsubscribe}
	if total > int64(len(lines)) {
		view.More = fmt.Sprintf(text.More, total-int64(len(lines)))
	}
	view.Channels = lines
	token, err := d.tokens.Mint(recipient.ID, domain.EmailGroupMessageDigest, now)
	if err != nil {
		return Email{}, err
	}
	view.UnsubscribeURL = d.publicURL + unsubscribePagePath + "?token=" + url.QueryEscape(token)
	var html, plain bytes.Buffer
	if err := d.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render digest HTML: %w", err))
	}
	if err := d.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render digest text: %w", err))
	}
	return Email{To: recipient.Email, Subject: view.Subject, Text: plain.String(), HTML: html.String(), Unsubscribe: d.publicURL + unsubscribeAPIPath + "?token=" + url.QueryEscape(token)}, nil
}

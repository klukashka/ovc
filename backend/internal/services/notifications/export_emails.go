package notifications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const myDataPage = "/settings/data"

type exportRequestedEmail struct {
	emails *AccountEmails
}

func (b exportRequestedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (Composition, error) {
	var payload domain.ExportRequestedPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	userID, err := decodeID[domain.UserID](payload.User)
	if err != nil {
		return Composition{}, err
	}
	if _, err := decodeID[domain.ExportID](payload.Export); err != nil {
		return Composition{}, err
	}
	user, err := b.emails.store.Repositories().Users.ByID(ctx, userID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if user.State != domain.AccountActive {
		return Composition{Obsolete: true}, nil
	}
	email, err := b.render(user, time.UnixMicro(payload.RequestedAtMicros))
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Identity: payload.Export, Recipient: AccountRecipient(user.State)}, nil
}

func (b exportRequestedEmail) render(user repositories.User, requestedAt time.Time) (Email, error) {
	text, found := core.ExportRequestedText(user.Language)
	if !found {
		return Email{}, domain.PermanentEffect(fmt.Errorf("%w: no export request text in %s", errUnknownKind, user.Language))
	}
	zone, err := time.LoadLocation(user.Timezone)
	if err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("load the recipient's timezone: %w", err))
	}
	when := core.LessonStartText(user.Language, requestedAt, zone, nil)
	lines := make([]string, 0, len(text.Lines))
	for index, line := range text.Lines {
		if index == 0 {
			line = fmt.Sprintf(line, when)
		}
		lines = append(lines, line)
	}
	view := accountView{Language: user.Language, Subject: text.Subject, Lines: lines, Action: text.Action, ActionURL: b.emails.publicURL + myDataPage, Note: text.Note}
	var html, plain bytes.Buffer
	if err := b.emails.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the export request email: %w", err))
	}
	if err := b.emails.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the export request email: %w", err))
	}
	return Email{To: user.Email, Subject: text.Subject, Text: plain.String(), HTML: html.String()}, nil
}

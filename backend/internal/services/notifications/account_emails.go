package notifications

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

//go:embed templates/account.html
var accountHTML string

//go:embed templates/account.txt
var accountText string

var errMalformedPayload = errors.New("the outbox payload is malformed")

type accountView struct {
	Language  domain.Language
	Subject   string
	Lines     []string
	Action    string
	ActionURL string
	Note      string
}

type AccountEmails struct {
	store     *repositories.Store
	random    core.RandomSource
	publicURL string
	html      *htmltemplate.Template
	text      *texttemplate.Template
}

func NewAccountEmails(store *repositories.Store, random core.RandomSource, publicURL string) (*AccountEmails, error) {
	html, err := htmltemplate.New("account.html").Option("missingkey=error").Parse(accountHTML)
	if err != nil {
		return nil, fmt.Errorf("parse the account email HTML template: %w", err)
	}
	text, err := texttemplate.New("account.txt").Option("missingkey=error").Parse(accountText)
	if err != nil {
		return nil, fmt.Errorf("parse the account email text template: %w", err)
	}
	return &AccountEmails{store: store, random: random, publicURL: strings.TrimRight(publicURL, "/"), html: html, text: text}, nil
}

func (a *AccountEmails) Builders() map[domain.OutboxKind]Builder {
	return map[domain.OutboxKind]Builder{
		domain.EventEmailConfirmation: tokenEmail{emails: a, purpose: domain.EmailPurposeVerify, page: "/verify-email"},
		domain.EventPasswordReset:     tokenEmail{emails: a, purpose: domain.EmailPurposeReset, page: "/reset-password"},
		domain.EventSignUpDuplicate:   duplicateEmail{emails: a},
	}
}

type tokenEmail struct {
	emails  *AccountEmails
	purpose domain.EmailPurpose
	page    string
}

func (b tokenEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.EmailRequestPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	requestID, err := decodeID[domain.EmailRequestID](payload.Request)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	request, err := repos.EmailTokens.Request(ctx, requestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if request.Purpose != b.purpose {
		return Composition{}, domain.PermanentEffect(errMalformedPayload)
	}
	user, err := repos.Users.ByID(ctx, request.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	tokenID, token, minted, err := b.mint(ctx, request, now)
	if err != nil {
		return Composition{}, err
	}
	if !minted {
		return Composition{Obsolete: true}, nil
	}
	email, err := b.emails.render(user, row.Kind, b.emails.publicURL+b.page+"?token="+token)
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Identity: hex.EncodeToString(tokenID[:]), Recipient: AccountRecipient(user.State)}, nil
}

func (b tokenEmail) eligible(state domain.AccountState) bool {
	if b.purpose == domain.EmailPurposeVerify {
		return state == domain.AccountUnverified
	}
	return state == domain.AccountUnverified || state == domain.AccountActive
}

func (b tokenEmail) mint(ctx context.Context, request repositories.EmailRequest, now time.Time) (domain.EmailTokenID, string, bool, error) {
	id, err := core.NewID[domain.EmailTokenID](b.emails.random)
	if err != nil {
		return id, "", false, err
	}
	raw, token, err := core.RandomToken(b.emails.random, domain.EmailTokenBytes)
	if err != nil {
		return id, "", false, err
	}
	hash := sha256.Sum256(raw)
	minted := false
	err = b.emails.store.WithTx(ctx, func(repos repositories.Repositories) error {
		current, err := b.current(ctx, repos, request)
		if err != nil || !current {
			return err
		}
		if _, err := repos.EmailTokens.DiscardUnusedFor(ctx, request.UserID, request.Purpose); err != nil {
			return err
		}
		minted = true
		return repos.EmailTokens.Create(ctx, repositories.NewEmailToken{
			ID: id, RequestID: request.ID, UserID: request.UserID, Purpose: request.Purpose, TokenHash: hash[:],
			ExpiresAt: now.Add(request.Purpose.TokenLifetime()), CreatedAt: now,
		})
	})
	return id, token, minted, err
}

func (b tokenEmail) current(ctx context.Context, repos repositories.Repositories, request repositories.EmailRequest) (bool, error) {
	locked, err := repos.Users.LockCredentials(ctx, request.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return false, nil
	}
	if err != nil || !b.eligible(locked.State) {
		return false, err
	}
	if _, err := repos.EmailTokens.Request(ctx, request.ID); errors.Is(err, repositories.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	used, err := repos.EmailTokens.RequestUsed(ctx, request.ID)
	if err != nil || used {
		return false, err
	}
	superseded, err := repos.EmailTokens.Superseded(ctx, request)
	return !superseded, err
}

type duplicateEmail struct {
	emails *AccountEmails
}

func (b duplicateEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (Composition, error) {
	var payload domain.AccountPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	userID, err := decodeID[domain.UserID](payload.User)
	if err != nil {
		return Composition{}, err
	}
	user, err := b.emails.store.Repositories().Users.ByID(ctx, userID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if user.State != domain.AccountUnverified && user.State != domain.AccountActive {
		return Composition{Obsolete: true}, nil
	}
	email, err := b.emails.render(user, row.Kind, b.emails.publicURL+"/forgot-password")
	if err != nil {
		return Composition{}, err
	}
	return Composition{Email: email, Recipient: AccountRecipient(user.State)}, nil
}

func (a *AccountEmails) render(user repositories.User, event domain.OutboxKind, actionURL string) (Email, error) {
	text, found := core.AccountEmailText(user.Language, event)
	if !found {
		return Email{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, event, user.Language))
	}
	view := accountView{Language: user.Language, Subject: text.Subject, Lines: text.Lines, Action: text.Action, ActionURL: actionURL, Note: text.Note}
	var html, plain bytes.Buffer
	if err := a.html.Execute(&html, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the account email: %w", err))
	}
	if err := a.text.Execute(&plain, view); err != nil {
		return Email{}, domain.PermanentEffect(fmt.Errorf("render the account email: %w", err))
	}
	return Email{To: user.Email, Subject: text.Subject, Text: plain.String(), HTML: html.String()}, nil
}

func decodePayload[Payload domain.EmailRequestPayload | domain.AccountPayload | domain.InvitationEmailPayload | domain.RelationshipEndedPayload | domain.TutorDeletedPayload | domain.LessonReminderPayload | domain.LessonCancelledPayload | domain.SeriesChangedPayload | domain.GroupArchivedPayload](encoded []byte, target *Payload) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.PermanentEffect(fmt.Errorf("%w: %w", errMalformedPayload, err))
	}
	return nil
}

func decodeID[ID ~[16]byte](text string) (ID, error) {
	var id ID
	decoded, err := hex.DecodeString(text)
	if err != nil || len(decoded) != len(id) {
		return id, domain.PermanentEffect(errMalformedPayload)
	}
	copy(id[:], decoded)
	return id, nil
}

package chat

import (
	"context"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Attachment struct {
	ID        domain.AttachmentID
	Filename  string
	MediaType string
	SizeBytes int64
	Width     *int32
	Height    *int32
	URL       string
	ExpiresAt time.Time
}

type Message struct {
	ID                domain.MessageID
	Seq               int64
	SenderLabel       *string
	SenderLabelKind   *domain.LabelKind
	Own               bool
	Body              *string
	CreatedAt         time.Time
	EditedAt          *time.Time
	Deleted           bool
	Erased            bool
	Version           int32
	SentBeforeReclaim bool
	Attachments       []Attachment
	ReplyTo           *Quote
	Reactions         []ReactionView
}

type Quote struct {
	MessageID         domain.MessageID
	Seq               int64
	SenderLabel       *string
	SenderLabelKind   *domain.LabelKind
	Own               bool
	Preview           *string
	State             domain.QuoteState
	SentBeforeReclaim bool
}

type LastMessage struct {
	PeerRead        *bool
	Deleted         bool
	Erased          bool
	Seq             int64
	CreatedAt       time.Time
	Own             bool
	SenderLabel     *string
	SenderLabelKind *domain.LabelKind
	Preview         *string
	AttachmentCount int64
}

type sender struct {
	label *string
	kind  *domain.LabelKind
	own   bool
}

func deletedUser() sender {
	kind := domain.LabelDeletedUser
	return sender{kind: &kind}
}

type labelInput struct {
	viewer domain.UserID
	tutor  domain.UserID
	sender *domain.UserID
	state  string
	name   *string
	card   string
}

func senderOf(viewer domain.UserID, row repositories.MessageRow) sender {
	return senderLabel(labelInput{viewer: viewer, tutor: row.TutorID, sender: row.SenderID, state: row.SenderState, name: row.SenderName, card: row.CardName})
}

func senderLabel(input labelInput) sender {
	switch {
	case input.sender == nil:
		return deletedUser()
	case *input.sender == input.viewer:
		return sender{own: true}
	case input.state == string(domain.AccountDeleted):
		return deletedUser()
	case input.viewer == input.tutor:
		if input.card == "" {
			return deletedUser()
		}
		name := input.card
		return sender{label: &name}
	case input.name == nil:
		return deletedUser()
	}
	name := *input.name
	return sender{label: &name}
}

func (s *Service) messageView(viewer domain.UserID, row repositories.MessageRow, files []repositories.MessageAttachment, now time.Time) Message {
	from := senderOf(viewer, row)
	message := Message{
		ID: row.ID, Seq: row.Seq, SenderLabel: from.label, SenderLabelKind: from.kind, Own: from.own, Body: row.Body, CreatedAt: row.CreatedAt,
		EditedAt: row.EditedAt, Deleted: row.DeletedAt != nil, Erased: row.ErasedAt != nil, Version: row.Version,
		SentBeforeReclaim: row.SentBeforeReclaim, Attachments: make([]Attachment, 0, len(files)),
	}
	for _, file := range files {
		disposition := domain.DispositionAttachment
		if file.Width != nil {
			disposition = domain.DispositionInline
		}
		url, expires := s.deps.Addresses.ChatAddress(file.ID, row.ID, disposition, now)
		message.Attachments = append(message.Attachments, Attachment{
			ID: file.ID, Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height,
			URL: url, ExpiresAt: expires,
		})
	}
	return message
}

type noAddresses struct{}

func (noAddresses) ChatAddress(domain.AttachmentID, domain.MessageID, domain.Disposition, time.Time) (string, time.Time) {
	return "", time.Time{}
}

func (s *Service) withoutAddresses() *Service {
	free := *s
	free.deps.Addresses = noAddresses{}
	return &free
}

func messageAccounts(rows []repositories.MessageRow) []domain.UserID {
	accounts := make([]domain.UserID, 0, 2*len(rows))
	for _, row := range rows {
		accounts = append(accounts, row.TutorID)
		if row.SenderID != nil {
			accounts = append(accounts, *row.SenderID)
		}
	}
	return accounts
}

func (s *Service) messageViews(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, rows []repositories.MessageRow, now time.Time,
) ([]Message, error) {
	views, _, err := s.messagePage(ctx, repos, viewer, rows, now)
	return views, err
}

func (s *Service) messagePage(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, rows []repositories.MessageRow, now time.Time,
) ([]Message, []domain.UserID, error) {
	ids := make([]domain.MessageID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	files, err := repos.Messages.AttachmentsOf(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	byMessage := map[domain.MessageID][]repositories.MessageAttachment{}
	for _, file := range files {
		message := domain.MessageID(file.MessageID)
		byMessage[message] = append(byMessage[message], file)
	}
	quotes, quoted, err := quotesOf(ctx, repos, viewer, rows)
	if err != nil {
		return nil, nil, err
	}
	reactionRows, err := repos.Messages.ReactionsOf(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	reactions := reactionViews(viewer, reactionRows)
	accounts := append(messageAccounts(rows), messageAccounts(quoted)...)
	for _, row := range reactionRows {
		accounts = append(accounts, row.UserID)
	}
	views := make([]Message, 0, len(rows))
	for _, row := range rows {
		view := s.messageView(viewer, row, byMessage[row.ID], now)
		view.Reactions = reactions[row.ID]
		if row.ReplyToMessageID != nil {
			view.ReplyTo = quotes[*row.ReplyToMessageID]
		}
		views = append(views, view)
	}
	return views, accounts, nil
}

func quotesOf(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, rows []repositories.MessageRow,
) (map[domain.MessageID]*Quote, []repositories.MessageRow, error) {
	ids := make([]domain.MessageID, 0, len(rows))
	seen := map[domain.MessageID]bool{}
	for _, row := range rows {
		if row.ReplyToMessageID != nil && !seen[*row.ReplyToMessageID] {
			seen[*row.ReplyToMessageID] = true
			ids = append(ids, *row.ReplyToMessageID)
		}
	}
	quoted, err := repos.Messages.Views(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	quotes := make(map[domain.MessageID]*Quote, len(quoted))
	for _, row := range quoted {
		quote := quoteOf(viewer, row)
		quotes[row.ID] = &quote
	}
	return quotes, quoted, nil
}

func quoteOf(viewer domain.UserID, row repositories.MessageRow) Quote {
	from := senderOf(viewer, row)
	state := domain.QuoteAvailable
	switch {
	case row.ErasedAt != nil:
		state = domain.QuoteRemoved
	case row.DeletedAt != nil:
		state = domain.QuoteDeleted
	}
	return Quote{
		MessageID: row.ID, Seq: row.Seq, SenderLabel: from.label, SenderLabelKind: from.kind, Own: from.own, Preview: previewOf(row), State: state,
		SentBeforeReclaim: row.SentBeforeReclaim,
	}
}

func (s *Service) viewOf(ctx context.Context, repos repositories.Repositories, viewer domain.UserID, id domain.MessageID, now time.Time) (Message, error) {
	row, err := repos.Messages.View(ctx, id)
	if err != nil {
		return Message{}, err
	}
	views, err := s.messageViews(ctx, repos, viewer, []repositories.MessageRow{row}, now)
	if err != nil {
		return Message{}, err
	}
	return views[0], nil
}

func lastMessages(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, channels []domain.ChannelID,
) (map[domain.ChannelID]*LastMessage, []domain.UserID, error) {
	rows, err := repos.Messages.LastOf(ctx, channels)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]domain.MessageID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	files, err := repos.Messages.AttachmentsOf(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	counts := map[domain.MessageID]int64{}
	for _, file := range files {
		counts[domain.MessageID(file.MessageID)]++
	}
	last := make(map[domain.ChannelID]*LastMessage, len(rows))
	for _, row := range rows {
		from := senderOf(viewer, row)
		last[row.ChannelID] = &LastMessage{
			Seq: row.Seq, CreatedAt: row.CreatedAt, Own: from.own, SenderLabel: from.label, SenderLabelKind: from.kind, Preview: previewOf(row),
			AttachmentCount: counts[row.ID], Deleted: row.DeletedAt != nil, Erased: row.ErasedAt != nil,
		}
	}
	return last, messageAccounts(rows), nil
}

func previewOf(row repositories.MessageRow) *string {
	if row.Body == nil || row.DeletedAt != nil || row.ErasedAt != nil || strings.TrimSpace(*row.Body) == "" {
		return nil
	}
	preview := []rune(*row.Body)
	if len(preview) > domain.MessagePreviewCodePoints {
		preview = preview[:domain.MessagePreviewCodePoints]
	}
	text := string(preview)
	return &text
}

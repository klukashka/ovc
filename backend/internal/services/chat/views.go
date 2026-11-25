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

func senderOf(viewer domain.UserID, row repositories.MessageRow) sender {
	switch {
	case row.SenderID == nil:
		return deletedUser()
	case *row.SenderID == viewer:
		return sender{own: true}
	case row.SenderState == string(domain.AccountDeleted):
		return deletedUser()
	case viewer == row.TutorID:
		if row.CardName == "" {
			return deletedUser()
		}
		name := row.CardName
		return sender{label: &name}
	case row.SenderName == nil:
		return deletedUser()
	}
	name := *row.SenderName
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

func (s *Service) messageViews(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, rows []repositories.MessageRow, now time.Time,
) ([]Message, error) {
	ids := make([]domain.MessageID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	files, err := repos.Messages.AttachmentsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMessage := map[domain.MessageID][]repositories.MessageAttachment{}
	for _, file := range files {
		message := domain.MessageID(file.MessageID)
		byMessage[message] = append(byMessage[message], file)
	}
	quotes, err := quotesOf(ctx, repos, viewer, rows)
	if err != nil {
		return nil, err
	}
	views := make([]Message, 0, len(rows))
	for _, row := range rows {
		view := s.messageView(viewer, row, byMessage[row.ID], now)
		if row.ReplyToMessageID != nil {
			view.ReplyTo = quotes[*row.ReplyToMessageID]
		}
		views = append(views, view)
	}
	return views, nil
}

func quotesOf(
	ctx context.Context, repos repositories.Repositories, viewer domain.UserID, rows []repositories.MessageRow,
) (map[domain.MessageID]*Quote, error) {
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
		return nil, err
	}
	quotes := make(map[domain.MessageID]*Quote, len(quoted))
	for _, row := range quoted {
		quote := quoteOf(viewer, row)
		quotes[row.ID] = &quote
	}
	return quotes, nil
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
) (map[domain.ChannelID]*LastMessage, error) {
	rows, err := repos.Messages.LastOf(ctx, channels)
	if err != nil {
		return nil, err
	}
	ids := make([]domain.MessageID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	files, err := repos.Messages.AttachmentsOf(ctx, ids)
	if err != nil {
		return nil, err
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
			AttachmentCount: counts[row.ID],
		}
	}
	return last, nil
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

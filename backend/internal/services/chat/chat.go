package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	cursorSeparator          = "."
	fieldCursor              = "query.cursor"
	fieldReplyTo             = "body.reply_to_message_id"
	fieldPinnedMessage       = "body.message_id"
	fieldAroundSeq           = "query.around_seq"
	erasureBatch       int32 = 100
)

var (
	ErrNotFound         = errors.New("the channel is outside the caller's channels")
	ErrUnauthenticated  = errors.New("the session or the account no longer allows the write")
	ErrReadOnly         = errors.New("the channel is read-only")
	ErrEditWindowPassed = errors.New("the message can no longer be edited")
	ErrConflict         = errors.New("the message changed since the caller read it")
	ErrQuotaExceeded    = errors.New("the tutor's file quota has no room")
)

type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return "too many message writes"
}

type NotFoundAtError struct {
	Path string
}

func (e *NotFoundAtError) Error() string {
	return "nothing the caller may reference was found at " + e.Path
}

type ValidationError struct {
	Path string
	Code domain.FieldCode
}

func (e *ValidationError) Error() string {
	return "the request is invalid at " + e.Path
}

type Addresses interface {
	ChatAddress(attachment domain.AttachmentID, message domain.MessageID, disposition domain.Disposition, now time.Time) (string, time.Time)
}

type ErasureSettings struct {
	Wall         func() time.Time
	Budget       time.Duration
	MessageBatch int32
}

func DefaultErasureSettings() ErasureSettings {
	return ErasureSettings{Wall: clock.System{}.Now, Budget: 20 * time.Second, MessageBatch: 500}
}

type PresenceCleanupSettings struct {
	Wall   func() time.Time
	Budget time.Duration
	Batch  int32
}

func DefaultPresenceCleanupSettings() PresenceCleanupSettings {
	return PresenceCleanupSettings{Wall: clock.System{}.Now, Budget: 20 * time.Second, Batch: 500}
}

type Dependencies struct {
	Digest          DigestSettings
	PresenceCleanup PresenceCleanupSettings
	Store           *repositories.Store
	Clock           clock.Source
	Random          core.RandomSource
	Addresses       Addresses
	Erasure         ErasureSettings
	Logger          *slog.Logger
	Scheduler       core.Scheduler
	Search          SearchSettings
}

type Service struct {
	deps     Dependencies
	writes   *fixedWindowLimit
	searches *fixedWindowLimit
	slots    chan struct{}
	reads    *readSignals
}

func New(dependencies Dependencies) *Service {
	if dependencies.Digest.Wall == nil {
		dependencies.Digest = DefaultDigestSettings()
	}
	if dependencies.PresenceCleanup.Wall == nil {
		dependencies.PresenceCleanup = DefaultPresenceCleanupSettings()
	}
	if dependencies.Erasure.Wall == nil {
		dependencies.Erasure = DefaultErasureSettings()
	}
	if dependencies.Scheduler == nil {
		panic("chat: the read signal scheduler is required")
	}
	if dependencies.Search.StatementTimeout == 0 {
		dependencies.Search = DefaultSearchSettings()
	}
	service := &Service{
		deps: dependencies, writes: newWriteLimit(), searches: newSearchLimit(), slots: make(chan struct{}, domain.MessageSearchSlots),
	}
	service.reads = newReadSignals(dependencies.Scheduler, dependencies.Logger, service.signalRead)
	return service
}

type TutorLabel struct {
	Name   string
	Colour *domain.Colour
}

type Member struct {
	UserID domain.UserID
	Label  string
	Online *bool
}

type Channel struct {
	ID             domain.ChannelID
	Kind           domain.ChannelKind
	Label          *string
	LabelKind      *domain.LabelKind
	Colour         *domain.Colour
	Tutor          *TutorLabel
	UserID         *domain.UserID
	UnreadCount    int64
	Writable       bool
	Online         *bool
	Unconfirmed    bool
	RelationshipID *domain.RelationshipID
	GroupID        *domain.GroupID
	Members        []Member
	TutorView      bool
	LastMessage    *LastMessage
	Pinned         *Pinned
	PeerReadSeq    *int64
}

type Query struct {
	Search string
	Cursor string
	Limit  int32
}

type Page struct {
	Items          []Channel
	NextCursor     *string
	UnreadChannels int64
}

func (s *Service) List(ctx context.Context, viewer domain.UserID, query Query) (Page, error) {
	now, err := s.now(ctx)
	if err != nil {
		return Page{}, err
	}
	listing := repositories.ChannelQuery{Now: now, Member: viewer, Limit: query.Limit + 1}
	if query.Search != "" {
		listing.Search = &query.Search
	}
	if query.Cursor != "" {
		after, valid := decodeCursor(query.Cursor)
		if !valid {
			return Page{}, &ValidationError{Path: fieldCursor, Code: domain.FieldInvalid}
		}
		listing.After = &after
	}
	var page Page
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Channels.List(ctx, listing)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := encodeCursor(repositories.ChannelCursor{ActivityAt: last.LastActivityAt, ID: last.ChannelID})
			page.NextCursor = &next
		}
		ids := make([]domain.ChannelID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ChannelID)
		}
		last, err := lastMessages(ctx, repos, viewer, ids)
		if err != nil {
			return err
		}
		page.Items = make([]Channel, 0, len(rows))
		for _, row := range rows {
			channel := channelView(viewer, row)
			channel.LastMessage = last[row.ChannelID]
			page.Items = append(page.Items, channel)
		}
		page.UnreadChannels, err = repos.Channels.CountUnread(ctx, viewer, domain.UnreadChannelsCap)
		return err
	})
	return page, err
}

func (s *Service) Channel(ctx context.Context, viewer domain.UserID, id domain.ChannelID) (Channel, error) {
	now, err := s.now(ctx)
	if err != nil {
		return Channel{}, err
	}
	var channel Channel
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Channels.List(ctx, repositories.ChannelQuery{Now: now, Member: viewer, Channel: &id, Limit: 1})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		channel = channelView(viewer, rows[0])
		last, err := lastMessages(ctx, repos, viewer, []domain.ChannelID{id})
		if err != nil {
			return err
		}
		channel.LastMessage = last[id]
		if channel.Pinned, err = pinnedView(ctx, repos, viewer, id); err != nil {
			return err
		}
		if channel.PeerReadSeq, err = repos.Messages.PeerReadSeq(ctx, id, viewer); err != nil {
			return err
		}
		if rows[0].TutorID != viewer {
			return nil
		}
		channel.TutorView = true
		channel.RelationshipID, channel.GroupID = rows[0].RelationshipID, rows[0].GroupID
		labels, err := repos.Channels.MemberLabels(ctx, id, now)
		if err != nil {
			return err
		}
		channel.Members = make([]Member, 0, len(labels))
		for _, label := range labels {
			name, err := domain.CardName(label.DisplayName)
			if err != nil {
				return err
			}
			channel.Members = append(channel.Members, Member{UserID: label.UserID, Label: name, Online: presenceOf(channel.Writable, label.Online)})
		}
		return nil
	})
	return channel, err
}

func (s *Service) UnreadChannels(ctx context.Context, viewer domain.UserID) (int64, error) {
	return s.deps.Store.Repositories().Channels.CountUnread(ctx, viewer, domain.UnreadChannelsCap)
}

func channelView(viewer domain.UserID, row repositories.ChannelRow) Channel {
	channel := Channel{
		ID: row.ChannelID, Kind: row.Kind, Writable: row.ReadOnlySince == nil,
		UnreadCount: min(row.LastSeq-row.LastReadSeq, domain.UnreadCountCap),
	}
	if row.TutorID == viewer {
		label, colour := row.TutorLabel, domain.Colour(row.TutorColour)
		channel.Label, channel.Colour = &label, &colour
		if row.CardNameErased {
			kind := domain.ErasedCardLabel(row.CardDiscardReason)
			channel.Label, channel.LabelKind = nil, &kind
		}
		if row.Kind == domain.ChannelDirect {
			channel.Unconfirmed = row.Provisional
			if row.OtherPartyActive {
				channel.UserID = row.CardStudentID
			}
		}
	} else {
		channel.Colour = row.OwnColour
		tutor := row.TutorName
		switch row.Kind {
		case domain.ChannelDirect:
			channel.Label = &tutor
			if row.OtherPartyActive {
				other := row.TutorID
				channel.UserID = &other
			}
		case domain.ChannelGroup:
			channel.Label = row.GroupName
			channel.Tutor = &TutorLabel{Name: tutor, Colour: row.OwnColour}
		}
	}
	if row.Kind == domain.ChannelDirect && channel.UserID != nil {
		channel.Online = presenceOf(channel.Writable, row.OtherPartyOnline)
	}
	return channel
}

func presenceOf(writable bool, online bool) *bool {
	if !writable {
		return nil
	}
	return &online
}

func encodeCursor(cursor repositories.ChannelCursor) string {
	text := strconv.FormatInt(cursor.ActivityAt.UnixMicro(), 10) + cursorSeparator + cursor.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func decodeCursor(text string) (repositories.ChannelCursor, bool) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return repositories.ChannelCursor{}, false
	}
	micros, idText, found := strings.Cut(string(decoded), cursorSeparator)
	if !found {
		return repositories.ChannelCursor{}, false
	}
	instant, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return repositories.ChannelCursor{}, false
	}
	id, valid := domain.ParseUUID[domain.ChannelID](idText)
	if !valid {
		return repositories.ChannelCursor{}, false
	}
	return repositories.ChannelCursor{ActivityAt: time.UnixMicro(instant).UTC(), ID: id}, true
}

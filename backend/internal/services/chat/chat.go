package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	cursorSeparator       = "."
	fieldCursor           = "query.cursor"
	erasureBatch    int32 = 100
)

var ErrNotFound = errors.New("the channel is outside the caller's channels")

type ValidationError struct {
	Path string
	Code domain.FieldCode
}

func (e *ValidationError) Error() string {
	return "the request is invalid at " + e.Path
}

type Dependencies struct {
	Store  *repositories.Store
	Logger *slog.Logger
}

type Service struct {
	deps Dependencies
}

func New(dependencies Dependencies) *Service {
	return &Service{deps: dependencies}
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
	listing := repositories.ChannelQuery{Member: viewer, Limit: query.Limit + 1}
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
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
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
		page.Items = make([]Channel, 0, len(rows))
		for _, row := range rows {
			page.Items = append(page.Items, channelView(viewer, row))
		}
		page.UnreadChannels, err = repos.Channels.CountUnread(ctx, viewer, domain.UnreadChannelsCap)
		return err
	})
	return page, err
}

func (s *Service) Channel(ctx context.Context, viewer domain.UserID, id domain.ChannelID) (Channel, error) {
	var channel Channel
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Channels.List(ctx, repositories.ChannelQuery{Member: viewer, Channel: &id, Limit: 1})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		channel = channelView(viewer, rows[0])
		if rows[0].TutorID != viewer {
			return nil
		}
		channel.TutorView = true
		channel.RelationshipID, channel.GroupID = rows[0].RelationshipID, rows[0].GroupID
		labels, err := repos.Channels.MemberLabels(ctx, id)
		if err != nil {
			return err
		}
		channel.Members = make([]Member, 0, len(labels))
		for _, label := range labels {
			channel.Members = append(channel.Members, Member{UserID: label.UserID, Label: label.DisplayName, Online: presenceOf(channel.Writable)})
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
		channel.Online = presenceOf(channel.Writable)
	}
	return channel
}

func presenceOf(writable bool) *bool {
	if !writable {
		return nil
	}
	offline := false
	return &offline
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

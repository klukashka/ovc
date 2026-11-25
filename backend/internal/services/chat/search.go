package chat

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	fieldSearchTerm   = "body.q"
	fieldSearchCursor = "body.cursor"
)

var (
	ErrSearchBusy     = errors.New("every message search slot is taken")
	ErrSearchTimedOut = errors.New("the message search ran out of time")
)

type SearchSettings struct {
	StatementTimeout time.Duration
	QueueWait        time.Duration
}

func DefaultSearchSettings() SearchSettings {
	return SearchSettings{StatementTimeout: domain.MessageSearchTimeout, QueueWait: domain.MessageSearchQueueWait}
}

type SearchRequest struct {
	Term   string
	Cursor *string
}

type SearchPage struct {
	Items      []Message
	NextCursor *string
}

func SearchTermLength(term string) int64 {
	return int64(utf8.RuneCountInString(strings.TrimSpace(term)))
}

func validSearchTerm(term string) (string, *ValidationError) {
	trimmed := strings.TrimSpace(term)
	length := utf8.RuneCountInString(trimmed)
	if length < domain.MessageSearchMinTerm {
		return "", &ValidationError{Path: fieldSearchTerm, Code: domain.FieldTooShort}
	}
	if length > domain.MessageSearchMaxTerm {
		return "", &ValidationError{Path: fieldSearchTerm, Code: domain.FieldTooLong}
	}
	for _, character := range trimmed {
		if unicode.IsControl(character) {
			return "", &ValidationError{Path: fieldSearchTerm, Code: domain.FieldInvalid}
		}
	}
	return trimmed, nil
}

func (s *Service) Search(ctx context.Context, viewer domain.UserID, channel domain.ChannelID, request SearchRequest) (SearchPage, error) {
	term, invalid := validSearchTerm(request.Term)
	if invalid != nil {
		return SearchPage{}, invalid
	}
	var before *int64
	if request.Cursor != nil {
		seq, valid := decodeSeqCursor(*request.Cursor)
		if !valid {
			return SearchPage{}, &ValidationError{Path: fieldSearchCursor, Code: domain.FieldInvalid}
		}
		before = &seq
	}
	now, err := s.now(ctx)
	if err != nil {
		return SearchPage{}, err
	}
	release, retryAfter, allowed := s.searches.take(viewer, now)
	if !allowed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.rate_limited", userAttribute(viewer), slog.String("limit", string(domain.LimitSearches)))
		return SearchPage{}, &ThrottledError{RetryAfter: retryAfter}
	}
	if err := s.searchable(ctx, viewer, channel); err != nil {
		release()
		return SearchPage{}, err
	}
	freeSlot, err := s.takeSearchSlot(ctx, viewer, channel)
	if err != nil {
		return SearchPage{}, err
	}
	defer freeSlot()
	page, err := s.search(ctx, viewer, channel, domain.LikeLiteral(term), before, now)
	switch {
	case errors.Is(err, ErrNotFound):
		release()
		return SearchPage{}, err
	case repositories.IsStatementTimeout(ctx, err):
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "chat.search_timed_out", channelAttribute(channel), userAttribute(viewer))
		return SearchPage{}, ErrSearchTimedOut
	case err != nil:
		return SearchPage{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "chat.searched", channelAttribute(channel), userAttribute(viewer),
		slog.Int("result_count", len(page.Items)), slog.Bool("has_more", page.NextCursor != nil))
	return page, nil
}

func (s *Service) searchable(ctx context.Context, viewer domain.UserID, channel domain.ChannelID) error {
	return s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if _, err := repos.Messages.Membership(ctx, channel, viewer); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return nil
	})
}

func (s *Service) takeSearchSlot(ctx context.Context, viewer domain.UserID, channel domain.ChannelID) (func(), error) {
	free := func() { <-s.slots }
	select {
	case s.slots <- struct{}{}:
		return free, nil
	default:
	}
	expired := make(chan struct{})
	timer := s.deps.Scheduler.AfterFunc(s.deps.Search.QueueWait, func() { close(expired) })
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		return free, nil
	case <-expired:
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.search_busy", channelAttribute(channel), userAttribute(viewer))
		return nil, ErrSearchBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) search(
	ctx context.Context, viewer domain.UserID, channel domain.ChannelID, pattern string, before *int64, now time.Time,
) (SearchPage, error) {
	var page SearchPage
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := repos.Messages.LimitStatements(ctx, s.deps.Search.StatementTimeout); err != nil {
			return err
		}
		if _, err := repos.Messages.Membership(ctx, channel, viewer); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		rows, err := repos.Messages.Search(ctx, repositories.SearchQuery{
			Channel: channel, Pattern: pattern, Before: before, RowLimit: domain.MessageSearchPage + 1,
		})
		if err != nil {
			return err
		}
		if len(rows) > domain.MessageSearchPage {
			rows = rows[:domain.MessageSearchPage]
			next := encodeSeqCursor(rows[len(rows)-1].Seq)
			page.NextCursor = &next
		}
		page.Items, err = s.messageViews(ctx, repos, viewer, rows, now)
		return err
	})
	return page, err
}

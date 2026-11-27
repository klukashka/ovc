package api

import (
	"context"
	"log/slog"
	"sync"
)

type accessCount string

const (
	accessRecentUpdates     accessCount = "recent_updates"
	accessTodayLessons      accessCount = "today_lessons"
	accessNeedsChecking     accessCount = "needs_checking"
	accessUpcomingDeadlines accessCount = "upcoming_deadlines"
	accessUnreadChannels    accessCount = "unread_channels"
	accessJoinRequests      accessCount = "join_requests"
	accessTodo              accessCount = "todo"
	accessRecentlyChecked   accessCount = "recently_checked"
	accessNextSevenDays     accessCount = "next_seven_days"
	accessTutors            accessCount = "tutors"
	accessQueryLength       accessCount = "query_length"
)

type accessCountsKey struct{}

type recordedCount struct {
	key   accessCount
	value int64
}

type accessCounts struct {
	mu     sync.Mutex
	counts []recordedCount
}

func withAccessCounts(ctx context.Context) context.Context {
	return context.WithValue(ctx, accessCountsKey{}, &accessCounts{})
}

func recordAccessCount(ctx context.Context, key accessCount, value int64) {
	holder, found := ctx.Value(accessCountsKey{}).(*accessCounts)
	if !found {
		panic("an access count was recorded outside the request middleware")
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.counts = append(holder.counts, recordedCount{key: key, value: value})
}

func accessCountAttributes(ctx context.Context) []slog.Attr {
	holder, found := ctx.Value(accessCountsKey{}).(*accessCounts)
	if !found {
		return nil
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	attributes := make([]slog.Attr, 0, len(holder.counts))
	for _, count := range holder.counts {
		attributes = append(attributes, slog.Int64(string(count.key), count.value))
	}
	return attributes
}

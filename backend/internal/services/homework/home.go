package homework

import (
	"context"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type UpcomingDeadlines struct {
	Items []IssueItem
	Count int64
}

func (s *Service) UpcomingDeadlines(ctx context.Context, tutor domain.UserID) (UpcomingDeadlines, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return UpcomingDeadlines{}, err
	}
	before := now.Add(domain.HomeDeadlineWindow)
	upcoming := UpcomingDeadlines{Items: []IssueItem{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Homework.Issues(ctx, repositories.IssueListQuery{
			Tutor: tutor, Status: domain.IssueActive, DeadlineBefore: &before, Now: now, PageSize: domain.HomePanelLimit,
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			upcoming.Items = append(upcoming.Items, itemOfRow(row))
		}
		upcoming.Count, err = repos.Homework.IssuesByDeadlineCount(ctx, tutor, now, before, domain.HomeCountCap)
		if err != nil {
			return err
		}
		return s.complete(ctx, repos, tutor, upcoming.Items, nil)
	})
	return upcoming, err
}

package homework

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type ReviewQueueQuery struct {
	Issue   *domain.IssueID
	Group   *domain.GroupID
	Student *domain.RelationshipID
	Cursor  string
	Limit   int32
}

type ReviewQueueItem struct {
	Submission          domain.SubmissionID
	Issue               domain.IssueID
	Title               string
	Relationship        domain.RelationshipID
	DisplayName         string
	SubmittedAt         *time.Time
	FirstSubmittedAt    *time.Time
	Late                bool
	HasDraft            bool
	IssuedBeforeReclaim bool
}

type ReviewQueuePage struct {
	Items      []ReviewQueueItem
	NextCursor *string
	Count      int64
}

func (s *Service) ReviewQueue(ctx context.Context, tutor domain.UserID, query ReviewQueueQuery) (ReviewQueuePage, error) {
	after, afterID, err := parseCursor[domain.SubmissionID](query.Cursor)
	if err != nil {
		return ReviewQueuePage{}, err
	}
	page := ReviewQueuePage{Items: []ReviewQueueItem{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := reviewFiltersInScope(ctx, repos, tutor, query); err != nil {
			return err
		}
		filter := repositories.ReviewQueueQuery{
			Tutor: tutor, Issue: query.Issue, Group: query.Group, Relationship: query.Student, AfterFirstSubmittedAt: after,
			AfterID: afterID, PageSize: query.Limit + 1,
		}
		rows, err := repos.Homework.ReviewQueue(ctx, filter)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			if last.FirstSubmittedAt != nil {
				next := encodeCursor(*last.FirstSubmittedAt, last.ID)
				page.NextCursor = &next
			}
		}
		for _, row := range rows {
			page.Items = append(page.Items, ReviewQueueItem{
				Submission: row.ID, Issue: row.IssueID, Title: row.Title, Relationship: row.RelationshipID, DisplayName: row.DisplayName,
				SubmittedAt: row.SubmittedAt, FirstSubmittedAt: row.FirstSubmittedAt, Late: row.Late, HasDraft: row.HasDraft,
				IssuedBeforeReclaim: row.IssuedBeforeReclaim,
			})
		}
		page.Count, err = repos.Homework.ReviewQueueCount(ctx, filter, s.deps.Caps.ReviewQueueCount)
		return err
	})
	return page, err
}

func reviewFiltersInScope(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, query ReviewQueueQuery) error {
	checks := []func() error{}
	if query.Issue != nil {
		checks = append(checks, func() error { return repos.Homework.ReviewIssueInScope(ctx, tutor, *query.Issue) })
	}
	if query.Group != nil {
		checks = append(checks, func() error { return repos.Homework.GroupInScope(ctx, tutor, *query.Group) })
	}
	if query.Student != nil {
		checks = append(checks, func() error { return repos.Homework.StudentInScope(ctx, tutor, *query.Student) })
	}
	for _, check := range checks {
		err := check()
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
	}
	return nil
}

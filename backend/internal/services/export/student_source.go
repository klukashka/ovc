package export

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

type submissionPair struct {
	work homework.StudentWork
}

type studentSource struct {
	content  Content
	reader   pageReader
	student  domain.UserID
	pageSize int32
}

func studentAssignmentGroups() []domain.AssignmentGroup {
	return []domain.AssignmentGroup{domain.AssignmentsTodo, domain.AssignmentsSubmitted, domain.AssignmentsChecked, domain.AssignmentsClosed}
}

func (s studentSource) tutors(ctx context.Context) ([]tutoring.MyTutor, error) {
	var tutors []tutoring.MyTutor
	err := s.reader.page(ctx, func(repos repositories.Repositories, _ time.Time) ([]domain.UserID, error) {
		found, referenced, err := s.content.Tutors.StudentTutorsIn(ctx, repos, s.student)
		tutors = found
		return referenced, err
	})
	return tutors, err
}

func (s studentSource) lessons(
	ctx context.Context, within func(repositories.Repositories, time.Time, []schedule.Lesson) error, visit func([]schedule.Lesson) error,
) error {
	after := schedule.LessonCursor{}
	seen := map[domain.LessonID]bool{}
	for {
		var page schedule.ExportLessonPage
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			found, err := s.content.Lessons.StudentLessonPageIn(ctx, repos, s.student, after, s.pageSize)
			if err != nil {
				return nil, err
			}
			page = found
			if within != nil {
				if err := within(repos, now, page.Lessons); err != nil {
					return nil, err
				}
			}
			return page.Accounts, nil
		})
		if err != nil {
			return err
		}
		fresh := make([]schedule.Lesson, 0, len(page.Lessons))
		for _, lesson := range page.Lessons {
			if !seen[lesson.ID] {
				seen[lesson.ID] = true
				fresh = append(fresh, lesson)
			}
		}
		if visit != nil {
			if err := visit(fresh); err != nil {
				return err
			}
		}
		if page.Next == nil {
			return nil
		}
		after = *page.Next
	}
}

func (s studentSource) assignments(ctx context.Context) ([]homework.AssignmentItem, error) {
	var items []homework.AssignmentItem
	err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
		items = nil
		seen := map[domain.SubmissionID]bool{}
		for _, group := range studentAssignmentGroups() {
			cursor := ""
			for {
				page, err := s.content.Homework.AssignmentsIn(ctx, repos, s.student,
					homework.AssignmentQuery{Group: group, Cursor: cursor, Limit: s.pageSize}, now)
				if err != nil {
					return nil, err
				}
				for _, item := range page.Items {
					if !seen[item.Submission] {
						seen[item.Submission] = true
						items = append(items, item)
					}
				}
				if page.NextCursor == nil {
					break
				}
				cursor = *page.NextCursor
			}
		}
		return nil, nil
	})
	return items, err
}

func (s studentSource) submissions(
	ctx context.Context, within func(repositories.Repositories, time.Time, []submissionPair) error, visit func([]submissionPair) error,
) error {
	items, err := s.assignments(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(items); start += int(s.pageSize) {
		chunk := items[start:min(start+int(s.pageSize), len(items))]
		var pairs []submissionPair
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			pairs = nil
			var referenced []domain.UserID
			for _, item := range chunk {
				work, accounts, err := s.content.Homework.StudentSubmissionIn(ctx, repos, s.student, item.Submission, now)
				if errors.Is(err, homework.ErrNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				pairs = append(pairs, submissionPair{work: work})
				referenced = append(referenced, accounts...)
			}
			if within != nil {
				if err := within(repos, now, pairs); err != nil {
					return nil, err
				}
			}
			return referenced, nil
		})
		if err != nil {
			return err
		}
		if visit != nil {
			if err := visit(pairs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s studentSource) channels(ctx context.Context) ([]chat.Channel, error) {
	var channels []chat.Channel
	err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
		channels = nil
		var referenced []domain.UserID
		cursor := ""
		for {
			page, accounts, err := s.content.Chat.ListIn(ctx, repos, s.student, chat.Query{Filter: domain.ChannelFilterAll, Cursor: cursor, Limit: s.pageSize}, now)
			if err != nil {
				return nil, err
			}
			channels = append(channels, page.Items...)
			referenced = append(referenced, accounts...)
			if page.NextCursor == nil {
				return referenced, nil
			}
			cursor = *page.NextCursor
		}
	})
	return channels, err
}

func (s studentSource) messages(
	ctx context.Context, channel domain.ChannelID, within func(repositories.Repositories, time.Time, []chat.Message) error,
	visit func([]chat.Message) error,
) error {
	cursor := ""
	for {
		var history chat.History
		gone := false
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			found, referenced, err := s.content.Chat.HistoryIn(ctx, repos, s.student, channel, chat.HistoryQuery{Cursor: cursor, Limit: s.pageSize}, now)
			if errors.Is(err, chat.ErrNotFound) {
				gone = true
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			history = found
			if within != nil {
				if err := within(repos, now, history.Items); err != nil {
					return nil, err
				}
			}
			return referenced, nil
		})
		if err != nil || gone {
			return err
		}
		if visit != nil {
			if err := visit(history.Items); err != nil {
				return err
			}
		}
		if history.NextCursor == nil {
			return nil
		}
		cursor = *history.NextCursor
	}
}

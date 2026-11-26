package schedule

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type LessonCursor struct {
	StartsAt time.Time
	ID       domain.LessonID
}

type ExportLessonPage struct {
	Lessons  []Lesson
	Next     *LessonCursor
	Accounts []domain.UserID
}

type noAddresses struct{}

func (noAddresses) LessonAddress(domain.AttachmentID, domain.LessonID, domain.Disposition, time.Time) (string, time.Time) {
	return "", time.Time{}
}

func (s *Service) withoutAddresses() *Service {
	free := *s
	free.deps.Addresses = noAddresses{}
	return &free
}

func (s *Service) StudentLessonPageIn(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, after LessonCursor, limit int32,
) (ExportLessonPage, error) {
	rows, err := repos.Lessons.StudentCalendarAfter(ctx, student, after.StartsAt, after.ID, limit+1)
	if err != nil {
		return ExportLessonPage{}, err
	}
	page := ExportLessonPage{}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.Next = &LessonCursor{StartsAt: last.StartsAt, ID: last.ID}
	}
	account, err := repos.Users.ByID(ctx, student)
	if err != nil {
		return ExportLessonPage{}, err
	}
	free := s.withoutAddresses()
	seen := map[domain.LessonID]bool{}
	for _, row := range rows {
		if seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		lesson, err := free.studentLesson(ctx, repos, account, row.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return ExportLessonPage{}, err
		}
		page.Lessons = append(page.Lessons, lesson)
		page.Accounts = append(page.Accounts, row.TutorID)
	}
	return page, nil
}

func (s *Service) TutorLessonPageIn(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, after LessonCursor, limit int32,
) (ExportLessonPage, error) {
	rows, err := repos.Lessons.TutorCalendarAfter(ctx, tutor, after.StartsAt, after.ID, limit+1)
	if err != nil {
		return ExportLessonPage{}, err
	}
	page := ExportLessonPage{}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.Next = &LessonCursor{StartsAt: last.StartsAt, ID: last.ID}
	}
	account, err := repos.Users.ByID(ctx, tutor)
	if err != nil {
		return ExportLessonPage{}, err
	}
	free := s.withoutAddresses()
	for _, row := range rows {
		for _, referenced := range row.AccountIds {
			if referenced.Valid {
				page.Accounts = append(page.Accounts, domain.UserID(referenced.Bytes))
			}
		}
		lesson, err := free.tutorLesson(ctx, repos, account, row.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return ExportLessonPage{}, err
		}
		page.Lessons = append(page.Lessons, lesson)
	}
	return page, nil
}

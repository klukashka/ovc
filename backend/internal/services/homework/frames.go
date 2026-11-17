package homework

import (
	"context"
	"slices"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type frameEntry struct {
	signal        domain.EntitySignal
	student       domain.UserID
	toStudent     bool
	lessonChoices []domain.LessonID
}

type frameBatch struct {
	entries []frameEntry
}

func lessonsOf(links ...*domain.LessonID) []domain.LessonID {
	var lessons []domain.LessonID
	for _, link := range links {
		if link != nil && !slices.Contains(lessons, *link) {
			lessons = append(lessons, *link)
		}
	}
	return lessons
}

func (b *frameBatch) tutorIssue(tutor domain.UserID, issue domain.IssueID, lessons []domain.LessonID) {
	b.entries = append(b.entries, frameEntry{signal: domain.EntitySignal{
		Users: []domain.UserID{tutor}, Entity: domain.EntityIssue, ID: domain.EntityID(issue), Lessons: lessons,
	}})
}

func (b *frameBatch) tutorSubmission(tutor domain.UserID, submission domain.SubmissionID) {
	b.entries = append(b.entries, frameEntry{signal: domain.EntitySignal{
		Users: []domain.UserID{tutor}, Entity: domain.EntitySubmission, ID: domain.EntityID(submission),
	}})
}

func (b *frameBatch) tutorSubmissionOfIssue(tutor domain.UserID, submission domain.SubmissionID, issue domain.IssueID, lessons []domain.LessonID) {
	b.entries = append(b.entries, frameEntry{signal: domain.EntitySignal{
		Users: []domain.UserID{tutor}, Entity: domain.EntitySubmission, ID: domain.EntityID(submission), Issue: &issue, Lessons: lessons,
	}})
}

func (b *frameBatch) studentSubmission(student domain.UserID, submission domain.SubmissionID, lessons []domain.LessonID) {
	b.entries = append(b.entries, frameEntry{
		signal:  domain.EntitySignal{Users: []domain.UserID{student}, Entity: domain.EntitySubmission, ID: domain.EntityID(submission)},
		student: student, toStudent: true, lessonChoices: lessons,
	})
}

func (b *frameBatch) send(ctx context.Context, repos repositories.Repositories) error {
	visible, err := b.visibleLessons(ctx, repos)
	if err != nil {
		return err
	}
	signals := make([]domain.EntitySignal, 0, len(b.entries))
	for _, entry := range b.entries {
		signal := entry.signal
		if entry.toStudent {
			for _, lesson := range entry.lessonChoices {
				if visible[entry.student][lesson] {
					signal.Lessons = append(signal.Lessons, lesson)
				}
			}
		}
		signals = append(signals, signal)
	}
	return repos.Signals.Entities(ctx, signals)
}

func (b *frameBatch) visibleLessons(ctx context.Context, repos repositories.Repositories) (map[domain.UserID]map[domain.LessonID]bool, error) {
	var students []domain.UserID
	choices := map[domain.UserID][]domain.LessonID{}
	for _, entry := range b.entries {
		if !entry.toStudent || len(entry.lessonChoices) == 0 {
			continue
		}
		if _, seen := choices[entry.student]; !seen {
			students = append(students, entry.student)
		}
		for _, lesson := range entry.lessonChoices {
			if !slices.Contains(choices[entry.student], lesson) {
				choices[entry.student] = append(choices[entry.student], lesson)
			}
		}
	}
	visible := make(map[domain.UserID]map[domain.LessonID]bool, len(students))
	for _, student := range students {
		rows, err := repos.Lessons.StudentNotificationLessons(ctx, student, choices[student])
		if err != nil {
			return nil, err
		}
		seen := make(map[domain.LessonID]bool, len(rows))
		for _, row := range rows {
			seen[row.ID] = true
		}
		visible[student] = seen
	}
	return visible, nil
}

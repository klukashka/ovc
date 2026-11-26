package export

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

type tutorSource struct {
	content  Content
	reader   pageReader
	tutor    domain.UserID
	pageSize int32
}

type libraryEntry struct {
	item     library.LibraryItem
	template *library.Template
}

func tutorIssueStatuses() []domain.IssueStatus {
	return []domain.IssueStatus{domain.IssueActive, domain.IssueWithdrawn}
}

func (s tutorSource) chat() studentSource {
	return studentSource{content: s.content, reader: s.reader, student: s.tutor, pageSize: s.pageSize}
}

func (s tutorSource) cards(
	ctx context.Context, within func(repositories.Repositories, time.Time, []tutoring.Card) error, visit func([]tutoring.Card) error,
) error {
	cursor := ""
	for {
		var page tutoring.CardPage
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			found, referenced, err := s.content.Cards.CardsIn(ctx, repos, s.tutor,
				tutoring.CardQuery{Filter: domain.CardFilterAll, Cursor: cursor, Limit: s.pageSize}, now)
			if err != nil {
				return nil, err
			}
			page = found
			if within != nil {
				if err := within(repos, now, page.Items); err != nil {
					return nil, err
				}
			}
			return referenced, nil
		})
		if err != nil {
			return err
		}
		if visit != nil {
			if err := visit(page.Items); err != nil {
				return err
			}
		}
		if page.NextCursor == nil {
			return nil
		}
		cursor = *page.NextCursor
	}
}

func (s tutorSource) groups(ctx context.Context) ([]tutoring.Group, error) {
	var groups []tutoring.Group
	err := s.reader.page(ctx, func(repos repositories.Repositories, _ time.Time) ([]domain.UserID, error) {
		found, referenced, err := s.content.Cards.GroupsIn(ctx, repos, s.tutor)
		groups = found
		return referenced, err
	})
	return groups, err
}

func (s tutorSource) lessons(
	ctx context.Context, within func(repositories.Repositories, time.Time, []schedule.Lesson) error, visit func([]schedule.Lesson) error,
) error {
	after := schedule.LessonCursor{}
	seen := map[domain.LessonID]bool{}
	for {
		var page schedule.ExportLessonPage
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			found, err := s.content.TutorLessons.TutorLessonPageIn(ctx, repos, s.tutor, after, s.pageSize)
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

func (s tutorSource) issueIDs(ctx context.Context) ([]domain.IssueID, error) {
	var ids []domain.IssueID
	err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
		ids = nil
		for _, status := range tutorIssueStatuses() {
			cursor := ""
			for {
				page, err := s.content.Issues.IssuesIn(ctx, repos, s.tutor,
					homework.IssueListQuery{Status: status, Cursor: cursor, Limit: s.pageSize}, now)
				if err != nil {
					return nil, err
				}
				for _, item := range page.Items {
					ids = append(ids, item.ID)
				}
				if page.NextCursor == nil {
					break
				}
				cursor = *page.NextCursor
			}
		}
		return nil, nil
	})
	return ids, err
}

func (s tutorSource) issues(
	ctx context.Context, ids []domain.IssueID, within func(repositories.Repositories, time.Time, homework.Issue) error,
	visit func(homework.Issue) error,
) error {
	for _, id := range ids {
		var issue homework.Issue
		gone := false
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			found, referenced, err := s.content.Issues.IssueIn(ctx, repos, s.tutor, id, now)
			if errors.Is(err, homework.ErrNotFound) {
				gone = true
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			issue = found
			if within != nil {
				if err := within(repos, now, issue); err != nil {
					return nil, err
				}
			}
			return referenced, nil
		})
		if err != nil {
			return err
		}
		if gone || visit == nil {
			continue
		}
		if err := visit(issue); err != nil {
			return err
		}
	}
	return nil
}

func (s tutorSource) submissions(
	ctx context.Context, ids []domain.IssueID, within func(repositories.Repositories, time.Time, []homework.TutorWork) error,
	visit func([]homework.TutorWork) error,
) error {
	for _, id := range ids {
		var works []homework.TutorWork
		err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
			works = nil
			issue, _, err := s.content.Issues.IssueIn(ctx, repos, s.tutor, id, now)
			if errors.Is(err, homework.ErrNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			var referenced []domain.UserID
			for _, submission := range issue.Submissions {
				work, accounts, err := s.content.Issues.TutorSubmissionIn(ctx, repos, s.tutor, submission.ID, now)
				if errors.Is(err, homework.ErrNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				works = append(works, work)
				referenced = append(referenced, accounts...)
			}
			if within != nil {
				if err := within(repos, now, works); err != nil {
					return nil, err
				}
			}
			return referenced, nil
		})
		if err != nil {
			return err
		}
		if visit != nil {
			if err := visit(works); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s tutorSource) folders(ctx context.Context) ([]library.Folder, error) {
	var folders []library.Folder
	err := s.reader.page(ctx, func(repos repositories.Repositories, _ time.Time) ([]domain.UserID, error) {
		found, err := s.content.Library.FoldersIn(ctx, repos, s.tutor)
		folders = found
		return nil, err
	})
	return folders, err
}

func (s tutorSource) library(
	ctx context.Context, folders []library.Folder, within func(repositories.Repositories, time.Time, []libraryEntry) error,
	visit func([]libraryEntry) error,
) error {
	places := make([]*domain.FolderID, 0, len(folders)+1)
	places = append(places, nil)
	for index := range folders {
		places = append(places, &folders[index].ID)
	}
	for _, place := range places {
		cursor := ""
		for {
			var entries []libraryEntry
			var next *string
			gone := false
			err := s.reader.page(ctx, func(repos repositories.Repositories, now time.Time) ([]domain.UserID, error) {
				entries, next = nil, nil
				listing, err := s.content.Library.ListingIn(ctx, repos, s.tutor,
					library.ListingQuery{Folder: place, Cursor: cursor, Limit: s.pageSize}, now)
				if errors.Is(err, library.ErrNotFound) {
					gone = true
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				next = listing.NextCursor
				for _, item := range listing.Items {
					entry := libraryEntry{item: item}
					if item.Template != nil {
						template, err := s.content.Library.TemplateIn(ctx, repos, s.tutor, item.Template.ID, now)
						if errors.Is(err, library.ErrNotFound) {
							continue
						}
						if err != nil {
							return nil, err
						}
						entry.template = &template
					}
					entries = append(entries, entry)
				}
				if within != nil {
					if err := within(repos, now, entries); err != nil {
						return nil, err
					}
				}
				return nil, nil
			})
			if err != nil {
				return err
			}
			if gone {
				break
			}
			if visit != nil {
				if err := visit(entries); err != nil {
					return err
				}
			}
			if next == nil {
				break
			}
			cursor = *next
		}
	}
	return nil
}

package export

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

var errNoContentReaders = errors.New("the export builder has no readers of the owning services")

type TutorReader interface {
	StudentTutorsIn(ctx context.Context, repos repositories.Repositories, student domain.UserID) ([]tutoring.MyTutor, []domain.UserID, error)
}

type LessonReader interface {
	StudentLessonPageIn(
		ctx context.Context, repos repositories.Repositories, student domain.UserID, after schedule.LessonCursor, limit int32,
	) (schedule.ExportLessonPage, error)
}

type HomeworkReader interface {
	AssignmentsIn(
		ctx context.Context, repos repositories.Repositories, student domain.UserID, query homework.AssignmentQuery, now time.Time,
	) (homework.AssignmentPage, error)
	StudentSubmissionIn(
		ctx context.Context, repos repositories.Repositories, student domain.UserID, id domain.SubmissionID, now time.Time,
	) (homework.StudentWork, []domain.UserID, error)
}

type ChatReader interface {
	ListIn(ctx context.Context, repos repositories.Repositories, viewer domain.UserID, query chat.Query, now time.Time) (chat.Page, []domain.UserID, error)
	HistoryIn(
		ctx context.Context, repos repositories.Repositories, viewer domain.UserID, channel domain.ChannelID, query chat.HistoryQuery, now time.Time,
	) (chat.History, []domain.UserID, error)
}

type FileAuthorizer interface {
	ExportableFile(
		ctx context.Context, repos repositories.Repositories, viewer domain.UserID, id domain.AttachmentID, reference files.DownloadReference, now time.Time,
	) (repositories.ServableFile, error)
}

type Content struct {
	Tutors   TutorReader
	Lessons  LessonReader
	Homework HomeworkReader
	Chat     ChatReader
	Files    FileAuthorizer
}

func (c Content) complete() bool {
	return c.Tutors != nil && c.Lessons != nil && c.Homework != nil && c.Chat != nil && c.Files != nil
}

package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var ErrUploadAccountingActivation = errors.New("chat upload accounting activation is invalid")

type DailyUploads struct {
	queries *dbgen.Queries
}

func (r DailyUploads) Activation(ctx context.Context) (time.Time, error) {
	rows, err := r.queries.GetChatUploadAccountingActivation(ctx)
	if err != nil {
		return time.Time{}, done(err, "read chat upload accounting activation")
	}
	if len(rows) != 1 || !rows[0].Singleton || rows[0].ActiveAt.IsZero() || !rows[0].ActiveAt.Equal(rows[0].ActiveAt.UTC().Truncate(24*time.Hour)) {
		return time.Time{}, ErrUploadAccountingActivation
	}
	return rows[0].ActiveAt, nil
}

func (r DailyUploads) Bytes(ctx context.Context, student domain.UserID, tutor domain.UserID, day time.Time) (int64, error) {
	total, err := r.queries.GetDailyChatBytes(ctx, dbgen.GetDailyChatBytesParams{StudentID: student, TutorID: tutor, UploadDay: day})
	return one(total, err, "read daily chat bytes")
}

func (r DailyUploads) Add(ctx context.Context, student domain.UserID, tutor domain.UserID, day time.Time, size int64) error {
	return done(r.queries.AddDailyChatBytes(ctx, dbgen.AddDailyChatBytesParams{
		StudentID: student, TutorID: tutor, UploadDay: day, CompletedBytes: size,
	}), "add daily chat bytes")
}

func (r DailyUploads) LockCleanupStudents(ctx context.Context, before time.Time) ([]domain.UserID, error) {
	students, err := r.queries.LockDailyUploadCleanupStudents(ctx, before)
	return one(students, err, "lock daily upload cleanup students")
}

func (r DailyUploads) DeletePastBatch(ctx context.Context, students []domain.UserID, before time.Time) (int64, error) {
	ids := make([]pgtype.UUID, 0, len(students))
	for _, student := range students {
		ids = append(ids, pgtype.UUID{Bytes: student, Valid: true})
	}
	deleted, err := r.queries.DeletePastDailyChatBytes(ctx, dbgen.DeletePastDailyChatBytesParams{Students: ids, BeforeDay: before})
	return counted(deleted, err, "delete past daily chat bytes")
}

func (r DailyUploads) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorDailyChatBytesBatch(ctx, dbgen.DeleteTutorDailyChatBytesBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete daily chat bytes of purged tutor")
}

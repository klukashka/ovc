package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
)

var errFileSizeChanged = errors.New("a stored file's length differs from its recorded size")

type studentBuild struct {
	service    *Service
	claim      Claim
	source     studentSource
	sink       *partSink
	text       core.ExportText
	location   *time.Location
	account    accountRecord
	inventory  *inventory
	counts     map[core.ExportSheet]int
	files      int
	unreadable int
	done       int32
}

func (s *Service) newStudentBuild(reader pageReader, claim Claim, prepared preparedContent) (*studentBuild, error) {
	if !s.deps.Content.complete() {
		return nil, errNoContentReaders
	}
	return &studentBuild{
		service: s, claim: claim, source: studentSource{content: s.deps.Content, reader: reader, student: claim.Export.UserID, pageSize: s.deps.Timing.ReadPage}, sink: reader.sink,
		text: prepared.text, location: prepared.location, account: prepared.account, inventory: newInventory(), counts: map[core.ExportSheet]int{},
	}, nil
}

func (b *studentBuild) admitter() FileAuthorizer {
	return b.service.deps.Content.Files
}

func (b *studentBuild) admitLessons(ctx context.Context, repos repositories.Repositories, now time.Time, lessons []schedule.Lesson) error {
	for _, lesson := range lessons {
		at := reference(domain.ReferenceLesson, lesson.ID)
		for _, material := range lesson.Materials {
			if err := b.inventory.admit(ctx, repos, b.admitter(), b.claim.Export.UserID, material.AttachmentID, at, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *studentBuild) admitSubmissions(ctx context.Context, repos repositories.Repositories, now time.Time, pairs []submissionPair) error {
	for _, pair := range pairs {
		for _, wanted := range submissionFiles(pair.work) {
			if err := b.inventory.admit(ctx, repos, b.admitter(), b.claim.Export.UserID, wanted.attachment, wanted.at, now); err != nil {
				return err
			}
		}
	}
	return nil
}

type wantedFile struct {
	attachment domain.AttachmentID
	at         files.DownloadReference
}

func submissionFiles(work homework.StudentWork) []wantedFile {
	var wanted []wantedFile
	snapshot := reference(domain.ReferenceIssueSnapshot, work.Issue)
	for _, block := range work.Blocks {
		if block.Material != nil {
			wanted = append(wanted, wantedFile{attachment: block.Material.File.Attachment, at: snapshot})
		}
		for _, image := range block.Images {
			wanted = append(wanted, wantedFile{attachment: image.Attachment, at: snapshot})
		}
	}
	answers := reference(domain.ReferenceAnswer, work.ID)
	for _, answer := range work.Answers {
		for _, file := range answer.Files {
			wanted = append(wanted, wantedFile{attachment: file.Attachment, at: answers})
		}
	}
	if work.Review != nil {
		corrected := reference(domain.ReferenceReviewItem, work.Review.Revision)
		for _, item := range work.Review.Items {
			for _, file := range item.CorrectedFiles {
				wanted = append(wanted, wantedFile{attachment: file.Attachment, at: corrected})
			}
		}
	}
	return wanted
}

func (b *studentBuild) admitMessages(ctx context.Context, repos repositories.Repositories, now time.Time, messages []chat.Message) error {
	for _, message := range messages {
		at := reference(domain.ReferenceMessage, message.ID)
		for _, attachment := range message.Attachments {
			if err := b.inventory.admit(ctx, repos, b.admitter(), b.claim.Export.UserID, attachment.ID, at, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *studentBuild) takeInventory(ctx context.Context) error {
	tutors, err := b.source.tutors(ctx)
	if err != nil {
		return err
	}
	b.inventory.items += boundedCount(len(tutors))
	countLessons := func(lessons []schedule.Lesson) error {
		b.inventory.items += boundedCount(len(lessons))
		return nil
	}
	if err := b.source.lessons(ctx, func(repos repositories.Repositories, now time.Time, lessons []schedule.Lesson) error {
		return b.admitLessons(ctx, repos, now, lessons)
	}, countLessons); err != nil {
		return err
	}
	countSubmissions := func(pairs []submissionPair) error {
		b.inventory.items += boundedCount(len(pairs))
		return nil
	}
	if err := b.source.submissions(ctx, func(repos repositories.Repositories, now time.Time, pairs []submissionPair) error {
		return b.admitSubmissions(ctx, repos, now, pairs)
	}, countSubmissions); err != nil {
		return err
	}
	channels, err := b.source.channels(ctx)
	if err != nil {
		return err
	}
	countMessages := func(messages []chat.Message) error {
		b.inventory.items += boundedCount(len(messages))
		return nil
	}
	for _, channel := range channels {
		if err := b.source.messages(ctx, channel.ID, func(repos repositories.Repositories, now time.Time, messages []chat.Message) error {
			return b.admitMessages(ctx, repos, now, messages)
		}, countMessages); err != nil {
			return err
		}
	}
	b.sink.total = b.inventory.items + boundedCount(len(b.inventory.files)) + 1
	return nil
}

func boundedCount(count int) int32 {
	if count < 0 {
		return 0
	}
	if count > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(count)
}

func (b *studentBuild) fileBytes() int64 {
	return b.inventory.fileBytes()
}

func (b *studentBuild) advance(count int) error {
	b.done += boundedCount(count)
	return b.sink.progressed(min(b.done, b.sink.total))
}

func (b *studentBuild) write(ctx context.Context, archive *exportformat.Archive) error {
	if err := b.storeFiles(ctx, archive); err != nil {
		return err
	}
	if err := b.writeTables(ctx, archive); err != nil {
		return err
	}
	if err := b.writeCalendar(ctx, archive); err != nil {
		return err
	}
	if err := b.writeData(ctx, archive); err != nil {
		return err
	}
	return b.sink.progressed(b.sink.total)
}

func (b *studentBuild) storeFiles(ctx context.Context, archive *exportformat.Archive) error {
	for _, file := range b.inventory.ordered() {
		if err := b.storeFile(ctx, archive, file); err != nil {
			return err
		}
		if err := b.advance(1); err != nil {
			return err
		}
	}
	return nil
}

func (b *studentBuild) storeFile(ctx context.Context, archive *exportformat.Archive, file *archivedFile) error {
	body, err := b.service.deps.Objects.Open(ctx, file.storageKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		b.unreadable++
		b.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.file.unreadable", exportAttribute(b.claim.Export.ID), buildAttribute(&b.claim.Build),
			slog.String("attachment_id", file.attachment.String()), slog.String("kind", string(file.kind)))
		return nil
	}
	if err != nil {
		return err
	}
	entry, err := archive.Create(file.entry)
	if err != nil {
		return errors.Join(err, body.Close())
	}
	digest := sha256.New()
	copied, copyErr := io.Copy(io.MultiWriter(entry, digest), body)
	closeErr := body.Close()
	if copyErr != nil {
		return errors.Join(fmt.Errorf("stream a file into the archive: %w", copyErr), closeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close a stored file: %w", closeErr)
	}
	if copied != file.size {
		return errFileSizeChanged
	}
	file.digest = hex.EncodeToString(digest.Sum(nil))
	file.stored = true
	b.files++
	return nil
}

func (b *studentBuild) writeCalendar(ctx context.Context, archive *exportformat.Archive) error {
	entry, err := archive.Create(exportformat.EntryCalendar)
	if err != nil {
		return err
	}
	calendar, err := exportformat.NewCalendar(entry, b.service.deps.PublicHost, b.claim.Started)
	if err != nil {
		return err
	}
	err = b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, lesson := range lessons {
			label := b.labelOf(lesson.Audience.Label, lesson.Audience.LabelKind)
			summary := lesson.Title
			if summary == "" {
				summary = label
			}
			description := label
			if lesson.Description != nil && *lesson.Description != "" {
				description += "\n" + *lesson.Description
			}
			if err := calendar.WriteEvent(exportformat.CalendarEvent{
				Lesson: lesson.ID, Start: lesson.StartsAt, End: lesson.EndsAt(), Summary: summary, Description: description,
				Cancelled: lesson.Status == domain.LessonCancelled,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return calendar.Close()
}

func (b *studentBuild) labelOf(label *string, kind *domain.LabelKind) string {
	switch {
	case label != nil:
		return *label
	case kind != nil:
		return b.text.Labels[*kind]
	}
	return ""
}

func (b *studentBuild) sheetCounts() []sheetCount {
	counts := make([]sheetCount, 0, len(b.counts))
	for _, sheet := range core.StudentExportSheets() {
		counts = append(counts, sheetCount{sheet: sheet, rows: b.counts[sheet]})
	}
	return counts
}

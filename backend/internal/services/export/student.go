package export

import (
	"context"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
)

type studentBuild struct {
	contentBuild
	source studentSource
}

func (s *Service) newStudentBuild(reader pageReader, claim Claim, prepared preparedContent) (*studentBuild, error) {
	if !s.deps.Content.completeForStudents() {
		return nil, errNoContentReaders
	}
	return &studentBuild{
		contentBuild: newContentBuild(s, reader, claim, prepared),
		source:       studentSource{content: s.deps.Content, reader: reader, student: claim.Export.UserID, pageSize: s.deps.Timing.ReadPage},
	}, nil
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

func (b *studentBuild) writeCalendar(ctx context.Context, archive *exportformat.Archive) error {
	return b.writeLessonCalendar(ctx, archive, func(visit func([]schedule.Lesson) error) error {
		return b.source.lessons(ctx, nil, visit)
	})
}

func (b *studentBuild) sheetCounts() []sheetCount {
	return b.countsOf(core.StudentExportSheets())
}

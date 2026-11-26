package export

import (
	"context"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

type tutorBuild struct {
	contentBuild
	source  tutorSource
	issues  []domain.IssueID
	folders []library.Folder
	groups  []tutoring.Group

	foldersByID map[string]schemas.LibraryFolder
}

func (s *Service) newTutorBuild(reader pageReader, claim Claim, prepared preparedContent) (*tutorBuild, error) {
	if !s.deps.Content.completeForTutors() {
		return nil, errNoContentReaders
	}
	return &tutorBuild{
		contentBuild: newContentBuild(s, reader, claim, prepared),
		source:       tutorSource{content: s.deps.Content, reader: reader, tutor: claim.Export.UserID, pageSize: s.deps.Timing.ReadPage},
	}, nil
}

func (b *tutorBuild) admit(ctx context.Context, repos repositories.Repositories, now time.Time, attachment domain.AttachmentID, at files.DownloadReference) error {
	return b.inventory.admit(ctx, repos, b.admitter(), b.claim.Export.UserID, attachment, at, now)
}

func (b *tutorBuild) admitLessons(ctx context.Context, repos repositories.Repositories, now time.Time, lessons []schedule.Lesson) error {
	for _, lesson := range lessons {
		at := reference(domain.ReferenceLesson, lesson.ID)
		for _, material := range lesson.Materials {
			if err := b.admit(ctx, repos, now, material.AttachmentID, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func issueBlockFiles(blocks []homework.IssueBlock, at files.DownloadReference) []wantedFile {
	var wanted []wantedFile
	for _, block := range blocks {
		if block.Material != nil {
			wanted = append(wanted, wantedFile{attachment: block.Material.File.Attachment, at: at})
		}
		for _, image := range block.Images {
			wanted = append(wanted, wantedFile{attachment: image.Attachment, at: at})
		}
	}
	return wanted
}

func tutorWorkFiles(work homework.TutorWork) []wantedFile {
	wanted := issueBlockFiles(work.Blocks, reference(domain.ReferenceIssueSnapshot, work.Issue))
	answers := reference(domain.ReferenceAnswer, work.ID)
	for _, answer := range work.Answers {
		for _, file := range answer.Files {
			wanted = append(wanted, wantedFile{attachment: file.Attachment, at: answers})
		}
	}
	if latest := latestPublished(work.Revisions); latest != nil {
		corrected := reference(domain.ReferenceReviewItem, latest.ID)
		for _, item := range latest.Items {
			for _, file := range item.CorrectedFiles {
				wanted = append(wanted, wantedFile{attachment: file.Attachment, at: corrected})
			}
		}
	}
	return wanted
}

func libraryFiles(entries []libraryEntry) []wantedFile {
	var wanted []wantedFile
	for _, entry := range entries {
		if material := entry.item.Material; material != nil {
			wanted = append(wanted, wantedFile{attachment: material.Attachment, at: reference(domain.ReferenceMaterial, material.ID)})
		}
		if template := entry.template; template != nil {
			at := reference(domain.ReferenceTemplate, template.ID)
			for _, block := range template.Blocks {
				if block.Material != nil {
					wanted = append(wanted, wantedFile{attachment: block.Material.File.Attachment, at: at})
				}
				for _, image := range block.Images {
					wanted = append(wanted, wantedFile{attachment: image.Attachment, at: at})
				}
			}
		}
	}
	return wanted
}

func (b *tutorBuild) admitAll(ctx context.Context, repos repositories.Repositories, now time.Time, wanted []wantedFile) error {
	for _, file := range wanted {
		if err := b.admit(ctx, repos, now, file.attachment, file.at); err != nil {
			return err
		}
	}
	return nil
}

func (b *tutorBuild) admitMessages(ctx context.Context, repos repositories.Repositories, now time.Time, messages []chat.Message) error {
	for _, message := range messages {
		at := reference(domain.ReferenceMessage, message.ID)
		for _, attachment := range message.Attachments {
			if err := b.admit(ctx, repos, now, attachment.ID, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *tutorBuild) count(count int) error {
	b.inventory.items += boundedCount(count)
	return nil
}

func (b *tutorBuild) takeInventory(ctx context.Context) error {
	if err := b.source.cards(ctx, nil, func(cards []tutoring.Card) error { return b.count(len(cards)) }); err != nil {
		return err
	}
	groups, err := b.source.groups(ctx)
	if err != nil {
		return err
	}
	b.groups = groups
	b.inventory.items += boundedCount(len(groups))
	if err := b.source.lessons(ctx, func(repos repositories.Repositories, now time.Time, lessons []schedule.Lesson) error {
		return b.admitLessons(ctx, repos, now, lessons)
	}, func(lessons []schedule.Lesson) error { return b.count(len(lessons)) }); err != nil {
		return err
	}
	if b.issues, err = b.source.issueIDs(ctx); err != nil {
		return err
	}
	if err := b.source.issues(ctx, b.issues, func(repos repositories.Repositories, now time.Time, issue homework.Issue) error {
		return b.admitAll(ctx, repos, now, issueBlockFiles(issue.Blocks, reference(domain.ReferenceIssueSnapshot, issue.ID)))
	}, func(homework.Issue) error { return b.count(1) }); err != nil {
		return err
	}
	if err := b.source.submissions(ctx, b.issues, func(repos repositories.Repositories, now time.Time, works []homework.TutorWork) error {
		for _, work := range works {
			if err := b.admitAll(ctx, repos, now, tutorWorkFiles(work)); err != nil {
				return err
			}
		}
		return nil
	}, func(works []homework.TutorWork) error { return b.count(len(works)) }); err != nil {
		return err
	}
	if b.folders, err = b.source.folders(ctx); err != nil {
		return err
	}
	if err := b.source.library(ctx, b.folders, func(repos repositories.Repositories, now time.Time, entries []libraryEntry) error {
		return b.admitAll(ctx, repos, now, libraryFiles(entries))
	}, func(entries []libraryEntry) error { return b.count(len(entries)) }); err != nil {
		return err
	}
	chats := b.source.chat()
	channels, err := chats.channels(ctx)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if err := chats.messages(ctx, channel.ID, func(repos repositories.Repositories, now time.Time, messages []chat.Message) error {
			return b.admitMessages(ctx, repos, now, messages)
		}, func(messages []chat.Message) error { return b.count(len(messages)) }); err != nil {
			return err
		}
	}
	b.sink.total = b.inventory.items + boundedCount(len(b.inventory.files)) + 1
	return nil
}

func (b *tutorBuild) write(ctx context.Context, archive *exportformat.Archive) error {
	if err := b.storeFiles(ctx, archive); err != nil {
		return err
	}
	if err := b.writeTables(ctx, archive); err != nil {
		return err
	}
	if err := b.writeLessonCalendar(ctx, archive, func(visit func([]schedule.Lesson) error) error {
		return b.source.lessons(ctx, nil, visit)
	}); err != nil {
		return err
	}
	if err := b.writeData(ctx, archive); err != nil {
		return err
	}
	return b.sink.progressed(b.sink.total)
}

func (b *tutorBuild) sheetCounts() []sheetCount {
	return b.countsOf(core.TutorExportSheets())
}

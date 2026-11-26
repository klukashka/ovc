package export

import (
	"bufio"
	"context"
	"fmt"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

func (b *tutorBuild) writeData(ctx context.Context, archive *exportformat.Archive) error {
	entry, err := archive.Create(exportformat.EntryData)
	if err != nil {
		return err
	}
	stream := &jsonStream{output: bufio.NewWriter(entry)}
	if err := b.writeDataParts(ctx, stream); err != nil {
		return err
	}
	if err := stream.output.Flush(); err != nil {
		return fmt.Errorf("finish the data copy: %w", err)
	}
	return nil
}

func (b *tutorBuild) writeDataParts(ctx context.Context, stream *jsonStream) error {
	if err := stream.raw("{"); err != nil {
		return err
	}
	header := []func() error{
		func() error { return writeField(stream, "format_version", domain.ExportFormatVersion) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "exported_at", b.claim.Started.UTC()) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "role", string(domain.RoleTutor)) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "account", b.account) },
	}
	for _, write := range header {
		if err := write(); err != nil {
			return err
		}
	}
	for _, part := range []func(context.Context, *jsonStream) error{
		b.dataStudents, b.dataGroups, b.dataLessons, b.dataSeries, b.dataIssues, b.dataSubmissions, b.dataFolders, b.dataLibraryItems,
		b.dataTemplates, b.dataMessages, b.dataFiles,
	} {
		if err := part(ctx, stream); err != nil {
			return err
		}
	}
	return stream.raw("}\n")
}

func (b *tutorBuild) dataStudents(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("students"); err != nil {
		return err
	}
	err := b.source.cards(ctx, nil, func(cards []tutoring.Card) error {
		for _, card := range cards {
			if err := writeItem(stream, cardBody(card)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataGroups(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("groups"); err != nil {
		return err
	}
	groups := b.groups
	for _, group := range groups {
		if err := writeItem(stream, groupBody(group)); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataLessons(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("lessons"); err != nil {
		return err
	}
	err := b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, lesson := range lessons {
			if err := writeItem(stream, tutorLessonBody(lesson, b.inventory.path)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataSeries(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("series"); err != nil {
		return err
	}
	series, err := b.series(ctx)
	if err != nil {
		return err
	}
	for _, found := range series {
		if err := writeItem(stream, found); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataIssues(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("issues"); err != nil {
		return err
	}
	err := b.source.issues(ctx, b.issues, nil, func(issue homework.Issue) error {
		return writeItem(stream, issueBody(issue, b.inventory.path))
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataSubmissions(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("submissions"); err != nil {
		return err
	}
	err := b.source.submissions(ctx, b.issues, nil, func(works []homework.TutorWork) error {
		for _, work := range works {
			if err := writeItem(stream, tutorSubmissionBody(work, b.inventory.path)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataFolders(_ context.Context, stream *jsonStream) error {
	if err := stream.startArray("library_folders"); err != nil {
		return err
	}
	for _, folder := range b.folders {
		if err := writeItem(stream, folderBody(folder)); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataLibraryItems(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("library_items"); err != nil {
		return err
	}
	err := b.source.library(ctx, b.folders, nil, func(entries []libraryEntry) error {
		for _, entry := range entries {
			if err := writeItem(stream, libraryItemBody(entry.item, b.inventory.path)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataTemplates(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("templates"); err != nil {
		return err
	}
	err := b.source.library(ctx, b.folders, nil, func(entries []libraryEntry) error {
		for _, entry := range entries {
			if entry.template == nil {
				continue
			}
			if err := writeItem(stream, templateBody(*entry.template, b.inventory.path)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataMessages(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("messages"); err != nil {
		return err
	}
	chats := b.source.chat()
	channels, err := chats.channels(ctx)
	if err != nil {
		return err
	}
	for index, channel := range channels {
		if index > 0 {
			if err := stream.raw(","); err != nil {
				return err
			}
		}
		if err := stream.raw("{"); err != nil {
			return err
		}
		if err := writeField(stream, "channel", channelBody(channel)); err != nil {
			return err
		}
		if err := stream.raw(`,"messages":[`); err != nil {
			return err
		}
		stream.items = 0
		err := chats.messages(ctx, channel.ID, nil, func(messages []chat.Message) error {
			for _, message := range messages {
				if err := writeItem(stream, tutorMessageBody(message, b.inventory.path)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if err := stream.raw("]}"); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *tutorBuild) dataFiles(_ context.Context, stream *jsonStream) error {
	if err := stream.startArray("files"); err != nil {
		return err
	}
	for _, file := range b.storedFiles() {
		if err := writeItem(stream, schemas.TutorExportFileEntry{
			Path: file.entry, Filename: file.filename, MediaType: file.mediaType, SizeBytes: file.size, SHA256: file.digest,
			References: append([]string{}, file.references...),
		}); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

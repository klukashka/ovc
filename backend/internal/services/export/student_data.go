package export

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/schedule"
)

type jsonStream struct {
	output *bufio.Writer
	items  int
}

func (s *jsonStream) raw(text string) error {
	if _, err := s.output.WriteString(text); err != nil {
		return fmt.Errorf("write the data copy: %w", err)
	}
	return nil
}

func encodedValue[Value any](stream *jsonStream, value Value) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode the data copy: %w", err)
	}
	if _, err := stream.output.Write(encoded); err != nil {
		return fmt.Errorf("write the data copy: %w", err)
	}
	return nil
}

func writeField[Value any](stream *jsonStream, key string, value Value) error {
	if err := encodedValue(stream, key); err != nil {
		return err
	}
	if err := stream.raw(":"); err != nil {
		return err
	}
	return encodedValue(stream, value)
}

func (s *jsonStream) startArray(key string) error {
	s.items = 0
	if err := s.raw(","); err != nil {
		return err
	}
	if err := encodedValue(s, key); err != nil {
		return err
	}
	return s.raw(":[")
}

func writeItem[Value any](stream *jsonStream, value Value) error {
	if stream.items > 0 {
		if err := stream.raw(","); err != nil {
			return err
		}
	}
	stream.items++
	return encodedValue(stream, value)
}

func (b *studentBuild) writeData(ctx context.Context, archive *exportformat.Archive) error {
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

func (b *studentBuild) writeDataParts(ctx context.Context, stream *jsonStream) error {
	if err := stream.raw("{"); err != nil {
		return err
	}
	header := []func() error{
		func() error { return writeField(stream, "format_version", domain.ExportFormatVersion) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "exported_at", b.claim.Started.UTC()) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "role", string(domain.RoleStudent)) },
		func() error { return stream.raw(",") },
		func() error { return writeField(stream, "account", b.account) },
	}
	for _, write := range header {
		if err := write(); err != nil {
			return err
		}
	}
	for _, part := range []func(context.Context, *jsonStream) error{
		b.dataTutors, b.dataLessons, b.dataHomework, b.dataSubmissions, b.dataMessages, b.dataFiles,
	} {
		if err := part(ctx, stream); err != nil {
			return err
		}
	}
	return stream.raw("}\n")
}

func (b *studentBuild) dataTutors(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("tutors"); err != nil {
		return err
	}
	tutors, err := b.source.tutors(ctx)
	if err != nil {
		return err
	}
	for _, tutor := range tutors {
		if err := writeItem(stream, tutorBody(tutor)); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *studentBuild) dataLessons(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("lessons"); err != nil {
		return err
	}
	err := b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, lesson := range lessons {
			if err := writeItem(stream, lessonBody(lesson, b.inventory.path)); err != nil {
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

func (b *studentBuild) dataHomework(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("homework"); err != nil {
		return err
	}
	listed, err := b.source.assignments(ctx)
	if err != nil {
		return err
	}
	for _, item := range listed {
		if err := writeItem(stream, assignmentBody(item)); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

func (b *studentBuild) dataSubmissions(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("submissions"); err != nil {
		return err
	}
	err := b.source.submissions(ctx, nil, func(pairs []submissionPair) error {
		for _, pair := range pairs {
			if err := writeItem(stream, submissionBody(pair.work, b.inventory.path)); err != nil {
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

func (b *studentBuild) dataMessages(ctx context.Context, stream *jsonStream) error {
	if err := stream.startArray("messages"); err != nil {
		return err
	}
	channels, err := b.source.channels(ctx)
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
		err := b.source.messages(ctx, channel.ID, nil, func(messages []chat.Message) error {
			for _, message := range messages {
				if err := writeItem(stream, messageBody(message, b.inventory.path)); err != nil {
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

func (b *studentBuild) dataFiles(_ context.Context, stream *jsonStream) error {
	if err := stream.startArray("files"); err != nil {
		return err
	}
	for _, entry := range b.fileEntries() {
		if err := writeItem(stream, entry); err != nil {
			return err
		}
	}
	return stream.raw("]")
}

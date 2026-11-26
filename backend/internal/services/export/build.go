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
	"ovc/backend/internal/services/schedule"
)

var errFileSizeChanged = errors.New("a stored file's length differs from its recorded size")

type contentBuild struct {
	service    *Service
	claim      Claim
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

func newContentBuild(s *Service, reader pageReader, claim Claim, prepared preparedContent) contentBuild {
	return contentBuild{
		service: s, claim: claim, sink: reader.sink, text: prepared.text, location: prepared.location, account: prepared.account,
		inventory: newInventory(), counts: map[core.ExportSheet]int{},
	}
}

func (b *contentBuild) admitter() FileAuthorizer {
	return b.service.deps.Content.Files
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

func (b *contentBuild) fileBytes() int64 {
	return b.inventory.fileBytes()
}

func (b *contentBuild) advance(count int) error {
	b.done += boundedCount(count)
	return b.sink.progressed(min(b.done, b.sink.total))
}

func (b *contentBuild) storeFiles(ctx context.Context, archive *exportformat.Archive) error {
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

func (b *contentBuild) storeFile(ctx context.Context, archive *exportformat.Archive, file *archivedFile) error {
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

func (b *contentBuild) labelOf(label *string, kind *domain.LabelKind) string {
	switch {
	case label != nil:
		return *label
	case kind != nil:
		return b.text.Labels[*kind]
	}
	return ""
}

func (b *contentBuild) writeLessonCalendar(ctx context.Context, archive *exportformat.Archive, lessons func(func([]schedule.Lesson) error) error) error {
	entry, err := archive.Create(exportformat.EntryCalendar)
	if err != nil {
		return err
	}
	calendar, err := exportformat.NewCalendar(entry, b.service.deps.PublicHost, b.claim.Started)
	if err != nil {
		return err
	}
	err = lessons(func(found []schedule.Lesson) error {
		for _, lesson := range found {
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

func (b *contentBuild) countsOf(sheets []core.ExportSheet) []sheetCount {
	counts := make([]sheetCount, 0, len(sheets))
	for _, sheet := range sheets {
		counts = append(counts, sheetCount{sheet: sheet, rows: b.counts[sheet]})
	}
	return counts
}

func (b *contentBuild) headersOf(text core.ExportSheetText, columns []core.ExportColumn) []string {
	headers := append([]string{}, text.Headers...)
	for index, column := range columns {
		if column.Date && index < len(headers) {
			headers[index] = fmt.Sprintf(headers[index], b.location.String())
		}
	}
	return headers
}

func (b *contentBuild) storedFiles() []*archivedFile {
	var stored []*archivedFile
	for _, file := range b.inventory.ordered() {
		if file.stored {
			stored = append(stored, file)
		}
	}
	return stored
}

func (b *contentBuild) date(instant *time.Time) exportformat.Cell {
	if instant == nil {
		return exportformat.Empty()
	}
	return exportformat.Date(instant.In(b.location))
}

func (b *contentBuild) flag(value bool) exportformat.Cell {
	if value {
		return exportformat.Text(b.text.Yes)
	}
	return exportformat.Text(b.text.No)
}

func optional(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func (b *contentBuild) fileCell(name string, path *string) string {
	if path == nil {
		return name + " (" + b.text.NotAvailable + ")"
	}
	return name + " (" + *path + ")"
}

func (b *contentBuild) row(sheet *exportformat.Sheet, name core.ExportSheet, cells ...exportformat.Cell) error {
	b.counts[name]++
	return sheet.WriteRow(cells...)
}

func (b *contentBuild) storedCount() int {
	return b.files
}

func (b *contentBuild) unreadableCount() int {
	return b.unreadable
}

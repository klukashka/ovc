package export

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/schedule"
)

type sheetCount struct {
	sheet core.ExportSheet
	rows  int
}

func (b *studentBuild) headers(sheet core.ExportSheet) []string {
	columns := core.StudentExportColumns(sheet)
	headers := append([]string{}, b.text.Sheets[sheet].Headers...)
	for index, column := range columns {
		if column.Date && index < len(headers) {
			headers[index] = fmt.Sprintf(headers[index], b.location.String())
		}
	}
	return headers
}

func (b *studentBuild) date(instant *time.Time) exportformat.Cell {
	if instant == nil {
		return exportformat.Empty()
	}
	return exportformat.Date(instant.In(b.location))
}

func (b *studentBuild) flag(value bool) exportformat.Cell {
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

func (b *studentBuild) fileCell(name string, path *string) string {
	if path == nil {
		return name + " (" + b.text.NotAvailable + ")"
	}
	return name + " (" + *path + ")"
}

func (b *studentBuild) writeTables(ctx context.Context, archive *exportformat.Archive) error {
	entry, err := archive.Create(exportformat.EntryTables)
	if err != nil {
		return err
	}
	workbook := exportformat.NewWorkbook(entry, b.claim.Started, exportformat.RowsPerSheet)
	if err := writeAccountSheet(workbook, b.text, b.location, b.account, domain.RoleStudent); err != nil {
		return err
	}
	writers := map[core.ExportSheet]func(context.Context, *exportformat.Sheet) error{
		core.ExportSheetTutors: b.writeTutors, core.ExportSheetLessons: b.writeLessons, core.ExportSheetHomework: b.writeHomework,
		core.ExportSheetAnswers: b.writeAnswers, core.ExportSheetFeedback: b.writeFeedback, core.ExportSheetMessages: b.writeMessages,
		core.ExportSheetFiles: b.writeFilesSheet,
	}
	for _, name := range core.StudentExportSheets() {
		sheet, err := workbook.StartSheet(b.text.Sheets[name].Title, b.headers(name))
		if err != nil {
			return err
		}
		if err := writers[name](ctx, sheet); err != nil {
			return err
		}
	}
	return workbook.Close()
}

func (b *studentBuild) row(sheet *exportformat.Sheet, name core.ExportSheet, cells ...exportformat.Cell) error {
	b.counts[name]++
	return sheet.WriteRow(cells...)
}

func (b *studentBuild) writeTutors(ctx context.Context, sheet *exportformat.Sheet) error {
	tutors, err := b.source.tutors(ctx)
	if err != nil {
		return err
	}
	for _, found := range tutors {
		tutor := tutorBody(found)
		var endedBy string
		if tutor.EndedBy != nil {
			endedBy = string(*tutor.EndedBy)
		}
		if err := b.row(sheet, core.ExportSheetTutors, exportformat.Text(tutor.Tutor.Name), exportformat.Text(string(tutor.State)),
			exportformat.Text(strings.Join(tutor.Tutor.Subjects, ", ")), exportformat.Text(tutor.Tutor.Bio), b.date(tutor.EndedAt),
			exportformat.Text(endedBy), b.date(tutor.AccountClosedUntil)); err != nil {
			return err
		}
	}
	return b.advance(len(tutors))
}

func (b *studentBuild) writeLessons(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, found := range lessons {
			lesson := lessonBody(found, b.inventory.path)
			materials := make([]string, 0, len(lesson.Materials))
			for _, material := range lesson.Materials {
				materials = append(materials, b.fileCell(material.Filename, material.Path))
			}
			var tutor string
			if lesson.Tutor != nil {
				tutor = lesson.Tutor.Name
			}
			if err := b.row(sheet, core.ExportSheetLessons, exportformat.Text(lesson.Title), b.date(&lesson.StartsAt), b.date(&lesson.EndsAt),
				exportformat.Text(string(lesson.Status)), b.flag(lesson.TookPlace), exportformat.Text(lesson.Timezone),
				exportformat.Text(b.labelOf(lesson.Audience.Label, lesson.Audience.LabelKind)), exportformat.Text(tutor),
				exportformat.Text(optional(lesson.Description)), b.date(lesson.CancelledAt), exportformat.Text(strings.Join(materials, "\n"))); err != nil {
				return err
			}
		}
		return b.advance(len(lessons))
	})
}

func (b *studentBuild) writeHomework(ctx context.Context, sheet *exportformat.Sheet) error {
	listed, err := b.source.assignments(ctx)
	if err != nil {
		return err
	}
	for _, found := range listed {
		item := assignmentBody(found)
		if err := b.row(sheet, core.ExportSheetHomework, exportformat.Text(item.Title), exportformat.Text(item.Tutor.Name),
			exportformat.Text(string(item.Status)), b.flag(item.Late), b.date(item.DeadlineAt), b.date(&item.IssuedAt), b.date(item.SubmittedAt),
			b.date(item.CheckedAt), exportformat.Text(optional(item.FeedbackExcerpt)), b.flag(item.IssuedBeforeReclaim)); err != nil {
			return err
		}
	}
	return b.advance(len(listed))
}

func blockOf(blocks []schemas.StudentExportTemplateBlock, question string) (schemas.StudentExportTemplateBlock, bool) {
	index := slices.IndexFunc(blocks, func(block schemas.StudentExportTemplateBlock) bool { return block.ID == question })
	if index < 0 {
		return schemas.StudentExportTemplateBlock{}, false
	}
	return blocks[index], true
}

func optionTexts(block schemas.StudentExportTemplateBlock, chosen *[]string) string {
	if chosen == nil || block.Options == nil {
		return ""
	}
	var texts []string
	for _, option := range *block.Options {
		if slices.Contains(*chosen, option.ID) {
			texts = append(texts, option.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func revealedOf(submission schemas.StudentExportSubmission, question string) (schemas.StudentRevealedQuestion, bool) {
	if submission.Revealed == nil {
		return schemas.StudentRevealedQuestion{}, false
	}
	for _, revealed := range submission.Revealed.Questions {
		if revealed.QuestionID == question {
			return revealed, true
		}
	}
	return schemas.StudentRevealedQuestion{}, false
}

func correctAnswer(block schemas.StudentExportTemplateBlock, revealed schemas.StudentRevealedQuestion) string {
	var parts []string
	if chosen := optionTexts(block, revealed.CorrectOptionIDs); chosen != "" {
		parts = append(parts, chosen)
	}
	if revealed.AcceptedAnswers != nil {
		parts = append(parts, strings.Join(*revealed.AcceptedAnswers, "\n"))
	}
	if revealed.CorrectValue != nil {
		value := *revealed.CorrectValue
		if revealed.Tolerance != nil {
			value += " ± " + *revealed.Tolerance
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\n")
}

func (b *studentBuild) writeAnswers(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.submissions(ctx, nil, func(pairs []submissionPair) error {
		for _, pair := range pairs {
			submission := submissionBody(pair.work, b.inventory.path)
			for _, answer := range submission.Answers {
				block, _ := blockOf(submission.Blocks, answer.QuestionID)
				attached := make([]string, 0, len(answer.Files))
				for _, file := range answer.Files {
					attached = append(attached, b.fileCell(file.Filename, file.Path))
				}
				var correct, explanation string
				if revealed, found := revealedOf(submission, answer.QuestionID); found {
					correct, explanation = correctAnswer(block, revealed), optional(revealed.Explanation)
				}
				if err := b.row(sheet, core.ExportSheetAnswers, exportformat.Text(submission.Title), exportformat.Text(optional(block.Prompt)),
					exportformat.Text(optional(answer.Value.Text)), exportformat.Text(optionTexts(block, answer.Value.OptionIDs)),
					exportformat.Text(strings.Join(attached, "\n")), exportformat.Text(correct), exportformat.Text(explanation)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (b *studentBuild) writeFeedback(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.submissions(ctx, nil, func(pairs []submissionPair) error {
		for _, pair := range pairs {
			submission := submissionBody(pair.work, b.inventory.path)
			if submission.Review == nil {
				continue
			}
			if err := b.row(sheet, core.ExportSheetFeedback, exportformat.Text(submission.Title), exportformat.Text(submission.Review.Feedback),
				exportformat.Empty(), exportformat.Empty(), exportformat.Empty()); err != nil {
				return err
			}
			for _, item := range submission.Review.Items {
				block, _ := blockOf(submission.Blocks, item.QuestionID)
				corrected := make([]string, 0, len(item.CorrectedFiles))
				for _, file := range item.CorrectedFiles {
					corrected = append(corrected, b.fileCell(file.Filename, file.Path))
				}
				if err := b.row(sheet, core.ExportSheetFeedback, exportformat.Text(submission.Title), exportformat.Empty(),
					exportformat.Text(optional(block.Prompt)), exportformat.Text(item.Comment), exportformat.Text(strings.Join(corrected, "\n"))); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (b *studentBuild) channelLabel(channel schemas.ChannelItem) string {
	if label := b.labelOf(channel.Label, channel.LabelKind); label != "" {
		return label
	}
	if channel.Tutor != nil {
		return channel.Tutor.Name
	}
	return ""
}

func (b *studentBuild) senderOf(own bool, label *string, kind *domain.LabelKind) string {
	if own {
		return b.text.You
	}
	return b.labelOf(label, kind)
}

func (b *studentBuild) bodyOf(message schemas.StudentExportMessage) string {
	if message.Deleted || message.Erased {
		return b.text.MessageRemoved
	}
	return optional(message.Body)
}

func (b *studentBuild) quoteOf(quote *schemas.MessageQuote) string {
	if quote == nil {
		return ""
	}
	if quote.State != domain.QuoteAvailable {
		return b.text.MessageRemoved
	}
	return optional(quote.Preview)
}

func reactionsOf(message schemas.StudentExportMessage) string {
	parts := make([]string, 0, len(message.Reactions))
	for _, reaction := range message.Reactions {
		parts = append(parts, string(reaction.Reaction)+" "+strconv.FormatInt(reaction.Count, 10))
	}
	return strings.Join(parts, ", ")
}

func (b *studentBuild) writeMessages(ctx context.Context, sheet *exportformat.Sheet) error {
	channels, err := b.source.channels(ctx)
	if err != nil {
		return err
	}
	for _, found := range channels {
		channel := channelBody(found)
		label := b.channelLabel(channel)
		err := b.source.messages(ctx, found.ID, nil, func(messages []chat.Message) error {
			for _, viewed := range messages {
				message := messageBody(viewed, b.inventory.path)
				attached := make([]string, 0, len(message.Attachments))
				for _, attachment := range message.Attachments {
					attached = append(attached, b.fileCell(attachment.Filename, attachment.Path))
				}
				if err := b.row(sheet, core.ExportSheetMessages, exportformat.Text(label), b.date(&message.CreatedAt),
					exportformat.Text(b.senderOf(message.Own, message.SenderLabel, message.SenderLabelKind)), exportformat.Text(b.bodyOf(message)),
					b.date(message.EditedAt), exportformat.Text(b.quoteOf(message.ReplyTo)), exportformat.Text(strings.Join(attached, "\n")),
					exportformat.Text(reactionsOf(message))); err != nil {
					return err
				}
			}
			return b.advance(len(messages))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *studentBuild) fileEntries() []schemas.StudentExportFileEntry {
	var entries []schemas.StudentExportFileEntry
	for _, file := range b.inventory.ordered() {
		if !file.stored {
			continue
		}
		entries = append(entries, schemas.StudentExportFileEntry{
			Path: file.entry, Filename: file.filename, MediaType: file.mediaType, SizeBytes: file.size, SHA256: file.digest,
			References: append([]string{}, file.references...),
		})
	}
	return entries
}

func (b *studentBuild) writeFilesSheet(_ context.Context, sheet *exportformat.Sheet) error {
	for _, entry := range b.fileEntries() {
		if err := b.row(sheet, core.ExportSheetFiles, exportformat.Text(entry.Path), exportformat.Text(entry.Filename), exportformat.Text(entry.MediaType),
			exportformat.Integer(entry.SizeBytes), exportformat.Text(entry.SHA256), exportformat.Text(strings.Join(entry.References, "\n"))); err != nil {
			return err
		}
	}
	return nil
}

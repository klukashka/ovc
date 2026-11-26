package export

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

func (b *tutorBuild) headers(sheet core.ExportSheet) []string {
	return b.headersOf(b.text.TutorSheets[sheet], core.TutorExportColumns(sheet))
}

func (b *tutorBuild) writeTables(ctx context.Context, archive *exportformat.Archive) error {
	entry, err := archive.Create(exportformat.EntryTables)
	if err != nil {
		return err
	}
	workbook := exportformat.NewWorkbook(entry, b.claim.Started, exportformat.RowsPerSheet)
	if err := writeAccountSheet(workbook, b.text, b.location, b.account, domain.RoleTutor); err != nil {
		return err
	}
	writers := map[core.ExportSheet]func(context.Context, *exportformat.Sheet) error{
		core.ExportSheetStudents: b.writeStudents, core.ExportSheetGroups: b.writeGroups, core.ExportSheetGroupMembers: b.writeGroupMembers,
		core.ExportSheetLessons: b.writeLessons, core.ExportSheetSeries: b.writeSeries, core.ExportSheetIssues: b.writeIssues,
		core.ExportSheetSubmissions: b.writeSubmissions, core.ExportSheetAnswers: b.writeAnswers, core.ExportSheetReviews: b.writeReviews,
		core.ExportSheetLibrary: b.writeLibrary, core.ExportSheetMessages: b.writeMessages, core.ExportSheetFiles: b.writeFilesSheet,
	}
	for _, name := range core.TutorExportSheets() {
		sheet, err := workbook.StartSheet(b.text.TutorSheets[name].Title, b.headers(name))
		if err != nil {
			return err
		}
		if err := writers[name](ctx, sheet); err != nil {
			return err
		}
	}
	return workbook.Close()
}

func optionalCode[Code ~string](code *Code) string {
	if code == nil {
		return ""
	}
	return string(*code)
}

func optionalInteger[Number ~int32 | ~int64](number *Number) exportformat.Cell {
	if number == nil {
		return exportformat.Empty()
	}
	return exportformat.Integer(int64(*number))
}

func (b *tutorBuild) writeStudents(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.cards(ctx, nil, func(cards []tutoring.Card) error {
		for _, found := range cards {
			card := cardBody(found)
			var accountName, accountEmail, invitation string
			if card.Account != nil {
				accountName, accountEmail = card.Account.Name, card.Account.Email
			}
			invitationExpires := exportformat.Empty()
			if card.Invitation != nil {
				invitation, invitationExpires = string(card.Invitation.Status), b.date(&card.Invitation.ExpiresAt)
			}
			groups := make([]string, 0, len(card.Groups))
			for _, group := range card.Groups {
				groups = append(groups, group.Name)
			}
			if err := b.row(sheet, core.ExportSheetStudents, exportformat.Text(card.DisplayName), exportformat.Text(string(card.State)),
				exportformat.Text(optional(card.Email)), b.flag(card.EmailErased), exportformat.Text(accountName), exportformat.Text(accountEmail),
				exportformat.Text(strings.Join(groups, ", ")), exportformat.Text(card.Notes), exportformat.Text(invitation), invitationExpires,
				b.date(card.LinkedAt), b.date(card.EndedAt), exportformat.Text(optionalCode(card.EndedBy)), b.date(&card.CreatedAt)); err != nil {
				return err
			}
		}
		return b.advance(len(cards))
	})
}

func (b *tutorBuild) writeGroups(ctx context.Context, sheet *exportformat.Sheet) error {
	groups := b.groups
	for _, found := range groups {
		group := groupBody(found)
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.DisplayName)
		}
		if err := b.row(sheet, core.ExportSheetGroups, exportformat.Text(group.Name), exportformat.Text(string(group.Colour)), b.date(group.ArchivedAt),
			exportformat.Text(strings.Join(members, "\n"))); err != nil {
			return err
		}
	}
	return b.advance(len(groups))
}

func (b *tutorBuild) writeGroupMembers(ctx context.Context, sheet *exportformat.Sheet) error {
	groups := b.groups
	for _, found := range groups {
		group := groupBody(found)
		for _, member := range group.Members {
			if err := b.row(sheet, core.ExportSheetGroupMembers, exportformat.Text(group.Name), exportformat.Text(member.DisplayName),
				exportformat.Text(optional(member.Name)), b.date(&member.AddedAt)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *tutorBuild) memberLabels(audience schemas.LessonAudience) string {
	labels := make([]string, 0, len(audience.Members))
	for _, member := range audience.Members {
		label := b.labelOf(member.Label, member.LabelKind)
		if member.Label != nil && member.LabelKind != nil {
			label += " (" + b.text.Labels[*member.LabelKind] + ")"
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, "\n")
}

func (b *tutorBuild) writeLessons(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, found := range lessons {
			lesson := tutorLessonBody(found, b.inventory.path)
			materials := make([]string, 0, len(lesson.Materials))
			for _, material := range lesson.Materials {
				materials = append(materials, b.fileCell(material.Filename, material.Path))
			}
			if err := b.row(sheet, core.ExportSheetLessons, exportformat.Text(lesson.Title), b.date(&lesson.StartsAt), b.date(&lesson.EndsAt),
				exportformat.Text(string(lesson.Status)), b.flag(lesson.TookPlace), exportformat.Text(lesson.Timezone),
				exportformat.Text(b.labelOf(lesson.Audience.Label, lesson.Audience.LabelKind)), exportformat.Text(b.memberLabels(lesson.Audience)),
				exportformat.Text(optional(lesson.Description)), b.date(lesson.CancelledAt), exportformat.Text(optional(lesson.SeriesID)),
				exportformat.Text(strings.Join(materials, "\n"))); err != nil {
				return err
			}
		}
		return b.advance(len(lessons))
	})
}

func (b *tutorBuild) series(ctx context.Context) ([]schemas.LessonSeries, error) {
	var series []schemas.LessonSeries
	seen := map[domain.SeriesID]bool{}
	err := b.source.lessons(ctx, nil, func(lessons []schedule.Lesson) error {
		for _, lesson := range lessons {
			if lesson.Series == nil || seen[lesson.Series.ID] {
				continue
			}
			seen[lesson.Series.ID] = true
			series = append(series, *seriesBody(*lesson.Series))
		}
		return nil
	})
	return series, err
}

func (b *tutorBuild) writeSeries(ctx context.Context, sheet *exportformat.Sheet) error {
	series, err := b.series(ctx)
	if err != nil {
		return err
	}
	for _, found := range series {
		weekdays := make([]string, 0, len(found.Weekdays))
		for _, day := range found.Weekdays {
			weekdays = append(weekdays, strconv.Itoa(day))
		}
		if err := b.row(sheet, core.ExportSheetSeries, exportformat.Text(found.ID), exportformat.Text(strings.Join(weekdays, ", ")),
			exportformat.Text(found.Start), exportformat.Integer(int64(found.DurationMinutes)), exportformat.Text(found.Timezone),
			exportformat.Text(found.FirstDate), exportformat.Text(optional(found.EndDate)), optionalCount(found.Count)); err != nil {
			return err
		}
	}
	return nil
}

func (b *tutorBuild) audienceOf(audience schemas.IssueAudience) string {
	switch {
	case audience.Name != nil:
		return *audience.Name
	case audience.DisplayName != nil:
		return *audience.DisplayName
	}
	return b.labelOf(nil, audience.LabelKind)
}

func (b *tutorBuild) writeIssues(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.issues(ctx, b.issues, nil, func(found homework.Issue) error {
		issue := issueBody(found, b.inventory.path)
		materials := issueMaterialCells(b, issue.Blocks)
		if err := b.row(sheet, core.ExportSheetIssues, exportformat.Text(issue.Title), exportformat.Text(b.audienceOf(issue.Audience)),
			exportformat.Text(string(issue.Status)), exportformat.Text(string(issue.AnswerPolicy)), b.date(issue.DeadlineAt), b.date(&issue.IssuedAt),
			b.date(issue.WithdrawnAt), exportformat.Integer(int64(issue.MaxPoints)), exportformat.Text(strings.Join(materials, "\n"))); err != nil {
			return err
		}
		return b.advance(1)
	})
}

func issueMaterialCells(b *tutorBuild, blocks []schemas.TutorExportIssueBlock) []string {
	var cells []string
	for _, block := range blocks {
		if block.Material != nil {
			cells = append(cells, b.fileCell(block.Material.Filename, block.Material.Path))
		}
		if block.Images != nil {
			for _, image := range *block.Images {
				cells = append(cells, b.fileCell(image.Filename, image.Path))
			}
		}
	}
	return cells
}

func (b *tutorBuild) writeSubmissions(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.submissions(ctx, b.issues, nil, func(works []homework.TutorWork) error {
		for _, found := range works {
			work := tutorSubmissionBody(found, b.inventory.path)
			if err := b.row(sheet, core.ExportSheetSubmissions, exportformat.Text(work.Title), exportformat.Text(work.Student.DisplayName),
				exportformat.Text(string(work.Status)), b.flag(work.Late), b.flag(work.IssuedBeforeReclaim), b.date(work.SubmittedAt),
				b.date(work.CheckedAt), optionalInteger(work.AutoPoints), optionalInteger(work.FinalPoints), exportformat.Integer(int64(work.MaxPoints)),
				optionalInteger(work.Percentage)); err != nil {
				return err
			}
		}
		return b.advance(len(works))
	})
}

func tutorBlockOf(blocks []schemas.TutorExportIssueBlock, question string) (schemas.TutorExportIssueBlock, bool) {
	index := slices.IndexFunc(blocks, func(block schemas.TutorExportIssueBlock) bool { return block.ID == question })
	if index < 0 {
		return schemas.TutorExportIssueBlock{}, false
	}
	return blocks[index], true
}

func tutorOptionTexts(block schemas.TutorExportIssueBlock, chosen *[]string) string {
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

func acceptedAnswers(block schemas.TutorExportIssueBlock) string {
	var parts []string
	if block.Options != nil {
		for _, option := range *block.Options {
			if option.Correct != nil && *option.Correct {
				parts = append(parts, option.Text)
			}
		}
	}
	if block.AcceptedAnswers != nil {
		parts = append(parts, *block.AcceptedAnswers...)
	}
	if block.Value != nil {
		value := *block.Value
		if block.Tolerance != nil {
			value += " ± " + *block.Tolerance
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\n")
}

func (b *tutorBuild) writeAnswers(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.submissions(ctx, b.issues, nil, func(works []homework.TutorWork) error {
		for _, found := range works {
			work := tutorSubmissionBody(found, b.inventory.path)
			for _, answer := range work.Answers {
				block, _ := tutorBlockOf(work.Blocks, answer.QuestionID)
				attached := make([]string, 0, len(answer.Files))
				for _, file := range answer.Files {
					attached = append(attached, b.fileCell(file.Filename, file.Path))
				}
				if err := b.row(sheet, core.ExportSheetAnswers, exportformat.Text(work.Title), exportformat.Text(work.Student.DisplayName),
					exportformat.Text(optional(block.Prompt)), exportformat.Text(optional(answer.Value.Text)),
					exportformat.Text(tutorOptionTexts(block, answer.Value.OptionIDs)), exportformat.Text(strings.Join(attached, "\n")),
					optionalInteger(answer.AutoPoints), exportformat.Text(acceptedAnswers(block))); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (b *tutorBuild) writeReviews(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.submissions(ctx, b.issues, nil, func(works []homework.TutorWork) error {
		for _, found := range works {
			work := tutorSubmissionBody(found, b.inventory.path)
			for _, revision := range work.Revisions {
				if err := b.row(sheet, core.ExportSheetReviews, exportformat.Text(work.Title), exportformat.Text(work.Student.DisplayName),
					b.date(revision.PublishedAt), exportformat.Text(revision.Feedback), exportformat.Empty(), exportformat.Empty(), exportformat.Empty(),
					exportformat.Empty()); err != nil {
					return err
				}
				for _, item := range revision.Items {
					block, _ := tutorBlockOf(work.Blocks, item.QuestionID)
					corrected := make([]string, 0, len(item.CorrectedFiles))
					for _, file := range item.CorrectedFiles {
						corrected = append(corrected, b.fileCell(file.Filename, file.Path))
					}
					points := item.Points
					if item.OverridePoints != nil {
						points = item.OverridePoints
					}
					if err := b.row(sheet, core.ExportSheetReviews, exportformat.Text(work.Title), exportformat.Text(work.Student.DisplayName),
						b.date(revision.PublishedAt), exportformat.Empty(), exportformat.Text(optional(block.Prompt)), optionalInteger(points),
						exportformat.Text(item.Comment), exportformat.Text(strings.Join(corrected, "\n"))); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func pathNames(path []schemas.LibraryPathEntry) string {
	names := make([]string, 0, len(path))
	for _, entry := range path {
		names = append(names, entry.Name)
	}
	return strings.Join(names, " / ")
}

func (b *tutorBuild) folderPath(folder *string) string {
	if folder == nil {
		return ""
	}
	byID := b.folderIndex()
	var names []string
	for current := folder; current != nil && len(names) <= len(byID); {
		entry, found := byID[*current]
		if !found {
			break
		}
		names = append([]string{entry.Name}, names...)
		current = entry.ParentID
	}
	return strings.Join(names, " / ")
}

func (b *tutorBuild) writeLibrary(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.source.library(ctx, b.folders, nil, func(entries []libraryEntry) error {
		for _, entry := range entries {
			item := libraryItemBody(entry.item, b.inventory.path)
			folder := pathNames(item.Path)
			if folder == "" {
				folder = b.folderPath(item.FolderID)
			}
			var file string
			if item.Filename != nil {
				file = b.fileCell(*item.Filename, item.FilePath)
			}
			if err := b.row(sheet, core.ExportSheetLibrary, exportformat.Text(string(item.Kind)), exportformat.Text(item.Title), exportformat.Text(folder),
				exportformat.Text(optional(item.Filename)), exportformat.Text(file), optionalInteger(item.QuestionCount), optionalInteger(item.MaxPoints),
				b.date(&item.UpdatedAt)); err != nil {
				return err
			}
		}
		return b.advance(len(entries))
	})
}

func (b *tutorBuild) writeMessages(ctx context.Context, sheet *exportformat.Sheet) error {
	return b.writeChatSheet(ctx, sheet, b.source.chat())
}

func optionalCount(count *int) exportformat.Cell {
	if count == nil {
		return exportformat.Empty()
	}
	return exportformat.Integer(int64(*count))
}

func (b *tutorBuild) folderIndex() map[string]schemas.LibraryFolder {
	if b.foldersByID == nil {
		b.foldersByID = make(map[string]schemas.LibraryFolder, len(b.folders))
		for _, found := range b.folders {
			body := folderBody(found)
			b.foldersByID[body.ID] = body
		}
	}
	return b.foldersByID
}

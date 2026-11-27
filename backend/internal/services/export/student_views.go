package export

import (
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

type filePaths func(attachment domain.AttachmentID, reference files.DownloadReference) *string

func reference(kind domain.ReferenceKind, item [16]byte) files.DownloadReference {
	return files.DownloadReference{Kind: kind, Item: domain.EntityID(item)}
}

func utcOf(instant *time.Time) *time.Time {
	if instant == nil {
		return nil
	}
	value := instant.UTC()
	return &value
}

func tutorBody(tutor tutoring.MyTutor) schemas.MyTutor {
	body := schemas.MyTutor{
		RelationshipID: tutor.RelationshipID.String(), State: tutor.State, Colour: tutor.Colour,
		Tutor:   schemas.MyTutorProfile{Name: tutor.Tutor.Name, Subjects: append([]string{}, tutor.Tutor.Subjects...), Bio: tutor.Tutor.Bio},
		EndedAt: utcOf(tutor.EndedAt), EndedBy: tutor.EndedBy, AccountClosedUntil: utcOf(tutor.AccountClosedUntil),
	}
	if tutor.Tutor.AvatarID != nil {
		avatar := tutor.Tutor.AvatarID.String()
		body.Tutor.AvatarID = &avatar
	}
	if tutor.ChannelID != nil {
		channel := tutor.ChannelID.String()
		body.ChannelID = &channel
	}
	return body
}

func lessonBody(lesson schedule.Lesson, path filePaths) schemas.StudentExportLesson {
	audience := schemas.LessonAudience{
		Kind: lesson.Audience.Kind, Label: lesson.Audience.Label, LabelKind: lesson.Audience.LabelKind, Colour: lesson.Audience.Colour,
	}
	if lesson.Audience.RelationshipID != nil {
		relationship := lesson.Audience.RelationshipID.String()
		audience.RelationshipID = &relationship
	}
	if lesson.Audience.GroupID != nil {
		group := lesson.Audience.GroupID.String()
		audience.GroupID = &group
	}
	if lesson.Audience.Members != nil {
		audience.Members = make([]schemas.LessonMember, 0, len(lesson.Audience.Members))
		for _, member := range lesson.Audience.Members {
			audience.Members = append(audience.Members, schemas.LessonMember{Label: member.Label, LabelKind: member.LabelKind})
		}
	}
	detail := schemas.LessonDetail{
		ID: lesson.ID.String(), Title: lesson.Title, Description: lesson.Description, StartsAt: lesson.StartsAt, EndsAt: lesson.EndsAt(),
		DurationMinutes: lesson.DurationMinutes, Status: lesson.Status, TookPlace: lesson.TookPlace, Timezone: lesson.Timezone, Version: lesson.Version,
		CancelledAt: utcOf(lesson.CancelledAt), OtherTimezone: lesson.OtherTimezone, Audience: audience, RoomOpen: lesson.RoomOpen,
		EmbedAllowed: lesson.EmbedAllowed, Materials: []schemas.LessonMaterial{},
	}
	if lesson.Tutor != nil {
		detail.Tutor = &schemas.LessonTutor{Name: lesson.Tutor.Name, Colour: lesson.Tutor.Colour}
	}
	if lesson.ChannelID != nil {
		channel := lesson.ChannelID.String()
		detail.ChannelID = &channel
	}
	if lesson.SeriesID != nil {
		series := lesson.SeriesID.String()
		detail.SeriesID = &series
	}
	if lesson.OccurrenceDate != nil {
		occurrence := lesson.OccurrenceDate.String()
		detail.OccurrenceDate = &occurrence
	}
	if lesson.Series != nil {
		detail.Series = seriesBody(*lesson.Series)
	}
	body := schemas.StudentExportLesson{LessonDetail: detail, Materials: make([]schemas.StudentExportLessonMaterial, 0, len(lesson.Materials))}
	for _, material := range lesson.Materials {
		item := schemas.StudentExportLessonMaterial{
			AttachmentID: material.AttachmentID.String(), Title: material.Title, Filename: material.Filename, MediaType: material.MediaType,
			SizeBytes: material.SizeBytes, Width: material.Width, Height: material.Height,
			Path: path(material.AttachmentID, reference(domain.ReferenceLesson, lesson.ID)),
		}
		if material.MaterialID != nil {
			id := material.MaterialID.String()
			item.MaterialID = &id
		}
		body.Materials = append(body.Materials, item)
	}
	return body
}

func seriesBody(series schedule.SeriesView) *schemas.LessonSeries {
	body := &schemas.LessonSeries{
		ID: series.ID.String(), Weekdays: make([]int, 0, len(series.Weekdays)), Start: series.Start.String(), DurationMinutes: series.DurationMinutes,
		Timezone: series.Timezone, FirstDate: series.First.String(), Count: series.Count,
	}
	for _, day := range series.Weekdays {
		body.Weekdays = append(body.Weekdays, int(day))
	}
	if series.EndDate != nil {
		end := series.EndDate.String()
		body.EndDate = &end
	}
	return body
}

func assignmentTutorBody(tutor homework.TutorRef) schemas.StudentAssignmentTutor {
	return schemas.StudentAssignmentTutor{RelationshipID: tutor.Relationship.String(), Name: tutor.Name, Colour: tutor.Colour}
}

func assignmentBody(item homework.AssignmentItem) schemas.StudentAssignmentItem {
	return schemas.StudentAssignmentItem{
		SubmissionID: item.Submission.String(), Title: item.Title, Tutor: assignmentTutorBody(item.Tutor), Status: item.Status,
		Late: item.Late, IssuedBeforeReclaim: item.IssuedBeforeReclaim, DeadlineAt: utcOf(item.Deadline),
		IssuedAt: item.IssuedAt.UTC(), SubmittedAt: utcOf(item.SubmittedAt), CheckedAt: utcOf(item.CheckedAt),
		FeedbackExcerpt: item.FeedbackExcerpt, HasCorrectedFiles: item.HasCorrectedFiles, FeedbackNew: item.FeedbackNew,
	}
}

func exportFiles(views []homework.AnswerFileView, at files.DownloadReference, path filePaths) []schemas.StudentExportFile {
	body := make([]schemas.StudentExportFile, 0, len(views))
	for _, file := range views {
		body = append(body, schemas.StudentExportFile{
			AttachmentID: file.Attachment.String(), Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes,
			Width: file.Width, Height: file.Height, Path: path(file.Attachment, at),
		})
	}
	return body
}

func templateFileBody(file library.StudentFile, at files.DownloadReference, path filePaths) schemas.StudentExportTemplateFile {
	return schemas.StudentExportTemplateFile{
		Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height,
		Path: path(file.Attachment, at),
	}
}

func blockBody(block library.StudentBlock, at files.DownloadReference, path filePaths) schemas.StudentExportTemplateBlock {
	student := schemas.StudentTemplateBlock{
		ID: block.ID, Type: block.Type, Text: block.Text, Prompt: block.Prompt, MaxLength: block.MaxLength, MaxFiles: block.MaxFiles,
	}
	if block.Options != nil {
		options := make([]schemas.StudentTemplateOption, 0, len(block.Options))
		for _, option := range block.Options {
			options = append(options, schemas.StudentTemplateOption{ID: option.ID, Text: option.Text})
		}
		student.Options = &options
	}
	if block.AllowedKinds != nil {
		kinds := block.AllowedKinds
		student.AllowedKinds = &kinds
	}
	body := schemas.StudentExportTemplateBlock{StudentTemplateBlock: student}
	if block.Material != nil {
		file := block.Material.File
		body.Material = &schemas.StudentExportTemplateMaterial{
			Title: block.Material.Title, Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width,
			Height: file.Height, Path: path(file.Attachment, at),
		}
	}
	if block.Images != nil {
		images := make([]schemas.StudentExportTemplateFile, 0, len(block.Images))
		for _, image := range block.Images {
			images = append(images, templateFileBody(image, at, path))
		}
		body.Images = &images
	}
	return body
}

func submissionBody(work homework.StudentWork, path filePaths) schemas.StudentExportSubmission {
	snapshot := reference(domain.ReferenceIssueSnapshot, work.Issue)
	answers := reference(domain.ReferenceAnswer, work.ID)
	base := schemas.StudentSubmission{
		ID: work.ID.String(), Title: work.Title, Tutor: assignmentTutorBody(work.Tutor), Status: work.Status, Late: work.Late,
		IssuedBeforeReclaim: work.IssuedBeforeReclaim, DeadlineAt: utcOf(work.Deadline), Version: work.Version,
		Blocks: []schemas.StudentTemplateBlock{}, Answers: []schemas.StudentAnswer{}, EditableQuestionIDs: work.Editable,
	}
	if work.Revealed != nil {
		revealed := schemas.StudentRevealed{Questions: make([]schemas.StudentRevealedQuestion, 0, len(*work.Revealed))}
		for _, question := range *work.Revealed {
			revealed.Questions = append(revealed.Questions, schemas.StudentRevealedQuestion{
				QuestionID: question.Question, CorrectOptionIDs: question.CorrectOptionIDs, AcceptedAnswers: question.AcceptedAnswers,
				CorrectValue: question.CorrectValue, Tolerance: question.Tolerance, Explanation: question.Explanation,
			})
		}
		base.Revealed = &revealed
	}
	body := schemas.StudentExportSubmission{
		StudentSubmission: base, Blocks: make([]schemas.StudentExportTemplateBlock, 0, len(work.Blocks)),
		Answers: make([]schemas.StudentExportAnswer, 0, len(work.Answers)),
	}
	for _, block := range work.Blocks {
		body.Blocks = append(body.Blocks, blockBody(block, snapshot, path))
	}
	for _, answer := range work.Answers {
		body.Answers = append(body.Answers, schemas.StudentExportAnswer{
			StudentAnswer: schemas.StudentAnswer{
				QuestionID: answer.Question,
				Value:      schemas.StudentAnswerValue{OptionIDs: answer.Value.OptionIDs, Text: answer.Value.Text, AttachmentIDs: answer.Value.AttachmentIDs},
				Files:      []schemas.SubmissionFile{},
			},
			Files: exportFiles(answer.Files, answers, path),
		})
	}
	if work.Review != nil {
		corrected := reference(domain.ReferenceReviewItem, work.Review.Revision)
		review := schemas.StudentExportReview{
			StudentReview: schemas.StudentReview{Feedback: work.Review.Feedback, Items: []schemas.StudentReviewItem{}},
			Items:         make([]schemas.StudentExportReviewItem, 0, len(work.Review.Items)),
		}
		for _, item := range work.Review.Items {
			review.Items = append(review.Items, schemas.StudentExportReviewItem{
				StudentReviewItem: schemas.StudentReviewItem{QuestionID: item.Question, Comment: item.Comment, CorrectedFiles: []schemas.SubmissionFile{}},
				CorrectedFiles:    exportFiles(item.CorrectedFiles, corrected, path),
			})
		}
		body.Review = &review
	}
	return body
}

func channelBody(channel chat.Channel) schemas.ChannelItem {
	body := schemas.ChannelItem{
		ID: channel.ID.String(), Kind: channel.Kind, Label: channel.Label, LabelKind: channel.LabelKind, Colour: channel.Colour,
		UnreadCount: channel.UnreadCount, Writable: channel.Writable, Online: channel.Online, Unconfirmed: channel.Unconfirmed,
	}
	if channel.Tutor != nil {
		body.Tutor = &schemas.ChannelTutor{Name: channel.Tutor.Name, Colour: channel.Tutor.Colour}
	}
	if channel.UserID != nil {
		user := channel.UserID.String()
		body.UserID = &user
	}
	if last := channel.LastMessage; last != nil {
		body.LastMessage = &schemas.ChannelLastMessage{
			Seq: last.Seq, CreatedAt: last.CreatedAt.UTC(), Own: last.Own, SenderLabel: last.SenderLabel, SenderLabelKind: last.SenderLabelKind,
			Preview: last.Preview, AttachmentCount: last.AttachmentCount, PeerRead: nil,
		}
	}
	return body
}

func messageBody(message chat.Message, path filePaths) schemas.StudentExportMessage {
	base := schemas.Message{
		ID: message.ID.String(), Seq: message.Seq, SenderLabel: message.SenderLabel, SenderLabelKind: message.SenderLabelKind, Own: message.Own,
		Body: message.Body, CreatedAt: message.CreatedAt.UTC(), EditedAt: utcOf(message.EditedAt), Deleted: message.Deleted, Erased: message.Erased,
		Version: message.Version, SentBeforeReclaim: message.SentBeforeReclaim, Attachments: []schemas.MessageAttachment{},
		Reactions: make([]schemas.MessageReaction, 0, len(message.Reactions)),
	}
	for _, reaction := range message.Reactions {
		item := schemas.MessageReaction{Reaction: reaction.Reaction, Count: reaction.Count, Mine: reaction.Mine}
		if reaction.Members != nil {
			members := make([]schemas.ReactionMember, 0, len(*reaction.Members))
			for _, member := range *reaction.Members {
				members = append(members, schemas.ReactionMember{Label: member.Label, LabelKind: member.LabelKind})
			}
			item.Members = &members
		}
		base.Reactions = append(base.Reactions, item)
	}
	if quote := message.ReplyTo; quote != nil {
		base.ReplyTo = &schemas.MessageQuote{
			MessageID: quote.MessageID.String(), Seq: quote.Seq, SenderLabel: quote.SenderLabel, SenderLabelKind: quote.SenderLabelKind, Own: quote.Own,
			Preview: quote.Preview, State: quote.State, SentBeforeReclaim: quote.SentBeforeReclaim,
		}
	}
	body := schemas.StudentExportMessage{Message: base, Attachments: make([]schemas.StudentExportMessageAttachment, 0, len(message.Attachments))}
	for _, attachment := range message.Attachments {
		body.Attachments = append(body.Attachments, schemas.StudentExportMessageAttachment{
			ID: attachment.ID.String(), Filename: attachment.Filename, MediaType: attachment.MediaType, SizeBytes: attachment.SizeBytes,
			Width: attachment.Width, Height: attachment.Height, Path: path(attachment.ID, reference(domain.ReferenceMessage, message.ID)),
		})
	}
	return body
}

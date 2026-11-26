package export

import (
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

func optionalString[ID interface{ String() string }](id *ID) *string {
	if id == nil {
		return nil
	}
	text := (*id).String()
	return &text
}

func cardBody(card tutoring.Card) schemas.TutorExportCard {
	fields := schemas.CardFields{
		ID: card.ID.String(), State: card.State, DisplayName: card.DisplayName, Email: card.Email, EmailErased: card.EmailErased,
		Groups: make([]schemas.CardGroup, 0, len(card.Groups)), LinkedAt: utcOf(card.LinkedAt), EndedAt: utcOf(card.EndedAt),
		EndedBy: card.EndedBy, CreatedAt: card.CreatedAt.UTC(),
	}
	if card.Account != nil {
		fields.Account = &schemas.CardAccount{
			Name: card.Account.Name, Email: card.Account.Email, AvatarID: optionalString(card.Account.AvatarID), EmailConfirmed: card.Account.EmailConfirmed,
		}
	}
	for _, group := range card.Groups {
		fields.Groups = append(fields.Groups, schemas.CardGroup{ID: group.ID.String(), Name: group.Name, Colour: group.Colour})
	}
	body := schemas.TutorExportCard{
		StudentCard: schemas.StudentCard{CardFields: fields, Notes: card.Notes}, ChannelID: optionalString(card.ChannelID),
	}
	if card.Invitation != nil {
		body.Invitation = &schemas.TutorExportCardInvitation{ExpiresAt: card.Invitation.ExpiresAt.UTC(), Status: card.Invitation.Status}
	}
	return body
}

func groupBody(group tutoring.Group) schemas.TutorExportGroup {
	members := make([]schemas.GroupMember, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, schemas.GroupMember{
			RelationshipID: member.RelationshipID.String(), DisplayName: member.DisplayName, Name: member.Name,
			AvatarID: optionalString(member.AvatarID), AddedAt: member.AddedAt.UTC(),
		})
	}
	return schemas.TutorExportGroup{
		Group: schemas.Group{
			ID: group.ID.String(), Name: group.Name, Colour: group.Colour, ArchivedAt: utcOf(group.ArchivedAt), Members: members,
		},
		ChannelID: optionalString(group.ChannelID),
	}
}

func tutorLessonBody(lesson schedule.Lesson, path filePaths) schemas.TutorExportLesson {
	student := lessonBody(lesson, path)
	body := schemas.TutorExportLesson{LessonDetail: student.LessonDetail, Materials: make([]schemas.TutorExportLessonMaterial, 0, len(student.Materials))}
	for _, material := range student.Materials {
		body.Materials = append(body.Materials, schemas.TutorExportLessonMaterial(material))
	}
	return body
}

func issueItemBody(item homework.IssueItem) schemas.IssueItem {
	body := schemas.IssueItem{
		ID: item.ID.String(), Title: item.Title, DeadlineAt: utcOf(item.Deadline), AnswerPolicy: item.Policy, Status: item.Status,
		IssuedAt: item.IssuedAt.UTC(), WithdrawnAt: utcOf(item.WithdrawnAt), HasManualQuestions: item.HasManualQuestions,
		MaxPoints: item.MaxPoints, Version: item.Version, LessonID: optionalString(item.Lesson),
		Progress: schemas.IssueProgress{
			Students: item.Progress.Students, Submitted: item.Progress.Submitted, Checked: item.Progress.Checked,
			Returned: item.Progress.Returned, Late: item.Progress.Late, NeedsChecking: item.Progress.NeedsChecking,
		},
	}
	if item.Audience.Relationship != nil {
		body.Audience = schemas.IssueAudience{RelationshipID: optionalString(item.Audience.Relationship), LabelKind: item.Audience.LabelKind}
		if item.Audience.LabelKind == nil {
			name := item.Audience.DisplayName
			body.Audience.DisplayName = &name
		}
	}
	if item.Audience.Group != nil {
		name := item.Audience.Name
		body.Audience = schemas.IssueAudience{GroupID: optionalString(item.Audience.Group), Name: &name}
	}
	if item.Submission != nil {
		body.Submission = &schemas.IssueStudentSubmission{
			ID: item.Submission.ID.String(), Status: item.Submission.Status, Late: item.Submission.Late, FinalPoints: item.Submission.FinalPoints,
			IssuedBeforeReclaim: item.Submission.IssuedBeforeReclaim,
		}
	}
	return body
}

func issueFileBody(file homework.IssueFile, at files.DownloadReference, path filePaths) schemas.TutorExportTemplateFile {
	return schemas.TutorExportTemplateFile{
		AttachmentID: file.Attachment.String(), Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes,
		Width: file.Width, Height: file.Height, Path: path(file.Attachment, at),
	}
}

func issueBlockBody(block homework.IssueBlock, at files.DownloadReference, path filePaths) schemas.TutorExportIssueBlock {
	base := schemas.IssueBlock{
		ID: block.ID, Type: block.Type, Text: block.Text, Prompt: block.Prompt, Scoring: block.Scoring, AcceptedAnswers: block.AcceptedAnswers,
		Value: block.Value, Tolerance: block.Tolerance, MaxLength: block.MaxLength, AllowedKinds: block.AllowedKinds, MaxFiles: block.MaxFiles,
		Points: block.Points, Explanation: block.Explanation,
	}
	if block.Options != nil {
		options := make([]schemas.TemplateOption, 0, len(*block.Options))
		for _, option := range *block.Options {
			options = append(options, schemas.TemplateOption{ID: option.ID, Text: option.Text, Correct: option.Correct})
		}
		base.Options = &options
	}
	body := schemas.TutorExportIssueBlock{IssueBlock: base}
	if block.Material != nil {
		file := block.Material.File
		body.Material = &schemas.TutorExportIssueMaterial{
			AttachmentID: file.Attachment.String(), Title: block.Material.Title, Filename: file.Filename, MediaType: file.MediaType,
			SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height, Path: path(file.Attachment, at),
		}
	}
	if block.ImageIDs != nil {
		images := make([]schemas.TutorExportTemplateFile, 0, len(block.Images))
		for _, image := range block.Images {
			images = append(images, issueFileBody(image, at, path))
		}
		body.Images = &images
	}
	return body
}

func issueBody(issue homework.Issue, path filePaths) schemas.TutorExportIssue {
	at := reference(domain.ReferenceIssueSnapshot, issue.ID)
	base := schemas.Issue{
		IssueItem: issueItemBody(issue.IssueItem), Blocks: []schemas.IssueBlock{},
		Submissions: make([]schemas.IssueSubmission, 0, len(issue.Submissions)),
	}
	for _, submission := range issue.Submissions {
		base.Submissions = append(base.Submissions, schemas.IssueSubmission{
			ID: submission.ID.String(), Student: schemas.IssueSubmissionStudent{RelationshipID: submission.Relationship.String(), DisplayName: submission.DisplayName},
			Status: submission.Status, Late: submission.Late, FirstSubmittedAt: utcOf(submission.FirstSubmittedAt),
			SubmittedAt: utcOf(submission.SubmittedAt), CheckedAt: utcOf(submission.CheckedAt), FinalPoints: submission.FinalPoints,
			Percentage: submission.Percentage, IssuedBeforeReclaim: submission.IssuedBeforeReclaim, HasDraft: submission.HasDraft,
		})
	}
	body := schemas.TutorExportIssue{Issue: base, Blocks: make([]schemas.TutorExportIssueBlock, 0, len(issue.Blocks))}
	for _, block := range issue.Blocks {
		body.Blocks = append(body.Blocks, issueBlockBody(block, at, path))
	}
	return body
}

func tutorFiles(views []homework.AnswerFileView, at files.DownloadReference, path filePaths) []schemas.TutorExportFile {
	body := make([]schemas.TutorExportFile, 0, len(views))
	for _, file := range views {
		body = append(body, schemas.TutorExportFile{
			AttachmentID: file.Attachment.String(), Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes,
			Width: file.Width, Height: file.Height, Path: path(file.Attachment, at),
		})
	}
	return body
}

func latestPublished(revisions []homework.TutorRevision) *homework.TutorRevision {
	var latest *homework.TutorRevision
	for index := range revisions {
		revision := &revisions[index]
		if revision.State != domain.ReviewPublished {
			continue
		}
		if latest == nil || revision.Revision > latest.Revision {
			latest = revision
		}
	}
	return latest
}

func revisionBody(revision homework.TutorRevision, path filePaths) schemas.TutorExportRevision {
	var action *string
	if revision.Action != nil {
		text := string(*revision.Action)
		action = &text
	}
	corrected := reference(domain.ReferenceReviewItem, revision.ID)
	body := schemas.TutorExportRevision{
		TutorReviewRevision: schemas.TutorReviewRevision{
			Revision: revision.Revision, State: string(revision.State), Action: action, Feedback: revision.Feedback, Items: []schemas.TutorReviewItem{},
			Version: revision.Version, PublishedAt: utcOf(revision.PublishedAt),
		},
		Items: make([]schemas.TutorExportReviewItem, 0, len(revision.Items)),
	}
	for _, item := range revision.Items {
		body.Items = append(body.Items, schemas.TutorExportReviewItem{
			TutorReviewItem: schemas.TutorReviewItem{
				QuestionID: item.Question, Points: item.Points, OverridePoints: item.OverridePoints, OverrideReason: item.OverrideReason,
				Comment: item.Comment, CorrectedFiles: []schemas.SubmissionFile{},
			},
			CorrectedFiles: tutorFiles(item.CorrectedFiles, corrected, path),
		})
	}
	return body
}

func tutorSubmissionBody(work homework.TutorWork, path filePaths) schemas.TutorExportSubmission {
	snapshot := reference(domain.ReferenceIssueSnapshot, work.Issue)
	answers := reference(domain.ReferenceAnswer, work.ID)
	base := schemas.TutorSubmission{
		ID: work.ID.String(), IssueID: work.Issue.String(), Title: work.Title,
		Student: schemas.TutorSubmissionStudent{RelationshipID: work.Relationship.String(), DisplayName: work.DisplayName},
		Status:  work.Status, Late: work.Late, IssuedBeforeReclaim: work.IssuedBeforeReclaim, DeadlineAt: utcOf(work.Deadline),
		FirstSubmittedAt: utcOf(work.FirstSubmittedAt), SubmittedAt: utcOf(work.SubmittedAt), CheckedAt: utcOf(work.CheckedAt),
		Version: work.Version, AnswerPolicy: work.Policy, Blocks: []schemas.IssueBlock{}, Answers: []schemas.TutorAnswer{},
		Revisions: []schemas.TutorReviewRevision{}, AutoPoints: work.AutoPoints, FinalPoints: work.FinalPoints, MaxPoints: work.MaxPoints,
		Percentage: work.Percentage,
	}
	body := schemas.TutorExportSubmission{
		TutorSubmission: base, Blocks: make([]schemas.TutorExportIssueBlock, 0, len(work.Blocks)),
		Answers: make([]schemas.TutorExportAnswer, 0, len(work.Answers)), Revisions: []schemas.TutorExportRevision{},
	}
	for _, block := range work.Blocks {
		body.Blocks = append(body.Blocks, issueBlockBody(block, snapshot, path))
	}
	for _, answer := range work.Answers {
		body.Answers = append(body.Answers, schemas.TutorExportAnswer{
			TutorAnswer: schemas.TutorAnswer{
				QuestionID: answer.Question,
				Value:      schemas.StudentAnswerValue{OptionIDs: answer.Value.OptionIDs, Text: answer.Value.Text, AttachmentIDs: answer.Value.AttachmentIDs},
				Normalised: answer.Normalised, NormaliserVersion: answer.NormaliserVersion, AutoPoints: answer.AutoPoints, AutoCorrect: answer.AutoCorrect,
				FrozenAt: utcOf(answer.FrozenAt), Files: []schemas.SubmissionFile{},
			},
			Files: tutorFiles(answer.Files, answers, path),
		})
	}
	if latest := latestPublished(work.Revisions); latest != nil {
		body.Revisions = append(body.Revisions, revisionBody(*latest, path))
	}
	return body
}

func folderBody(folder library.Folder) schemas.LibraryFolder {
	return schemas.LibraryFolder{ID: folder.ID.String(), Name: folder.Name, ParentID: optionalString(folder.Parent)}
}

func libraryPathBody(path []library.PathEntry) []schemas.LibraryPathEntry {
	if path == nil {
		return nil
	}
	body := make([]schemas.LibraryPathEntry, 0, len(path))
	for _, entry := range path {
		body = append(body, schemas.LibraryPathEntry{ID: entry.ID.String(), Name: entry.Name})
	}
	return body
}

func libraryItemBody(item library.LibraryItem, path filePaths) schemas.TutorExportLibraryItem {
	if material := item.Material; material != nil {
		attachment, filename, mediaType, size := material.Attachment.String(), material.Filename, material.MediaType, material.SizeBytes
		return schemas.TutorExportLibraryItem{
			Kind: domain.LibraryItemMaterial, ID: material.ID.String(), Title: material.Title, FolderID: optionalString(material.Folder),
			Path: libraryPathBody(material.Path), UpdatedAt: material.UpdatedAt.UTC(), AttachmentID: &attachment, Filename: &filename,
			MediaType: &mediaType, SizeBytes: &size, Width: material.Width, Height: material.Height,
			FilePath: path(material.Attachment, reference(domain.ReferenceMaterial, material.ID)),
		}
	}
	template := item.Template
	questionCount, maxPoints, ready, version := template.QuestionCount, template.MaxPoints, template.Ready, template.Version
	return schemas.TutorExportLibraryItem{
		Kind: domain.LibraryItemTemplate, ID: template.ID.String(), Title: template.Title, FolderID: optionalString(template.Folder),
		Path: libraryPathBody(template.Path), UpdatedAt: template.UpdatedAt.UTC(), QuestionCount: &questionCount, MaxPoints: &maxPoints,
		Ready: &ready, Version: &version,
	}
}

func tutorTemplateFileBody(file library.TemplateFile, at files.DownloadReference, path filePaths) schemas.TutorExportTemplateFile {
	return schemas.TutorExportTemplateFile{
		AttachmentID: file.Attachment.String(), Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes,
		Width: file.Width, Height: file.Height, Path: path(file.Attachment, at),
	}
}

func templateBody(template library.Template, path filePaths) schemas.TutorExportTemplate {
	at := reference(domain.ReferenceTemplate, template.ID)
	base := schemas.Template{
		ID: template.ID.String(), Title: template.Title, FolderID: optionalString(template.Folder), Blocks: []schemas.TemplateBlock{},
		Version: template.Version, Ready: template.Ready, Problems: make([]schemas.TemplateProblem, 0, len(template.Problems)),
		QuestionCount: template.QuestionCount, MaxPoints: template.MaxPoints, CreatedAt: template.CreatedAt.UTC(), UpdatedAt: template.UpdatedAt.UTC(),
	}
	for _, problem := range template.Problems {
		base.Problems = append(base.Problems, schemas.TemplateProblem{BlockID: problem.Block, Code: problem.Code})
	}
	body := schemas.TutorExportTemplate{Template: base, Blocks: make([]schemas.TutorExportTemplateBlock, 0, len(template.Blocks))}
	for _, block := range template.Blocks {
		view := schemas.TemplateBlock{
			ID: block.ID, Type: block.Type, Text: block.Text, MaterialID: block.MaterialID, Prompt: block.Prompt, ImageIDs: block.ImageIDs,
			Scoring: block.Scoring, AcceptedAnswers: block.AcceptedAnswers, Value: block.Value, Tolerance: block.Tolerance, MaxLength: block.MaxLength,
			AllowedKinds: block.AllowedKinds, MaxFiles: block.MaxFiles, Points: block.Points, Explanation: block.Explanation,
		}
		if block.Options != nil {
			options := make([]schemas.TemplateOption, 0, len(*block.Options))
			for _, option := range *block.Options {
				options = append(options, schemas.TemplateOption{ID: option.ID, Text: option.Text, Correct: option.Correct})
			}
			view.Options = &options
		}
		exported := schemas.TutorExportTemplateBlock{TemplateBlock: view}
		if block.Material != nil {
			file := block.Material.File
			exported.Material = &schemas.TutorExportTemplateMaterial{
				MaterialID: block.Material.Material.String(), AttachmentID: file.Attachment.String(), Title: block.Material.Title,
				Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height,
				Deleted: block.Material.Deleted, Path: path(file.Attachment, at),
			}
		}
		if block.ImageIDs != nil {
			images := make([]schemas.TutorExportTemplateFile, 0, len(block.Images))
			for _, image := range block.Images {
				images = append(images, tutorTemplateFileBody(image, at, path))
			}
			exported.Images = &images
		}
		body.Blocks = append(body.Blocks, exported)
	}
	return body
}

func tutorMessageBody(message chat.Message, path filePaths) schemas.TutorExportMessage {
	student := messageBody(message, path)
	body := schemas.TutorExportMessage{Message: student.Message, Attachments: make([]schemas.TutorExportMessageAttachment, 0, len(student.Attachments))}
	for _, attachment := range student.Attachments {
		body.Attachments = append(body.Attachments, schemas.TutorExportMessageAttachment(attachment))
	}
	return body
}

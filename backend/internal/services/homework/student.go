package homework

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

const assignmentCursorParts = 3

var errStoredAnswerInvalid = errors.New("a stored answer cannot be decoded")

type TutorRef struct {
	Relationship domain.RelationshipID
	Name         string
	Colour       *domain.Colour
}

type AssignmentItem struct {
	Submission          domain.SubmissionID
	Title               string
	Tutor               TutorRef
	Status              domain.SubmissionStatus
	Late                bool
	IssuedBeforeReclaim bool
	Deadline            *time.Time
	IssuedAt            time.Time
	SubmittedAt         *time.Time
	CheckedAt           *time.Time
	FeedbackExcerpt     *string
	HasCorrectedFiles   bool
}

type AssignmentCounts struct {
	Todo   int64
	Closed int64
}

type AssignmentPage struct {
	Items      []AssignmentItem
	NextCursor *string
	Counts     AssignmentCounts
}

type AssignmentQuery struct {
	Group  domain.AssignmentGroup
	Tutor  *domain.RelationshipID
	Cursor string
	Limit  int32
}

type AnswerFileView struct {
	Attachment domain.AttachmentID
	Filename   string
	MediaType  string
	SizeBytes  int64
	Width      *int32
	Height     *int32
	URL        string
	ExpiresAt  time.Time
}

type StudentAnswer struct {
	Question string
	Value    domain.AnswerValue
	Files    []AnswerFileView
}

type RevealedQuestion struct {
	Question         string
	CorrectOptionIDs *[]string
	AcceptedAnswers  *[]string
	CorrectValue     *string
	Tolerance        *string
	Explanation      *string
}

type StudentReviewItem struct {
	Question       string
	Comment        string
	CorrectedFiles []AnswerFileView
}

type StudentReview struct {
	Feedback string
	Items    []StudentReviewItem
}

type StudentWork struct {
	ID                  domain.SubmissionID
	Title               string
	Tutor               TutorRef
	Status              domain.SubmissionStatus
	Late                bool
	IssuedBeforeReclaim bool
	Deadline            *time.Time
	Version             int64
	Blocks              []library.StudentBlock
	Answers             []StudentAnswer
	Editable            []string
	Review              *StudentReview
	Revealed            *[]RevealedQuestion
}

func (s *Service) Assignments(ctx context.Context, student domain.UserID, query AssignmentQuery) (AssignmentPage, error) {
	cursor, err := parseAssignmentCursor(query.Group, query.Cursor)
	if err != nil {
		return AssignmentPage{}, err
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return AssignmentPage{}, err
	}
	page := AssignmentPage{Items: []AssignmentItem{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if query.Tutor != nil {
			if err := repos.Homework.StudentTutorInScope(ctx, student, *query.Tutor, now); errors.Is(err, repositories.ErrNotFound) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
		}
		rows, err := repos.Homework.Assignments(ctx, repositories.AssignmentQuery{
			Student: student, Group: query.Group, Relationship: query.Tutor, After: cursor.set, AfterDeadlineAt: cursor.deadline,
			AfterInstant: cursor.instant, AfterID: cursor.id, PageSize: query.Limit + 1,
		}, now)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			next := encodeAssignmentCursor(query.Group, rows[len(rows)-1])
			page.NextCursor = &next
		}
		summaries, err := publishedSummaries(ctx, repos, rows)
		if err != nil {
			return err
		}
		for _, row := range rows {
			item := AssignmentItem{
				Submission: row.ID, Title: row.Title, Tutor: TutorRef{Relationship: row.RelationshipID, Name: row.TutorName, Colour: row.Colour},
				Status: row.Status, Late: row.Late, IssuedBeforeReclaim: row.IssuedBeforeReclaim, Deadline: row.DeadlineAt,
				IssuedAt: row.IssuedAt, SubmittedAt: row.SubmittedAt, CheckedAt: row.CheckedAt,
			}
			if row.PublishedRevisionID.Valid {
				if summary, found := summaries[domain.ReviewRevisionID(row.PublishedRevisionID.Bytes)]; found {
					item.FeedbackExcerpt, item.HasCorrectedFiles = domain.FeedbackExcerpt(summary.Feedback), summary.HasCorrectedFiles
				}
			}
			page.Items = append(page.Items, item)
		}
		counts, err := repos.Homework.AssignmentCounts(ctx, student, domain.AssignmentCountCap, now)
		page.Counts = AssignmentCounts{Todo: counts.Todo, Closed: counts.Closed}
		return err
	})
	return page, err
}

func (s *Service) StudentSubmission(ctx context.Context, student domain.UserID, id domain.SubmissionID) (StudentWork, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return StudentWork{}, err
	}
	var work StudentWork
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		work, err = s.studentWork(ctx, repos, student, id, now)
		return err
	})
	return work, err
}

func (s *Service) studentWork(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, id domain.SubmissionID, now time.Time,
) (StudentWork, error) {
	row, err := repos.Homework.StudentSubmission(ctx, student, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return StudentWork{}, ErrNotFound
	}
	if err != nil {
		return StudentWork{}, err
	}
	tutor := domain.TutorStanding{State: row.TutorState, DeletedAt: row.TutorDeletedAt}
	if !domain.ReadableStudentSubmissionAt(row.IssueStatus, row.MarkedForErasure, row.RelationshipState, tutor, now) {
		return StudentWork{}, ErrNotFound
	}
	stored, err := decodeSnapshot(row.Snapshot)
	if err != nil {
		return StudentWork{}, err
	}
	described, err := s.describeBlocks(ctx, repos, row.IssueID, row.Snapshot, now)
	if err != nil {
		return StudentWork{}, err
	}
	answers, err := s.answerViews(ctx, repos, id, now)
	if err != nil {
		return StudentWork{}, err
	}
	work := StudentWork{
		ID: row.ID, Title: row.Title, Tutor: TutorRef{Relationship: row.RelationshipID, Name: row.TutorName, Colour: row.Colour},
		Status: row.Status, Late: row.Late, IssuedBeforeReclaim: row.IssuedBeforeReclaim, Deadline: row.DeadlineAt, Version: row.Version,
		Blocks: make([]library.StudentBlock, 0, len(described)), Editable: domain.EditableQuestions(row.Status, stored),
		Answers: make([]StudentAnswer, 0, len(answers)),
	}
	for _, block := range described {
		work.Blocks = append(work.Blocks, studentBlockOf(block))
	}
	for _, answer := range answers {
		work.Answers = append(work.Answers, StudentAnswer{Question: answer.question, Value: answer.value, Files: answer.files})
	}
	if row.PublishedRevisionID.Valid {
		work.Review, err = s.studentReview(ctx, repos, domain.ReviewRevisionID(row.PublishedRevisionID.Bytes), stored, now)
		if err != nil {
			return StudentWork{}, err
		}
	}
	if domain.AnswersRevealed(row.AnswerPolicy, row.Status) {
		revealed := revealedOf(stored)
		work.Revealed = &revealed
	}
	return work, nil
}

func publishedSummaries(
	ctx context.Context, repos repositories.Repositories, rows []repositories.AssignmentRow,
) (map[domain.ReviewRevisionID]repositories.PublishedReviewSummary, error) {
	var revisions []domain.ReviewRevisionID
	for _, row := range rows {
		if row.PublishedRevisionID.Valid {
			revisions = append(revisions, domain.ReviewRevisionID(row.PublishedRevisionID.Bytes))
		}
	}
	found, err := repos.Homework.PublishedReviewSummaries(ctx, revisions)
	if err != nil {
		return nil, err
	}
	summaries := make(map[domain.ReviewRevisionID]repositories.PublishedReviewSummary, len(found))
	for _, summary := range found {
		summaries[summary.ID] = summary
	}
	return summaries, nil
}

func (s *Service) studentReview(
	ctx context.Context, repos repositories.Repositories, id domain.ReviewRevisionID, blocks []domain.IssueBlock, now time.Time,
) (*StudentReview, error) {
	revision, err := repos.Homework.PublishedRevision(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := decodeReviewItems(revision.Items)
	if err != nil {
		return nil, err
	}
	visible := domain.VisibleReview{Feedback: revision.Feedback, Items: items}
	if visible.Empty() {
		return nil, nil
	}
	files, err := s.correctedFiles(ctx, repos, []domain.ReviewRevisionID{id}, now)
	if err != nil {
		return nil, err
	}
	shown := map[string]domain.ReviewItem{}
	for _, item := range visible.Shown() {
		shown[item.QuestionID] = item
	}
	review := &StudentReview{Feedback: revision.Feedback, Items: []StudentReviewItem{}}
	for _, block := range blocks {
		item, found := shown[block.ID]
		if !found {
			continue
		}
		review.Items = append(review.Items, StudentReviewItem{Question: item.QuestionID, Comment: item.Comment, CorrectedFiles: files.of(id, item.QuestionID)})
	}
	return review, nil
}

type answerView struct {
	question string
	value    domain.AnswerValue
	stored   repositories.StoredAnswer
	files    []AnswerFileView
}

func (s *Service) answerViews(ctx context.Context, repos repositories.Repositories, submission domain.SubmissionID, now time.Time) ([]answerView, error) {
	rows, err := repos.Homework.Answers(ctx, submission)
	if err != nil {
		return nil, err
	}
	files, err := repos.Homework.AnswerFiles(ctx, submission)
	if err != nil {
		return nil, err
	}
	byQuestion := map[string][]AnswerFileView{}
	for _, file := range files {
		if file.QuestionID == nil {
			continue
		}
		question := domain.QuestionID(*file.QuestionID).String()
		url, expiresAt := s.deps.Addresses.AnswerAddress(file.ID, submission, dispositionOf(file.Width), now)
		byQuestion[question] = append(byQuestion[question], AnswerFileView{
			Attachment: file.ID, Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width,
			Height: file.Height, URL: url, ExpiresAt: expiresAt,
		})
	}
	views := make([]answerView, 0, len(rows))
	for _, row := range rows {
		var value domain.AnswerValue
		if err := json.Unmarshal(row.Value, &value); err != nil {
			return nil, fmt.Errorf("%w: %w", errStoredAnswerInvalid, err)
		}
		question := row.QuestionID.String()
		attached := byQuestion[question]
		if attached == nil {
			attached = []AnswerFileView{}
		}
		if value.AttachmentIDs != nil {
			referenced := make([]string, 0, len(attached))
			for _, file := range attached {
				referenced = append(referenced, file.Attachment.String())
			}
			value.AttachmentIDs = &referenced
		}
		views = append(views, answerView{question: question, value: value, stored: row, files: attached})
	}
	return views, nil
}

func studentBlockOf(block IssueBlock) library.StudentBlock {
	student := library.StudentBlock{ID: block.ID, Type: block.Type}
	switch block.Type {
	case domain.BlockInstruction:
		student.Text = emptyWhenMissing(block.Text)
	case domain.BlockMaterial:
		if block.Material != nil {
			student.Material = &library.StudentMaterial{Title: block.Material.Title, File: studentFileOf(block.Material.File)}
		}
	case domain.BlockSingleChoice, domain.BlockMultipleChoice:
		studentQuestion(&student, block)
		student.Options = []library.StudentOption{}
		if block.Options != nil {
			for _, option := range *block.Options {
				student.Options = append(student.Options, library.StudentOption{ID: option.ID, Text: option.Text})
			}
		}
	case domain.BlockShortText, domain.BlockShortNumber:
		studentQuestion(&student, block)
	case domain.BlockLongText:
		studentQuestion(&student, block)
		student.MaxLength = block.MaxLength
	case domain.BlockFile:
		studentQuestion(&student, block)
		student.MaxFiles = block.MaxFiles
		if block.AllowedKinds != nil {
			student.AllowedKinds = *block.AllowedKinds
		}
	}
	return student
}

func studentQuestion(student *library.StudentBlock, block IssueBlock) {
	student.Prompt = emptyWhenMissing(block.Prompt)
	student.Images = make([]library.StudentFile, 0, len(block.Images))
	for _, image := range block.Images {
		student.Images = append(student.Images, studentFileOf(image))
	}
}

func studentFileOf(file IssueFile) library.StudentFile {
	return library.StudentFile{
		Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height, URL: file.URL,
		ExpiresAt: file.ExpiresAt,
	}
}

func emptyWhenMissing(text *string) *string {
	if text != nil {
		return text
	}
	empty := ""
	return &empty
}

func revealedOf(blocks []domain.IssueBlock) []RevealedQuestion {
	revealed := []RevealedQuestion{}
	for _, block := range blocks {
		if !block.Type.Question() {
			continue
		}
		question := RevealedQuestion{Question: block.ID, Explanation: block.Explanation}
		switch block.Type {
		case domain.BlockSingleChoice, domain.BlockMultipleChoice:
			correct := []string{}
			if block.Options != nil {
				for _, option := range *block.Options {
					if option.Correct != nil && *option.Correct {
						correct = append(correct, option.ID)
					}
				}
			}
			question.CorrectOptionIDs = &correct
		case domain.BlockShortText:
			accepted := []string{}
			if block.AcceptedAnswers != nil {
				accepted = slices.Clone(*block.AcceptedAnswers)
			}
			question.AcceptedAnswers = &accepted
		case domain.BlockShortNumber:
			question.CorrectValue, question.Tolerance = block.Value, block.Tolerance
		case domain.BlockInstruction, domain.BlockMaterial, domain.BlockLongText, domain.BlockFile:
		}
		revealed = append(revealed, question)
	}
	return revealed
}

type assignmentCursor struct {
	set      bool
	deadline *time.Time
	instant  *time.Time
	id       domain.SubmissionID
}

func encodeAssignmentCursor(group domain.AssignmentGroup, row repositories.AssignmentRow) string {
	deadline := ""
	instant := row.IssuedAt
	switch group {
	case domain.AssignmentsTodo:
		if row.DeadlineAt != nil {
			deadline = strconv.FormatInt(row.DeadlineAt.UnixMicro(), 10)
		}
	case domain.AssignmentsSubmitted:
		if row.SubmittedAt != nil {
			instant = *row.SubmittedAt
		}
	case domain.AssignmentsChecked:
		if row.CheckedAt != nil {
			instant = *row.CheckedAt
		}
	case domain.AssignmentsClosed:
	}
	text := strings.Join([]string{deadline, strconv.FormatInt(instant.UnixMicro(), 10), row.ID.String()}, cursorSeparator)
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func parseAssignmentCursor(group domain.AssignmentGroup, text string) (assignmentCursor, error) {
	if text == "" {
		return assignmentCursor{}, nil
	}
	refused := invalid(fieldCursor, domain.FieldInvalid)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return assignmentCursor{}, refused
	}
	parts := strings.Split(string(decoded), cursorSeparator)
	if len(parts) != assignmentCursorParts {
		return assignmentCursor{}, refused
	}
	cursor := assignmentCursor{set: true}
	if parts[0] != "" {
		if group != domain.AssignmentsTodo {
			return assignmentCursor{}, refused
		}
		micros, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return assignmentCursor{}, refused
		}
		deadline := time.UnixMicro(micros).UTC()
		cursor.deadline = &deadline
	}
	micros, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return assignmentCursor{}, refused
	}
	instant := time.UnixMicro(micros).UTC()
	cursor.instant = &instant
	id, valid := domain.ParseUUID[domain.SubmissionID](parts[2])
	if !valid {
		return assignmentCursor{}, refused
	}
	cursor.id = id
	return cursor, nil
}

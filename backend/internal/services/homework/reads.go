package homework

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const cursorSeparator = "_"

var errStoredFileGone = errors.New("a file named by a stored snapshot is gone")

func (s *Service) Issues(ctx context.Context, tutor domain.UserID, query IssueListQuery) (IssuePage, error) {
	after, afterID, err := parseCursor[domain.IssueID](query.Cursor)
	if err != nil {
		return IssuePage{}, err
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return IssuePage{}, err
	}
	page := IssuePage{Items: []IssueItem{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := requireFilters(ctx, repos, tutor, query); err != nil {
			return err
		}
		rows, err := repos.Homework.Issues(ctx, repositories.IssueListQuery{
			Tutor: tutor, Status: query.Status, Group: query.Group, Relationship: query.Student, NeedsChecking: query.NeedsChecking,
			DeadlineBefore: query.DeadlineBefore, Now: now, AfterInstant: after, AfterID: afterID, PageSize: query.Limit + 1,
		})
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			instant := last.IssuedAt
			if query.DeadlineBefore != nil && last.DeadlineAt != nil {
				instant = *last.DeadlineAt
			}
			next := encodeCursor(instant, last.ID)
			page.NextCursor = &next
		}
		for _, row := range rows {
			page.Items = append(page.Items, itemOfRow(row))
		}
		return s.complete(ctx, repos, tutor, page.Items, query.Student)
	})
	return page, err
}

func requireFilters(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, query IssueListQuery) error {
	if query.Group != nil {
		if err := repos.Homework.GroupInScope(ctx, tutor, *query.Group); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	if query.Student != nil {
		if err := repos.Homework.StudentInScope(ctx, tutor, *query.Student); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	return nil
}

func itemOfRow(row repositories.IssueListRow) IssueItem {
	item := IssueItem{
		ID: row.ID, Title: row.Title, Lesson: row.LessonID, Deadline: row.DeadlineAt, Policy: row.AnswerPolicy, Status: row.Status,
		IssuedAt: row.IssuedAt, WithdrawnAt: row.WithdrawnAt, HasManualQuestions: row.HasManualQuestions, MaxPoints: row.MaxPoints,
		Version:  row.Version,
		Audience: Audience{Relationship: row.RelationshipID, Group: row.GroupID},
	}
	if row.CardName != nil {
		item.Audience.DisplayName = *row.CardName
	}
	if row.GroupName != nil {
		item.Audience.Name = *row.GroupName
	}
	return item
}

func itemOfIssue(row repositories.TutorIssue) IssueItem {
	return itemOfRow(repositories.IssueListRow{
		ID: row.ID, RelationshipID: row.RelationshipID, GroupID: row.GroupID, LessonID: row.LessonID, Title: row.Title,
		MaxPoints: row.MaxPoints, HasManualQuestions: row.HasManualQuestions, DeadlineAt: row.DeadlineAt, AnswerPolicy: row.AnswerPolicy,
		Status: row.Status, Version: row.Version, IssuedAt: row.IssuedAt, WithdrawnAt: row.WithdrawnAt, CardName: row.CardName,
		GroupName: row.GroupName,
	})
}

func (s *Service) complete(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, items []IssueItem, student *domain.RelationshipID,
) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]domain.IssueID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	progress, err := repos.Homework.Progress(ctx, tutor, ids)
	if err != nil {
		return err
	}
	byIssue := make(map[domain.IssueID]Progress, len(progress))
	for _, row := range progress {
		byIssue[row.IssueID] = Progress{
			Students: row.Students, Submitted: row.Submitted, Checked: row.Checked, Returned: row.Returned, Late: row.Late,
			NeedsChecking: row.NeedsChecking,
		}
	}
	submissions := map[domain.IssueID]StudentSubmission{}
	if student != nil {
		rows, err := repos.Homework.RelationshipSubmissions(ctx, tutor, *student, ids)
		if err != nil {
			return err
		}
		for _, row := range rows {
			submissions[row.IssueID] = StudentSubmission{
				ID: row.ID, Status: row.Status, Late: row.Late, FinalPoints: row.FinalPoints, IssuedBeforeReclaim: row.IssuedBeforeReclaim,
			}
		}
	}
	for index := range items {
		items[index].Progress = byIssue[items[index].ID]
		if submission, found := submissions[items[index].ID]; found {
			items[index].Submission = &submission
		}
	}
	return nil
}

func (s *Service) itemOf(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.IssueID) (IssueItem, error) {
	row, err := repos.Homework.Issue(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return IssueItem{}, ErrNotFound
	}
	if err != nil {
		return IssueItem{}, err
	}
	items := []IssueItem{itemOfIssue(row)}
	if err := s.complete(ctx, repos, tutor, items, nil); err != nil {
		return IssueItem{}, err
	}
	return items[0], nil
}

func (s *Service) Issue(ctx context.Context, tutor domain.UserID, id domain.IssueID) (Issue, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Homework.Issue(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		items := []IssueItem{itemOfIssue(row)}
		if err := s.complete(ctx, repos, tutor, items, nil); err != nil {
			return err
		}
		issue.IssueItem = items[0]
		if issue.Blocks, err = s.describeBlocks(ctx, repos, row.ID, row.Snapshot, now); err != nil {
			return err
		}
		issue.Submissions, err = s.issueSubmissions(ctx, repos, tutor, row)
		return err
	})
	return issue, err
}

func (s *Service) issueSubmissions(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, row repositories.TutorIssue) ([]IssueSubmission, error) {
	rows, err := repos.Homework.IssueSubmissions(ctx, tutor, row.ID)
	if err != nil {
		return nil, err
	}
	submissions := make([]IssueSubmission, 0, len(rows))
	for _, submission := range rows {
		view := IssueSubmission{
			ID: submission.ID, Relationship: submission.RelationshipID, DisplayName: submission.DisplayName, Status: submission.Status,
			Late: submission.Late, FirstSubmittedAt: submission.FirstSubmittedAt, SubmittedAt: submission.SubmittedAt,
			CheckedAt: submission.CheckedAt, FinalPoints: submission.FinalPoints, IssuedBeforeReclaim: submission.IssuedBeforeReclaim,
			HasDraft: submission.HasDraft,
		}
		if submission.FinalPoints != nil {
			if percentage, scored := domain.ScorePercentage(int64(*submission.FinalPoints), int64(row.MaxPoints)); scored {
				view.Percentage = &percentage
			}
		}
		submissions = append(submissions, view)
	}
	return submissions, nil
}

func (s *Service) describeBlocks(
	ctx context.Context, repos repositories.Repositories, issue domain.IssueID, snapshot []byte, now time.Time,
) ([]IssueBlock, error) {
	stored, err := decodeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	var wanted []domain.AttachmentID
	for _, block := range stored {
		if block.ImageIDs != nil {
			for _, text := range *block.ImageIDs {
				if image, valid := domain.ParseUUID[domain.AttachmentID](text); valid {
					wanted = append(wanted, image)
				}
			}
		}
		if block.Material != nil {
			if attachment, valid := domain.ParseUUID[domain.AttachmentID](block.Material.AttachmentID); valid {
				wanted = append(wanted, attachment)
			}
		}
	}
	files := map[domain.AttachmentID]IssueFile{}
	if len(wanted) > 0 {
		described, err := repos.Library.TemplateImageFiles(ctx, wanted)
		if err != nil {
			return nil, err
		}
		for _, file := range described {
			url, expiresAt := s.deps.Addresses.IssueSnapshotAddress(file.ID, issue, dispositionOf(file.Width), now)
			files[file.ID] = IssueFile{
				Attachment: file.ID, Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width,
				Height: file.Height, URL: url, ExpiresAt: expiresAt,
			}
		}
	}
	blocks := make([]IssueBlock, 0, len(stored))
	for _, block := range stored {
		described := IssueBlock{TemplateBlock: block.TemplateBlock}
		if block.ImageIDs != nil {
			described.Images = make([]IssueFile, 0, len(*block.ImageIDs))
			for _, text := range *block.ImageIDs {
				image, _ := domain.ParseUUID[domain.AttachmentID](text)
				file, found := files[image]
				if !found {
					return nil, fmt.Errorf("issue %s: %w", issue, errStoredFileGone)
				}
				described.Images = append(described.Images, file)
			}
		}
		if block.Material != nil {
			attachment, _ := domain.ParseUUID[domain.AttachmentID](block.Material.AttachmentID)
			file, found := files[attachment]
			if !found {
				return nil, fmt.Errorf("issue %s: %w", issue, errStoredFileGone)
			}
			described.Material = &IssueMaterial{Title: block.Material.Title, File: file}
		}
		blocks = append(blocks, described)
	}
	return blocks, nil
}

func dispositionOf(width *int32) domain.Disposition {
	if width != nil {
		return domain.DispositionInline
	}
	return domain.DispositionAttachment
}

type cursorID interface {
	~[16]byte
	String() string
}

func encodeCursor[ID cursorID](instant time.Time, id ID) string {
	text := strconv.FormatInt(instant.UnixMicro(), 10) + cursorSeparator + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func parseCursor[ID cursorID](text string) (*time.Time, ID, error) {
	var none ID
	if text == "" {
		return nil, none, nil
	}
	refused := invalid(fieldCursor, domain.FieldInvalid)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, none, refused
	}
	micros, idText, found := strings.Cut(string(decoded), cursorSeparator)
	if !found {
		return nil, none, refused
	}
	value, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, none, refused
	}
	id, valid := domain.ParseUUID[ID](idText)
	if !valid {
		return nil, none, refused
	}
	instant := time.UnixMicro(value).UTC()
	return &instant, id, nil
}

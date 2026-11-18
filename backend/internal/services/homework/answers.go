package homework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/library"
)

const (
	limitAnswerSaveSecond = "answer_saves_second"
	limitAnswerSaveHour   = "answer_saves_hour"
	recordSubmission      = "submission_id"
	fieldAnswers          = "body.answers"
	saveAttempts          = 2
)

var errFilePlanChanged = errors.New("the files a save binds or releases changed under the locks")

type AnswerInput struct {
	Question string
	Value    domain.AnswerValue
}

type AnswersSave struct {
	Version int64
	SaveID  domain.EntityID
	Answers []AnswerInput
}

type studentLocks struct {
	parties      repositories.SubmissionParties
	relationship domain.RelationshipState
	issue        repositories.LockedSubmissionIssue
	submission   repositories.LockedSubmission
	blocks       []domain.IssueBlock
	now          time.Time
}

type filePlan struct {
	bound    []repositories.BoundAnswerFile
	released []domain.AttachmentID
	bytes    int64
}

func (p filePlan) touches() bool {
	return len(p.bound) > 0 || len(p.released) > 0
}

func (s *Service) reserveAnswerSlots(ctx context.Context, principal accounts.Principal, id domain.SubmissionID) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockStudent(ctx, repos, principal)
		if err != nil {
			return err
		}
		student := principal.UserID
		second, secondRetry := domain.FixedWindow(now, domain.AnswerSaveSecondWindow)
		perSubmission, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
			Purpose:     domain.PurposeAnswerSavesSecond,
			Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeAnswerSavesSecond), student.String()+":"+id.String()),
			WindowStart: second,
		})
		if err != nil {
			return err
		}
		hour, hourRetry := domain.FixedWindow(now, domain.AnswerSaveHourWindow)
		perStudent, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
			Purpose:     domain.PurposeAnswerSavesHour,
			Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeAnswerSavesHour), student.String()),
			WindowStart: hour,
		})
		if err != nil {
			return err
		}
		retryAfter, limit := time.Duration(0), ""
		if perSubmission > domain.AnswerSavesPerSecond {
			retryAfter, limit = secondRetry, limitAnswerSaveSecond
		}
		if perStudent > domain.AnswerSavesPerHour {
			retryAfter, limit = max(retryAfter, hourRetry), limitAnswerSaveHour
		}
		if retryAfter == 0 {
			return nil
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "homework.rate_limited", slog.String("user_id", student.String()),
			slog.String(recordSubmission, id.String()), slog.String("limit", limit))
		return &ThrottledError{RetryAfter: retryAfter}
	})
}

func (s *Service) lockStudent(ctx context.Context, repos repositories.Repositories, principal accounts.Principal) (time.Time, error) {
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return time.Time{}, err
	}
	err = accounts.LockActingAccount(ctx, repos, principal, now, principal.State)
	if errors.Is(err, accounts.ErrUnauthenticated) {
		return time.Time{}, ErrUnauthenticated
	}
	if err != nil {
		return time.Time{}, err
	}
	return s.deps.Clock.InstantWithin(ctx, repos.TestClock)
}

func (s *Service) lockStudentWork(
	ctx context.Context, repos repositories.Repositories, principal accounts.Principal, id domain.SubmissionID, withProfile bool,
) (studentLocks, error) {
	now, err := s.lockStudent(ctx, repos, principal)
	if err != nil {
		return studentLocks{}, err
	}
	parties, err := repos.Homework.SubmissionParties(ctx, principal.UserID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return studentLocks{}, ErrNotFound
	}
	if err != nil {
		return studentLocks{}, err
	}
	if withProfile {
		err := repos.TutorProfiles.Lock(ctx, parties.TutorID)
		if errors.Is(err, repositories.ErrNotFound) {
			return studentLocks{}, ErrNotFound
		}
		if err != nil {
			return studentLocks{}, err
		}
	}
	relationship, err := repos.Homework.LockSubmissionRelationship(ctx, parties.TutorID, parties.RelationshipID)
	if errors.Is(err, repositories.ErrNotFound) {
		return studentLocks{}, ErrNotFound
	}
	if err != nil {
		return studentLocks{}, err
	}
	issue, err := repos.Homework.LockSubmissionIssue(ctx, parties.TutorID, parties.IssueID)
	if errors.Is(err, repositories.ErrNotFound) {
		return studentLocks{}, ErrNotFound
	}
	if err != nil {
		return studentLocks{}, err
	}
	tutor, err := repos.Users.TutorStanding(ctx, parties.TutorID)
	if errors.Is(err, repositories.ErrNotFound) {
		return studentLocks{}, ErrNotFound
	}
	if err != nil {
		return studentLocks{}, err
	}
	if !domain.ReadableStudentSubmissionAt(issue.Status, issue.MarkedForErasure, relationship, tutor, now) {
		return studentLocks{}, ErrNotFound
	}
	submission, err := repos.Homework.LockSubmission(ctx, parties.IssueID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return studentLocks{}, ErrNotFound
	}
	if err != nil {
		return studentLocks{}, err
	}
	blocks, err := decodeSnapshot(issue.Snapshot)
	if err != nil {
		return studentLocks{}, err
	}
	return studentLocks{parties: parties, relationship: relationship, issue: issue, submission: submission, blocks: blocks, now: now}, nil
}

func (s *Service) SaveAnswers(ctx context.Context, principal accounts.Principal, id domain.SubmissionID, save AnswersSave) (StudentWork, error) {
	if err := s.reserveAnswerSlots(ctx, principal, id); err != nil {
		return StudentWork{}, err
	}
	withProfile, err := s.decideFiles(ctx, principal.UserID, id, save)
	if err != nil {
		return StudentWork{}, err
	}
	for attempt := 1; ; attempt++ {
		work, err := s.saveAnswers(ctx, principal, id, save, withProfile)
		if !errors.Is(err, errFilePlanChanged) {
			return work, err
		}
		if attempt == saveAttempts {
			return StudentWork{}, ErrConflict
		}
		withProfile = !withProfile
	}
}

func (s *Service) decideFiles(ctx context.Context, student domain.UserID, id domain.SubmissionID, save AnswersSave) (bool, error) {
	if !slices.ContainsFunc(save.Answers, func(answer AnswerInput) bool { return answer.Value.AttachmentIDs != nil }) {
		return false, nil
	}
	var touches bool
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if _, err := repos.Homework.SubmissionParties(ctx, student, id); errors.Is(err, repositories.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		existing, err := repos.Homework.AnswerReferences(ctx, id)
		if err != nil {
			return err
		}
		touches = requestTouchesFiles(save, existing)
		return nil
	})
	return touches, err
}

func requestTouchesFiles(save AnswersSave, existing []repositories.AnswerReference) bool {
	for _, answer := range save.Answers {
		if answer.Value.AttachmentIDs == nil {
			continue
		}
		held := heldFiles(existing, answer.Question)
		wanted := *answer.Value.AttachmentIDs
		if len(held) != len(wanted) {
			return true
		}
		for _, text := range wanted {
			attachment, valid := domain.ParseUUID[domain.AttachmentID](text)
			if !valid || !slices.Contains(held, attachment) {
				return true
			}
		}
	}
	return false
}

func heldFiles(existing []repositories.AnswerReference, question string) []domain.AttachmentID {
	var held []domain.AttachmentID
	for _, reference := range existing {
		if reference.QuestionID != nil && domain.QuestionID(*reference.QuestionID).String() == question {
			held = append(held, reference.AttachmentID)
		}
	}
	return held
}

func (s *Service) saveAnswers(
	ctx context.Context, principal accounts.Principal, id domain.SubmissionID, save AnswersSave, withProfile bool,
) (StudentWork, error) {
	var work StudentWork
	var plan filePlan
	changed, replayed := 0, false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := s.lockStudentWork(ctx, repos, principal, id, withProfile)
		if err != nil {
			return err
		}
		student := principal.UserID
		if locked.submission.LastSaveID.Valid && domain.EntityID(locked.submission.LastSaveID.Bytes) == save.SaveID {
			replayed = true
			work, err = s.studentWork(ctx, repos, student, id, locked.now)
			return err
		}
		if !domain.WritableStudentSubmission(locked.submission.Status, locked.relationship) {
			return ErrNotEditable
		}
		if locked.submission.Version != save.Version {
			s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "homework.save_conflict", slog.String(recordSubmission, id.String()),
				slog.String("kind", "answers"))
			return ErrConflict
		}
		questions, err := checkAnswers(save.Answers, locked)
		if err != nil {
			return err
		}
		plan, err = s.planFiles(ctx, repos, student, id, save, questions, locked.now)
		if err != nil {
			return err
		}
		if plan.touches() && !withProfile {
			return errFilePlanChanged
		}
		if locked.submission.FileBytes+plan.bytes > s.deps.Caps.SubmissionFileBytes {
			return invalid(fieldAnswers, domain.FieldTooMany)
		}
		if err := s.applyFiles(ctx, repos, locked.parties.TutorID, id, plan, locked.now); err != nil {
			return err
		}
		for index, answer := range save.Answers {
			encoded, err := json.Marshal(answer.Value)
			if err != nil {
				return fmt.Errorf("encode an answer: %w", err)
			}
			if _, err := repos.Homework.UpsertAnswer(ctx, id, questions[index], encoded, locked.now); err != nil {
				return err
			}
			changed++
		}
		status := locked.submission.Status
		if status == domain.SubmissionNotStarted {
			status = domain.SubmissionDraft
		}
		if err := repos.Homework.SaveSubmission(ctx, id, status, save.SaveID, locked.now); err != nil {
			return err
		}
		work, err = s.studentWork(ctx, repos, student, id, locked.now)
		return err
	})
	if errors.Is(err, ErrQuotaExceeded) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", slog.String(recordSubmission, id.String()),
			slog.String("context", string(domain.ContextAnswer)))
	}
	if err != nil || replayed {
		return work, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "homework.submission.saved", slog.String(recordSubmission, id.String()),
		slog.Int("answers_changed", changed), slog.Int("files_bound", len(plan.bound)), slog.Int("files_released", len(plan.released)))
	return work, nil
}

func checkAnswers(answers []AnswerInput, locked studentLocks) ([]domain.QuestionID, error) {
	blocks := map[string]domain.IssueBlock{}
	for _, block := range locked.blocks {
		if block.Type.Question() {
			blocks[block.ID] = block
		}
	}
	submitted := locked.submission.FirstSubmittedAt != nil
	for _, answer := range answers {
		if block, found := blocks[answer.Question]; found && block.Type.Automatic() && submitted {
			return nil, ErrAnswerFrozen
		}
	}
	questions := make([]domain.QuestionID, 0, len(answers))
	var files []domain.AttachmentID
	for index, answer := range answers {
		path := fieldAnswers + "[" + strconv.Itoa(index) + "]"
		block, found := blocks[answer.Question]
		question, valid := domain.ParseUUID[domain.QuestionID](answer.Question)
		if !found || !valid || slices.Contains(questions, question) {
			return nil, invalid(path+".question_id", domain.FieldInvalid)
		}
		if problem := checkValue(block, answer.Value, path+".value"); problem != nil {
			return nil, problem
		}
		if answer.Value.AttachmentIDs != nil {
			for _, text := range *answer.Value.AttachmentIDs {
				attachment, _ := domain.ParseUUID[domain.AttachmentID](text)
				if slices.Contains(files, attachment) {
					return nil, invalid(path+".value.attachment_ids", domain.FieldInvalid)
				}
				files = append(files, attachment)
			}
		}
		questions = append(questions, question)
	}
	return questions, nil
}

func checkValue(block domain.IssueBlock, value domain.AnswerValue, path string) *ValidationError {
	if !value.FitsQuestion(block.Type) {
		return invalid(path, domain.FieldInvalid)
	}
	switch block.Type {
	case domain.BlockSingleChoice, domain.BlockMultipleChoice:
		selected := *value.OptionIDs
		if block.Type == domain.BlockSingleChoice && len(selected) > 1 {
			return invalid(path+".option_ids", domain.FieldInvalid)
		}
		var options []string
		if block.Options != nil {
			for _, option := range *block.Options {
				options = append(options, option.ID)
			}
		}
		for index, option := range selected {
			if !slices.Contains(options, option) || slices.Contains(selected[:index], option) {
				return invalid(path+".option_ids", domain.FieldInvalid)
			}
		}
	case domain.BlockShortText:
		return textWithin(*value.Text, domain.AnswerShortTextMaxRunes, path)
	case domain.BlockShortNumber:
		return textWithin(*value.Text, domain.AnswerNumberMaxRunes, path)
	case domain.BlockLongText:
		if block.MaxLength == nil {
			return invalid(path, domain.FieldInvalid)
		}
		return textWithin(*value.Text, int(*block.MaxLength), path)
	case domain.BlockFile:
		files := *value.AttachmentIDs
		for index, text := range files {
			if _, valid := domain.ParseUUID[domain.AttachmentID](text); !valid || slices.Contains(files[:index], text) {
				return invalid(path+".attachment_ids", domain.FieldInvalid)
			}
		}
		if block.MaxFiles == nil || len(files) > int(*block.MaxFiles) {
			return invalid(path+".attachment_ids", domain.FieldTooMany)
		}
	case domain.BlockInstruction, domain.BlockMaterial:
		return invalid(path, domain.FieldInvalid)
	}
	return nil
}

func textWithin(text string, limit int, path string) *ValidationError {
	if !utf8.ValidString(text) || containsNull(text) {
		return invalid(path+".text", domain.FieldInvalid)
	}
	if utf8.RuneCountInString(text) > limit {
		return invalid(path+".text", domain.FieldTooLong)
	}
	return nil
}

func containsNull(text string) bool {
	for _, letter := range text {
		if letter == 0 {
			return true
		}
	}
	return false
}

func (s *Service) planFiles(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, id domain.SubmissionID, save AnswersSave,
	questions []domain.QuestionID, now time.Time,
) (filePlan, error) {
	if !slices.ContainsFunc(save.Answers, func(answer AnswerInput) bool { return answer.Value.AttachmentIDs != nil }) {
		return filePlan{}, nil
	}
	existing, err := repos.Homework.AnswerReferences(ctx, id)
	if err != nil {
		return filePlan{}, err
	}
	var plan filePlan
	var fresh []domain.AttachmentID
	wantedBy := map[domain.AttachmentID]domain.QuestionID{}
	for index, answer := range save.Answers {
		if answer.Value.AttachmentIDs == nil {
			continue
		}
		held := heldFiles(existing, answer.Question)
		var wanted []domain.AttachmentID
		for _, text := range *answer.Value.AttachmentIDs {
			attachment, _ := domain.ParseUUID[domain.AttachmentID](text)
			wanted = append(wanted, attachment)
			if !slices.Contains(held, attachment) {
				fresh = append(fresh, attachment)
				wantedBy[attachment] = questions[index]
			}
		}
		for _, attachment := range held {
			if !slices.Contains(wanted, attachment) {
				plan.released = append(plan.released, attachment)
			}
		}
	}
	for _, reference := range existing {
		if slices.Contains(plan.released, reference.AttachmentID) {
			plan.bytes -= reference.SizeBytes
		}
	}
	bindable, err := repos.Homework.LockBindableAnswerUploads(ctx, student, id, fresh, now)
	if err != nil {
		return filePlan{}, err
	}
	for _, attachment := range fresh {
		index := slices.IndexFunc(bindable, func(upload repositories.BindableAnswerUpload) bool { return upload.ID == attachment })
		question := wantedBy[attachment]
		if index < 0 || bindable[index].ScopeQuestionID == nil || domain.QuestionID(*bindable[index].ScopeQuestionID) != question {
			return filePlan{}, ErrNotFound
		}
		plan.bound = append(plan.bound, repositories.BoundAnswerFile{Attachment: attachment, Question: question})
		plan.bytes += bindable[index].SizeBytes
	}
	return plan, nil
}

func (s *Service) applyFiles(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.SubmissionID, plan filePlan, now time.Time,
) error {
	if !plan.touches() {
		return nil
	}
	removed, err := repos.Homework.RemoveAnswerReferences(ctx, id, plan.released)
	if err != nil {
		return err
	}
	if err := library.ReleaseReferences(ctx, repos, s.deps.Random, s.deps.Logger, domain.ReferenceAnswer, removed, now); err != nil {
		return err
	}
	bound := make([]domain.AttachmentID, 0, len(plan.bound))
	for _, file := range plan.bound {
		bound = append(bound, file.Attachment)
	}
	if err := repos.Library.BindUploads(ctx, bound, now); err != nil {
		return err
	}
	if err := repos.Homework.AddAnswerReferences(ctx, id, plan.bound, now); err != nil {
		return err
	}
	charged, err := repos.Library.ChargeAll(ctx, tutor, bound, now)
	if err != nil {
		return err
	}
	if !charged {
		return ErrQuotaExceeded
	}
	if _, err := repos.Homework.RefreshFileBytes(ctx, id); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, attachment := range bound {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", slog.String(recordAttached, attachment.String()),
				slog.String(recordRefKind, string(domain.ReferenceAnswer)))
		}
	})
	return nil
}

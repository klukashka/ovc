package files

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

var errUnregisteredContext = errors.New("the upload context has no upload policy")

type dailyAggregate int

const (
	noDailyAggregate dailyAggregate = iota
	chatDailyAggregate
	answerDailyAggregate
)

type uploadPolicy struct {
	context     domain.AttachmentContext
	keyPrefix   string
	maxBytes    int64
	counted     []domain.AttachmentContext
	openLimit   int64
	daily       dailyAggregate
	answerBytes bool
	class       domain.UploadCapacityClass
	kinds       kindPolicy
}

type scopeClosure struct {
	reason domain.UploadRejection
	err    error
}

type lockedScope struct {
	tutor   domain.UserID
	closure *scopeClosure
	kinds   *kindPolicy
}

type scopeRow struct {
	kind     *domain.ScopeKind
	id       *domain.EntityID
	question *domain.EntityID
}

type uploadScope interface {
	row() scopeRow
	lock(ctx context.Context, repos repositories.Repositories, uploader accounts.Principal, now time.Time) (lockedScope, error)
	describe(opened *openUpload)
}

type uploadTarget struct {
	policy uploadPolicy
	scope  uploadScope
	hourly *hourlyUploads
}

type channelScope struct {
	channel domain.ChannelID
}

func (c channelScope) row() scopeRow {
	kind, id := domain.ScopeChannel, domain.EntityID(c.channel)
	return scopeRow{kind: &kind, id: &id}
}

func (c channelScope) lock(ctx context.Context, repos repositories.Repositories, uploader accounts.Principal, _ time.Time) (lockedScope, error) {
	channel, err := repos.Channels.LockOfMemberForShare(ctx, c.channel, uploader.UserID)
	if err != nil {
		return lockedScope{}, err
	}
	locked := lockedScope{tutor: channel.TutorID}
	if channel.ReadOnlySince != nil {
		locked.closure = &scopeClosure{reason: domain.RejectedReadOnly, err: ErrReadOnly}
	}
	return locked, nil
}

func (c channelScope) describe(opened *openUpload) {
	opened.channel = c.channel
}

type ownLibraryScope struct{}

func (ownLibraryScope) row() scopeRow {
	return scopeRow{}
}

func (ownLibraryScope) lock(_ context.Context, _ repositories.Repositories, uploader accounts.Principal, _ time.Time) (lockedScope, error) {
	if uploader.Role != domain.RoleTutor {
		return lockedScope{}, repositories.ErrNotFound
	}
	return lockedScope{tutor: uploader.UserID}, nil
}

func (ownLibraryScope) describe(*openUpload) {}

type submissionScope struct {
	context    domain.AttachmentContext
	submission domain.SubmissionID
	question   domain.QuestionID
}

func (c submissionScope) row() scopeRow {
	kind, id, question := domain.ScopeSubmission, domain.EntityID(c.submission), domain.EntityID(c.question)
	return scopeRow{kind: &kind, id: &id, question: &question}
}

func (c submissionScope) lock(
	ctx context.Context, repos repositories.Repositories, uploader accounts.Principal, now time.Time,
) (lockedScope, error) {
	asStudent := c.context == domain.ContextAnswer
	if asStudent != (uploader.Role == domain.RoleStudent) {
		return lockedScope{}, repositories.ErrNotFound
	}
	submission, err := repos.Homework.UploadSubmission(ctx, c.submission, uploader.UserID, asStudent)
	if err != nil {
		return lockedScope{}, err
	}
	relationship, err := repos.Homework.LockUploadRelationship(ctx, submission.RelationshipID, submission.TutorID)
	if err != nil {
		return lockedScope{}, err
	}
	issue, err := repos.Homework.LockUploadIssue(ctx, submission.IssueID, submission.TutorID, c.question)
	if err != nil {
		return lockedScope{}, err
	}
	status, err := repos.Homework.LockUploadSubmission(ctx, submission.ID, submission.IssueID)
	if err != nil {
		return lockedScope{}, err
	}
	if !issue.FileQuestion {
		return lockedScope{}, repositories.ErrNotFound
	}
	if !asStudent {
		if !domain.ReviewableSubmission(issue.Status, issue.MarkedForErasure, status) {
			return lockedScope{}, repositories.ErrNotFound
		}
		return lockedScope{tutor: submission.TutorID}, nil
	}
	tutor, err := repos.Users.TutorStanding(ctx, submission.TutorID)
	if err != nil {
		return lockedScope{}, err
	}
	if !domain.ReadableStudentSubmissionAt(issue.Status, issue.MarkedForErasure, relationship, tutor, now) {
		return lockedScope{}, repositories.ErrNotFound
	}
	kinds := answerKinds(issue.AllowedKinds)
	locked := lockedScope{tutor: submission.TutorID, kinds: &kinds}
	if !domain.WritableStudentSubmission(status, relationship) {
		locked.closure = &scopeClosure{reason: domain.RejectedNotEditable, err: ErrNotEditable}
	}
	return locked, nil
}

func (c submissionScope) describe(opened *openUpload) {
	opened.submission = c.submission
}

func answerKinds(allowed []domain.AnswerFileKind) kindPolicy {
	var kinds []fileKind
	for _, kind := range allowed {
		switch kind {
		case domain.AnswerFileImages:
			kinds = append(kinds, pictureKinds...)
		case domain.AnswerFilePDF:
			kinds = append(kinds, kindPDF)
		case domain.AnswerFileOffice:
			kinds = append(kinds, officeKinds...)
		case domain.AnswerFileAudio:
			kinds = append(kinds, audioKinds...)
		}
	}
	return kindPolicy{allowed: kinds}
}

var (
	libraryContexts  = []domain.AttachmentContext{domain.ContextLibrary, domain.ContextPromptImage}
	homeworkContexts = []domain.AttachmentContext{domain.ContextAnswer, domain.ContextCorrected}
)

func chatPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextChat, keyPrefix: domain.ChatKeyPrefix, maxBytes: domain.ChatMaxBytes,
		counted: []domain.AttachmentContext{domain.ContextChat}, openLimit: domain.ChatOpenUploadsPerUser, daily: chatDailyAggregate,
		class: domain.UploadCapacityChat, kinds: chatKinds,
	}
}

func libraryPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextLibrary, keyPrefix: domain.LibraryKeyPrefix, maxBytes: domain.LibraryMaxBytes,
		counted: libraryContexts, openLimit: domain.LibraryOpenUploadsPerTutor, class: domain.UploadCapacityLibrary, kinds: libraryKinds,
	}
}

func promptImagePolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextPromptImage, keyPrefix: domain.PromptImageKeyPrefix, maxBytes: domain.PromptImageMaxBytes,
		counted: libraryContexts, openLimit: domain.LibraryOpenUploadsPerTutor, class: domain.UploadCapacityLibrary, kinds: promptImageKinds,
	}
}

func answerPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextAnswer, keyPrefix: domain.AnswerKeyPrefix, maxBytes: domain.HomeworkMaxBytes, counted: homeworkContexts,
		openLimit: domain.HomeworkOpenUploadsPerUser, daily: answerDailyAggregate, answerBytes: true, class: domain.UploadCapacityHomework,
	}
}

func correctedPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextCorrected, keyPrefix: domain.CorrectedKeyPrefix, maxBytes: domain.HomeworkMaxBytes, counted: homeworkContexts,
		openLimit: domain.HomeworkOpenUploadsPerUser, class: domain.UploadCapacityHomework, kinds: correctedKinds,
	}
}

func uploadPolicyOf(uploaded domain.AttachmentContext) (uploadPolicy, bool) {
	switch uploaded {
	case domain.ContextChat:
		return chatPolicy(), true
	case domain.ContextLibrary:
		return libraryPolicy(), true
	case domain.ContextPromptImage:
		return promptImagePolicy(), true
	case domain.ContextAnswer:
		return answerPolicy(), true
	case domain.ContextCorrected:
		return correctedPolicy(), true
	case domain.ContextAvatar:
		return uploadPolicy{}, false
	}
	return uploadPolicy{}, false
}

func (s *Service) chatTarget(channel domain.ChannelID) uploadTarget {
	return uploadTarget{policy: chatPolicy(), scope: channelScope{channel: channel}, hourly: s.hourly}
}

func (s *Service) libraryTarget(uploaded domain.AttachmentContext) (uploadTarget, error) {
	switch uploaded {
	case domain.ContextLibrary:
		return uploadTarget{policy: libraryPolicy(), scope: ownLibraryScope{}, hourly: s.libraryHourly}, nil
	case domain.ContextPromptImage:
		return uploadTarget{policy: promptImagePolicy(), scope: ownLibraryScope{}, hourly: s.libraryHourly}, nil
	case domain.ContextAvatar, domain.ContextChat, domain.ContextAnswer, domain.ContextCorrected:
		return uploadTarget{}, errUnregisteredContext
	}
	return uploadTarget{}, errUnregisteredContext
}

func (s *Service) homeworkTarget(uploaded domain.AttachmentContext, submission domain.SubmissionID, question domain.QuestionID) (uploadTarget, error) {
	scope := submissionScope{context: uploaded, submission: submission, question: question}
	switch uploaded {
	case domain.ContextAnswer:
		return uploadTarget{policy: answerPolicy(), scope: scope, hourly: s.homeworkHourly}, nil
	case domain.ContextCorrected:
		return uploadTarget{policy: correctedPolicy(), scope: scope, hourly: s.homeworkHourly}, nil
	case domain.ContextAvatar, domain.ContextChat, domain.ContextLibrary, domain.ContextPromptImage:
		return uploadTarget{}, errUnregisteredContext
	}
	return uploadTarget{}, errUnregisteredContext
}

package files

import (
	"context"
	"errors"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

var errUnregisteredContext = errors.New("the upload context has no upload policy")

type uploadPolicy struct {
	context    domain.AttachmentContext
	keyPrefix  string
	maxBytes   int64
	counted    []domain.AttachmentContext
	openLimit  int64
	dailyBytes bool
	class      domain.UploadCapacityClass
	kinds      kindPolicy
}

type lockedScope struct {
	tutor    domain.UserID
	readOnly bool
}

type uploadScope interface {
	row() (*domain.ScopeKind, *domain.EntityID)
	lock(ctx context.Context, repos repositories.Repositories, uploader accounts.Principal) (lockedScope, error)
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

func (c channelScope) row() (*domain.ScopeKind, *domain.EntityID) {
	kind, id := domain.ScopeChannel, domain.EntityID(c.channel)
	return &kind, &id
}

func (c channelScope) lock(ctx context.Context, repos repositories.Repositories, uploader accounts.Principal) (lockedScope, error) {
	channel, err := repos.Channels.LockOfMemberForShare(ctx, c.channel, uploader.UserID)
	if err != nil {
		return lockedScope{}, err
	}
	return lockedScope{tutor: channel.TutorID, readOnly: channel.ReadOnlySince != nil}, nil
}

func (c channelScope) describe(opened *openUpload) {
	opened.channel = c.channel
}

type ownLibraryScope struct{}

func (ownLibraryScope) row() (*domain.ScopeKind, *domain.EntityID) {
	return nil, nil
}

func (ownLibraryScope) lock(_ context.Context, _ repositories.Repositories, uploader accounts.Principal) (lockedScope, error) {
	if uploader.Role != domain.RoleTutor {
		return lockedScope{}, repositories.ErrNotFound
	}
	return lockedScope{tutor: uploader.UserID}, nil
}

func (ownLibraryScope) describe(*openUpload) {}

var libraryContexts = []domain.AttachmentContext{domain.ContextLibrary, domain.ContextPromptImage}

func chatPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextChat, keyPrefix: domain.ChatKeyPrefix, maxBytes: domain.ChatMaxBytes,
		counted: []domain.AttachmentContext{domain.ContextChat}, openLimit: domain.ChatOpenUploadsPerUser, dailyBytes: true,
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

func uploadPolicyOf(uploaded domain.AttachmentContext) (uploadPolicy, bool) {
	switch uploaded {
	case domain.ContextChat:
		return chatPolicy(), true
	case domain.ContextLibrary:
		return libraryPolicy(), true
	case domain.ContextPromptImage:
		return promptImagePolicy(), true
	case domain.ContextAvatar, domain.ContextAnswer, domain.ContextCorrected:
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

package files

import (
	"context"
	"errors"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var errUnregisteredContext = errors.New("the upload context has no upload policy")

type uploadPolicy struct {
	context    domain.AttachmentContext
	keyPrefix  string
	maxBytes   int64
	counted    []domain.AttachmentContext
	openLimit  int64
	dailyBytes bool
}

type lockedScope struct {
	tutor    domain.UserID
	readOnly bool
}

type uploadScope interface {
	row() (domain.ScopeKind, domain.EntityID)
	lock(ctx context.Context, repos repositories.Repositories, uploader domain.UserID) (lockedScope, error)
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

func (c channelScope) row() (domain.ScopeKind, domain.EntityID) {
	return domain.ScopeChannel, domain.EntityID(c.channel)
}

func (c channelScope) lock(ctx context.Context, repos repositories.Repositories, uploader domain.UserID) (lockedScope, error) {
	channel, err := repos.Channels.LockOfMemberForShare(ctx, c.channel, uploader)
	if err != nil {
		return lockedScope{}, err
	}
	return lockedScope{tutor: channel.TutorID, readOnly: channel.ReadOnlySince != nil}, nil
}

func (c channelScope) describe(opened *openUpload) {
	opened.channel = c.channel
}

func chatPolicy() uploadPolicy {
	return uploadPolicy{
		context: domain.ContextChat, keyPrefix: domain.ChatKeyPrefix, maxBytes: domain.ChatMaxBytes,
		counted: []domain.AttachmentContext{domain.ContextChat}, openLimit: domain.ChatOpenUploadsPerUser, dailyBytes: true,
	}
}

func uploadPolicyOf(uploaded domain.AttachmentContext) (uploadPolicy, bool) {
	switch uploaded {
	case domain.ContextChat:
		return chatPolicy(), true
	case domain.ContextAvatar, domain.ContextPromptImage, domain.ContextLibrary, domain.ContextAnswer, domain.ContextCorrected:
		return uploadPolicy{}, false
	}
	return uploadPolicy{}, false
}

func (s *Service) chatTarget(channel domain.ChannelID) uploadTarget {
	return uploadTarget{policy: chatPolicy(), scope: channelScope{channel: channel}, hourly: s.hourly}
}

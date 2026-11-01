package files

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

type ChatUploadAdmission struct {
	service         *Service
	opened          openUpload
	release         func()
	once            sync.Once
	settled         bool
	createAttempted bool
}

func (s *Service) BeginChatUpload(ctx context.Context, principal accounts.Principal, channel domain.ChannelID, declared int64) (*ChatUploadAdmission, error) {
	release, err := s.gate.enter(principal.UserID)
	if err != nil {
		s.rejectedFor(ctx, principal.UserID, domain.RejectedBusy)
		return nil, err
	}
	opened, err := s.openChatUpload(ctx, ChatUpload{Principal: principal, Channel: channel, Declared: declared}, "upload")
	if err != nil {
		release()
		if errors.Is(err, repositories.ErrUploadCapacity) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "upload.capacity.rejected")
		}
		return nil, err
	}
	return &ChatUploadAdmission{service: s, opened: opened, release: release}, nil
}

func (a *ChatUploadAdmission) Close(ctx context.Context) {
	a.once.Do(func() {
		defer a.release()
		if !a.settled {
			settling, cancel := a.service.settlementContext(ctx, a.opened.id)
			defer cancel()
			if !a.createAttempted {
				if err := a.abandon(settling); err != nil {
					a.service.deps.Logger.LogAttrs(settling, slog.LevelError, "upload.discard_failed", attachmentAttribute(a.opened.id), slog.String("kind", core.ErrorKind(err)))
				}
			} else {
				a.service.discardOrReport(settling, a.opened, false)
			}
		}
	})
}

func (a *ChatUploadAdmission) Upload(ctx context.Context, filename string, body io.Reader, finish func() error) (Upload, error) {
	s := a.service
	name, valid := cleanFileName(filename)
	if !valid {
		s.rejectedFor(ctx, a.opened.uploader.UserID, domain.RejectedFilename)
		return Upload{}, ErrInvalidName
	}
	if err := a.name(ctx, name); err != nil {
		return Upload{}, err
	}
	a.createAttempted = true
	upload, err := s.deps.Objects.CreateMultipartUpload(ctx, a.opened.key, objectContentType)
	if err != nil {
		return Upload{}, err
	}
	a.opened.upload = upload
	recording, recordedCancel := s.settlementContext(ctx, a.opened.id)
	recorded, err := s.recordUploadID(recording, a.opened, a.opened.created)
	if err != nil || !recorded {
		settling := recording
		defer recordedCancel()
		if aborted := s.deps.Objects.AbortMultipartUpload(settling, a.opened.key, upload); aborted != nil {
			s.deps.Logger.LogAttrs(settling, slog.LevelError, "upload.abort_failed", attachmentAttribute(a.opened.id), slog.String("kind", core.ErrorKind(aborted)))
		}
		s.discardOrReport(settling, a.opened, false)
		a.settled = true
		if err != nil {
			return Upload{}, err
		}
		return Upload{}, ErrOutOfScope
	}
	recordedCancel()
	content, completed, err := s.streamChatUpload(ctx, a.opened, ChatUpload{Body: body, Finish: finish}, name)
	settling, cancel := s.settlementContext(ctx, a.opened.id)
	defer cancel()
	a.settled = true
	if err != nil {
		s.reportStreamFailure(ctx, a.opened, err)
		s.discardOrReport(settling, a.opened, completed)
		return Upload{}, err
	}
	if err := s.settleChatUpload(settling, a.opened, content); err != nil {
		return Upload{}, err
	}
	mediaType := content.kind.mediaType()
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.accepted", userAttribute(a.opened.uploader.UserID), attachmentAttribute(a.opened.id), channelAttribute(a.opened.channel), slog.String("context", string(domain.ContextChat)), slog.String("media_type", mediaType), slog.Int64("size_bytes", content.size))
	return Upload{ID: a.opened.id, Filename: name, MediaType: mediaType, SizeBytes: content.size}, nil
}

func (a *ChatUploadAdmission) name(ctx context.Context, name string) error {
	return a.service.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, found, err := lockUpload(ctx, repos, a.opened.id)
		if err != nil {
			return err
		}
		if !found || row.State != domain.AttachmentUploading {
			return ErrOutOfScope
		}
		intent, err := repos.UploadIntents.Lock(ctx, a.opened.id)
		if err != nil {
			return err
		}
		if intent.CleanupRequested || intent.Phase != domain.UploadOpening {
			return ErrOutOfScope
		}
		return repos.Attachments.NameUploading(ctx, a.opened.id, name, a.opened.created)
	})
}

func (a *ChatUploadAdmission) abandon(ctx context.Context) error {
	now, err := a.service.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	return a.service.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, found, err := lockUpload(ctx, repos, a.opened.id)
		if err != nil {
			return err
		}
		intent, err := repos.UploadIntents.Lock(ctx, a.opened.id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if intent.StorageKey != a.opened.key || intent.MultipartUploadID != nil || intent.CompletionStarted {
			return repositories.ErrUploadTransition
		}
		if found {
			if row.State != domain.AttachmentUploading && row.State != domain.AttachmentDeleted {
				return repositories.ErrUploadTransition
			}
			if _, err := repos.Attachments.MarkDeleted(ctx, row.ID, now); err != nil {
				return err
			}
			if _, err := repos.Attachments.DeleteRow(ctx, row.ID); err != nil {
				return err
			}
		}
		return repos.UploadIntents.Remove(ctx, a.opened.id, a.opened.key, now)
	})
}

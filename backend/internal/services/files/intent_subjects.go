package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type intentEffect struct {
	subject    domain.UploadSubjectKind
	id         domain.AttachmentID
	key        string
	upload     string
	generation *domain.UploadCleanupGeneration
	attempt    *domain.UploadAttemptID
}

func (e intentEffect) cleanupKey(generation domain.UploadCleanupGeneration) string {
	if e.subject == domain.UploadSubjectExportBuild {
		return domain.ExportIntentCleanupKey(domain.ExportBuildID(e.id), generation)
	}
	return fmt.Sprintf("storage_delete:%s:intent:%d", e.id, generation)
}

func (e intentEffect) manualKey(attempt domain.UploadAttemptID) string {
	if e.subject == domain.UploadSubjectExportBuild {
		return fmt.Sprintf("storage_delete:export:%s:manual:%s", e.id, attempt)
	}
	return fmt.Sprintf("storage_delete:%s:manual:%s", e.id, attempt)
}

func (e intentEffect) outboxPayload() (domain.OutboxKind, []byte, error) {
	kind, encoded, err := domain.KindStorageDelete, []byte(nil), error(nil)
	if e.subject == domain.UploadSubjectExportBuild {
		kind = domain.KindExportIntent
		encoded, err = json.Marshal(domain.ExportIntentPayload{Build: e.id.String(), Key: e.key, Generation: e.generation, Attempt: e.attempt})
	} else {
		encoded, err = json.Marshal(domain.AttachmentPayload{Attachment: e.id.String(), Key: e.key, Generation: e.generation, Attempt: e.attempt})
	}
	if err != nil {
		return "", nil, fmt.Errorf("encode upload cleanup: %w", err)
	}
	return kind, encoded, nil
}

func intentAttributes(subject domain.UploadSubjectKind, id domain.AttachmentID, extra ...slog.Attr) []slog.Attr {
	if subject == domain.UploadSubjectExportBuild {
		return append([]slog.Attr{slog.String("build_id", id.String()), slog.String("subject_kind", string(subject))}, extra...)
	}
	return append([]slog.Attr{attachmentAttribute(id)}, extra...)
}

func decodeStrict[Payload any](raw []byte) (Payload, error) {
	var payload Payload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, domain.PermanentEffect(fmt.Errorf("%w: %w", errMalformedPayload, err))
	}
	return payload, nil
}

func exportIntentEffect(raw []byte) (intentEffect, error) {
	payload, err := decodeStrict[domain.ExportIntentPayload](raw)
	if err != nil {
		return intentEffect{}, err
	}
	build, ok := domain.ParseUUID[domain.ExportBuildID](payload.Build)
	if !ok || payload.Key != domain.ExportStorageKey(build) {
		return intentEffect{}, domain.PermanentEffect(errMalformedPayload)
	}
	return intentEffect{subject: domain.UploadSubjectExportBuild, id: build.Identity(), key: payload.Key, generation: payload.Generation, attempt: payload.Attempt}, nil
}

type subjectLock struct {
	found     bool
	keeps     bool
	protected bool
	live      bool
	deleted   bool
}

func lockSubject(ctx context.Context, repos repositories.Repositories, subject domain.UploadSubjectKind, id domain.AttachmentID, storageKey string, liveSince time.Time) (subjectLock, error) {
	if subject == domain.UploadSubjectExportBuild {
		return lockExportSubject(ctx, repos, domain.ExportBuildID(id))
	}
	row, found, err := lockUpload(ctx, repos, id)
	if err != nil || !found {
		return subjectLock{}, err
	}
	return subjectLock{
		found:     true,
		keeps:     row.StorageKey != storageKey || row.State != domain.AttachmentDeleted,
		protected: row.State == domain.AttachmentPending || row.State == domain.AttachmentBound,
		live:      row.State == domain.AttachmentUploading && row.CreatedAt.After(liveSince),
		deleted:   row.State == domain.AttachmentDeleted,
	}, nil
}

func lockExportSubject(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID) (subjectLock, error) {
	row, err := repos.Exports.LockByBuild(ctx, build)
	if errors.Is(err, repositories.ErrNotFound) {
		return subjectLock{}, nil
	}
	if err != nil {
		return subjectLock{}, err
	}
	ready := row.State == domain.ExportReady
	return subjectLock{found: true, keeps: ready, protected: ready, live: row.State == domain.ExportBuilding}, nil
}

func (s *Service) RequestExportCleanup(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, now time.Time) error {
	return s.settleExportIntent(ctx, repos, build, false, now)
}

func (s *Service) ConfirmAndCleanExport(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, now time.Time) error {
	return s.settleExportIntent(ctx, repos, build, true, now)
}

func (s *Service) settleExportIntent(ctx context.Context, repos repositories.Repositories, build domain.ExportBuildID, completed bool, now time.Time) error {
	locked, err := repos.UploadIntents.Lock(ctx, build.Identity())
	if err != nil {
		return err
	}
	if completed {
		if err := repos.UploadIntents.ConfirmComplete(ctx, build.Identity(), locked.StorageKey, now); err != nil {
			return err
		}
	}
	if err := repos.UploadIntents.Revoke(ctx, build.Identity(), now); err != nil {
		return err
	}
	intent, err := repos.UploadIntents.Lock(ctx, build.Identity())
	if err != nil {
		return err
	}
	if intent.SubjectKind != domain.UploadSubjectExportBuild {
		return repositories.ErrUploadTransition
	}
	return s.queueIntentCleanup(ctx, repos, intent, now)
}

func (s *Service) deleteExportObject(ctx context.Context, raw []byte) (domain.OutboxDoneReason, error) {
	payload, err := decodeStrict[domain.ExportObjectPayload](raw)
	if err != nil {
		return "", err
	}
	export, exportOK := domain.ParseUUID[domain.ExportID](payload.Export)
	build, buildOK := domain.ParseUUID[domain.ExportBuildID](payload.Build)
	if !exportOK || !buildOK || payload.Key != domain.ExportStorageKey(build) || !strings.HasPrefix(payload.Key, domain.ExportKeyPrefix) {
		return "", domain.PermanentEffect(errMalformedPayload)
	}
	if err := s.deps.Objects.Delete(ctx, payload.Key); err != nil {
		return "", err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.object.deleted", slog.String("export_id", export.String()), slog.String("build_id", build.String()))
	return domain.DoneSent, nil
}

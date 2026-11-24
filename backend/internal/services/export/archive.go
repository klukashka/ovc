package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
	"ovc/backend/internal/repositories"
)

const archiveContentType = "application/zip"

var (
	errBudgetPassed    = errors.New("the export build passed its time budget")
	errUnknownLanguage = errors.New("the export's language has no catalogue")
)

type uploadState struct {
	id    storage.MultipartUploadID
	parts []storage.CompletedPart
}

type partSink struct {
	service      *Service
	ctx          context.Context
	claim        Claim
	started      time.Time
	lastProgress time.Time
	upload       *uploadState
	buffer       []byte
	next         storage.PartNumber
	done         int32
	total        int32
}

func (s *Service) writeArchive(ctx context.Context, claim Claim, started time.Time) (int64, uploadState, error) {
	created, err := s.deps.Objects.CreateMultipartUpload(ctx, claim.Object, archiveContentType)
	if err != nil {
		return 0, uploadState{}, err
	}
	upload := uploadState{id: created}
	if err := s.recordUpload(ctx, claim, created); err != nil {
		return 0, upload, err
	}
	sink := &partSink{service: s, ctx: ctx, claim: claim, started: started, lastProgress: started, upload: &upload,
		buffer: make([]byte, 0, domain.ExportPartBytes), total: 1}
	archive := exportformat.NewArchive(sink, domain.ExportTableBytesAllowance, claim.Started)
	if err := s.writeContent(ctx, archive, sink, claim); err != nil {
		return 0, upload, err
	}
	if err := archive.Close(); err != nil {
		return 0, upload, err
	}
	if err := sink.flush(); err != nil {
		return 0, upload, err
	}
	return archive.Written(), upload, nil
}

func (p *partSink) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		count := min(cap(p.buffer)-len(p.buffer), len(data))
		p.buffer = append(p.buffer, data[:count]...)
		data = data[count:]
		written += count
		if len(p.buffer) == cap(p.buffer) {
			if err := p.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (p *partSink) flush() error {
	if len(p.buffer) == 0 && len(p.upload.parts) > 0 {
		return nil
	}
	if err := p.checkpoint(true); err != nil {
		return err
	}
	p.next++
	part, err := p.service.deps.Objects.UploadPart(p.ctx, p.claim.Object, p.upload.id, p.next, p.buffer)
	if err != nil {
		return err
	}
	p.upload.parts = append(p.upload.parts, part)
	p.buffer = p.buffer[:0]
	return nil
}

func (p *partSink) progressed(done int32) error {
	p.done = done
	return p.checkpoint(false)
}

func (p *partSink) checkpoint(boundary bool) error {
	wall := p.service.deps.Wall()
	if wall.Sub(p.started) > p.service.deps.Timing.Budget {
		return errBudgetPassed
	}
	if wall.Sub(p.lastProgress) >= domain.ExportProgressInterval {
		now, err := p.service.deps.Clock.Instant(p.ctx)
		if err != nil {
			return err
		}
		recorded, err := p.service.deps.Store.Repositories().Exports.RecordProgress(p.ctx, p.claim.Export.ID, p.claim.Build, p.done, p.total, now)
		if err != nil {
			return err
		}
		if !recorded {
			return errBuildStopped
		}
		p.lastProgress = wall
		return nil
	}
	if !boundary {
		return nil
	}
	row, err := p.service.deps.Store.Repositories().Exports.ByID(p.ctx, p.claim.Export.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return errBuildStopped
	}
	if err != nil {
		return err
	}
	if !stillBuilding(row, true, p.claim.Build) {
		return errBuildStopped
	}
	return nil
}

type accountRecord struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Language  string    `json:"language"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
}

type dataCopy struct {
	FormatVersion int           `json:"format_version"`
	ExportedAt    time.Time     `json:"exported_at"`
	Role          string        `json:"role"`
	Account       accountRecord `json:"account"`
}

func (s *Service) writeContent(ctx context.Context, archive *exportformat.Archive, sink *partSink, claim Claim) error {
	text, found := core.ExportTextFor(claim.Export.Language)
	if !found {
		return errUnknownLanguage
	}
	location, err := time.LoadLocation(claim.Export.Timezone)
	if err != nil {
		return fmt.Errorf("load the export's timezone: %w", err)
	}
	user, err := s.deps.Store.Repositories().Users.ByID(ctx, claim.Export.UserID)
	if err != nil {
		return err
	}
	account := accountRecord{Name: user.Name, Email: user.Email, Role: string(user.Role), Language: string(user.Language), Timezone: user.Timezone, CreatedAt: user.CreatedAt.UTC()}
	if err := writeTables(archive, claim.Started, text, location, account, user.Role); err != nil {
		return err
	}
	if err := writeCalendar(archive, s.deps.PublicHost, claim.Started); err != nil {
		return err
	}
	if err := writeData(archive, dataCopy{FormatVersion: domain.ExportFormatVersion, ExportedAt: claim.Started.UTC(), Role: string(user.Role), Account: account}); err != nil {
		return err
	}
	return sink.progressed(1)
}

func writeTables(archive *exportformat.Archive, modified time.Time, text core.ExportText, location *time.Location, account accountRecord, role domain.Role) error {
	entry, err := archive.Create(exportformat.EntryTables)
	if err != nil {
		return err
	}
	workbook := exportformat.NewWorkbook(entry, modified, exportformat.RowsPerSheet)
	headers := append([]string{}, text.Account.Headers...)
	headers[len(headers)-1] = fmt.Sprintf(headers[len(headers)-1], location.String())
	sheet, err := workbook.StartSheet(text.Account.Title, headers)
	if err != nil {
		return err
	}
	if err := sheet.WriteRow(exportformat.Text(account.Name), exportformat.Text(account.Email), exportformat.Text(text.RoleNames[role]),
		exportformat.Text(account.Language), exportformat.Text(account.Timezone), exportformat.Date(account.CreatedAt.In(location))); err != nil {
		return err
	}
	return workbook.Close()
}

func writeCalendar(archive *exportformat.Archive, host string, stamp time.Time) error {
	entry, err := archive.Create(exportformat.EntryCalendar)
	if err != nil {
		return err
	}
	calendar, err := exportformat.NewCalendar(entry, host, stamp)
	if err != nil {
		return err
	}
	return calendar.Close()
}

func writeData(archive *exportformat.Archive, data dataCopy) error {
	entry, err := archive.Create(exportformat.EntryData)
	if err != nil {
		return err
	}
	return encodeJSON(entry, data)
}

func encodeJSON(destination io.Writer, value dataCopy) error {
	if err := json.NewEncoder(destination).Encode(value); err != nil {
		return fmt.Errorf("write the data copy: %w", err)
	}
	return nil
}

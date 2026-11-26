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

type builtArchive struct {
	size       int64
	purgeDue   *time.Time
	sheets     []sheetCount
	files      int
	unreadable int
}

func (s *Service) writeArchive(ctx context.Context, claim Claim, started time.Time) (builtArchive, uploadState, error) {
	created, err := s.deps.Objects.CreateMultipartUpload(ctx, claim.Object, archiveContentType)
	if err != nil {
		return builtArchive{}, uploadState{}, err
	}
	upload := uploadState{id: created}
	if err := s.recordUpload(ctx, claim, created); err != nil {
		return builtArchive{}, upload, err
	}
	sink := &partSink{service: s, ctx: ctx, claim: claim, started: started, lastProgress: started, upload: &upload,
		buffer: make([]byte, 0, domain.ExportPartBytes), total: 1}
	reader := pageReader{store: s.deps.Store, clock: s.deps.Clock, sink: sink, recorder: &purgeDueRecorder{}}
	prepared, err := s.prepareContent(ctx, reader, claim)
	if err != nil {
		return builtArchive{}, upload, err
	}
	var student *studentBuild
	bound := s.deps.Timing.TableAllowance
	if prepared.role == domain.RoleStudent {
		if student, err = s.newStudentBuild(reader, claim, prepared); err != nil {
			return builtArchive{}, upload, err
		}
		if err := student.takeInventory(ctx); err != nil {
			return builtArchive{}, upload, err
		}
		bound += student.fileBytes()
	}
	archive := exportformat.NewArchive(sink, bound, claim.Started)
	if student != nil {
		err = student.write(ctx, archive)
	} else {
		err = s.writeOwnAccount(archive, sink, claim, prepared)
	}
	if err != nil {
		return builtArchive{}, upload, err
	}
	if err := archive.Close(); err != nil {
		return builtArchive{}, upload, err
	}
	if err := sink.flush(); err != nil {
		return builtArchive{}, upload, err
	}
	built := builtArchive{size: archive.Written(), purgeDue: reader.recorder.earliest}
	if student != nil {
		built.sheets, built.files, built.unreadable = student.sheetCounts(), student.files, student.unreadable
	}
	return built, upload, nil
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

type preparedContent struct {
	text     core.ExportText
	location *time.Location
	account  accountRecord
	role     domain.Role
}

func (s *Service) prepareContent(ctx context.Context, reader pageReader, claim Claim) (preparedContent, error) {
	text, found := core.ExportTextFor(claim.Export.Language)
	if !found {
		return preparedContent{}, errUnknownLanguage
	}
	location, err := time.LoadLocation(claim.Export.Timezone)
	if err != nil {
		return preparedContent{}, fmt.Errorf("load the export's timezone: %w", err)
	}
	var user repositories.User
	err = reader.page(ctx, func(repos repositories.Repositories, _ time.Time) ([]domain.UserID, error) {
		var err error
		user, err = repos.Users.ByID(ctx, claim.Export.UserID)
		return nil, err
	})
	if err != nil {
		return preparedContent{}, err
	}
	account := accountRecord{Name: user.Name, Email: user.Email, Role: string(user.Role), Language: string(user.Language), Timezone: user.Timezone, CreatedAt: user.CreatedAt.UTC()}
	return preparedContent{text: text, location: location, account: account, role: user.Role}, nil
}

func (s *Service) writeOwnAccount(archive *exportformat.Archive, sink *partSink, claim Claim, prepared preparedContent) error {
	entry, err := archive.Create(exportformat.EntryTables)
	if err != nil {
		return err
	}
	workbook := exportformat.NewWorkbook(entry, claim.Started, exportformat.RowsPerSheet)
	if err := writeAccountSheet(workbook, prepared.text, prepared.location, prepared.account, prepared.role); err != nil {
		return err
	}
	if err := workbook.Close(); err != nil {
		return err
	}
	if err := writeCalendar(archive, s.deps.PublicHost, claim.Started); err != nil {
		return err
	}
	data := dataCopy{FormatVersion: domain.ExportFormatVersion, ExportedAt: claim.Started.UTC(), Role: string(prepared.role), Account: prepared.account}
	if err := writeData(archive, data); err != nil {
		return err
	}
	return sink.progressed(1)
}

func writeAccountSheet(workbook *exportformat.Workbook, text core.ExportText, location *time.Location, account accountRecord, role domain.Role) error {
	headers := append([]string{}, text.Account.Headers...)
	headers[len(headers)-1] = fmt.Sprintf(headers[len(headers)-1], location.String())
	sheet, err := workbook.StartSheet(text.Account.Title, headers)
	if err != nil {
		return err
	}
	return sheet.WriteRow(exportformat.Text(account.Name), exportformat.Text(account.Email), exportformat.Text(text.RoleNames[role]),
		exportformat.Text(account.Language), exportformat.Text(account.Timezone), exportformat.Date(account.CreatedAt.In(location)))
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

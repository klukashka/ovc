package repositories

import (
	"bytes"
	"errors"
	"slices"

	"ovc/backend/internal/domain"
)

var (
	ErrExportInvalidationsUnapplied = errors.New("the transaction named users whose exports it did not withdraw")
	ErrExportWithdrawalRepeated     = errors.New("a transaction withdraws exports at most once")
	errExportInvalidationsOutsideTx = errors.New("exports are invalidated only inside a transaction")
)

type ExportInvalidation struct {
	User   domain.UserID
	Cause  domain.ExportInvalidationCause
	Own    bool
	Purged bool
}

type exportInvalidations struct {
	entries   map[domain.UserID]ExportInvalidation
	withdrawn bool
}

func newExportInvalidations() *exportInvalidations {
	return &exportInvalidations{entries: map[domain.UserID]ExportInvalidation{}}
}

func (s *exportInvalidations) open() error {
	if s == nil {
		return errExportInvalidationsOutsideTx
	}
	return nil
}

func (s *exportInvalidations) startWithdrawal() error {
	if s == nil {
		return errExportInvalidationsOutsideTx
	}
	if s.withdrawn {
		return ErrExportWithdrawalRepeated
	}
	s.withdrawn = true
	return nil
}

func (s *exportInvalidations) name(entry ExportInvalidation) error {
	if s == nil {
		return errExportInvalidationsOutsideTx
	}
	current, found := s.entries[entry.User]
	if !found {
		s.entries[entry.User] = entry
		return nil
	}
	if entry.Cause.Precedes(current.Cause) {
		current.Cause = entry.Cause
	}
	current.Own = current.Own || entry.Own
	current.Purged = current.Purged || entry.Purged
	s.entries[entry.User] = current
	return nil
}

func (s *exportInvalidations) nameAll(cause domain.ExportInvalidationCause, users ...domain.UserID) error {
	if s == nil {
		return errExportInvalidationsOutsideTx
	}
	for _, user := range users {
		if err := s.name(ExportInvalidation{User: user, Cause: cause}); err != nil {
			return err
		}
	}
	return nil
}

func (s *exportInvalidations) nameParties(cause domain.ExportInvalidationCause, tutor domain.UserID, students ...*domain.UserID) error {
	users := []domain.UserID{tutor}
	for _, student := range students {
		if student != nil {
			users = append(users, *student)
		}
	}
	return s.nameAll(cause, users...)
}

func (s *exportInvalidations) take() []ExportInvalidation {
	if s == nil || len(s.entries) == 0 {
		return nil
	}
	taken := make([]ExportInvalidation, 0, len(s.entries))
	for _, entry := range s.entries {
		taken = append(taken, entry)
	}
	clear(s.entries)
	slices.SortFunc(taken, func(a, b ExportInvalidation) int { return bytes.Compare(a.User[:], b.User[:]) })
	return taken
}

func (s *exportInvalidations) unapplied() error {
	if s != nil && len(s.entries) > 0 {
		return ErrExportInvalidationsUnapplied
	}
	return nil
}

func (r Repositories) InvalidateExports(cause domain.ExportInvalidationCause, users ...domain.UserID) error {
	return r.invalidations.nameAll(cause, users...)
}

func (r Repositories) InvalidateOwnExport(cause domain.ExportInvalidationCause, user domain.UserID) error {
	return r.invalidations.name(ExportInvalidation{User: user, Cause: cause, Own: true})
}

func (r Repositories) PurgeExportOf(user domain.UserID) error {
	return r.invalidations.name(ExportInvalidation{User: user, Own: true, Purged: true})
}

func (r Repositories) StartExportWithdrawal() error {
	return r.invalidations.startWithdrawal()
}

func (r Repositories) TakeExportInvalidations() []ExportInvalidation {
	return r.invalidations.take()
}

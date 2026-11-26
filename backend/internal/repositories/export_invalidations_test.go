package repositories_test

import (
	"context"
	"errors"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestATransactionWithUnappliedExportInvalidationsIsRolledBack(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, student := f.tutor(t, 1), f.student(t, 2)

	txErr := f.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if _, err := repos.Users.MarkDeleted(context.Background(), student, instant); err != nil {
			return err
		}
		return repos.InvalidateExports(domain.ExportCauseRelationshipEnded, tutor, student)
	})
	outsideErr := f.repos.InvalidateExports(domain.ExportCauseRelationshipEnded, tutor)

	if !errors.Is(txErr, repositories.ErrExportInvalidationsUnapplied) {
		t.Fatalf("a transaction that named users without withdrawing their exports returned %v", txErr)
	}
	var state string
	if err := f.pool.QueryRow(context.Background(), "SELECT state FROM users WHERE id = $1", student).Scan(&state); err != nil || state != string(domain.AccountActive) {
		t.Fatalf("the refused transaction left the account %q: %v", state, err)
	}
	if outsideErr == nil {
		t.Fatal("naming users outside a transaction was accepted")
	}
}

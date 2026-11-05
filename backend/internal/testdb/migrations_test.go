package testdb

import (
	"errors"
	"testing"
)

func TestMigrationTargetsOnlyTheOwnedFixturePool(t *testing.T) {
	t.Parallel()
	owned := NewMigratedTo(t, 21)
	other := NewMigratedTo(t, 21)
	forged := &Database{Name: other.Name, URL: other.URL, Pool: other.Pool}
	owned.Name, owned.URL, owned.Pool = other.Name, other.URL, other.Pool

	forgedUp := forged.MigrateUpTo(t.Context(), 22)
	forgedDown := forged.MigrateDownTo(t.Context(), 20)
	ownedUp := owned.MigrateUpTo(t.Context(), 22)
	var originalHasMeetings, otherHasMeetings bool
	originalErr := owned.migrationPool.QueryRow(t.Context(), "SELECT to_regclass('public.meetings') IS NOT NULL").Scan(&originalHasMeetings)
	otherErr := other.Pool.QueryRow(t.Context(), "SELECT to_regclass('public.meetings') IS NOT NULL").Scan(&otherHasMeetings)

	if !errors.Is(forgedUp, ErrUnownedMigrationDatabase) || !errors.Is(forgedDown, ErrUnownedMigrationDatabase) {
		t.Fatal("an externally constructed database gained migration authority")
	}
	if ownedUp != nil || originalErr != nil || otherErr != nil || !originalHasMeetings || otherHasMeetings {
		t.Fatalf("migration escaped fixture ownership: migration=%v original=%v other=%v original_has_table=%t other_has_table=%t", ownedUp, originalErr, otherErr, originalHasMeetings, otherHasMeetings)
	}
}

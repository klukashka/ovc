package startup_test

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"

	"ovc/backend/internal/testdb"
	"ovc/backend/migrations"
)

func TestPresenceMigrationCanBeRolledBackAndReapplied(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	fixture := testdb.NewMigratedTo(t, 17)
	database, err := sql.Open("pgx", fixture.URL.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	files, err := fs.Sub(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, files)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var first bool
	if err := database.QueryRowContext(ctx, "SELECT to_regclass('user_presence') IS NOT NULL AND to_regclass('user_presence_instances') IS NOT NULL").Scan(&first); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	var removed bool
	if err := database.QueryRowContext(ctx, "SELECT to_regclass('user_presence') IS NULL AND to_regclass('user_presence_instances') IS NULL").Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var restored bool
	if err := database.QueryRowContext(ctx, "SELECT to_regclass('user_presence') IS NOT NULL AND to_regclass('user_presence_instances') IS NOT NULL AND to_regclass('user_presence_instances_expiry') IS NOT NULL").Scan(&restored); err != nil {
		t.Fatal(err)
	}

	if !first || !removed || !restored {
		t.Fatalf("presence migration lifecycle: first %t, removed %t, restored %t", first, removed, restored)
	}
}

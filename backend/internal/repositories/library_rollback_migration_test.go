package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	lessonMaterialsMigration = 36
	insertLibraryTutor       = `INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES (md5('owner')::uuid, 'ABCD2345', 1073741824, 1073741824, now(), now())`
	insertLibraryFolder = `INSERT INTO folders (id, tutor_id, name, normalised_name, created_at, updated_at)
VALUES (md5('folder')::uuid, md5('owner')::uuid, 'Notes', 'notes', now(), now())`
	insertLibraryMaterial = `INSERT INTO materials (id, tutor_id, title, normalised_title, attachment_id, created_at, updated_at)
VALUES (md5('material')::uuid, md5('owner')::uuid, 'Notes', 'notes', md5('file')::uuid, now(), now())`
	insertLessonGroup = `INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at)
VALUES (md5('group')::uuid, md5('owner')::uuid, 'Evening', 'clay', now(), now())`
	insertGroupLesson = `INSERT INTO lessons (id, tutor_id, group_id, title, starts_at, duration_minutes, status, timezone, created_at, updated_at)
VALUES (md5('lesson')::uuid, md5('owner')::uuid, md5('group')::uuid, 'Lesson', now(), 60, 'scheduled', 'UTC', now(), now())`
	insertLessonMaterial = `INSERT INTO lesson_materials (lesson_id, material_id, tutor_id, position, created_at)
VALUES (md5('lesson')::uuid, md5('material')::uuid, md5('owner')::uuid, 0, now())`
)

func libraryDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, lessonMaterialsMigration+1)
	for _, fixture := range fixtures {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestLibraryRollbacksStopBeforeDroppingLibraryRows(t *testing.T) {
	t.Parallel()
	t.Run("a rollback with a lesson material is refused before any table is dropped", func(t *testing.T) {
		t.Parallel()
		database := libraryDatabase(t, insertLibraryOwner, insertLibraryTutor, insertLibraryFile, insertLibraryMaterial, insertLessonGroup,
			insertGroupLesson, insertLessonMaterial)

		err := database.MigrateDownTo(t.Context(), capacityClassesMigration)

		if err == nil {
			t.Fatal("a rollback with a lesson material was not refused")
		}
		if !scalar(t, database, `SELECT (SELECT count(*) = 1 FROM lesson_materials) AND (SELECT count(*) = 1 FROM materials)`) {
			t.Fatal("the refused rollback dropped library rows")
		}
	})
	t.Run("a rollback with a library material is refused at the materials migration", func(t *testing.T) {
		t.Parallel()
		database := libraryDatabase(t, insertLibraryOwner, insertLibraryTutor, insertLibraryFile, insertLibraryMaterial)

		err := database.MigrateDownTo(t.Context(), capacityClassesMigration)

		if err == nil {
			t.Fatal("a rollback with a library material was not refused")
		}
		if !scalar(t, database, `SELECT count(*) = 1 FROM materials`) {
			t.Fatal("the refused rollback dropped the library material")
		}
	})
	t.Run("a rollback with a library folder is refused at the folders migration", func(t *testing.T) {
		t.Parallel()
		database := libraryDatabase(t, insertLibraryOwner, insertLibraryTutor, insertLibraryFolder)

		err := database.MigrateDownTo(t.Context(), capacityClassesMigration)

		if err == nil {
			t.Fatal("a rollback with a library folder was not refused")
		}
		if !scalar(t, database, `SELECT count(*) = 1 FROM folders`) {
			t.Fatal("the refused rollback dropped the library folder")
		}
	})
	t.Run("an empty library rolls back to the capacity classes and migrates again", func(t *testing.T) {
		t.Parallel()
		database := libraryDatabase(t)

		downErr := database.MigrateDownTo(t.Context(), capacityClassesMigration)
		dropped := downErr == nil && scalar(t, database, `SELECT to_regclass('materials') IS NULL AND to_regclass('lesson_materials') IS NULL`)
		againErr := database.MigrateUpTo(t.Context(), lessonMaterialsMigration+1)

		if downErr != nil || !dropped || againErr != nil {
			t.Fatalf("the empty library did not roll back and migrate again: %v %v", downErr, againErr)
		}
	})
}

package api_test

import (
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const unrelatedExportEmail = "unrelated-export@example.org"

func TestDeletedTutorsStudentExportExpiresAtAccountClosedUntil(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	unrelated := h.exportAccountAs(t, domain.RoleStudent, 4, unrelatedExportEmail)
	if deleted := h.deleteMe(t, world.tutor.token, world.tutor.password); deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s", deleted.status, deleted.body)
	}
	closing := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, closing.Add(-24*time.Hour))
	student := h.readyExportOf(t, h.signedInAgain(t, world.first))
	other := h.readyExportOf(t, h.signedInAgain(t, unrelated))

	h.at(t, closing.Add(-time.Minute))
	early, _ := h.mintExport(t, h.signedInAgain(t, world.first).token)
	h.at(t, closing)
	late, _ := h.mintExport(t, h.signedInAgain(t, world.first).token)
	h.tick(t)
	h.tick(t)

	if student.ExpiresAt == nil || !student.ExpiresAt.Equal(closing) {
		t.Fatalf("the student's export expires at %v, want the tutor's account_closed_until %v", student.ExpiresAt, closing)
	}
	if other.ExpiresAt == nil || other.BuiltAt == nil || !other.ExpiresAt.Equal(other.BuiltAt.Add(domain.ExportRetention)) {
		t.Fatalf("an export without a deleted tutor expires at %v", other.ExpiresAt)
	}
	if early.status != http.StatusOK || late.status != http.StatusNotFound {
		t.Fatalf("minting answered %d a minute before the closing and %d at it", early.status, late.status)
	}
	expired, _ := h.exportOf(t, world.first.id)
	if expired.State != domain.ExportExpired || expired.StorageKey != nil || h.objectExists(t, *student.StorageKey) {
		t.Fatalf("at the closing the export is %s with its object kept %v", expired.State, h.objectExists(t, *student.StorageKey))
	}
	if h.outboxKinds(t, domain.KindExportObject) != 1 {
		t.Fatalf("the expiry queued %d object deletions", h.outboxKinds(t, domain.KindExportObject))
	}
}

func TestABuildReachingPublicationAfterItsTutorsClosingIsWithdrawn(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	if deleted := h.deleteMe(t, world.tutor.token, world.tutor.password); deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s", deleted.status, deleted.body)
	}
	closing := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, closing.Add(-time.Hour))
	student := h.signedInAgain(t, world.first)
	if requested := h.requestExport(t, student.token, student.password); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
	held.holdNext(multipartComplete)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, _ := h.exportOf(t, world.first.id)

	h.at(t, closing)
	held.resume()
	awaitTick(t, building)
	h.tick(t)
	withdrawn, _ := h.exportOf(t, world.first.id)
	limits := h.exportLimitRows(t, world.first.id)
	_, intentOpen := h.intentOf(t, *row.BuildID)
	objectKept := h.objectExists(t, *row.StorageKey)
	failedNotices := h.exportNotifications(t, world.first.id, domain.NotificationExportFailed)
	readyNotices := h.exportNotifications(t, world.first.id, domain.NotificationExportReady)
	records := h.invalidationsOf(t, row.ID)
	again := h.requestExport(t, h.signedInAgain(t, world.first).token, student.password)

	if withdrawn.ID != row.ID || withdrawn.State != domain.ExportFailed || withdrawn.Failure == nil || *withdrawn.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("a build that reached publication after the closing is %s", withdrawn.State)
	}
	if limits != 0 || intentOpen || objectKept {
		t.Fatalf("the withdrawn build left %d limit rows, its intent open %v, its object kept %v", limits, intentOpen, objectKept)
	}
	if failedNotices != 1 || readyNotices != 0 {
		t.Fatalf("the owner got %d export.failed and %d export.ready notifications", failedNotices, readyNotices)
	}
	if len(records) != 1 || records[0].Cause != string(domain.ExportCauseTutorDeleted) || records[0].State != string(domain.ExportBuilding) {
		t.Fatalf("the withdrawal was recorded as %+v", records)
	}
	if again.status != http.StatusAccepted {
		t.Fatalf("a new request after the withdrawal answered %d %s", again.status, again.body)
	}
}

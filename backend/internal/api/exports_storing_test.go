package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func multipartCreate(request *http.Request) bool {
	return request.Method == http.MethodPost && request.URL.Query().Has("uploads")
}

func multipartPart(request *http.Request) bool {
	return request.Method == http.MethodPut && request.URL.Query().Has("partNumber")
}

func multipartComplete(request *http.Request) bool {
	return request.Method == http.MethodPost && request.URL.Query().Has("uploadId")
}

func (h *authHarness) awaitExportState(t *testing.T, user domain.UserID, state domain.ExportState) repositories.Export {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	system := clock.System{}
	deadline := system.Now().Add(pauseDeadline)
	for system.Now().Before(deadline) {
		if row, found := h.exportOf(t, user); found && row.State == state {
			return row
		}
		<-ticker.C
	}
	t.Fatalf("the export never became %s", state)
	return repositories.Export{}
}

func (h *authHarness) buildingExport(t *testing.T, user domain.UserID, token string) {
	t.Helper()
	if requested := h.requestExport(t, token, accountPassword); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
}

func TestExportDeletedBeforeTheCompletionMarkerLeavesNothing(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartPart)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, _ := h.exportOf(t, user)

	deleted := h.deleteExport(t, token)
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	if _, found := h.exportOf(t, user); found || h.objectExists(t, *row.StorageKey) || h.openUploads(t, *row.StorageKey) != 0 {
		t.Fatal("a deletion before the completion marker left a row, an object or an open upload")
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.exportNotifications(t, user, domain.NotificationExportReady) != 0 {
		t.Fatal("the cleanup kept the intent or a ready notification was sent")
	}
}

func TestExportDeletedWhileCompleteIsHeldIsNeverPublished(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartComplete)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, _ := h.exportOf(t, user)

	h.deleteExport(t, token)
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	if _, found := h.exportOf(t, user); found || h.exportNotifications(t, user, domain.NotificationExportReady) != 0 {
		t.Fatal("an export deleted during Complete was published or notified")
	}
	if h.objectExists(t, *row.StorageKey) || h.outboxKinds(t, domain.KindExportIntent) != 2 {
		t.Fatalf("the late acknowledgement left the object, or did not queue generation 1 (%d intent cleanups)", h.outboxKinds(t, domain.KindExportIntent))
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.exportCapacityUsed(t) != 0 {
		t.Fatal("generation 1 did not remove the intent and release its capacity")
	}
}

func TestExportCompleteHeldPastItsDeadlineLeavesTheIntentUncertain(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartComplete)
	building := h.tickInBackground(t)
	held.awaitReached(t)

	failed := h.awaitExportState(t, user, domain.ExportFailed)
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	intent, open := h.intentOf(t, *failed.BuildID)
	if *failed.Failure != domain.ExportFailureError || h.exportNotifications(t, user, domain.NotificationExportReady) != 0 {
		t.Fatalf("a Complete past its deadline left the export %s %s", failed.State, *failed.Failure)
	}
	if !open || intent.Phase != domain.UploadUncertain || h.exportCapacityUsed(t) != 1 {
		t.Fatalf("the intent is %v %s with capacity %d; want it uncertain with its capacity held", open, intent.Phase, h.exportCapacityUsed(t))
	}
	if intent.MultipartUploadID == nil || strings.Contains(h.logs.String(), *intent.MultipartUploadID) {
		t.Fatal("a record holds the multipart upload identifier, or the intent lost it")
	}
}

func TestExportBuildOverItsBudgetFailsAndLeavesNothing(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartCreate)
	building := h.tickInBackground(t)
	held.awaitReached(t)

	h.wall.Step(2*time.Hour + time.Second)
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	row, _ := h.exportOf(t, user)
	if row.State != domain.ExportFailed || *row.Failure != domain.ExportFailureTooLong || h.exportNotifications(t, user, domain.NotificationExportFailed) != 1 {
		t.Fatalf("a build over its budget is %s", row.State)
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.objectExists(t, *row.StorageKey) || h.openUploads(t, *row.StorageKey) != 0 {
		t.Fatal("a build over its budget left its intent, object or open upload")
	}
}

func TestExportBuildThatFailsLeavesNothing(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.unloadableTimezone(t, user)
	h.buildingExport(t, user, token)

	h.tick(t)
	h.tick(t)

	row, _ := h.exportOf(t, user)
	if row.State != domain.ExportFailed || *row.Failure != domain.ExportFailureError || h.exportNotifications(t, user, domain.NotificationExportFailed) != 1 {
		t.Fatalf("a failing build is %s", row.State)
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.objectExists(t, *row.StorageKey) || h.openUploads(t, *row.StorageKey) != 0 {
		t.Fatal("a failed build left its intent, object or open upload")
	}
}

func TestExportRecoveryFailsABuildWhoseWorkerStopped(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	if _, err := h.exports.Claim(context.Background(), authStart, 0); err != nil {
		t.Fatal(err)
	}
	h.at(t, authStart.Add(2*time.Hour+14*time.Minute))
	h.tick(t)
	early, _ := h.exportOf(t, user)

	h.at(t, authStart.Add(2*time.Hour+15*time.Minute))
	h.tick(t)

	recovered, _ := h.exportOf(t, user)
	if early.State != domain.ExportBuilding || recovered.State != domain.ExportFailed || *recovered.Failure != domain.ExportFailureTooLong {
		t.Fatalf("recovery left the stopped build %s before the bound and %s at it", early.State, recovered.State)
	}
	if _, open := h.intentOf(t, *recovered.BuildID); open || h.exportNotifications(t, user, domain.NotificationExportFailed) != 1 {
		t.Fatal("recovery did not clean the stopped build's intent or notify its owner")
	}
}

func TestExportBuildStoppedByShutdownRequestsCleanup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	claim, err := h.exports.Claim(context.Background(), authStart, 0)
	if err != nil {
		t.Fatal(err)
	}
	stopped, stop := context.WithCancel(context.Background())
	stop()

	buildErr := h.exports.Build(stopped, claim)
	intent, open := h.intentOf(t, claim.Build)

	row, _ := h.exportOf(t, user)
	if buildErr != nil || row.State != domain.ExportFailed || !open || !intent.CleanupRequested {
		t.Fatalf("a build stopped by shutdown returned %v and left the export %s, its intent open %v", buildErr, row.State, open)
	}
}

func TestUploadRecoveryLeavesAStreamingExportIntact(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartPart)
	building := h.tickInBackground(t)
	held.awaitReached(t)

	_, recoverErr := h.files.RecoverUploads(context.Background(), authStart.Add(48*time.Hour))
	held.resume()
	awaitTick(t, building)

	row, _ := h.exportOf(t, user)
	if recoverErr != nil || row.State != domain.ExportReady || !h.objectExists(t, *row.StorageKey) {
		t.Fatalf("upload recovery during a build left it %s: %v", row.State, recoverErr)
	}
}

func TestDeletingAReadyOrExpiredExportQueuesTheObjectWithoutAnIntent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	readyUser, readyToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, readyToken)
	ready, _ := h.exportOf(t, readyUser)

	deleted := h.deleteExport(t, readyToken)
	queued := h.outboxKinds(t, domain.KindExportObject)
	h.tick(t)

	if deleted.status != http.StatusNoContent || queued != 1 || h.exportCapacityUsed(t) != 0 || h.outboxKinds(t, domain.KindExportIntent) != 0 {
		t.Fatalf("deleting a ready export answered %d and queued %d object deletions", deleted.status, queued)
	}
	if h.objectExists(t, *ready.StorageKey) {
		t.Fatal("the ready export's object outlived its deletion")
	}
}

func TestDeletingAQueuedExportNeedsNoStorageWork(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)

	deleted := h.deleteExport(t, token)
	again := h.deleteExport(t, token)

	if deleted.status != http.StatusNoContent || again.status != http.StatusNoContent {
		t.Fatalf("deleting a queued export answered %d then %d", deleted.status, again.status)
	}
	if _, found := h.exportOf(t, user); found || h.countRows(t, "SELECT count(id) FROM outbox WHERE lane = $1", string(domain.LaneStorageDelete)) != 0 {
		t.Fatal("deleting a queued export left its row or queued storage work")
	}
}

func TestExportBuildStoppedByAWorkerShutdownRequestsCleanup(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartPart)
	running, stop := context.WithCancel(context.Background())
	defer stop()
	stopped := make(chan error, 1)
	go func() { stopped <- h.worker.Run(running) }()
	held.awaitReached(t)

	stop()
	held.resume()
	awaitTick(t, stopped)

	row, _ := h.exportOf(t, user)
	intent, open := h.intentOf(t, *row.BuildID)
	if row.State != domain.ExportFailed || *row.Failure != domain.ExportFailureError || !open || !intent.CleanupRequested {
		t.Fatalf("a build stopped by the worker's shutdown left the export %s, its intent open %v", row.State, open)
	}
}

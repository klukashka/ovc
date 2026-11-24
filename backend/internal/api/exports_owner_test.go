package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestExportOwnerDeletedDuringABuildIsNeverPublished(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	held.holdNext(multipartPart)
	building := h.tickInBackground(t)
	held.awaitReached(t)

	deleted := h.deleteMe(t, token, accountPassword)
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	row, _ := h.exportOf(t, user)
	if deleted.status != http.StatusNoContent || row.State != domain.ExportFailed || *row.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("the owner's deletion answered %d and left the export %s", deleted.status, row.State)
	}
	if h.exportNotifications(t, user, domain.NotificationExportReady)+h.exportNotifications(t, user, domain.NotificationExportFailed) != 0 {
		t.Fatal("a deleted owner received an export notification")
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.objectExists(t, *row.StorageKey) || h.openUploads(t, *row.StorageKey) != 0 {
		t.Fatal("the withdrawn build left its intent, object or open upload")
	}
}

func TestExportOwnerDeletionWithdrawsAReadyExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.readyExport(t, token)
	ready, _ := h.exportOf(t, user)

	h.deleteMe(t, token, accountPassword)
	h.tick(t)

	row, _ := h.exportOf(t, user)
	if row.State != domain.ExportFailed || row.StorageKey != nil || h.objectExists(t, *ready.StorageKey) {
		t.Fatalf("the owner's deletion left the ready export %s with its object kept %v", row.State, h.objectExists(t, *ready.StorageKey))
	}
	if h.countRows(t, "SELECT count(user_id) FROM export_limits WHERE user_id = $1::uuid", user.String()) != 0 {
		t.Fatal("the withdrawn export kept its 24-hour limit")
	}
}

func TestExportOfABlockedOwnerIsNotBuilt(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.buildingExport(t, user, token)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	h.tick(t)

	if _, found := h.exportOf(t, user); found || h.countRows(t, "SELECT count(attachment_id) FROM upload_intents WHERE subject_kind = $1", string(domain.UploadSubjectExportBuild)) != 0 {
		t.Fatal("the export of a blocked owner was kept or built")
	}
}

func TestExportBlockWithdrawsAReadyExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.readyExport(t, token)

	_, blockErr := h.operators().Block(context.Background(), studentEmail)
	h.tick(t)

	row, _ := h.exportOf(t, user)
	if blockErr != nil || row.State != domain.ExportFailed || *row.Failure != domain.ExportFailureWithdrawn || h.outboxKinds(t, domain.KindExportObject) != 1 {
		t.Fatalf("blocking left the ready export %s: %v", row.State, blockErr)
	}
	if !strings.Contains(h.logs.String(), `"cause":"account_blocked"`) {
		t.Fatal("no export.invalidated record names the block")
	}
}

func TestPurgeDeletesTheExportRowsOnEveryPath(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	deletedUser, deletedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.unloadableTimezone(t, deletedUser)
	h.buildingExport(t, deletedUser, deletedToken)
	h.tick(t)
	h.deleteMe(t, deletedToken, accountPassword)
	h.at(t, authStart.Add(31*24*time.Hour))

	h.tick(t)

	if found := h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", deletedUser.String()); found != 0 {
		t.Fatal("the purge did not delete an account that held a failed export")
	}
	if _, kept := h.exportOf(t, deletedUser); kept {
		t.Fatal("the purge kept the account's export row")
	}
}

func TestImmediatePurgeOfAnUnconfirmedAccountDeletesItsExportRows(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.unloadableTimezone(t, user)
	h.buildingExport(t, user, token)
	h.tick(t)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET state = 'unverified', email_verified_at = NULL WHERE id = $1", user); err != nil {
		t.Fatal(err)
	}

	deleted := h.deleteMe(t, token, accountPassword)

	if deleted.status != http.StatusNoContent || h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", user.String()) != 0 {
		t.Fatalf("the immediate purge answered %d and kept the account", deleted.status)
	}
}

func TestStorageDeletionRefusesMismatchedExportPayloads(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)
	row, _ := h.exportOf(t, user)
	generation := domain.UploadInitialCleanup
	attachmentNamingTheExport, err := json.Marshal(domain.AttachmentPayload{Attachment: row.BuildID.String(), Key: *row.StorageKey, Generation: &generation})
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, err := json.Marshal(domain.ExportIntentPayload{Build: row.BuildID.String(), Key: "chat/elsewhere", Generation: &generation})
	if err != nil {
		t.Fatal(err)
	}
	objectElsewhere, err := json.Marshal(domain.ExportObjectPayload{Export: row.ID.String(), Build: row.BuildID.String(), Key: "avatars/someone"})
	if err != nil {
		t.Fatal(err)
	}
	payloads := map[string]repositories.ClaimedOutboxRow{
		"attachment payload naming an export": {Kind: domain.KindStorageDelete, Payload: attachmentNamingTheExport},
		"export intent with another key":      {Kind: domain.KindExportIntent, Payload: foreignKey},
		"export object outside exports/":      {Kind: domain.KindExportObject, Payload: objectElsewhere},
		"unknown kind":                        {Kind: domain.OutboxKind("export_other"), Payload: foreignKey},
	}

	refusals := map[string]error{}
	for name, payload := range payloads {
		_, refusals[name] = h.files.DeleteObject(context.Background(), payload, authStart)
	}

	for name, refusal := range refusals {
		if !errors.Is(refusal, domain.ErrPermanentEffect) {
			t.Errorf("%s: %v; want a permanent refusal", name, refusal)
		}
	}
	if !h.objectExists(t, *row.StorageKey) {
		t.Fatal("a refused payload deleted the ready export's object")
	}
}

func TestOperatorInspectsAndRetriesAnExportBuildCleanup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	claim, err := h.exports.Claim(context.Background(), authStart, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.deleteExport(t, token)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE outbox SET status = 'dead', done_at = now(), last_error_code = 'permanent' WHERE kind = $1", string(domain.KindExportIntent)); err != nil {
		t.Fatal(err)
	}

	inspection, inspectErr := h.files.InspectUpload(context.Background(), claim.Build.Identity(), domain.UploadAttemptID{})
	cycle, retryErr := h.files.RetryUploadCleanup(context.Background(), claim.Build.Identity(), domain.UploadAttemptID{7})
	h.tick(t)
	_, remains := h.intentOf(t, claim.Build)

	if inspectErr != nil || inspection.Subject != domain.UploadSubjectExportBuild || !inspection.CleanupRequested {
		t.Fatalf("inspecting the build's intent gave %+v: %v", inspection, inspectErr)
	}
	if retryErr != nil || cycle.Status != domain.OutboxPending || h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1 AND idempotency_key LIKE '%:manual:%'", string(domain.KindExportIntent)) != 1 {
		t.Fatalf("the operator retry gave %+v: %v", cycle, retryErr)
	}
	if remains {
		t.Fatal("the operator's cleanup cycle did not remove the build's intent")
	}
}

func TestExportRecordsCarryNoSecretsOrAddress(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)
	_, minted := h.mintExport(t, token)
	row, _ := h.exportOf(t, user)
	failing, failingToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.unloadableTimezone(t, failing)
	h.buildingExport(t, failing, failingToken)

	h.tick(t)

	records := h.logs.String()
	for name, secret := range map[string]string{
		"password": accountPassword, "storage key": *row.StorageKey, "signed address": minted.URL, "signature": "X-Amz-Signature",
		"tutor email": tutorEmail, "student email": studentEmail,
	} {
		if strings.Contains(records, secret) {
			t.Errorf("a record holds the %s", name)
		}
	}
	if !strings.Contains(records, `"msg":"export.download.minted"`) || !strings.Contains(records, `"msg":"export.failed"`) {
		t.Fatal("the minting or failure records are missing")
	}
}

func TestStorageDeletionRefusesAnExportPayloadNamingAnAttachmentIntent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	attachment := domain.ExportBuildID{0xa7}
	storageKey := domain.ExportStorageKey(attachment)
	if _, err := h.database.Pool.Exec(context.Background(), `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, subject_kind, phase,
    cleanup_requested, created_at, updated_at) VALUES ($1, $2, 'chat', 'attachment', 'cleanup', true, now(), now())`, attachment.Identity(), storageKey); err != nil {
		t.Fatal(err)
	}
	generation := domain.UploadInitialCleanup
	payload, err := json.Marshal(domain.ExportIntentPayload{Build: attachment.String(), Key: storageKey, Generation: &generation})
	if err != nil {
		t.Fatal(err)
	}

	_, refusal := h.files.DeleteObject(context.Background(), repositories.ClaimedOutboxRow{Kind: domain.KindExportIntent, Payload: payload}, authStart)

	if !errors.Is(refusal, domain.ErrPermanentEffect) {
		t.Fatalf("an export cleanup naming an attachment intent gave %v; want a permanent refusal", refusal)
	}
	if _, open := h.intentOf(t, attachment); !open {
		t.Fatal("the refused cleanup removed the attachment intent")
	}
}

func TestUploadExpiryLeavesAnExportIntentAlone(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.buildingExport(t, user, token)
	claim, err := h.exports.Claim(context.Background(), authStart, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, expireErr := h.files.ExpirePending(context.Background(), authStart.Add(48*time.Hour))

	intent, open := h.intentOf(t, claim.Build)
	if expireErr != nil || !open || intent.CleanupRequested {
		t.Fatalf("upload expiry touched the export intent (open %v, cleanup %v): %v", open, intent.CleanupRequested, expireErr)
	}
}

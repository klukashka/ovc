package api_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/export"
	"ovc/backend/internal/services/tutoring"
)

const sevenDays = 7 * 24 * time.Hour

func (h *authHarness) provisionalStudent(t *testing.T, tutor exportAccount) (studentCard, exportAccount) {
	t.Helper()
	card, student := h.linkedStudent(t, tutor.token, newcomerEmail)
	token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	return card, exportAccount{id: student, token: token, email: newcomerEmail, password: newcomerPassword}
}

func (h *authHarness) reclaimingOwner(t *testing.T, tutor exportAccount) (studentCard, exportAccount) {
	t.Helper()
	card, student := h.squat(t, tutor.token, newcomerEmail, newcomerEmail)
	owner := h.reclaimed(t, newcomerEmail)
	return card, exportAccount{id: student, token: owner, email: newcomerEmail, password: ownerPassword}
}

func TestStudentDeletionWithdrawsTheExportsOfItsTutorsAndChannelMembers(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	own, coMember, outsider := h.readyExportOf(t, world.first), h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)
	tutor := h.holdBuildOf(t, held, world.tutor)

	deleted := h.deleteMe(t, world.first.token, world.first.password)
	tutor.finish(t, h)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	h.assertWithdrawn(t, tutor.row, domain.ExportCauseAccountDeleted, true)
	h.assertWithdrawn(t, coMember, domain.ExportCauseAccountDeleted, true)
	h.assertWithdrawn(t, own, domain.ExportCauseAccountDeleted, false)
	h.assertStillReady(t, outsider)
}

func TestTutorDeletionWithdrawsTheExportsOfItsStudentsAndChannelMembers(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	own, second, outsider := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)
	first := h.holdBuildOf(t, held, world.first)

	deleted := h.deleteMe(t, world.tutor.token, world.tutor.password)
	first.finish(t, h)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	h.assertWithdrawn(t, first.row, domain.ExportCauseTutorDeleted, true)
	h.assertWithdrawn(t, second, domain.ExportCauseTutorDeleted, true)
	h.assertWithdrawn(t, own, domain.ExportCauseAccountDeleted, false)
	h.assertStillReady(t, outsider)
}

func TestOperatorBlockWithdrawsTheExportsOfRelatedAccounts(t *testing.T) {
	t.Parallel()
	t.Run("a blocked student", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		world := h.exportWorld(t)
		own, tutor := h.readyExportOf(t, world.first), h.readyExportOf(t, world.tutor)
		coMember, outsider := h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)

		_, blockErr := h.operators().Block(context.Background(), world.first.email)
		h.tick(t)

		if blockErr != nil {
			t.Fatal(blockErr)
		}
		h.assertWithdrawn(t, tutor, domain.ExportCauseAccountBlocked, true)
		h.assertWithdrawn(t, coMember, domain.ExportCauseAccountBlocked, true)
		h.assertWithdrawn(t, own, domain.ExportCauseAccountBlocked, false)
		h.assertStillReady(t, outsider)
	})
	t.Run("a blocked tutor", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		world := h.exportWorld(t)
		own, first := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.first)
		second, outsider := h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)

		_, blockErr := h.operators().Block(context.Background(), world.tutor.email)
		h.tick(t)

		if blockErr != nil {
			t.Fatal(blockErr)
		}
		h.assertWithdrawn(t, first, domain.ExportCauseAccountBlocked, true)
		h.assertWithdrawn(t, second, domain.ExportCauseAccountBlocked, true)
		h.assertWithdrawn(t, own, domain.ExportCauseAccountBlocked, false)
		h.assertStillReady(t, outsider)
	})
}

func TestDeletingAnUnconfirmedAccountWithdrawsItsTutorsExport(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	_, student := h.provisionalStudent(t, tutor)
	tutorExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, outsider)

	deleted := h.deleteMe(t, student.token, student.password)
	h.tick(t)

	if deleted.status != http.StatusNoContent || h.userExists(t, newcomerEmail) {
		t.Fatalf("deleting the unconfirmed account answered %d", deleted.status)
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
	h.assertStillReady(t, outsiderExport)
}

func TestSevenDayCleanupWithdrawsItsTutorsExport(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	h.provisionalStudent(t, tutor)
	h.at(t, authStart.Add(sevenDays-time.Hour))
	tutorExport, outsiderExport := h.readyExportOf(t, h.signedInAgain(t, tutor)), h.readyExportOf(t, h.signedInAgain(t, outsider))

	h.tickAt(t, authStart.Add(sevenDays+time.Hour))
	h.tick(t)

	if h.userExists(t, newcomerEmail) {
		t.Fatal("the 7-day cleanup kept the unconfirmed account")
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
	h.assertStillReady(t, outsiderExport)
}

func TestSevenDayCleanupBatchesWithdrawTheTutorsExportBeforeTheFinalTransaction(t *testing.T) {
	t.Parallel()
	h := newFlowHarnessForLockContention(t)
	tutor := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail)
	card, student := h.provisionalStudent(t, tutor)
	h.seedMessages(t, h.directChannel(t, card.ID).ID, student.id, 2)
	tutorExport := h.readyExportOf(t, tutor)
	confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	h.unverifiedSinceBeforeTheCleanup(t, student.id)
	var lastMessage string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT id::text FROM messages WHERE sender_id = $1 ORDER BY id DESC LIMIT 1", student.id).Scan(&lastMessage); err != nil {
		t.Fatal(err)
	}
	hold := h.database.HoldRow(t, "SELECT "+lockedRowPosition+" FROM messages WHERE id = $1::uuid FOR UPDATE", lastMessage)
	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	batches := tutoring.New(tutoring.Dependencies{Store: h.store, Clock: h.clock, Random: randomPerTest(t), Logger: logger, Exports: h.effects, PurgeBatchRows: 1})
	purger := accounts.NewPurger(h.store, h.files, batches, h.effects, logger)

	cleaning := make(chan error, 1)
	go func() {
		_, err := purger.CleanupUnverified(context.Background(), authStart)
		cleaning <- err
	}()
	secondBatch := h.database.AwaitDirectWaitersOn(t, hold.Row, 1)[0]
	afterFirstBatch, _ := h.exportOf(t, tutor.id)
	confirming := h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email",
		fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), signUpHeaders())
	h.database.AwaitTransactionWaitersOf(t, secondBatch, 1)
	hold.Release(t)
	cleaned := awaitError(t, cleaning)
	confirmed := awaitAnswer(t, confirming)
	h.tick(t)

	if afterFirstBatch.State != domain.ExportFailed || afterFirstBatch.Failure == nil || *afterFirstBatch.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("after the first batch the tutor's export is %s", afterFirstBatch.State)
	}
	if cleaned != nil || confirmed.status != http.StatusOK || !h.userExists(t, newcomerEmail) {
		t.Fatalf("the cleanup returned %v and the confirmation answered %d %s", cleaned, confirmed.status, confirmed.body)
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
}

func awaitError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(pauseDeadline):
		t.Fatal("the background work did not finish")
	}
	return nil
}

func TestStudentPurgeWithdrawsNoOtherExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	if deleted := h.deleteMe(t, world.first.token, world.first.password); deleted.status != http.StatusNoContent {
		t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
	}
	h.at(t, authStart.Add(domain.DeletedAccountRetention-time.Hour))
	tutorExport, outsiderExport := h.readyExportOf(t, h.signedInAgain(t, world.tutor)), h.readyExportOf(t, h.signedInAgain(t, world.outsider))

	h.tickAt(t, authStart.Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", world.first.id.String()) != 0 {
		t.Fatal("the purge did not run")
	}
	expiredTutorExport, _ := h.exportOf(t, tutorExport.UserID)
	if expiredTutorExport.ID != tutorExport.ID || expiredTutorExport.State != domain.ExportExpired ||
		tutorExport.ExpiresAt == nil || !tutorExport.ExpiresAt.Equal(authStart.Add(domain.DeletedAccountRetention)) {
		t.Fatalf("the tutor's export, which read the deleted student, is %s with expiry %v, want it expired at the student's purge due time",
			expiredTutorExport.State, tutorExport.ExpiresAt)
	}
	if records := h.invalidationsOf(t, tutorExport.ID); len(records) != 0 || h.exportNotifications(t, tutorExport.UserID, domain.NotificationExportFailed) != 0 {
		t.Fatalf("the purge withdrew the tutor's export %+v", records)
	}
	h.assertStillReady(t, outsiderExport)
}

func TestTutorEndingWithdrawsBothExports(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	tutor, second, outsider := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)
	first := h.holdBuildOf(t, held, world.first)

	ended := h.endAsTutor(t, world.tutor.token, world.firstCard.ID)
	first.finish(t, h)

	if ended.status != http.StatusOK {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}
	h.assertWithdrawn(t, first.row, domain.ExportCauseRelationshipEnded, true)
	h.assertWithdrawn(t, tutor, domain.ExportCauseRelationshipEnded, true)
	h.assertStillReady(t, second)
	h.assertStillReady(t, outsider)
}

func TestStudentEndingWithdrawsBothExports(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	tutor, first := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.first)
	second, outsider := h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)

	stopped := h.stopStudying(t, world.first.token, world.firstCard.ID)
	h.tick(t)

	if stopped.status != http.StatusOK {
		t.Fatalf("stopping answered %d %s", stopped.status, stopped.body)
	}
	h.assertWithdrawn(t, first, domain.ExportCauseRelationshipEnded, true)
	h.assertWithdrawn(t, tutor, domain.ExportCauseRelationshipEnded, true)
	h.assertStillReady(t, second)
	h.assertStillReady(t, outsider)
}

func TestStoppingStudyingOnAProvisionalLinkWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	card, student := h.provisionalStudent(t, tutor)
	tutorExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, outsider)

	stopped := h.stopStudying(t, student.token, card.ID)
	h.tick(t)

	if stopped.status != http.StatusNoContent {
		t.Fatalf("stopping answered %d %s", stopped.status, stopped.body)
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
	h.assertStillReady(t, outsiderExport)
}

func TestTutorRemovalWithdrawsBothExports(t *testing.T) {
	t.Parallel()
	t.Run("a reclaim-pending link", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
		card, owner := h.reclaimingOwner(t, tutor)
		tutorExport, ownerExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, owner), h.readyExportOf(t, outsider)

		removed := h.removeStudent(t, tutor.token, card.ID)
		h.tick(t)

		if removed.status != http.StatusOK {
			t.Fatalf("the removal answered %d %s", removed.status, removed.body)
		}
		h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertWithdrawn(t, ownerExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertStillReady(t, outsiderExport)
	})
	t.Run("a provisional link", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
		card, _ := h.provisionalStudent(t, tutor)
		tutorExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, outsider)

		removed := h.removeStudent(t, tutor.token, card.ID)
		h.tick(t)

		if removed.status != http.StatusOK {
			t.Fatalf("the removal answered %d %s", removed.status, removed.body)
		}
		h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertStillReady(t, outsiderExport)
	})
}

func TestDroppingAReclaimLinkWithdrawsBothExports(t *testing.T) {
	t.Parallel()
	t.Run("a dropped link", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
		card, owner := h.reclaimingOwner(t, tutor)
		tutorExport, ownerExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, owner), h.readyExportOf(t, outsider)

		dropped := h.decideReclaim(t, owner.token, reclaimChoice{DropLinks: []string{card.ID}})
		h.tick(t)

		if dropped.status != http.StatusOK {
			t.Fatalf("dropping answered %d %s", dropped.status, dropped.body)
		}
		h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertWithdrawn(t, ownerExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertStillReady(t, outsiderExport)
	})
	t.Run("a kept link of a blocked tutor", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
		card, owner := h.reclaimingOwner(t, tutor)
		tutorExport, ownerExport, outsiderExport := h.readyExportOf(t, tutor), h.readyExportOf(t, owner), h.readyExportOf(t, outsider)
		if _, err := h.store.Repositories().Users.Block(context.Background(), tutor.id, authStart); err != nil {
			t.Fatal(err)
		}

		kept := h.decideReclaim(t, owner.token, reclaimChoice{KeepLinks: []string{card.ID}})
		h.tick(t)

		if kept.status != http.StatusOK {
			t.Fatalf("keeping answered %d %s", kept.status, kept.body)
		}
		h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, false)
		h.assertWithdrawn(t, ownerExport, domain.ExportCauseRelationshipDiscarded, true)
		h.assertStillReady(t, outsiderExport)
	})
}

func TestReclaimExpiryWithdrawsBothExports(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	_, owner := h.reclaimingOwner(t, tutor)
	deadline := *h.reclaimDeadline(t, owner.id)
	h.at(t, deadline.Add(-time.Hour))
	tutorExport, ownerExport := h.readyExportOf(t, h.signedInAgain(t, tutor)), h.readyExportOf(t, h.signedInAgain(t, owner))
	outsiderExport := h.readyExportOf(t, h.signedInAgain(t, outsider))

	h.tickAt(t, deadline)
	h.tick(t)

	if h.reclaimDeadline(t, owner.id) != nil {
		t.Fatal("the reclaim did not expire")
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseRelationshipDiscarded, true)
	h.assertWithdrawn(t, ownerExport, domain.ExportCauseRelationshipDiscarded, true)
	h.assertStillReady(t, outsiderExport)
}

func TestReclaimByConfirmationWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	h.squat(t, tutor.token, newcomerEmail, newcomerEmail)
	outsiderExport := h.readyExportOf(t, outsider)
	confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	building := h.holdBuildOf(t, held, tutor)

	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), "")
	building.finish(t, h)

	if confirmed.status != http.StatusOK || outcomeOf(t, confirmed) != "choice_required" {
		t.Fatalf("the confirmation answered %d %s", confirmed.status, confirmed.body)
	}
	h.assertWithdrawn(t, building.row, domain.ExportCauseReclaimPending, true)
	h.assertStillReady(t, outsiderExport)
}

func TestReclaimByPasswordResetWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	h.squat(t, tutor.token, newcomerEmail, newcomerEmail)
	resetToken := h.resetToken(t, newcomerEmail)
	tutorExport, outsiderExport := h.readyExportOf(t, h.signedInAgain(t, tutor)), h.readyExportOf(t, h.signedInAgain(t, outsider))

	reset := h.reset(t, resetToken)
	h.tick(t)

	if reset.status != http.StatusOK || outcomeOf(t, reset) != "choice_required" {
		t.Fatalf("the reset answered %d %s", reset.status, reset.body)
	}
	h.assertWithdrawn(t, tutorExport, domain.ExportCauseReclaimPending, true)
	h.assertStillReady(t, outsiderExport)
}

func TestRemovingAGroupMemberWithdrawsTheMembersAndTheTutorsExports(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	tutor, remaining, outsider := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.second), h.readyExportOf(t, world.outsider)
	first := h.holdBuildOf(t, held, world.first)

	changed := h.groupCall(t, world.tutor.token, http.MethodPatch, world.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.firstCard.ID))
	first.finish(t, h)

	if changed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", changed.status, changed.body)
	}
	h.assertWithdrawn(t, first.row, domain.ExportCauseGroupMemberRemoved, true)
	h.assertWithdrawn(t, tutor, domain.ExportCauseGroupMemberRemoved, true)
	h.assertStillReady(t, remaining)
	h.assertStillReady(t, outsider)
}

func TestCardEmailErasureWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	h.newCard(t, tutor.token, cardName, cardEmail)
	h.at(t, authStart.Add(cardRetention-time.Hour))
	tutorExport, outsiderExport := h.readyExportOf(t, h.signedInAgain(t, tutor)), h.readyExportOf(t, h.signedInAgain(t, outsider))

	h.tickAt(t, authStart.Add(cardRetention))
	h.tick(t)

	h.assertWithdrawn(t, tutorExport, domain.ExportCauseCardEmailErased, true)
	h.assertStillReady(t, outsiderExport)
}

func TestWithdrawingACardWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	tutor, outsider := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail), h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail)
	card := h.newCard(t, tutor.token, cardName, cardEmail)
	outsiderExport := h.readyExportOf(t, outsider)
	building := h.holdBuildOf(t, held, tutor)

	withdrawn := h.cardCall(t, tutor.token, http.MethodDelete, card.ID, "", "")
	repeated := h.cardCall(t, tutor.token, http.MethodDelete, card.ID, "", "")
	building.finish(t, h)

	if withdrawn.status != http.StatusNoContent || repeated.status != http.StatusNoContent {
		t.Fatalf("withdrawing the card answered %d, then %d", withdrawn.status, repeated.status)
	}
	h.assertWithdrawn(t, building.row, domain.ExportCauseCardErased, true)
	h.assertStillReady(t, outsiderExport)
}

func TestMergingACardWithdrawsTheTutorsExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	merged := h.newCard(t, world.tutor.token, "Merged card", "")
	tutor, first, outsider := h.readyExportOf(t, world.tutor), h.readyExportOf(t, world.first), h.readyExportOf(t, world.outsider)

	accepted := h.acceptSignedIn(t, world.first.token, linkToken(t, merged.Invitation))
	h.tick(t)

	if accepted.status != http.StatusOK || h.storedRetention(t, merged.ID).State != string(domain.RelationshipDiscarded) {
		t.Fatalf("accepting the second card answered %d %s", accepted.status, accepted.body)
	}
	h.assertWithdrawn(t, tutor, domain.ExportCauseCardErased, true)
	h.assertStillReady(t, first)
	h.assertStillReady(t, outsider)
}

func TestAQueuedExportSurvivesAnInvalidationAndIsBuiltAfterIt(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	h.buildingExport(t, world.first.id, world.first.token)

	ended := h.endAsTutor(t, world.tutor.token, world.firstCard.ID)
	endedAt := h.now(t)
	queued, _ := h.exportOf(t, world.first.id)
	h.advance(t, time.Minute)
	h.tick(t)

	if ended.status != http.StatusOK || queued.State != domain.ExportQueued {
		t.Fatalf("the ending answered %d and left the queued export %s", ended.status, queued.State)
	}
	built, _ := h.exportOf(t, world.first.id)
	if built.ID != queued.ID || built.State != domain.ExportReady || built.BuildStartedAt == nil || !built.BuildStartedAt.After(endedAt) {
		t.Fatalf("the queued export became %s, started at %v", built.State, built.BuildStartedAt)
	}
	if len(h.invalidationsOf(t, built.ID)) != 0 || h.exportNotifications(t, world.first.id, domain.NotificationExportFailed) != 0 {
		t.Fatal("the queued export was withdrawn")
	}
}

func TestFailedAndExpiredExportsAreLeftAloneByAnInvalidation(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	h.readyExportOf(t, world.tutor)
	h.tickAt(t, authStart.Add(domain.ExportRetention))
	h.unloadableTimezone(t, world.first.id)
	h.buildingExport(t, world.first.id, h.signedInAgain(t, world.first).token)
	h.tick(t)
	h.tick(t)
	failed, _ := h.exportOf(t, world.first.id)
	expired, _ := h.exportOf(t, world.tutor.id)
	h.advance(t, time.Minute)
	tutor := h.signedInAgain(t, world.tutor)
	endedAt := h.now(t)

	ended := h.endAsTutor(t, tutor.token, world.firstCard.ID)
	h.tick(t)

	if ended.status != http.StatusOK || failed.State != domain.ExportFailed || expired.State != domain.ExportExpired {
		t.Fatalf("the ending answered %d over a %s and an %s export", ended.status, failed.State, expired.State)
	}
	for _, before := range []struct {
		row      domain.ExportID
		user     domain.UserID
		state    domain.ExportState
		notified int64
	}{{failed.ID, world.first.id, domain.ExportFailed, 1}, {expired.ID, world.tutor.id, domain.ExportExpired, 0}} {
		after, _ := h.exportOf(t, before.user)
		if after.ID != before.row || after.State != before.state || len(h.invalidationsOf(t, before.row)) != 0 {
			t.Fatalf("the %s export became %s", before.state, after.State)
		}
		if got := h.exportNotifications(t, before.user, domain.NotificationExportFailed); got != before.notified {
			t.Fatalf("the owner of the %s export got %d export.failed notifications", before.state, got)
		}
	}
	var queuedAfter int64
	err := h.database.Pool.QueryRow(context.Background(), "SELECT count(id) FROM outbox WHERE lane = $1 AND created_at >= $2", string(domain.LaneStorageDelete), endedAt).Scan(&queuedAfter)
	if err != nil || queuedAfter != 0 {
		t.Fatalf("the ending queued %d storage deletions: %v", queuedAfter, err)
	}
}

func TestAWithdrawnExportDoesNotBlockANewRequest(t *testing.T) {
	t.Parallel()
	t.Run("a ready export", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		world := h.exportWorld(t)
		h.readyExportOf(t, world.first)

		ended := h.endAsTutor(t, world.tutor.token, world.firstCard.ID)
		limits := h.exportLimitRows(t, world.first.id)
		again := h.requestExport(t, world.first.token, world.first.password)

		if ended.status != http.StatusOK || limits != 0 || again.status != http.StatusAccepted {
			t.Fatalf("after the withdrawal %d limit rows remained and a new request answered %d %s", limits, again.status, again.body)
		}
	})
	t.Run("a building export", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		world := h.exportWorld(t)
		h.buildingExport(t, world.first.id, world.first.token)
		if _, err := h.exports.Claim(context.Background(), authStart, 0); err != nil {
			t.Fatal(err)
		}

		ended := h.endAsTutor(t, world.tutor.token, world.firstCard.ID)
		withdrawn, _ := h.exportOf(t, world.first.id)
		again := h.requestExport(t, world.first.token, world.first.password)

		if ended.status != http.StatusOK || withdrawn.State != domain.ExportFailed || again.status != http.StatusAccepted {
			t.Fatalf("the withdrawn build is %s and a new request answered %d %s", withdrawn.State, again.status, again.body)
		}
	})
}

func TestSevenDayCleanupBeforeTheCompletionMarkerLeavesNoObjectOrUpload(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	tutor := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail)
	_, student := h.provisionalStudent(t, tutor)
	building := h.holdBuildOf(t, held, tutor)
	h.unverifiedSinceBeforeTheCleanup(t, student.id)

	_, cleanupErr := h.purger.CleanupUnverified(context.Background(), authStart)
	building.finish(t, h)
	h.tick(t)

	if cleanupErr != nil || h.userExists(t, newcomerEmail) {
		t.Fatalf("the cleanup returned %v", cleanupErr)
	}
	if h.exportNotifications(t, tutor.id, domain.NotificationExportReady) != 0 {
		t.Fatal("the withdrawn build was published")
	}
	h.assertWithdrawn(t, building.row, domain.ExportCauseRelationshipDiscarded, true)
}

func TestSevenDayCleanupWhileCompleteIsHeldNeverPublishes(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	tutor := h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail)
	h.provisionalStudent(t, tutor)
	if requested := h.requestExport(t, tutor.token, tutor.password); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
	held.holdNext(multipartComplete)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, _ := h.exportOf(t, tutor.id)

	_, cleanupErr := h.purger.CleanupUnverified(context.Background(), authStart.Add(sevenDays+time.Hour))
	held.resume()
	awaitTick(t, building)
	h.tick(t)

	after, _ := h.exportOf(t, tutor.id)
	if cleanupErr != nil || after.State != domain.ExportFailed || h.exportNotifications(t, tutor.id, domain.NotificationExportReady) != 0 {
		t.Fatalf("the cleanup returned %v and left the export %s", cleanupErr, after.State)
	}
	if h.objectExists(t, *row.StorageKey) || h.outboxKinds(t, domain.KindExportIntent) != 2 {
		t.Fatalf("the late acknowledgement left the object, or did not queue generation 1 (%d intent cleanups)", h.outboxKinds(t, domain.KindExportIntent))
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.exportCapacityUsed(t) != 0 {
		t.Fatal("generation 1 did not remove the intent and release its capacity")
	}
}

func TestAnInvalidationWaitsForAClaimInProgressAndWithdrawsItsBuild(t *testing.T) {
	t.Parallel()
	h := newFlowHarnessForLockContention(t)
	world := h.exportWorld(t)
	h.buildingExport(t, world.first.id, world.first.token)
	hold := h.database.HoldRow(t, "SELECT "+lockedRowPosition+" FROM upload_intent_capacity WHERE capacity_class = $1 FOR UPDATE",
		string(domain.UploadCapacityExport))

	claiming := h.tickInBackground(t)
	claim := h.database.AwaitDirectWaitersOn(t, hold.Row, 1)[0]
	ending := h.sendInBackground(http.MethodPost, "/api/v1/students/"+world.firstCard.ID+"/end", "", jsonCall(world.tutor.token))
	h.database.AwaitTransactionWaitersOf(t, claim, 1)
	hold.Release(t)
	awaitTick(t, claiming)
	ended := awaitAnswer(t, ending)
	h.tick(t)
	h.tick(t)

	if ended.status != http.StatusOK {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}
	row, _ := h.exportOf(t, world.first.id)
	if row.State != domain.ExportFailed || row.Failure == nil || *row.Failure != domain.ExportFailureWithdrawn || row.BuildID == nil {
		t.Fatalf("the claimed export is %s", row.State)
	}
	if intent, open := h.intentOf(t, *row.BuildID); open && !intent.CleanupRequested {
		t.Fatal("the withdrawn build's intent has no cleanup requested")
	}
	if h.exportNotifications(t, world.first.id, domain.NotificationExportReady) != 0 || h.exportNotifications(t, world.first.id, domain.NotificationExportFailed) != 1 {
		t.Fatal("the withdrawn build was published, or its owner was not told once")
	}
}

func TestAClaimSkipsAnExportAnInvalidationHolds(t *testing.T) {
	t.Parallel()
	h := newFlowHarnessForLockContention(t)
	world := h.exportWorld(t)
	first := h.readyExportOf(t, world.first)
	h.buildingExport(t, world.tutor.id, world.tutor.token)
	hold := h.database.HoldRow(t, "SELECT "+lockedRowPosition+" FROM exports WHERE user_id = $1::uuid FOR UPDATE", world.first.id.String())

	ending := h.sendInBackground(http.MethodPost, "/api/v1/students/"+world.firstCard.ID+"/end", "", jsonCall(world.tutor.token))
	h.database.AwaitDirectWaitersOn(t, hold.Row, 1)
	_, skipped := h.exports.Claim(context.Background(), h.now(t), 0)
	duringTheEffect, _ := h.exportOf(t, world.tutor.id)
	hold.Release(t)
	ended := awaitAnswer(t, ending)
	endedAt := h.now(t)
	h.advance(t, time.Minute)
	h.tick(t)

	if !errors.Is(skipped, export.ErrNothingToClaim) || duringTheEffect.State != domain.ExportQueued {
		t.Fatalf("the claim during the effect returned %v and left the export %s", skipped, duringTheEffect.State)
	}
	if ended.status != http.StatusOK {
		t.Fatalf("the ending answered %d %s", ended.status, ended.body)
	}
	built, _ := h.exportOf(t, world.tutor.id)
	if built.State != domain.ExportReady || built.BuildStartedAt == nil || !built.BuildStartedAt.After(endedAt) {
		t.Fatalf("the skipped export became %s, started at %v", built.State, built.BuildStartedAt)
	}
	h.assertWithdrawn(t, first, domain.ExportCauseRelationshipEnded, true)
}

func TestARequestDuringAnInvalidationWaitsAndBuildsFromTheDataAfterIt(t *testing.T) {
	t.Parallel()
	h := newFlowHarnessForLockContention(t)
	world := h.exportWorld(t)
	tutor := h.readyExportOf(t, world.tutor)
	hold := h.database.HoldRow(t, "SELECT "+lockedRowPosition+" FROM exports WHERE user_id = $1::uuid FOR UPDATE", world.tutor.id.String())

	ending := h.sendInBackground(http.MethodPost, "/api/v1/students/"+world.firstCard.ID+"/end", "", jsonCall(world.tutor.token))
	ender := h.database.AwaitDirectWaitersOn(t, hold.Row, 1)[0]
	requesting := h.sendInBackground(http.MethodPost, exportsRoute, fmt.Sprintf(`{"password":%q}`, world.first.password), jsonCall(world.first.token))
	h.database.AwaitAdvisoryWaitersOf(t, ender, 1)
	_, insertedDuringTheEffect := h.exportOf(t, world.first.id)
	hold.Release(t)
	ended := awaitAnswer(t, ending)
	requested := awaitAnswer(t, requesting)
	endedAt := h.now(t)
	h.advance(t, time.Minute)
	h.tick(t)

	if insertedDuringTheEffect {
		t.Fatal("the student's request inserted its export while the ending was withdrawing exports")
	}
	if ended.status != http.StatusOK || requested.status != http.StatusAccepted {
		t.Fatalf("the ending answered %d %s and the request %d %s", ended.status, ended.body, requested.status, requested.body)
	}
	built, _ := h.exportOf(t, world.first.id)
	if built.State != domain.ExportReady || built.BuildStartedAt == nil || !built.BuildStartedAt.After(endedAt) {
		t.Fatalf("the requested export became %s, started at %v", built.State, built.BuildStartedAt)
	}
	h.assertWithdrawn(t, tutor, domain.ExportCauseRelationshipEnded, true)
}

func TestInvalidationRecordsCarryNoSecretsOrAddress(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	tutor := h.readyExportOf(t, world.tutor)
	_, minted := h.mintExport(t, world.tutor.token)
	first := h.readyExportOf(t, world.first)

	h.endAsTutor(t, world.tutor.token, world.firstCard.ID)
	h.tick(t)

	records := h.logs.String()
	for name, secret := range map[string]string{
		"tutor storage key": *tutor.StorageKey, "student storage key": *first.StorageKey, "signed address": minted.URL, "signature": "X-Amz-Signature",
		"tutor email": tutorEmail, "student email": firstExportEmail, "card name": "First student", "password": accountPassword,
	} {
		if strings.Contains(records, secret) {
			t.Errorf("a record holds the %s", name)
		}
	}
	for _, event := range []string{invalidatedRecord, `"msg":"export.invalidation.applied"`, `"msg":"notification.created"`} {
		if !strings.Contains(records, event) {
			t.Fatalf("no %s record", event)
		}
	}
}

func (h *authHarness) unverifiedSinceBeforeTheCleanup(t *testing.T, student domain.UserID) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET unverified_since = $1 WHERE id = $2", authStart.Add(-sevenDays-time.Hour), student); err != nil {
		t.Fatal(err)
	}
}

package api_test

import (
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const endRelationship = "UPDATE relationships SET state = 'ended', ended_at = now(), ended_by = 'tutor' WHERE id = $1::uuid"

func TestIssueRacesEnding(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.issueParties(t)
	members := h.activeCards(t, parties.tutor, 10, 2)
	group := h.newGroup(t, parties.tutorToken, "Evening club", members)
	single := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)

	releaseMember := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", members[1], endRelationship)
	toGroup := h.sendInBackground(http.MethodPost, issuesRoute, issueJSON(parties.template.ID, toGroup(group.ID)), jsonCall(parties.tutorToken))
	h.waitForLockWaiters(t, 1)
	releaseMember()
	groupAnswer := awaitAnswer(t, toGroup)
	releaseSingle := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", single, endRelationship)
	toSingle := h.sendInBackground(http.MethodPost, issuesRoute, issueJSON(parties.template.ID, toCard(single)), jsonCall(parties.tutorToken))
	h.waitForLockWaiters(t, 1)
	releaseSingle()
	singleAnswer := awaitAnswer(t, toSingle)

	h.mustAnswer(t, groupAnswer, http.StatusCreated)
	stored := h.submissionsOf(t, decodeLibrary[issueItemBody](t, groupAnswer).ID)
	if len(stored) != 1 || stored[0].student != (domain.UserID{10}).String() {
		t.Fatalf("the group issue created submissions %+v", stored)
	}
	if singleAnswer.status != http.StatusConflict || code(t, singleAnswer) != "relationship_not_active" {
		t.Fatalf("issuing to a card ended meanwhile answered %s", describe(singleAnswer))
	}
	if count := h.countRows(t, "SELECT count(id) FROM submissions WHERE relationship_id = $1::uuid", single); count != 0 {
		t.Fatalf("the ended card got %d submissions", count)
	}
}

func TestWithdrawRacesReclaim(t *testing.T) {
	t.Parallel()
	reclaim := "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid"
	t.Run("reclaim first", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.issueParties(t)
		card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
		issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
		frames := h.recordFrames(t)

		release := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", card, reclaim)
		withdrawal := h.sendInBackground(http.MethodPost, issuesRoute+"/"+issue.ID+"/withdraw", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 1)
		release()
		h.mustAnswer(t, awaitAnswer(t, withdrawal), http.StatusOK)

		sent := frames.settle(t, h)
		if got := framesTo(sent, domain.UserID{10}); len(got) != 0 {
			t.Fatalf("the reclaimed student got frames %v", got)
		}
		if rows := h.countRows(t, "SELECT count(id) FROM notifications WHERE event = 'issue.withdrawn' AND user_id = $1::uuid", (domain.UserID{10}).String()); rows != 0 {
			t.Fatalf("the reclaimed student got %d rows", rows)
		}
	})
	t.Run("withdrawal first", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.issueParties(t)
		card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
		issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
		frames := h.recordFrames(t)

		release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM issues WHERE id = $1::uuid FOR UPDATE", issue.ID)
		withdrawal := h.sendInBackground(http.MethodPost, issuesRoute+"/"+issue.ID+"/withdraw", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 1)
		reclaimed := h.execInBackground(t, reclaim, card)
		h.waitForBlockedStatements(t, 2)
		release()
		h.mustAnswer(t, awaitAnswer(t, withdrawal), http.StatusOK)
		if err := <-reclaimed; err != nil {
			t.Fatal(err)
		}

		sent := frames.settle(t, h)
		submission := h.submissionsOf(t, issue.ID)[0].id
		if !hasFrame(sent, domain.UserID{10}, domain.EntitySubmission, submission) || notificationFrames(sent, domain.UserID{10}) != 1 {
			t.Fatalf("the student withdrawn first got frames %v", framesTo(sent, domain.UserID{10}))
		}
		if h.storedRelationship(t, card).State != string(domain.RelationshipReclaimPending) {
			t.Fatal("the reclaim did not apply after the withdrawal")
		}
	})
}

func (h *authHarness) execInBackground(t *testing.T, statement string, key string) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := h.database.Pool.Exec(t.Context(), statement, key)
		done <- err
	}()
	return done
}

func (h *authHarness) waitForBlockedStatements(t *testing.T, expected int64) {
	t.Helper()
	deadline := time.NewTimer(pauseDeadline)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked int64
		if err := h.database.Pool.QueryRow(t.Context(),
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked >= expected {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("%d statements wait on locks instead of %d", blocked, expected)
		case <-poll.C:
		}
	}
}

func TestReclaimFlagsSubmissions(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.issueParties(t)
	card, _ := h.squat(t, parties.tutorToken, memberEmail(30), memberEmail(30))
	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card.ID)))
	frames := h.recordFrames(t)

	h.reclaimed(t, memberEmail(30))

	sent := frames.settle(t, h)
	submission := h.submissionsOf(t, issue.ID)[0].id
	if !hasFrame(sent, parties.tutor, domain.EntitySubmission, submission) {
		t.Fatalf("the tutor got frames %v", framesTo(sent, parties.tutor))
	}
	listed := h.issuesPage(t, parties.tutorToken, map[string][]string{"student": {card.ID}})
	detail := h.issueDetail(t, parties.tutorToken, issue.ID)
	if len(listed.Items) != 1 || listed.Items[0].Submission == nil || !listed.Items[0].Submission.IssuedBeforeReclaim ||
		len(detail.Submissions) != 1 || !detail.Submissions[0].IssuedBeforeReclaim {
		t.Fatalf("the work kept no cue: %+v %+v", listed.Items, detail.Submissions)
	}
}

func TestTutorPurgeIssues(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	upload := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	pictured := h.createTemplate(t, parties.tutorToken, "Pictured", nil, imageQuestion(blockID(43, 1), uploadedID(t, upload)))
	h.issued(t, parties.tutorToken, issueJSON(pictured.ID, toCard(card)))
	h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	tutor := parties.tutor.String()
	for name, query := range map[string]string{
		"issues":      "SELECT count(id) FROM issues WHERE tutor_id = $1::uuid",
		"submissions": "SELECT count(id) FROM submissions WHERE tutor_id = $1::uuid",
		"references":  "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'issue_snapshot' AND $1::text <> ''",
	} {
		if count := h.countRows(t, query, tutor); count != 0 {
			t.Errorf("the purged tutor keeps %d %s", count, name)
		}
	}
	if h.userExists(t, tutorEmail) || !h.userExists(t, memberEmail(10)) {
		t.Fatal("the purge did not remove exactly the tutor")
	}
}

func TestStudentPurgeSubmissions(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	h.mustAnswer(t, h.deleteMe(t, h.studentToken(t, 10), accountPassword), http.StatusNoContent)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if count := h.countRows(t, "SELECT count(id) FROM submissions WHERE student_id = $1::uuid", (domain.UserID{10}).String()); count != 0 {
		t.Fatalf("the purged student keeps %d submissions", count)
	}
	if h.userExists(t, memberEmail(10)) {
		t.Fatal("the student was not purged")
	}
	if h.countRows(t, "SELECT count(id) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NULL", issue.ID) != 1 {
		t.Fatal("the student's purge did not keep the tutor's issue")
	}
}

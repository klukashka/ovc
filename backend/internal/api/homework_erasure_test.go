package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/homework"
)

type erasureFixture struct {
	h            *authHarness
	tutor        domain.UserID
	tutorToken   string
	student      domain.UserID
	studentToken string
	card         string
	issue        string
	lesson       string
	submission   string
	file         string
	address      string
	usage        int64
}

func newErasureFixture(t *testing.T) erasureFixture {
	t.Helper()
	h := newUploadHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	token := h.studentToken(t, 10)
	issue, work := h.issueWork(t, parties.tutorToken, toCard(card), "Erasure sentinel title", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	lesson := h.lessonWith(t, parties.tutorToken, card)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, issue, `{"lesson_id":"`+lesson+`","version":1}`), http.StatusOK)
	file := h.answerFile(t, token, work[0].id, workQuestion(6), "erasure sentinel.pdf", 64)
	saved := h.saved(t, token, work[0].id, answersJSON(t, 1, saveID(1), attached(workQuestion(6), file), typedAnswer(workQuestion(5), "erasure sentinel answer")))
	answer, found := answerTo(saved.Answers, workQuestion(6))
	if !found || len(answer.Files) != 1 {
		t.Fatal("the fixture has no bound file")
	}
	h.setRelationshipState(t, card, domain.RelationshipProvisional)
	return erasureFixture{h: h, tutor: parties.tutor, tutorToken: parties.tutorToken, student: domain.UserID{10}, studentToken: token, card: card,
		issue: issue, lesson: lesson, submission: work[0].id, file: file, address: answer.Files[0].URL, usage: h.fileUsage(t, parties.tutor)}
}

func (f erasureFixture) cleanup(t *testing.T) domain.HomeworkCleanupCounts {
	t.Helper()
	result, err := (homework.Cleanup{Store: f.h.store, Random: f.h.random, Logger: f.h.logger}).Run(t.Context(), f.h.now(t))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *erasureFixture) unverify(t *testing.T) {
	t.Helper()
	if _, err := f.h.database.Pool.Exec(t.Context(), "UPDATE users SET state = 'unverified', email_verified_at = NULL, unverified_since = $2 WHERE id = $1", f.student, f.h.now(t).Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.studentToken = accessToken(t, f.h.login(t, memberEmail(10), accountPassword))
}

func TestDroppedProvisionalWorkErased(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"tutor removal", "stop studying", "reclaim drop", "keep treated as drop", "reclaim expiry", "seven day final purge", "immediate unverified DeleteAccount purge zero batches", "student deletion provisional", "student deletion reclaim", "tutor deletion provisional", "tutor deletion reclaim"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			f := newErasureFixture(t)
			h := f.h
			pending := strings.Contains(path, "reclaim") || path == "keep treated as drop"
			if pending {
				h.setRelationshipState(t, f.card, domain.RelationshipReclaimPending)
			}
			purged := strings.Contains(path, "purge")
			var otherTutor domain.UserID
			var otherSubmission, otherFile string
			var otherUsage int64
			if !purged && !strings.HasPrefix(path, "student deletion") {
				var token string
				otherTutor, token = h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 20, "other-tutor@example.org")
				card := h.linkedSignedIn(t, token, f.studentToken, "Other card", memberEmail(10))
				_, work := h.issueWork(t, token, toCard(card.ID), "Other tutor work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
				otherSubmission = work[0].id
				otherFile = h.answerFile(t, f.studentToken, otherSubmission, workQuestion(6), "other.pdf", 80)
				h.saved(t, f.studentToken, otherSubmission, answersJSON(t, 1, saveID(2), attached(workQuestion(6), otherFile)))
				otherUsage = h.fileUsage(t, otherTutor)
			}
			if path == "keep treated as drop" {
				if _, err := h.store.Repositories().Users.Block(t.Context(), f.tutor, h.now(t)); err != nil {
					t.Fatal(err)
				}
			}
			if path == "reclaim expiry" {
				if _, err := h.database.Pool.Exec(t.Context(), "UPDATE users SET reclaim_until = $2 WHERE id = $1", f.student, h.now(t).Add(-time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			var pendingFile string
			if purged {
				pendingFile = h.answerFile(t, f.studentToken, f.submission, workQuestion(6), "unbound.pdf", 32)
				f.unverify(t)
			}
			frames := h.recordFrames(t)

			switch path {
			case "tutor removal":
				h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
			case "stop studying":
				h.mustAnswer(t, h.stopStudying(t, f.studentToken, f.card), http.StatusNoContent)
			case "reclaim drop":
				h.mustAnswer(t, h.decideReclaim(t, f.studentToken, reclaimChoice{DropLinks: []string{f.card}}), http.StatusOK)
			case "keep treated as drop":
				h.mustAnswer(t, h.decideReclaim(t, f.studentToken, reclaimChoice{KeepLinks: []string{f.card}}), http.StatusOK)
			case "reclaim expiry":
				if _, err := h.tutoring.ExpireReclaims(t.Context(), h.now(t)); err != nil {
					t.Fatal(err)
				}
			case "seven day final purge":
				if _, err := h.purger.CleanupUnverified(t.Context(), h.now(t)); err != nil {
					t.Fatal(err)
				}
			case "immediate unverified DeleteAccount purge zero batches":
				h.mustAnswer(t, h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
			case "student deletion provisional", "student deletion reclaim":
				h.mustAnswer(t, h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
			case "tutor deletion provisional", "tutor deletion reclaim":
				h.mustAnswer(t, h.deleteMe(t, f.tutorToken, accountPassword), http.StatusNoContent)
			}
			marked := h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NOT NULL", f.issue)
			beforeCleanup := h.fileUsage(t, f.tutor)
			orphaned := h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid", f.submission)
			usersAfterRemoval := h.countRows(t, "SELECT count(*) FROM users WHERE id = $1::uuid", f.student.String())
			uploaderCleared := h.countRows(t, "SELECT count(*) FROM attachments WHERE id = $1::uuid AND uploader_id IS NULL AND state = 'bound'", f.file)
			referencesBeforeCleanup := h.referencesOf(t, f.file)
			chargesBeforeCleanup := h.chargesOf(t, f.file)
			queuedBeforeCleanup := h.countRows(t, "SELECT count(*) FROM outbox WHERE kind = 'storage_delete' AND payload->>'attachment' = $1", f.file) != 0
			var pendingRows, pendingDeletions int64
			if purged {
				pendingRows = h.countRows(t, "SELECT count(*) FROM attachments WHERE id = $1::uuid", pendingFile)
				pendingDeletions = h.countRows(t, "SELECT count(*) FROM outbox WHERE kind = 'storage_delete' AND payload->>'attachment' = $1", pendingFile)
			}
			sent := frames.settle(t, h)
			denied := h.fetch(t, f.address, nil)
			result := f.cleanup(t)

			if marked != 1 || beforeCleanup != f.usage || orphaned != 1 {
				t.Fatalf("marking=%d usage=%d submissions=%d", marked, beforeCleanup, orphaned)
			}
			if purged && (usersAfterRemoval != 0 || uploaderCleared != 1 || referencesBeforeCleanup != 1 || chargesBeforeCleanup != 1 || queuedBeforeCleanup) {
				t.Fatalf("final purge: users=%d uploader cleared=%d references=%d charges=%d queued=%t", usersAfterRemoval, uploaderCleared, referencesBeforeCleanup, chargesBeforeCleanup, queuedBeforeCleanup)
			}
			if purged && (pendingRows != 0 || pendingDeletions != 1) {
				t.Fatalf("final purge retained an unbound answer: rows=%d deletions=%d", pendingRows, pendingDeletions)
			}
			if !h.storageDeleteQueued(t, f.file) {
				t.Fatal("cleanup did not queue the answer file for storage deletion")
			}
			if hasFrame(sent, f.student, domain.EntitySubmission, f.submission) != (!pending && !purged) {
				t.Fatalf("unexpected final recipients: %v", sent)
			}
			if !hasFrame(sent, f.tutor, domain.EntitySubmission, f.submission) {
				t.Fatal("the tutor received no erasure hint")
			}
			if tutorFrames := homeworkFramesTo(sent, f.tutor); len(tutorFrames) != 1 || tutorFrames[0].issue != f.issue || tutorFrames[0].lessons != f.lesson {
				t.Fatalf("the tutor's erasure frames are %+v", tutorFrames)
			}
			checkStudentHomeworkFrames(t, sent, f.student)
			assertRefused(t, "a pre-mark answer address", denied, http.StatusNotFound, "not_found")
			if result.IssuesErased != 1 || result.ReferencesRemoved != 1 || h.referencesOf(t, f.file) != 0 || h.chargesOf(t, f.file) != 0 || h.fileUsage(t, f.tutor) != f.usage-64-pdfHeaderBytes {
				t.Fatalf("incorrect erasure: %+v", result)
			}
			if h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid", f.submission) != 0 || h.countRows(t, "SELECT count(*) FROM answers WHERE submission_id = $1::uuid", f.submission) != 0 {
				t.Fatal("the cleanup retained orphan homework")
			}
			if otherSubmission != "" && (h.submissionField(t, otherSubmission, "status") != "draft" || h.chargesOf(t, otherFile) != 1 || h.fileUsage(t, otherTutor) != otherUsage || hasFrame(sent, otherTutor, domain.EntitySubmission, otherSubmission)) {
				t.Fatal("removal changed the other tutor's work")
			}
		})
	}
}

func TestHomeworkCleanupRollbackAndRetry(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	independent, work := h.issueWork(t, f.tutorToken, toCard(h.linkedCard(t, f.tutor, 11, domain.RelationshipActive)), "Independent", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.markIssueForErasure(t, independent)
	installHomeworkADR0009Item8Failure(t, f)
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger}

	failed, firstErr := cleanup.Run(t.Context(), h.now(t))
	retained := h.referencesOf(t, f.file)
	usageAfterFailure := h.fileUsage(t, f.tutor)
	queuedAfterFailure := h.storageDeleteQueued(t, f.file)
	if _, err := h.database.Pool.Exec(t.Context(), "DROP TRIGGER fail_homework_cleanup ON quota_charges; DROP FUNCTION fail_homework_cleanup()"); err != nil {
		t.Fatal(err)
	}
	retried, retryErr := cleanup.Run(t.Context(), h.now(t))
	repeated, repeatErr := cleanup.Run(t.Context(), h.now(t))

	if firstErr == nil || failed.IssuesErased != 1 || failed.ReferencesRemoved != 0 || retained != 1 || usageAfterFailure != f.usage || queuedAfterFailure {
		t.Fatalf("failure did not isolate and roll back: %+v %v", failed, firstErr)
	}
	if retryErr != nil || repeatErr != nil || retried.IssuesErased != 1 || retried.ReferencesRemoved != 1 || repeated != (domain.HomeworkCleanupCounts{}) {
		t.Fatalf("retry: %+v %v; repeat: %+v %v", retried, retryErr, repeated, repeatErr)
	}
	if h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid", work[0].id) != 0 {
		t.Fatal("the independent issue did not finish")
	}
}

func installHomeworkADR0009Item8Failure(t *testing.T, f erasureFixture) {
	t.Helper()
	statement := fmt.Sprintf(`CREATE FUNCTION fail_homework_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF OLD.attachment_id = '%s'::uuid THEN RAISE EXCEPTION 'fixture failure'; END IF; RETURN OLD; END $$;
 CREATE TRIGGER fail_homework_cleanup BEFORE DELETE ON quota_charges FOR EACH ROW EXECUTE FUNCTION fail_homework_cleanup()`, f.file)
	if _, err := f.h.database.Pool.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
}

func TestHomeworkLifecycleRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h

	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	h.tick(t)

	logs := h.logs.String()
	for _, secret := range []string{"Erasure sentinel title", "erasure sentinel.pdf", "erasure sentinel answer", f.address} {
		if strings.Contains(logs, secret) {
			t.Fatalf("diagnostics contain seeded content %q", secret)
		}
	}
	if h.countLogs(`"msg":"homework.issues.marked_for_erasure"`, `"count":1`) != 1 || h.countLogs(`"msg":"job.run"`, `"job":"homework.cleanup"`, `"issues_erased":1`, `"submissions_released":0`, `"references_removed":1`) != 1 {
		t.Fatal("missing committed lifecycle counters")
	}
}

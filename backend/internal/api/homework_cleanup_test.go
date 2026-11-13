package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/homework"
)

func TestHomeworkCleanupBatchesChildrenAndResumes(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO answers (submission_id, question_id, value, created_at, updated_at)
 SELECT $1::uuid, ('00000000-0000-4000-8000-' || lpad(n::text,12,'0'))::uuid, '{}'::jsonb, $2, $2 FROM generate_series(1,500) AS n;
 `, f.submission, h.now(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO answers (submission_id, question_id, value, created_at, updated_at)
 VALUES ($1::uuid, 'ffffffff-ffff-4fff-bfff-ffffffffffff', '{}'::jsonb, $2, $2)`, f.submission, h.now(t)); err != nil {
		t.Fatal(err)
	}
	failingQuestion := "ffffffff-ffff-4fff-bfff-ffffffffffff"
	installHomeworkAnswerADR0009Item8Failure(t, h, failingQuestion)
	other := h.issued(t, f.tutorToken, issueJSON(h.readyTemplate(t, f.tutorToken, "Independent", 60).ID, toCard(h.linkedCard(t, f.tutor, 11, domain.RelationshipActive))))
	h.markIssueForErasure(t, other.ID)
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger}

	partial, firstErr := cleanup.Run(t.Context(), h.now(t))
	remaining := h.countRows(t, "SELECT count(*) FROM answers WHERE submission_id = $1::uuid", f.submission)
	marker := h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NOT NULL", f.issue)
	if _, err := h.database.Pool.Exec(t.Context(), "DROP TRIGGER fail_homework_answer ON answers; DROP FUNCTION fail_homework_answer()"); err != nil {
		t.Fatal(err)
	}
	resumed, resumedErr := cleanup.Run(t.Context(), h.now(t))

	if firstErr == nil || partial.IssuesErased != 1 || partial.ReferencesRemoved != 1 || remaining != 3 || marker != 1 {
		t.Fatalf("bounded committed progress: %+v, remaining=%d marker=%d error=%v", partial, remaining, marker, firstErr)
	}
	if resumedErr != nil || resumed.IssuesErased != 1 || resumed.ReferencesRemoved != 0 {
		t.Fatalf("resume: %+v %v", resumed, resumedErr)
	}
	if h.countRows(t, "SELECT snapshot_bytes_total FROM tutor_profiles WHERE user_id = $1::uuid", f.tutor.String()) != 0 {
		t.Fatal("snapshot bytes were not subtracted exactly once")
	}
}

func installHomeworkAnswerADR0009Item8Failure(t *testing.T, h *authHarness, question string) {
	t.Helper()
	statement := fmt.Sprintf(`CREATE FUNCTION fail_homework_answer() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF OLD.question_id = '%s'::uuid THEN RAISE EXCEPTION 'fixture failure'; END IF; RETURN OLD; END $$;
 CREATE TRIGGER fail_homework_answer BEFORE DELETE ON answers FOR EACH ROW EXECUTE FUNCTION fail_homework_answer()`, question)
	if _, err := h.database.Pool.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
}

func TestHomeworkCleanupDeletesNotificationSubjects(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	for _, recipient := range []domain.UserID{f.tutor, f.student} {
		for _, event := range []string{"issue.created", "issue.withdrawn", "submission.submitted", "submission.checked", "submission.returned", "submission.review_updated"} {
			if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
    VALUES (gen_random_uuid(), $1, $2, $3, $4, $5::uuid, $6::uuid, $7, $8)`, recipient, f.tutor, f.student, event, f.submission, f.card, "cleanup:"+recipient.String()+event, h.now(t)); err != nil {
				t.Fatal(err)
			}
		}
	}
	beforeUnrelated := h.countRows(t, "SELECT count(*) FROM notifications WHERE subject_id <> $1::uuid", f.submission)

	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	f.cleanup(t)

	if h.countRows(t, "SELECT count(*) FROM notifications WHERE subject_id = $1::uuid", f.submission) != 0 {
		t.Fatal("erased homework retained notification subjects")
	}
	if h.countRows(t, "SELECT count(*) FROM notifications WHERE subject_id <> $1::uuid", f.submission) != beforeUnrelated {
		t.Fatal("erasure removed unrelated notifications")
	}
}

func TestHomeworkCleanupPreservesSharedAttachments(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.issueParties(t)
	firstCard := h.linkedCard(t, parties.tutor, 10, domain.RelationshipProvisional)
	secondCard := h.linkedCard(t, parties.tutor, 11, domain.RelationshipActive)
	upload := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "shared.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	file := uploadedID(t, upload)
	key := h.storageKey(t, file)
	template := h.createTemplate(t, parties.tutorToken, "Shared", nil, imageQuestion(blockID(43, 1), file))
	first := h.issued(t, parties.tutorToken, issueJSON(template.ID, toCard(firstCard)))
	second := h.issued(t, parties.tutorToken, issueJSON(template.ID, toCard(secondCard)))
	usage := h.fileUsage(t, parties.tutor)
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger}

	h.mustAnswer(t, h.removeStudent(t, parties.tutorToken, firstCard), http.StatusOK)
	firstResult, firstErr := cleanup.Run(t.Context(), h.now(t))
	retainedReferences := h.referencesOf(t, file)
	retainedUsage := h.fileUsage(t, parties.tutor)
	retainedObject := h.objectExists(t, key)
	h.mustAnswer(t, h.deleteTemplate(t, parties.tutorToken, template.ID), http.StatusNoContent)
	h.markIssueForErasure(t, second.ID)
	lastResult, lastErr := cleanup.Run(t.Context(), h.now(t))
	queued := h.storageDeleteQueued(t, file)
	h.tick(t)

	if firstErr != nil || lastErr != nil || firstResult.IssuesErased != 1 || lastResult.IssuesErased != 1 {
		t.Fatalf("cleanup: %+v %v; %+v %v", firstResult, firstErr, lastResult, lastErr)
	}
	if retainedReferences != 2 || retainedUsage != usage || !retainedObject {
		t.Fatal("first erasure released a shared attachment")
	}
	if h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid", first.ID) != 0 || h.chargesOf(t, file) != 0 || !queued || h.objectExists(t, key) {
		t.Fatal("last reference did not queue and complete physical deletion")
	}
}

func TestImmediateUnverifiedPurgeDrainsEarlierDiscardedWork(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.issueParties(t)
	card, student := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	old := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card.ID)))
	h.mustAnswer(t, h.removeStudent(t, parties.tutorToken, card.ID), http.StatusOK)
	nonprovisional := h.countRows(t, `SELECT count(*) FROM submissions JOIN relationships ON relationships.id = submissions.relationship_id
 WHERE submissions.student_id = $1::uuid AND relationships.state <> 'provisional'`, student.String())

	h.mustAnswer(t, h.deleteMe(t, token, newcomerPassword), http.StatusNoContent)

	if nonprovisional != 1 {
		t.Fatal("fixture did not prove historical non-provisional work on an unverified account")
	}
	if h.countRows(t, "SELECT count(*) FROM submissions WHERE issue_id = $1::uuid", old.ID) != 0 || h.userExists(t, newcomerEmail) {
		t.Fatal("immediate purge skipped StudentBatch for earlier discarded work")
	}
	if h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NOT NULL", old.ID) != 1 {
		t.Fatal("student batching removed tutor issue history")
	}
}

func TestHomeworkReleaseStagesKeepFileProjectionsConsistent(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.setRelationshipState(t, f.card, domain.RelationshipActive)
	second := h.answerFile(t, f.studentToken, f.submission, workQuestion(6), "second.pdf", 40)
	saved := h.saved(t, f.studentToken, f.submission, answersJSON(t, h.studentWork(t, f.studentToken, f.submission).Version, saveID(3), attached(workQuestion(6), f.file, second)))
	files, found := answerTo(saved.Answers, workQuestion(6))
	if !found {
		t.Fatal("the fixture has no file answer")
	}
	failedFile, removedFile := second, f.file
	if failedFile < removedFile {
		failedFile, removedFile = removedFile, failedFile
	}
	address := ""
	for _, file := range files.Files {
		if file.AttachmentID == removedFile {
			address = file.URL
		}
	}
	failing := f
	failing.file = failedFile
	installHomeworkADR0009Item8Failure(t, failing)
	h.mustAnswer(t, h.endAsTutor(t, f.tutorToken, f.card), http.StatusOK)
	other := h.issued(t, f.tutorToken, issueJSON(h.readyTemplate(t, f.tutorToken, "Independent", 60).ID, toCard(h.linkedCard(t, f.tutor, 11, domain.RelationshipActive))))
	h.markIssueForErasure(t, other.ID)
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger, Batch: 1}

	partial, firstErr := cleanup.Run(t.Context(), h.now(t))
	student := h.studentWork(t, f.studentToken, f.submission)
	tutor := h.tutorWork(t, f.tutorToken, f.submission)
	remaining := h.answerReferences(t, f.submission)
	marker := h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid AND release_pending_since IS NOT NULL", f.submission)
	staleAddress := h.fetch(t, address, nil)
	if _, err := h.database.Pool.Exec(t.Context(), "DROP TRIGGER fail_homework_cleanup ON quota_charges; DROP FUNCTION fail_homework_cleanup()"); err != nil {
		t.Fatal(err)
	}
	resumed, resumeErr := cleanup.Run(t.Context(), h.now(t))

	if firstErr == nil || partial.ReferencesRemoved != 1 || partial.SubmissionsReleased != 0 || partial.IssuesErased != 1 || remaining != 1 || marker != 1 {
		t.Fatalf("stage progress: %+v remaining=%d marker=%d error=%v", partial, remaining, marker, firstErr)
	}
	studentAnswer, hasStudentAnswer := answerTo(student.Answers, workQuestion(6))
	if !hasStudentAnswer || fileIDs(studentAnswer.Files) != failedFile {
		t.Fatal("student projection lists a released file between stages")
	}
	hasTutorAnswer := false
	for _, answer := range tutor.Answers {
		if answer.QuestionID == workQuestion(6) {
			hasTutorAnswer = true
			if fileIDs(answer.Files) != failedFile {
				t.Fatal("tutor projection lists a released file between stages")
			}
		}
	}
	if !hasTutorAnswer {
		t.Fatal("tutor projection omits the remaining file answer")
	}
	assertRefused(t, "released address between stages", staleAddress, http.StatusNotFound, "not_found")
	if resumeErr != nil || resumed.ReferencesRemoved != 1 || resumed.SubmissionsReleased != 1 {
		t.Fatalf("release resume: %+v %v", resumed, resumeErr)
	}
}

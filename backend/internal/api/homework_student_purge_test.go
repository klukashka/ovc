package api_test

import (
	"errors"
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
)

func TestStudentPurgeDrainsSeveralSubmissionsPerBatch(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	for index := range 4 {
		_, work := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Purge work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
		file := h.answerFile(t, parties.studentToken, work[0].id, workQuestion(6), "answer.pdf", 64)
		saved := h.saved(t, parties.studentToken, work[0].id, answersJSON(t, 1, saveID(byte(index+1)), attached(workQuestion(6), file)))
		h.submitted(t, parties.studentToken, work[0].id, saved.Version)
		h.reviewSaved(t, parties.tutorToken, work[0].id, reviewJSON(t, 1, 0, saveID(byte(index+10)), "Feedback", fullyScored("Checked")...))
	}
	h.setRelationshipState(t, parties.card.ID, domain.RelationshipEnded)
	now := h.now(t)
	purge := homework.Purge{Random: h.random, Logger: h.logger}
	var progressed int64

	err := h.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
		if _, err := repos.Users.LockForPurge(t.Context(), parties.student); err != nil {
			return err
		}
		var err error
		progressed, err = purge.StudentBatch(t.Context(), repos, parties.student, domain.HomeworkCleanupBatch, now)
		return err
	})

	if err != nil || progressed == 0 {
		t.Fatalf("student batch: progress=%d error=%v", progressed, err)
	}
	if remaining := h.countRows(t, "SELECT count(*) FROM submissions WHERE student_id = $1::uuid", parties.student.String()); remaining != 0 {
		t.Fatalf("one batch left %d of five submissions", remaining)
	}
	if remaining := h.countRows(t, "SELECT count(*) FROM issues WHERE tutor_id = $1::uuid", parties.tutor.String()); remaining != 5 {
		t.Fatalf("purge changed tutor issue history: %d", remaining)
	}
}

func TestPendingAnswerRequiresUploader(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	file := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "pending.pdf", 64)

	_, err := h.database.Pool.Exec(t.Context(), "UPDATE attachments SET uploader_id = NULL WHERE id = $1::uuid AND state = 'pending'", file)

	var violation interface{ SQLState() string }
	if !errors.As(err, &violation) || violation.SQLState() != "23514" {
		t.Fatalf("pending answer without uploader: want check violation, got %v", err)
	}
}

func TestAnswerBindingWithAnotherUploaderIsRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	file := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "another-uploader.pdf", 64)
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE attachments SET uploader_id = $2::uuid WHERE id = $1::uuid", file, parties.tutor.String()); err != nil {
		t.Fatal(err)
	}

	answer := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), file)))

	assertRefused(t, "binding with another uploader", answer, http.StatusNotFound, "not_found")
	if h.answerReferences(t, parties.submission) != 0 {
		t.Fatal("another uploader's attachment acquired an answer reference")
	}
}

func TestStudentPurgeSharesAnswerBudgetAndResumes(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.workParties(t, workBlocks()...)
	_, work := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Second", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	for _, submission := range []string{parties.submission, work[0].id} {
		if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO answers (submission_id, question_id, value, created_at, updated_at)
 SELECT $1::uuid, gen_random_uuid(), '{}'::jsonb, $2, $2 FROM generate_series(1,300)`, submission, h.now(t)); err != nil {
			t.Fatal(err)
		}
	}
	h.setRelationshipState(t, parties.card.ID, domain.RelationshipEnded)
	now := h.now(t)
	purge := homework.Purge{Random: h.random, Logger: h.logger}
	batch := func() error {
		return h.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
			if _, err := repos.Users.LockForPurge(t.Context(), parties.student); err != nil {
				return err
			}
			_, err := purge.StudentBatch(t.Context(), repos, parties.student, domain.HomeworkCleanupBatch, now)
			return err
		})
	}

	firstErr := batch()
	remainingAnswers := h.countRows(t, "SELECT count(*) FROM answers JOIN submissions ON submissions.id = answers.submission_id WHERE student_id = $1::uuid", parties.student.String())
	remainingSubmissions := h.countRows(t, "SELECT count(*) FROM submissions WHERE student_id = $1::uuid", parties.student.String())
	secondErr := batch()

	if firstErr != nil || secondErr != nil || remainingAnswers != 100 || remainingSubmissions != 1 {
		t.Fatalf("shared answer budget: answers=%d submissions=%d errors=%v/%v", remainingAnswers, remainingSubmissions, firstErr, secondErr)
	}
	if h.countRows(t, "SELECT count(*) FROM submissions WHERE student_id = $1::uuid", parties.student.String()) != 0 {
		t.Fatal("student purge did not resume")
	}
}

func TestUnverifiedDeletionBatchBudgetIsRetryable(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.setRelationshipState(t, f.card, domain.RelationshipDiscarded)
	f.unverify(t)
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
 SELECT gen_random_uuid(), $1, $2, $1, 'issue.created', $3::uuid, $4::uuid, 'purge-budget:' || n, $5 FROM generate_series(1,10001) AS n`, f.student, f.tutor, f.submission, f.card, h.now(t)); err != nil {
		t.Fatal(err)
	}

	first := h.deleteMe(t, f.studentToken, accountPassword)
	remaining := h.countRows(t, "SELECT count(*) FROM notifications WHERE subject_id = $1::uuid", f.submission)
	stillPresent := h.userExists(t, memberEmail(10))
	second := h.deleteMe(t, f.studentToken, accountPassword)

	assertRefused(t, "deletion batch budget", first, http.StatusServiceUnavailable, "service_unavailable")
	if first.headers.Get("Retry-After") != "1" || !stillPresent || remaining == 0 || remaining > 10 {
		t.Fatalf("retry lost progress or account: rows=%d present=%t retry=%q", remaining, stillPresent, first.headers.Get("Retry-After"))
	}
	h.mustAnswer(t, second, http.StatusNoContent)
	if h.userExists(t, memberEmail(10)) {
		t.Fatal("retry did not finish deletion")
	}
}

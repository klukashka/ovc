package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
)

type expiryCase struct {
	name    string
	context domain.AttachmentContext
	change  func(t *testing.T, h *authHarness, parties homeworkUploadParties)
	expires bool
}

func (h *authHarness) pendingHomeworkUpload(t *testing.T, parties homeworkUploadParties, uploaded domain.AttachmentContext) string {
	t.Helper()
	token := parties.studentToken
	if uploaded == domain.ContextCorrected {
		h.submitStored(t, parties.submission, authStart)
		token = parties.tutorToken
	}
	answer := h.homeworkUpload(t, token, homeworkRoute(uploaded, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))
	h.mustAnswer(t, answer, http.StatusCreated)
	return uploadedID(t, answer)
}

func TestUnbindableHomeworkUploadsExpire(t *testing.T) {
	t.Parallel()
	cases := []expiryCase{
		{"a draft answer stays", domain.ContextAnswer, func(*testing.T, *authHarness, homeworkUploadParties) {}, false},
		{"an answer of submitted work expires", domain.ContextAnswer, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.submitStored(t, parties.submission, authStart)
		}, true},
		{"an answer on an ended link expires", domain.ContextAnswer, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.setRelationshipState(t, parties.card.ID, domain.RelationshipEnded)
		}, true},
		{"an answer on a reclaim-pending link expires", domain.ContextAnswer, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.setRelationshipState(t, parties.card.ID, domain.RelationshipReclaimPending)
		}, true},
		{"a corrected file of submitted work stays", domain.ContextCorrected, func(*testing.T, *authHarness, homeworkUploadParties) {}, false},
		{"a corrected file of checked work stays", domain.ContextCorrected, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.exec(t, "UPDATE submissions SET status = 'checked' WHERE id = $1::uuid", parties.submission)
		}, false},
		{"a corrected file of closed work expires", domain.ContextCorrected, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.exec(t, "UPDATE submissions SET status = 'closed' WHERE id = $1::uuid", parties.submission)
		}, true},
		{"a corrected file of an issue marked for erasure expires", domain.ContextCorrected, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.markIssueForErasure(t, parties.issue)
		}, true},
		{"a corrected file of a blocked tutor expires", domain.ContextCorrected, func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.exec(t, "UPDATE users SET state = 'blocked', state_before_block = 'active' WHERE id = $1::uuid", parties.tutor.String())
		}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.homeworkUploadParties(t)
			upload := h.pendingHomeworkUpload(t, parties, testCase.context)
			testCase.change(t, h, parties)

			expired, err := h.files.ExpirePending(context.Background(), authStart)

			pending := h.uploadState(t, upload) == string(domain.AttachmentPending)
			if err != nil || pending == testCase.expires || (expired == 1) != testCase.expires {
				t.Fatalf("the expiry removed %d uploads and left the upload pending %t: %v", expired, pending, err)
			}
		})
	}
}

func TestDailyAnswerBytesLeaveWithTheirDay(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	yesterday := authStart.Add(-24 * time.Hour)
	h.addDailyAnswerBytes(t, parties.student, parties.tutor, yesterday, 40)
	h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart, 60)

	deleted, err := h.files.CleanupDailyUploads(context.Background(), authStart)

	if err != nil || deleted != 1 || h.dailyAnswerBytes(t, parties.student, parties.tutor, yesterday) != 0 ||
		h.dailyAnswerBytes(t, parties.student, parties.tutor, authStart) != 60 {
		t.Fatalf("the daily cleanup removed %d answer rows: %v", deleted, err)
	}
}

func TestDailyAnswerBytesStayWhileTheirDayHasAnUploadingAnswer(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	yesterday := authStart.Add(-24 * time.Hour)
	h.addDailyAnswerBytes(t, parties.student, parties.tutor, yesterday, 40)
	h.exec(t, `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, quota_tutor_id, storage_key, filename, state, reserved_bytes,
    created_at, updated_at)
SELECT '`+fixtureID(64, 1)+`'::uuid, student_id, 'answer', 'submission', id, tutor_id, 'answers/uploading', 'page.pdf', 'uploading', 10,
    '`+yesterday.Format(time.RFC3339)+`'::timestamptz, now()
FROM submissions WHERE id = $1::uuid`, parties.submission)

	deleted, err := h.files.CleanupDailyUploads(context.Background(), authStart)

	if err != nil || deleted != 0 || h.dailyAnswerBytes(t, parties.student, parties.tutor, yesterday) != 40 {
		t.Fatalf("the daily cleanup removed %d answer rows of a day with an uploading answer: %v", deleted, err)
	}
}

func TestTheTutorPurgeRemovesTheDailyAnswerBytes(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart, 60)
	h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart.Add(-24*time.Hour), 40)
	steps := homework.Purge{Random: randomPerTest(t), Logger: h.logger}.TutorSteps()

	err := repositories.NewStore(h.database.Pool).WithTx(context.Background(), func(repos repositories.Repositories) error {
		for _, step := range steps {
			for {
				deleted, err := step(context.Background(), repos, parties.tutor, 1, authStart)
				if err != nil {
					return err
				}
				if deleted == 0 {
					break
				}
			}
		}
		return nil
	})

	remaining := h.countRows(t, "SELECT count(student_id) FROM daily_answer_upload_bytes WHERE tutor_id = $1::uuid", parties.tutor.String())
	if err != nil || remaining != 0 {
		t.Fatalf("the tutor purge left %d daily answer rows: %v", remaining, err)
	}
}

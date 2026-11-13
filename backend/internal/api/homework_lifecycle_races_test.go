package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

func TestHomeworkRemovalRacesWrites(t *testing.T) {
	t.Parallel()
	for _, removalFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("removal_first_%t", removalFirst), func(t *testing.T) {
			t.Parallel()
			f := newErasureFixture(t)
			h := f.h
			version := h.studentWork(t, f.studentToken, f.submission).Version
			table := "submissions"
			id := f.submission
			if removalFirst {
				table, id = "issues", f.issue
			}
			release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM "+table+" WHERE id = $1::uuid FOR UPDATE", id)
			saveBody := answersJSON(t, version, saveID(30), typedAnswer(workQuestion(5), "Racing answer"))
			var removal, saving <-chan pendingAnswer

			if removalFirst {
				removal = h.sendInBackground(http.MethodPost, studentsPath+"/"+f.card+"/remove", "", jsonCall(f.tutorToken))
				h.waitForLockWaiters(t, 1)
				saving = h.sendInBackground(http.MethodPut, submissionsRoute+f.submission+"/answers", saveBody, jsonCall(f.studentToken))
			} else {
				saving = h.sendInBackground(http.MethodPut, submissionsRoute+f.submission+"/answers", saveBody, jsonCall(f.studentToken))
				h.waitForLockWaiters(t, 1)
				removal = h.sendInBackground(http.MethodPost, studentsPath+"/"+f.card+"/remove", "", jsonCall(f.tutorToken))
			}
			h.waitForBlockedStatements(t, 2)
			release()
			saved, removed := awaitAnswer(t, saving), awaitAnswer(t, removal)

			h.mustAnswer(t, removed, http.StatusOK)
			if removalFirst {
				assertRefused(t, "autosave after removal", saved, http.StatusNotFound, "not_found")
			} else {
				h.mustAnswer(t, saved, http.StatusOK)
			}
			if h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NOT NULL", f.issue) != 1 {
				t.Fatal("racing removal lost its marker")
			}
		})
	}
}

func TestTutorDeletionRacesHomeworkWrites(t *testing.T) {
	t.Parallel()
	for _, deletionFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("deletion_first_%t", deletionFirst), func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.workParties(t, workBlocks()...)
			table, id := "submissions", parties.submission
			if deletionFirst {
				table, id = "issues", parties.issue
			}
			release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM "+table+" WHERE id = $1::uuid FOR UPDATE", id)
			body := answersJSON(t, 1, saveID(30), typedAnswer(workQuestion(5), "Racing answer"))
			var deletion, saving <-chan pendingAnswer

			if deletionFirst {
				deletion = h.deleteInBackground(parties.tutorToken, accountPassword)
				h.waitForLockWaiters(t, 1)
				saving = h.sendInBackground(http.MethodPut, submissionsRoute+parties.submission+"/answers", body, jsonCall(parties.studentToken))
			} else {
				saving = h.sendInBackground(http.MethodPut, submissionsRoute+parties.submission+"/answers", body, jsonCall(parties.studentToken))
				h.waitForLockWaiters(t, 1)
				deletion = h.deleteInBackground(parties.tutorToken, accountPassword)
			}
			h.waitForBlockedStatements(t, 2)
			release()
			saved, deleted := awaitAnswer(t, saving), awaitAnswer(t, deletion)

			h.mustAnswer(t, deleted, http.StatusNoContent)
			if deletionFirst {
				assertRefused(t, "autosave after tutor deletion", saved, http.StatusConflict, "not_editable")
			} else {
				h.mustAnswer(t, saved, http.StatusOK)
			}
			if h.studentWork(t, parties.studentToken, parties.submission).Status != "closed" {
				t.Fatal("tutor deletion did not close racing work")
			}
		})
	}
}

func TestHomeworkCleanupRacesPurgeTutorBatch(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	now := h.now(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id = $1::uuid FOR UPDATE", f.tutor.String())
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger}

	cleaned := core.Run(t.Context(), h.logger, "test-homework-cleanup", func() error { _, err := cleanup.Run(t.Context(), now); return err })
	h.waitForLockWaiters(t, 1)
	purged := core.Run(t.Context(), h.logger, "test-homework-purge", func() error {
		for {
			var count int64
			err := h.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
				if _, err := repos.Users.LockForPurge(t.Context(), f.tutor); err != nil {
					return err
				}
				var err error
				count, err = h.tutoring.PurgeTutorBatch(t.Context(), repos, f.tutor, now)
				return err
			})
			if err != nil || count == 0 {
				return err
			}
		}
	})
	h.waitForBlockedStatements(t, 2)
	release()
	cleanupErr, purgeErr := <-cleaned, <-purged

	if cleanupErr != nil || purgeErr != nil {
		t.Fatalf("cleanup=%v purge=%v", cleanupErr, purgeErr)
	}
	if h.countRows(t, "SELECT snapshot_bytes_total FROM tutor_profiles WHERE user_id = $1::uuid", f.tutor.String()) != 0 || h.chargesOf(t, f.file) != 0 {
		t.Fatal("racing purge left accounting or charged files")
	}
}

func TestHomeworkCleanupDeadlineDoesNotStopWorkerScheduling(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	now := h.now(t)
	cleanup := homework.Cleanup{Store: h.store, Random: h.random, Logger: h.logger, Budget: 500 * time.Millisecond}
	queue := outbox.New(h.store, h.random, h.logger)
	jobs := []worker.Job{
		{Name: domain.JobHomeworkCleanup, Every: time.Minute, RunHomeworkCleanup: cleanup.Run},
		{Name: "test.after_homework_cleanup", Every: time.Minute, Run: h.tutoring.ExpireReclaims},
	}
	scheduler := worker.New(h.logger, h.clock, queue, nil, jobs, worker.DefaultTiming())
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id = $1::uuid FOR UPDATE", f.tutor.String())

	err := scheduler.Tick(t.Context(), now)
	release()
	cleanup.Budget = domain.HomeworkCleanupBudget
	resumed, resumeErr := cleanup.Run(t.Context(), now)

	if err != nil || resumeErr != nil || resumed.IssuesErased != 1 {
		t.Fatalf("deadline/resume: %v %v %+v", err, resumeErr, resumed)
	}
	if h.countLogs(`"msg":"job.run"`, `"job":"homework.cleanup"`, `"result":"failed"`) != 1 || h.countLogs(`"msg":"job.run"`, `"job":"test.after_homework_cleanup"`, `"result":"ok"`) != 1 {
		t.Fatal("a cleanup deadline stopped subsequent scheduling")
	}
}

func TestUnverifiedConfirmationBeforeFinalPurgeKeepsWork(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.issueParties(t)
	card, student := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	signedIn := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.tick(t)
	confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card.ID)))
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE users SET unverified_since = $2 WHERE id = $1", student, h.now(t).Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var batched int64
	now := h.now(t)
	if err := h.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
		if _, err := repos.Users.LockForPurge(t.Context(), student); err != nil {
			return err
		}
		var err error
		batched, err = h.tutoring.PurgeStudentHomeworkBatch(t.Context(), repos, student, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q}`, confirmation), signedIn)
	purged, purgeErr := h.purger.CleanupUnverified(context.Background(), now)

	h.mustAnswer(t, confirmed, http.StatusOK)
	if purgeErr != nil || purged != 0 || batched != 0 {
		t.Fatalf("confirmation lost to final purge: %d %d %v", purged, batched, purgeErr)
	}
	if h.storedRelationship(t, card.ID).State != "active" || len(h.submissionsOf(t, issue.ID)) != 1 || h.countRows(t, "SELECT count(*) FROM issues WHERE id = $1::uuid AND erase_pending_since IS NULL", issue.ID) != 1 {
		t.Fatal("confirmation did not preserve the link and its work")
	}
}

type lifecycleRaceRequest struct {
	method  string
	path    string
	body    string
	token   string
	table   string
	id      string
	success int
}

func prepareLifecycleRaceRequest(t *testing.T, f erasureFixture, action string) lifecycleRaceRequest {
	t.Helper()
	h := f.h
	request := lifecycleRaceRequest{method: http.MethodPost, token: f.studentToken, table: "submissions", id: f.submission, success: http.StatusOK}
	switch action {
	case "issuing":
		template := h.readyTemplate(t, f.tutorToken, "Racing issue", 70)
		request.path, request.body, request.token = issuesRoute, issueJSON(template.ID, toCard(f.card)), f.tutorToken
		request.table, request.id, request.success = "relationships", f.card, http.StatusCreated
	case "submission":
		request.path = submissionsRoute + f.submission + "/submit"
		request.body = fmt.Sprintf(`{"version":%d}`, h.studentWork(t, f.studentToken, f.submission).Version)
	case "review publication":
		h.submitted(t, f.studentToken, f.submission, h.studentWork(t, f.studentToken, f.submission).Version)
		h.reviewSaved(t, f.tutorToken, f.submission, reviewJSON(t, 1, 0, saveID(50), "Racing review", fullyScored("Review")...))
		request.path, request.body, request.token = submissionsRoute+f.submission+"/check", `{"revision":1}`, f.tutorToken
		request.table, request.id = "review_revisions", h.revisionID(t, f.submission, 1)
	}
	return request
}

func TestHomeworkLifecycleRacesIssuingSubmissionAndPublication(t *testing.T) {
	t.Parallel()
	for _, deletion := range []bool{false, true} {
		for _, lifecycleFirst := range []bool{false, true} {
			for _, action := range []string{"issuing", "submission", "review publication"} {
				t.Run(fmt.Sprintf("deletion_%t/lifecycle_first_%t/%s", deletion, lifecycleFirst, action), func(t *testing.T) {
					t.Parallel()
					f := newErasureFixture(t)
					h := f.h
					if deletion {
						h.setRelationshipState(t, f.card, domain.RelationshipActive)
					}
					request := prepareLifecycleRaceRequest(t, f, action)
					table, id := request.table, request.id
					if lifecycleFirst {
						table, id = "issues", f.issue
					}
					release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM "+table+" WHERE id = $1::uuid FOR UPDATE", id)
					frames := h.recordFrames(t)
					beforeRows := h.notificationRows(t, f.student)
					usage := h.fileUsage(t, f.tutor)
					lifecycle := func() <-chan pendingAnswer {
						if deletion {
							return h.deleteInBackground(f.tutorToken, accountPassword)
						}
						return h.sendInBackground(http.MethodPost, studentsPath+"/"+f.card+"/remove", "", jsonCall(f.tutorToken))
					}
					write := func() <-chan pendingAnswer {
						return h.sendInBackground(request.method, request.path, request.body, jsonCall(request.token))
					}
					var ended, written <-chan pendingAnswer

					if lifecycleFirst {
						ended = lifecycle()
						h.waitForLockWaiters(t, 1)
						written = write()
					} else {
						written = write()
						h.waitForLockWaiters(t, 1)
						ended = lifecycle()
					}
					h.waitForBlockedStatements(t, 2)
					release()
					result, changed := awaitAnswer(t, written), awaitAnswer(t, ended)
					sent := frames.settle(t, h)

					expected := http.StatusOK
					if deletion {
						expected = http.StatusNoContent
					}
					h.mustAnswer(t, changed, expected)
					if !lifecycleFirst {
						h.mustAnswer(t, result, request.success)
					} else if !deletion {
						assertRefused(t, "write after removal", result, http.StatusNotFound, "not_found")
					} else if action == "submission" {
						assertRefused(t, "submit after deletion", result, http.StatusConflict, "not_editable")
					} else {
						assertRefused(t, "tutor write after deletion", result, http.StatusUnauthorized, "unauthenticated")
					}
					if h.fileUsage(t, f.tutor) != usage || h.chargesOf(t, f.file) != 1 {
						t.Fatal("lifecycle/write race released charges synchronously")
					}
					if lifecycleFirst && h.notificationRows(t, f.student) != beforeRows {
						t.Fatal("a refused racing write created a student notification")
					}
					if action != "review publication" || !deletion || !lifecycleFirst {
						if !hasFrame(sent, f.tutor, domain.EntitySubmission, f.submission) || !hasFrame(sent, f.student, domain.EntitySubmission, f.submission) {
							t.Fatalf("missing authorized lifecycle/write recipients: %v", sent)
						}
					} else if hasFrame(sent, f.student, domain.EntitySubmission, f.submission) {
						t.Fatal("unchanged submitted work received a publication hint")
					}
					for _, frame := range sent {
						if frame.to != f.student && frame.to != f.tutor {
							t.Fatal("homework race named an unrelated recipient")
						}
					}
				})
			}
		}
	}
}

func TestHomeworkLifecycleRacesHeldUploadCompletion(t *testing.T) {
	t.Parallel()
	for _, deletion := range []bool{false, true} {
		for _, lifecycleFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("deletion_%t/lifecycle_first_%t", deletion, lifecycleFirst), func(t *testing.T) {
				t.Parallel()
				h, held := newHeldStorageHarness(t)
				parties := h.workParties(t, workBlocks()...)
				if !deletion {
					h.setRelationshipState(t, parties.card.ID, domain.RelationshipProvisional)
				}
				usage := h.fileUsage(t, parties.tutor)
				frames := h.recordFrames(t)
				held.holdNext(func(request *http.Request) bool {
					return request.Method == http.MethodPost && request.URL.Query().Has("uploadId")
				})
				upload := h.startUploadTo(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, workQuestion(6)))
				upload.begin(t, "held.pdf", pdfFile(1024))
				written := upload.completeInBackground(nil)
				held.awaitReached(t)
				lifecycle := func() <-chan pendingAnswer {
					if deletion {
						return h.deleteInBackground(parties.tutorToken, accountPassword)
					}
					return h.sendInBackground(http.MethodPost, studentsPath+"/"+parties.card.ID+"/remove", "", jsonCall(parties.tutorToken))
				}
				var changed response

				if lifecycleFirst {
					changed = awaitAnswer(t, lifecycle())
					held.resume()
				} else {
					release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM issues WHERE id = $1::uuid FOR UPDATE", parties.issue)
					held.resume()
					h.waitForLockWaiters(t, 1)
					changing := lifecycle()
					h.waitForBlockedStatements(t, 2)
					release()
					changed = awaitAnswer(t, changing)
				}
				result := upload.answer(t)
				writeErr := <-written
				sent := frames.settle(t, h)

				expected := http.StatusOK
				if deletion {
					expected = http.StatusNoContent
				}
				h.mustAnswer(t, changed, expected)
				if writeErr != nil {
					t.Fatal(writeErr)
				}
				if !lifecycleFirst {
					h.mustAnswer(t, result, http.StatusCreated)
				} else if deletion {
					assertRefused(t, "upload completion after deletion", result, http.StatusConflict, "not_editable")
				} else {
					assertRefused(t, "upload completion after removal", result, http.StatusNotFound, "not_found")
				}
				if h.fileUsage(t, parties.tutor) != usage || h.answerReferences(t, parties.submission) != 0 {
					t.Fatal("unbound upload changed homework charges")
				}
				if !hasFrame(sent, parties.student, domain.EntitySubmission, parties.submission) || !hasFrame(sent, parties.tutor, domain.EntitySubmission, parties.submission) {
					t.Fatalf("missing final recipients: %v", sent)
				}
				if lifecycleFirst {
					for _, row := range h.uploadsIn(t, parties.student, domain.ContextAnswer) {
						if row.State == string(domain.AttachmentPending) || row.State == string(domain.AttachmentBound) {
							t.Fatal("a late upload became publishable")
						}
					}
				}
			})
		}
	}
}

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

type homeworkEmailCount struct {
	kind  domain.OutboxKind
	group domain.EmailGroup
	count int
}

type tutorStateCase struct {
	name        string
	deactivate  func(t *testing.T, h *authHarness, parties endingParties)
	dropped     []homeworkEmailCount
	finished    string
	sentEmails  int
	cappedSlots int64
}

func TestHomeworkEmailsDropForInactiveTutor(t *testing.T) {
	t.Parallel()
	cases := []tutorStateCase{
		{name: "blocked with results off", deactivate: blockTutorAfterResultsOff, finished: "done:obsolete", dropped: []homeworkEmailCount{
			{kind: domain.EventIssueCreated, group: domain.EmailGroupHomework, count: 2},
			{kind: domain.EventSubmissionChecked, group: domain.EmailGroupResults, count: 1},
			{kind: domain.EventReviewUpdated, group: domain.EmailGroupResults, count: 1},
			{kind: domain.EventSubmissionReturned, group: domain.EmailGroupResults, count: 1},
		}},
		{name: "deletion started", deactivate: deleteTutor, finished: "done:obsolete", dropped: []homeworkEmailCount{
			{kind: domain.EventIssueCreated, group: domain.EmailGroupHomework, count: 1},
			{kind: domain.EventSubmissionChecked, group: domain.EmailGroupResults, count: 1},
			{kind: domain.EventReviewUpdated, group: domain.EmailGroupResults, count: 1},
			{kind: domain.EventSubmissionReturned, group: domain.EmailGroupResults, count: 1},
		}},
		{name: "active", deactivate: keepTutorActive, finished: "done:sent", sentEmails: 5, cappedSlots: 5},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.queueEveryStudentHomeworkEmail(t)
			testCase.deactivate(t, h, parties)

			h.tick(t)

			if sent := h.homeworkEmailsTo(t, studentEmail); sent != testCase.sentEmails {
				t.Fatalf("the student got %d homework emails, want %d", sent, testCase.sentEmails)
			}
			if slots := h.homeworkEmailSlots(t, parties.student); slots != testCase.cappedSlots {
				t.Fatalf("the student's homework email counter is %d, want %d", slots, testCase.cappedSlots)
			}
			for _, dropped := range testCase.dropped {
				logged := h.countLogs(`"msg":"email.dropped"`, fmt.Sprintf(`"event":%q`, dropped.kind), fmt.Sprintf(`"group":%q`, dropped.group),
					`"reason":"tutor_inactive"`)
				if logged != dropped.count {
					t.Errorf("%s logged %d tutor_inactive drops, want %d", dropped.kind, logged, dropped.count)
				}
			}
			for kind, queued := range map[domain.OutboxKind]int64{
				domain.EventIssueCreated: 2, domain.EventSubmissionChecked: 1, domain.EventReviewUpdated: 1, domain.EventSubmissionReturned: 1,
			} {
				if finished := h.outboxOf(t, kind, testCase.finished); finished != queued {
					t.Errorf("%s finished %d rows as %s, want %d", kind, finished, testCase.finished, queued)
				}
			}
		})
	}
}

func (h *authHarness) queueEveryStudentHomeworkEmail(t *testing.T) endingParties {
	t.Helper()
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice")...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Even better", fullyScored("Nice")...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 2)
	returned := h.submittedByCard(t, parties.tutorToken, parties.card.ID, parties.studentToken, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, returned, reviewJSON(t, 1, 0, saveID(3), "Try again", commented(scored(workQuestion(5), 1), "Too short")))
	h.published(t, parties.tutorToken, returned, domain.ReviewReturn, 1)
	return parties.endingParties
}

func blockTutorAfterResultsOff(t *testing.T, h *authHarness, parties endingParties) {
	t.Helper()
	h.mustAnswer(t, h.savePreferences(t, parties.studentToken, `{"groups":[{"group":"results","enabled":false}]}`), http.StatusOK)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
}

func deleteTutor(t *testing.T, h *authHarness, parties endingParties) {
	t.Helper()
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
}

func keepTutorActive(*testing.T, *authHarness, endingParties) {}

func (h *authHarness) homeworkEmailsTo(t *testing.T, recipient string) int {
	t.Helper()
	count := 0
	for _, message := range h.mailTo(t, recipient) {
		if strings.Contains(message.Subject, issueTitle) {
			count++
		}
	}
	return count
}

func (h *authHarness) homeworkEmailSlots(t *testing.T, student domain.UserID) int64 {
	t.Helper()
	subject := h.keys.ThrottleSubject(string(domain.PurposeHomeworkEmails), student.String())
	var slots int64
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT coalesce(sum(count), 0)::bigint FROM rate_limit_counters WHERE purpose = $1 AND subject = $2",
		string(domain.PurposeHomeworkEmails), subject).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	return slots
}

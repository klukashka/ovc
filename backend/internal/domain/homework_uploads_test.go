package domain_test

import (
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestOnlyAStudentOnAnOpenLinkWritesOpenWork(t *testing.T) {
	t.Parallel()
	writableStatuses := []domain.SubmissionStatus{domain.SubmissionNotStarted, domain.SubmissionDraft, domain.SubmissionReturned}
	writableLinks := []domain.RelationshipState{domain.RelationshipActive, domain.RelationshipProvisional}
	var wrong []string

	for _, status := range domain.SubmissionStatuses {
		for _, link := range domain.RelationshipStates {
			want := slices.Contains(writableStatuses, status) && slices.Contains(writableLinks, link)
			if domain.WritableStudentSubmission(status, link) != want {
				wrong = append(wrong, string(status)+"/"+string(link))
			}
		}
	}

	if len(wrong) != 0 || len(domain.SubmissionStatuses) != 6 || len(domain.RelationshipStates) != 6 {
		t.Fatalf("the writability rule disagrees for %v", wrong)
	}
}

func TestOnlySubmittedOrCheckedWorkOfALiveIssueIsReviewable(t *testing.T) {
	t.Parallel()
	reviewableStatuses := []domain.SubmissionStatus{domain.SubmissionSubmitted, domain.SubmissionChecked}
	var wrong []string

	for _, issue := range []domain.IssueStatus{domain.IssueActive, domain.IssueWithdrawn} {
		for _, marked := range []bool{false, true} {
			for _, status := range domain.SubmissionStatuses {
				want := issue == domain.IssueActive && !marked && slices.Contains(reviewableStatuses, status)
				if domain.ReviewableSubmission(issue, marked, status) != want {
					wrong = append(wrong, string(issue)+"/"+string(status))
				}
			}
		}
	}

	if len(wrong) != 0 {
		t.Fatalf("the reviewability rule disagrees for %v", wrong)
	}
}

func TestADeletedTutorsWorkIsReadableUntilAccountClosedUntil(t *testing.T) {
	t.Parallel()
	deletedAt := time.Date(2030, time.March, 1, 9, 30, 0, 0, time.UTC)
	closedUntil := deletedAt.Add(domain.DeletedAccountRetention)
	cases := []struct {
		name  string
		tutor domain.TutorStanding
		now   time.Time
		want  bool
	}{
		{"an active tutor long after", domain.TutorStanding{State: domain.AccountActive}, closedUntil.Add(time.Hour), true},
		{"a blocked tutor", domain.TutorStanding{State: domain.AccountBlocked}, closedUntil.Add(time.Hour), true},
		{"a deleted tutor at the deletion", domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}, deletedAt, true},
		{"a deleted tutor a minute before", domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}, closedUntil.Add(-time.Minute), true},
		{"a deleted tutor a microsecond before", domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}, closedUntil.Add(-time.Microsecond), true},
		{"a deleted tutor at the instant", domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}, closedUntil, false},
		{"a deleted tutor after", domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}, closedUntil.Add(time.Hour), false},
		{"a deleted tutor without a deletion time", domain.TutorStanding{State: domain.AccountDeleted}, deletedAt, false},
	}
	var wrong []string

	for _, testCase := range cases {
		cutoffSays := testCase.tutor.State != domain.AccountDeleted ||
			(testCase.tutor.DeletedAt != nil && testCase.tutor.DeletedAt.After(domain.ReadableTutorWorkCutoff(testCase.now)))
		if testCase.tutor.ReadableAt(testCase.now) != testCase.want || cutoffSays != testCase.want {
			wrong = append(wrong, testCase.name)
		}
	}

	if len(wrong) != 0 {
		t.Fatalf("the readability of a deleted tutor's work disagrees for %v", wrong)
	}
}

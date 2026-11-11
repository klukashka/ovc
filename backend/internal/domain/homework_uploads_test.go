package domain_test

import (
	"slices"
	"testing"

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

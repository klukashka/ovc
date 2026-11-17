package api_test

import (
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func onlyFrame(t *testing.T, frames []sentFrame, what string) sentFrame {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("%s: expected one frame, got %+v", what, frames)
	}
	return frames[0]
}

func framesOfKind(frames []sentFrame, kind domain.EntityKind) []sentFrame {
	var found []sentFrame
	for _, frame := range frames {
		if frame.kind == string(kind) {
			found = append(found, frame)
		}
	}
	return found
}

type lessonLinkScene struct {
	h          *authHarness
	parties    issueParties
	unrostered domain.UserID
	rostered   domain.UserID
	group      string
	previous   string
	next       string
}

func newLessonLinkScene(t *testing.T) lessonLinkScene {
	t.Helper()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cards := h.activeCards(t, parties.tutor, 10, 2)
	group := h.newGroup(t, parties.tutorToken, "Evening club", cards)
	return lessonLinkScene{
		h: h, parties: parties, unrostered: domain.UserID{10}, rostered: domain.UserID{11}, group: group.ID,
		previous: h.scheduled(t, parties.tutorToken, groupLessonFor(group.ID, "2030-03-12", "10:00")).Lessons[0].ID,
		next:     h.scheduled(t, parties.tutorToken, groupLessonFor(group.ID, "2030-03-19", "10:00")).Lessons[0].ID,
	}
}

func (s lessonLinkScene) issue(t *testing.T) string {
	t.Helper()
	return s.h.issued(t, s.parties.tutorToken, issueJSONWith(s.parties.template.ID, toGroup(s.group), &s.previous, nil, "after_check")).ID
}

func (s lessonLinkScene) patch(t *testing.T, issue string, body string) {
	t.Helper()
	s.h.mustAnswer(t, s.h.patchIssue(t, s.parties.tutorToken, issue, body), http.StatusOK)
}

func (s lessonLinkScene) movedAndUnseen(t *testing.T) string {
	t.Helper()
	issue := s.issue(t)
	s.h.cancelWithRoster(t, s.previous, s.rostered)
	s.patch(t, issue, `{"lesson_id":"`+s.next+`","version":1}`)
	s.h.cancelWithRoster(t, s.next)
	return issue
}

type issueEventLessons struct {
	tutor      string
	unrostered string
	rostered   string
}

func (s lessonLinkScene) checkIssueEvent(t *testing.T, sent []sentFrame, issue string, want issueEventLessons) {
	t.Helper()
	tutorFrames := homeworkFramesTo(sent, s.parties.tutor)
	issueFrame := onlyFrame(t, framesOfKind(tutorFrames, domain.EntityIssue), "the tutor's issue frame")
	if issueFrame.id != issue || issueFrame.lessons != want.tutor || issueFrame.issue != "" {
		t.Errorf("the tutor's issue frame is %+v", issueFrame)
	}
	submissionFrames := framesOfKind(tutorFrames, domain.EntitySubmission)
	if len(submissionFrames) != 2 {
		t.Errorf("the tutor got submission frames %+v", tutorFrames)
	}
	for _, frame := range submissionFrames {
		if frame.issue != "" || frame.lessons != "" {
			t.Errorf("a tutor submission frame of an issue event carries %+v", frame)
		}
	}
	for student, lessons := range map[domain.UserID]string{s.unrostered: want.unrostered, s.rostered: want.rostered} {
		frame := onlyFrame(t, homeworkFramesTo(sent, student), "a student's frame")
		if frame.lessons != lessons {
			t.Errorf("the student %s got lessons %q, want %q", student, frame.lessons, lessons)
		}
		checkStudentHomeworkFrames(t, sent, student, strings.Split(lessons, ",")...)
	}
}

func TestIssuingNamesTheLinkedLessonToEveryRecipient(t *testing.T) {
	t.Parallel()
	scene := newLessonLinkScene(t)
	frames := scene.h.recordFrames(t)

	issue := scene.issue(t)

	linked := lessonList(scene.previous)
	scene.checkIssueEvent(t, frames.settle(t, scene.h), issue, issueEventLessons{tutor: linked, unrostered: linked, rostered: linked})
}

func TestMovingTheLinkedLessonNamesOnlyTheLessonsEachRecipientSees(t *testing.T) {
	t.Parallel()
	scene := newLessonLinkScene(t)
	issue := scene.issue(t)
	scene.h.cancelWithRoster(t, scene.previous, scene.rostered)
	frames := scene.h.recordFrames(t)

	scene.patch(t, issue, `{"lesson_id":"`+scene.next+`","version":1}`)

	scene.checkIssueEvent(t, frames.settle(t, scene.h), issue, issueEventLessons{
		tutor: lessonList(scene.previous, scene.next), unrostered: lessonList(scene.next), rostered: lessonList(scene.previous, scene.next),
	})
}

func TestADeadlineChangeNamesALessonNoStudentSeesOnlyToTheTutor(t *testing.T) {
	t.Parallel()
	scene := newLessonLinkScene(t)
	issue := scene.movedAndUnseen(t)
	frames := scene.h.recordFrames(t)

	scene.patch(t, issue, `{"deadline_at":"2030-04-01T10:00:00Z","version":2}`)

	scene.checkIssueEvent(t, frames.settle(t, scene.h), issue, issueEventLessons{tutor: lessonList(scene.next)})
}

func TestUnlinkingAnUnseenLessonNamesItOnlyToTheTutor(t *testing.T) {
	t.Parallel()
	scene := newLessonLinkScene(t)
	issue := scene.movedAndUnseen(t)
	frames := scene.h.recordFrames(t)

	scene.patch(t, issue, `{"lesson_id":null,"version":2}`)

	scene.checkIssueEvent(t, frames.settle(t, scene.h), issue, issueEventLessons{tutor: lessonList(scene.next)})
}

type linkedSubmission struct {
	h          *authHarness
	parties    endingParties
	issue      string
	submission string
	lesson     string
	draft      int64
}

func newLinkedSubmission(t *testing.T) linkedSubmission {
	t.Helper()
	h := newAuthHarness(t)
	parties := h.activeStudentInAGroup(t)
	issue, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Trip to Paris", string(domain.AnswerPolicyAfterCheck), nil, workBlocks()...)
	lesson := h.lessonWith(t, parties.tutorToken, parties.card.ID)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, issue, `{"lesson_id":"`+lesson+`","version":1}`), http.StatusOK)
	draft := h.saved(t, parties.studentToken, submissions[0].id, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "We saw the tower")))
	return linkedSubmission{h: h, parties: parties, issue: issue, submission: submissions[0].id, lesson: lesson, draft: draft.Version}
}

func (w linkedSubmission) checkSubmissionEvent(t *testing.T, sent []sentFrame) {
	t.Helper()
	tutorFrame := onlyFrame(t, homeworkFramesTo(sent, w.parties.tutor), "the tutor")
	if tutorFrame.kind != string(domain.EntitySubmission) || tutorFrame.id != w.submission || tutorFrame.issue != w.issue || tutorFrame.lessons != w.lesson {
		t.Errorf("the tutor's frame is %+v", tutorFrame)
	}
	studentFrame := onlyFrame(t, homeworkFramesTo(sent, w.parties.student), "the student")
	if studentFrame.id != w.submission || studentFrame.lessons != w.lesson {
		t.Errorf("the student's frame is %+v", studentFrame)
	}
	checkStudentHomeworkFrames(t, sent, w.parties.student, w.lesson)
}

func TestSubmittingSendsOneFramePerRecipient(t *testing.T) {
	t.Parallel()
	work := newLinkedSubmission(t)
	frames := work.h.recordFrames(t)

	work.h.submitted(t, work.parties.studentToken, work.submission, work.draft)

	work.checkSubmissionEvent(t, frames.settle(t, work.h))
}

func TestPublishingAReviewSendsOneFramePerRecipient(t *testing.T) {
	t.Parallel()
	work := newLinkedSubmission(t)
	work.h.submitted(t, work.parties.studentToken, work.submission, work.draft)
	work.h.reviewSaved(t, work.parties.tutorToken, work.submission, reviewJSON(t, 1, 0, saveID(2), "Good", fullyScored("Nice")...))
	frames := work.h.recordFrames(t)

	work.h.published(t, work.parties.tutorToken, work.submission, domain.ReviewCheck, 1)

	work.checkSubmissionEvent(t, frames.settle(t, work.h))
}

func TestClosingOnEndingNamesTheIssueOnlyToTheTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.activeStudentInAGroup(t)
	issue, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Trip to Paris", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	lesson := h.lessonWith(t, parties.tutorToken, parties.card.ID)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, issue, `{"lesson_id":"`+lesson+`","version":1}`), http.StatusOK)
	frames := h.recordFrames(t)

	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

	sent := frames.settle(t, h)
	tutorFrame := onlyFrame(t, homeworkFramesTo(sent, parties.tutor), "the tutor")
	if tutorFrame.id != submissions[0].id || tutorFrame.issue != issue || tutorFrame.lessons != lesson {
		t.Fatalf("the tutor's closing frame is %+v", tutorFrame)
	}
	studentFrame := onlyFrame(t, homeworkFramesTo(sent, parties.student), "the student")
	if studentFrame.id != submissions[0].id || studentFrame.lessons != lesson {
		t.Fatalf("the student's closing frame is %+v", studentFrame)
	}
	checkStudentHomeworkFrames(t, sent, parties.student, lesson)
}

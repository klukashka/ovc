package api_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func (h *authHarness) visibleLessons(t *testing.T, token string, lessons []string) []string {
	t.Helper()
	listed := calendarIDs(h.calendarOf(t, token, calendarFrom, calendarTo))
	var visible []string
	for _, lesson := range lessons {
		onPage := h.lessonPage(t, token, lesson).status == http.StatusOK
		if onPage != slices.Contains(listed, lesson) {
			t.Fatalf("lesson %s is on its page %t but in the calendar %t", lesson, onPage, !onPage)
		}
		if onPage {
			visible = append(visible, lesson)
		}
	}
	return visible
}

func (h *authHarness) labelledLessonRows(t *testing.T, token string) int {
	t.Helper()
	labelled := 0
	for _, row := range h.notificationsOf(t, token) {
		if row.Link != nil && row.Link.Kind == "lesson" && row.Label != nil {
			labelled++
		}
	}
	return labelled
}

func TestArchivingWhoseMembersChangeTwiceAnswersConflictAndChangesNothing(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	members := h.activeCards(t, tutor, 11, 2)
	joining, later := h.linkedCard(t, tutor, 13, domain.RelationshipActive), h.linkedCard(t, tutor, 14, domain.RelationshipActive)
	group := h.newGroup(t, token, archivedGroupName, members).ID
	lesson := h.scheduled(t, token, groupLessonFor(group, "2030-03-12", "18:00")).Lessons[0].ID
	releaseGroup := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM groups WHERE id = $1::uuid FOR NO KEY UPDATE", group,
		"INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) SELECT $1::uuid, '"+joining+"'::uuid, tutor_id, now() FROM groups WHERE id = $1::uuid")
	heldGroup := h.contested
	releaseCard := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR NO KEY UPDATE", joining,
		"INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) SELECT '"+group+"'::uuid, '"+later+"'::uuid, tutor_id, now() FROM relationships WHERE id = $1::uuid")
	heldCard := h.contested
	h.contested = heldGroup
	answer := h.sendInBackground(http.MethodPost, groupsRoute+"/"+group+"/archive", "", jsonCall(token))
	h.waitForLockWaiters(t, 1)

	releaseGroup()
	h.contested = heldCard
	h.waitForLockWaiters(t, 1)
	releaseCard()
	refused := awaitAnswer(t, answer)

	if refused.status != http.StatusConflict || code(t, refused) != "conflict" {
		t.Fatalf("the archiving answered %d %s", refused.status, refused.body)
	}
	if archived := h.countRows(t, "SELECT count(id) FROM groups WHERE id = $1::uuid AND archived_at IS NOT NULL", group); archived != 0 {
		t.Fatal("the refused archiving archived the group")
	}
	if stored := h.storedLesson(t, lesson); stored.Status != "scheduled" || stored.Version != 1 || h.outboxKeysOf(t, domain.EventGroupArchived) != 0 {
		t.Fatalf("the refused archiving changed the lesson %+v or queued emails", stored)
	}
	if rows := h.countRows(t, "SELECT count(id) FROM notifications WHERE event = $1", "group.archived"); rows != 0 {
		t.Fatalf("the refused archiving wrote %d rows", rows)
	}
}

func TestAFormerMembersArchivedNoticeCarriesTheNeutralLabel(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00"))
	h.archive(t, parties.tutorToken, parties.group)
	if removed := h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.card+`"]}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}

	h.tick(t)
	rows := h.notificationsOf(t, parties.studentToken)

	emails := h.emailsWithSubject(t, memberEmail(10), "Group archived: Group lesson")
	if len(emails) != 1 || strings.Contains(emails[0].Text, groupName) || strings.Contains(emails[0].HTML, groupName) || len(listedLessons(emails[0])) != 1 {
		t.Fatalf("the former member got %v", subjects(h.mailTo(t, memberEmail(10))))
	}
	if len(rows) != 1 || rows[0].Event != "group.archived" || rows[0].Label != nil || rows[0].LabelKind == nil || *rows[0].LabelKind != "group" {
		t.Fatalf("the former member's rows are %+v", rows)
	}
}

func TestArchivedNoticesToStudentsAreDroppedWhileTheTutorIsBlocked(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00"))
	h.archive(t, parties.tutorToken, parties.group)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	h.tick(t)

	if len(h.mailTo(t, memberEmail(10))) != 0 || len(h.mailTo(t, memberEmail(11))) != 0 {
		t.Fatal("a member got the archived notice from a blocked tutor")
	}
	if h.countLogs(`"msg":"email.dropped"`, `"event":"group.archived"`, `"reason":"tutor_inactive"`, `"group":"lesson_changes"`) != 2 {
		t.Fatalf("the drops were reported as:\n%s", h.logs.String())
	}
}

func TestACancellationEmailOfALinkThatBecameReclaimPendingIsObsolete(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	link := squattedLink(t, h)
	lesson := h.lessonAt(t, link.tutorToken, link.card, "2030-03-12", "18:00")
	h.tick(t)
	confirmation := latestToken(t, h.mailTo(t, memberEmail(provisionalStudent)), confirmSubject)
	if result := h.cancelCall(t, link.tutorToken, lesson, "this", 1); result.status != http.StatusOK {
		t.Fatalf("the cancellation answered %d %s", result.status, result.body)
	}
	reclaim := h.verify(t, `{"token":"`+confirmation+`","new_password":"`+ownerPassword+`"}`, "")
	if reclaim.status != http.StatusOK || outcomeOf(t, reclaim) != "choice_required" {
		t.Fatalf("the reclaim answered %d %s", reclaim.status, reclaim.body)
	}

	h.tick(t)

	if cancelled := h.changeEmailsTo(t, memberEmail(provisionalStudent), "Lesson cancelled"); len(cancelled) != 0 {
		t.Fatalf("the mailbox owner got %v", subjects(cancelled))
	}
	if h.outboxOf(t, domain.EventLessonCancelled, "done:obsolete") != 1 {
		t.Fatal("the cancellation email was not dropped as obsolete")
	}
}

func TestAReclaimPendingLinksLessonsAreHiddenUntilTheOwnerKeepsIt(t *testing.T) {
	t.Parallel()
	for name, keep := range map[string]bool{"kept": true, "dropped": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			link := squattedLink(t, h)
			cancelled := h.lessonAt(t, link.tutorToken, link.card, "2030-03-12", "18:00")
			ended := h.lessonAt(t, link.tutorToken, link.card, "2030-03-10", "13:00")
			scheduled := h.lessonAt(t, link.tutorToken, link.card, "2030-03-14", "18:00")
			h.cancelCall(t, link.tutorToken, cancelled, "this", 1)
			h.tickAt(t, time.Date(2030, time.March, 10, 11, 31, 0, 0, time.UTC))
			lessons := []string{cancelled, ended, scheduled}
			owner := h.reclaimed(t, memberEmail(provisionalStudent))
			pending := h.visibleLessons(t, owner, lessons)
			labelled := h.labelledLessonRows(t, owner)
			choice := reclaimChoice{DropLinks: []string{link.card}}
			if keep {
				choice = reclaimChoice{KeepLinks: []string{link.card}}
			}

			decided := h.decideReclaim(t, owner, choice)
			after := h.visibleLessons(t, owner, lessons)

			if decided.status != http.StatusOK || len(pending) != 0 || labelled != 0 {
				t.Fatalf("while pending the account saw %v and %d labelled rows; the choice answered %d %s", pending, labelled, decided.status, decided.body)
			}
			if keep && !slices.Equal(after, lessons) {
				t.Fatalf("after the keep the account sees %v", after)
			}
			if !keep && (len(after) != 0 || h.rosterRowsOf(t, link.student) != 0) {
				t.Fatalf("after the drop the account sees %v and keeps %d roster rows", after, h.rosterRowsOf(t, link.student))
			}
		})
	}
}

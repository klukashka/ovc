package api_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
)

const (
	absentLesson     = "00000000-0000-4000-8000-000000000001"
	absentRoutingKey = "00000000-0000-4000-8000-000000000002"
)

func expectRefusal(t *testing.T, what string, result response, status int, errorCode string) {
	t.Helper()
	if result.status != status || code(t, result) != errorCode {
		t.Fatalf("%s answered %d %s, expected %d %s", what, result.status, result.body, status, errorCode)
	}
}

func oneToOneLesson(t *testing.T, h *authHarness, parties visibilityParties) string {
	t.Helper()
	return h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, firstLessonLocal)
}

func groupLesson(t *testing.T, h *authHarness, parties visibilityParties) string {
	t.Helper()
	request := groupLessonFor(parties.group, firstLessonDate, firstLessonLocal)
	request.AllowOverlap = true
	return h.scheduled(t, parties.tutorToken, request).Lessons[0].ID
}

func TestMeetingJoinAuthorization(t *testing.T) {
	t.Run("the tutor window opens ten minutes before the start", testJoinTutorWindow)
	t.Run("the tutor window closes with the lesson", testJoinTutorWindowCloses)
	t.Run("the grace period is an exact boundary for both roles", testJoinGraceBoundary)
	t.Run("an unconfirmed tutor is refused like every tutor route", testJoinUnconfirmedTutor)
	t.Run("the student joins only a live lesson", testJoinStudentNeedsLive)
	t.Run("a blocked tutor's open room refuses students", testJoinBlockedTutor)
	t.Run("identifiers outside the scope are not found", testJoinOutsideScope)
	t.Run("two tutors sharing one student stay separate", testJoinSharedStudent)
	t.Run("an ended relationship stops further joins", testJoinEndedRelationship)
	t.Run("a provisional student joins one to one", testJoinProvisional)
	t.Run("a reclaim-pending student is not found and spends no slot", testJoinReclaimPending)
	t.Run("group membership decides and joining adds to the roster", testJoinGroupRoster)
	t.Run("an absent room closes without ending the lesson", testJoinAbsentRoom)
	t.Run("provider failure and capacity are distinct refusals", testJoinProviderAndCapacity)
	t.Run("a provider outage on an open room refuses without closing it", testJoinProviderDownOnOpenRoom)
	t.Run("the tutor limit refuses before any reservation", testJoinTutorLimit)
	t.Run("two joins at the last slot admit one under the production guard", testJoinTutorLimitRace)
	t.Run("a fresh uncertain outcome pauses joining that lesson", testJoinUncertainPause)
	t.Run("the provider never receives the lesson title", testJoinNeutralRoomName)
	t.Run("the request budget stops work on a held provider", testJoinRequestBudget)
	t.Run("a deployment without test control refuses before any change", testJoinDisabledDeployment)
	t.Run("credentials never reach logs or the outbox", testJoinCredentialPrivacy)
}

func testJoinTutorWindow(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	h.at(t, firstLessonStart.Add(-20*time.Minute))
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.at(t, firstLessonStart.Add(-domain.MeetingJoinWindowLead-time.Second))

	early := h.joinCall(t, parties.tutorToken, lesson)
	h.at(t, firstLessonStart.Add(-domain.MeetingJoinWindowLead))
	onTime := h.joined(t, parties.tutorToken, lesson)
	view := h.roomLesson(t, parties.studentToken, lesson)

	expectRefusal(t, "an early join", early, http.StatusConflict, "outside_join_window")
	if onTime.EmbedAllowed || view.EmbedAllowed || !view.RoomOpen || view.Status != "live" {
		t.Fatalf("the first join: %+v, the lesson %+v", onTime, view)
	}
	if !slices.Equal(h.rosterOf(t, lesson), []domain.UserID{parties.student}) {
		t.Fatalf("the roster froze as %v", h.rosterOf(t, lesson))
	}
}

func testJoinTutorWindowCloses(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	cancelled := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "19:00")
	h.exec2(t, "UPDATE lessons SET status = 'cancelled', cancelled_at = $2::timestamptz, cancel_cause = 'tutor' WHERE id = $1::uuid", cancelled, rfc3339(authStart))
	h.joined(t, parties.tutorToken, lesson)
	if result := h.endCall(t, parties.tutorToken, lesson); result.status != http.StatusOK {
		t.Fatalf("end answered %d %s", result.status, result.body)
	}
	unjoined := h.lessonAt(t, parties.tutorToken, parties.card, "2030-03-11", "18:30")

	ended := h.joinCall(t, parties.tutorToken, lesson)
	refusedCancelled := h.joinCall(t, parties.tutorToken, cancelled)
	h.at(t, firstLessonEnd.Add(30*time.Minute))
	tutorToken := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.exec(t, "UPDATE lessons SET starts_at = starts_at - interval '30 minutes' WHERE id = $1::uuid", unjoined)
	past := h.joinCall(t, tutorToken, unjoined)

	expectRefusal(t, "joining an ended lesson", ended, http.StatusConflict, "outside_join_window")
	expectRefusal(t, "joining a cancelled lesson", refusedCancelled, http.StatusConflict, "outside_join_window")
	expectRefusal(t, "joining past the grace", past, http.StatusConflict, "outside_join_window")
}

func testJoinGraceBoundary(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	h.at(t, firstLessonEnd.Add(domain.LessonEndGrace-time.Second))
	tutorToken := accessToken(t, h.login(t, tutorEmail, accountPassword))
	studentToken := h.studentToken(t, 10)

	lastTutor := h.joinCall(t, tutorToken, lesson)
	lastStudent := h.joinCall(t, studentToken, lesson)
	h.at(t, firstLessonEnd.Add(domain.LessonEndGrace))
	closedTutor := h.joinCall(t, tutorToken, lesson)
	closedStudent := h.joinCall(t, studentToken, lesson)

	if lastTutor.status != http.StatusOK || lastStudent.status != http.StatusOK {
		t.Fatalf("the last instant answered %d and %d", lastTutor.status, lastStudent.status)
	}
	expectRefusal(t, "a tutor at the grace", closedTutor, http.StatusConflict, "outside_join_window")
	expectRefusal(t, "a student at the grace", closedStudent, http.StatusConflict, "lesson_not_live")
}

func testJoinUnconfirmedTutor(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 3, "unconfirmed@example.org")

	result := h.joinCall(t, token, absentLesson)

	expectRefusal(t, "an unconfirmed tutor", result, http.StatusForbidden, "email_not_verified")
}

func testJoinStudentNeedsLive(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)

	before := h.joinCall(t, parties.studentToken, lesson)
	frames := h.recordFrames(t)
	h.joined(t, parties.tutorToken, lesson)
	sent := frames.settle(t, h)
	after := h.joined(t, parties.studentToken, lesson)

	expectRefusal(t, "a student before the tutor", before, http.StatusConflict, "lesson_not_live")
	if !hasFrame(sent, parties.student, domain.EntityLesson, lesson) || !hasFrame(sent, parties.tutor, domain.EntityLesson, lesson) {
		t.Fatalf("the room opening sent %v", sent)
	}
	room := h.currentRoom(t, lesson)
	if err := h.enter(t, after, room, lesson, parties.student, domain.ParticipantViewer); err != nil {
		t.Fatalf("the student link does not enter: %v", err)
	}
	if err := h.enter(t, after, room, lesson, parties.student, domain.ParticipantModerator); err == nil {
		t.Fatal("the student link entered as a moderator")
	}
	if h.countRows(t, "SELECT count(id) FROM meetings WHERE lesson_id = $1::uuid AND first_join_at IS NOT NULL", lesson) != 1 {
		t.Fatal("the first join was not recorded")
	}
}

func testJoinBlockedTutor(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	roomsBefore := h.providerRooms(t)

	refused := h.joinCall(t, parties.studentToken, lesson)

	expectRefusal(t, "a student of a blocked tutor", refused, http.StatusConflict, "lesson_not_live")
	if strings.Contains(refused.body, `"url"`) {
		t.Fatalf("the refusal carried a link: %s", refused.body)
	}
	participants := h.countRows(t, "SELECT count(participant_id) FROM lesson_participants WHERE lesson_id = $1::uuid", lesson)
	if participants != 1 || h.currentRoom(t, lesson) != room || h.meetingCount(t, lesson) != 1 || h.providerRooms(t) != roomsBefore {
		t.Fatalf("the refused join left %d participants, %d meetings, %d provider rooms", participants, h.meetingCount(t, lesson), h.providerRooms(t))
	}
}

func testJoinOutsideScope(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	_, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	stranger := h.seedStudentAccount(t, 40, domain.AccountActive)
	strangerToken := h.studentToken(t, 40)

	foreignTutor := h.joinCall(t, intruder, lesson)
	foreignStudent := h.joinCall(t, strangerToken, lesson)
	otherGroupStudent := h.joinCall(t, h.studentToken(t, 11), lesson)
	unknown := h.joinCall(t, parties.tutorToken, absentLesson)
	malformed := h.joinCall(t, parties.tutorToken, "not-an-identifier")
	anonymous := h.send(t, http.MethodPost, lessonRoute(lesson, "join"), "", nil)

	for name, result := range map[string]response{
		"a foreign tutor": foreignTutor, "a foreign student": foreignStudent, "another student": otherGroupStudent, "an unknown lesson": unknown, "a malformed identifier": malformed,
	} {
		expectRefusal(t, name, result, http.StatusNotFound, "not_found")
	}
	expectRefusal(t, "no token", anonymous, http.StatusUnauthorized, "unauthenticated")
	if stranger == (domain.UserID{}) || h.meetingCount(t, lesson) != 1 {
		t.Fatal("a refused join changed the rooms")
	}
}

func testJoinSharedStudent(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	shared := h.lessonOfSecondTutor(t, second, secondToken, parties.student)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	h.joined(t, secondToken, shared)

	crossFirst := h.joinCall(t, secondToken, lesson)
	crossSecond := h.joinCall(t, parties.tutorToken, shared)
	first := h.joined(t, parties.studentToken, lesson)
	otherRoom := h.joined(t, parties.studentToken, shared)

	expectRefusal(t, "the second tutor on the first lesson", crossFirst, http.StatusNotFound, "not_found")
	expectRefusal(t, "the first tutor on the second lesson", crossSecond, http.StatusNotFound, "not_found")
	if first.URL == otherRoom.URL || h.participantOf(t, lesson, parties.student) == h.participantOf(t, shared, parties.student) {
		t.Fatal("the shared student has one identity across two tutors")
	}
	if err := h.enter(t, first, h.currentRoom(t, shared), shared, parties.student, domain.ParticipantViewer); err == nil {
		t.Fatal("a link of one tutor entered the room of another")
	}
}

func (h *authHarness) lessonOfSecondTutor(t *testing.T, second domain.UserID, token string, student domain.UserID) string {
	t.Helper()
	var card string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT id::text FROM relationships WHERE tutor_id = $1::uuid AND student_id = $2::uuid", second.String(), student.String()).Scan(&card)
	if err != nil {
		t.Fatal(err)
	}
	return h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
}

func testJoinEndedRelationship(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	h.joined(t, parties.studentToken, lesson)
	if result := h.cardCall(t, parties.tutorToken, http.MethodPost, parties.card, "/end", ""); result.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", result.status, result.body)
	}

	refused := h.joinCall(t, parties.studentToken, lesson)
	visible := h.lessonPage(t, parties.studentToken, lesson)
	tutorStays := h.joinCall(t, parties.tutorToken, lesson)

	expectRefusal(t, "a student of an ended relationship", refused, http.StatusNotFound, "not_found")
	if visible.status != http.StatusOK || tutorStays.status != http.StatusOK {
		t.Fatalf("roster visibility %d, the tutor %d", visible.status, tutorStays.status)
	}
}

func testJoinProvisional(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 20, domain.RelationshipProvisional)
	lesson := h.lessonAt(t, tutorToken, card, firstLessonDate, firstLessonLocal)
	studentToken := h.studentToken(t, 20)
	h.joined(t, tutorToken, lesson)

	answer := h.joined(t, studentToken, lesson)

	if err := h.enter(t, answer, h.currentRoom(t, lesson), lesson, domain.UserID{20}, domain.ParticipantViewer); err != nil {
		t.Fatalf("the provisional student does not enter: %v", err)
	}
}

func testJoinReclaimPending(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 20, domain.RelationshipProvisional)
	lesson := h.lessonAt(t, tutorToken, card, firstLessonDate, firstLessonLocal)
	studentToken := h.studentToken(t, 20)
	h.joined(t, tutorToken, lesson)
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", card)

	refused := h.joinCall(t, studentToken, lesson)

	expectRefusal(t, "a reclaim-pending student", refused, http.StatusNotFound, "not_found")
	if slots := h.slots(t, domain.PurposeMeetingJoin, domain.UserID{20}, domain.MeetingJoinWindow); slots != 0 {
		t.Fatalf("the refused join used %d slots", slots)
	}
}

func testJoinGroupRoster(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := groupLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	late := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)
	lateToken := h.studentToken(t, 12)
	notMember := h.joinCall(t, lateToken, lesson)
	h.changeGroupMembers(t, parties.tutorToken, parties.group, []string{late}, []string{parties.other})

	joined := h.joinCall(t, lateToken, lesson)
	removed := h.joinCall(t, h.studentToken(t, 11), lesson)
	removedVisible := h.lessonPage(t, h.studentToken(t, 11), lesson)

	expectRefusal(t, "a student outside the group", notMember, http.StatusNotFound, "not_found")
	if joined.status != http.StatusOK || !slices.Contains(h.rosterOf(t, lesson), domain.UserID{12}) {
		t.Fatalf("the late member joined with %d, roster %v", joined.status, h.rosterOf(t, lesson))
	}
	expectRefusal(t, "a removed member", removed, http.StatusNotFound, "not_found")
	if removedVisible.status != http.StatusOK {
		t.Fatalf("the historical roster page answered %d", removedVisible.status)
	}
}

func (h *authHarness) changeGroupMembers(t *testing.T, token string, group string, add []string, remove []string) {
	t.Helper()
	result := h.groupCall(t, token, http.MethodPatch, group, "", fmt.Sprintf(`{"add":%s,"remove":%s}`, idList(add), idList(remove)))
	if result.status != http.StatusOK {
		t.Fatalf("changing the members answered %d %s", result.status, result.body)
	}
}

func testJoinAbsentRoom(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	first := h.joined(t, parties.tutorToken, lesson)
	room := h.currentRoom(t, lesson)
	if err := h.provider.EndMeeting(context.Background(), room); err != nil {
		t.Fatal(err)
	}

	refused := h.joinCall(t, parties.studentToken, lesson)
	view := h.roomLesson(t, parties.studentToken, lesson)
	second := h.joined(t, parties.tutorToken, lesson)

	expectRefusal(t, "a student of an absent room", refused, http.StatusConflict, "lesson_not_live")
	if view.RoomOpen || view.Status != "live" || h.closeReason(t, lesson) != string(domain.MeetingCloseProviderNotFound) {
		t.Fatalf("the lesson %+v closed with %s", view, h.closeReason(t, lesson))
	}
	if first.URL == second.URL || h.currentRoom(t, lesson).ID == room.ID {
		t.Fatal("the tutor rejoin kept the absent room")
	}
}

func testJoinProviderAndCapacity(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	unavailable := h.joinCall(t, parties.tutorToken, lesson)
	state := h.lessonState(t, lesson)
	if err := h.provider.SimulateUnavailable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	h.at(t, h.now(t).Add(domain.MeetingUncertainPause))
	h.exec(t, "UPDATE meeting_recovery_capacity SET unsettled = 4096 WHERE $1::text = 'full'", "full")
	full := h.joinCall(t, parties.tutorToken, lesson)
	rows := h.meetingCount(t, lesson)

	expectRefusal(t, "an unavailable provider", unavailable, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "full admission", full, http.StatusServiceUnavailable, "service_unavailable")
	if state != "scheduled" || rows != 1 || h.providerRooms(t) != 0 {
		t.Fatalf("lesson %s, %d rows, %d rooms", state, rows, h.providerRooms(t))
	}
}

func testJoinProviderDownOnOpenRoom(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	student := h.joinCall(t, parties.studentToken, lesson)
	tutor := h.joinCall(t, parties.tutorToken, lesson)
	view := h.roomLesson(t, parties.studentToken, lesson)
	if err := h.provider.SimulateUnavailable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	recovered := h.joinCall(t, parties.studentToken, lesson)

	expectRefusal(t, "a student during an outage", student, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "a tutor during an outage", tutor, http.StatusServiceUnavailable, "provider_unavailable")
	if !view.RoomOpen || h.currentRoom(t, lesson) != room || h.meetingCount(t, lesson) != 1 || recovered.status != http.StatusOK {
		t.Fatalf("the outage changed the room: %+v rows=%d recovered=%d", view, h.meetingCount(t, lesson), recovered.status)
	}
}

func testJoinTutorLimit(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.seedTutorObligations(t, lesson, domain.MeetingUnsettledPerTutor)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	other := h.lessonOfSecondTutor(t, second, secondToken, parties.student)
	reserved := h.unsettled(t)

	refused := h.joinCall(t, parties.tutorToken, lesson)
	afterRefusal := h.unsettled(t)
	unaffected := h.joinCall(t, secondToken, other)

	expectRefusal(t, "a tutor at the limit", refused, http.StatusServiceUnavailable, "service_unavailable")
	if afterRefusal != reserved || h.lessonState(t, lesson) != "scheduled" || h.providerRooms(t) != 1 {
		t.Fatalf("the refusal reserved %d from %d, lesson %s, %d rooms", afterRefusal, reserved, h.lessonState(t, lesson), h.providerRooms(t))
	}
	if unaffected.status != http.StatusOK || h.unsettled(t) != reserved+1 {
		t.Fatalf("another tutor answered %d with %d unsettled", unaffected.status, h.unsettled(t))
	}
}

func testJoinTutorLimitRace(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	first := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:00")
	second := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:01")
	h.seedTutorObligations(t, first, domain.MeetingUnsettledPerTutor-1)
	reserved := h.unsettled(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM meeting_recovery_capacity WHERE singleton AND $1::text = 'capacity' FOR UPDATE", "capacity")
	firstJoin := h.inBackground(first, "join", parties.tutorToken)
	secondJoin := h.inBackground(second, "join", parties.tutorToken)
	h.waitForLockWaiters(t, 2)

	release()
	answers := []response{awaitAnswer(t, firstJoin), awaitAnswer(t, secondJoin)}

	admitted, refused := answers[0], answers[1]
	if admitted.status != http.StatusOK {
		admitted, refused = answers[1], answers[0]
	}
	if admitted.status != http.StatusOK {
		t.Fatalf("no join was admitted: %d %s and %d %s", answers[0].status, answers[0].body, answers[1].status, answers[1].body)
	}
	expectRefusal(t, "the join behind the last slot", refused, http.StatusServiceUnavailable, "service_unavailable")
	if h.unsettled(t) != reserved+1 || h.providerRooms(t) != 1 {
		t.Fatalf("the race left %d unsettled from %d, %d rooms", h.unsettled(t), reserved, h.providerRooms(t))
	}
}

func testJoinUncertainPause(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	other := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:03")
	if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	failed := h.joinCall(t, parties.tutorToken, lesson)
	if err := h.provider.SimulateUnavailable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	rows, reserved := h.meetingCount(t, lesson), h.unsettled(t)

	paused := h.joinCall(t, parties.tutorToken, lesson)
	rowsWhilePaused, reservedWhilePaused := h.meetingCount(t, lesson), h.unsettled(t)
	otherLesson := h.joinCall(t, parties.tutorToken, other)
	h.at(t, h.now(t).Add(domain.MeetingUncertainPause))
	afterPause := h.joinCall(t, parties.tutorToken, lesson)

	expectRefusal(t, "the failed join", failed, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "the paused join", paused, http.StatusServiceUnavailable, "provider_unavailable")
	if rowsWhilePaused != rows || reservedWhilePaused != reserved {
		t.Fatalf("the paused join wrote: %d rows from %d, %d reserved from %d", rowsWhilePaused, rows, reservedWhilePaused, reserved)
	}
	if otherLesson.status != http.StatusOK || afterPause.status != http.StatusOK {
		t.Fatalf("another lesson answered %d and the later join %d", otherLesson.status, afterPause.status)
	}
}

func testJoinNeutralRoomName(t *testing.T) {
	t.Parallel()
	for name, title := range map[string]string{"a distinctive title": "A distinctive lesson title", "a blank title": "   "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newMeetingHarness(t)
			parties := h.visibilityParties(t)
			request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal)
			request.AllowOverlap, request.Title = true, title
			lesson := h.scheduled(t, parties.tutorToken, request).Lessons[0].ID

			answer := h.joinCall(t, parties.tutorToken, lesson)

			if answer.status != http.StatusOK || h.uncertainRows(t) != 0 {
				t.Fatalf("join answered %d %s with %d uncertain", answer.status, answer.body, h.uncertainRows(t))
			}
			if h.countRows(t, "SELECT count(id) FROM fake_meeting_rooms WHERE name = $1", domain.MeetingRoomName) != 1 {
				t.Fatal("the room does not carry the neutral name")
			}
		})
	}
}

func testJoinRequestBudget(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	waiting := oneToOneLesson(t, h, parties)
	short := h.instance(t, func(dependencies *meetings.LifecycleDependencies) { dependencies.Budget = 200 * time.Millisecond })
	tooShortForCreate := h.instance(t, func(dependencies *meetings.LifecycleDependencies) {
		dependencies.Budget = meetings.DefaultRoomTiming().ProviderCall - time.Second
	})
	before := h.unsettled(t)
	release := h.holdProvider(t)

	restart, restartErr := h.trySendTo(short, http.MethodPost, lessonRoute(lesson, "restart-room"), parties.tutorToken)
	release()
	refusedCreate, createErr := h.trySendTo(tooShortForCreate, http.MethodPost, lessonRoute(waiting, "join"), parties.tutorToken)

	if err := errors.Join(restartErr, createErr); err != nil {
		t.Fatal(err)
	}
	expectRefusal(t, "a restart past its budget", restart, http.StatusServiceUnavailable, "service_unavailable")
	expectRefusal(t, "a budget too short for Create", refusedCreate, http.StatusServiceUnavailable, "provider_unavailable")
	if h.meetingCount(t, waiting) != 0 || h.unsettled(t) != before || h.meetingCount(t, lesson) != 1 {
		t.Fatalf("budget refusals: %d rows, %d unsettled from %d", h.meetingCount(t, waiting), h.unsettled(t), before)
	}
}

func testJoinDisabledDeployment(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	disabled := h.disabledInstance(t)
	before := h.countRows(t, "SELECT count(count) FROM rate_limit_counters WHERE purpose = $1", string(domain.PurposeLessonWrites))

	join, joinErr := h.trySendTo(disabled, http.MethodPost, lessonRoute(lesson, "join"), parties.tutorToken)
	end, endErr := h.trySendTo(disabled, http.MethodPost, lessonRoute(lesson, "end"), parties.tutorToken)
	restart, restartErr := h.trySendTo(disabled, http.MethodPost, lessonRoute(lesson, "restart-room"), parties.tutorToken)
	page, pageErr := h.trySendTo(disabled, http.MethodGet, lessonsRoute+"/"+lesson, parties.tutorToken)
	hint, hintErr := h.trySendTo(disabled, http.MethodGet, "/api/v1/integrations/bbb/not-a-key/ended", "")

	if err := errors.Join(joinErr, endErr, restartErr, pageErr, hintErr); err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]response{"join": join, "end": end, "restart": restart} {
		expectRefusal(t, name, result, http.StatusServiceUnavailable, "service_unavailable")
	}
	if page.status != http.StatusOK || hint.status != http.StatusOK {
		t.Fatalf("lesson page %d, hint %d", page.status, hint.status)
	}
	after := h.countRows(t, "SELECT count(count) FROM rate_limit_counters WHERE purpose = $1", string(domain.PurposeLessonWrites))
	if h.meetingCount(t, lesson) != 0 || h.providerRooms(t) != 0 || h.lessonState(t, lesson) != "scheduled" || before != after {
		t.Fatal("a disabled deployment changed state")
	}
}

func testJoinCredentialPrivacy(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	tutorLink := h.joined(t, parties.tutorToken, lesson)
	studentLink := h.joined(t, parties.studentToken, lesson)
	room := h.currentRoom(t, lesson)

	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM outbox WHERE strpos(payload::text, $1) > 0", tutorLink.URL) != 0 {
		t.Fatal("the outbox holds a join link")
	}
	if !strings.Contains(h.logs.String(), `"msg":"meeting.join.authorized"`) {
		t.Fatal("the authorized join was not reported")
	}
	for _, private := range []string{tutorLink.URL, studentLink.URL, room.ID.String(), lessonTitle, "Member account 10", "participant_id"} {
		if strings.Contains(h.logs.String(), private) {
			t.Fatalf("the logs hold %q", private)
		}
	}
}

func TestMeetingJoinSessionRaces(t *testing.T) {
	t.Run("logout during Create keeps cleanup and answers unauthenticated", testRaceLogoutDuringCreate)
	t.Run("logout during the link call discards the link", testRaceLogoutDuringLink)
	t.Run("a block during the link call discards the student link", testRaceBlockDuringLink)
	t.Run("purge during Create still ends the late room", testRacePurgeDuringCreate)
	t.Run("a removed member loses the link released after the provider call", testRaceRemovedDuringLink)
	t.Run("aborted joins leave no uncertain obligation", testRaceClientAborts)
}

func testRaceLogoutDuringCreate(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	join := h.inBackground(lesson, "join", parties.tutorToken)
	awaitClosed(t, reached)

	logout := h.send(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + parties.tutorToken})
	release()
	answer := awaitAnswer(t, join)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if logout.status != http.StatusNoContent {
		t.Fatalf("logout answered %d", logout.status)
	}
	expectRefusal(t, "a join after logout", answer, http.StatusUnauthorized, "unauthenticated")
	if h.lessonState(t, lesson) != "scheduled" || h.closeReason(t, lesson) != string(domain.MeetingCloseProviderError) {
		t.Fatalf("the lesson is %s and its room %s", h.lessonState(t, lesson), h.closeReason(t, lesson))
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the refused generation stayed: %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func testRaceLogoutDuringLink(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	reached, release := h.linkGate.arm(t)
	join := h.inBackground(lesson, "join", parties.studentToken)
	awaitClosed(t, reached)

	h.send(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + parties.studentToken})
	release()
	answer := awaitAnswer(t, join)

	expectRefusal(t, "a student join after logout", answer, http.StatusUnauthorized, "unauthenticated")
	if strings.Contains(answer.body, "url") {
		t.Fatal("the refusal released a link")
	}
}

func testRaceBlockDuringLink(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	reached, release := h.linkGate.arm(t)
	join := h.inBackground(lesson, "join", parties.studentToken)
	awaitClosed(t, reached)

	if _, err := h.store.Repositories().Users.Block(context.Background(), parties.student, h.now(t)); err != nil {
		t.Fatal(err)
	}
	release()
	answer := awaitAnswer(t, join)

	if answer.status == http.StatusOK {
		t.Fatalf("a blocked account received a link: %s", answer.body)
	}
	expectRefusal(t, "a blocked student", answer, http.StatusUnauthorized, "unauthenticated")
}

func testRacePurgeDuringCreate(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	join := h.inBackground(lesson, "join", parties.tutorToken)
	awaitClosed(t, reached)
	identity, err := h.store.Repositories().Meetings.Current(context.Background(), lessonUUID(t, lesson))
	if err != nil {
		t.Fatal(err)
	}
	if deleted := h.deleteMe(t, parties.tutorToken, accountPassword); deleted.status != http.StatusNoContent {
		t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
	}
	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)
	lessonsAfterPurge := h.lessonRows(t, parties.tutor)

	release()
	answer := awaitAnswer(t, join)
	orphan, err := h.store.Repositories().MeetingRecovery.ByProviderID(context.Background(), identity.ProviderMeetingID)
	if err != nil {
		t.Fatal(err)
	}
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	expectRefusal(t, "a join of a purged tutor", answer, http.StatusUnauthorized, "unauthenticated")
	if lessonsAfterPurge != 0 || orphan.MeetingID != nil || orphan.CreateState != domain.MeetingCreateStarted || orphan.Settlement == nil || *orphan.Settlement != domain.MeetingRecoverySettledQuietAbsence {
		t.Fatalf("%d lessons survived, the orphan ledger: %+v", lessonsAfterPurge, orphan)
	}
	if !strings.Contains(h.logs.String(), `"msg":"meeting.room.created_after_settlement","ended":true`) {
		t.Fatal("the Create returning after the purge-time settlement was not ended")
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the orphaned generation stayed: %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func testRaceRemovedDuringLink(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := groupLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	otherToken := h.studentToken(t, 11)
	reached, release := h.linkGate.arm(t)
	join := h.inBackground(lesson, "join", otherToken)
	awaitClosed(t, reached)

	h.changeGroupMembers(t, parties.tutorToken, parties.group, nil, []string{parties.other})
	release()
	answer := awaitAnswer(t, join)

	expectRefusal(t, "a member removed during the link call", answer, http.StatusNotFound, "not_found")
}

func (h *authHarness) abortedJoin(t *testing.T, lesson string, token string) {
	t.Helper()
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.server.URL+lessonRoute(lesson, "join"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range jsonCall(token) {
		request.Header.Set(name, value)
	}
	sent := make(chan error, 1)
	go func() {
		answered, err := http.DefaultClient.Do(request)
		if err == nil {
			err = answered.Body.Close()
		}
		sent <- err
	}()
	awaitClosed(t, reached)
	cancel()
	if err := <-sent; !errors.Is(err, context.Canceled) {
		t.Fatalf("the aborted request answered %v", err)
	}
	release()
}

func testRaceClientAborts(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lessons := make([]string, 0, 3)
	for _, start := range []string{"18:00", "18:02", "18:04"} {
		lessons = append(lessons, h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, start))
	}

	for _, lesson := range lessons {
		before := h.countLogs(`"msg":"meeting.join.`)
		h.abortedJoin(t, lesson, parties.tutorToken)
		h.awaitLogs(t, before+1, `"msg":"meeting.join.`)
	}
	h.at(t, h.now(t).Add(2*time.Minute))
	h.tick(t)
	h.at(t, h.now(t).Add(2*time.Minute))
	h.tick(t)

	openRooms := h.countRows(t, "SELECT count(id) FROM meetings WHERE state = 'open' AND $1::text = 'open'", "open")
	if h.uncertainRows(t) != 0 || h.unsettled(t) != openRooms || h.providerRooms(t) != openRooms {
		t.Fatalf("aborts left %d uncertain, %d unsettled, %d rooms, %d open", h.uncertainRows(t), h.unsettled(t), h.providerRooms(t), openRooms)
	}
}

func TestMeetingCloseAfterClockStepBack(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	joinedAt := h.now(t)
	h.at(t, joinedAt.Add(-time.Hour))
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))

	end := h.endCall(t, token, lesson)
	h.tick(t)
	closedBeforeRestore := h.closeReason(t, lesson)
	cycles := h.cycleCount(t, room.ID.String())
	h.at(t, joinedAt.Add(time.Minute))
	h.tick(t)

	if end.status != http.StatusOK || decodeLesson(t, end).Status != "ended" {
		t.Fatalf("end answered %d %s", end.status, end.body)
	}
	if closedBeforeRestore != string(domain.MeetingCloseTutorEnd) || cycles != 1 {
		t.Fatalf("the room stayed %s with %d cycles", closedBeforeRestore, cycles)
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the stepped-back close left %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func TestMeetingConcurrentJoin(t *testing.T) {
	t.Run("duplicate tutor requests share one room across instances", testConcurrentTutorJoins)
	t.Run("restart and restart leave one usable room", testConcurrentRestartRestart)
	t.Run("a later restart invalidates earlier restart links", testSequentialRestarts)
	t.Run("a second restart during the replacement Create shares it", testRestartDuringReplacement)
	t.Run("restart waits and a join behind it reuses the replacement", testConcurrentRestartJoin)
	t.Run("a join paused on its link never releases the replaced generation", testConcurrentJoinThenRestart)
	t.Run("a link issued before a restart is discarded", testConcurrentLinkBeforeRestart)
	t.Run("end during Create never reopens", testConcurrentEndDuringCreate)
	t.Run("join after end is refused", testConcurrentJoinAfterEnd)
	t.Run("end during the link call discards the link", testConcurrentEndDuringLink)
	t.Run("end while restart waits on the provider publishes nothing", testConcurrentEndDuringRestart)
	t.Run("end during the replacement Create owns the late result", testConcurrentEndDuringReplacement)
	t.Run("restart after end is refused", testConcurrentRestartAfterEnd)
}

func testConcurrentTutorJoins(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	second := h.secondInstance(t, 16)
	const callers = 6
	answers := make(chan pendingAnswer, callers)

	for index := range callers {
		go func() {
			if index%2 == 0 {
				result, err := h.trySend(http.MethodPost, lessonRoute(lesson, "join"), "", jsonCall(parties.tutorToken))
				answers <- pendingAnswer{result: result, err: err}
				return
			}
			result, err := h.trySendTo(second, http.MethodPost, lessonRoute(lesson, "join"), parties.tutorToken)
			answers <- pendingAnswer{result: result, err: err}
		}()
	}
	links := make([]joinAnswer, 0, callers)
	for range callers {
		answer := <-answers
		if answer.err != nil {
			t.Fatal(answer.err)
		}
		links = append(links, decodeJoin(t, answer.result))
	}

	room := h.currentRoom(t, lesson)
	if h.meetingCount(t, lesson) != 1 || h.providerRooms(t) != 1 || h.unsettled(t) != 1 {
		t.Fatalf("%d meetings, %d rooms, %d unsettled", h.meetingCount(t, lesson), h.providerRooms(t), h.unsettled(t))
	}
	for _, link := range links {
		if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
			t.Fatalf("a duplicate join link does not enter the one room: %v", err)
		}
	}
	if h.countRows(t, "SELECT count(account_id) FROM lesson_participants WHERE lesson_id = $1::uuid", lesson) != 1 {
		t.Fatal("duplicate joins made several participant identities")
	}
}

func (h *authHarness) liveWithRoom(t *testing.T, parties visibilityParties) (string, joinAnswer, domain.Meeting) {
	t.Helper()
	lesson := oneToOneLesson(t, h, parties)
	answer := h.joined(t, parties.tutorToken, lesson)
	return lesson, answer, h.currentRoom(t, lesson)
}

func (h *authHarness) holdProvider(t *testing.T) func() {
	t.Helper()
	return h.holdRow(t, providerStateLock, "state")
}

func testConcurrentRestartRestart(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, old, oldRoom := h.liveWithRoom(t, parties)
	release := h.holdProvider(t)
	first := h.inBackground(lesson, "restart-room", parties.tutorToken)
	h.waitForLockWaiters(t, 1)
	second := h.inBackground(lesson, "restart-room", parties.tutorToken)
	h.waitForLockWaiters(t, 2)

	release()
	firstLink, secondLink := decodeJoin(t, awaitAnswer(t, first)), decodeJoin(t, awaitAnswer(t, second))

	room := h.currentRoom(t, lesson)
	if room.ID == oldRoom.ID || h.providerRooms(t) != 1 || h.meetingCount(t, lesson) != 2 {
		t.Fatalf("simultaneous restarts left %d rooms, %d rows", h.providerRooms(t), h.meetingCount(t, lesson))
	}
	for _, link := range []joinAnswer{firstLink, secondLink} {
		if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
			t.Fatalf("a restart link does not enter the one room: %v", err)
		}
	}
	if err := h.enter(t, old, oldRoom, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the old link still enters")
	}
}

func testSequentialRestarts(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	earlier := decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))
	earlierRoom := h.currentRoom(t, lesson)

	later := decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))

	room := h.currentRoom(t, lesson)
	if err := h.enter(t, later, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
		t.Fatalf("a later restart does not enter: %v", err)
	}
	if err := h.enter(t, earlier, earlierRoom, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the link of the earlier restart survived the later restart")
	}
}

func testRestartDuringReplacement(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	first := h.inBackground(lesson, "restart-room", parties.tutorToken)
	awaitClosed(t, reached)
	requests := h.countLogs(`"msg":"meeting.room.restart_requested"`, `"closed":false`)

	second := h.inBackground(lesson, "restart-room", parties.tutorToken)
	h.awaitLogs(t, requests+1, `"msg":"meeting.room.restart_requested"`, `"closed":false`)
	release()
	firstLink, secondLink := decodeJoin(t, awaitAnswer(t, first)), decodeJoin(t, awaitAnswer(t, second))

	room := h.currentRoom(t, lesson)
	if h.providerRooms(t) != 1 || h.meetingCount(t, lesson) != 2 {
		t.Fatalf("the second restart left %d rooms, %d rows", h.providerRooms(t), h.meetingCount(t, lesson))
	}
	if h.countLogs(`"msg":"meeting.room.restart_requested"`, `"closed":true`) != 1 || h.countLogs(`"msg":"meeting.room.restart_requested"`, `"closed":false`) != requests+1 {
		t.Fatalf("the restarts were not reported with what they closed:\n%s", h.logs.String())
	}
	for _, link := range []joinAnswer{firstLink, secondLink} {
		if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
			t.Fatalf("a link does not enter the shared replacement: %v", err)
		}
	}
}

func testConcurrentRestartJoin(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	release := h.holdProvider(t)
	restart := h.inBackground(lesson, "restart-room", parties.tutorToken)
	h.waitForLockWaiters(t, 1)

	student := h.joinCall(t, parties.studentToken, lesson)
	join := h.inBackground(lesson, "join", parties.tutorToken)
	h.waitForLockWaiters(t, 2)
	release()
	restarted, joined := decodeJoin(t, awaitAnswer(t, restart)), decodeJoin(t, awaitAnswer(t, join))

	expectRefusal(t, "a student during the revocation", student, http.StatusConflict, "lesson_not_live")
	room := h.currentRoom(t, lesson)
	if h.providerRooms(t) != 1 {
		t.Fatalf("restart and join left %d rooms", h.providerRooms(t))
	}
	for _, link := range []joinAnswer{restarted, joined} {
		if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
			t.Fatalf("a link does not enter the one room: %v", err)
		}
	}
}

func testConcurrentJoinThenRestart(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, oldRoom := h.liveWithRoom(t, parties)
	gated, gate := h.gatedInstance(t)
	reached, release := gate.arm(t)
	stale := make(chan pendingAnswer, 1)
	go func() {
		result, err := h.trySendTo(gated, http.MethodPost, lessonRoute(lesson, "join"), parties.studentToken)
		stale <- pendingAnswer{result: result, err: err}
	}()
	awaitClosed(t, reached)

	replacement := decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))
	release()
	answer := awaitAnswer(t, stale)

	room := h.currentRoom(t, lesson)
	if room.ID == oldRoom.ID {
		t.Fatal("the restart kept the room")
	}
	if err := h.enter(t, replacement, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
		t.Fatalf("the replacement link: %v", err)
	}
	released := decodeJoin(t, answer)
	if err := h.enter(t, released, room, lesson, parties.student, domain.ParticipantViewer); err != nil {
		t.Fatalf("the student was released a link of a stale generation: %v", err)
	}
}

func testConcurrentLinkBeforeRestart(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, oldRoom := h.liveWithRoom(t, parties)
	issued, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { h.releaseOnce(release) })
	barrier := slog.New(slog.NewJSONHandler(&linkIssuedBarrier{logs: h.logs, issued: issued, release: release}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	gated := h.instanceLogging(t, barrier)
	stale := make(chan pendingAnswer, 1)
	go func() {
		result, err := h.trySendTo(gated, http.MethodPost, lessonRoute(lesson, "join"), parties.studentToken)
		stale <- pendingAnswer{result: result, err: err}
	}()
	awaitClosed(t, issued)
	links := h.countRows(t, "SELECT count(digest) FROM fake_meeting_links WHERE room_id = $1::uuid", oldRoom.ID.String())

	decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))
	h.releaseOnce(release)
	answer := awaitAnswer(t, stale)

	if links != 2 {
		t.Fatalf("the paused call issued %d links", links)
	}
	released := decodeJoin(t, answer)
	if err := h.enter(t, released, h.currentRoom(t, lesson), lesson, parties.student, domain.ParticipantViewer); err != nil {
		t.Fatalf("the student received a link of the replaced generation: %v", err)
	}
}

func (h *authHarness) reopenedLiveWithoutRoom(t *testing.T, parties visibilityParties) string {
	t.Helper()
	lesson, _, room := h.liveWithRoom(t, parties)
	if err := h.provider.EndMeeting(context.Background(), room); err != nil {
		t.Fatal(err)
	}
	expectRefusal(t, "a student of the absent room", h.joinCall(t, parties.studentToken, lesson), http.StatusConflict, "lesson_not_live")
	return lesson
}

func testConcurrentEndDuringCreate(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := h.reopenedLiveWithoutRoom(t, parties)
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	join := h.inBackground(lesson, "join", parties.tutorToken)
	awaitClosed(t, reached)

	end := h.endCall(t, parties.tutorToken, lesson)
	release()
	answer := awaitAnswer(t, join)
	view := h.roomLesson(t, parties.tutorToken, lesson)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if end.status != http.StatusOK {
		t.Fatalf("end answered %d %s", end.status, end.body)
	}
	expectRefusal(t, "a join ended meanwhile", answer, http.StatusConflict, "outside_join_window")
	if view.Status != "ended" || view.RoomOpen {
		t.Fatalf("the lesson reopened: %+v", view)
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the late generation stayed: %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func testConcurrentJoinAfterEnd(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	if result := h.endCall(t, parties.tutorToken, lesson); result.status != http.StatusOK {
		t.Fatalf("end answered %d", result.status)
	}

	tutor := h.joinCall(t, parties.tutorToken, lesson)
	student := h.joinCall(t, parties.studentToken, lesson)

	expectRefusal(t, "a tutor after end", tutor, http.StatusConflict, "outside_join_window")
	expectRefusal(t, "a student after end", student, http.StatusConflict, "lesson_not_live")
}

func testConcurrentEndDuringLink(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	reached, release := h.linkGate.arm(t)
	join := h.inBackground(lesson, "join", parties.tutorToken)
	awaitClosed(t, reached)

	end := h.endCall(t, parties.tutorToken, lesson)
	release()
	answer := awaitAnswer(t, join)

	if end.status != http.StatusOK {
		t.Fatalf("end answered %d", end.status)
	}
	expectRefusal(t, "a link released after end", answer, http.StatusConflict, "outside_join_window")
}

func testConcurrentEndDuringRestart(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, oldRoom := h.liveWithRoom(t, parties)
	release := h.holdProvider(t)
	restart := h.inBackground(lesson, "restart-room", parties.tutorToken)
	h.waitForLockWaiters(t, 1)

	end := h.endCall(t, parties.tutorToken, lesson)
	release()
	answer := awaitAnswer(t, restart)

	if end.status != http.StatusOK {
		t.Fatalf("end answered %d %s", end.status, end.body)
	}
	expectRefusal(t, "a restart ended meanwhile", answer, http.StatusConflict, "lesson_not_live")
	if h.meetingCount(t, lesson) != 1 || h.providerRooms(t) != 0 {
		t.Fatalf("the restart replaced the room of %s: %d rows, %d rooms", oldRoom.ID, h.meetingCount(t, lesson), h.providerRooms(t))
	}
}

func testConcurrentEndDuringReplacement(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	reached, release := h.createGate.arm()
	t.Cleanup(release)
	restart := h.inBackground(lesson, "restart-room", parties.tutorToken)
	awaitClosed(t, reached)

	end := h.endCall(t, parties.tutorToken, lesson)
	release()
	answer := awaitAnswer(t, restart)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if end.status != http.StatusOK {
		t.Fatalf("end answered %d", end.status)
	}
	expectRefusal(t, "a restart whose replacement lost", answer, http.StatusConflict, "lesson_not_live")
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the refused late result stayed: %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func testConcurrentRestartAfterEnd(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	if result := h.endCall(t, parties.tutorToken, lesson); result.status != http.StatusOK {
		t.Fatalf("end answered %d", result.status)
	}

	restart := h.restartCall(t, parties.tutorToken, lesson)

	expectRefusal(t, "a restart after end", restart, http.StatusConflict, "lesson_not_live")
}

func TestMeetingRestartRevocation(t *testing.T) {
	t.Run("the old link stops working and the fresh one enters", testRestartRevokes)
	t.Run("a member removed from the group loses the old room and gets none of the replacement", testRestartRevokesRemovedMember)
	t.Run("an unconfirmed End publishes no replacement", testRestartUnavailable)
	t.Run("restart needs a live lesson", testRestartNeedsLive)
	t.Run("restart past the grace keeps the room", testRestartPastGrace)
}

func testRestartRevokes(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, old, oldRoom := h.liveWithRoom(t, parties)
	oldStudent := h.joined(t, parties.studentToken, lesson)
	frames := h.recordFrames(t)

	fresh := decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))
	sent := frames.settle(t, h)
	room := h.currentRoom(t, lesson)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if err := h.enter(t, old, oldRoom, lesson, parties.tutor, domain.ParticipantModerator); !errors.Is(err, meeting.FailureNotFound) {
		t.Fatalf("the old tutor link answered %v", err)
	}
	if err := h.enter(t, oldStudent, oldRoom, lesson, parties.student, domain.ParticipantViewer); !errors.Is(err, meeting.FailureNotFound) {
		t.Fatalf("the old student link answered %v", err)
	}
	if err := h.enter(t, old, room, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the old link entered the replacement")
	}
	if err := h.enter(t, fresh, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
		t.Fatalf("the fresh link: %v", err)
	}
	if room.ID == oldRoom.ID || h.closeReason(t, lesson) != string(domain.MeetingCloseTutorRestart) {
		t.Fatal("the restart did not replace and close the room")
	}
	if !hasFrame(sent, parties.student, domain.EntityLesson, lesson) || !h.roomLesson(t, parties.studentToken, lesson).RoomOpen {
		t.Fatalf("the student did not learn of the replacement: %v", sent)
	}
	if h.unsettled(t) != 1 {
		t.Fatalf("cleanup left %d unsettled", h.unsettled(t))
	}
}

func testRestartRevokesRemovedMember(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := groupLesson(t, h, parties)
	h.joined(t, parties.tutorToken, lesson)
	otherToken := h.studentToken(t, 11)
	oldOther := h.joined(t, otherToken, lesson)
	oldRoom := h.currentRoom(t, lesson)
	if err := h.enter(t, oldOther, oldRoom, lesson, parties.otherStudent, domain.ParticipantViewer); err != nil {
		t.Fatalf("the member did not enter the old room: %v", err)
	}
	participant := h.participantOf(t, lesson, parties.otherStudent)
	h.changeGroupMembers(t, parties.tutorToken, parties.group, nil, []string{parties.other})
	frames := h.recordFrames(t)

	decodeJoin(t, h.restartCall(t, parties.tutorToken, lesson))
	sent := frames.settle(t, h)
	room := h.currentRoom(t, lesson)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)
	rejoin := h.joinCall(t, otherToken, lesson)

	if err := h.enter(t, oldOther, oldRoom, lesson, parties.otherStudent, domain.ParticipantViewer); !errors.Is(err, meeting.FailureNotFound) {
		t.Fatalf("the removed member's old link answered %v", err)
	}
	if err := h.enter(t, oldOther, room, lesson, parties.otherStudent, domain.ParticipantViewer); err == nil {
		t.Fatal("the removed member's old link entered the replacement")
	}
	if hasFrame(sent, parties.otherStudent, domain.EntityLesson, lesson) {
		t.Fatalf("the removed member learned of the replacement: %v", sent)
	}
	expectRefusal(t, "a join of the removed member after the restart", rejoin, http.StatusNotFound, "not_found")
	if h.countRows(t, "SELECT count(room_id) FROM fake_meeting_participants WHERE participant_id = $1::uuid", participant.String()) != 0 {
		t.Fatal("the ended room kept the removed member's participant and name")
	}
}

func testRestartUnavailable(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, old, oldRoom := h.liveWithRoom(t, parties)
	if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	refused := h.restartCall(t, parties.tutorToken, lesson)
	view := h.roomLesson(t, parties.studentToken, lesson)
	student := h.joinCall(t, parties.studentToken, lesson)
	tutor := h.joinCall(t, parties.tutorToken, lesson)
	rowsWhileDown := h.meetingCount(t, lesson)
	if err := h.provider.SimulateUnavailable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	recovered := h.joined(t, parties.tutorToken, lesson)

	expectRefusal(t, "a restart without End", refused, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "a tutor join with the old room unconfirmed", tutor, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "a student without a room", student, http.StatusConflict, "lesson_not_live")
	if view.RoomOpen || rowsWhileDown != 1 {
		t.Fatalf("a replacement appeared: %+v rows=%d", view, rowsWhileDown)
	}
	room := h.currentRoom(t, lesson)
	if room.ID == oldRoom.ID || h.providerRooms(t) != 1 {
		t.Fatalf("after recovery the old room stayed: %d rooms", h.providerRooms(t))
	}
	if err := h.enter(t, old, oldRoom, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the old link still enters after confirmed revocation")
	}
	if err := h.enter(t, recovered, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
		t.Fatalf("the recovered link: %v", err)
	}
}

func testRestartNeedsLive(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)

	refused := h.restartCall(t, parties.tutorToken, lesson)

	expectRefusal(t, "a restart before the lesson is live", refused, http.StatusConflict, "lesson_not_live")
}

func testRestartPastGrace(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	h.at(t, firstLessonEnd.Add(domain.LessonEndGrace))
	tutorToken := accessToken(t, h.login(t, tutorEmail, accountPassword))
	roomsBefore := h.providerRooms(t)

	refused := h.restartCall(t, tutorToken, lesson)

	expectRefusal(t, "a restart past the grace", refused, http.StatusConflict, "outside_join_window")
	if h.currentRoom(t, lesson) != room || h.meetingCount(t, lesson) != 1 || h.providerRooms(t) != roomsBefore {
		t.Fatalf("the refused restart replaced the room: %d meetings, %d provider rooms", h.meetingCount(t, lesson), h.providerRooms(t))
	}
}

func TestMeetingEndAndStatus(t *testing.T) {
	t.Run("end closes access and queues cleanup atomically", testEndClosesAndCleans)
	t.Run("end needs a live lesson and repeats idempotently", testEndIdempotent)
	t.Run("the status job ends a live lesson", testStatusJobEndsLive)
	t.Run("the status job closes a stale creating row with its lesson", testStatusJobClosesCreating)
	t.Run("tutor deletion closes access and queues cleanup", testTutorDeletionCloses)
	t.Run("students never reach end or restart", testEndAudience)
	t.Run("a lesson took place once it has been live", testLessonTookPlace)
	t.Run("a lesson nobody joined did not take place", testLessonDidNotTakePlace)
}

func testLessonTookPlace(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)

	before := h.roomLesson(t, parties.studentToken, lesson)
	h.joined(t, parties.tutorToken, lesson)
	while := h.roomLesson(t, parties.studentToken, lesson)
	ended := h.endCall(t, parties.tutorToken, lesson)
	afterForTutor := h.roomLesson(t, parties.tutorToken, lesson)
	afterForStudent := h.roomLesson(t, parties.studentToken, lesson)

	if before.TookPlace || !while.TookPlace {
		t.Fatalf("before %+v, while live %+v", before, while)
	}
	if ended.status != http.StatusOK || afterForTutor.Status != "ended" || !afterForTutor.TookPlace || !afterForStudent.TookPlace {
		t.Fatalf("end answered %d, tutor %+v, student %+v", ended.status, afterForTutor, afterForStudent)
	}
}

func testLessonDidNotTakePlace(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)

	h.tickAt(t, firstLessonEnd.Add(domain.LessonEndGrace+time.Minute))
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	view := h.roomLesson(t, fresh, lesson)

	if view.Status != "ended" || view.TookPlace {
		t.Fatalf("an unattended lesson: %+v", view)
	}
}

func testEndClosesAndCleans(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, link, room := h.liveWithRoom(t, parties)
	frames := h.recordFrames(t)

	result := h.endCall(t, parties.tutorToken, lesson)
	sent := frames.settle(t, h)
	queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.KindMeetingCleanup))
	enteredBeforeWorker := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator)
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	view := decodeLesson(t, result)
	if view.Status != "ended" || queued != 1 || h.closeReason(t, lesson) != string(domain.MeetingCloseTutorEnd) {
		t.Fatalf("end: %+v queued=%d", view, queued)
	}
	if !hasFrame(sent, parties.student, domain.EntityLesson, lesson) {
		t.Fatalf("no frame for the student: %v", sent)
	}
	if enteredBeforeWorker != nil {
		t.Fatalf("the provider room closed before its cleanup ran: %v", enteredBeforeWorker)
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("cleanup left %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
	if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the link survived cleanup")
	}
}

func testEndIdempotent(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	scheduled := h.endCall(t, parties.tutorToken, lesson)
	h.joined(t, parties.tutorToken, lesson)

	first := h.endCall(t, parties.tutorToken, lesson)
	second := h.endCall(t, parties.tutorToken, lesson)

	expectRefusal(t, "ending a scheduled lesson", scheduled, http.StatusConflict, "lesson_not_live")
	if decodeLesson(t, first).Status != "ended" || decodeLesson(t, second).Status != "ended" {
		t.Fatalf("end answers %d and %d", first.status, second.status)
	}
	if second.headers.Get("Cache-Control") != "no-store" || h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.KindMeetingCleanup)) != 1 {
		t.Fatal("a repeated end queued more cleanup")
	}
}

func testStatusJobEndsLive(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)

	h.tickAt(t, firstLessonEnd.Add(domain.LessonEndGrace-time.Minute))
	beforeGrace := h.lessonState(t, lesson)
	h.tickAt(t, firstLessonEnd.Add(domain.LessonEndGrace))
	afterGrace := h.lessonState(t, lesson)
	h.tickAt(t, firstLessonEnd.Add(domain.LessonEndGrace+time.Minute))

	if beforeGrace != "live" || afterGrace != "ended" || h.closeReason(t, lesson) != string(domain.MeetingCloseStatusJob) {
		t.Fatalf("status: %s then %s, closed %s", beforeGrace, afterGrace, h.closeReason(t, lesson))
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("the job left %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
}

func testStatusJobClosesCreating(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	h.seedRecoveryMeeting(t, lesson)
	h.exec(t, "UPDATE meeting_recovery_capacity SET unsettled = 1 WHERE $1::text = 'seeded'", "seeded")
	h.at(t, firstLessonEnd.Add(domain.LessonEndGrace))

	_, err := h.jobs.run(endDueLessons)(context.Background(), h.now(t))

	if err != nil {
		t.Fatal(err)
	}
	if h.lessonState(t, lesson) != "ended" || h.closeReason(t, lesson) != string(domain.MeetingCloseStatusJob) {
		t.Fatalf("lesson %s, room %s", h.lessonState(t, lesson), h.closeReason(t, lesson))
	}
	if h.cycleCount(t, lesson) != 1 {
		t.Fatalf("the closed room queued %d cycles", h.cycleCount(t, lesson))
	}
}

func testTutorDeletionCloses(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, link, room := h.liveWithRoom(t, parties)
	waiting := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:40")
	h.seedRecoveryMeeting(t, waiting)
	h.exec(t, "UPDATE meeting_recovery_capacity SET unsettled = 2 WHERE $1::text = 'seeded'", "seeded")
	frames := h.recordFrames(t)

	deleted := h.deleteMe(t, parties.tutorToken, accountPassword)
	sent := frames.settle(t, h)
	student := h.joinCall(t, parties.studentToken, lesson)
	ended, closed, waitingClosed := h.lessonState(t, lesson), h.closeReason(t, lesson), h.closeReason(t, waiting)
	cycles := h.cycleCount(t, room.ID.String())
	h.at(t, h.now(t).Add(time.Minute))
	h.tick(t)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("deletion answered %d %s", deleted.status, deleted.body)
	}
	if ended != "ended" || closed != string(domain.MeetingCloseTutorEnd) || waitingClosed != string(domain.MeetingCloseTutorEnd) || cycles != 1 {
		t.Fatalf("lesson %s, room %s, waiting room %s, %d cycles", ended, closed, waitingClosed, cycles)
	}
	if !hasFrame(sent, parties.student, domain.EntityLesson, lesson) {
		t.Fatalf("no frame for the student: %v", sent)
	}
	expectRefusal(t, "a student of a deleted tutor", student, http.StatusNotFound, "not_found")
	if h.countLogs(`"msg":"meeting.closed_for_deletion"`, `"lessons":1`, `"rooms":2`) != 1 {
		t.Fatalf("the deletion was not reported:\n%s", h.logs.String())
	}
	if h.providerRooms(t) != 0 || h.unsettled(t) != 0 {
		t.Fatalf("deletion left %d rooms, %d unsettled", h.providerRooms(t), h.unsettled(t))
	}
	if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err == nil {
		t.Fatal("the link survived the deletion")
	}
}

func testEndAudience(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)

	end := h.endCall(t, parties.studentToken, lesson)
	restart := h.restartCall(t, parties.studentToken, lesson)

	expectRefusal(t, "a student ending", end, http.StatusNotFound, "not_found")
	expectRefusal(t, "a student restarting", restart, http.StatusNotFound, "not_found")
	if h.lessonState(t, lesson) != "live" {
		t.Fatal("a student changed the lesson")
	}
}

func (h *authHarness) hint(t *testing.T, key string) response {
	t.Helper()
	return h.send(t, http.MethodGet, "/api/v1/integrations/bbb/"+key+"/ended", "", nil)
}

func (h *authHarness) routingKey(t *testing.T, lesson string) string {
	t.Helper()
	var key string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT routing_key::text FROM meetings WHERE lesson_id = $1::uuid AND state = 'open'", lesson).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func aForeignEndedHintRevealsNothing(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	parties := h.visibilityParties(t)
	h.at(t, firstLessonStart.Add(-5*time.Minute))
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.joined(t, token, h.lessonAt(t, token, parties.card, firstLessonDate, firstLessonLocal))
	var key string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT routing_key::text FROM meetings").Scan(&key); err != nil {
		t.Fatal(err)
	}
	return h.hint(t, key), h.hint(t, randomCardID)
}

func (h *authHarness) hintRows(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.KindMeetingEndedHint))
}

func TestMeetingEndedHints(t *testing.T) {
	t.Run("unknown and malformed keys reveal nothing", testHintUnknown)
	t.Run("young rooms and rooms without a join are ignored", testHintIgnored)
	t.Run("a forged hint cannot close a room that exists", testHintForged)
	t.Run("confirmed absence closes the room and not the lesson", testHintConfirmed)
	t.Run("a saturated permit pool still answers 200", testHintSaturated)
	t.Run("a held database wait is bounded and still answers 200", testHintWaitBounded)
	t.Run("a hint creates no room and renews no cycle", testHintNoAuthority)
	t.Run("the recording callback stays absent", testHintRecordingAbsent)
}

func testHintUnknown(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)

	unknown := h.hint(t, absentRoutingKey)
	malformed := h.hint(t, "not-a-routing-key")

	if unknown.status != http.StatusOK || malformed.status != http.StatusOK || unknown.body != malformed.body {
		t.Fatalf("hints answered %d %q and %d %q", unknown.status, unknown.body, malformed.status, malformed.body)
	}
	if h.hintRows(t) != 0 || h.meetingCount(t, lesson) != 1 {
		t.Fatal("an unknown hint queued work")
	}
}

func testHintIgnored(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	young := h.hint(t, key)
	youngRows := h.hintRows(t)

	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))
	h.exec(t, "UPDATE meetings SET first_join_at = NULL WHERE lesson_id = $1::uuid", lesson)
	joinless := h.hint(t, key)

	if young.status != http.StatusOK || joinless.status != http.StatusOK || youngRows != 0 || h.hintRows(t) != 0 {
		t.Fatalf("ignored hints: %d %d young=%d rows=%d", young.status, joinless.status, youngRows, h.hintRows(t))
	}
}

func testHintForged(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, link, room := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))

	h.hint(t, key)
	h.hint(t, key)
	debounced := h.hintRows(t)
	for range domain.MeetingHintRechecks + 2 {
		h.tick(t)
		h.at(t, h.now(t).Add(domain.MeetingHintRecheckAfter))
	}
	h.tick(t)

	if debounced != 1 {
		t.Fatalf("a repeated hint queued %d checks", debounced)
	}
	if h.hintRows(t) != domain.MeetingHintRechecks+1 {
		t.Fatalf("the chain made %d checks", h.hintRows(t))
	}
	if h.roomLesson(t, parties.tutorToken, lesson).RoomOpen != true || h.lessonState(t, lesson) != "live" {
		t.Fatal("a forged hint closed the room")
	}
	if err := h.enter(t, link, room, lesson, parties.tutor, domain.ParticipantModerator); err != nil {
		t.Fatalf("the room stopped working: %v", err)
	}
}

func testHintConfirmed(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))
	if err := h.provider.EndMeeting(context.Background(), room); err != nil {
		t.Fatal(err)
	}
	frames := h.recordFrames(t)

	h.hint(t, key)
	h.tick(t)
	sent := frames.settle(t, h)
	view := h.roomLesson(t, parties.tutorToken, lesson)
	rejoined := h.joined(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), lesson)

	if view.RoomOpen || view.Status != "live" || h.closeReason(t, lesson) != string(domain.MeetingCloseProviderNotFound) {
		t.Fatalf("hint result: %+v %s", view, h.closeReason(t, lesson))
	}
	if !hasFrame(sent, parties.student, domain.EntityLesson, lesson) {
		t.Fatalf("no frame for the closed room: %v", sent)
	}
	if rejoined.URL == "" || h.currentRoom(t, lesson).ID == room.ID {
		t.Fatal("the tutor could not rejoin after the room closed")
	}
}

func testHintSaturated(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))
	bounded := h.instance(t, func(dependencies *meetings.LifecycleDependencies) {
		dependencies.HintPermits, dependencies.HintWait = 1, 10*time.Minute
	})
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM meetings WHERE routing_key = $1::uuid FOR UPDATE", key)
	held := make(chan pendingAnswer, 1)
	go func() {
		result, err := h.trySendTo(bounded, http.MethodGet, "/api/v1/integrations/bbb/"+key+"/ended", "")
		held <- pendingAnswer{result: result, err: err}
	}()
	h.waitForLockWaiters(t, 1)

	declined, err := h.trySendTo(bounded, http.MethodGet, "/api/v1/integrations/bbb/"+key+"/ended", "")
	if err != nil {
		t.Fatal(err)
	}
	logsBeforeRelease := h.countLogs(`"msg":"meeting.hint.declined"`)
	release()
	first := <-held

	if first.err != nil || first.result.status != http.StatusOK || declined.status != http.StatusOK {
		t.Fatalf("saturated hints answered %v %d and %d", first.err, first.result.status, declined.status)
	}
	if logsBeforeRelease != 1 || h.hintRows(t) != 1 {
		t.Fatalf("declined=%d rows=%d", logsBeforeRelease, h.hintRows(t))
	}
}

func testHintWaitBounded(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))
	bounded := h.instance(t, func(dependencies *meetings.LifecycleDependencies) { dependencies.HintWait = 50 * time.Millisecond })
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM meetings WHERE routing_key = $1::uuid FOR UPDATE", key)

	answer, err := h.trySendTo(bounded, http.MethodGet, "/api/v1/integrations/bbb/"+key+"/ended", "")
	rowsWhileHeld := h.hintRows(t)
	release()

	if err != nil {
		t.Fatal(err)
	}
	if answer.status != http.StatusOK || rowsWhileHeld != 0 || h.countLogs(`"msg":"meeting.hint.failed"`) != 1 {
		t.Fatalf("a held hint answered %d with %d rows", answer.status, rowsWhileHeld)
	}
}

func testHintNoAuthority(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	key := h.routingKey(t, lesson)
	h.at(t, h.now(t).Add(domain.MeetingHintMinimumAge))
	if result := h.endCall(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), lesson); result.status != http.StatusOK {
		t.Fatalf("end answered %d", result.status)
	}
	h.tick(t)
	cycles := h.countRows(t, "SELECT count(outbox_id) FROM meeting_recovery_cycles WHERE provider_meeting_id = $1::uuid", room.ID.String())

	h.hint(t, key)
	h.tick(t)

	if h.hintRows(t) != 0 || h.providerRooms(t) != 0 || h.meetingCount(t, lesson) != 1 {
		t.Fatal("a hint for a closed room did work")
	}
	if h.countRows(t, "SELECT count(outbox_id) FROM meeting_recovery_cycles WHERE provider_meeting_id = $1::uuid", room.ID.String()) != cycles {
		t.Fatal("a hint renewed a cleanup cycle")
	}
}

func testHintRecordingAbsent(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)

	result := h.send(t, http.MethodPost, "/api/v1/integrations/bbb/00000000-0000-4000-8000-000000000003/recording-ready", "{}", jsonCall(""))

	if result.status != http.StatusNotFound {
		t.Fatalf("the recording callback answered %d", result.status)
	}
}

func TestMeetingParticipantIdentity(t *testing.T) {
	t.Run("identifiers are opaque, stable and unique per lesson", testIdentityOpaque)
	t.Run("no identity exists before authorization", testIdentityAfterAuthorization)
	t.Run("the same account races to one identity", testIdentityRace)
	t.Run("a tutor purge removes its lessons' identities", testIdentityLessonPurge)
	t.Run("a student purge removes the account's identities", testIdentityStudentPurge)
	t.Run("the table refuses duplicates", testIdentityConstraints)
}

func testIdentityOpaque(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	later := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:05")
	h.joined(t, parties.studentToken, lesson)
	first := h.participantOf(t, lesson, parties.student)

	h.joined(t, parties.studentToken, lesson)
	h.joined(t, parties.tutorToken, later)
	h.joined(t, parties.studentToken, later)

	if first != h.participantOf(t, lesson, parties.student) {
		t.Fatal("the identity changed between joins")
	}
	if first == h.participantOf(t, later, parties.student) || first == h.participantOf(t, lesson, parties.tutor) {
		t.Fatal("identities are shared between lessons or accounts")
	}
	if first == domain.ParticipantID(parties.student) {
		t.Fatal("the identity is the account identifier")
	}
	for _, account := range []domain.UserID{parties.student, parties.tutor} {
		if h.countRows(t, "SELECT count(room_id) FROM fake_meeting_participants WHERE participant_id = $1::uuid", account.String()) != 0 {
			t.Fatalf("the provider received the account identifier %s", account)
		}
	}
}

func testIdentityAfterAuthorization(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.seedStudentAccount(t, 40, domain.AccountActive)

	h.joinCall(t, h.studentToken(t, 40), lesson)

	if h.countRows(t, "SELECT count(account_id) FROM lesson_participants WHERE account_id = $1::uuid", domain.UserID{40}.String()) != 0 {
		t.Fatal("a refused account got an identity")
	}
}

func testIdentityRace(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	const callers = 5
	answers := make([]<-chan pendingAnswer, 0, callers)

	for range callers {
		answers = append(answers, h.inBackground(lesson, "join", parties.studentToken))
	}
	for _, answer := range answers {
		decodeJoin(t, awaitAnswer(t, answer))
	}

	if h.countRows(t, "SELECT count(account_id) FROM lesson_participants WHERE account_id = $1::uuid", parties.student.String()) != 1 {
		t.Fatal("concurrent joins made several identities")
	}
}

func testIdentityLessonPurge(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.joined(t, parties.studentToken, lesson)
	h.deleteMe(t, parties.tutorToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.countRows(t, "SELECT count(account_id) FROM lesson_participants WHERE lesson_id = $1::uuid", lesson) != 0 {
		t.Fatal("the identities outlived the lesson")
	}
}

func testIdentityStudentPurge(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.joined(t, parties.studentToken, lesson)
	h.deleteMe(t, parties.studentToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.countRows(t, "SELECT count(account_id) FROM lesson_participants WHERE account_id = $1::uuid", parties.student.String()) != 0 {
		t.Fatal("the identity outlived the account")
	}
}

func testIdentityConstraints(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.joined(t, parties.studentToken, lesson)
	participant := h.participantOf(t, lesson, parties.student)

	_, sameAccount := h.database.Pool.Exec(context.Background(), "INSERT INTO lesson_participants (lesson_id, account_id, participant_id, created_at) VALUES ($1::uuid, $2::uuid, gen_random_uuid(), now())", lesson, parties.student.String())
	_, sameIdentity := h.database.Pool.Exec(context.Background(), "INSERT INTO lesson_participants (lesson_id, account_id, participant_id, created_at) VALUES ($1::uuid, gen_random_uuid(), $2, now())", lesson, participant)

	if sameAccount == nil || sameIdentity == nil {
		t.Fatalf("duplicates were accepted: %v %v", sameAccount, sameIdentity)
	}
}

func (h *authHarness) slots(t *testing.T, purpose domain.RateLimitPurpose, user domain.UserID, size time.Duration) int64 {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), size)
	var count int64
	err := h.database.Pool.QueryRow(context.Background(), "SELECT coalesce(sum(count), 0)::bigint FROM rate_limit_counters WHERE purpose = $1 AND subject = $2 AND window_start = $3",
		string(purpose), h.keys.ThrottleSubject(string(purpose), user.String()), window).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) fillSlots(t *testing.T, purpose domain.RateLimitPurpose, user domain.UserID, size time.Duration, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), size)
	_, err := h.database.Pool.Exec(context.Background(), "INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(purpose), h.keys.ThrottleSubject(string(purpose), user.String()), window, count)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMeetingRateAdmission(t *testing.T) {
	t.Run("the thirtieth attempt passes and the next is refused without a slot", testRateBoundary)
	t.Run("scope and authentication refusals use no slot", testRateFreeRefusals)
	t.Run("refusals after admission keep the charge", testRateChargedRefusals)
	t.Run("the budget is shared across lessons and resets each minute", testRateSharedAndWindow)
	t.Run("a cross-instance race admits exactly the budget", testRateCrossInstance)
	t.Run("end and restart use the tutor's lesson writes", testRateLessonWrites)
}

func testRateBoundary(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.fillSlots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow, domain.MeetingJoinsPerMinute-1)

	allowed := h.joinCall(t, parties.studentToken, lesson)
	refused := h.joinCall(t, parties.studentToken, lesson)

	if allowed.status != http.StatusOK {
		t.Fatalf("the thirtieth attempt answered %d %s", allowed.status, allowed.body)
	}
	expectRefusal(t, "the thirty-first attempt", refused, http.StatusTooManyRequests, "rate_limited")
	if refused.headers.Get("Retry-After") == "" || h.slots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow) != domain.MeetingJoinsPerMinute {
		t.Fatalf("Retry-After %q, slots %d", refused.headers.Get("Retry-After"), h.slots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow))
	}
}

func testRateFreeRefusals(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	h.seedStudentAccount(t, 40, domain.AccountActive)
	stranger := h.studentToken(t, 40)

	h.joinCall(t, stranger, lesson)
	h.joinCall(t, parties.tutorToken, "not-an-identifier")
	h.joinCall(t, parties.tutorToken, absentLesson)
	h.send(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + parties.studentToken})
	revoked := h.joinCall(t, parties.studentToken, lesson)

	expectRefusal(t, "a revoked session", revoked, http.StatusUnauthorized, "unauthenticated")
	for _, user := range []domain.UserID{{40}, parties.student} {
		if count := h.slots(t, domain.PurposeMeetingJoin, user, domain.MeetingJoinWindow); count != 0 {
			t.Fatalf("a free refusal used %d slots of %s", count, user)
		}
	}
	if count := h.slots(t, domain.PurposeMeetingJoin, parties.tutor, domain.MeetingJoinWindow); count != 1 {
		t.Fatalf("the tutor used %d slots, only the join that made the room", count)
	}
}

func testRateChargedRefusals(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	h.at(t, firstLessonStart.Add(-20*time.Minute))
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)

	early := h.joinCall(t, parties.tutorToken, lesson)
	notLive := h.joinCall(t, parties.studentToken, lesson)

	expectRefusal(t, "an early tutor", early, http.StatusConflict, "outside_join_window")
	expectRefusal(t, "an early student", notLive, http.StatusConflict, "lesson_not_live")
	if h.slots(t, domain.PurposeMeetingJoin, parties.tutor, domain.MeetingJoinWindow) != 1 || h.slots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow) != 1 {
		t.Fatal("a refusal after admission lost its charge")
	}
}

func testRateSharedAndWindow(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	other := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:20")
	h.fillSlots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow, domain.MeetingJoinsPerMinute)

	onFirst := h.joinCall(t, parties.studentToken, lesson)
	onOther := h.joinCall(t, parties.studentToken, other)
	window, _ := domain.FixedWindow(h.now(t), domain.MeetingJoinWindow)
	h.at(t, window.Add(domain.MeetingJoinWindow))
	nextMinute := h.joinCall(t, accessToken(t, h.login(t, "member-10@example.org", accountPassword)), lesson)

	expectRefusal(t, "a full budget on the first lesson", onFirst, http.StatusTooManyRequests, "rate_limited")
	expectRefusal(t, "a full budget on another lesson", onOther, http.StatusTooManyRequests, "rate_limited")
	if nextMinute.status != http.StatusOK {
		t.Fatalf("the next minute answered %d %s", nextMinute.status, nextMinute.body)
	}
}

func testRateCrossInstance(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, _ := h.liveWithRoom(t, parties)
	second := h.secondInstance(t, 16)
	const free = 4
	h.fillSlots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow, domain.MeetingJoinsPerMinute-free)
	const callers = 12
	answers := make(chan pendingAnswer, callers)

	for index := range callers {
		go func() {
			if index%2 == 0 {
				result, err := h.trySend(http.MethodPost, lessonRoute(lesson, "join"), "", jsonCall(parties.studentToken))
				answers <- pendingAnswer{result: result, err: err}
				return
			}
			result, err := h.trySendTo(second, http.MethodPost, lessonRoute(lesson, "join"), parties.studentToken)
			answers <- pendingAnswer{result: result, err: err}
		}()
	}
	admitted, refused := 0, 0
	for range callers {
		answer := <-answers
		if answer.err != nil {
			t.Fatal(answer.err)
		}
		switch answer.result.status {
		case http.StatusOK:
			admitted++
		case http.StatusTooManyRequests:
			refused++
		default:
			t.Fatalf("a racing join answered %d %s", answer.result.status, answer.result.body)
		}
	}

	if admitted != free || refused != callers-free || h.slots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow) != domain.MeetingJoinsPerMinute {
		t.Fatalf("admitted %d, refused %d, slots %d", admitted, refused, h.slots(t, domain.PurposeMeetingJoin, parties.student, domain.MeetingJoinWindow))
	}
}

func testRateLessonWrites(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _, room := h.liveWithRoom(t, parties)
	_, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	base := h.slots(t, domain.PurposeLessonWrites, parties.tutor, domain.LessonWriteWindow)
	roomsBefore := h.providerRooms(t)

	foreign := h.endCall(t, intruder, lesson)
	foreignRestart := h.restartCall(t, intruder, lesson)
	student := h.endCall(t, parties.studentToken, lesson)
	unscoped := h.slots(t, domain.PurposeLessonWrites, domain.UserID{2}, domain.LessonWriteWindow)
	untouched := h.currentRoom(t, lesson) == room && h.meetingCount(t, lesson) == 1 && h.providerRooms(t) == roomsBefore
	restart := h.restartCall(t, parties.tutorToken, lesson)
	first := h.endCall(t, parties.tutorToken, lesson)
	second := h.endCall(t, parties.tutorToken, lesson)
	charged := h.slots(t, domain.PurposeLessonWrites, parties.tutor, domain.LessonWriteWindow)
	h.exec3(t, "UPDATE rate_limit_counters SET count = $2::text::int WHERE purpose = $1 AND window_start = $3::timestamptz", string(domain.PurposeLessonWrites), fmt.Sprint(domain.LessonWritesPerHour), rfc3339(windowOf(h.now(t), domain.LessonWriteWindow)))
	refused := h.endCall(t, parties.tutorToken, lesson)
	afterRefusal := h.slots(t, domain.PurposeLessonWrites, parties.tutor, domain.LessonWriteWindow)

	expectRefusal(t, "a foreign tutor", foreign, http.StatusNotFound, "not_found")
	expectRefusal(t, "a foreign tutor's restart", foreignRestart, http.StatusNotFound, "not_found")
	if !untouched {
		t.Fatal("a foreign tutor's restart changed the room")
	}
	expectRefusal(t, "a student", student, http.StatusNotFound, "not_found")
	if restart.status != http.StatusOK || first.status != http.StatusOK || second.status != http.StatusOK {
		t.Fatalf("restart %d, end %d, repeated %d", restart.status, first.status, second.status)
	}
	if unscoped != 0 || charged != base+3 {
		t.Fatalf("foreign slots %d, charged %d from %d", unscoped, charged, base)
	}
	expectRefusal(t, "the write budget", refused, http.StatusTooManyRequests, "rate_limited")
	if refused.headers.Get("Retry-After") == "" || afterRefusal != domain.LessonWritesPerHour {
		t.Fatalf("Retry-After %q, slots %d", refused.headers.Get("Retry-After"), afterRefusal)
	}
}

func windowOf(now time.Time, size time.Duration) time.Time {
	window, _ := domain.FixedWindow(now, size)
	return window
}

func (h *authHarness) disabledInstance(t *testing.T) string {
	t.Helper()
	options := h.options
	options.Meetings = meetings.NewLifecycle(meetings.LifecycleDependencies{Store: h.store, Provider: h.provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger})
	return h.serve(t, options)
}

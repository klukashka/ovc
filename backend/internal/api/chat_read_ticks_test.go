package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	readSignalSent       = `"msg":"chat.read_signal_sent"`
	readSignalSuppressed = `"msg":"chat.read_signal_suppressed"`
	readSignalFailed     = `"msg":"chat.read_signal_failed"`
	lockedChannelRow     = "SELECT " + lockedRowPosition + " FROM channels WHERE id = $1::uuid FOR UPDATE"
	suspendTheStudentRow = "UPDATE channel_members SET suspended_since = now() WHERE channel_id = $1::uuid AND user_id <> (SELECT tutor_id FROM channels WHERE id = $1::uuid)"
)

type sharedMarker struct {
	shared   int64
	lastRead int64
	readAt   *time.Time
}

func (h *authHarness) peerReadOf(t *testing.T, token string, path string) *int64 {
	t.Helper()
	result := h.send(t, http.MethodGet, path, "", jsonCall(token))
	h.mustAnswer(t, result, http.StatusOK)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &fields); err != nil {
		t.Fatal(err)
	}
	raw, carried := fields["peer_read_seq"]
	if !carried {
		t.Fatalf("%s carries no peer_read_seq: %s", path, result.body)
	}
	var seq *int64
	if err := json.Unmarshal(raw, &seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

func (h *authHarness) peerReadEverywhere(t *testing.T, token string, channel string) []*int64 {
	t.Helper()
	base := channelsRoute + "/" + channel
	return []*int64{
		h.peerReadOf(t, token, base),
		h.peerReadOf(t, token, base+"/messages"),
		h.peerReadOf(t, token, base+"/messages?after_seq=1"),
		h.peerReadOf(t, token, base+"/messages?around_seq=2"),
	}
}

func everyPeerReadIs(seqs []*int64, want int64) bool {
	return !slices.ContainsFunc(seqs, func(seq *int64) bool { return seq == nil || *seq != want })
}

func everyPeerReadIsNull(seqs []*int64) bool {
	return !slices.ContainsFunc(seqs, func(seq *int64) bool { return seq != nil })
}

func (h *authHarness) sharedMarkerOf(t *testing.T, channel string, user domain.UserID) sharedMarker {
	t.Helper()
	var marker sharedMarker
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT shared_read_seq, last_read_seq, last_read_at FROM channel_members WHERE channel_id = $1::uuid AND user_id = $2::uuid",
		channel, user.String()).Scan(&marker.shared, &marker.lastRead, &marker.readAt)
	if err != nil {
		t.Fatal(err)
	}
	return marker
}

func readFramesTo(frames []sentFrame, user domain.UserID, channel string) int {
	count := 0
	for _, frame := range framesTo(frames, user) {
		if frame.kind == string(domain.FrameReadUpdated) && frame.id == channel {
			count++
		}
	}
	return count
}

func (h *authHarness) reactivate(t *testing.T, parties endingParties) {
	t.Helper()
	regenerated := h.cardCall(t, parties.tutorToken, http.MethodPost, parties.card.ID, "/invitation", "")
	var issued struct {
		Invitation cardInvitation `json:"invitation"`
	}
	if err := json.Unmarshal([]byte(regenerated.body), &issued); err != nil || regenerated.status != http.StatusOK {
		t.Fatalf("inviting again answered %d %s", regenerated.status, regenerated.body)
	}
	h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, linkToken(t, &issued.Invitation)), http.StatusOK)
}

func TestAStudentsReadReachesTheTutorAsTheDirectChannelsPeerRead(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 5)
	log := h.recordFrames(t)

	read, _ := h.readChannel(t, parties.studentToken, direct.ID, 3)
	frames := log.settle(t, h)
	forTutor := h.peerReadEverywhere(t, parties.tutorToken, direct.ID)
	forStudent := h.peerReadEverywhere(t, parties.studentToken, direct.ID)

	h.mustAnswer(t, read, http.StatusOK)
	if readFramesTo(frames, parties.tutor, direct.ID) != 1 || readFramesTo(frames, parties.student, direct.ID) != 1 {
		t.Fatalf("the read was announced as %v", frames)
	}
	if !everyPeerReadIs(forTutor, 3) || !everyPeerReadIs(forStudent, 0) {
		t.Fatalf("the tutor sees the student's read as %v and the student the tutor's as %v", forTutor, forStudent)
	}
	if h.countLogs(`"msg":"chat.read_moved"`, `"exposed":true`, direct.ID) != 1 || h.countLogs(readSignalSent, direct.ID, parties.student.String()) != 1 {
		t.Fatal("the read logged no exposed move or no sent signal")
	}
	if h.countLogs(`"msg":"chat.read_signal`, `"seq"`) != 0 {
		t.Fatal("a read signal record carries a sequence number")
	}
}

func TestAGroupChannelExposesNoMembersRead(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	h.seedMessages(t, group.ID, parties.tutor, 5)
	h.exec2(t, "UPDATE channel_members SET last_read_seq = 4, shared_read_seq = 4 WHERE channel_id = $1::uuid AND user_id = $2::uuid",
		group.ID, parties.student.String())
	log := h.recordFrames(t)

	h.readChannel(t, parties.studentToken, group.ID, 5)
	h.readChannel(t, parties.tutorToken, group.ID, 5)
	frames := log.settle(t, h)
	forTutor := h.peerReadEverywhere(t, parties.tutorToken, group.ID)
	forStudent := h.peerReadEverywhere(t, parties.studentToken, group.ID)

	if readFramesTo(frames, parties.tutor, group.ID) != 1 || readFramesTo(frames, parties.student, group.ID) != 1 {
		t.Fatalf("the group reads were announced as %v", frames)
	}
	if !everyPeerReadIsNull(forTutor) || !everyPeerReadIsNull(forStudent) {
		t.Fatalf("the group exposes %v to the tutor and %v to the student", forTutor, forStudent)
	}
	if h.countLogs(readSignalSent, group.ID) != 0 || h.sharedMarkerOf(t, group.ID, parties.tutor).shared != 0 {
		t.Fatal("a group read was exposed")
	}
}

func TestReadsWithinOneSecondSignalOnceAtOnceAndOnceMoreWhenTheWindowEnds(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 5)
	log := h.recordFrames(t)

	h.readChannel(t, parties.studentToken, direct.ID, 2)
	immediate := log.settle(t, h)
	h.readChannel(t, parties.studentToken, direct.ID, 3)
	withinWindow := log.settle(t, h)
	h.reads.advance(time.Second)
	h.awaitLogs(t, 2, readSignalSent, direct.ID)
	trailing := log.settle(t, h)
	afterTrailing := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)
	h.reads.advance(time.Second)
	closed := log.settle(t, h)
	h.readChannel(t, parties.studentToken, direct.ID, 4)
	reopened := log.settle(t, h)

	counts := []int{
		readFramesTo(immediate, parties.tutor, direct.ID), readFramesTo(withinWindow, parties.tutor, direct.ID),
		readFramesTo(trailing, parties.tutor, direct.ID), readFramesTo(closed, parties.tutor, direct.ID), readFramesTo(reopened, parties.tutor, direct.ID),
	}
	if !slices.Equal(counts, []int{1, 0, 1, 0, 1}) {
		t.Fatalf("the tutor's read frames per step are %v", counts)
	}
	if afterTrailing == nil || *afterTrailing != 3 || h.countLogs(readSignalSent, direct.ID) != 3 {
		t.Fatalf("after the trailing signal the tutor sees %v", afterTrailing)
	}
}

func TestATrailingSignalIsSuppressedWhenTheChannelBecameReadOnlyInTheWindow(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 5)
	log := h.recordFrames(t)
	h.readChannel(t, parties.studentToken, direct.ID, 2)
	immediate := log.settle(t, h)

	h.readChannel(t, parties.studentToken, direct.ID, 3)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.reads.advance(time.Second)
	h.awaitLogs(t, 1, readSignalSuppressed, `"reason":"read_only"`, direct.ID)
	later := log.settle(t, h)

	if readFramesTo(immediate, parties.tutor, direct.ID) != 1 || readFramesTo(later, parties.tutor, direct.ID) != 0 {
		t.Fatalf("the tutor was sent %v and then %v", immediate, later)
	}
	if peer := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID); peer == nil || *peer != 3 {
		t.Fatalf("the read made while the channel was writable is exposed as %v", peer)
	}
}

func TestATrailingSignalIsSuppressedWhenTheRecipientIsReclaimedInTheWindow(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, account := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	h.seedMessages(t, channel.ID, account, 5)
	log := h.recordFrames(t)
	h.readChannel(t, tutorToken, channel.ID, 2)
	immediate := log.settle(t, h)

	h.readChannel(t, tutorToken, channel.ID, 3)
	h.reclaimed(t, newcomerEmail)
	h.reads.advance(time.Second)
	h.awaitLogs(t, 1, readSignalSuppressed, `"reason":"inactive"`, channel.ID)
	later := log.settle(t, h)

	if readFramesTo(immediate, account, channel.ID) != 1 || readFramesTo(later, account, channel.ID) != 0 {
		t.Fatalf("the account was sent %v and then %v", immediate, later)
	}
}

func TestATrailingSignalWaitingOnTheChannelLockSeesASuspensionCommittedMeanwhile(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.student, 5)
	log := h.recordFrames(t)
	h.readChannel(t, parties.tutorToken, direct.ID, 2)
	immediate := log.settle(t, h)
	h.readChannel(t, parties.tutorToken, direct.ID, 3)
	release := h.holdRowThen(t, lockedChannelRow, direct.ID, suspendTheStudentRow)

	h.reads.advance(time.Second)
	h.waitForLockWaiters(t, 1)
	release()
	h.awaitLogs(t, 1, readSignalSuppressed, `"reason":"inactive"`, direct.ID)
	later := log.settle(t, h)

	if readFramesTo(immediate, parties.student, direct.ID) != 1 || readFramesTo(later, parties.student, direct.ID) != 0 {
		t.Fatalf("the student was sent %v and then %v", immediate, later)
	}
}

func TestAFailedTrailingSignalIsLoggedAsAWarningWithItsKind(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.student, 5)
	log := h.recordFrames(t)
	h.readChannel(t, parties.tutorToken, direct.ID, 2)
	immediate := log.settle(t, h)
	h.readChannel(t, parties.tutorToken, direct.ID, 3)
	release := h.holdRow(t, lockedChannelRow, direct.ID)

	h.reads.advance(time.Second)
	h.waitForLockWaiters(t, 1)
	h.cancelLockWaiters(t, 1)
	h.awaitLogs(t, 1, readSignalFailed, `"level":"WARN"`, `"kind":`, direct.ID)
	release()
	later := log.settle(t, h)

	if readFramesTo(immediate, parties.student, direct.ID) != 1 || readFramesTo(later, parties.student, direct.ID) != 0 {
		t.Fatalf("the student was sent %v and then %v", immediate, later)
	}
	if h.countLogs(readSignalFailed, `"seq"`) != 0 || h.countLogs(readSignalSent, direct.ID) != 1 {
		t.Fatal("the failed signal logged a sequence number or a sent record")
	}
}

func TestAReadInAReadOnlyChannelIsNotExposed(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 5)
	h.readChannel(t, parties.studentToken, direct.ID, 2)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	log := h.recordFrames(t)

	read, _ := h.readChannel(t, parties.studentToken, direct.ID, 4)
	frames := log.settle(t, h)
	forTutor := h.peerReadEverywhere(t, parties.tutorToken, direct.ID)

	h.mustAnswer(t, read, http.StatusOK)
	if readFramesTo(frames, parties.tutor, direct.ID) != 0 || readFramesTo(frames, parties.student, direct.ID) != 1 {
		t.Fatalf("the read-only read was announced as %v", frames)
	}
	if !everyPeerReadIs(forTutor, 2) || h.sharedMarkerOf(t, direct.ID, parties.student).lastRead != 4 {
		t.Fatalf("the tutor sees %v", forTutor)
	}
}

func TestASuspendedStudentNeitherLearnsNorRevealsReads(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, account := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	h.seedMessages(t, channel.ID, tutor, 5)
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.readChannel(t, squatterToken, channel.ID, 2)
	h.reclaimed(t, newcomerEmail)
	log := h.recordFrames(t)

	read, _ := h.readChannel(t, tutorToken, channel.ID, 4)
	frames := log.settle(t, h)
	forTutor := h.peerReadEverywhere(t, tutorToken, channel.ID)

	h.mustAnswer(t, read, http.StatusOK)
	if readFramesTo(frames, account, channel.ID) != 0 || readFramesTo(frames, tutor, channel.ID) != 1 {
		t.Fatalf("the tutor's read was announced as %v", frames)
	}
	if !everyPeerReadIs(forTutor, 2) || h.sharedMarkerOf(t, channel.ID, tutor).shared != 0 {
		t.Fatalf("the tutor sees the suspended row's read as %v", forTutor)
	}
}

func TestAfterReactivationThePeerReadStaysFrozenUntilTheMemberReportsAPosition(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 10)
	h.readChannel(t, parties.studentToken, direct.ID, 5)
	h.reads.advance(time.Second)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.readChannel(t, parties.studentToken, direct.ID, 10)
	h.reactivate(t, parties)
	frozen := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)
	beforeReports := h.sharedMarkerOf(t, direct.ID, parties.student)
	log := h.recordFrames(t)

	h.readChannel(t, parties.studentToken, direct.ID, 3)
	behind := log.settle(t, h)
	afterBehind := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)
	h.readChannel(t, parties.studentToken, direct.ID, 7)
	reported := log.settle(t, h)
	afterReported := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)
	h.reads.advance(time.Second)
	h.readChannel(t, parties.studentToken, direct.ID, 99)
	clamped := log.settle(t, h)
	afterClamped := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)
	afterReports := h.sharedMarkerOf(t, direct.ID, parties.student)

	if frozen == nil || *frozen != 5 || afterBehind == nil || *afterBehind != 5 || afterReported == nil || *afterReported != 7 ||
		afterClamped == nil || *afterClamped != 10 {
		t.Fatalf("the tutor sees %v, %v, %v and %v", frozen, afterBehind, afterReported, afterClamped)
	}
	steps := []int{
		readFramesTo(behind, parties.tutor, direct.ID), readFramesTo(reported, parties.tutor, direct.ID), readFramesTo(clamped, parties.tutor, direct.ID),
		readFramesTo(behind, parties.student, direct.ID), readFramesTo(reported, parties.student, direct.ID), readFramesTo(clamped, parties.student, direct.ID),
	}
	if !slices.Equal(steps, []int{0, 1, 1, 0, 0, 0}) {
		t.Fatalf("the read frames per step are %v", steps)
	}
	if beforeReports.lastRead != 10 || afterReports.lastRead != 10 || beforeReports.readAt == nil || afterReports.readAt == nil ||
		!beforeReports.readAt.Equal(*afterReports.readAt) {
		t.Fatalf("the reports moved the student's own marker from %+v to %+v", beforeReports, afterReports)
	}
}

func TestKeepingAReclaimedLinkLowersTheExposedRead(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	for _, body := range []string{"First", "Second", "Third"} {
		h.sent(t, tutorToken, channel.ID, body)
	}
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.readChannel(t, squatterToken, channel.ID, 3)
	beforeReclaim := h.peerReadOf(t, tutorToken, channelsRoute+"/"+channel.ID)

	owner := h.reclaimed(t, newcomerEmail)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	afterKeep := h.peerReadOf(t, tutorToken, channelsRoute+"/"+channel.ID)

	if beforeReclaim == nil || *beforeReclaim != 3 || afterKeep == nil || *afterKeep != 0 {
		t.Fatalf("the tutor saw %v before the reclaim and %v after the kept link", beforeReclaim, afterKeep)
	}
}

func TestASendExposesTheSendersReadToTheOtherMember(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 2)
	log := h.recordFrames(t)

	reply := h.sent(t, parties.studentToken, direct.ID, "A reply")
	frames := log.settle(t, h)
	forTutor := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)

	want := []sentFrame{{kind: string(domain.FrameMessageNew), id: reply.ID}, {kind: string(domain.FrameReadUpdated), id: direct.ID}}
	if !slices.Equal(chatFramesTo(frames, parties.tutor), want) {
		t.Fatalf("the tutor was sent %v", chatFramesTo(frames, parties.tutor))
	}
	if forTutor == nil || *forTutor != reply.Seq {
		t.Fatalf("the tutor sees %v after the reply at %d", forTutor, reply.Seq)
	}
}

func TestAPurgedStudentLeavesTheTutorNoPeerRead(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	h.seedMessages(t, direct.ID, parties.tutor, 3)
	h.readChannel(t, parties.studentToken, direct.ID, 3)
	before := h.peerReadOf(t, parties.tutorToken, channelsRoute+"/"+direct.ID)

	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	h.tick(t)
	after := h.peerReadEverywhere(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), direct.ID)

	if before == nil || *before != 3 || !everyPeerReadIsNull(after) {
		t.Fatalf("the tutor saw %v before the purge and %v after", before, after)
	}
}

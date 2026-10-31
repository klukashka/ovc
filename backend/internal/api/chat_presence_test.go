package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

const (
	firstPresenceEmail  = "first-learner@example.org"
	secondPresenceEmail = "second-learner@example.org"
	formerPresenceEmail = "former-learner@example.org"
	outsideTutorEmail   = "outside-tutor@example.org"
	presenceRowLock     = "SELECT " + lockedRowPosition + " FROM user_presence WHERE user_id = $1::uuid FOR UPDATE"
	userRowShare        = "SELECT " + lockedRowPosition + " FROM users WHERE id = $1::uuid FOR SHARE"
	userRowNoKeyUpdate  = "SELECT " + lockedRowPosition + " FROM users WHERE id = $1::uuid FOR NO KEY UPDATE"
	userRowForPurge     = "SELECT " + lockedRowPosition + " FROM users WHERE id = $1::uuid FOR UPDATE"
	deleteUserRow       = "DELETE FROM users WHERE id = $1::uuid"
)

type storedPresence struct {
	OnlineUntil time.Time
	LastSeenAt  time.Time
}

func (h *authHarness) presenceOf(t *testing.T, user domain.UserID) (storedPresence, bool) {
	t.Helper()
	if h.countRows(t, "SELECT count(user_id) FROM user_presence WHERE user_id = $1::uuid", user.String()) == 0 {
		return storedPresence{}, false
	}
	var stored storedPresence
	err := h.database.Pool.QueryRow(context.Background(), "SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1::uuid",
		user.String()).Scan(&stored.OnlineUntil, &stored.LastSeenAt)
	if err != nil {
		t.Fatal(err)
	}
	return storedPresence{OnlineUntil: stored.OnlineUntil.UTC(), LastSeenAt: stored.LastSeenAt.UTC()}, true
}

func (h *authHarness) awaitPresence(t *testing.T, user domain.UserID, onlineUntil time.Time) storedPresence {
	t.Helper()
	var stored storedPresence
	h.waitFor(t, "the persisted presence", func() bool {
		found, ok := h.presenceOf(t, user)
		stored = found
		return ok && found.OnlineUntil.Equal(onlineUntil)
	})
	return stored
}

func (h *liveHarness) awaitRegistry(t *testing.T, user domain.UserID, sockets int) {
	t.Helper()
	h.waitFor(t, "the registry's sockets", func() bool {
		presence := h.gateway.Presence(user)
		return presence.Sockets == sockets && !presence.Pending
	})
}

func (h *authHarness) awaitAnnounced(t *testing.T, at int, user domain.UserID, online bool) {
	t.Helper()
	h.awaitLogs(t, at, `"msg":"presence.announced"`, `"user_id":"`+user.String()+`"`, fmt.Sprintf(`"online":%t`, online))
}

func (c *socketClient) typing(t *testing.T, kind domain.FrameType, channel string, ref string) {
	t.Helper()
	c.sendText(t, fmt.Sprintf(`{"type":%q,"ref":%q,"payload":{"channel_id":%q}}`, kind, ref, channel))
}

func (c *socketClient) pongAfter(t *testing.T, ref string) []serverFrame {
	t.Helper()
	c.ping(t, ref)
	var skipped []serverFrame
	for {
		frame, closed := c.next(t)
		if closed != 0 {
			t.Fatalf("the socket closed with %d before the pong", closed)
		}
		if frame.Type == domain.FramePong && frame.Ref != nil && *frame.Ref == ref {
			return skipped
		}
		skipped = append(skipped, frame)
	}
}

func typingOf(t *testing.T, frame serverFrame) (string, *string) {
	t.Helper()
	var payload struct {
		ChannelID string  `json:"channel_id"`
		UserID    *string `json:"user_id"`
	}
	if frame.Type != domain.FrameTyping || json.Unmarshal(frame.Payload, &payload) != nil || frame.Ref != nil {
		t.Fatalf("not a typing frame: %s %s", frame.Type, frame.Payload)
	}
	if fields := fieldNames(payloadFields(t, frame)); !slices.Equal(fields, []string{"channel_id", "user_id"}) {
		t.Fatalf("a typing frame carries %v", fields)
	}
	return payload.ChannelID, payload.UserID
}

func presenceFrame(online bool, user domain.UserID) sentFrame {
	return sentFrame{kind: presenceKind(online), id: user.String()}
}

func sortedFrames(frames []sentFrame) []sentFrame {
	sorted := slices.Clone(frames)
	slices.SortFunc(sorted, func(a, b sentFrame) int {
		if a.kind != b.kind {
			return compareText(a.kind, b.kind)
		}
		return compareText(a.id, b.id)
	})
	return sorted
}

func compareText(a string, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

type groupOfTwo struct {
	tutor        domain.UserID
	tutorToken   string
	first        domain.UserID
	firstToken   string
	second       domain.UserID
	secondToken  string
	firstCard    studentCard
	secondCard   studentCard
	group        studentGroup
	groupChannel string
}

func (h *authHarness) groupOfTwoStudents(t *testing.T) groupOfTwo {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first, firstToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, firstPresenceEmail)
	second, secondToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, secondPresenceEmail)
	firstCard := h.linkedSignedIn(t, tutorToken, firstToken, "First card", firstPresenceEmail)
	secondCard := h.linkedSignedIn(t, tutorToken, secondToken, "Second card", secondPresenceEmail)
	group := h.newGroup(t, tutorToken, groupName, []string{firstCard.ID, secondCard.ID})
	return groupOfTwo{
		tutor: tutor, tutorToken: tutorToken, first: first, firstToken: firstToken, second: second, secondToken: secondToken,
		firstCard: firstCard, secondCard: secondCard, group: group, groupChannel: h.groupChannel(t, group.ID).ID,
	}
}

func TestPresenceIsPersistedAndExtendedWhileConnected(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, quietTiming())
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.connect(t, token)
	connected := h.awaitPresence(t, student, authStart.Add(domain.PresenceWindow))
	h.advance(t, 30*time.Second)
	h.awaitTickers(t, scheduler, realtime.DefaultTiming().PresenceEvery, 1)
	beforeTheTick, _ := h.presenceOf(t, student)

	scheduler.advance(realtime.DefaultTiming().PresenceEvery)

	extended := h.awaitPresence(t, student, authStart.Add(30*time.Second+domain.PresenceWindow))
	if !connected.LastSeenAt.Equal(authStart) || beforeTheTick != connected {
		t.Fatalf("the connection wrote %+v and before the extension the row was %+v", connected, beforeTheTick)
	}
	if !extended.LastSeenAt.Equal(authStart.Add(30 * time.Second)) {
		t.Fatalf("the extension wrote %+v", extended)
	}
	h.awaitLogs(t, 1, `"msg":"presence.batch"`, `"online":true`, `"accounts":1`, `"skipped":0`)
}

func TestTheLastSocketClosingReportsPendingUntilItsLeaseIsReleased(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	first := h.connect(t, token)
	last := h.connect(t, token)
	h.awaitPresence(t, student, authStart.Add(domain.PresenceWindow))
	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, student, 1)
	afterTheFirst, _ := h.presenceOf(t, student)
	release := h.holdRow(t, presenceRowLock, student.String())
	h.advance(t, 10*time.Second)

	if err := last.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.waitForLockWaiters(t, 1)
	whileWriting := h.gateway.Presence(student)
	release()
	h.awaitRegistry(t, student, 0)

	closed, _ := h.presenceOf(t, student)
	if !afterTheFirst.OnlineUntil.Equal(authStart.Add(domain.PresenceWindow)) {
		t.Fatalf("closing one of two sockets wrote %+v", afterTheFirst)
	}
	if whileWriting.Online || whileWriting.Sockets != 0 || !whileWriting.Pending {
		t.Fatalf("the closing socket did not retain a pending release: %+v", whileWriting)
	}
	if !closed.OnlineUntil.Equal(authStart.Add(10*time.Second)) || !closed.LastSeenAt.Equal(authStart.Add(10*time.Second)) {
		t.Fatalf("the closing wrote %+v", closed)
	}
}

func TestAClockAdvanceKeepsConnectedAccountsOnline(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{testControl: true, gateway: func(deps *realtime.Dependencies) { deps.Scheduler = scheduler }})
	parties := h.groupOfTwoStudents(t)
	h.connect(t, parties.firstToken)
	h.awaitPresence(t, parties.first, authStart.Add(domain.PresenceWindow))

	advanced := h.send(t, http.MethodPost, "/__test__/clock", `{"advance_seconds":600}`, map[string]string{"Content-Type": "application/json"})

	h.mustAnswer(t, advanced, http.StatusOK)
	kept, _ := h.presenceOf(t, parties.first)
	if !kept.OnlineUntil.Equal(authStart.Add(10*time.Minute + domain.PresenceWindow)) {
		t.Fatalf("after the advance the connected account's presence is %+v", kept)
	}
	h.sent(t, parties.tutorToken, parties.groupChannel, "After the jump")
	if h.digestPending(t, parties.first) || !h.digestPending(t, parties.second) {
		t.Fatal("the send flagged the connected member or did not flag the offline one")
	}
}

func TestPresenceFramesReachOnlyUsersSharingAWritableChannel(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, quietTiming())
	parties := h.groupOfTwoStudents(t)
	former, formerToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 4, formerPresenceEmail)
	formerCard := h.linkedSignedIn(t, parties.tutorToken, formerToken, "Former card", formerPresenceEmail)
	outsider, outsiderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, outsideTutorEmail)
	_, provisional := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	provisionalToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	log := h.recordFrames(t)
	for _, account := range []struct {
		user  domain.UserID
		token string
	}{{parties.first, parties.firstToken}, {parties.second, parties.secondToken}, {former, formerToken}, {provisional, provisionalToken}, {outsider, outsiderToken}} {
		h.connect(t, account.token)
		h.awaitAnnounced(t, 1, account.user, true)
	}
	tutorSocket := h.connect(t, parties.tutorToken)
	h.awaitAnnounced(t, 1, parties.tutor, true)
	connected := log.settle(t, h.authHarness)
	_, listed := h.channelList(t, parties.tutorToken, "")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, formerCard.ID), http.StatusOK)

	if err := tutorSocket.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.tutor, 0)
	scheduler.advance(realtime.DefaultTiming().PresenceSettle)
	h.awaitAnnounced(t, 1, parties.tutor, false)
	closed := log.settle(t, h.authHarness)

	wantTutor := sortedFrames([]sentFrame{
		presenceFrame(true, parties.first), presenceFrame(true, parties.second), presenceFrame(true, former), presenceFrame(true, provisional),
	})
	if got := sortedFrames(framesTo(connected, parties.tutor)); !slices.Equal(got, wantTutor) {
		t.Fatalf("the tutor was sent %v", got)
	}
	for name, student := range map[string]domain.UserID{"first": parties.first, "second": parties.second, "former": former, "provisional": provisional} {
		if got := framesTo(connected, student); !slices.Equal(got, []sentFrame{presenceFrame(true, parties.tutor)}) {
			t.Fatalf("the %s student was sent %v while connecting", name, got)
		}
	}
	if got := framesTo(append(connected, closed...), outsider); len(got) != 0 {
		t.Fatalf("a tutor sharing no channel was sent %v", got)
	}
	for name, student := range map[string]domain.UserID{"first": parties.first, "second": parties.second, "provisional": provisional} {
		if got := framesTo(closed, student); !slices.Equal(got, []sentFrame{presenceFrame(false, parties.tutor)}) {
			t.Fatalf("the %s student was sent %v when the tutor left", name, got)
		}
	}
	if got := presenceFrames(framesTo(closed, former)); len(got) != 0 {
		t.Fatalf("the student whose relationship ended was sent %v", got)
	}
	online := map[string]*bool{}
	for _, item := range listed.Items {
		if item.Kind == string(domain.ChannelDirect) && item.Label != nil {
			online[*item.Label] = item.Online
		}
	}
	for _, label := range []string{"First card", "Second card", "Former card", cardName} {
		if online[label] == nil || !*online[label] {
			t.Fatalf("the tutor's list shows %s as %v", label, online[label])
		}
	}
}

func TestPresenceChangesAreDebouncedPerAccount(t *testing.T) {
	t.Parallel()
	timing := quietTiming()
	timing.PresenceSettle = 7 * time.Second
	h, scheduler := newScheduledHarness(t, timing)
	parties := h.activeStudentInAGroup(t)
	settle := timing.PresenceSettle
	log := h.recordFrames(t)
	first := h.connect(t, parties.studentToken)
	h.awaitAnnounced(t, 1, parties.student, true)
	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.student, 0)
	second := h.connect(t, parties.studentToken)
	if err := second.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.student, 0)
	withinTheWindow := log.settle(t, h.authHarness)

	scheduler.advance(settle)
	h.awaitAnnounced(t, 1, parties.student, false)
	settled := log.settle(t, h.authHarness)
	h.connect(t, parties.studentToken)
	pendingWhileSettling := scheduler.pendingTimers(settle)
	scheduler.advance(settle)
	h.awaitAnnounced(t, 2, parties.student, true)
	reconnected := log.settle(t, h.authHarness)

	if got := framesTo(withinTheWindow, parties.tutor); !slices.Equal(got, []sentFrame{presenceFrame(true, parties.student)}) {
		t.Fatalf("within one window the tutor was sent %v", got)
	}
	if got := framesTo(settled, parties.tutor); !slices.Equal(got, []sentFrame{presenceFrame(false, parties.student)}) {
		t.Fatalf("after the window the tutor was sent %v", got)
	}
	if pendingWhileSettling != 1 {
		t.Fatalf("reconnecting inside the next window armed %d settle timers", pendingWhileSettling)
	}
	if got := framesTo(reconnected, parties.tutor); !slices.Equal(got, []sentFrame{presenceFrame(true, parties.student)}) {
		t.Fatalf("after the next window the tutor was sent %v", got)
	}
}

func TestTypingIsCoalescedBeforeTheDatabaseAndRelayedToOtherConnectedMembers(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, quietTiming())
	parties := h.groupOfTwoStudents(t)
	coalesce := realtime.DefaultTiming().TypingCoalesce
	tutorSocket := h.connect(t, parties.tutorToken)
	typist := h.connect(t, parties.firstToken)
	typistElsewhere := h.connect(t, parties.firstToken)
	classmate := h.connect(t, parties.secondToken)
	log := h.recordFrames(t)

	typist.typing(t, domain.FrameTypingStart, parties.groupChannel, "first")
	tutorFirst, classmateFirst := tutorSocket.expect(t, domain.FrameTyping), classmate.expect(t, domain.FrameTyping)
	relayed := typingFrames(log.settle(t, h.authHarness))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", parties.groupChannel)
	typist.typing(t, domain.FrameTypingStart, parties.groupChannel, "coalesced")
	typist.typing(t, domain.FrameTypingStop, parties.groupChannel, "coalesced-stop")
	typist.ping(t, "after-coalesced")
	answeredWhileLocked := typist.expect(t, domain.FramePong)
	release()
	coalesced := typingFrames(log.settle(t, h.authHarness))
	scheduler.advance(coalesce)
	typist.typing(t, domain.FrameTypingStop, parties.groupChannel, "stopped")
	typist.ping(t, "after-stop")
	typist.expect(t, domain.FramePong)
	stopped := typingFrames(log.settle(t, h.authHarness))
	scheduler.advance(coalesce)
	typist.typing(t, domain.FrameTypingStart, parties.groupChannel, "second")
	tutorSecond := tutorSocket.expect(t, domain.FrameTyping)

	channel, shown := typingOf(t, tutorFirst)
	if channel != parties.groupChannel || shown == nil || *shown != parties.first.String() {
		t.Fatalf("the tutor saw %s typing in %s", ptrText(shown), channel)
	}
	if channel, anonymous := typingOf(t, classmateFirst); channel != parties.groupChannel || anonymous != nil {
		t.Fatalf("the other student saw %s typing in %s", ptrText(anonymous), channel)
	}
	wantRelayed := sortedFrames([]sentFrame{{to: parties.tutor, kind: string(domain.FrameTyping), id: parties.first.String()},
		{to: parties.second, kind: string(domain.FrameTyping)}})
	if !slices.Equal(sortedFrames(relayed), wantRelayed) {
		t.Fatalf("the first frame was relayed as %v", relayed)
	}
	if answeredWhileLocked.Ref == nil || *answeredWhileLocked.Ref != "after-coalesced" || len(coalesced) != 0 {
		t.Fatalf("a coalesced frame reached the database or a member: %v", coalesced)
	}
	if len(stopped) != 0 {
		t.Fatalf("a typing.stop was relayed as %v", stopped)
	}
	if _, again := typingOf(t, tutorSecond); again == nil || *again != parties.first.String() {
		t.Fatal("the next window's frame was not relayed")
	}
	typistElsewhere.ping(t, "own-other-socket")
	if next := typistElsewhere.expect(t, domain.FramePong); next.Ref == nil || *next.Ref != "own-other-socket" {
		t.Fatal("the typist's own other socket was sent its typing")
	}
	h.awaitLogs(t, 2, `"realtime.delivery"`, `"kind":"typing"`, `"recipients":2`)
}

func presenceFrames(frames []sentFrame) []sentFrame {
	var presence []sentFrame
	for _, frame := range frames {
		if frame.kind == presenceKind(true) || frame.kind == presenceKind(false) {
			presence = append(presence, frame)
		}
	}
	return presence
}

func typingFrames(frames []sentFrame) []sentFrame {
	var typing []sentFrame
	for _, frame := range frames {
		if frame.kind == string(domain.FrameTyping) {
			typing = append(typing, frame)
		}
	}
	return typing
}

func ptrText(value *string) string {
	if value == nil {
		return "null"
	}
	return *value
}

func TestStudentsReceiveAnonymousTypingAboutOtherStudents(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	parties := h.groupOfTwoStudents(t)
	tutorSocket := h.connect(t, parties.tutorToken)
	first := h.connect(t, parties.firstToken)
	second := h.connect(t, parties.secondToken)
	direct := h.directChannel(t, parties.secondCard.ID).ID

	second.typing(t, domain.FrameTypingStart, parties.groupChannel, "group")
	aboutSecondToTutor, aboutSecondToFirst := tutorSocket.expect(t, domain.FrameTyping), first.expect(t, domain.FrameTyping)
	tutorSocket.typing(t, domain.FrameTypingStart, parties.groupChannel, "tutor")
	aboutTutorToFirst, aboutTutorToSecond := first.expect(t, domain.FrameTyping), second.expect(t, domain.FrameTyping)
	second.typing(t, domain.FrameTypingStart, direct, "direct")
	directToTutor := tutorSocket.expect(t, domain.FrameTyping)
	first.ping(t, "first-after")
	firstAfter := first.expect(t, domain.FramePong)

	if _, user := typingOf(t, aboutSecondToTutor); user == nil || *user != parties.second.String() {
		t.Fatalf("the tutor saw %s typing", ptrText(user))
	}
	if _, user := typingOf(t, aboutSecondToFirst); user != nil {
		t.Fatalf("a student saw another student's account %s typing", *user)
	}
	for name, frame := range map[string]serverFrame{"first": aboutTutorToFirst, "second": aboutTutorToSecond} {
		if _, user := typingOf(t, frame); user == nil || *user != parties.tutor.String() {
			t.Fatalf("the %s student saw %s typing for the tutor", name, ptrText(user))
		}
	}
	if channel, user := typingOf(t, directToTutor); channel != direct || user == nil || *user != parties.second.String() {
		t.Fatalf("the tutor saw %s typing in %s", ptrText(user), channel)
	}
	if firstAfter.Ref == nil || *firstAfter.Ref != "first-after" {
		t.Fatal("the direct channel's typing reached a student outside it")
	}
}

func TestTypingFromANonMemberOrIntoAReadOnlyChannelIsRefused(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	parties := h.groupOfTwoStudents(t)
	_, outsiderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, outsideTutorEmail)
	ended := h.directChannel(t, parties.secondCard.ID).ID
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.secondCard.ID), http.StatusOK)
	outsider := h.connect(t, outsiderToken)
	second := h.connect(t, parties.secondToken)
	tutorSocket := h.connect(t, parties.tutorToken)
	log := h.recordFrames(t)

	outsider.typing(t, domain.FrameTypingStart, parties.groupChannel, "outside-start")
	outsider.typing(t, domain.FrameTypingStop, ended, "outside-stop")
	outsider.typing(t, domain.FrameTypingStart, randomChannelID, "random")
	outsider.sendText(t, `{"type":"typing.start","ref":"malformed","payload":{"channel_id":"not-a-channel"}}`)
	outsider.sendText(t, `{"type":"typing.stop","ref":"no-payload"}`)
	second.typing(t, domain.FrameTypingStart, ended, "ended-student")
	tutorSocket.typing(t, domain.FrameTypingStop, ended, "ended-tutor")
	second.typing(t, domain.FrameTypingStart, parties.groupChannel, "left-group")
	refusals := map[string]domain.ErrorCode{}
	for range 5 {
		code, _, ref := errorPayload(t, outsider.expect(t, domain.FrameError))
		refusals[ptrText(ref)] = code
	}
	for _, socket := range []*socketClient{second, tutorSocket, second} {
		code, _, ref := errorPayload(t, socket.expect(t, domain.FrameError))
		refusals[ptrText(ref)] = code
	}
	outsider.requireAlive(t)
	second.requireAlive(t)
	frames := log.settle(t, h.authHarness)

	want := map[string]domain.ErrorCode{
		"outside-start": domain.ErrorNotFound, "outside-stop": domain.ErrorNotFound, "random": domain.ErrorNotFound, "malformed": domain.ErrorBadRequest,
		"no-payload": domain.ErrorBadRequest, "ended-student": domain.ErrorChannelReadOnly, "ended-tutor": domain.ErrorChannelReadOnly,
		"left-group": domain.ErrorNotFound,
	}
	for ref, code := range want {
		if refusals[ref] != code {
			t.Fatalf("the frame %s was answered %s, want %s; all: %v", ref, refusals[ref], code, refusals)
		}
	}
	for _, frame := range frames {
		if frame.kind == string(domain.FrameTyping) {
			t.Fatalf("a refused typing frame was relayed: %v", frames)
		}
	}
	h.awaitLogs(t, 4, `"msg":"typing.rejected"`, `"reason":"not_found"`)
	h.awaitLogs(t, 2, `"msg":"typing.rejected"`, `"reason":"read_only"`)
}

func TestPresenceWaitsOnlyForConflictingUserRowLocks(t *testing.T) {
	t.Parallel()
	h, _ := newScheduledHarness(t, quietTiming())
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.connect(t, studentToken)
	h.connect(t, tutorToken)
	h.awaitPresence(t, student, authStart.Add(domain.PresenceWindow))
	h.awaitPresence(t, tutor, authStart.Add(domain.PresenceWindow))
	releaseStudent := h.holdRow(t, userRowNoKeyUpdate, student.String())
	releaseTutor := h.holdRow(t, userRowShare, tutor.String())
	h.advance(t, 20*time.Second)

	extendedWhileHeld := h.gateway.ExtendPresence(context.Background())
	studentRow, _ := h.presenceOf(t, student)
	tutorRow, _ := h.presenceOf(t, tutor)
	releaseStudent()
	releaseTutor()
	purge := h.holdRowThen(t, userRowForPurge, student.String(), deleteUserRow)
	waiting := make(chan error, 1)
	go func() { waiting <- h.gateway.ExtendPresence(context.Background()) }()
	h.waitForLockWaiters(t, 1)
	purge()
	afterThePurge := <-waiting

	if extendedWhileHeld != nil || afterThePurge != nil {
		t.Fatal(extendedWhileHeld, afterThePurge)
	}
	want := authStart.Add(20*time.Second + domain.PresenceWindow)
	if !studentRow.OnlineUntil.Equal(want) || !tutorRow.OnlineUntil.Equal(want) {
		t.Fatalf("while the user rows were locked the batch wrote %+v and %+v", studentRow, tutorRow)
	}
	if _, kept := h.presenceOf(t, student); kept {
		t.Fatal("the purged account kept its presence row")
	}
	h.awaitLogs(t, 1, `"msg":"presence.batch"`, `"online":true`, `"accounts":0`, `"skipped":1`)
}

func TestAPresenceBatchSurvivesAConcurrentPurge(t *testing.T) {
	t.Parallel()
	h, _ := newScheduledHarness(t, quietTiming())
	kept, keptToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, firstPresenceEmail)
	purged, purgedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, secondPresenceEmail)
	h.connect(t, keptToken)
	purgedSocket := h.connect(t, purgedToken)
	h.awaitPresence(t, kept, authStart.Add(domain.PresenceWindow))
	h.awaitPresence(t, purged, authStart.Add(domain.PresenceWindow))
	h.advance(t, 40*time.Second)
	purge := h.holdRowThen(t, userRowForPurge, purged.String(), deleteUserRow)

	batch := make(chan error, 1)
	go func() { batch <- h.gateway.ExtendPresence(context.Background()) }()
	h.waitForLockWaiters(t, 1)
	purge()
	batched := <-batch
	purgedSocket.expectClose(t, domain.CloseUnauthenticated)
	h.awaitRegistry(t, purged, 0)
	h.advance(t, 10*time.Second)
	next := h.gateway.ExtendPresence(context.Background())

	if batched != nil || next != nil {
		t.Fatal(batched, next)
	}
	if row, _ := h.presenceOf(t, kept); !row.OnlineUntil.Equal(authStart.Add(50*time.Second + domain.PresenceWindow)) {
		t.Fatalf("the surviving account's presence is %+v", row)
	}
	if _, found := h.presenceOf(t, purged); found {
		t.Fatal("the purged account has a presence row")
	}
	h.awaitLogs(t, 1, `"msg":"presence.batch"`, `"accounts":0`, `"skipped":1`)
	h.awaitLogs(t, 1, `"msg":"presence.batch"`, `"accounts":1`, `"skipped":0`, `"online":true`)
}

func TestTheGatewaysTimersRunOnItsScheduler(t *testing.T) {
	t.Parallel()
	defaults := realtime.DefaultTiming()
	timing := defaults
	timing.PresenceSettle = 7 * time.Second
	h, scheduler := newScheduledHarness(t, timing)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	connected := h.connect(t, token)
	h.awaitTickers(t, scheduler, timing.LapseCheckEvery, 1)
	h.advance(t, 15*time.Minute+timing.TokenGrace-time.Second)
	scheduler.advance(timing.LapseCheckEvery)
	h.awaitLogs(t, 1, `"msg":"realtime.lapse"`, `"checked":1`, `"closed":0`)
	connected.requireAlive(t)
	h.advance(t, time.Second)
	scheduler.advance(timing.LapseCheckEvery)
	connected.expectClose(t, domain.CloseUnauthenticated)
	silent := h.open(t)
	h.awaitTimers(t, scheduler, timing.AuthWithin, 1)
	scheduler.advance(timing.AuthWithin - time.Millisecond)
	stillArmed := scheduler.pendingTimers(timing.AuthWithin)

	scheduler.advance(time.Millisecond)

	silent.expectClose(t, domain.CloseUnauthenticated)
	if stillArmed != 1 {
		t.Fatalf("the authentication timer fired before its duration: %d armed", stillArmed)
	}
	if defaults.AuthWithin != 5*time.Second || defaults.TokenGrace != 30*time.Second || defaults.LapseCheckEvery != time.Second ||
		defaults.SweepEvery != time.Minute || defaults.PingEvery != 25*time.Second || defaults.PresenceEvery != time.Minute ||
		defaults.PresenceSettle != 5*time.Second || defaults.TypingCoalesce != 2*time.Second {
		t.Fatalf("the default timing changed: %+v", defaults)
	}
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"timeout"`)
	h.awaitLogs(t, 1, `"ws.token.lapsed"`)
}

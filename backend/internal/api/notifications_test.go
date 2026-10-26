package api_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	squatterChosenName = "A name the account chose"
	keptCardName       = "The tutor's own card name"
	mergedCardName     = "A second card for the same student"
)

type acceptancePath struct {
	name   string
	accept func(t *testing.T, h *authHarness, accounts numberedAccounts, tutorToken string) string
}

func (h *authHarness) numberedStudent(t *testing.T, accounts numberedAccounts) (domain.UserID, string) {
	t.Helper()
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 0x60+accounts.number, accounts.studentEmail())
	h.setAccount(t, student, squatterChosenName, domain.LanguageEnglish)
	return student, token
}

func acceptancePaths() []acceptancePath {
	return []acceptancePath{
		{"a new account, provisional", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutorToken string) string {
			t.Helper()
			card := h.newCard(t, tutorToken, keptCardName, accounts.newcomerEmail())
			h.mustAnswer(t, h.acceptInvitation(t, linkToken(t, card.Invitation), accounts.newcomerEmail()), http.StatusAccepted)
			h.setAccount(t, h.userID(t, accounts.newcomerEmail()), squatterChosenName, domain.LanguageEnglish)
			return card.ID
		}},
		{"a signed-in acceptance", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutorToken string) string {
			t.Helper()
			_, token := h.numberedStudent(t, accounts)
			return h.linkedSignedIn(t, tutorToken, token, keptCardName, accounts.studentEmail()).ID
		}},
		{"a merge", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutorToken string) string {
			t.Helper()
			_, token := h.numberedStudent(t, accounts)
			kept := h.linkedSignedIn(t, tutorToken, token, keptCardName, accounts.studentEmail())
			second := h.newCard(t, tutorToken, mergedCardName, "")
			h.mustAnswer(t, h.acceptSignedIn(t, token, linkToken(t, second.Invitation)), http.StatusOK)
			return kept.ID
		}},
		{"a reactivation", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutorToken string) string {
			t.Helper()
			_, token := h.numberedStudent(t, accounts)
			kept := h.linkedSignedIn(t, tutorToken, token, keptCardName, accounts.studentEmail())
			h.mustAnswer(t, h.endAsTutor(t, tutorToken, kept.ID), http.StatusOK)
			h.mustAnswer(t, h.acceptSignedIn(t, token, h.regeneratedLink(t, tutorToken, kept.ID)), http.StatusOK)
			return kept.ID
		}},
	}
}

func TestInvitationAcceptanceNotifiesTheTutorWithTheCardName(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	var number byte
	for _, path := range acceptancePaths() {
		number++
		accounts := numberedAccounts{number: number}
		t.Run(path.name, func(t *testing.T) {
			_, tutorToken := h.numberedTutor(t, accounts)

			kept := path.accept(t, h, accounts, tutorToken)
			listed := h.notificationsOf(t, tutorToken)

			if len(listed) == 0 || listed[0].Event != string(domain.NotificationInvitationAccepted) {
				t.Fatalf("the tutor's newest notifications are %v", eventsOf(listed))
			}
			newest := listed[0]
			if newest.Label == nil || *newest.Label != keptCardName || newest.Colour != nil || newest.ReadAt != nil {
				t.Fatalf("the accepted invitation shows %+v", newest)
			}
			if newest.Link == nil || newest.Link.Kind != string(domain.LinkStudentCard) || newest.Link.ID != kept {
				t.Fatalf("the accepted invitation links to %+v instead of the card %s", newest.Link, kept)
			}
		})
	}
}

func (h *authHarness) createdRowsFor(t *testing.T, request domain.JoinRequestID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM notifications WHERE event = 'join_request.created' AND subject_id = $1::uuid", request.String())
}

type visibleRequestPath func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string) (domain.JoinRequestID, func(t *testing.T))

func visibleRequestPaths() map[string]visibleRequestPath {
	return map[string]visibleRequestPath{
		"created visible": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) (domain.JoinRequestID, func(t *testing.T)) {
			t.Helper()
			_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
			code := h.storedCode(t, tutor).Display()
			request := h.enteredRequest(t, token, code)
			return request, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.enterCode(t, token, code), http.StatusOK)
			}
		},
		"released at confirmation": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) (domain.JoinRequestID, func(t *testing.T)) {
			t.Helper()
			_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)
			request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
			h.confirmInOwnBrowser(t, studentEmail, token)
			return request, func(t *testing.T) {
				t.Helper()
				h.tick(t)
			}
		},
		"un-suppressed by the expiry job": func(t *testing.T, h *authHarness, tutor domain.UserID, _ string) (domain.JoinRequestID, func(t *testing.T)) {
			t.Helper()
			h.fillPendingRequests(t, tutor)
			request := h.suppressedForLimit(t, tutor, 2, studentEmail)
			if _, err := h.database.Pool.Exec(context.Background(), "UPDATE join_requests SET expires_at = $1 WHERE id = $2::uuid",
				authStart, h.oldestFiller(t, tutor, "filler-")); err != nil {
				t.Fatal(err)
			}
			h.advance(t, time.Hour)
			h.tick(t)
			return request, func(t *testing.T) {
				t.Helper()
				h.advance(t, time.Hour)
				h.tick(t)
			}
		},
		"reopened after an unblock": func(t *testing.T, h *authHarness, tutor domain.UserID, tutorToken string) (domain.JoinRequestID, func(t *testing.T)) {
			t.Helper()
			student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
			closed := h.closedByUnblock(t, tutor, tutorToken, student, token)
			if h.createdRowsFor(t, closed) != 0 {
				t.Fatal("the suppressed request produced a row before it was reopened")
			}
			if reopened := h.enteredRequest(t, token, h.storedCode(t, tutor).Display()); reopened != closed {
				t.Fatalf("the code entry created %s instead of reopening %s", reopened, closed)
			}
			return closed, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.enterCode(t, token, h.storedCode(t, tutor).Display()), http.StatusOK)
			}
		},
	}
}

func TestARequestBecomingVisibleNotifiesTheTutorOnce(t *testing.T) {
	t.Parallel()
	for name, path := range visibleRequestPaths() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			request, repeat := path(t, h, tutor, tutorToken)
			before := h.createdRowsFor(t, request)

			repeat(t)
			after := h.createdRowsFor(t, request)
			listed := h.notificationsOf(t, h.relogin(t, tutorEmail))

			if before != 1 || after != 1 {
				t.Fatalf("the request has %d rows when it became visible and %d after the repeat", before, after)
			}
			if !slices.ContainsFunc(listed, linksTo(domain.NotificationJoinRequestCreated, domain.LinkJoinRequests, request.String())) {
				t.Fatalf("no row links the tutor to the visible request: %+v", listed)
			}
		})
	}
}

func TestHeldOrSuppressedRequestsReachTheTutorWithNothing(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	blocked, blockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, unconfirmed := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, newcomerEmail)
	code := h.storedCode(t, tutor).Display()
	h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, blockedToken, code)), true)
	before := h.notificationRows(t, tutor)
	log := h.recordFrames(t)

	h.enteredRequest(t, unconfirmed, code)
	h.enteredRequest(t, blockedToken, code)
	frames := log.settle(t, h)

	if after := h.notificationRows(t, tutor); after != before {
		t.Fatalf("held and suppressed requests gave the tutor %d new rows", after-before)
	}
	if addressed := framesTo(frames, tutor); len(addressed) != 0 {
		t.Fatalf("the tutor received %v about held or suppressed requests", addressed)
	}
	if len(framesTo(frames, blocked)) != 1 {
		t.Fatalf("the blocked student's own code entry sent %v", framesTo(frames, blocked))
	}
}

func linksTo(event domain.NotificationEvent, kind domain.NotificationLinkKind, id string) func(notificationItem) bool {
	return func(item notificationItem) bool {
		return item.Event == string(event) && item.Link != nil && item.Link.Kind == string(kind) && item.Link.ID == id
	}
}

type studentView struct {
	event    string
	label    bool
	colour   bool
	linkKind string
}

func viewsOf(items []notificationItem) []studentView {
	views := make([]studentView, 0, len(items))
	for _, item := range items {
		view := studentView{event: item.Event, label: item.Label != nil, colour: item.Colour != nil}
		if item.Link != nil {
			view.linkKind = item.Link.Kind
		}
		views = append(views, view)
	}
	return views
}

func TestBlockStateNeverReachesTheStudent(t *testing.T) {
	t.Parallel()
	t.Run("decisions, blocks and unblocks", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		declined, declinedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
		blocked, blockedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
		code := h.storedCode(t, tutor).Display()
		plain := requestText(h.enteredRequest(t, declinedToken, code))
		withBlock := requestText(h.enteredRequest(t, blockedToken, code))
		log := h.recordFrames(t)

		h.mustAnswer(t, h.declineRequest(t, tutorToken, plain, false), http.StatusOK)
		h.mustAnswer(t, h.declineRequest(t, tutorToken, withBlock, true), http.StatusOK)
		decisions := log.settle(t, h)
		declinedViews, blockedViews := viewsOf(h.notificationsOf(t, declinedToken)), viewsOf(h.notificationsOf(t, blockedToken))
		suppressed := requestText(h.enteredRequest(t, blockedToken, code))
		entry := log.settle(t, h)
		h.mustAnswer(t, h.unblock(t, tutorToken, blocked), http.StatusNoContent)
		h.mustAnswer(t, h.declineRequest(t, tutorToken, plain, true), http.StatusOK)
		if _, err := h.operators().Block(context.Background(), studentEmail); err != nil {
			t.Fatal(err)
		}
		later := log.settle(t, h)

		if len(declinedViews) != 1 || !slices.Equal(declinedViews, blockedViews) {
			t.Fatalf("a plain decline shows %v and a decline with a block %v", declinedViews, blockedViews)
		}
		plainFrames := entityKinds(decisions, declined)
		if !slices.Equal(plainFrames, entityKinds(decisions, blocked)) || notificationFrames(decisions, declined) != notificationFrames(decisions, blocked) {
			t.Fatalf("a plain decline sent %v and a decline with a block %v", framesTo(decisions, declined), framesTo(decisions, blocked))
		}
		if got := framesTo(entry, blocked); !slices.Equal(got, []sentFrame{{kind: string(domain.EntityJoinRequest), id: suppressed}}) {
			t.Fatalf("the suppressed code entry sent %v", got)
		}
		if got := append(framesTo(later, declined), framesTo(later, blocked)...); len(got) != 0 {
			t.Fatalf("an unblock, a decline adding a block or an operator block sent the students %v", got)
		}
	})
	t.Run("un-suppression", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		h.fillPendingRequests(t, tutor)
		waiting := h.suppressedForLimit(t, tutor, 2, studentEmail)
		student := h.userID(t, studentEmail)
		log := h.recordFrames(t)

		h.mustAnswer(t, h.declineRequest(t, tutorToken, h.oldestFiller(t, tutor, "filler-"), false), http.StatusOK)
		frames := log.settle(t, h)

		if h.storedRequest(t, waiting).Suppressed != "none" {
			t.Fatal("the decline did not un-suppress the waiting request")
		}
		if got := framesTo(frames, student); len(got) != 0 {
			t.Fatalf("the un-suppression sent the student %v", got)
		}
		if !hasFrame(frames, tutor, domain.EntityJoinRequest, waiting.String()) || notificationFrames(frames, tutor) != 1 {
			t.Fatalf("the un-suppression sent the tutor %v", framesTo(frames, tutor))
		}
	})
	t.Run("un-suppression skips an operator-blocked requester", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		h.fillPendingRequests(t, tutor)
		blockedRequest := h.suppressedForLimit(t, tutor, 2, studentEmail)
		otherRequest := h.suppressedForLimit(t, tutor, 3, newcomerEmail)
		blockedStudent := h.userID(t, studentEmail)
		if _, err := h.operators().Block(context.Background(), studentEmail); err != nil {
			t.Fatal(err)
		}
		log := h.recordFrames(t)

		h.mustAnswer(t, h.declineRequest(t, tutorToken, h.oldestFiller(t, tutor, "filler-"), false), http.StatusOK)
		whileBlocked := log.settle(t, h)
		stateWhileBlocked := h.storedRequest(t, blockedRequest)
		visibleWhileBlocked := h.tutorRequests(t, tutorToken)
		rowsWhileBlocked := h.notificationsOf(t, tutorToken)
		if _, err := h.operators().Unblock(context.Background(), studentEmail); err != nil {
			t.Fatal(err)
		}
		unblocked := log.settle(t, h)
		stateAfterUnblock := h.storedRequest(t, blockedRequest)
		h.mustAnswer(t, h.declineRequest(t, tutorToken, h.oldestFiller(t, tutor, "filler-"), false), http.StatusOK)
		freed := log.settle(t, h)
		rowsAfterFreedSlot := h.notificationsOf(t, tutorToken)

		if stateWhileBlocked.Suppressed != "limit" || h.storedRequest(t, otherRequest).Suppressed != "none" {
			t.Fatalf("while blocked the requester's request is %+v and the next one %+v", stateWhileBlocked, h.storedRequest(t, otherRequest))
		}
		if hasFrame(whileBlocked, tutor, domain.EntityJoinRequest, blockedRequest.String()) || notificationFrames(whileBlocked, tutor) != 1 ||
			!hasFrame(whileBlocked, tutor, domain.EntityJoinRequest, otherRequest.String()) {
			t.Fatalf("the freed slot sent the tutor %v", framesTo(whileBlocked, tutor))
		}
		if countLinksTo(rowsWhileBlocked, blockedRequest) != 0 || countLinksTo(rowsWhileBlocked, otherRequest) != 1 {
			t.Fatalf("while blocked the tutor has the notification rows %+v", rowsWhileBlocked)
		}
		if len(visibleWhileBlocked) != domain.VisiblePendingRequestCap || listsRequest(visibleWhileBlocked, blockedRequest) {
			t.Fatalf("while blocked the tutor sees %d requests", len(visibleWhileBlocked))
		}
		if stateAfterUnblock.Suppressed != "limit" || len(framesTo(unblocked, tutor)) != 0 {
			t.Fatalf("the unblock left the request %+v and sent the tutor %v", stateAfterUnblock, framesTo(unblocked, tutor))
		}
		if h.storedRequest(t, blockedRequest).Suppressed != "none" || countLinksTo(rowsAfterFreedSlot, blockedRequest) != 1 ||
			!hasFrame(freed, tutor, domain.EntityJoinRequest, blockedRequest.String()) || notificationFrames(freed, tutor) != 1 {
			t.Fatalf("the slot freed after the unblock sent the tutor %v", framesTo(freed, tutor))
		}
		if got := framesTo(slices.Concat(whileBlocked, unblocked, freed), blockedStudent); len(got) != 0 {
			t.Fatalf("the student was sent %v", got)
		}
	})
}

func countLinksTo(items []notificationItem, request domain.JoinRequestID) int {
	count := 0
	for _, item := range items {
		if linksTo(domain.NotificationJoinRequestCreated, domain.LinkJoinRequests, request.String())(item) {
			count++
		}
	}
	return count
}

func listsRequest(requests []tutorRequest, request domain.JoinRequestID) bool {
	return slices.ContainsFunc(requests, func(listed tutorRequest) bool { return listed.ID == request.String() })
}

func TestTutorOnlyCardEditsSendTheStudentNothing(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	log := h.recordFrames(t)

	notes := h.cardCall(t, parties.tutorToken, http.MethodPatch, parties.card.ID, "", `{"notes":"Private notes"}`)
	renamed := h.cardCall(t, parties.tutorToken, http.MethodPatch, parties.card.ID, "", `{"display_name":"A new card name"}`)
	email := h.cardCall(t, parties.tutorToken, http.MethodPatch, parties.card.ID, "", `{"email":"elsewhere@example.org"}`)
	frames := log.settle(t, h)

	if notes.status != http.StatusOK || renamed.status != http.StatusOK || email.status == http.StatusOK {
		t.Fatalf("the edits answered %d, %d and %d", notes.status, renamed.status, email.status)
	}
	if got := framesTo(frames, parties.student); len(got) != 0 {
		t.Fatalf("tutor-only edits sent the student %v", got)
	}
	if got := entityKinds(frames, parties.tutor); len(got) != 2 {
		t.Fatalf("the tutor's other tabs received %v for two committed edits", got)
	}
}

type codeEntryOutcome struct {
	name    string
	arrange func(t *testing.T, h *authHarness, accounts numberedAccounts, tutor domain.UserID, tutorToken string) (domain.UserID, string)
}

func TestCodeEntrySendsTheStudentOneIdenticalFrameForEveryOutcome(t *testing.T) {
	t.Parallel()
	outcomes := []codeEntryOutcome{
		{"created", func(t *testing.T, h *authHarness, accounts numberedAccounts, _ domain.UserID, _ string) (domain.UserID, string) {
			t.Helper()
			return h.numberedStudent(t, accounts)
		}},
		{"existing", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutor domain.UserID, _ string) (domain.UserID, string) {
			t.Helper()
			student, token := h.numberedStudent(t, accounts)
			h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
			return student, token
		}},
		{"held", func(t *testing.T, h *authHarness, accounts numberedAccounts, _ domain.UserID, _ string) (domain.UserID, string) {
			t.Helper()
			return h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 0x60+accounts.number, accounts.studentEmail())
		}},
		{"suppressed", func(t *testing.T, h *authHarness, accounts numberedAccounts, tutor domain.UserID, tutorToken string) (domain.UserID, string) {
			t.Helper()
			student, token := h.numberedStudent(t, accounts)
			h.declineRequest(t, tutorToken, requestText(h.enteredRequest(t, token, h.storedCode(t, tutor).Display())), true)
			return student, token
		}},
	}
	h := newFlowHarness(t)
	log := h.recordFrames(t)
	var number byte
	for _, outcome := range outcomes {
		number++
		accounts := numberedAccounts{number: number}
		t.Run(outcome.name, func(t *testing.T) {
			tutor, tutorToken := h.numberedTutor(t, accounts)
			student, token := outcome.arrange(t, h, accounts, tutor, tutorToken)
			log.settle(t, h)

			request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
			frames := log.settle(t, h)

			if got := framesTo(frames, student); !slices.Equal(got, []sentFrame{{kind: string(domain.EntityJoinRequest), id: request.String()}}) {
				t.Fatalf("the code entry sent the student %v", got)
			}
		})
	}
}

func TestAcceptAndDeclineNotifyTheStudent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	accepted, acceptedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	declined, declinedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
	code := h.storedCode(t, tutor).Display()
	toAccept := h.enteredRequest(t, acceptedToken, code)
	toDecline := h.enteredRequest(t, declinedToken, code)
	log := h.recordFrames(t)

	card := decodeCard(t, h.acceptRequest(t, tutorToken, toAccept.String()))
	h.mustAnswer(t, h.declineRequest(t, tutorToken, toDecline.String(), false), http.StatusOK)
	frames := log.settle(t, h)
	acceptedRows := h.notificationsOf(t, acceptedToken)
	declinedRows := h.notificationsOf(t, declinedToken)

	if len(acceptedRows) != 1 || acceptedRows[0].Event != string(domain.NotificationJoinRequestAccepted) ||
		acceptedRows[0].Label == nil || *acceptedRows[0].Label != endingTutorAccountName || acceptedRows[0].Colour == nil ||
		acceptedRows[0].Link == nil || acceptedRows[0].Link.Kind != string(domain.LinkMyTutors) || acceptedRows[0].Link.ID != card.ID {
		t.Fatalf("the accepted student sees %+v", acceptedRows)
	}
	if len(declinedRows) != 1 || declinedRows[0].Event != string(domain.NotificationJoinRequestDeclined) || declinedRows[0].Label != nil ||
		declinedRows[0].Colour != nil || declinedRows[0].Link == nil || declinedRows[0].Link.ID != toDecline.String() {
		t.Fatalf("the declined student sees %+v", declinedRows)
	}
	if notificationFrames(frames, accepted) != 1 || !hasFrame(frames, accepted, domain.EntityJoinRequest, toAccept.String()) ||
		!hasFrame(frames, accepted, domain.EntityRelationship, card.ID) {
		t.Fatalf("the accepted student received %v", framesTo(frames, accepted))
	}
	if notificationFrames(frames, declined) != 1 || !hasFrame(frames, declined, domain.EntityJoinRequest, toDecline.String()) {
		t.Fatalf("the declined student received %v", framesTo(frames, declined))
	}
}

func (h *authHarness) endedRows(t *testing.T, user domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM notifications WHERE event = 'relationship.ended' AND user_id = $1::uuid", user.String())
}

func TestEndingNotifiesTheOtherPartyInApp(t *testing.T) {
	t.Parallel()
	t.Run("the tutor ends", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.activeStudentInAGroup(t)

		h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
		h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
		listed := h.notificationsOf(t, parties.studentToken)

		if h.endedRows(t, parties.student) != 1 || h.endedRows(t, parties.tutor) != 0 {
			t.Fatal("ending twice gave other than one row to the student and none to the tutor")
		}
		if listed[0].Label == nil || *listed[0].Label != endingTutorAccountName || listed[0].Colour == nil ||
			listed[0].Link == nil || listed[0].Link.Kind != string(domain.LinkMyTutors) || listed[0].Link.ID != parties.card.ID {
			t.Fatalf("the student sees %+v", listed[0])
		}
	})
	t.Run("the student ends", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.activeStudentInAGroup(t)

		h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
		h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
		listed := h.notificationsOf(t, parties.tutorToken)

		if h.endedRows(t, parties.tutor) != 1 || h.endedRows(t, parties.student) != 0 {
			t.Fatal("stopping twice gave other than one row to the tutor and none to the student")
		}
		ended := listed[slices.IndexFunc(listed, linksTo(domain.NotificationRelationshipEnded, domain.LinkStudentCard, parties.card.ID))]
		if ended.Label == nil || *ended.Label != cardName || ended.Colour != nil {
			t.Fatalf("the tutor sees %+v", ended)
		}
	})
}

func TestDeletionEndingsCreateNoNotificationRows(t *testing.T) {
	t.Parallel()
	for name, deleting := range map[string]func(parties endingParties) string{
		"the student deletes": func(parties endingParties) string { return parties.studentToken },
		"the tutor deletes":   func(parties endingParties) string { return parties.tutorToken },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			parties := h.activeStudentInAGroup(t)

			h.mustAnswer(t, h.deleteMe(t, deleting(parties), accountPassword), http.StatusNoContent)

			if h.endedRows(t, parties.tutor)+h.endedRows(t, parties.student) != 0 {
				t.Fatal("an ending by deletion produced a relationship.ended row")
			}
			if h.storedRelationship(t, parties.card.ID).State != string(domain.RelationshipEnded) {
				t.Fatal("the deletion did not end the relationship")
			}
		})
	}
}

func TestNotificationNewIsSignalledOnlyWhenARowWasInserted(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, conflicting := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, studentEmail)
	_, fresh := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, newcomerEmail)
	code := h.storedCode(t, tutor).Display()
	taken := h.enteredRequest(t, conflicting, code)
	h.enteredRequest(t, fresh, code)
	h.seedNotificationKeyed(t, tutor, domain.JoinRequestCreatedKey(taken, tutor))
	log := h.recordFrames(t)

	h.confirmInOwnBrowser(t, studentEmail, conflicting)
	conflicted := log.settle(t, h)
	h.confirmInOwnBrowser(t, newcomerEmail, fresh)
	inserted := log.settle(t, h)

	if notificationFrames(conflicted, tutor) != 0 || !hasFrame(conflicted, tutor, domain.EntityJoinRequest, taken.String()) {
		t.Fatalf("a release whose row already existed sent the tutor %v", framesTo(conflicted, tutor))
	}
	if notificationFrames(inserted, tutor) != 1 {
		t.Fatalf("a release that inserted a row sent the tutor %v", framesTo(inserted, tutor))
	}
	if h.notificationRows(t, tutor) != 2 {
		t.Fatalf("the tutor has %d rows instead of the seeded and the inserted one", h.notificationRows(t, tutor))
	}
}

func (h *authHarness) seedNotificationKeyed(t *testing.T, recipient domain.UserID, key string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `INSERT INTO notifications (id, user_id, tutor_id, event, subject_id, idempotency_key, created_at)
		VALUES (gen_random_uuid(), $1, $1, 'join_request.created', gen_random_uuid(), $2, $3)`, recipient, key, authStart)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsAreScopedToTheCaller(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	owner, ownerToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
	own := h.seedNotifications(t, owner, tutor, 2, authStart)
	foreign := h.seedNotifications(t, other, tutor, 1, authStart)

	listed := h.notificationsOf(t, ownerToken)
	byIDs := h.markRead(t, ownerToken, fmt.Sprintf(`{"ids":[%q,%q]}`, own[0], foreign[0]))
	through := h.markRead(t, ownerToken, fmt.Sprintf(`{"through":%q}`, foreign[0]))
	both := h.markRead(t, ownerToken, fmt.Sprintf(`{"ids":[%q],"through":%q}`, own[0], foreign[0]))
	random := h.markRead(t, ownerToken, `{"ids":["00000000-0000-4000-8000-000000000000"]}`)

	if len(listed) != 2 || slices.ContainsFunc(listed, func(item notificationItem) bool { return item.ID == foreign[0] }) {
		t.Fatalf("the caller lists %v", listed)
	}
	for name, refused := range map[string]response{"ids": byIDs, "through": through, "both fields": both} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Fatalf("marking another account's row through %s answered %d %s", name, refused.status, refused.body)
		}
	}
	if unread := h.notificationsOf(t, otherToken); unread[0].ReadAt != nil {
		t.Fatal("another account's row was marked read")
	}
	if own := h.notificationsOf(t, ownerToken); own[0].ReadAt != nil || own[1].ReadAt != nil {
		t.Fatal("a refused request marked the caller's rows read")
	}
}

func anotherAccountsNotification(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	other, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
	foreign := h.seedNotifications(t, other, tutor, 1, authStart)
	outOfScope := h.markRead(t, token, fmt.Sprintf(`{"ids":[%q]}`, foreign[0]))
	random := h.markRead(t, token, `{"ids":["00000000-0000-4000-8000-000000000000"]}`)
	return outOfScope, random
}

func TestNotificationListPagesNewestFirstWithoutGapsOrRepeats(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	seeded := h.seedNotifications(t, student, tutor, 23, authStart)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE notifications SET created_at = $1 WHERE user_id = $2 AND id < $3::uuid",
		authStart, student, seeded[11]); err != nil {
		t.Fatal(err)
	}

	var walked []string
	var statuses []int
	cursor := ""
	pages := 0
	for pages < 10 {
		result, page := h.notificationPage(t, token, "?limit=5"+cursor)
		statuses = append(statuses, result.status)
		pages++
		for _, item := range page.Items {
			walked = append(walked, item.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = "&cursor=" + *page.NextCursor
	}
	badCursor, _ := h.notificationPage(t, token, "?cursor=not-a-cursor")
	tooMany, _ := h.notificationPage(t, token, "?limit=101")
	none, _ := h.notificationPage(t, token, "?limit=0")
	defaulted, firstPage := h.notificationPage(t, token, "")

	expected := h.newestFirst(t, student)
	if slices.ContainsFunc(statuses, func(status int) bool { return status != http.StatusOK }) {
		t.Fatalf("the pages answered %v", statuses)
	}
	if !slices.Equal(walked, expected) || pages != 5 {
		t.Fatalf("walking %d pages gave %v, want %v", pages, walked, expected)
	}
	for name, refused := range map[string]response{"a bad cursor": badCursor, "a limit above 100": tooMany, "a limit of 0": none} {
		if refused.status != http.StatusUnprocessableEntity {
			t.Fatalf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	if len(firstPage.Items) != 23 || defaulted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the default page holds %d rows with Cache-Control %q", len(firstPage.Items), defaulted.headers.Get("Cache-Control"))
	}
}

func (h *authHarness) newestFirst(t *testing.T, user domain.UserID) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT id::text FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id DESC", user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestUnreadCountStopsAt100(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	seeded := h.seedNotifications(t, student, tutor, 110, authStart)

	_, full := h.notificationPage(t, token, "?limit=1")
	marked := h.markRead(t, token, fmt.Sprintf(`{"ids":[%s]}`, quotedList(seeded[:15])))
	_, after := h.notificationPage(t, token, "?limit=1")

	if full.UnreadCount != 100 {
		t.Fatalf("110 unread rows count as %d", full.UnreadCount)
	}
	if marked.status != http.StatusOK || !strings.Contains(marked.body, `"unread_count":95`) || after.UnreadCount != 95 {
		t.Fatalf("marking 15 rows answered %d %s and then counted %d", marked.status, marked.body, after.UnreadCount)
	}
}

func quotedList(ids []string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, fmt.Sprintf("%q", id))
	}
	return strings.Join(quoted, ",")
}

func TestMarkingReadThroughARowLeavesNewerRowsUnread(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	seeded := h.seedNotifications(t, student, tutor, 5, authStart)
	log := h.recordFrames(t)

	marked := h.markRead(t, token, fmt.Sprintf(`{"through":%q}`, seeded[2]))
	repeated := h.markRead(t, token, fmt.Sprintf(`{"through":%q}`, seeded[2]))
	frames := log.settle(t, h)
	neither := h.markRead(t, token, `{}`)
	both := h.markRead(t, token, fmt.Sprintf(`{"ids":[%q],"through":%q}`, seeded[0], seeded[1]))
	emptyBesideThrough := h.markRead(t, token, fmt.Sprintf(`{"ids":[],"through":%q}`, seeded[1]))
	empty := h.markRead(t, token, `{"ids":[]}`)
	listed := h.notificationsOf(t, token)

	if marked.status != http.StatusOK || !strings.Contains(marked.body, `"unread_count":2`) || repeated.status != http.StatusOK ||
		marked.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("marking through a row answered %d %s, then %d %s", marked.status, marked.body, repeated.status, repeated.body)
	}
	for index, item := range listed {
		if read := item.ReadAt != nil; read != (index >= 2) {
			t.Fatalf("row %d of the newest-first list is read %t", index, read)
		}
	}
	if !slices.Equal(framesTo(frames, student), []sentFrame{{kind: string(domain.EntityNotification), id: seeded[2]}}) {
		t.Fatalf("marking read sent the caller %v", framesTo(frames, student))
	}
	refusals := map[string]struct {
		answer response
		field  string
	}{
		"neither field":                {neither, "body.ids:required"},
		"both fields":                  {both, "body.through:not_allowed"},
		"an empty list beside through": {emptyBesideThrough, "body.through:not_allowed"},
		"an empty list":                {empty, "body.ids:invalid"},
	}
	for name, refused := range refusals {
		if refused.answer.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, refused.answer), []string{refused.field}) {
			t.Fatalf("%s answered %d %s", name, refused.answer.status, refused.answer.body)
		}
	}
}

func (h *authHarness) requestingStudent(t *testing.T, id byte, email string, name string) (domain.UserID, string) {
	t.Helper()
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, id, email)
	h.setAccount(t, student, name, domain.LanguageEnglish)
	return student, token
}

func createdRowFor(items []notificationItem, rows map[string]string, request domain.JoinRequestID) notificationItem {
	for _, item := range items {
		if rows[item.ID] == request.String() {
			return item
		}
	}
	return notificationItem{}
}

func (h *authHarness) createdRowSubjects(t *testing.T, tutor domain.UserID) map[string]string {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT id::text, subject_id::text FROM notifications WHERE user_id = $1 AND event = 'join_request.created'", tutor)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	subjects := map[string]string{}
	for rows.Next() {
		var id, subject string
		if err := rows.Scan(&id, &subject); err != nil {
			t.Fatal(err)
		}
		subjects[id] = subject
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return subjects
}

func TestLabelsHideADeletedStudentsAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, acceptedToken := h.requestingStudent(t, 2, studentEmail, requestingName)
	_, expiredToken := h.requestingStudent(t, 3, newcomerEmail, endingStudentName)
	_, declinedToken := h.requestingStudent(t, 4, otherEmail, squatterChosenName)
	_, blockedToken := h.requestingStudent(t, 5, "blocked-requester@example.org", keptCardName)
	code := h.storedCode(t, tutor).Display()
	accepted := h.enteredRequest(t, acceptedToken, code)
	expired := h.enteredRequest(t, expiredToken, code)
	declined := h.enteredRequest(t, declinedToken, code)
	blocked := h.enteredRequest(t, blockedToken, code)
	h.mustAnswer(t, h.acceptRequest(t, tutorToken, accepted.String()), http.StatusOK)
	h.mustAnswer(t, h.declineRequest(t, tutorToken, declined.String(), false), http.StatusOK)

	for _, token := range []string{acceptedToken, expiredToken, declinedToken} {
		h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
	}
	_, blockErr := h.operators().Block(context.Background(), "blocked-requester@example.org")
	listed := h.notificationsOf(t, tutorToken)
	raw := h.send(t, http.MethodGet, notificationsPath, "", jsonCall(tutorToken))
	subjects := h.createdRowSubjects(t, tutor)

	if blockErr != nil {
		t.Fatal(blockErr)
	}
	if shown := createdRowFor(listed, subjects, accepted); shown.ID == "" || shown.Label != nil || !shown.LabelDeleted || shown.Link != nil {
		t.Fatalf("the accepted request of a deleted account shows %+v", shown)
	}
	for name, request := range map[string]domain.JoinRequestID{"expired at the deletion": expired, "declined": declined, "operator-blocked": blocked} {
		if shown := createdRowFor(listed, subjects, request); shown.ID == "" || shown.Label != nil || shown.LabelDeleted || shown.Link != nil {
			t.Fatalf("the %s request shows %+v", name, shown)
		}
	}
	for _, private := range []string{requestingName, endingStudentName, squatterChosenName, keptCardName, studentEmail, newcomerEmail, otherEmail} {
		if strings.Contains(raw.body, private) {
			t.Fatalf("the list still shows %q", private)
		}
	}
}

func TestAcceptedRequestLabelsShowTheTutorsCardName(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.requestingStudent(t, 2, studentEmail, requestingName)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	card := decodeCard(t, h.acceptRequest(t, tutorToken, request.String()))
	h.mustAnswer(t, h.cardCall(t, tutorToken, http.MethodPatch, card.ID, "", fmt.Sprintf(`{"display_name":%q}`, keptCardName)), http.StatusOK)

	h.setAccount(t, student, squatterChosenName, domain.LanguageEnglish)
	beforeEnding := createdRowFor(h.notificationsOf(t, tutorToken), h.createdRowSubjects(t, tutor), request)
	h.mustAnswer(t, h.endAsTutor(t, tutorToken, card.ID), http.StatusOK)
	afterEnding := createdRowFor(h.notificationsOf(t, tutorToken), h.createdRowSubjects(t, tutor), request)
	raw := h.send(t, http.MethodGet, notificationsPath, "", jsonCall(tutorToken))

	for name, shown := range map[string]notificationItem{"while active": beforeEnding, "after the ending": afterEnding} {
		if shown.Label == nil || *shown.Label != keptCardName || shown.LabelDeleted || shown.Link != nil {
			t.Fatalf("the accepted request %s shows %+v", name, shown)
		}
	}
	for _, private := range []string{requestingName, squatterChosenName} {
		if strings.Contains(raw.body, private) {
			t.Fatalf("the list shows the account name %q", private)
		}
	}
}

func linked(items []notificationItem) int {
	count := 0
	for _, item := range items {
		if item.Link != nil {
			count++
		}
	}
	return count
}

func TestLinksAreNullOutsideTheCallersScope(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	provisional := h.newCard(t, tutorToken, keptCardName, newcomerEmail)
	h.mustAnswer(t, h.acceptInvitation(t, linkToken(t, provisional.Invitation), newcomerEmail), http.StatusAccepted)
	request := h.enteredRequest(t, studentToken, h.storedCode(t, tutor).Display())
	before := h.notificationsOf(t, tutorToken)

	h.mustAnswer(t, h.removeStudent(t, tutorToken, provisional.ID), http.StatusOK)
	h.mustAnswer(t, h.declineRequest(t, tutorToken, request.String(), false), http.StatusOK)
	after := h.notificationsOf(t, tutorToken)
	declined := h.notificationsOf(t, studentToken)

	if len(before) != 2 || !slices.ContainsFunc(before, linksTo(domain.NotificationInvitationAccepted, domain.LinkStudentCard, provisional.ID)) ||
		!slices.ContainsFunc(before, linksTo(domain.NotificationJoinRequestCreated, domain.LinkJoinRequests, request.String())) {
		t.Fatalf("before the scope changes the rows are %+v", before)
	}
	if len(after) != 2 || linked(after) != 0 {
		t.Fatalf("after the removal and the decline the rows are %+v", after)
	}
	if len(declined) != 1 || !slices.ContainsFunc(declined, linksTo(domain.NotificationJoinRequestDeclined, domain.LinkMyTutors, request.String())) {
		t.Fatalf("the declined student sees %+v", declined)
	}
}

type purgePath func(t *testing.T, h *authHarness, parties endingParties) (domain.UserID, func(t *testing.T))

func TestPurgeDeletesNotificationsNamingThePurgedAccount(t *testing.T) {
	t.Parallel()
	afterRetention := func(h *authHarness, after time.Duration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			h.at(t, h.now(t).Add(after))
			h.tick(t)
		}
	}
	paths := map[string]purgePath{
		"the 30-day purge of a student": func(t *testing.T, h *authHarness, parties endingParties) (domain.UserID, func(t *testing.T)) {
			t.Helper()
			h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
			return parties.student, afterRetention(h, domain.DeletedAccountRetention+time.Hour)
		},
		"the 30-day purge of a tutor": func(t *testing.T, h *authHarness, parties endingParties) (domain.UserID, func(t *testing.T)) {
			t.Helper()
			h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
			return parties.tutor, afterRetention(h, domain.DeletedAccountRetention+time.Hour)
		},
		"the 7-day cleanup": func(t *testing.T, h *authHarness, parties endingParties) (domain.UserID, func(t *testing.T)) {
			t.Helper()
			return h.provisionalWithRows(t, parties), afterRetention(h, 8*24*time.Hour)
		},
		"the immediate purge": func(t *testing.T, h *authHarness, parties endingParties) (domain.UserID, func(t *testing.T)) {
			t.Helper()
			squatter := h.provisionalWithRows(t, parties)
			token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			return squatter, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.deleteMe(t, token, newcomerPassword), http.StatusNoContent)
			}
		},
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
			h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, h.regeneratedLink(t, parties.tutorToken, parties.card.ID)), http.StatusOK)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			purged, purge := path(t, h, parties)
			named := h.rowsNaming(t, purged)

			purge(t)

			if named == 0 {
				t.Fatal("no row named the account before the purge")
			}
			if h.userExists(t, emailOf(t, h, purged, parties)) {
				t.Fatal("the account was not purged")
			}
			if left := h.rowsNaming(t, purged); left != 0 {
				t.Fatalf("%d notification rows still name the purged account", left)
			}
		})
	}
}

func emailOf(t *testing.T, _ *authHarness, user domain.UserID, parties endingParties) string {
	t.Helper()
	switch user {
	case parties.tutor:
		return tutorEmail
	case parties.student:
		return studentEmail
	}
	return newcomerEmail
}

func (h *authHarness) provisionalWithRows(t *testing.T, parties endingParties) domain.UserID {
	t.Helper()
	card := h.newCard(t, parties.tutorToken, keptCardName, newcomerEmail)
	h.mustAnswer(t, h.acceptInvitation(t, linkToken(t, card.Invitation), newcomerEmail), http.StatusAccepted)
	return h.userID(t, newcomerEmail)
}

func TestNoNotificationNamesAPurgedAccountAfterAConcurrentProducer(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unconfirmed := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
	student := h.userID(t, newcomerEmail)
	request := h.enteredRequest(t, unconfirmed, h.storedCode(t, tutor).Display())
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE join_requests SET state = 'pending' WHERE id = $1::uuid", request.String()); err != nil {
		t.Fatal(err)
	}
	release := h.holdUserRow(t, student)

	purge := h.deleteInBackground(unconfirmed, accountPassword)
	h.waitForLockWaiters(t, 1)
	decline := h.sendInBackground(http.MethodPost, joinRequestsPath+"/"+request.String()+"/decline", `{"block":false}`, jsonCall(tutorToken))
	h.waitForLockWaiters(t, 2)
	release()
	purged, declined := awaitAnswer(t, purge), awaitAnswer(t, decline)

	if purged.status != http.StatusNoContent || declined.status != http.StatusNotFound {
		t.Fatalf("the purge answered %d and the waiting decline %d %s", purged.status, declined.status, declined.body)
	}
	if left := h.rowsNaming(t, student); left != 0 {
		t.Fatalf("%d rows name the purged account", left)
	}
}

func TestNotificationsOlderThan90DaysAreDeleted(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.seedNotifications(t, student, tutor, 3, authStart)
	kept := h.seedNotifications(t, tutor, tutor, 2, authStart.Add(24*time.Hour))

	h.at(t, authStart.Add(domain.NotificationRetention+time.Hour))
	h.tick(t)

	if h.notificationRows(t, student) != 0 || h.notificationRows(t, tutor) != int64(len(kept)) {
		t.Fatalf("after the cleanup %d old and %d recent rows remain", h.notificationRows(t, student), h.notificationRows(t, tutor))
	}
	if h.countLogs(`"job":"notifications.cleanup"`, `"result":"ok"`, `"batch_size":3`) != 1 {
		t.Fatalf("the cleanup reported no run of three rows: %s", h.logs.String())
	}
}

func TestNotificationRecordsCarryNoNamesOrEmails(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	code := h.storedCode(t, parties.tutor).Display()
	_, requester := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, newcomerEmail)
	h.setAccount(t, h.userID(t, newcomerEmail), requestingName, domain.LanguageEnglish)

	h.mustAnswer(t, h.decline(t, parties.tutorToken, h.enteredRequest(t, requester, code)), http.StatusOK)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.mustAnswer(t, h.markRead(t, parties.studentToken, fmt.Sprintf(`{"through":%q}`, h.notificationsOf(t, parties.studentToken)[0].ID)),
		http.StatusOK)

	if h.countLogs(`"msg":"notification.created"`) < 4 {
		t.Fatalf("too few notification.created records: %s", h.logs.String())
	}
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if !strings.Contains(line, `"msg":"notification`) {
			continue
		}
		for _, private := range []string{cardName, endingStudentName, endingTutorAccountName, requestingName, studentEmail, newcomerEmail, tutorEmail} {
			if strings.Contains(line, private) {
				t.Fatalf("a notification record holds %q: %s", private, line)
			}
		}
	}
}

func (h *authHarness) decline(t *testing.T, token string, request domain.JoinRequestID) response {
	t.Helper()
	return h.declineRequest(t, token, request.String(), false)
}

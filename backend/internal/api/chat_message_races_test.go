package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type chatWrite struct {
	name      string
	send      func(h *authHarness, bearerToken string, channel string, message string) <-chan pendingAnswer
	unchanged func(t *testing.T, h *authHarness, channel string, message string)
}

func chatWrites() []chatWrite {
	return []chatWrite{
		{name: "a send", send: func(h *authHarness, bearerToken string, channel string, _ string) <-chan pendingAnswer {
			return h.sendMessageInBackground(bearerToken, channel, "Racing words", nil)
		}, unchanged: func(t *testing.T, h *authHarness, channel string, _ string) {
			t.Helper()
			if h.messageCount(t, channel) != 1 {
				t.Fatal("the refused send stored a message")
			}
		}},
		{name: "an edit", send: func(h *authHarness, bearerToken string, _ string, message string) <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, messagesRoute+message, editRequest("Racing edit", 1), jsonCall(bearerToken))
		}, unchanged: func(t *testing.T, h *authHarness, _ string, message string) {
			t.Helper()
			if h.storedMessage(t, message).Version != 1 {
				t.Fatal("the refused edit changed the message")
			}
		}},
		{name: "a delete", send: func(h *authHarness, bearerToken string, _ string, message string) <-chan pendingAnswer {
			return h.sendInBackground(http.MethodDelete, messagesRoute+message, "", jsonCall(bearerToken))
		}, unchanged: func(t *testing.T, h *authHarness, _ string, message string) {
			t.Helper()
			if h.storedMessage(t, message).Deleted {
				t.Fatal("the refused delete removed the message")
			}
		}},
		{name: "a read-marker move", send: func(h *authHarness, bearerToken string, channel string, _ string) <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, channelsRoute+"/"+channel+"/read", `{"seq":1}`, jsonCall(bearerToken))
		}, unchanged: func(t *testing.T, h *authHarness, channel string, _ string) {
			t.Helper()
			for _, member := range h.channelMembers(t, channel) {
				if member.LastRead != 0 {
					t.Fatalf("the refused read moved the marker of %s", member.User)
				}
			}
		}},
	}
}

func TestStudentChatWritesRecheckTheSessionUnderTheRowLock(t *testing.T) {
	t.Parallel()
	for _, write := range chatWrites() {
		t.Run(write.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			parties := h.activeStudentInAGroup(t)
			direct := h.directChannel(t, parties.card.ID)
			message := h.sent(t, parties.studentToken, direct.ID, "Before the sign-out")
			h.exec(t, "UPDATE channel_members SET last_read_seq = 0, shared_read_seq = 0 WHERE channel_id = $1::uuid", direct.ID)
			release := h.holdUserRow(t, parties.student)

			signOut := h.sendInBackground(http.MethodPost, "/api/v1/auth/logout-all", "", jsonCall(parties.studentToken))
			h.waitForLockWaiters(t, 1)
			racing := write.send(h, parties.studentToken, direct.ID, message.ID)
			h.waitForLockWaiters(t, 2)
			release()
			signedOut, refused := awaitAnswer(t, signOut), awaitAnswer(t, racing)

			h.mustAnswer(t, signedOut, http.StatusNoContent)
			if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
				t.Fatalf("%s after the sign-out answered %s", write.name, describe(refused))
			}
			write.unchanged(t, h, direct.ID, message.ID)
			if marker := h.channelMembers(t, direct.ID)[parties.student.String()].LastRead; marker != 0 {
				t.Fatalf("the student's marker moved to %d", marker)
			}
		})
	}
}

func TestTutorChatWritesRacingADeletionOrBlockAnswer401(t *testing.T) {
	t.Parallel()
	for _, write := range chatWrites()[:3] {
		for changeName, change := range racingStateChanges() {
			t.Run(write.name+" after "+changeName, func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
				_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
				card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
				direct := h.directChannel(t, card.ID)
				message := h.sent(t, tutorToken, direct.ID, "Before the change")
				release := h.holdTutorProfile(t, tutor)

				changed := change.start(t, h, tutorToken)
				h.waitForLockWaiters(t, 1)
				racing := write.send(h, tutorToken, direct.ID, message.ID)
				h.waitForLockWaiters(t, 2)
				release()
				awaitChange(t, changed)
				refused := awaitAnswer(t, racing)

				if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
					t.Fatalf("%s after %s answered %s", write.name, changeName, describe(refused))
				}
				write.unchanged(t, h, direct.ID, message.ID)
			})
		}
	}
}

func TestTheDigestFlagIsSetForOfflineMembersWithoutLockingUserRows(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	group := h.groupChannel(t, parties.group.ID)
	releaseStudent := h.holdUserRow(t, parties.student)
	releaseOther := h.holdUserRow(t, other)

	sent := awaitAnswer(t, h.sendMessageInBackground(parties.tutorToken, group.ID, "While their rows are held", nil))
	flagged := h.digestPending(t, parties.student) && h.digestPending(t, other)
	releaseStudent()
	releaseOther()
	h.sent(t, parties.tutorToken, group.ID, "A second message")

	h.mustAnswer(t, sent, http.StatusCreated)
	if !flagged || h.digestPending(t, parties.tutor) {
		t.Fatalf("the send flagged the students %t and the sender %t", flagged, h.digestPending(t, parties.tutor))
	}
	if h.countRows(t, "SELECT count(user_id) FROM digest_pending WHERE user_id = ANY ($1::uuid[])",
		fmt.Sprintf("{%s,%s}", parties.student, other)) != 2 {
		t.Fatal("a second send added another flag")
	}
}

type chargeRace struct {
	arrange func(t *testing.T, h *authHarness) racingWrites
}

func chargeRaces() map[string]chargeRace {
	return map[string]chargeRace{
		"a binding and a group membership change":       {arrange: bindingAndMembershipChange},
		"an author's delete and the tutor's ending":     {arrange: authorDeleteAndEnding},
		"a tutor's delete and a removal":                {arrange: tutorDeleteAndRemoval},
		"the erasure job and a group membership change": {arrange: erasureAndMembershipChange},
		"a student's purge batch and a binding":         {arrange: studentPurgeAndBinding},
		"a tutor's purge batch and an author's delete":  {arrange: tutorPurgeAndAuthorDelete},
	}
}

func TestChargeReleasesMembershipChangesAndEndingsNeverDeadlock(t *testing.T) {
	t.Parallel()
	for name, race := range chargeRaces() {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, reversed %t", name, reversed), func(t *testing.T) {
				t.Parallel()
				h := newFlowHarness(t)
				writes := race.arrange(t, h)
				first, second := writes.first, writes.second
				if reversed {
					first, second = second, first
				}
				release := writes.hold()

				firstAnswer := first()
				h.waitForLockWaiters(t, 1)
				secondAnswer := second()
				h.waitForLockWaiters(t, 2)
				release()
				one, other := awaitAnswer(t, firstAnswer), awaitAnswer(t, secondAnswer)
				if reversed {
					one, other = other, one
				}

				writes.after(t, one, other)
				h.assertMirrored(t)
			})
		}
	}
}

func groupWithAnotherCard(t *testing.T, h *authHarness) (endingParties, studentCard) {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	return parties, h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
}

func bindingAndMembershipChange(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties, otherCard := groupWithAnotherCard(t, h)
	group := h.groupChannel(t, parties.group.ID)
	upload := h.pendingUpload(t, parties.studentToken, group.ID, "notes.txt", 30)
	return racingWrites{
		hold: func() func() {
			return h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", group.ID)
		},
		first: func() <-chan pendingAnswer {
			return h.sendMessageInBackground(parties.studentToken, group.ID, "With a file", []string{upload})
		},
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, groupsRoute+"/"+parties.group.ID, fmt.Sprintf(`{"add":[%q]}`, otherCard.ID), jsonCall(parties.tutorToken))
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the binding and the membership change")(t, first, second)
			if h.usage(t, parties.tutor) != 30 {
				t.Fatalf("the binding charged %d bytes", h.usage(t, parties.tutor))
			}
		},
	}
}

func authorDeleteAndEnding(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "notes.txt", 30)
	message := h.sent(t, parties.studentToken, direct.ID, "With a file", upload)
	return racingWrites{
		hold: func() func() { return h.holdTutorProfile(t, parties.tutor) },
		first: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodDelete, messagesRoute+message.ID, "", jsonCall(parties.studentToken))
		},
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, studentsPath+"/"+parties.card.ID+"/end", "", jsonCall(parties.tutorToken))
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the delete and the ending")(t, first, second)
			if h.usage(t, parties.tutor) != 0 || h.chargesOf(t, upload) != 0 {
				t.Fatal("the delete left its charge")
			}
		},
	}
}

func tutorDeleteAndRemoval(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	squatCard, _ := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.tutorToken, direct.ID, "notes.txt", 30)
	message := h.sent(t, parties.tutorToken, direct.ID, "With a file", upload)
	return racingWrites{
		hold: func() func() { return h.holdTutorProfile(t, parties.tutor) },
		first: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodDelete, messagesRoute+message.ID, "", jsonCall(parties.tutorToken))
		},
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, studentsPath+"/"+squatCard.ID+"/remove", "", jsonCall(parties.tutorToken))
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the tutor's delete and the removal")(t, first, second)
			if h.usage(t, parties.tutor) != 0 {
				t.Fatal("the tutor's delete left its charge")
			}
		},
	}
}

func erasureAndMembershipChange(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties, otherCard := groupWithAnotherCard(t, h)
	squatCard, _ := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	squatChat := h.directChannel(t, squatCard.ID)
	upload := h.pendingUpload(t, parties.tutorToken, squatChat.ID, "notes.txt", 30)
	h.sent(t, parties.tutorToken, squatChat.ID, "With a file", upload)
	h.mustAnswer(t, h.removeStudent(t, parties.tutorToken, squatCard.ID), http.StatusOK)
	return racingWrites{
		hold:  func() func() { return h.holdTutorProfile(t, parties.tutor) },
		first: func() <-chan pendingAnswer { return h.tickAsAnswer(t) },
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, groupsRoute+"/"+parties.group.ID, fmt.Sprintf(`{"add":[%q]}`, otherCard.ID), jsonCall(parties.tutorToken))
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the erasure job and the membership change")(t, first, second)
			if h.messageCount(t, squatChat.ID) != 0 || h.usage(t, parties.tutor) != 0 || !removedFile(h.uploadState(t, upload)) {
				t.Fatalf("the erasure job left %d messages, %d bytes and the file %s: %s", h.messageCount(t, squatChat.ID), h.usage(t, parties.tutor),
					h.uploadState(t, upload), h.failures())
			}
		},
	}
}

func studentPurgeAndBinding(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties, _ := groupWithAnotherCard(t, h)
	group := h.groupChannel(t, parties.group.ID)
	upload := h.pendingUpload(t, parties.studentToken, group.ID, "notes.txt", 30)
	h.sent(t, parties.studentToken, group.ID, "With a file", upload)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	tutorToken := h.relogin(t, tutorEmail)
	tutorUpload := h.pendingUpload(t, tutorToken, group.ID, "tutor.txt", 20)
	return racingWrites{
		hold:  func() func() { return h.holdTutorProfile(t, parties.tutor) },
		first: func() <-chan pendingAnswer { return h.tickAsAnswer(t) },
		second: func() <-chan pendingAnswer {
			return h.sendMessageInBackground(tutorToken, group.ID, "The tutor's file", []string{tutorUpload})
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			answersOK("the student's purge and the binding")(t, first, second)
			if h.userExists(t, studentEmail) || h.usage(t, parties.tutor) != 20 || !removedFile(h.uploadState(t, upload)) {
				t.Fatalf("after the purge and the binding the usage is %d, the student exists %t, the file is %s: %s", h.usage(t, parties.tutor),
					h.userExists(t, studentEmail), h.uploadState(t, upload), h.failures())
			}
		},
	}
}

func tutorPurgeAndAuthorDelete(t *testing.T, h *authHarness) racingWrites {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	upload := h.pendingUpload(t, parties.studentToken, direct.ID, "notes.txt", 30)
	message := h.sent(t, parties.studentToken, direct.ID, "With a file", upload)
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	studentToken := h.relogin(t, studentEmail)
	return racingWrites{
		hold:  func() func() { return h.holdTutorProfile(t, parties.tutor) },
		first: func() <-chan pendingAnswer { return h.tickAsAnswer(t) },
		second: func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodDelete, messagesRoute+message.ID, "", jsonCall(studentToken))
		},
		after: func(t *testing.T, first response, second response) {
			t.Helper()
			if first.status != http.StatusOK || (second.status != http.StatusNoContent && second.status != http.StatusNotFound) {
				t.Fatalf("the tutor's purge answered %s and the author's delete %s", describe(first), describe(second))
			}
			if h.userExists(t, tutorEmail) || h.countRows(t, "SELECT count(*) FROM quota_charges WHERE tutor_id = $1::uuid", parties.tutor.String()) != 0 ||
				h.uploadState(t, upload) == "bound" {
				t.Fatal("the tutor's purge left the tutor, a charge or a bound file")
			}
		},
	}
}

func (h *authHarness) failures() string {
	var failed []string
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if strings.Contains(line, `"level":"ERROR"`) {
			failed = append(failed, line)
		}
	}
	return strings.Join(failed, "\n")
}

func removedFile(state string) bool {
	return state == string(domain.AttachmentDeleted) || state == "gone"
}

package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type mirrorPath struct {
	name string
	act  func(t *testing.T, h *authHarness)
}

func everyMembershipPath() []mirrorPath {
	return []mirrorPath{
		{"an invitation sign-up", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		}},
		{"a signed-in acceptance", func(t *testing.T, h *authHarness) { h.activeStudentInAGroup(t) }},
		{"an accepted join request", func(t *testing.T, h *authHarness) {
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, requestingEmail)
			request := h.enteredRequest(t, studentToken, h.storedCode(t, tutor).Display())
			h.mustAnswer(t, h.acceptRequest(t, tutorToken, request.String()), http.StatusOK)
		}},
		{"a confirmation in the account's own browser", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.mustAnswer(t, h.confirmInOwnBrowser(t, newcomerEmail, accessToken(t, h.login(t, newcomerEmail, newcomerPassword))), http.StatusOK)
		}},
		{"a reclaim", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.reclaimed(t, newcomerEmail)
		}},
		{"the owner keeping a reclaimed link", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			owner := h.reclaimed(t, newcomerEmail)
			h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
		}},
		{"the owner dropping a reclaimed link", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			owner := h.reclaimed(t, newcomerEmail)
			h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}}), http.StatusOK)
		}},
		{"a removal", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
		}},
		{"stopping studying on a provisional link", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.mustAnswer(t, h.stopStudying(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), card.ID), http.StatusNoContent)
		}},
		{"the tutor ending", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
		}},
		{"the student stopping studying", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
		}},
		{"a reactivation", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			link := h.regeneratedLink(t, parties.tutorToken, parties.card.ID)
			h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, link), http.StatusOK)
		}},
		{"adding and removing group members", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
			other := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
			body := fmt.Sprintf(`{"add":[%q],"remove":[%q]}`, other.ID, parties.card.ID)
			h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", body), http.StatusOK)
		}},
		{"archiving", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.archive(t, parties.tutorToken, parties.group.ID), http.StatusOK)
		}},
		{"unarchiving", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.archive(t, parties.tutorToken, parties.group.ID), http.StatusOK)
			h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPost, parties.group.ID, "/unarchive", ""), http.StatusOK)
		}},
		{"the tutor removing a reclaim-pending link", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.reclaimed(t, newcomerEmail)
			h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
		}},
		{"a signed-in acceptance merging onto an ended card", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			fresh := h.newCard(t, parties.tutorToken, "A second card", "")
			h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, linkToken(t, fresh.Invitation)), http.StatusOK)
		}},
		{"a signed-in acceptance merging onto a reclaim-pending card keeps it", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, "", newcomerEmail)
			owner := h.reclaimed(t, newcomerEmail)
			fresh := h.newCard(t, tutorToken, "A second card", newcomerEmail)
			h.mustAnswer(t, h.acceptSignedIn(t, owner, linkToken(t, fresh.Invitation)), http.StatusOK)
		}},
		{"a join request reactivating an ended link", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			request := h.enteredRequest(t, parties.studentToken, h.storedCode(t, parties.tutor).Display())
			h.mustAnswer(t, h.acceptRequest(t, parties.tutorToken, request.String()), http.StatusOK)
		}},
		{"a student's deletion", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
		}},
		{"a tutor's deletion", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
			h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
		}},
		{"the 7-day cleanup", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.advance(t, 8*24*time.Hour)
			h.tick(t)
		}},
		{"the immediate purge of an unconfirmed account", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.mustAnswer(t, h.deleteMe(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), newcomerPassword), http.StatusNoContent)
		}},
		{"reclaim.expire", func(t *testing.T, h *authHarness) {
			_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			h.reclaimed(t, newcomerEmail)
			h.advance(t, domain.ReclaimWindow+time.Hour)
			h.tick(t)
		}},
		{"a student's purge", func(t *testing.T, h *authHarness) {
			parties := h.activeStudentInAGroup(t)
			h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
			h.advance(t, domain.DeletedAccountRetention+time.Hour)
			h.tick(t)
		}},
	}
}

func TestEveryRelationshipAndGroupPathMirrorsChannelMembership(t *testing.T) {
	t.Parallel()
	for _, path := range everyMembershipPath() {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)

			path.act(t, h)

			h.assertMirrored(t)
		})
	}
	t.Run("a new or re-added group member starts at the channel's latest message", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.groupChannel(t, parties.group.ID)
		h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
		h.setChannelSeq(t, channel.ID, 7)

		readded := h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, parties.card.ID))

		h.mustAnswer(t, readded, http.StatusOK)
		member := h.channelMembers(t, channel.ID)[parties.student.String()]
		if member.LastRead != 7 || member.Digested != 7 || member.Suspended {
			t.Fatalf("the re-added member's row is %+v", member)
		}
		h.assertMirrored(t)
	})
	t.Run("keeping a reclaimed link restores the row at the provisional boundary", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
		channel := h.directChannel(t, card.ID)
		h.setChannelSeq(t, channel.ID, 4)
		owner := h.reclaimed(t, newcomerEmail)
		reclaimed := h.directChannel(t, card.ID)
		h.setChannelSeq(t, channel.ID, 6)

		kept := h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}})

		h.mustAnswer(t, kept, http.StatusOK)
		restored := h.channelMembers(t, channel.ID)[squatter.String()]
		if reclaimed.ProvisionalThroughSeq == nil || *reclaimed.ProvisionalThroughSeq != 4 {
			t.Fatalf("the reclaim recorded the provisional boundary %v", reclaimed.ProvisionalThroughSeq)
		}
		if restored.Suspended || restored.LastRead != 4 || restored.Digested != 6 {
			t.Fatalf("the kept member's row is %+v", restored)
		}
		h.assertMirrored(t)
	})
}

func TestAConfirmationSignalsTheTutorsChannelOnly(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	studentToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	log := h.recordFrames(t)

	h.mustAnswer(t, h.confirmInOwnBrowser(t, newcomerEmail, studentToken), http.StatusOK)
	frames := log.settle(t, h)

	channelFrames := func(user domain.UserID) []sentFrame {
		return slices.DeleteFunc(framesTo(frames, user), func(frame sentFrame) bool { return frame.kind != string(domain.EntityChannel) })
	}
	if got := channelFrames(tutor); !slices.Equal(got, []sentFrame{{kind: string(domain.EntityChannel), id: channel.ID}}) {
		t.Fatalf("the confirmation sent the tutor %v", got)
	}
	if got := channelFrames(student); len(got) != 0 {
		t.Fatalf("the confirmation sent the student %v", got)
	}
}

func TestANoOpAcceptanceSendsNoChannelFrame(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	fresh := h.newCard(t, parties.tutorToken, "A second card", "")
	log := h.recordFrames(t)

	h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, linkToken(t, fresh.Invitation)), http.StatusOK)
	frames := log.settle(t, h)

	for _, user := range []domain.UserID{parties.tutor, parties.student} {
		if slices.Contains(entityKinds(frames, user), string(domain.EntityChannel)) {
			t.Fatalf("an acceptance that changed no channel sent %v", framesTo(frames, user))
		}
	}
	h.assertMirrored(t)
}

func TestEndingAndArchivingMakeChannelsReadOnlyAndReactivationRestores(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)

	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	_, endedForTutor := h.channelCall(t, parties.tutorToken, direct.ID)
	endedRead, endedForStudent := h.channelCall(t, parties.studentToken, direct.ID)
	leftGroup, _ := h.channelCall(t, parties.studentToken, group.ID)
	h.mustAnswer(t, h.archive(t, parties.tutorToken, parties.group.ID), http.StatusOK)
	_, archived := h.channelCall(t, parties.tutorToken, group.ID)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPost, parties.group.ID, "/unarchive", ""), http.StatusOK)
	_, unarchived := h.channelCall(t, parties.tutorToken, group.ID)
	h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, h.regeneratedLink(t, parties.tutorToken, parties.card.ID)), http.StatusOK)
	_, reactivated := h.channelCall(t, parties.studentToken, direct.ID)

	if endedForTutor.Writable || endedForStudent.Writable || endedRead.status != http.StatusOK || endedForTutor.Online != nil {
		t.Fatalf("after the ending the tutor sees %+v and the student %d %+v", endedForTutor, endedRead.status, endedForStudent)
	}
	if leftGroup.status != http.StatusNotFound {
		t.Fatalf("the ended student still reaches the group channel: %d %s", leftGroup.status, leftGroup.body)
	}
	if archived.Writable || !unarchived.Writable {
		t.Fatalf("the archived group's channel is writable %t, after unarchiving %t", archived.Writable, unarchived.Writable)
	}
	if !reactivated.Writable || reactivated.ID != direct.ID || reactivated.Online == nil || *reactivated.Online {
		t.Fatalf("the reactivated relationship's channel is %+v", reactivated)
	}
}

func TestARemovedGroupMemberLosesTheChannel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	group := h.groupChannel(t, parties.group.ID)
	_, before := h.channelCall(t, parties.studentToken, group.ID)

	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
	refused, _ := h.channelCall(t, parties.studentToken, group.ID)
	_, listed := h.channelList(t, parties.studentToken, "")
	_, header := h.channelCall(t, parties.tutorToken, group.ID)

	if before.ID != group.ID || refused.status != http.StatusNotFound || slices.Contains(listedChannelIDs(listed), group.ID) {
		t.Fatalf("the removed member reaches the group channel: %d, listed %v", refused.status, listedChannelIDs(listed))
	}
	if len(header.Members) != 0 || header.Members == nil {
		t.Fatalf("the tutor's header still lists %v", header.Members)
	}
	if _, kept := h.channelMembers(t, group.ID)[parties.student.String()]; kept {
		t.Fatal("the removed member's row survived")
	}
}

func TestRemovingAGroupMemberDeletesTheirPendingUploadsToTheChannel(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, requestingEmail)
	other := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, other.ID)), http.StatusOK)
	group := h.groupChannel(t, parties.group.ID)
	direct := h.directChannel(t, parties.card.ID)
	h.pendingChatUpload(t, "0b3c7a52-3f7e-4e21-9a3d-000000000001", parties.student, group.ID)
	h.pendingChatUpload(t, "0b3c7a52-3f7e-4e21-9a3d-000000000002", parties.student, direct.ID)
	h.pendingChatUpload(t, "0b3c7a52-3f7e-4e21-9a3d-000000000003", h.userID(t, requestingEmail), group.ID)

	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)

	if h.uploadState(t, "0b3c7a52-3f7e-4e21-9a3d-000000000001") != "deleted" || !h.deletionQueued(t, "0b3c7a52-3f7e-4e21-9a3d-000000000001") {
		t.Fatal("the removed member's pending upload to the group channel survived")
	}
	for _, kept := range []string{"0b3c7a52-3f7e-4e21-9a3d-000000000002", "0b3c7a52-3f7e-4e21-9a3d-000000000003"} {
		if h.uploadState(t, kept) != "pending" || h.deletionQueued(t, kept) {
			t.Fatalf("the upload %s of another channel or member was deleted", kept)
		}
	}
}

type channelDiscardPath struct {
	name    string
	discard func(t *testing.T, h *authHarness, tutorToken string, card studentCard)
}

func everyChannelDiscardPath() []channelDiscardPath {
	return []channelDiscardPath{
		{"a removal", func(t *testing.T, h *authHarness, tutorToken string, card studentCard) {
			h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
		}},
		{"stopping studying on the provisional link", func(t *testing.T, h *authHarness, _ string, card studentCard) {
			h.mustAnswer(t, h.stopStudying(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), card.ID), http.StatusNoContent)
		}},
		{"the 7-day cleanup", func(t *testing.T, h *authHarness, _ string, _ studentCard) {
			h.advance(t, 8*24*time.Hour)
			h.tick(t)
		}},
		{"the immediate purge of the unconfirmed account", func(t *testing.T, h *authHarness, _ string, _ studentCard) {
			h.mustAnswer(t, h.deleteMe(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)), newcomerPassword), http.StatusNoContent)
		}},
		{"a dropped reclaim link with its suspended row", func(t *testing.T, h *authHarness, _ string, card studentCard) {
			owner := h.reclaimed(t, newcomerEmail)
			h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}}), http.StatusOK)
		}},
		{"reclaim.expire", func(t *testing.T, h *authHarness, _ string, _ studentCard) {
			h.reclaimed(t, newcomerEmail)
			h.advance(t, domain.ReclaimWindow+time.Hour)
			h.tick(t)
		}},
		{"the tutor's deletion", func(t *testing.T, h *authHarness, tutorToken string, _ studentCard) {
			h.mustAnswer(t, h.deleteMe(t, tutorToken, accountPassword), http.StatusNoContent)
		}},
	}
}

func TestDiscardingAProvisionalLinkQueuesItsChannelsErasure(t *testing.T) {
	t.Parallel()
	for _, path := range everyChannelDiscardPath() {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
			channel := h.directChannel(t, card.ID)
			upload, streaming := "0b3c7a52-3f7e-4e21-9a3d-0000000000aa", fixtureID(9, 1)
			h.pendingChatUpload(t, upload, squatter, channel.ID)
			h.insertUploading(t, streaming, tutor, channel.ID, tutor, 1<<10, "", authStart.Add(30*24*time.Hour))

			path.discard(t, h, tutorToken, card)
			queued := h.directChannel(t, card.ID)
			members := h.channelMembers(t, channel.ID)
			streamingState := h.uploadState(t, streaming)
			hidden := response{status: http.StatusNotFound}
			if path.name != "the tutor's deletion" {
				hidden, _ = h.channelCall(t, h.relogin(t, tutorEmail), channel.ID)
			}
			h.advance(t, time.Minute)
			h.tick(t)
			erased := h.directChannel(t, card.ID)

			if !queued.Discarded || len(members) != 0 || h.countLogs(`"msg":"channel.erase_queued"`, channel.ID) != 1 {
				t.Fatalf("the discarded channel is %+v with members %v", queued, members)
			}
			if state := h.uploadState(t, upload); (state != "deleted" && state != "gone") || !h.deletionQueued(t, upload) {
				t.Fatalf("the channel's pending upload is %s after its discard", state)
			}
			if streamingState != string(domain.AttachmentUploading) || h.uploadState(t, streaming) != string(domain.AttachmentUploading) ||
				h.deletionQueued(t, streaming) {
				t.Fatalf("the channel's uploading row was not left to its request: %s", streamingState)
			}
			if hidden.status != http.StatusNotFound {
				t.Fatalf("the tutor still reaches the discarded channel: %d %s", hidden.status, hidden.body)
			}
			if !erased.Discarded || erased.ErasePending {
				t.Fatalf("after the erasure job the channel is %+v", erased)
			}
		})
	}
}

func TestASuspendedMemberReachesNothingAndIsShownToNoOne(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, owner := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	attachment := h.boundUpload(t, tutorToken, channel.ID, "welcome.txt", plainText(32), 1)
	ownerToken := h.reclaimed(t, newcomerEmail)
	h.setChannelSeq(t, channel.ID, 3)
	log := h.recordFrames(t)

	h.mustAnswer(t, h.patchMe(t, ownerToken, `{"name":"A chosen name"}`), http.StatusOK)
	h.mustAnswer(t, h.cardCall(t, tutorToken, http.MethodPatch, card.ID, "", `{"display_name":"A renamed card"}`), http.StatusOK)
	frames := log.settle(t, h)
	listedResult, listed := h.channelList(t, ownerToken, "")
	direct, _ := h.channelCall(t, ownerToken, channel.ID)
	random, _ := h.channelCall(t, ownerToken, randomChannelID)
	unread := h.send(t, http.MethodGet, channelsRoute+"/unread", "", jsonCall(ownerToken))
	_, forTutor := h.channelCall(t, tutorToken, channel.ID)
	_, tutorList := h.channelList(t, tutorToken, "")
	ownerUpload := h.chatUpload(t, ownerToken, channel.ID, "notes.txt", plainText(32))
	randomUpload := h.chatUpload(t, ownerToken, randomChannelID, "notes.txt", plainText(32))
	ownerAddress, _ := h.addressOf(t, ownerToken, attachment, "")
	randomAddress, _ := h.addressOf(t, ownerToken, fixtureID(9, 2), "")
	tutorAddress, _ := h.addressOf(t, tutorToken, attachment, "")

	if ownerUpload.status != http.StatusNotFound || refusal(t, ownerUpload) != refusal(t, randomUpload) || len(h.uploadsBy(t, owner)) != 0 {
		t.Fatalf("the suspended account uploads to its channel: %s", describe(ownerUpload))
	}
	if ownerAddress.status != http.StatusNotFound || refusal(t, ownerAddress) != refusal(t, randomAddress) || tutorAddress.status != http.StatusOK {
		t.Fatalf("the suspended account's download address answered %s, the tutor's %s", describe(ownerAddress), describe(tutorAddress))
	}
	if listedResult.status != http.StatusOK || len(listed.Items) != 0 || listed.UnreadChannels != 0 {
		t.Fatalf("the suspended account lists %d %+v", listedResult.status, listed)
	}
	if direct.status != http.StatusNotFound || refusal(t, direct) != refusal(t, random) {
		t.Fatalf("the suspended account reads its channel: %d %s", direct.status, direct.body)
	}
	if unread.status != http.StatusOK || unreadCount(t, unread) != 0 {
		t.Fatalf("the suspended account's unread count is %d %s", unread.status, unread.body)
	}
	if forTutor.UserID != nil || forTutor.Online != nil || len(forTutor.Members) != 0 || forTutor.Unconfirmed || !forTutor.Writable {
		t.Fatalf("the tutor is shown the suspended account: %+v", forTutor)
	}
	if len(tutorList.Items) != 1 || tutorList.Items[0].UserID != nil || tutorList.Items[0].Online != nil || tutorList.UnreadChannels != 1 {
		t.Fatalf("the tutor's list shows %+v", tutorList)
	}
	if got := framesTo(frames, owner); len(got) != 0 {
		t.Fatalf("the suspended account was sent %v", got)
	}
	if got := framesTo(frames, tutor); !slices.Equal(got, []sentFrame{{kind: string(domain.EntityChannel), id: channel.ID}, {kind: string(domain.EntityRelationship), id: card.ID}}) {
		t.Fatalf("the tutor received %v, which must hold nothing for the account's own name edit", got)
	}
}

func TestPurgesDeleteTheAccountsMemberRowsUnderTheChannelLocks(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	h.exec2(t, `INSERT INTO channel_members (channel_id, user_id, suspended_since, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, now(), now(), now())`, group.ID, parties.student.String())
	other, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, "another-tutor@example.org")
	h.enteredRequest(t, parties.studentToken, h.storedCode(t, other).Display())
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.advance(t, domain.DeletedAccountRetention+time.Hour)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR SHARE", direct.ID)

	purge := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	whileHeld := len(h.channelMembers(t, direct.ID)) + len(h.channelMembers(t, group.ID))
	requestsTaken := h.lockedElsewhere(t, "join_requests", "student_id", parties.student.String())
	release()
	awaitTick(t, purge)

	if whileHeld != 4 {
		t.Fatalf("the purge removed member rows before it held the channel locks: %d rows left", whileHeld)
	}
	if !requestsTaken {
		t.Fatal("the purge waited for the channel before it deleted the account's join requests")
	}
	for name, channel := range map[string]string{"direct": direct.ID, "group": group.ID} {
		members := h.channelMembers(t, channel)
		if _, left := members[parties.student.String()]; left || len(members) != 1 {
			t.Fatalf("after the purge the %s channel holds %v", name, members)
		}
	}
	if h.userExists(t, studentEmail) {
		t.Fatal("the student was not purged")
	}
}

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

const unknownAttachment = "00000000-0000-4000-8000-0000000000aa"

func TestGroupMembersHideTheAccountOfACardThatDoesNotShowIt(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	members := h.activeCards(t, tutor, 10, 4)
	for _, member := range []byte{10, 11, 12, 13} {
		h.withAvatar(t, h.studentToken(t, member))
	}
	group := decodeGroup(t, h.createGroup(t, token, newGroupBody(groupName, members), ""))
	h.setRelationshipState(t, members[1], domain.RelationshipReclaimPending)
	h.setRelationshipState(t, members[2], domain.RelationshipDiscarded)
	h.markDeleted(t, domain.UserID{13}, h.now(t))

	read := decodeGroup(t, h.groupCall(t, token, http.MethodGet, group.ID, "", ""))
	listed := h.listGroups(t, token)

	if len(listed) != 1 {
		t.Fatalf("the list shows %+v", listed)
	}
	for label, shown := range map[string][]groupMember{"the group": read.Members, "the list preview": listed[0].Members} {
		if len(shown) != 4 {
			t.Fatalf("%s holds %+v", label, shown)
		}
		if shown[0].Name == nil || *shown[0].Name != "Member account 10" || shown[0].AvatarID == nil {
			t.Fatalf("%s hides the account of an active card: %+v", label, shown[0])
		}
		for _, hidden := range shown[1:] {
			if hidden.Name != nil || hidden.AvatarID != nil || hidden.DisplayName != seededCardName {
				t.Fatalf("%s shows the account of a card that does not show it: %+v", label, hidden)
			}
		}
	}
	logs := h.logs.String()
	for _, card := range members[1:] {
		record := `"msg":"group.member.hidden_account","tutor_id":"` + tutor.String() + `","group_id":"` + group.ID + `","relationship_id":"` + card + `"`
		if !strings.Contains(logs, record) {
			t.Fatalf("no record %s in %s", record, logs)
		}
	}
	if strings.Contains(logs, "Member account 1") {
		t.Fatal("the logs carry a member's account name")
	}
}

func TestATutorCannotFetchAnAnswerFileThroughAReclaimPendingOrDiscardedLink(t *testing.T) {
	t.Parallel()
	for _, state := range []domain.RelationshipState{domain.RelationshipReclaimPending, domain.RelationshipDiscarded} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			f := newErasureFixture(t)
			h := f.h
			h.setRelationshipState(t, f.card, domain.RelationshipActive)
			before, earlier := h.addressOf(t, f.tutorToken, f.file, "?ref=answer:"+f.submission)
			h.mustAnswer(t, before, http.StatusOK)
			h.setRelationshipState(t, f.card, state)

			minted, _ := h.addressOf(t, f.tutorToken, f.file, "?ref=answer:"+f.submission)
			unknown, _ := h.addressOf(t, f.tutorToken, unknownAttachment, "?ref=answer:"+f.submission)
			served := h.fetch(t, earlier.URL, nil)

			assertRefused(t, "the tutor's mint through a "+string(state)+" link", minted, http.StatusNotFound, "not_found")
			if refusalOf(t, minted) != refusalOf(t, unknown) {
				t.Fatalf("the refusal %s differs from a random identifier's %s", minted.body, unknown.body)
			}
			if served.status != http.StatusNotFound {
				t.Fatalf("serving an address minted before the link became %s answered %d", state, served.status)
			}
		})
	}
}

func TestATutorStillFetchesAnAnswerFileThroughAnActiveOrEndedLink(t *testing.T) {
	t.Parallel()
	for _, state := range []domain.RelationshipState{domain.RelationshipActive, domain.RelationshipEnded} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			f := newErasureFixture(t)
			h := f.h
			h.setRelationshipState(t, f.card, state)

			minted, address := h.addressOf(t, f.tutorToken, f.file, "?ref=answer:"+f.submission)
			served := h.fetch(t, address.URL, nil)

			h.mustAnswer(t, minted, http.StatusOK)
			if served.status != http.StatusOK {
				t.Fatalf("serving the tutor's address through a %s link answered %d", state, served.status)
			}
		})
	}
}

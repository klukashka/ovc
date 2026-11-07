package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type relationshipChange struct {
	tutor   domain.UserID
	student domain.UserID
	id      string
	act     func(t *testing.T)
	toTutor bool
	toStudy bool
}

type relationshipChangePath struct {
	name    string
	arrange func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange
}

func provisionalLink(t *testing.T, h *authHarness) (domain.UserID, string, studentCard, domain.UserID) {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	return tutor, tutorToken, card, squatter
}

type numberedAccounts struct {
	number byte
}

func (a numberedAccounts) tutorEmail() string {
	return fmt.Sprintf("tutor-%d@example.org", a.number)
}

func (a numberedAccounts) studentEmail() string {
	return fmt.Sprintf("student-%d@example.org", a.number)
}

func (a numberedAccounts) newcomerEmail() string {
	return fmt.Sprintf("newcomer-%d@example.org", a.number)
}

func (h *authHarness) numberedTutor(t *testing.T, accounts numberedAccounts) (domain.UserID, string) {
	t.Helper()
	return h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 0x40+accounts.number, accounts.tutorEmail())
}

func (h *authHarness) numberedProvisionalLink(t *testing.T, accounts numberedAccounts) (domain.UserID, string, studentCard, domain.UserID) {
	t.Helper()
	tutor, tutorToken := h.numberedTutor(t, accounts)
	card, squatter := h.squat(t, tutorToken, accounts.newcomerEmail(), accounts.newcomerEmail())
	return tutor, tutorToken, card, squatter
}

func (h *authHarness) numberedParties(t *testing.T, accounts numberedAccounts) endingParties {
	t.Helper()
	tutor, tutorToken := h.numberedTutor(t, accounts)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 0x60+accounts.number, accounts.studentEmail())
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, accounts.studentEmail())
	return endingParties{tutor: tutor, tutorToken: tutorToken, student: student, studentToken: studentToken, card: card}
}

func relationshipChangePaths() []relationshipChangePath {
	return []relationshipChangePath{
		{"linking", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			tutor, tutorToken := h.numberedTutor(t, accounts)
			student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 0x60+accounts.number, accounts.studentEmail())
			card := h.newCard(t, tutorToken, cardName, accounts.studentEmail())
			return relationshipChange{tutor: tutor, student: student, id: card.ID, toTutor: true, toStudy: true, act: func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.acceptSignedIn(t, token, linkToken(t, card.Invitation)), http.StatusOK)
			}}
		}},
		{"confirmation", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			tutor, _, card, squatter := h.numberedProvisionalLink(t, accounts)
			token := accessToken(t, h.login(t, accounts.newcomerEmail(), newcomerPassword))
			return relationshipChange{tutor: tutor, student: squatter, id: card.ID, toTutor: true, toStudy: true, act: func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.confirmInOwnBrowser(t, accounts.newcomerEmail(), token), http.StatusOK)
			}}
		}},
		{"reclaim", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			tutor, _, card, squatter := h.numberedProvisionalLink(t, accounts)
			return relationshipChange{tutor: tutor, student: squatter, id: card.ID, toTutor: true, toStudy: true, act: func(t *testing.T) {
				t.Helper()
				h.reclaimed(t, accounts.newcomerEmail())
			}}
		}},
		{"the owner's choice", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			tutor, _, card, squatter := h.numberedProvisionalLink(t, accounts)
			owner := h.reclaimed(t, accounts.newcomerEmail())
			return relationshipChange{tutor: tutor, student: squatter, id: card.ID, toTutor: true, toStudy: true, act: func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
			}}
		}},
		{"removal", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			tutor, tutorToken, card, squatter := h.numberedProvisionalLink(t, accounts)
			return relationshipChange{tutor: tutor, student: squatter, id: card.ID, toTutor: true, toStudy: true, act: func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
			}}
		}},
		{"ending by the tutor", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			return parties.change(true, true, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			})
		}},
		{"ending by the student", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			return parties.change(true, true, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
			})
		}},
		{"reactivation", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
			link := h.regeneratedLink(t, parties.tutorToken, parties.card.ID)
			return parties.change(true, true, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.acceptSignedIn(t, parties.studentToken, link), http.StatusOK)
			})
		}},
		{"tutor deletion", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			return parties.change(false, true, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
			})
		}},
		{"student deletion", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			return parties.change(true, true, func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
			})
		}},
		{"purge", func(t *testing.T, h *authHarness, accounts numberedAccounts) relationshipChange {
			t.Helper()
			parties := h.numberedParties(t, accounts)
			h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
			return parties.change(true, false, func(t *testing.T) {
				t.Helper()
				h.at(t, h.now(t).Add(domain.DeletedAccountRetention+time.Hour))
				h.tick(t)
			})
		}},
	}
}

func (p endingParties) change(toTutor bool, toStudent bool, act func(t *testing.T)) relationshipChange {
	return relationshipChange{tutor: p.tutor, student: p.student, id: p.card.ID, act: act, toTutor: toTutor, toStudy: toStudent}
}

func TestEveryRelationshipChangePublishesEntityChangedToItsParties(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	log := h.recordFrames(t)
	var number byte
	for _, path := range relationshipChangePaths() {
		number++
		accounts := numberedAccounts{number: number}
		t.Run(path.name, func(t *testing.T) {
			change := path.arrange(t, h, accounts)
			log.settle(t, h)

			change.act(t)
			frames := log.settle(t, h)

			if got := hasFrame(frames, change.tutor, domain.EntityRelationship, change.id); got != change.toTutor {
				t.Fatalf("the tutor received a relationship frame %t, want %t: %v", got, change.toTutor, framesTo(frames, change.tutor))
			}
			if got := hasFrame(frames, change.student, domain.EntityRelationship, change.id); got != change.toStudy {
				t.Fatalf("the student received a relationship frame %t, want %t: %v", got, change.toStudy, framesTo(frames, change.student))
			}
		})
	}
}

func (h *authHarness) freshCardOf(t *testing.T, tutor domain.UserID) string {
	t.Helper()
	cards := h.cardsOf(t, tutor, domain.RelationshipInvited)
	if len(cards) != 1 {
		t.Fatalf("the tutor holds %d invited cards instead of the fresh one", len(cards))
	}
	return cards[0].ID
}

func TestARemovedOrDroppedStudentReceivesNothingMoreAboutTheRelationship(t *testing.T) {
	t.Parallel()
	losses := map[string]func(t *testing.T, h *authHarness, tutorToken string, card studentCard) func(t *testing.T){
		"a removal": func(_ *testing.T, h *authHarness, tutorToken string, card studentCard) func(t *testing.T) {
			return func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.removeStudent(t, tutorToken, card.ID), http.StatusOK)
			}
		},
		"a dropped reclaim link": func(t *testing.T, h *authHarness, _ string, card studentCard) func(t *testing.T) {
			t.Helper()
			owner := h.reclaimed(t, newcomerEmail)
			return func(t *testing.T) {
				t.Helper()
				h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}}), http.StatusOK)
			}
		},
	}
	for name, lose := range losses {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, tutorToken, card, student := provisionalLink(t, h)
			channel := h.directChannel(t, card.ID)
			loss := lose(t, h, tutorToken, card)
			log := h.recordFrames(t)

			loss(t)
			lost := log.settle(t, h)
			fresh := h.freshCardOf(t, tutor)
			h.mustAnswer(t, h.cardCall(t, tutorToken, http.MethodPatch, fresh, "", `{"notes":"After the loss"}`), http.StatusOK)
			h.regeneratedLink(t, tutorToken, fresh)
			later := log.settle(t, h)

			lastFrames := []sentFrame{{kind: string(domain.EntityChannel), id: channel.ID}, {kind: string(domain.EntityRelationship), id: card.ID}}
			if name == "a dropped reclaim link" {
				lastFrames = lastFrames[1:]
			}
			if got := framesTo(lost, student); !slices.Equal(got, lastFrames) {
				t.Fatalf("the loss sent the student %v instead of one last frame of each kind it still saw", got)
			}
			if got := framesTo(later, student); len(got) != 0 {
				t.Fatalf("after the loss the student received %v", got)
			}
			if !hasFrame(later, tutor, domain.EntityRelationship, fresh) {
				t.Fatalf("the tutor received %v for the fresh card", framesTo(later, tutor))
			}
		})
	}
}

func TestFrameRecipientsAreReadUnderTheRelationshipLock(t *testing.T) {
	t.Parallel()
	h := newFlowHarnessForLockContention(t)
	_, tutorToken, card, student := provisionalLink(t, h)
	channel := h.directChannel(t, card.ID)
	studentToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	log := h.recordFrames(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", card.ID)

	removal := h.sendInBackground(http.MethodPost, studentsPath+"/"+card.ID+"/remove", "", jsonCall(tutorToken))
	h.waitForLockWaiters(t, 1)
	edit := h.sendInBackground(http.MethodPatch, studentsPath+"/"+card.ID, `{"notes":"Written while the removal waited"}`, jsonCall(tutorToken))
	stop := h.sendInBackground(http.MethodPost, tutorsPath+card.ID+"/end", "", jsonCall(studentToken))
	h.waitForLockWaiters(t, 3)
	release()
	removed, edited, stopped := awaitAnswer(t, removal), awaitAnswer(t, edit), awaitAnswer(t, stop)
	frames := log.settle(t, h)

	if removed.status != http.StatusOK || edited.status != http.StatusNotFound || stopped.status != http.StatusNotFound {
		t.Fatalf("the removal answered %d, the waiting edit %d %s and the waiting stop %d %s",
			removed.status, edited.status, edited.body, stopped.status, stopped.body)
	}
	lastFrames := []sentFrame{{kind: string(domain.EntityChannel), id: channel.ID}, {kind: string(domain.EntityRelationship), id: card.ID}}
	if got := framesTo(frames, student); !slices.Equal(got, lastFrames) {
		t.Fatalf("the removed student received %v", got)
	}
}

func TestNoFrameReachesAnotherTutorSharingTheStudent(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	parties := h.activeStudentInAGroup(t)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 5, secondTutorEmail)
	h.shareStudentWith(t, other, parties.student)
	_, requesterToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, newcomerEmail)
	watching := h.connect(t, otherToken)
	acting := h.connect(t, parties.tutorToken)
	log := h.recordFrames(t)

	h.mustAnswer(t, h.cardCall(t, parties.tutorToken, http.MethodPatch, parties.card.ID, "", `{"notes":"Only mine"}`), http.StatusOK)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	request := h.enteredRequest(t, requesterToken, h.storedCode(t, parties.tutor).Display())
	frames := log.settle(t, h.authHarness)
	h.entityChanged(t, []domain.UserID{other}, 0x99)

	if got := framesTo(frames, other); len(got) != 0 {
		t.Fatalf("the other tutor was named in %v", got)
	}
	if first := watching.expect(t, domain.FrameEntityChanged); !sentinelPayload(t, first, domain.EntityID{0x99}) {
		t.Fatalf("the other tutor's socket received %s before the sentinel", first.Payload)
	}
	if received := acting.expect(t, domain.FrameEntityChanged); !sentinelPayload(t, received, mustEntity(t, parties.card.ID)) {
		t.Fatalf("the acting tutor's socket received %s first", received.Payload)
	}
	if !hasFrame(frames, parties.tutor, domain.EntityJoinRequest, request.String()) || notificationFrames(frames, parties.tutor) != 1 {
		t.Fatalf("the acting tutor was sent %v", framesTo(frames, parties.tutor))
	}
}

func mustEntity(t *testing.T, text string) domain.EntityID {
	t.Helper()
	id, valid := domain.ParseUUID[domain.EntityID](text)
	if !valid {
		t.Fatalf("%q is not an identifier", text)
	}
	return id
}

func sentinelPayload(t *testing.T, frame serverFrame, id domain.EntityID) bool {
	t.Helper()
	var payload struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("not an entity payload: %s", frame.Payload)
	}
	return payload.Kind == string(domain.EntityRelationship) && payload.ID == id.String()
}

func TestTutorDeletionSplitsItsSignalsAndLocksNoStudentRow(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	students := make(map[domain.UserID]string, 3)
	for index := range 3 {
		student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(10+index), fmt.Sprintf("student-%d@example.org", index))
		students[student] = h.linkedSignedIn(t, tutorToken, token, cardName, fmt.Sprintf("student-%d@example.org", index)).ID
	}
	var held domain.UserID
	for student := range students {
		held = student
		break
	}
	release := h.holdUserRow(t, held)
	log := h.recordFrames(t)

	deleted := awaitAnswer(t, h.deleteInBackground(tutorToken, accountPassword))
	release()
	frames := log.settle(t, h)

	if deleted.status != http.StatusNoContent {
		t.Fatalf("the tutor's deletion answered %d %s while a student's row was held", deleted.status, deleted.body)
	}
	for student, relationship := range students {
		readOnly := sentFrame{kind: string(domain.EntityChannel), id: h.directChannel(t, relationship).ID}
		if got := framesTo(frames, student); !slices.Equal(got, []sentFrame{readOnly, {kind: string(domain.EntityRelationship), id: relationship}}) {
			t.Fatalf("the student received %v", got)
		}
	}
	if got := framesTo(frames, tutor); len(got) != 0 {
		t.Fatalf("the deleted tutor was sent %v", got)
	}
}

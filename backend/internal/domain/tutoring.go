package domain

import (
	"strings"
	"time"
	"unicode"
)

type RelationshipID [16]byte

type InvitationID [16]byte

type JoinRequestID [16]byte

type GroupID [16]byte

type RelationshipState string

const (
	RelationshipInvited        RelationshipState = "invited"
	RelationshipProvisional    RelationshipState = "provisional"
	RelationshipReclaimPending RelationshipState = "reclaim_pending"
	RelationshipActive         RelationshipState = "active"
	RelationshipEnded          RelationshipState = "ended"
	RelationshipDiscarded      RelationshipState = "discarded"
)

var RelationshipStates = []RelationshipState{
	RelationshipInvited, RelationshipProvisional, RelationshipReclaimPending, RelationshipActive, RelationshipEnded, RelationshipDiscarded,
}

type DiscardReason string

const (
	DiscardWithdrawn     DiscardReason = "withdrawn"
	DiscardMerged        DiscardReason = "merged"
	DiscardRemoved       DiscardReason = "removed"
	DiscardTutorDeleted  DiscardReason = "tutor_deleted"
	DiscardStudentPurged DiscardReason = "student_purged"
)

type EndedBy string

const (
	EndedByTutor    EndedBy = "tutor"
	EndedByStudent  EndedBy = "student"
	EndedByDeletion EndedBy = "deletion"
)

type RevokeReason string

const (
	RevokeRegenerated   RevokeReason = "regenerated"
	RevokeWithdrawn     RevokeReason = "withdrawn"
	RevokeEmailChanged  RevokeReason = "email_changed"
	RevokeMismatchLimit RevokeReason = "mismatch_limit"
	RevokeLinked        RevokeReason = "linked"
	RevokeRemoved       RevokeReason = "removed"
	RevokeTutorDeleted  RevokeReason = "tutor_deleted"
	RevokeExpiredOnSend RevokeReason = "expired_on_send"
)

const InvitationMismatchLimit int16 = 5

const (
	InvitationTokenBytes = 32
	InvitationLifetime   = 14 * 24 * time.Hour
)

type InvitationStatus string

const (
	InvitationOpen                   InvitationStatus = "open"
	InvitationExpired                InvitationStatus = "expired"
	InvitationRevokedAfterMismatches InvitationStatus = "revoked_after_mismatches"
)

func (id RelationshipID) String() string {
	return formatUUID(id)
}

func (id GroupID) String() string {
	return formatUUID(id)
}

func (id InvitationID) String() string {
	return formatUUID(id)
}

type JoinRequestState string

const (
	JoinRequestHeld     JoinRequestState = "held"
	JoinRequestPending  JoinRequestState = "pending"
	JoinRequestAccepted JoinRequestState = "accepted"
	JoinRequestDeclined JoinRequestState = "declined"
	JoinRequestExpired  JoinRequestState = "expired"
)

type Suppression string

const (
	SuppressionNone    Suppression = "none"
	SuppressionBlocked Suppression = "blocked"
	SuppressionLimit   Suppression = "limit"
)

type Colour string

const (
	ColourClay  Colour = "clay"
	ColourSlate Colour = "slate"
	ColourMoss  Colour = "moss"
	ColourPlum  Colour = "plum"
	ColourOchre Colour = "ochre"
	ColourTeal  Colour = "teal"
	ColourRust  Colour = "rust"
	ColourInk   Colour = "ink"
)

var Palette = []Colour{ColourClay, ColourSlate, ColourMoss, ColourPlum, ColourOchre, ColourTeal, ColourRust, ColourInk}

func ChooseColour(taken []Colour) Colour {
	uses := make(map[Colour]int, len(Palette))
	for _, colour := range taken {
		uses[colour]++
	}
	chosen := Palette[0]
	for _, colour := range Palette {
		if uses[colour] < uses[chosen] {
			chosen = colour
		}
	}
	return chosen
}

type RelationshipCause string

const (
	CauseWithdrawn              RelationshipCause = "withdrawn"
	CauseMerged                 RelationshipCause = "merged"
	CauseAcceptedUnconfirmed    RelationshipCause = "accepted_unconfirmed"
	CauseAcceptedConfirmed      RelationshipCause = "accepted_confirmed"
	CauseJoinRequestAccepted    RelationshipCause = "join_request_accepted"
	CauseConfirmedInOwnBrowser  RelationshipCause = "confirmed_in_own_browser"
	CauseReclaimed              RelationshipCause = "reclaimed"
	CauseOwnerKept              RelationshipCause = "owner_kept"
	CauseOwnerKeptInactiveTutor RelationshipCause = "owner_kept_inactive_tutor"
	CauseOwnerDropped           RelationshipCause = "owner_dropped"
	CauseReclaimExpired         RelationshipCause = "reclaim_expired"
	CauseTutorRemoved           RelationshipCause = "tutor_removed"
	CauseTutorEnded             RelationshipCause = "tutor_ended"
	CauseStudentStopped         RelationshipCause = "student_stopped"
	CauseUnverifiedCleanup      RelationshipCause = "unverified_cleanup"
	CauseStudentDeleted         RelationshipCause = "student_deleted"
	CauseStudentPurged          RelationshipCause = "student_purged"
	CauseTutorDeleted           RelationshipCause = "tutor_deleted"
)

var RelationshipCauses = []RelationshipCause{
	CauseWithdrawn, CauseMerged, CauseAcceptedUnconfirmed, CauseAcceptedConfirmed, CauseJoinRequestAccepted,
	CauseConfirmedInOwnBrowser, CauseReclaimed, CauseOwnerKept, CauseOwnerKeptInactiveTutor, CauseOwnerDropped,
	CauseReclaimExpired, CauseTutorRemoved, CauseTutorEnded, CauseStudentStopped, CauseUnverifiedCleanup,
	CauseStudentDeleted, CauseStudentPurged, CauseTutorDeleted,
}

type RelationshipTransition struct {
	To            RelationshipState
	DiscardReason DiscardReason
	EndedBy       EndedBy
	NewCard       bool
}

type transitionKey struct {
	from  RelationshipState
	cause RelationshipCause
}

var (
	becomeActive  = RelationshipTransition{To: RelationshipActive}
	removal       = RelationshipTransition{To: RelationshipDiscarded, DiscardReason: DiscardRemoved, NewCard: true}
	tutorDeletion = RelationshipTransition{To: RelationshipDiscarded, DiscardReason: DiscardTutorDeleted}
	endedBy       = func(by EndedBy) RelationshipTransition {
		return RelationshipTransition{To: RelationshipEnded, EndedBy: by}
	}
)

var relationshipTransitions = map[transitionKey]RelationshipTransition{
	{RelationshipInvited, CauseWithdrawn}:           {To: RelationshipDiscarded, DiscardReason: DiscardWithdrawn},
	{RelationshipInvited, CauseMerged}:              {To: RelationshipDiscarded, DiscardReason: DiscardMerged},
	{RelationshipInvited, CauseTutorDeleted}:        tutorDeletion,
	{RelationshipInvited, CauseAcceptedUnconfirmed}: {To: RelationshipProvisional},
	{RelationshipInvited, CauseAcceptedConfirmed}:   becomeActive,
	{RelationshipInvited, CauseJoinRequestAccepted}: becomeActive,

	{RelationshipProvisional, CauseConfirmedInOwnBrowser}: becomeActive,
	{RelationshipProvisional, CauseReclaimed}:             {To: RelationshipReclaimPending},
	{RelationshipProvisional, CauseTutorRemoved}:          removal,
	{RelationshipProvisional, CauseStudentStopped}:        removal,
	{RelationshipProvisional, CauseUnverifiedCleanup}:     removal,
	{RelationshipProvisional, CauseStudentDeleted}:        removal,
	{RelationshipProvisional, CauseStudentPurged}:         removal,
	{RelationshipProvisional, CauseTutorDeleted}:          tutorDeletion,

	{RelationshipReclaimPending, CauseOwnerKept}:              becomeActive,
	{RelationshipReclaimPending, CauseAcceptedConfirmed}:      becomeActive,
	{RelationshipReclaimPending, CauseOwnerKeptInactiveTutor}: removal,
	{RelationshipReclaimPending, CauseOwnerDropped}:           removal,
	{RelationshipReclaimPending, CauseReclaimExpired}:         removal,
	{RelationshipReclaimPending, CauseTutorRemoved}:           removal,
	{RelationshipReclaimPending, CauseStudentDeleted}:         removal,
	{RelationshipReclaimPending, CauseStudentPurged}:          removal,
	{RelationshipReclaimPending, CauseTutorDeleted}:           tutorDeletion,

	{RelationshipActive, CauseTutorEnded}:     endedBy(EndedByTutor),
	{RelationshipActive, CauseStudentStopped}: endedBy(EndedByStudent),
	{RelationshipActive, CauseStudentDeleted}: endedBy(EndedByDeletion),
	{RelationshipActive, CauseTutorDeleted}:   endedBy(EndedByDeletion),

	{RelationshipEnded, CauseAcceptedConfirmed}:   becomeActive,
	{RelationshipEnded, CauseJoinRequestAccepted}: becomeActive,
	{RelationshipEnded, CauseStudentPurged}:       {To: RelationshipDiscarded, DiscardReason: DiscardStudentPurged},
}

func NextRelationshipState(from RelationshipState, cause RelationshipCause) (RelationshipTransition, bool) {
	transition, allowed := relationshipTransitions[transitionKey{from: from, cause: cause}]
	return transition, allowed
}

type TutorCode string

const (
	TutorCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	TutorCodeLength   = 8
)

func NormaliseTutorCode(input string) (TutorCode, bool) {
	var code strings.Builder
	for _, character := range input {
		switch {
		case character == '-' || unicode.IsSpace(character):
			continue
		case character >= 'a' && character <= 'z':
			character -= 'a' - 'A'
		}
		if !strings.ContainsRune(TutorCodeAlphabet, character) {
			return "", false
		}
		code.WriteRune(character)
		if code.Len() > TutorCodeLength {
			return "", false
		}
	}
	if code.Len() != TutorCodeLength {
		return "", false
	}
	return TutorCode(code.String()), true
}

func (c TutorCode) Display() string {
	return string(c[:TutorCodeLength/2]) + "-" + string(c[TutorCodeLength/2:])
}

var outcomePrecedence = []LinkOutcome{LinkOutcomeChoiceRequired, LinkOutcomeLinked, LinkOutcomeRequestSent, LinkOutcomeCodeInvalid}

func CombineOutcomes(outcomes ...LinkOutcome) (LinkOutcome, bool) {
	for _, candidate := range outcomePrecedence {
		for _, outcome := range outcomes {
			if outcome == candidate {
				return candidate, true
			}
		}
	}
	return "", false
}

type CardFilter string

const (
	CardFilterAll     CardFilter = ""
	CardFilterActive  CardFilter = "active"
	CardFilterInvited CardFilter = "invited"
	CardFilterEnded   CardFilter = "ended"
)

func (f CardFilter) States() ([]RelationshipState, bool) {
	switch f {
	case CardFilterAll:
		return []RelationshipState{RelationshipInvited, RelationshipProvisional, RelationshipReclaimPending, RelationshipActive, RelationshipEnded}, true
	case CardFilterActive:
		return []RelationshipState{RelationshipProvisional, RelationshipReclaimPending, RelationshipActive}, true
	case CardFilterInvited:
		return []RelationshipState{RelationshipInvited}, true
	case CardFilterEnded:
		return []RelationshipState{RelationshipEnded}, true
	}
	return nil, false
}

func CardShowsAccount(state RelationshipState) bool {
	switch state {
	case RelationshipProvisional, RelationshipActive, RelationshipEnded:
		return true
	case RelationshipInvited, RelationshipReclaimPending, RelationshipDiscarded:
		return false
	}
	return false
}

func CardTakesInvitations(state RelationshipState) bool {
	switch state {
	case RelationshipInvited, RelationshipEnded:
		return true
	case RelationshipProvisional, RelationshipReclaimPending, RelationshipActive, RelationshipDiscarded:
		return false
	}
	return false
}

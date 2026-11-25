package domain

import (
	"strings"
	"time"
)

type ChannelID [16]byte

func (id ChannelID) String() string {
	return formatUUID(id)
}

type MessageID [16]byte

func (id MessageID) String() string {
	return formatUUID(id)
}

const (
	MessageBodyMaxCodePoints = 4000
	MessageAttachmentsMax    = 10
	MessageEditWindow        = 24 * time.Hour
	MessageWritesPerMinute   = 30
	MessageWriteWindow       = time.Minute
	MessageSearchesPerMinute = 30
	MessageSearchWindow      = time.Minute
	MessageSearchPage        = 30
	MessageSearchMinTerm     = 2
	MessageSearchMaxTerm     = 100
	MessageSearchSlots       = 4
	MessageSearchTimeout     = 2 * time.Second
	MessageSearchQueueWait   = time.Second
	MessageSearchRetryAfter  = 5 * time.Second
	MessagePageDefault       = 50
	MessagePageMax           = 100
	MessagePreviewCodePoints = 120
	MessageClientKeyMax      = 64
	MessageClientKeyPattern  = `^[A-Za-z0-9._-]{1,64}$`

	LimitWrites   ChatLimit = "writes"
	LimitSearches ChatLimit = "searches"
)

type Reaction string

const (
	ReactionThumbsUp Reaction = "thumbs_up"
	ReactionHeart    Reaction = "heart"
	ReactionJoy      Reaction = "joy"
	ReactionWow      Reaction = "wow"
	ReactionSad      Reaction = "sad"
	ReactionCheck    Reaction = "check"
)

var Reactions = []Reaction{ReactionThumbsUp, ReactionHeart, ReactionJoy, ReactionWow, ReactionSad, ReactionCheck}

func ParseReaction(value string) (Reaction, bool) {
	for _, reaction := range Reactions {
		if string(reaction) == value {
			return reaction, true
		}
	}
	return "", false
}

func ReactionRank(reaction Reaction) int {
	for index, known := range Reactions {
		if known == reaction {
			return index
		}
	}
	return len(Reactions)
}

func LikeLiteral(term string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
}

type MessageDeleter string

const (
	DeletedByAuthor MessageDeleter = "author"
	DeletedByTutor  MessageDeleter = "tutor"
)

type ChannelKind string

const (
	ChannelDirect ChannelKind = "direct"
	ChannelGroup  ChannelKind = "group"
)

const (
	JobEraseChannels JobName = "chat.erase_channels"

	LabelDeletedUser LabelKind = "deleted_user"

	UnreadCountCap    = 99
	UnreadChannelsCap = 100
)

type MembershipChange string

const (
	MembershipJoined    MembershipChange = "joined"
	MembershipLeft      MembershipChange = "left"
	MembershipReadOnly  MembershipChange = "read_only"
	MembershipWritable  MembershipChange = "writable"
	MembershipDiscarded MembershipChange = "discarded"
)

type MembershipPath string

const (
	PathLinking        MembershipPath = "linking"
	PathConfirmation   MembershipPath = "confirmation"
	PathReclaim        MembershipPath = "reclaim"
	PathOwnerChoice    MembershipPath = "owner_choice"
	PathRemoval        MembershipPath = "removal"
	PathEnding         MembershipPath = "ending"
	PathGroupCreation  MembershipPath = "group_creation"
	PathGroupChange    MembershipPath = "group_change"
	PathArchiving      MembershipPath = "archiving"
	PathUnarchiving    MembershipPath = "unarchiving"
	PathTutorDeletion  MembershipPath = "tutor_deletion"
	PathStudentPurge   MembershipPath = "student_purge"
	PathJoinAcceptance MembershipPath = "join_acceptance"
)

type QuoteState string

const (
	QuoteAvailable QuoteState = "available"
	QuoteDeleted   QuoteState = "deleted"
	QuoteRemoved   QuoteState = "removed"
)

type UnpinCause string

const (
	UnpinDeleted UnpinCause = "deleted"
	UnpinPurged  UnpinCause = "purged"
	UnpinErased  UnpinCause = "erased"
)

type ReadSignalSuppression string

const (
	ReadSignalInactive ReadSignalSuppression = "inactive"
	ReadSignalReadOnly ReadSignalSuppression = "read_only"
	ReadSignalGone     ReadSignalSuppression = "gone"
)

package domain

type ChannelID [16]byte

func (id ChannelID) String() string {
	return formatUUID(id)
}

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

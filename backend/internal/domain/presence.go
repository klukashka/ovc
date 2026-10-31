package domain

type PresenceInstanceID [16]byte

const JobExpirePresence JobName = "chat.expire_presence"

func (id PresenceInstanceID) String() string {
	return formatUUID(id)
}

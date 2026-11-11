package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
)

type InstanceID [16]byte

type NotificationID [16]byte

func (id InstanceID) String() string {
	return formatUUID(id)
}

func (id NotificationID) String() string {
	return formatUUID(id)
}

func (id EntityID) String() string {
	return formatUUID(id)
}

type SignalKind string

const (
	SignalRecheck      SignalKind = "recheck"
	SignalNotification SignalKind = "notification"
	SignalEntity       SignalKind = "entity"
	SignalHeartbeat    SignalKind = "heartbeat"
	SignalChat         SignalKind = "chat"
	SignalTyping       SignalKind = "typing"
	SignalPresence     SignalKind = "presence"
)

type EntityKind string

const (
	EntityRelationship EntityKind = "relationship"
	EntityJoinRequest  EntityKind = "join_request"
	EntityNotification EntityKind = "notification"
	EntityLesson       EntityKind = "lesson"
	EntityChannel      EntityKind = "channel"
	EntityIssue        EntityKind = "issue"
	EntitySubmission   EntityKind = "submission"
)

func (k EntityKind) Valid() bool {
	switch k {
	case EntityRelationship, EntityJoinRequest, EntityNotification, EntityLesson, EntityChannel, EntityIssue, EntitySubmission:
		return true
	}
	return false
}

const (
	SignalChannel          = "ovc_realtime"
	SignalVersion          = 1
	MaxSignalBytes         = 7900
	MaxSignalRecipients    = 100
	MaxSignalsPerStatement = 1000
)

type Signal interface {
	Kind() SignalKind
}

type RecheckSignal struct {
	Users []UserID
}

type NotificationSignal struct {
	User         UserID
	Notification NotificationID
}

type EntitySignal struct {
	Users  []UserID
	Entity EntityKind
	ID     EntityID
}

type HeartbeatSignal struct {
	Instance InstanceID
}

type ChatSignal struct {
	Users   []UserID
	Event   FrameType
	Channel ChannelID
	Message *MessageID
	Seq     *int64
}

func (s ChatSignal) wellFormed() bool {
	newMessage := s.Event == FrameMessageNew && s.Message != nil && s.Seq != nil && *s.Seq > 0
	changedMessage := (s.Event == FrameMessageUpdated || s.Event == FrameMessageDeleted) && s.Message != nil && s.Seq == nil
	read := s.Event == FrameReadUpdated && s.Message == nil && s.Seq == nil
	return newMessage || changedMessage || read
}

type TypingSignal struct {
	Shown     []UserID
	Anonymous []UserID
	Channel   ChannelID
	Account   UserID
}

type PresenceSignal struct {
	Users   []UserID
	Account UserID
	Online  bool
}

func (RecheckSignal) Kind() SignalKind      { return SignalRecheck }
func (NotificationSignal) Kind() SignalKind { return SignalNotification }
func (EntitySignal) Kind() SignalKind       { return SignalEntity }
func (HeartbeatSignal) Kind() SignalKind    { return SignalHeartbeat }
func (ChatSignal) Kind() SignalKind         { return SignalChat }
func (TypingSignal) Kind() SignalKind       { return SignalTyping }
func (PresenceSignal) Kind() SignalKind     { return SignalPresence }

type SignalProblem string

const (
	SignalTooLarge         SignalProblem = "too_large"
	SignalSyntax           SignalProblem = "syntax"
	SignalDuplicateField   SignalProblem = "duplicate_field"
	SignalUnknownField     SignalProblem = "unknown_field"
	SignalWrongVersion     SignalProblem = "version"
	SignalUnknownKind      SignalProblem = "kind"
	SignalFieldsForKind    SignalProblem = "fields"
	SignalBadRecipients    SignalProblem = "recipients"
	SignalBadIdentifier    SignalProblem = "identifier"
	SignalUnknownEntity    SignalProblem = "entity"
	SignalUnknownEvent     SignalProblem = "event"
	SignalNotEncodableKind SignalProblem = "not_encodable"
)

type SignalError struct {
	Problem SignalProblem
}

func (e *SignalError) Error() string {
	return "invalid signal: " + string(e.Problem)
}

func signalError(problem SignalProblem) error {
	return &SignalError{Problem: problem}
}

func SplitRecipients(users []UserID) [][]UserID {
	unique := make([]UserID, 0, len(users))
	seen := make(map[UserID]bool, len(users))
	for _, user := range users {
		if !seen[user] {
			seen[user] = true
			unique = append(unique, user)
		}
	}
	return slices.Collect(slices.Chunk(unique, MaxSignalRecipients))
}

type signalWire struct {
	Version int        `json:"v"`
	Kind    SignalKind `json:"k"`
	Users   []string   `json:"u,omitempty"`
	Entity  EntityKind `json:"e,omitempty"`
	ID      string     `json:"i,omitempty"`
}

type chatSignalWire struct {
	Version int        `json:"v"`
	Kind    SignalKind `json:"k"`
	Users   []string   `json:"u"`
	Event   FrameType  `json:"e"`
	Channel string     `json:"c"`
	Message *string    `json:"i"`
	Seq     *int64     `json:"s"`
}

type typingSignalWire struct {
	Version   int        `json:"v"`
	Kind      SignalKind `json:"k"`
	Shown     []string   `json:"u"`
	Anonymous []string   `json:"n"`
	Channel   string     `json:"c"`
	Account   string     `json:"a"`
}

type presenceSignalWire struct {
	Version int        `json:"v"`
	Kind    SignalKind `json:"k"`
	Users   []string   `json:"u"`
	Account string     `json:"a"`
	Online  bool       `json:"o"`
}

func EncodeSignal(signal Signal) ([]byte, error) {
	switch typed := signal.(type) {
	case ChatSignal:
		return encodeChat(typed)
	case TypingSignal:
		return encodeTyping(typed)
	case PresenceSignal:
		return encodePresence(typed)
	}
	wire := signalWire{Version: SignalVersion, Kind: signal.Kind()}
	switch typed := signal.(type) {
	case RecheckSignal:
		users, err := recipientTexts(typed.Users)
		if err != nil {
			return nil, err
		}
		wire.Users = users
	case NotificationSignal:
		wire.Users = []string{typed.User.String()}
		wire.ID = typed.Notification.String()
	case EntitySignal:
		users, err := recipientTexts(typed.Users)
		if err != nil {
			return nil, err
		}
		if !typed.Entity.Valid() {
			return nil, signalError(SignalUnknownEntity)
		}
		wire.Users, wire.Entity, wire.ID = users, typed.Entity, typed.ID.String()
	case HeartbeatSignal:
		wire.ID = typed.Instance.String()
	default:
		return nil, signalError(SignalNotEncodableKind)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, signalError(SignalSyntax)
	}
	if len(encoded) > MaxSignalBytes {
		return nil, signalError(SignalTooLarge)
	}
	return encoded, nil
}

func encodeChat(signal ChatSignal) ([]byte, error) {
	users, err := recipientTexts(signal.Users)
	if err != nil {
		return nil, err
	}
	if !signal.wellFormed() {
		return nil, signalError(SignalUnknownEvent)
	}
	wire := chatSignalWire{Version: SignalVersion, Kind: SignalChat, Users: users, Event: signal.Event, Channel: signal.Channel.String(), Seq: signal.Seq}
	if signal.Message != nil {
		message := signal.Message.String()
		wire.Message = &message
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, signalError(SignalSyntax)
	}
	if len(encoded) > MaxSignalBytes {
		return nil, signalError(SignalTooLarge)
	}
	return encoded, nil
}

func encodeTyping(signal TypingSignal) ([]byte, error) {
	if len(signal.Shown)+len(signal.Anonymous) == 0 {
		return nil, signalError(SignalBadRecipients)
	}
	all, err := recipientTexts(append(slices.Clone(signal.Shown), signal.Anonymous...))
	if err != nil {
		return nil, err
	}
	wire := typingSignalWire{
		Version: SignalVersion, Kind: SignalTyping, Shown: all[:len(signal.Shown)], Anonymous: all[len(signal.Shown):],
		Channel: signal.Channel.String(), Account: signal.Account.String(),
	}
	return marshalSignal(wire)
}

func encodePresence(signal PresenceSignal) ([]byte, error) {
	users, err := recipientTexts(signal.Users)
	if err != nil {
		return nil, err
	}
	return marshalSignal(presenceSignalWire{Version: SignalVersion, Kind: SignalPresence, Users: users, Account: signal.Account.String(), Online: signal.Online})
}

func marshalSignal[Wire typingSignalWire | presenceSignalWire](wire Wire) ([]byte, error) {
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, signalError(SignalSyntax)
	}
	if len(encoded) > MaxSignalBytes {
		return nil, signalError(SignalTooLarge)
	}
	return encoded, nil
}

func recipientTexts(users []UserID) ([]string, error) {
	if len(users) == 0 || len(users) > MaxSignalRecipients {
		return nil, signalError(SignalBadRecipients)
	}
	texts := make([]string, 0, len(users))
	for _, user := range users {
		text := user.String()
		if slices.Contains(texts, text) {
			return nil, signalError(SignalBadRecipients)
		}
		texts = append(texts, text)
	}
	return texts, nil
}

const (
	signalFieldVersion = "v"
	signalFieldKind    = "k"
	signalFieldUsers   = "u"
	signalFieldEntity  = "e"
	signalFieldID      = "i"
	signalFieldChannel = "c"
	signalFieldSeq     = "s"
	signalFieldHidden  = "n"
	signalFieldAccount = "a"
	signalFieldOnline  = "o"
)

var signalFields = []string{
	signalFieldVersion, signalFieldKind, signalFieldUsers, signalFieldEntity, signalFieldID, signalFieldChannel, signalFieldSeq, signalFieldHidden,
	signalFieldAccount, signalFieldOnline,
}

var errSignalSyntax = errors.New("signal syntax")

func ParseSignal(payload string) (Signal, error) {
	if len(payload) > MaxSignalBytes {
		return nil, signalError(SignalTooLarge)
	}
	fields, err := signalObject(payload)
	if err != nil {
		return nil, err
	}
	if string(fields[signalFieldVersion]) != "1" {
		return nil, signalError(SignalWrongVersion)
	}
	var kind SignalKind
	if err := json.Unmarshal(fields[signalFieldKind], &kind); err != nil {
		return nil, signalError(SignalUnknownKind)
	}
	switch kind {
	case SignalRecheck:
		return parseRecheck(fields)
	case SignalNotification:
		return parseNotification(fields)
	case SignalEntity:
		return parseEntity(fields)
	case SignalHeartbeat:
		return parseHeartbeat(fields)
	case SignalChat:
		return parseChat(fields)
	case SignalTyping:
		return parseTyping(fields)
	case SignalPresence:
		return parsePresence(fields)
	}
	return nil, signalError(SignalUnknownKind)
}

func signalObject(payload string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(payload)))
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, signalError(SignalSyntax)
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, signalError(SignalSyntax)
		}
		name, ok := token.(string)
		if !ok {
			return nil, signalError(SignalSyntax)
		}
		if !slices.Contains(signalFields, name) {
			return nil, signalError(SignalUnknownField)
		}
		if _, seen := fields[name]; seen {
			return nil, signalError(SignalDuplicateField)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, signalError(SignalSyntax)
		}
		fields[name] = value
	}
	if err := expectDelimiter(decoder, '}'); err != nil {
		return nil, signalError(SignalSyntax)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, signalError(SignalSyntax)
	}
	return fields, nil
}

func expectDelimiter(decoder *json.Decoder, delimiter json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return errSignalSyntax
	}
	if found, ok := token.(json.Delim); !ok || found != delimiter {
		return errSignalSyntax
	}
	return nil
}

func requireFields(fields map[string]json.RawMessage, present ...string) error {
	if len(fields) != len(present)+2 {
		return signalError(SignalFieldsForKind)
	}
	for _, name := range present {
		if _, ok := fields[name]; !ok {
			return signalError(SignalFieldsForKind)
		}
	}
	return nil
}

func parseRecheck(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers); err != nil {
		return nil, err
	}
	users, err := parseRecipients(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	return RecheckSignal{Users: users}, nil
}

func parseNotification(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers, signalFieldID); err != nil {
		return nil, err
	}
	users, err := parseRecipients(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, signalError(SignalBadRecipients)
	}
	notification, err := parseIdentifier[NotificationID](fields[signalFieldID])
	if err != nil {
		return nil, err
	}
	return NotificationSignal{User: users[0], Notification: notification}, nil
}

func parseEntity(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers, signalFieldEntity, signalFieldID); err != nil {
		return nil, err
	}
	users, err := parseRecipients(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	var entity EntityKind
	if err := json.Unmarshal(fields[signalFieldEntity], &entity); err != nil || !entity.Valid() {
		return nil, signalError(SignalUnknownEntity)
	}
	id, err := parseIdentifier[EntityID](fields[signalFieldID])
	if err != nil {
		return nil, err
	}
	return EntitySignal{Users: users, Entity: entity, ID: id}, nil
}

func parseHeartbeat(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldID); err != nil {
		return nil, err
	}
	instance, err := parseIdentifier[InstanceID](fields[signalFieldID])
	if err != nil {
		return nil, err
	}
	return HeartbeatSignal{Instance: instance}, nil
}

func parseChat(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers, signalFieldEntity, signalFieldChannel, signalFieldID, signalFieldSeq); err != nil {
		return nil, err
	}
	users, err := parseRecipients(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	var event FrameType
	if err := json.Unmarshal(fields[signalFieldEntity], &event); err != nil {
		return nil, signalError(SignalUnknownEvent)
	}
	channel, err := parseIdentifier[ChannelID](fields[signalFieldChannel])
	if err != nil {
		return nil, err
	}
	signal := ChatSignal{Users: users, Event: event, Channel: channel}
	if string(fields[signalFieldID]) != "null" {
		message, err := parseIdentifier[MessageID](fields[signalFieldID])
		if err != nil {
			return nil, err
		}
		signal.Message = &message
	}
	if string(fields[signalFieldSeq]) != "null" {
		var seq int64
		if err := json.Unmarshal(fields[signalFieldSeq], &seq); err != nil {
			return nil, signalError(SignalFieldsForKind)
		}
		signal.Seq = &seq
	}
	if !signal.wellFormed() {
		return nil, signalError(SignalUnknownEvent)
	}
	return signal, nil
}

func parseTyping(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers, signalFieldHidden, signalFieldChannel, signalFieldAccount); err != nil {
		return nil, err
	}
	shown, err := parseRecipientList(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	anonymous, err := parseRecipientList(fields[signalFieldHidden])
	if err != nil {
		return nil, err
	}
	total := len(shown) + len(anonymous)
	if total == 0 || total > MaxSignalRecipients {
		return nil, signalError(SignalBadRecipients)
	}
	for _, user := range anonymous {
		if slices.Contains(shown, user) {
			return nil, signalError(SignalBadRecipients)
		}
	}
	channel, err := parseIdentifier[ChannelID](fields[signalFieldChannel])
	if err != nil {
		return nil, err
	}
	account, err := parseIdentifier[UserID](fields[signalFieldAccount])
	if err != nil {
		return nil, err
	}
	return TypingSignal{Shown: shown, Anonymous: anonymous, Channel: channel, Account: account}, nil
}

func parsePresence(fields map[string]json.RawMessage) (Signal, error) {
	if err := requireFields(fields, signalFieldUsers, signalFieldAccount, signalFieldOnline); err != nil {
		return nil, err
	}
	users, err := parseRecipients(fields[signalFieldUsers])
	if err != nil {
		return nil, err
	}
	account, err := parseIdentifier[UserID](fields[signalFieldAccount])
	if err != nil {
		return nil, err
	}
	var online bool
	switch string(fields[signalFieldOnline]) {
	case "true":
		online = true
	case "false":
	default:
		return nil, signalError(SignalFieldsForKind)
	}
	return PresenceSignal{Users: users, Account: account, Online: online}, nil
}

func parseRecipientList(raw json.RawMessage) ([]UserID, error) {
	var texts []string
	if err := json.Unmarshal(raw, &texts); err != nil || texts == nil || len(texts) > MaxSignalRecipients {
		return nil, signalError(SignalBadRecipients)
	}
	users := make([]UserID, 0, len(texts))
	for _, text := range texts {
		user, ok := canonicalUUID[UserID](text)
		if !ok || slices.Contains(users, user) {
			return nil, signalError(SignalBadRecipients)
		}
		users = append(users, user)
	}
	return users, nil
}

func parseRecipients(raw json.RawMessage) ([]UserID, error) {
	var texts []string
	if err := json.Unmarshal(raw, &texts); err != nil || len(texts) == 0 || len(texts) > MaxSignalRecipients {
		return nil, signalError(SignalBadRecipients)
	}
	users := make([]UserID, 0, len(texts))
	for _, text := range texts {
		user, ok := canonicalUUID[UserID](text)
		if !ok || slices.Contains(users, user) {
			return nil, signalError(SignalBadRecipients)
		}
		users = append(users, user)
	}
	return users, nil
}

func parseIdentifier[ID ~[16]byte](raw json.RawMessage) (ID, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ID{}, signalError(SignalBadIdentifier)
	}
	id, ok := canonicalUUID[ID](text)
	if !ok {
		return ID{}, signalError(SignalBadIdentifier)
	}
	return id, nil
}

func canonicalUUID[ID ~[16]byte](text string) (ID, bool) {
	id, ok := ParseUUID[ID](text)
	if !ok || formatUUID(id) != text {
		return ID{}, false
	}
	return id, true
}

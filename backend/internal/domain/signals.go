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
)

type EntityKind string

const (
	EntityRelationship EntityKind = "relationship"
	EntityJoinRequest  EntityKind = "join_request"
	EntityNotification EntityKind = "notification"
	EntityLesson       EntityKind = "lesson"
	EntityChannel      EntityKind = "channel"
)

func (k EntityKind) Valid() bool {
	switch k {
	case EntityRelationship, EntityJoinRequest, EntityNotification, EntityLesson, EntityChannel:
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

func (RecheckSignal) Kind() SignalKind      { return SignalRecheck }
func (NotificationSignal) Kind() SignalKind { return SignalNotification }
func (EntitySignal) Kind() SignalKind       { return SignalEntity }
func (HeartbeatSignal) Kind() SignalKind    { return SignalHeartbeat }

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

func EncodeSignal(signal Signal) ([]byte, error) {
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
)

var signalFields = []string{signalFieldVersion, signalFieldKind, signalFieldUsers, signalFieldEntity, signalFieldID}

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

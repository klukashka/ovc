package files

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	downloadTokenMessage   byte = 1
	downloadTokenReference byte = 2
	dispositionAttachment  byte = 0
	dispositionInline      byte = 1
	downloadIDBytes             = 16
	downloadExpiryBytes         = 8
	downloadTailBytes           = downloadExpiryBytes + 1
	messagePayloadBytes         = 1 + downloadIDBytes + downloadIDBytes + downloadTailBytes
	referencePayloadBytes       = 1 + downloadIDBytes + 1 + downloadIDBytes + downloadTailBytes
	referenceSeparator          = ":"
)

var (
	errDownloadTokenMalformed = errors.New("the download token is malformed")
	errDownloadTokenExpired   = errors.New("the download token has expired")
	errUnmintableReference    = errors.New("the reference kind has no download token layout")
)

type DownloadReference struct {
	Kind domain.ReferenceKind
	Item domain.EntityID
}

type referenceKindCode struct {
	kind domain.ReferenceKind
	code byte
}

var referenceKindCodes = []referenceKindCode{
	{domain.ReferenceMaterial, 1},
	{domain.ReferenceTemplate, 2},
	{domain.ReferenceLesson, 3},
	{domain.ReferenceIssueSnapshot, 4},
	{domain.ReferenceAnswer, 5},
	{domain.ReferenceReviewItem, 6},
}

var addressableKinds = []domain.ReferenceKind{domain.ReferenceMaterial, domain.ReferenceTemplate, domain.ReferenceLesson, domain.ReferenceIssueSnapshot}

func codeOfKind(kind domain.ReferenceKind) (byte, bool) {
	for _, entry := range referenceKindCodes {
		if entry.kind == kind {
			return entry.code, true
		}
	}
	return 0, false
}

func kindOfCode(code byte) (domain.ReferenceKind, bool) {
	for _, entry := range referenceKindCodes {
		if entry.code == code {
			return entry.kind, true
		}
	}
	return "", false
}

func parseReference(values []string) (DownloadReference, bool) {
	if len(values) != 1 {
		return DownloadReference{}, false
	}
	kindText, itemText, found := strings.Cut(values[0], referenceSeparator)
	if !found {
		return DownloadReference{}, false
	}
	kind := domain.ReferenceKind(kindText)
	if !slices.Contains(addressableKinds, kind) {
		return DownloadReference{}, false
	}
	item, valid := domain.ParseUUID[domain.EntityID](itemText)
	if !valid {
		return DownloadReference{}, false
	}
	return DownloadReference{Kind: kind, Item: item}, true
}

type downloadClaims struct {
	Attachment  domain.AttachmentID
	Reference   DownloadReference
	ExpiresAt   time.Time
	Disposition domain.Disposition
}

type downloadTokens struct {
	keys core.Keys
}

func downloadExpiry(now time.Time) time.Time {
	return now.Add(domain.DownloadAddressWindow).Truncate(domain.DownloadAddressWindow).Add(domain.DownloadAddressWindow)
}

func (t downloadTokens) mint(claims downloadClaims) (string, error) {
	payload := make([]byte, 0, referencePayloadBytes+sha256.Size)
	if claims.Reference.Kind == domain.ReferenceMessage {
		payload = append(payload, downloadTokenMessage)
		payload = append(payload, claims.Attachment[:]...)
	} else {
		code, known := codeOfKind(claims.Reference.Kind)
		if !known {
			return "", errUnmintableReference
		}
		payload = append(payload, downloadTokenReference)
		payload = append(payload, claims.Attachment[:]...)
		payload = append(payload, code)
	}
	payload = append(payload, claims.Reference.Item[:]...)
	payload = binary.BigEndian.AppendUint64(payload, unsigned(claims.ExpiresAt.Unix()))
	disposition := dispositionAttachment
	if claims.Disposition == domain.DispositionInline {
		disposition = dispositionInline
	}
	payload = append(payload, disposition)
	payload = append(payload, t.keys.DownloadMAC(payload)...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func tokenLengthAllowed(text string) bool {
	return len(text) == base64.RawURLEncoding.EncodedLen(messagePayloadBytes+sha256.Size) ||
		len(text) == base64.RawURLEncoding.EncodedLen(referencePayloadBytes+sha256.Size)
}

func payloadBytesOf(version byte) (int, bool) {
	switch version {
	case downloadTokenMessage:
		return messagePayloadBytes, true
	case downloadTokenReference:
		return referencePayloadBytes, true
	}
	return 0, false
}

func (t downloadTokens) verify(text string, now time.Time) (downloadClaims, error) {
	if !tokenLengthAllowed(text) {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || len(decoded) == 0 {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	payloadBytes, known := payloadBytesOf(decoded[0])
	if !known || len(decoded) != payloadBytes+sha256.Size {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	payload, mac := decoded[:payloadBytes], decoded[payloadBytes:]
	if !hmac.Equal(mac, t.keys.DownloadMAC(payload)) {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	var claims downloadClaims
	copy(claims.Attachment[:], payload[1:1+downloadIDBytes])
	rest := payload[1+downloadIDBytes:]
	claims.Reference.Kind = domain.ReferenceMessage
	if payload[0] == downloadTokenReference {
		kind, known := kindOfCode(rest[0])
		if !known {
			return downloadClaims{}, errDownloadTokenMalformed
		}
		claims.Reference.Kind = kind
		rest = rest[1:]
	}
	copy(claims.Reference.Item[:], rest[:downloadIDBytes])
	expiry := binary.BigEndian.Uint64(rest[downloadIDBytes : downloadIDBytes+downloadExpiryBytes])
	switch rest[downloadIDBytes+downloadExpiryBytes] {
	case dispositionAttachment:
		claims.Disposition = domain.DispositionAttachment
	case dispositionInline:
		claims.Disposition = domain.DispositionInline
	default:
		return downloadClaims{}, errDownloadTokenMalformed
	}
	if expiry > unsigned(now.Add(domain.DownloadAddressMaxAhead).Unix()) {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	claims.ExpiresAt = time.Unix(signed(expiry), 0).UTC()
	if !now.Before(claims.ExpiresAt) {
		return downloadClaims{}, errDownloadTokenExpired
	}
	return claims, nil
}

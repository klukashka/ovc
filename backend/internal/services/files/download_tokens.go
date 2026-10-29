package files

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	downloadTokenVersion  byte = 1
	dispositionAttachment byte = 0
	dispositionInline     byte = 1
	downloadIDBytes            = 16
	downloadExpiryBytes        = 8
	downloadPayloadBytes       = 1 + downloadIDBytes + downloadIDBytes + downloadExpiryBytes + 1
	downloadTokenBytes         = downloadPayloadBytes + sha256.Size
)

var (
	errDownloadTokenMalformed = errors.New("the download token is malformed")
	errDownloadTokenExpired   = errors.New("the download token has expired")
)

type downloadClaims struct {
	Attachment  domain.AttachmentID
	Message     domain.EntityID
	ExpiresAt   time.Time
	Disposition domain.Disposition
}

type downloadTokens struct {
	keys core.Keys
}

func downloadExpiry(now time.Time) time.Time {
	return now.Add(domain.DownloadAddressWindow).Truncate(domain.DownloadAddressWindow).Add(domain.DownloadAddressWindow)
}

func (t downloadTokens) mint(claims downloadClaims) string {
	payload := make([]byte, 0, downloadTokenBytes)
	payload = append(payload, downloadTokenVersion)
	payload = append(payload, claims.Attachment[:]...)
	payload = append(payload, claims.Message[:]...)
	payload = binary.BigEndian.AppendUint64(payload, unsigned(claims.ExpiresAt.Unix()))
	disposition := dispositionAttachment
	if claims.Disposition == domain.DispositionInline {
		disposition = dispositionInline
	}
	payload = append(payload, disposition)
	payload = append(payload, t.keys.DownloadMAC(payload)...)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func (t downloadTokens) verify(text string, now time.Time) (downloadClaims, error) {
	if base64.RawURLEncoding.EncodedLen(downloadTokenBytes) != len(text) {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || len(decoded) != downloadTokenBytes {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	payload, mac := decoded[:downloadPayloadBytes], decoded[downloadPayloadBytes:]
	if !hmac.Equal(mac, t.keys.DownloadMAC(payload)) || payload[0] != downloadTokenVersion {
		return downloadClaims{}, errDownloadTokenMalformed
	}
	var claims downloadClaims
	copy(claims.Attachment[:], payload[1:1+downloadIDBytes])
	copy(claims.Message[:], payload[1+downloadIDBytes:1+2*downloadIDBytes])
	expiry := binary.BigEndian.Uint64(payload[1+2*downloadIDBytes : 1+2*downloadIDBytes+downloadExpiryBytes])
	switch payload[downloadPayloadBytes-1] {
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

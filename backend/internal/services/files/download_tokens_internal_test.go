package files

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

var tokenNow = time.Date(2030, time.March, 10, 9, 2, 30, 0, time.UTC)

func codec(t *testing.T, material string) downloadTokens {
	t.Helper()
	keys, err := core.DeriveKeys(core.NewSecret(material))
	if err != nil {
		t.Fatal(err)
	}
	return downloadTokens{keys: keys}
}

func sampleClaims(disposition domain.Disposition) downloadClaims {
	return downloadClaims{
		Attachment: domain.AttachmentID{0xA1, 0x02}, Message: domain.EntityID{0xB2, 0x03},
		ExpiresAt: downloadExpiry(tokenNow), Disposition: disposition,
	}
}

func flipped(text string, index int) string {
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		panic(err)
	}
	decoded[index] ^= 0x01
	return base64.RawURLEncoding.EncodeToString(decoded)
}

func TestDownloadTokensHaveAFixedVersionedLayout(t *testing.T) {
	t.Parallel()
	signer := codec(t, "download-signing-material-for-tests")
	valid := signer.mint(sampleClaims(domain.DispositionAttachment))
	decoded, err := base64.RawURLEncoding.DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	wrongVersion := append([]byte{2}, decoded[1:downloadPayloadBytes]...)
	wrongVersion = append(wrongVersion, signer.keys.DownloadMAC(wrongVersion)...)
	farExpiry := sampleClaims(domain.DispositionAttachment)
	farExpiry.ExpiresAt = tokenNow.Add(domain.DownloadAddressMaxAhead + time.Second)
	cases := []struct {
		name  string
		token string
		err   error
	}{
		{"padding", valid + "=", errDownloadTokenMalformed},
		{"a character of the standard alphabet", "+" + valid[1:], errDownloadTokenMalformed},
		{"a slash of the standard alphabet", valid[:len(valid)-1] + "/", errDownloadTokenMalformed},
		{"a missing character", valid[:len(valid)-1], errDownloadTokenMalformed},
		{"an extra character", valid + "A", errDownloadTokenMalformed},
		{"an empty token", "", errDownloadTokenMalformed},
		{"a wrong version with a valid signature", base64.RawURLEncoding.EncodeToString(wrongVersion), errDownloadTokenMalformed},
		{"a flipped attachment byte", flipped(valid, 1), errDownloadTokenMalformed},
		{"a flipped message byte", flipped(valid, 17), errDownloadTokenMalformed},
		{"a flipped expiry byte", flipped(valid, 40), errDownloadTokenMalformed},
		{"a flipped disposition byte", flipped(valid, downloadPayloadBytes-1), errDownloadTokenMalformed},
		{"a flipped signature byte", flipped(valid, downloadTokenBytes-1), errDownloadTokenMalformed},
		{"an expiry beyond ten and a half minutes", signer.mint(farExpiry), errDownloadTokenMalformed},
		{"another key", codec(t, "another-signing-material-for-tests").mint(sampleClaims(domain.DispositionAttachment)), errDownloadTokenMalformed},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := signer.verify(testCase.token, tokenNow)

			if !errors.Is(err, testCase.err) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestDownloadTokensRoundTripUntilTheirExpiry(t *testing.T) {
	t.Parallel()
	signer := codec(t, "download-signing-material-for-tests")
	inline := sampleClaims(domain.DispositionInline)
	token := signer.mint(inline)

	early, earlyErr := signer.verify(token, tokenNow)
	last, lastErr := signer.verify(token, inline.ExpiresAt.Add(-time.Second))
	_, expiredErr := signer.verify(token, inline.ExpiresAt)

	if earlyErr != nil || early != inline || lastErr != nil || last != inline {
		t.Fatalf("the token decoded to %+v and %+v: %v %v", early, last, earlyErr, lastErr)
	}
	if !errors.Is(expiredErr, errDownloadTokenExpired) {
		t.Fatalf("the token at its expiry answered %v", expiredErr)
	}
	if len(token) != base64.RawURLEncoding.EncodedLen(1+16+16+8+1+32) {
		t.Fatalf("the token has %d characters", len(token))
	}
}

func TestDownloadAddressesExpireOnTheNextFiveMinuteBoundaryAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	cases := map[time.Time]time.Time{
		time.Date(2030, 3, 10, 9, 2, 30, 0, time.UTC):  time.Date(2030, 3, 10, 9, 10, 0, 0, time.UTC),
		time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC):   time.Date(2030, 3, 10, 9, 10, 0, 0, time.UTC),
		time.Date(2030, 3, 10, 9, 4, 59, 0, time.UTC):  time.Date(2030, 3, 10, 9, 10, 0, 0, time.UTC),
		time.Date(2030, 3, 10, 9, 5, 0, 1, time.UTC):   time.Date(2030, 3, 10, 9, 15, 0, 0, time.UTC),
		time.Date(2030, 3, 10, 23, 57, 0, 0, time.UTC): time.Date(2030, 3, 11, 0, 5, 0, 0, time.UTC),
	}
	for now, want := range cases {
		t.Run(now.Format(time.RFC3339Nano), func(t *testing.T) {
			t.Parallel()

			expiry := downloadExpiry(now)

			if !expiry.Equal(want) || expiry.Sub(now) <= domain.DownloadAddressWindow || expiry.Sub(now) > 2*domain.DownloadAddressWindow {
				t.Fatalf("expires at %s", expiry)
			}
		})
	}
}

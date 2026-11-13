package files

import (
	"encoding/base64"
	"errors"
	"strings"
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
		Attachment: domain.AttachmentID{0xA1, 0x02}, Reference: DownloadReference{Kind: domain.ReferenceMessage, Item: domain.EntityID{0xB2, 0x03}},
		ExpiresAt: downloadExpiry(tokenNow), Disposition: disposition,
	}
}

func referenceClaims(kind domain.ReferenceKind, disposition domain.Disposition) downloadClaims {
	claims := sampleClaims(disposition)
	claims.Reference = DownloadReference{Kind: kind, Item: domain.EntityID{0xC3, 0x04}}
	return claims
}

func minted(t *testing.T, signer downloadTokens, claims downloadClaims) string {
	t.Helper()
	token, err := signer.mint(claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func sealed(signer downloadTokens, payload []byte) string {
	return base64.RawURLEncoding.EncodeToString(append(payload, signer.keys.DownloadMAC(payload)...))
}

func decodedToken(t *testing.T, token string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
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
	valid := minted(t, signer, sampleClaims(domain.DispositionAttachment))
	decoded := decodedToken(t, valid)
	wrongVersion := append([]byte{downloadTokenReference}, decoded[1:messagePayloadBytes]...)
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
		{"a flipped disposition byte", flipped(valid, messagePayloadBytes-1), errDownloadTokenMalformed},
		{"a flipped signature byte", flipped(valid, messagePayloadBytes+31), errDownloadTokenMalformed},
		{"an expiry beyond ten and a half minutes", minted(t, signer, farExpiry), errDownloadTokenMalformed},
		{"another key", minted(t, codec(t, "another-signing-material-for-tests"), sampleClaims(domain.DispositionAttachment)), errDownloadTokenMalformed},
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
	token := minted(t, signer, inline)

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

func TestDownloadTokenV2Layout(t *testing.T) {
	t.Parallel()
	signer := codec(t, "download-signing-material-for-tests")
	material := referenceClaims(domain.ReferenceMaterial, domain.DispositionAttachment)
	valid := minted(t, signer, material)
	decoded := decodedToken(t, valid)
	withCode := func(code byte) string {
		payload := append([]byte(nil), decoded[:referencePayloadBytes]...)
		payload[1+downloadIDBytes] = code
		return sealed(signer, payload)
	}
	shortened := sealed(signer, append([]byte(nil), decoded[:referencePayloadBytes-1]...))
	lengthened := sealed(signer, append(append([]byte(nil), decoded[:referencePayloadBytes]...), 0))
	asVersionOne := sealed(signer, append([]byte{downloadTokenMessage}, decoded[1:referencePayloadBytes]...))
	cases := []struct {
		name  string
		token string
	}{
		{"a changed reference kind", flipped(valid, 1+downloadIDBytes)},
		{"a changed item", flipped(valid, 2+downloadIDBytes)},
		{"a changed disposition", flipped(valid, referencePayloadBytes-1)},
		{"a changed attachment", flipped(valid, 1)},
		{"a missing byte", valid[:len(valid)-2]},
		{"an extra byte", valid + "AA"},
		{"a signed payload one byte short", shortened},
		{"a signed payload one byte long", lengthened},
		{"a signed version two payload carrying version one", asVersionOne},
		{"a signed kind byte zero, which no kind uses", withCode(0)},
		{"a signed unknown kind byte", withCode(7)},
		{"a signed kind byte of the highest value", withCode(0xFF)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := signer.verify(testCase.token, tokenNow)

			if !errors.Is(err, errDownloadTokenMalformed) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("a message reference has no version two layout", func(t *testing.T) {
		t.Parallel()
		messageClaims := sampleClaims(domain.DispositionAttachment)

		token := minted(t, signer, messageClaims)

		if decodedToken(t, token)[0] != downloadTokenMessage || len(decodedToken(t, token)) != messagePayloadBytes+32 {
			t.Fatalf("a message reference minted %x", decodedToken(t, token))
		}
	})
	t.Run("an avatar reference cannot be minted", func(t *testing.T) {
		t.Parallel()
		avatar := referenceClaims(domain.ReferenceAvatar, domain.DispositionAttachment)

		_, err := signer.mint(avatar)

		if !errors.Is(err, errUnmintableReference) {
			t.Fatalf("an avatar reference minted with %v", err)
		}
	})
}

func TestDownloadTokenV2KindsRoundTripWithTheirFixedBytes(t *testing.T) {
	t.Parallel()
	signer := codec(t, "download-signing-material-for-tests")
	want := map[domain.ReferenceKind]byte{
		domain.ReferenceMaterial: 1, domain.ReferenceTemplate: 2, domain.ReferenceLesson: 3,
		domain.ReferenceIssueSnapshot: 4, domain.ReferenceAnswer: 5, domain.ReferenceReviewItem: 6,
	}
	for kind, code := range want {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			claims := referenceClaims(kind, domain.DispositionInline)

			token := minted(t, signer, claims)
			decoded := decodedToken(t, token)
			verified, err := signer.verify(token, tokenNow)

			if decoded[0] != downloadTokenReference || decoded[1+downloadIDBytes] != code || len(decoded) != referencePayloadBytes+32 {
				t.Fatalf("the %s token has the layout %x", kind, decoded)
			}
			if err != nil || verified != claims {
				t.Fatalf("the %s token decoded to %+v: %v", kind, verified, err)
			}
		})
	}
}

func TestReferenceParameterNamesOneAddressableKindAndAUUID(t *testing.T) {
	t.Parallel()
	item := domain.EntityID{0xD4, 0x05}
	accepted := map[string]DownloadReference{
		"material:" + item.String():       {Kind: domain.ReferenceMaterial, Item: item},
		"template:" + item.String():       {Kind: domain.ReferenceTemplate, Item: item},
		"lesson:" + item.String():         {Kind: domain.ReferenceLesson, Item: item},
		"issue_snapshot:" + item.String(): {Kind: domain.ReferenceIssueSnapshot, Item: item},
		"answer:" + item.String():         {Kind: domain.ReferenceAnswer, Item: item},
		"review_item:" + item.String():    {Kind: domain.ReferenceReviewItem, Item: item},
	}
	refused := [][]string{
		nil,
		{"material:" + item.String(), "material:" + item.String()},
		{""},
		{"material"},
		{"material:"},
		{"material:not-a-uuid"},
		{"Material:" + item.String()},
		{"message:" + item.String()},
		{"avatar:" + item.String()},
		{"series:" + item.String()},
		{"material:" + item.String() + ":extra"},
		{" material:" + item.String()},
	}
	for text, want := range accepted {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			reference, valid := parseReference([]string{text})

			if !valid || reference != want {
				t.Fatalf("%q parsed to %+v, %v", text, reference, valid)
			}
		})
	}
	for _, values := range refused {
		t.Run(strings.Join(values, "|"), func(t *testing.T) {
			t.Parallel()

			reference, valid := parseReference(values)

			if valid {
				t.Fatalf("%q parsed to %+v", values, reference)
			}
		})
	}
}

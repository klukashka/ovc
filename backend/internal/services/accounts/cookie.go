package accounts

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"

	"ovc/backend/internal/domain"
)

const (
	refreshRandomBytes = 32
	refreshMACBytes    = 32
)

var generationPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

type refreshCookie struct {
	session    domain.SessionID
	generation int64
	random     []byte
	mac        []byte
}

func (c refreshCookie) encode() string {
	encoding := base64.RawURLEncoding
	return strings.Join([]string{
		encoding.EncodeToString(c.session[:]),
		strconv.FormatInt(c.generation, 10),
		encoding.EncodeToString(c.random),
		encoding.EncodeToString(c.mac),
	}, ".")
}

func parseRefreshCookie(value string) (refreshCookie, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 || !generationPattern.MatchString(parts[1]) {
		return refreshCookie{}, false
	}
	encoding := base64.RawURLEncoding.Strict()
	session, sessionErr := encoding.DecodeString(parts[0])
	random, randomErr := encoding.DecodeString(parts[2])
	mac, macErr := encoding.DecodeString(parts[3])
	generation, generationErr := strconv.ParseInt(parts[1], 10, 64)
	if sessionErr != nil || randomErr != nil || macErr != nil || generationErr != nil ||
		len(session) != len(domain.SessionID{}) || len(random) != refreshRandomBytes || len(mac) != refreshMACBytes {
		return refreshCookie{}, false
	}
	cookie := refreshCookie{generation: generation, random: random, mac: mac}
	copy(cookie.session[:], session)
	return cookie, true
}

package files

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
)

const (
	fileNameMaxBytes     = 255
	fallbackDownloadName = "download"
	upperHex             = "0123456789ABCDEF"
)

func cleanFileName(given string) (string, bool) {
	name := given
	if cut := strings.LastIndexAny(name, `/\`); cut >= 0 {
		name = name[cut+1:]
	}
	if name == "" || len(name) > fileNameMaxBytes || !utf8.ValidString(name) {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", false
		}
	}
	return name, true
}

func ContentDisposition(disposition domain.Disposition, name string) string {
	if disposition == domain.DispositionInline {
		return string(domain.DispositionInline)
	}
	return string(domain.DispositionAttachment) + `; filename="` + asciiFallback(name) + `"; filename*=UTF-8''` + rfc5987(name)
}

func asciiFallback(name string) string {
	var fallback strings.Builder
	for _, r := range name {
		switch {
		case r == '\r', r == '\n', r == '"', r == '\\':
		case r < 0x20, r == 0x7F:
		case r > 0x7E:
			fallback.WriteByte('_')
		default:
			fallback.WriteRune(r)
		}
	}
	if fallback.Len() == 0 {
		return fallbackDownloadName
	}
	return fallback.String()
}

func rfc5987(name string) string {
	var encoded strings.Builder
	for index := 0; index < len(name); index++ {
		character := name[index]
		if attributeCharacter(character) {
			encoded.WriteByte(character)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(upperHex[character>>4])
		encoded.WriteByte(upperHex[character&0x0F])
	}
	return encoded.String()
}

func attributeCharacter(character byte) bool {
	switch {
	case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		return true
	}
	return strings.IndexByte("!#$&+-.^_`|~", character) >= 0
}

package exportformat

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
)

type FileKind string

const (
	FileKindLibrary      FileKind = "library"
	FileKindPromptImages FileKind = "prompt-images"
	FileKindChat         FileKind = "chat"
	FileKindAnswers      FileKind = "answers"
	FileKindCorrected    FileKind = "corrected"
)

const (
	EntryTables   = "tables.xlsx"
	EntryCalendar = "calendar.ics"
	EntryData     = "data.json"

	filesFolder        = "files"
	fallbackFileName   = "file"
	maxFileNameBytes   = 100
	maxExtensionBytes  = 16
	replacementForName = '_'
)

var (
	ErrUnsafeEntryName    = errors.New("the archive entry name is not a safe relative path")
	ErrDuplicateEntryName = errors.New("the archive already holds an entry with this name")
	ErrUnknownFileKind    = errors.New("the file kind is not one of the archive's folders")
)

func (kind FileKind) valid() bool {
	switch kind {
	case FileKindLibrary, FileKindPromptImages, FileKindChat, FileKindAnswers, FileKindCorrected:
		return true
	}
	return false
}

func FileEntryName(kind FileKind, attachment domain.AttachmentID, original string) (string, error) {
	if !kind.valid() {
		return "", ErrUnknownFileKind
	}
	return filesFolder + "/" + string(kind) + "/" + attachment.String() + "-" + SafeFileName(original), nil
}

func SafeFileName(original string) string {
	name := domain.NormaliseNFC(strings.ToValidUTF8(original, string(utf8.RuneError)))
	if last := strings.LastIndexAny(name, `/\`); last >= 0 {
		name = name[last+1:]
	}
	name = strings.Map(replaceUnsafeRune, name)
	name = strings.TrimFunc(name, isTrimmedAtEdge)
	if name == "" {
		return fallbackFileName
	}
	return capFileName(name)
}

func replaceUnsafeRune(letter rune) rune {
	if strings.ContainsRune(`/\:*?"<>|`, letter) || letter == utf8.RuneError {
		return replacementForName
	}
	if unsafeCategory(letter) || isNoncharacter(letter) {
		return replacementForName
	}
	return letter
}

func unsafeCategory(letter rune) bool {
	return unicode.In(letter, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Cs)
}

func isNoncharacter(letter rune) bool {
	return (letter >= 0xFDD0 && letter <= 0xFDEF) || letter&0xFFFE == 0xFFFE
}

func isTrimmedAtEdge(letter rune) bool {
	return letter == '.' || unicode.IsSpace(letter)
}

func capFileName(name string) string {
	if len(name) <= maxFileNameBytes {
		return name
	}
	stem, extension := name, ""
	if dot := strings.LastIndexByte(name, '.'); dot > 0 && len(name)-dot <= maxExtensionBytes {
		stem, extension = name[:dot], name[dot:]
	}
	stem = strings.TrimRightFunc(cutToBytes(stem, maxFileNameBytes-len(extension)), isTrimmedAtEdge)
	if stem == "" {
		return fallbackFileName + extension
	}
	return stem + extension
}

func cutToBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

func checkEntryName(name string) error {
	if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, `\:*?"<>|`) {
		return ErrUnsafeEntryName
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") {
			return ErrUnsafeEntryName
		}
	}
	for _, letter := range name {
		if unsafeCategory(letter) || isNoncharacter(letter) || letter == utf8.RuneError {
			return ErrUnsafeEntryName
		}
	}
	return nil
}

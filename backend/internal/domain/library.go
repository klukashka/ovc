package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type FolderID [16]byte

type MaterialID [16]byte

func (id FolderID) String() string {
	return formatUUID(id)
}

func (id MaterialID) String() string {
	return formatUUID(id)
}

type LibraryItemKind string

const (
	LibraryItemMaterial LibraryItemKind = "material"
	LibraryItemTemplate LibraryItemKind = "template"
)

const FieldTaken FieldCode = "taken"

const (
	FolderNameMaxRunes          = 100
	FolderNameNormalisedBytes   = 400
	LibraryTitleMaxRunes        = 200
	LibraryTitleNormalisedBytes = 800
	LibraryQueryMaxRunes        = 200
	FolderMaxDepth              = 3
	LibraryPageDefault          = 30
	LibraryPageMax              = 100
	LibraryWritesPerHour        = 600
	LibraryWriteWindow          = time.Hour
	PurposeLibraryWrites        = RateLimitPurpose("library_writes_hour")
)

type LibraryCaps struct {
	Folders   int64
	Materials int64
	Templates int64
	Rows      int64
}

var DefaultLibraryCaps = LibraryCaps{Folders: 1_000, Materials: 10_000, Templates: 2_000, Rows: 50_000}

type LibraryTextLimits struct {
	MaxRunes        int
	NormalisedBytes int
}

var (
	FolderNameLimits   = LibraryTextLimits{MaxRunes: FolderNameMaxRunes, NormalisedBytes: FolderNameNormalisedBytes}
	LibraryTitleLimits = LibraryTextLimits{MaxRunes: LibraryTitleMaxRunes, NormalisedBytes: LibraryTitleNormalisedBytes}
)

func (l LibraryTextLimits) Check(text string) (NormalisedText, FieldCode, bool) {
	if strings.ContainsFunc(text, forbiddenInLibraryText) {
		return "", FieldInvalid, false
	}
	if strings.TrimSpace(text) == "" {
		return "", FieldRequired, false
	}
	if utf8.RuneCountInString(text) > l.MaxRunes {
		return "", FieldTooLong, false
	}
	normalised := NormaliseTextV1(text)
	if normalised == "" {
		return "", FieldRequired, false
	}
	if len(normalised) > l.NormalisedBytes {
		return "", FieldTooLong, false
	}
	return normalised, "", true
}

func forbiddenInLibraryText(character rune) bool {
	return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
}

type FolderTree map[FolderID]*FolderID

func (t FolderTree) Level(folder FolderID) int {
	level := 1
	for current := t[folder]; current != nil && level <= len(t); current = t[*current] {
		level++
	}
	return level
}

func (t FolderTree) height(folder FolderID) int {
	tallest := 0
	for child, parent := range t {
		if parent != nil && *parent == folder {
			tallest = max(tallest, t.height(child))
		}
	}
	return tallest + 1
}

func (t FolderTree) contains(ancestor FolderID, folder FolderID) bool {
	for current, steps := &folder, 0; current != nil && steps <= len(t); steps++ {
		if *current == ancestor {
			return true
		}
		current = t[*current]
	}
	return false
}

func (t FolderTree) CanCreateUnder(parent *FolderID) bool {
	return parent == nil || t.Level(*parent) < FolderMaxDepth
}

func (t FolderTree) CanMove(folder FolderID, parent *FolderID) bool {
	if parent == nil {
		return t.height(folder) <= FolderMaxDepth
	}
	if t.contains(folder, *parent) {
		return false
	}
	return t.Level(*parent)+t.height(folder) <= FolderMaxDepth
}

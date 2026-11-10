package domain_test

import (
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func folderID(value byte) domain.FolderID {
	return domain.FolderID{value}
}

func parentOf(value byte) *domain.FolderID {
	parent := folderID(value)
	return &parent
}

func threeLevelTree() domain.FolderTree {
	return domain.FolderTree{
		folderID(1): nil,
		folderID(2): parentOf(1),
		folderID(3): parentOf(2),
		folderID(4): nil,
		folderID(5): parentOf(4),
		folderID(6): nil,
	}
}

func TestFolderDepthAndCyclesRule(t *testing.T) {
	t.Parallel()
	tree := threeLevelTree()
	cases := []struct {
		name    string
		allowed func() bool
		want    bool
	}{
		{"a root folder", func() bool { return tree.CanCreateUnder(nil) }, true},
		{"a second level under a root folder", func() bool { return tree.CanCreateUnder(parentOf(1)) }, true},
		{"a third level under a second level", func() bool { return tree.CanCreateUnder(parentOf(2)) }, true},
		{"a fourth level under a third level", func() bool { return tree.CanCreateUnder(parentOf(3)) }, false},
		{"moving a folder into itself", func() bool { return tree.CanMove(folderID(1), parentOf(1)) }, false},
		{"moving a folder into its child", func() bool { return tree.CanMove(folderID(1), parentOf(2)) }, false},
		{"moving a folder into its grandchild", func() bool { return tree.CanMove(folderID(1), parentOf(3)) }, false},
		{"moving a two-level subtree under a second level", func() bool { return tree.CanMove(folderID(4), parentOf(2)) }, false},
		{"moving a two-level subtree under a root folder", func() bool { return tree.CanMove(folderID(4), parentOf(6)) }, true},
		{"moving a leaf under a second level", func() bool { return tree.CanMove(folderID(6), parentOf(2)) }, true},
		{"moving a leaf under a third level", func() bool { return tree.CanMove(folderID(6), parentOf(3)) }, false},
		{"moving a deep folder to the root", func() bool { return tree.CanMove(folderID(3), nil) }, true},
		{"moving a three-level subtree to the root", func() bool { return tree.CanMove(folderID(1), nil) }, true},
		{"moving a three-level subtree under its sibling root", func() bool { return tree.CanMove(folderID(1), parentOf(6)) }, false},
	}

	for _, check := range cases {
		got := check.allowed()

		if got != check.want {
			t.Errorf("%s: got %v, want %v", check.name, got, check.want)
		}
	}
}

func TestFolderLevels(t *testing.T) {
	t.Parallel()
	tree := threeLevelTree()

	levels := []int{tree.Level(folderID(1)), tree.Level(folderID(2)), tree.Level(folderID(3)), tree.Level(folderID(5))}

	if levels[0] != 1 || levels[1] != 2 || levels[2] != 3 || levels[3] != 2 {
		t.Fatalf("levels %v", levels)
	}
}

func TestLibraryTextLimits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		text   string
		limits domain.LibraryTextLimits
		code   domain.FieldCode
	}{
		{"a control character", "Unit\u00071", domain.FolderNameLimits, domain.FieldInvalid},
		{"a bidirectional override", "Unit\u202e1", domain.FolderNameLimits, domain.FieldInvalid},
		{"only spaces", "   ", domain.FolderNameLimits, domain.FieldRequired},
		{"one code point too many", strings.Repeat("я", 101), domain.FolderNameLimits, domain.FieldTooLong},
		{"a name that normalises past its byte bound", strings.Repeat("\ufdfa", 50), domain.FolderNameLimits, domain.FieldTooLong},
		{"a title that normalises past its byte bound", strings.Repeat("\ufdfa", 100), domain.LibraryTitleLimits, domain.FieldTooLong},
	}

	for _, check := range cases {
		_, code, valid := check.limits.Check(check.text)

		if valid || code != check.code {
			t.Errorf("%s: valid %v with %q, want %q", check.name, valid, code, check.code)
		}
	}
}

func TestLibraryTextLimitsAcceptTheLongestOrdinaryText(t *testing.T) {
	t.Parallel()

	name, nameCode, nameValid := domain.FolderNameLimits.Check(strings.Repeat("Ё", 100))
	title, titleCode, titleValid := domain.LibraryTitleLimits.Check(strings.Repeat("Ж", 200))

	if !nameValid || !titleValid || nameCode != "" || titleCode != "" {
		t.Fatalf("ordinary text refused: %q %q", nameCode, titleCode)
	}
	if name != domain.NormalisedText(strings.Repeat("е", 100)) || len(title) != 400 {
		t.Fatalf("normalised %q and %d bytes", name, len(title))
	}
}

package exportformat_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
)

const firstAttachmentText = "0f8e1c52-6b1d-4c43-9d2a-5a0f3c9e7b11"

func attachmentID(t *testing.T, text string) domain.AttachmentID {
	t.Helper()
	id, ok := domain.ParseUUID[domain.AttachmentID](text)
	if !ok {
		t.Fatalf("bad attachment identifier %q", text)
	}
	return id
}

func TestSafeFileNameFollowsTheEntryNameRule(t *testing.T) {
	cases := map[string]string{
		"report.pdf":                 "report.pdf",
		"a\uFFFEb\uFDD0c.txt":        "a_b_c.txt",
		"a\uFDEFb\U0010FFFFc.txt":    "a_b_c.txt",
		"a\U0001FFFEb.txt":           "a_b.txt",
		"../x.pdf":                   "x.pdf",
		`..\..\windows\x.pdf`:        "x.pdf",
		"a/b/c.txt":                  "c.txt",
		"invoice\u202Etxt.exe":       "invoice_txt.exe",
		"nul\x00byte\x07.txt":        "nul_byte_.txt",
		"line\u2028break.txt":        "line_break.txt",
		`what:*?"<>|.txt`:            "what_______.txt",
		"  ..hidden.txt. . ":         "hidden.txt",
		"...":                        "file",
		"":                           "file",
		"   ":                        "file",
		" \t ":                       "_",
		"Cafe\u0301.txt":             "Café.txt",
		"bad\xffbyte.txt":            "bad_byte.txt",
		"..":                         "file",
		"report.final.version.pdf":   "report.final.version.pdf",
		"\u200Bzero-width-start.txt": "_zero-width-start.txt",
	}

	for original, want := range cases {
		got := exportformat.SafeFileName(original)

		if got != want {
			t.Errorf("SafeFileName(%q) = %q; want %q", original, got, want)
		}
	}
}

func TestSafeFileNameCutsTo100BytesKeepingAShortExtension(t *testing.T) {
	cases := []struct {
		original      string
		wantExtension string
	}{
		{strings.Repeat("я", 80) + ".pdf", ".pdf"},
		{strings.Repeat("a", 150) + ".docx", ".docx"},
		{strings.Repeat("ж", 70) + "." + strings.Repeat("e", 20), ""},
		{strings.Repeat("€", 60) + ".fifteen-bytes-x", ".fifteen-bytes-x"},
	}

	for _, testCase := range cases {
		got := exportformat.SafeFileName(testCase.original)

		if len(got) > 100 || !utf8.ValidString(got) {
			t.Errorf("SafeFileName(%q) = %q, %d bytes; want at most 100 valid bytes", testCase.original, got, len(got))
		}
		if !strings.HasSuffix(got, testCase.wantExtension) {
			t.Errorf("SafeFileName(%q) = %q; want the extension %q kept", testCase.original, got, testCase.wantExtension)
		}
	}
}

func TestFileEntryNamePlacesTheFileUnderItsKindWithTheAttachmentPrefix(t *testing.T) {
	attachment := attachmentID(t, firstAttachmentText)

	name, err := exportformat.FileEntryName(exportformat.FileKindChat, attachment, "../x.pdf")

	if err != nil || name != "files/chat/"+firstAttachmentText+"-x.pdf" {
		t.Fatalf("FileEntryName = %q, %v; want files/chat/%s-x.pdf", name, err, firstAttachmentText)
	}
}

func TestFileEntryNameRefusesAnUnknownKind(t *testing.T) {
	attachment := attachmentID(t, firstAttachmentText)

	_, err := exportformat.FileEntryName(exportformat.FileKind("../escape"), attachment, "x.pdf")

	if !errors.Is(err, exportformat.ErrUnknownFileKind) {
		t.Fatalf("FileEntryName with a made-up kind = %v; want ErrUnknownFileKind", err)
	}
}

package exportformat_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/exportformat"
)

func TestArchiveRefusesUnsafeAndDuplicateEntryNames(t *testing.T) {
	cases := map[string]error{
		"../x.pdf":           exportformat.ErrUnsafeEntryName,
		"files/../x.pdf":     exportformat.ErrUnsafeEntryName,
		"/etc/passwd":        exportformat.ErrUnsafeEntryName,
		`files\chat\x.pdf`:   exportformat.ErrUnsafeEntryName,
		"files//x.pdf":       exportformat.ErrUnsafeEntryName,
		"files/./x.pdf":      exportformat.ErrUnsafeEntryName,
		"files/x\x00.pdf":    exportformat.ErrUnsafeEntryName,
		"files/\u202Ex":      exportformat.ErrUnsafeEntryName,
		"bad\xff":            exportformat.ErrUnsafeEntryName,
		"C:/x.pdf":           exportformat.ErrUnsafeEntryName,
		"files/a:b":          exportformat.ErrUnsafeEntryName,
		"files/.hidden":      exportformat.ErrUnsafeEntryName,
		"files/a\u2028b":     exportformat.ErrUnsafeEntryName,
		"files/a\uFFFEb":     exportformat.ErrUnsafeEntryName,
		"files/a\U0001FFFFb": exportformat.ErrUnsafeEntryName,
		"files/a*b":          exportformat.ErrUnsafeEntryName,
		"files/a|b":          exportformat.ErrUnsafeEntryName,
		"":                   exportformat.ErrUnsafeEntryName,
		"data.json":          exportformat.ErrDuplicateEntryName,
	}
	archive := exportformat.NewArchive(&strings.Builder{}, 1<<20, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))
	if _, err := archive.Create(exportformat.EntryData); err != nil {
		t.Fatal(err)
	}

	for name, want := range cases {
		_, err := archive.Create(name)

		if !errors.Is(err, want) {
			t.Errorf("Create(%q) = %v; want %v", name, err, want)
		}
	}
}

func TestTwoFilesWithOneNameGetDistinctEntries(t *testing.T) {
	first := attachmentID(t, firstAttachmentText)
	second := attachmentID(t, "7d2b9e40-1c3a-4f5e-8b6d-0a9c8e7f6d55")
	archive := exportformat.NewArchive(&strings.Builder{}, 1<<20, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))

	firstName, firstErr := exportformat.FileEntryName(exportformat.FileKindAnswers, first, "essay.docx")
	secondName, secondErr := exportformat.FileEntryName(exportformat.FileKindAnswers, second, "essay.docx")
	_, firstCreate := archive.Create(firstName)
	_, secondCreate := archive.Create(secondName)

	if errors.Join(firstErr, secondErr, firstCreate, secondCreate) != nil || firstName == secondName {
		t.Fatalf("entries %q and %q: %v", firstName, secondName, errors.Join(firstErr, secondErr, firstCreate, secondCreate))
	}
}

func TestArchiveFailsOnceItPassesItsBound(t *testing.T) {
	var output bytes.Buffer
	archive := exportformat.NewArchive(&output, 4096, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))
	entry, err := archive.Create(exportformat.EntryData)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(bytes.Repeat([]byte{0}, 100)); err != nil {
		t.Fatal(err)
	}
	stored, err := archive.Create("files/chat/noise")
	if err != nil {
		t.Fatal(err)
	}

	_, writeErr := stored.Write(pseudoRandomBytes(64 << 10))
	closeErr := archive.Close()

	if !errors.Is(errors.Join(writeErr, closeErr), exportformat.ErrTooLarge) || archive.Written() > 4096 {
		t.Fatalf("writing past the bound: %v, %v, %d bytes written; want ErrTooLarge within 4096", writeErr, closeErr, archive.Written())
	}
}

func pseudoRandomBytes(size int) []byte {
	data := make([]byte, size)
	source := rand.NewChaCha8([32]byte{7})
	if _, err := source.Read(data); err != nil {
		panic(err)
	}
	return data
}

func TestArchiveEntriesUseUTF8NamesAndNoDirectories(t *testing.T) {
	var output bytes.Buffer
	archive := exportformat.NewArchive(&output, 1<<20, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))
	name := "files/library/" + firstAttachmentText + "-Домашнее задание.pdf"
	entry, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "pdf"); err != nil {
		t.Fatal(err)
	}

	closeErr := archive.Close()

	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if errors.Join(closeErr, err) != nil {
		t.Fatal(errors.Join(closeErr, err))
	}
	if len(reader.File) != 1 || reader.File[0].Name != name || reader.File[0].NonUTF8 || reader.File[0].FileInfo().IsDir() {
		t.Fatalf("entries = %+v; want the one UTF-8 file entry %q", reader.File, name)
	}
}

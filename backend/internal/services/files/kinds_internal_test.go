package files

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func mpegFrame() []byte {
	frame := make([]byte, 417)
	copy(frame, []byte{0xFF, 0xFB, 0x90, 0x00})
	return frame
}

func repeated(part []byte, times int) []byte {
	return bytes.Repeat(part, times)
}

func oggPage(t *testing.T, packet string) []byte {
	t.Helper()
	page := append([]byte("OggS"), 0, oggBeginningOfStream)
	page = append(page, make([]byte, 20)...)
	page = append(page, 1, size8(t, len(packet)))
	return append(page, packet...)
}

func m4aHeader(t *testing.T, major string, compatible ...string) []byte {
	t.Helper()
	box := []byte{0, 0, 0, 0}
	box = append(box, "ftyp"+major+"\x00\x00\x02\x00"...)
	for _, brand := range compatible {
		box = append(box, brand...)
	}
	binary.BigEndian.PutUint32(box, size32(t, len(box)))
	return append(box, make([]byte, 16)...)
}

func TestDetectedKindsFollowTheAllowlist(t *testing.T) {
	t.Parallel()
	zip := append(bytes.Clone(zipSignature), make([]byte, 26)...)
	cases := []struct {
		name     string
		head     []byte
		filename string
		kind     fileKind
		accepted bool
	}{
		{"a JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "photo.JPG", kindJPEG, true},
		{"a JPEG with the long extension", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "photo.jpeg", kindJPEG, true},
		{"a PNG", pngSignature, "chart.png", kindPNG, true},
		{"a WebP", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "sticker.webp", kindWebP, true},
		{"a GIF", []byte("GIF89a"), "loop.gif", kindGIF, true},
		{"a PDF", []byte("%PDF-1.7"), "notes.pdf", kindPDF, true},
		{"a WAV", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), "voice.wav", kindWAV, true},
		{"a Vorbis OGG", oggPage(t, "\x01vorbis"), "song.ogg", kindOGG, true},
		{"an Opus OGG", oggPage(t, "OpusHead"), "voice.opus", kindOGG, true},
		{"an OGG with the audio extension", oggPage(t, "OpusHead"), "voice.oga", kindOGG, true},
		{"an OGG of another codec", oggPage(t, "\x80theora"), "clip.ogg", "", false},
		{"an M4A by its major brand", m4aHeader(t, "M4A "), "lecture.m4a", kindM4A, true},
		{"an M4A by a compatible brand", m4aHeader(t, "isom", "mp42", "M4A "), "lecture.m4a", kindM4A, true},
		{"an MP4 video named as audio", m4aHeader(t, "isom", "mp42"), "lecture.m4a", "", false},
		{"an MP3 with an ID3 tag", []byte("ID3\x04\x00\x00\x00\x00\x01\x00"), "song.mp3", kindMP3, true},
		{"an MP3 with three frame headers", repeated(mpegFrame(), 3), "song.mp3", kindMP3, true},
		{"an MP3 with only two frame headers", repeated(mpegFrame(), 2), "song.mp3", "", false},
		{"an ID3 tag with an invalid size", []byte("ID3\x04\x00\x00\x80\x00\x01\x00"), "song.mp3", "", false},
		{"a DOCX container", zip, "essay.docx", kindDOCX, true},
		{"an ODP container", zip, "slides.odp", kindODP, true},
		{"a generic ZIP", zip, "archive.zip", "", false},
		{"a ZIP named as a PDF", zip, "notes.pdf", "", false},
		{"plain text", []byte("hello"), "notes.txt", kindPlainText, true},
		{"a PNG disguised as a JPEG", pngSignature, "photo.jpg", "", false},
		{"a PDF disguised as text", []byte("%PDF-1.7"), "notes.txt", "", false},
		{"a JPEG disguised as a PDF", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "notes.pdf", "", false},
		{"text without an extension", []byte("hello"), "README", "", false},
		{"a PDF without an extension", []byte("%PDF-1.7"), "notes", "", false},
		{"an executable", []byte("MZ\x90\x00"), "setup.exe", "", false},
		{"an unknown binary named as text", []byte("MZ\x90\x00"), "notes.txt", kindPlainText, true},
	}
	for _, script := range []string{".html", ".htm", ".svg", ".xml", ".js", ".mjs", ".sh", ".bat", ".cmd", ".ps1", ".vbs", ".hta"} {
		cases = append(cases, struct {
			name     string
			head     []byte
			filename string
			kind     fileKind
			accepted bool
		}{"text named " + script, []byte("<script>alert(1)</script>"), "page" + script, "", false})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			kind, accepted := detectKind(testCase.head, testCase.filename)

			if accepted != testCase.accepted || (accepted && kind != testCase.kind) {
				t.Fatalf("detected %q accepted %t", kind, accepted)
			}
		})
	}
}

func TestEveryDetectedKindHasItsMediaTypeAndOnlyItsExtensions(t *testing.T) {
	t.Parallel()
	expected := map[string]fileKind{
		".jpg": kindJPEG, ".jpeg": kindJPEG, ".png": kindPNG, ".webp": kindWebP, ".gif": kindGIF, ".pdf": kindPDF,
		".docx": kindDOCX, ".xlsx": kindXLSX, ".pptx": kindPPTX, ".odt": kindODT, ".ods": kindODS, ".odp": kindODP,
		".mp3": kindMP3, ".m4a": kindM4A, ".ogg": kindOGG, ".oga": kindOGG, ".opus": kindOGG, ".wav": kindWAV, ".txt": kindPlainText,
	}

	listed := map[string]fileKind{}
	for _, rule := range kindRules {
		for _, extension := range rule.extensions {
			listed[extension] = rule.kind
		}
	}

	if len(listed) != len(expected) {
		t.Fatalf("the table lists %d extensions, the specification %d", len(listed), len(expected))
	}
	for extension, kind := range expected {
		if listed[extension] != kind || !strings.Contains(kind.mediaType(), "/") {
			t.Errorf("%s maps to %q with media type %q", extension, listed[extension], kind.mediaType())
		}
	}
}

func TestPlainTextIsValidUTF8WithoutControlCharacters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		chunks []string
		valid  bool
	}{
		{"ASCII with tabs and line ends", []string{"one\ttwo\r\nthree\n"}, true},
		{"Cyrillic split across writes", []string{"Прив", "\xd0", "\xb5т"}, true},
		{"a four-byte rune split three ways", []string{"a\xf0\x9f", "\x98", "\x80b"}, true},
		{"a byte order mark", []string{"\ufeffhello"}, true},
		{"a NUL byte", []string{"hello\x00world"}, false},
		{"a bell", []string{"ring\x07"}, false},
		{"a delete", []string{"rub\x7f"}, false},
		{"a C1 control", []string{"next\u0085line"}, false},
		{"an invalid byte", []string{"bad\xffbyte"}, false},
		{"an overlong encoding", []string{"\xc0\xaf"}, false},
		{"a truncated rune at the end", []string{"cut\xe2\x82"}, false},
		{"a surrogate", []string{"\xed\xa0\x80"}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			check := newPlainTextCheck()

			var failed error
			for _, chunk := range testCase.chunks {
				if _, err := check.Write([]byte(chunk)); err != nil {
					failed = err
					break
				}
			}
			if failed == nil {
				failed = check.finish()
			}

			if (failed == nil) != testCase.valid {
				t.Fatalf("validity %t, error %v", testCase.valid, failed)
			}
		})
	}
}

func TestFileNamesStripPathsAndRefuseControlAndFormatCharacters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		given string
		name  string
		valid bool
	}{
		{"essay.docx", "essay.docx", true},
		{`C:\Users\anna\Desktop\essay.docx`, "essay.docx", true},
		{"../../etc/passwd.txt", "passwd.txt", true},
		{`folder/inner\name.pdf`, "name.pdf", true},
		{"домашка.pdf", "домашка.pdf", true},
		{"folder/", "", false},
		{"", "", false},
		{strings.Repeat("я", 127) + ".t", "", false},
		{strings.Repeat("a", 251) + ".txt", strings.Repeat("a", 251) + ".txt", true},
		{"line\nbreak.txt", "", false},
		{"tab\tname.txt", "", false},
		{"\u202egpj.exe", "", false},
		{"zero\u200bwidth.txt", "", false},
		{"bad\xffbyte.txt", "", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.given, func(t *testing.T) {
			t.Parallel()

			name, valid := cleanFileName(testCase.given)

			if valid != testCase.valid || name != testCase.name {
				t.Fatalf("cleaned to %q valid %t", name, valid)
			}
		})
	}
}

func TestContentDispositionUsesRFC6266WithASanitisedFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		disposition domain.Disposition
		name        string
		want        string
	}{
		{domain.DispositionAttachment, "essay.docx", `attachment; filename="essay.docx"; filename*=UTF-8''essay.docx`},
		{domain.DispositionAttachment, "домашка 1.pdf",
			`attachment; filename="_______ 1.pdf"; filename*=UTF-8''%D0%B4%D0%BE%D0%BC%D0%B0%D1%88%D0%BA%D0%B0%201.pdf`},
		{domain.DispositionAttachment, `say "hi" \ now.txt`, `attachment; filename="say hi  now.txt"; filename*=UTF-8''say%20%22hi%22%20%5C%20now.txt`},
		{domain.DispositionAttachment, "semi;colon=equals.txt", `attachment; filename="semi;colon=equals.txt"; filename*=UTF-8''semi%3Bcolon%3Dequals.txt`},
		{domain.DispositionAttachment, "日本.txt", `attachment; filename="__.txt"; filename*=UTF-8''%E6%97%A5%E6%9C%AC.txt`},
		{domain.DispositionAttachment, `"\`, `attachment; filename="download"; filename*=UTF-8''%22%5C`},
		{domain.DispositionInline, "photo.jpg", "inline"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := ContentDisposition(testCase.disposition, testCase.name)

			if header != testCase.want {
				t.Fatalf("got %s", header)
			}
		})
	}
}

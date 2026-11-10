package files

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func brandBox(t *testing.T, major string, declared int, compatible ...string) []byte {
	t.Helper()
	box := append([]byte{0, 0, 0, 0}, "ftyp"+major+"\x00\x00\x02\x00"...)
	for _, brand := range compatible {
		box = append(box, brand...)
	}
	if len(box) < declared {
		box = append(box, make([]byte, declared-len(box))...)
	}
	binary.BigEndian.PutUint32(box, size32(t, declared))
	return append(box, bytes.Repeat([]byte{0x11}, 64)...)
}

type kindCase struct {
	name     string
	policy   kindPolicy
	head     []byte
	filename string
	kind     fileKind
	accepted bool
}

func assertKinds(t *testing.T, cases []kindCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			kind, accepted := testCase.policy.detect(testCase.head, testCase.filename)

			if accepted != testCase.accepted || (accepted && kind != testCase.kind) {
				t.Fatalf("detected %q accepted %t", kind, accepted)
			}
		})
	}
}

func TestMP4BrandPrecedence(t *testing.T) {
	t.Parallel()
	cases := []kindCase{
		{"the isom brand", libraryKinds, brandBox(t, "isom", 24, "mp42"), "lecture.mp4", kindMP4, true},
		{"the iso2 brand", libraryKinds, brandBox(t, "iso2", 24, "isom"), "lecture.mp4", kindMP4, true},
		{"the mp41 brand", libraryKinds, brandBox(t, "mp41", 16), "lecture.mp4", kindMP4, true},
		{"the mp42 brand", libraryKinds, brandBox(t, "mp42", 20, "isom"), "lecture.mp4", kindMP4, true},
		{"the avc1 brand", libraryKinds, brandBox(t, "avc1", 20, "isom"), "lecture.mp4", kindMP4, true},
		{"the M4V brand", libraryKinds, brandBox(t, "M4V ", 20, "isom"), "lecture.mp4", kindMP4, true},
		{"an upper-case extension", libraryKinds, brandBox(t, "isom", 16), "LECTURE.MP4", kindMP4, true},
		{"a video major brand wins over a compatible M4A brand", libraryKinds, brandBox(t, "isom", 24, "M4A ", "mp42"), "lecture.mp4", kindMP4, true},
		{"a video major brand with a compatible M4A brand named as audio", libraryKinds, brandBox(t, "isom", 24, "M4A ", "mp42"), "lecture.m4a", "", false},
		{"an M4A major brand stays audio", libraryKinds, brandBox(t, "M4A ", 24, "isom", "mp42"), "lecture.m4a", kindM4A, true},
		{"an M4A major brand named as video", libraryKinds, brandBox(t, "M4A ", 24, "isom", "mp42"), "lecture.mp4", "", false},
		{"an M4A file whose M4A brand is only compatible", libraryKinds, brandBox(t, "mp42", 24, "M4A ", "isom"), "lecture.m4a", "", false},
		{"the QuickTime brand", libraryKinds, brandBox(t, "qt  ", 20, "qt  "), "lecture.mp4", "", false},
		{"the QuickTime brand with compatible video brands", libraryKinds, brandBox(t, "qt  ", 24, "isom", "mp42"), "lecture.mp4", "", false},
		{"the QuickTime brand named as a movie", libraryKinds, brandBox(t, "qt  ", 20, "qt  "), "lecture.mov", "", false},
		{"a video named as a movie", libraryKinds, brandBox(t, "isom", 20, "mp42"), "lecture.mov", "", false},
		{"another major brand with compatible video brands", libraryKinds, brandBox(t, "3gp4", 24, "isom", "mp42"), "lecture.mp4", "", false},
		{"a video without an extension", libraryKinds, brandBox(t, "isom", 16), "lecture", "", false},
		{"a PDF named as video", libraryKinds, []byte("%PDF-1.7 and more"), "lecture.mp4", "", false},
		{"plain text named as video", libraryKinds, []byte("hello there, a long enough line"), "lecture.mp4", "", false},
		{"a declared size of 15 bytes", libraryKinds, brandBox(t, "isom", 15), "lecture.mp4", "", false},
		{"a declared size of 16 bytes", libraryKinds, brandBox(t, "isom", 16), "lecture.mp4", kindMP4, true},
		{"a declared size of 4,096 bytes", libraryKinds, brandBox(t, "isom", 4096, "mp42"), "lecture.mp4", kindMP4, true},
		{"a declared size of 4,097 bytes", libraryKinds, brandBox(t, "isom", 4097, "mp42"), "lecture.mp4", "", false},
		{"a declared size beyond the received bytes", libraryKinds, brandBox(t, "isom", 24)[:20], "lecture.mp4", "", false},
		{"a declared size of zero", libraryKinds, brandBox(t, "isom", 0), "lecture.mp4", "", false},
		{"a chat upload of a video", chatKinds, brandBox(t, "isom", 20, "mp42"), "lecture.mp4", "", false},
		{"a chat upload of audio by its compatible brand", chatKinds, brandBox(t, "isom", 24, "mp42", "M4A "), "lecture.m4a", kindM4A, true},
		{"a prompt image of a video", promptImageKinds, brandBox(t, "isom", 20, "mp42"), "lecture.mp4", "", false},
	}

	assertKinds(t, cases)
}

func TestLibraryContextsAcceptOnlyTheirKinds(t *testing.T) {
	t.Parallel()
	zip := append(bytes.Clone(zipSignature), make([]byte, 26)...)
	cases := []kindCase{
		{"a library PDF", libraryKinds, []byte("%PDF-1.7"), "notes.pdf", kindPDF, true},
		{"a library DOCX", libraryKinds, zip, "essay.docx", kindDOCX, true},
		{"a library MP3", libraryKinds, repeated(mpegFrame(), 3), "song.mp3", kindMP3, true},
		{"a library PNG", libraryKinds, pngSignature, "chart.png", kindPNG, true},
		{"a library WebP", libraryKinds, []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "sticker.webp", kindWebP, true},
		{"library plain text", libraryKinds, []byte("hello"), "notes.txt", "", false},
		{"a library generic ZIP", libraryKinds, zip, "archive.zip", "", false},
		{"a prompt JPEG", promptImageKinds, []byte{0xFF, 0xD8, 0xFF, 0xE0}, "photo.jpg", kindJPEG, true},
		{"a prompt PNG", promptImageKinds, pngSignature, "chart.png", kindPNG, true},
		{"a prompt WebP", promptImageKinds, []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "sticker.webp", kindWebP, true},
		{"a prompt GIF", promptImageKinds, []byte("GIF89a"), "loop.gif", kindGIF, true},
		{"a prompt PDF", promptImageKinds, []byte("%PDF-1.7"), "notes.pdf", "", false},
		{"a prompt MP3", promptImageKinds, repeated(mpegFrame(), 3), "song.mp3", "", false},
		{"a prompt DOCX", promptImageKinds, zip, "essay.docx", "", false},
		{"prompt plain text", promptImageKinds, []byte("hello"), "notes.txt", "", false},
		{"chat plain text", chatKinds, []byte("hello"), "notes.txt", kindPlainText, true},
	}

	assertKinds(t, cases)
}

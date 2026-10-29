package files

import (
	"bytes"
	"encoding/binary"
	"path"
	"slices"
	"strings"
)

type fileKind string

const (
	kindJPEG      fileKind = "jpeg"
	kindPNG       fileKind = "png"
	kindWebP      fileKind = "webp"
	kindGIF       fileKind = "gif"
	kindPDF       fileKind = "pdf"
	kindDOCX      fileKind = "docx"
	kindXLSX      fileKind = "xlsx"
	kindPPTX      fileKind = "pptx"
	kindODT       fileKind = "odt"
	kindODS       fileKind = "ods"
	kindODP       fileKind = "odp"
	kindMP3       fileKind = "mp3"
	kindM4A       fileKind = "m4a"
	kindOGG       fileKind = "ogg"
	kindWAV       fileKind = "wav"
	kindPlainText fileKind = "text"
)

type kindRule struct {
	kind       fileKind
	mediaType  string
	extensions []string
}

var kindRules = []kindRule{
	{kindJPEG, "image/jpeg", []string{".jpg", ".jpeg"}},
	{kindPNG, "image/png", []string{".png"}},
	{kindWebP, "image/webp", []string{".webp"}},
	{kindGIF, "image/gif", []string{".gif"}},
	{kindPDF, "application/pdf", []string{".pdf"}},
	{kindDOCX, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", []string{".docx"}},
	{kindXLSX, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", []string{".xlsx"}},
	{kindPPTX, "application/vnd.openxmlformats-officedocument.presentationml.presentation", []string{".pptx"}},
	{kindODT, "application/vnd.oasis.opendocument.text", []string{".odt"}},
	{kindODS, "application/vnd.oasis.opendocument.spreadsheet", []string{".ods"}},
	{kindODP, "application/vnd.oasis.opendocument.presentation", []string{".odp"}},
	{kindMP3, "audio/mpeg", []string{".mp3"}},
	{kindM4A, "audio/mp4", []string{".m4a"}},
	{kindOGG, "audio/ogg", []string{".ogg", ".oga", ".opus"}},
	{kindWAV, "audio/wav", []string{".wav"}},
	{kindPlainText, "text/plain", []string{".txt"}},
}

func ruleOf(kind fileKind) kindRule {
	for _, rule := range kindRules {
		if rule.kind == kind {
			return rule
		}
	}
	panic("a detected kind has no rule")
}

func (k fileKind) mediaType() string {
	return ruleOf(k).mediaType
}

func (k fileKind) picture() bool {
	return k == kindJPEG || k == kindPNG || k == kindWebP || k == kindGIF
}

var officeKinds = []fileKind{kindDOCX, kindXLSX, kindPPTX, kindODT, kindODS, kindODP}

func (k fileKind) office() bool {
	return slices.Contains(officeKinds, k)
}

func kindForExtension(name string) (fileKind, bool) {
	extension := strings.ToLower(path.Ext(name))
	if extension == "" {
		return "", false
	}
	for _, rule := range kindRules {
		if slices.Contains(rule.extensions, extension) {
			return rule.kind, true
		}
	}
	return "", false
}

var zipSignature = []byte{'P', 'K', 0x03, 0x04}

func detectKind(head []byte, name string) (fileKind, bool) {
	claimed, known := kindForExtension(name)
	if !known {
		return "", false
	}
	detected, found := magicKind(head)
	switch {
	case found:
		return detected, detected == claimed
	case bytes.HasPrefix(head, zipSignature):
		return claimed, claimed.office()
	case claimed == kindPlainText:
		return kindPlainText, true
	}
	return "", false
}

func magicKind(head []byte) (fileKind, bool) {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return kindJPEG, true
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return kindPNG, true
	case riffForm(head, "WEBP"):
		return kindWebP, true
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return kindGIF, true
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return kindPDF, true
	case riffForm(head, "WAVE"):
		return kindWAV, true
	case oggAudio(head):
		return kindOGG, true
	case m4aAudio(head):
		return kindM4A, true
	case id3Tagged(head), mpegFrames(head, mpegFramesWithoutTag):
		return kindMP3, true
	}
	return "", false
}

func riffForm(head []byte, form string) bool {
	return len(head) >= 12 && bytes.Equal(head[0:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte(form))
}

const (
	oggHeaderBytes       = 27
	oggBeginningOfStream = 0x02
)

func oggAudio(head []byte) bool {
	if len(head) < oggHeaderBytes || !bytes.Equal(head[0:4], []byte("OggS")) || head[4] != 0 || head[5]&oggBeginningOfStream == 0 {
		return false
	}
	segments := int(head[26])
	start := oggHeaderBytes + segments
	if len(head) < start {
		return false
	}
	packet := head[start:]
	return bytes.HasPrefix(packet, []byte("\x01vorbis")) || bytes.HasPrefix(packet, []byte("OpusHead"))
}

const m4aBrand = "M4A "

func m4aAudio(head []byte) bool {
	if len(head) < 16 || !bytes.Equal(head[4:8], []byte("ftyp")) {
		return false
	}
	size := int(binary.BigEndian.Uint32(head[0:4]))
	if size < 16 || size > len(head) || (size-16)%4 != 0 {
		return false
	}
	if string(head[8:12]) == m4aBrand {
		return true
	}
	for offset := 16; offset+4 <= size; offset += 4 {
		if string(head[offset:offset+4]) == m4aBrand {
			return true
		}
	}
	return false
}

func id3Tagged(head []byte) bool {
	if len(head) < 10 || !bytes.Equal(head[0:3], []byte("ID3")) || head[3] < 2 || head[3] > 4 || head[4] == 0xFF {
		return false
	}
	for _, sizeByte := range head[6:10] {
		if sizeByte&0x80 != 0 {
			return false
		}
	}
	return true
}

const mpegFramesWithoutTag = 3

var (
	mpeg1LayerThreeBitrates = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	mpeg2LayerThreeBitrates = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	mpegSampleRates         = map[byte][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}
)

func mpegFrameLength(header []byte) (int, bool) {
	if len(header) < 4 || header[0] != 0xFF || header[1]&0xE0 != 0xE0 {
		return 0, false
	}
	version := (header[1] >> 3) & 0x03
	layer := (header[1] >> 1) & 0x03
	bitrateIndex := header[2] >> 4
	rateIndex := (header[2] >> 2) & 0x03
	padding := int((header[2] >> 1) & 0x01)
	rates, known := mpegSampleRates[version]
	if !known || layer != 0x01 || rateIndex == 3 {
		return 0, false
	}
	sampleRate := rates[rateIndex]
	if version == 3 {
		bitrate := mpeg1LayerThreeBitrates[bitrateIndex]
		if bitrate == 0 {
			return 0, false
		}
		return 144*bitrate*1000/sampleRate + padding, true
	}
	bitrate := mpeg2LayerThreeBitrates[bitrateIndex]
	if bitrate == 0 {
		return 0, false
	}
	return 72*bitrate*1000/sampleRate + padding, true
}

func mpegFrames(head []byte, required int) bool {
	offset := 0
	for frame := range required {
		if offset > len(head) {
			return false
		}
		length, valid := mpegFrameLength(head[offset:])
		if !valid {
			return false
		}
		if frame < required-1 {
			offset += length
		}
	}
	return true
}

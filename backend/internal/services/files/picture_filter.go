package files

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"ovc/backend/internal/domain"
)

type pictureSize struct {
	width  int
	height int
}

func (s pictureSize) known() bool {
	return s.width > 0 && s.height > 0
}

func (s pictureSize) previewable() bool {
	return s.known() && int64(s.width)*int64(s.height) <= domain.PreviewMaxPixels
}

var streamedFilters = map[fileKind]func(*bufio.Reader, io.Writer) (pictureSize, error){
	kindJPEG: filterJPEG, kindPNG: filterPNG, kindGIF: filterGIF,
}

func filterPicture(kind fileKind, input *bufio.Reader, output io.Writer) (pictureSize, error) {
	filter, streamed := streamedFilters[kind]
	if !streamed {
		panic("the picture filter got a kind it does not stream")
	}
	size, err := filter(input, output)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return pictureSize{}, ErrUnsupported
	}
	return size, err
}

func writeAll(output io.Writer, chunks ...[]byte) error {
	for _, chunk := range chunks {
		if _, err := output.Write(chunk); err != nil {
			return fmt.Errorf("write a filtered picture: %w", err)
		}
	}
	return nil
}

func copyExactly(output io.Writer, input io.Reader, count int64) error {
	copied, err := io.CopyN(output, input, count)
	if err != nil && copied < count {
		return err
	}
	return nil
}

func discardExactly(input *bufio.Reader, count int64) error {
	for count > 0 {
		step := min(count, int64(input.Size()))
		discarded, err := input.Discard(int(step))
		count -= int64(discarded)
		if err != nil {
			return err
		}
	}
	return nil
}

const (
	markerPrefix        = 0xFF
	markerStartOfImage  = 0xD8
	markerEndOfImage    = 0xD9
	markerStartOfScan   = 0xDA
	markerQuantization  = 0xDB
	markerHuffman       = 0xC4
	markerRestartPeriod = 0xDD
	markerTemporary     = 0x01
	markerApplication0  = 0xE0
	markerApplication1  = 0xE1
	markerApplication2  = 0xE2
	markerApplication14 = 0xEE
	jfifPrefixBytes     = 14
	exifOrientationTag  = 0x0112
	tiffShortType       = 3
)

var (
	jfifIdentifier  = []byte("JFIF\x00")
	exifIdentifier  = []byte("Exif\x00\x00")
	iccIdentifier   = []byte("ICC_PROFILE\x00")
	adobeIdentifier = []byte("Adobe")
)

func startOfFrame(marker byte) bool {
	switch marker {
	case 0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
		return true
	}
	return false
}

func restartMarker(marker byte) bool {
	return marker >= 0xD0 && marker <= 0xD7
}

func filterJPEG(input *bufio.Reader, output io.Writer) (pictureSize, error) {
	start := make([]byte, 2)
	if _, err := io.ReadFull(input, start); err != nil {
		return pictureSize{}, err
	}
	if start[0] != markerPrefix || start[1] != markerStartOfImage {
		return pictureSize{}, ErrUnsupported
	}
	if err := writeAll(output, start); err != nil {
		return pictureSize{}, err
	}
	var size pictureSize
	orientationKept, framed := false, false
	marker, err := nextMarker(input)
	for err == nil {
		switch {
		case marker == markerEndOfImage:
			if !framed {
				return pictureSize{}, ErrUnsupported
			}
			return size, writeAll(output, []byte{markerPrefix, markerEndOfImage})
		case marker == markerTemporary, restartMarker(marker):
			if err := writeAll(output, []byte{markerPrefix, marker}); err != nil {
				return pictureSize{}, err
			}
			marker, err = nextMarker(input)
			continue
		}
		var segment []byte
		segment, err = readSegment(input)
		if err != nil {
			return pictureSize{}, err
		}
		kept, keep := keptSegment(marker, segment, &orientationKept)
		if startOfFrame(marker) {
			if len(segment) < 5 {
				return pictureSize{}, ErrUnsupported
			}
			size = pictureSize{height: int(binary.BigEndian.Uint16(segment[1:3])), width: int(binary.BigEndian.Uint16(segment[3:5]))}
			framed = true
		}
		if keep {
			if err := writeSegment(output, marker, kept); err != nil {
				return pictureSize{}, err
			}
		}
		if marker == markerStartOfScan {
			if !framed {
				return pictureSize{}, ErrUnsupported
			}
			marker, err = copyEntropyData(input, output)
			continue
		}
		marker, err = nextMarker(input)
	}
	return pictureSize{}, err
}

func nextMarker(input *bufio.Reader) (byte, error) {
	prefix, err := input.ReadByte()
	if err != nil {
		return 0, err
	}
	if prefix != markerPrefix {
		return 0, ErrUnsupported
	}
	for {
		marker, err := input.ReadByte()
		if err != nil {
			return 0, err
		}
		if marker != markerPrefix {
			if marker == 0x00 {
				return 0, ErrUnsupported
			}
			return marker, nil
		}
	}
}

func readSegment(input *bufio.Reader) ([]byte, error) {
	declared := make([]byte, 2)
	if _, err := io.ReadFull(input, declared); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(declared))
	if length < 2 {
		return nil, ErrUnsupported
	}
	segment := make([]byte, length-2)
	if _, err := io.ReadFull(input, segment); err != nil {
		return nil, err
	}
	return segment, nil
}

func writeSegment(output io.Writer, marker byte, segment []byte) error {
	length, fits := uint16Of(len(segment) + 2)
	if !fits {
		return ErrUnsupported
	}
	header := []byte{markerPrefix, marker, 0, 0}
	binary.BigEndian.PutUint16(header[2:], length)
	return writeAll(output, header, segment)
}

func keptSegment(marker byte, segment []byte, orientationKept *bool) ([]byte, bool) {
	switch {
	case marker == markerQuantization, marker == markerHuffman, marker == markerRestartPeriod, marker == markerStartOfScan, startOfFrame(marker):
		return segment, true
	case marker == markerApplication0:
		if len(segment) < jfifPrefixBytes || !bytes.HasPrefix(segment, jfifIdentifier) {
			return nil, false
		}
		rebuilt := bytes.Clone(segment[:jfifPrefixBytes])
		rebuilt[12], rebuilt[13] = 0, 0
		return rebuilt, true
	case marker == markerApplication2:
		return segment, bytes.HasPrefix(segment, iccIdentifier)
	case marker == markerApplication14:
		return segment, bytes.HasPrefix(segment, adobeIdentifier)
	case marker == markerApplication1:
		if *orientationKept || !bytes.HasPrefix(segment, exifIdentifier) {
			return nil, false
		}
		orientation, found := exifOrientation(segment[len(exifIdentifier):])
		if !found {
			return nil, false
		}
		*orientationKept = true
		return orientationSegment(orientation), true
	}
	return nil, false
}

func exifOrientation(tiff []byte) (uint16, bool) {
	if len(tiff) < 8 {
		return 0, false
	}
	var order binary.ByteOrder
	switch string(tiff[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0, false
	}
	directory := int64(order.Uint32(tiff[4:8]))
	if directory+2 > int64(len(tiff)) {
		return 0, false
	}
	entries := int64(order.Uint16(tiff[directory : directory+2]))
	for index := range entries {
		entry := directory + 2 + index*12
		if entry+12 > int64(len(tiff)) {
			return 0, false
		}
		if order.Uint16(tiff[entry:entry+2]) == exifOrientationTag && order.Uint16(tiff[entry+2:entry+4]) == tiffShortType {
			value := order.Uint16(tiff[entry+8 : entry+10])
			return value, value >= 1 && value <= 8
		}
	}
	return 0, false
}

func orientationSegment(orientation uint16) []byte {
	segment := bytes.Clone(exifIdentifier)
	segment = append(segment, 'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1)
	segment = binary.BigEndian.AppendUint16(segment, exifOrientationTag)
	segment = binary.BigEndian.AppendUint16(segment, tiffShortType)
	segment = binary.BigEndian.AppendUint32(segment, 1)
	segment = binary.BigEndian.AppendUint16(segment, orientation)
	return append(segment, 0, 0, 0, 0, 0, 0)
}

func copyEntropyData(input *bufio.Reader, output io.Writer) (byte, error) {
	for {
		chunk, err := input.ReadSlice(markerPrefix)
		if errors.Is(err, bufio.ErrBufferFull) {
			if err := writeAll(output, chunk); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		if err := writeAll(output, chunk[:len(chunk)-1]); err != nil {
			return 0, err
		}
		following, err := input.ReadByte()
		for err == nil && following == markerPrefix {
			following, err = input.ReadByte()
		}
		if err != nil {
			return 0, err
		}
		if following == 0x00 || restartMarker(following) {
			if err := writeAll(output, []byte{markerPrefix, following}); err != nil {
				return 0, err
			}
			continue
		}
		return following, nil
	}
}

var (
	pngSignature   = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	pngCritical    = map[string]bool{"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true}
	pngAncillaries = map[string]bool{
		"tRNS": true, "gAMA": true, "cHRM": true, "sRGB": true, "iCCP": true, "sBIT": true, "pHYs": true, "bKGD": true,
		"acTL": true, "fcTL": true, "fdAT": true,
	}
)

const (
	pngMaxChunkBytes = 1<<31 - 1
	pngHeaderChunk   = "IHDR"
	pngEndChunk      = "IEND"
)

func filterPNG(input *bufio.Reader, output io.Writer) (pictureSize, error) {
	signature := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(input, signature); err != nil {
		return pictureSize{}, err
	}
	if !bytes.Equal(signature, pngSignature) {
		return pictureSize{}, ErrUnsupported
	}
	if err := writeAll(output, signature); err != nil {
		return pictureSize{}, err
	}
	var size pictureSize
	first := true
	for {
		header := make([]byte, 8)
		if _, err := io.ReadFull(input, header); err != nil {
			return pictureSize{}, err
		}
		length := int64(binary.BigEndian.Uint32(header[0:4]))
		chunkType := string(header[4:8])
		if length > pngMaxChunkBytes || (first && chunkType != pngHeaderChunk) {
			return pictureSize{}, ErrUnsupported
		}
		first = false
		critical := header[4]&0x20 == 0
		switch {
		case chunkType == pngHeaderChunk:
			body := make([]byte, 13+4)
			if length != 13 {
				return pictureSize{}, ErrUnsupported
			}
			if _, err := io.ReadFull(input, body); err != nil {
				return pictureSize{}, err
			}
			if crc32.ChecksumIEEE(append(bytes.Clone(header[4:8]), body[:13]...)) != binary.BigEndian.Uint32(body[13:]) {
				return pictureSize{}, ErrUnsupported
			}
			size = pictureSize{width: int(binary.BigEndian.Uint32(body[0:4])), height: int(binary.BigEndian.Uint32(body[4:8]))}
			if err := writeAll(output, header, body); err != nil {
				return pictureSize{}, err
			}
		case pngCritical[chunkType] || pngAncillaries[chunkType]:
			if err := writeAll(output, header); err != nil {
				return pictureSize{}, err
			}
			if err := copyExactly(output, input, length+4); err != nil {
				return pictureSize{}, err
			}
		case critical:
			return pictureSize{}, ErrUnsupported
		default:
			if err := discardExactly(input, length+4); err != nil {
				return pictureSize{}, err
			}
		}
		if chunkType == pngEndChunk {
			return size, nil
		}
	}
}

const (
	gifImageSeparator      = 0x2C
	gifExtensionIntroducer = 0x21
	gifTrailer             = 0x3B
	gifGraphicControl      = 0xF9
	gifApplication         = 0xFF
	gifColourTableFlag     = 0x80
)

var gifLoopApplication = []byte("NETSCAPE2.0")

func filterGIF(input *bufio.Reader, output io.Writer) (pictureSize, error) {
	header := make([]byte, 13)
	if _, err := io.ReadFull(input, header); err != nil {
		return pictureSize{}, err
	}
	if !bytes.HasPrefix(header, []byte("GIF87a")) && !bytes.HasPrefix(header, []byte("GIF89a")) {
		return pictureSize{}, ErrUnsupported
	}
	size := pictureSize{width: int(binary.LittleEndian.Uint16(header[6:8])), height: int(binary.LittleEndian.Uint16(header[8:10]))}
	if err := writeAll(output, header); err != nil {
		return pictureSize{}, err
	}
	if err := copyColourTable(input, output, header[10]); err != nil {
		return pictureSize{}, err
	}
	for {
		introducer, err := input.ReadByte()
		if err != nil {
			return pictureSize{}, err
		}
		switch introducer {
		case gifTrailer:
			return size, writeAll(output, []byte{gifTrailer})
		case gifImageSeparator:
			if err := copyGIFImage(input, output); err != nil {
				return pictureSize{}, err
			}
		case gifExtensionIntroducer:
			if err := filterGIFExtension(input, output); err != nil {
				return pictureSize{}, err
			}
		default:
			return pictureSize{}, ErrUnsupported
		}
	}
}

func copyColourTable(input *bufio.Reader, output io.Writer, packed byte) error {
	if packed&gifColourTableFlag == 0 {
		return nil
	}
	return copyExactly(output, input, int64(3)<<((packed&0x07)+1))
}

func copyGIFImage(input *bufio.Reader, output io.Writer) error {
	descriptor := make([]byte, 9)
	if _, err := io.ReadFull(input, descriptor); err != nil {
		return err
	}
	if err := writeAll(output, []byte{gifImageSeparator}, descriptor); err != nil {
		return err
	}
	if err := copyColourTable(input, output, descriptor[8]); err != nil {
		return err
	}
	if err := copyExactly(output, input, 1); err != nil {
		return err
	}
	return passSubBlocks(input, output)
}

func filterGIFExtension(input *bufio.Reader, output io.Writer) error {
	label, err := input.ReadByte()
	if err != nil {
		return err
	}
	switch label {
	case gifGraphicControl:
		if err := writeAll(output, []byte{gifExtensionIntroducer, label}); err != nil {
			return err
		}
		return passSubBlocks(input, output)
	case gifApplication:
		identifier, err := input.Peek(1 + len(gifLoopApplication))
		if err != nil {
			return err
		}
		if int(identifier[0]) == len(gifLoopApplication) && bytes.Equal(identifier[1:], gifLoopApplication) {
			if err := writeAll(output, []byte{gifExtensionIntroducer, label}); err != nil {
				return err
			}
			return passSubBlocks(input, output)
		}
	}
	return passSubBlocks(input, io.Discard)
}

func passSubBlocks(input *bufio.Reader, output io.Writer) error {
	for {
		length, err := input.ReadByte()
		if err != nil {
			return err
		}
		if err := writeAll(output, []byte{length}); err != nil {
			return err
		}
		if length == 0 {
			return nil
		}
		if err := copyExactly(output, input, int64(length)); err != nil {
			return err
		}
	}
}

const (
	webpFlagICC       = 0x20
	webpFlagExif      = 0x08
	webpFlagXMP       = 0x04
	webpChunkHeader   = 8
	webpExtendedBytes = 10
)

var webpKeptChunks = map[string]bool{"VP8 ": true, "VP8L": true, "VP8X": true, "ALPH": true, "ANIM": true, "ANMF": true, "ICCP": true}

func filterWebP(data []byte) ([]byte, pictureSize, error) {
	if len(data) < 12 || !riffForm(data, "WEBP") {
		return nil, pictureSize{}, ErrUnsupported
	}
	declared := int64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declared > int64(len(data)) || declared < 12 {
		return nil, pictureSize{}, ErrUnsupported
	}
	written := 12
	extended := -1
	iccKept := false
	var size pictureSize
	for offset := int64(12); offset < declared; {
		if offset+webpChunkHeader > declared {
			return nil, pictureSize{}, ErrUnsupported
		}
		fourCC := string(data[offset : offset+4])
		length := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		total := webpChunkHeader + length + length%2
		if offset+webpChunkHeader+length > declared {
			return nil, pictureSize{}, ErrUnsupported
		}
		total = min(total, declared-offset)
		body := data[offset+webpChunkHeader : offset+webpChunkHeader+length]
		if size == (pictureSize{}) {
			size = webpChunkSize(fourCC, body)
		}
		if webpKeptChunks[fourCC] {
			if fourCC == "VP8X" {
				extended = written
			}
			if fourCC == "ICCP" {
				iccKept = true
			}
			written += copy(data[written:], data[offset:offset+total])
		}
		offset += total
	}
	if extended >= 0 {
		if extended+webpChunkHeader+webpExtendedBytes > written {
			return nil, pictureSize{}, ErrUnsupported
		}
		flags := data[extended+webpChunkHeader] &^ (webpFlagExif | webpFlagXMP | webpFlagICC)
		if iccKept {
			flags |= webpFlagICC
		}
		data[extended+webpChunkHeader] = flags
	}
	binary.LittleEndian.PutUint32(data[4:8], uint32(written-8))
	return data[:written], size, nil
}

func webpChunkSize(fourCC string, body []byte) pictureSize {
	switch {
	case fourCC == "VP8X" && len(body) >= webpExtendedBytes:
		return pictureSize{width: int(uint24(body[4:7])) + 1, height: int(uint24(body[7:10])) + 1}
	case fourCC == "VP8L" && len(body) >= 5 && body[0] == 0x2F:
		bits := binary.LittleEndian.Uint32(body[1:5])
		return pictureSize{width: int(bits&0x3FFF) + 1, height: int((bits>>14)&0x3FFF) + 1}
	case fourCC == "VP8 " && len(body) >= 10 && bytes.Equal(body[3:6], []byte{0x9D, 0x01, 0x2A}):
		return pictureSize{width: int(binary.LittleEndian.Uint16(body[6:8]) & 0x3FFF), height: int(binary.LittleEndian.Uint16(body[8:10]) & 0x3FFF)}
	}
	return pictureSize{}
}

func uint24(data []byte) uint32 {
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
}

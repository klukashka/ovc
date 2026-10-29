package files

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func picture(width int, height int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 200, G: 120, B: 90, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, width/2, height/2), &image.Uniform{C: color.RGBA{R: 20, G: 40, B: 160, A: 255}}, image.Point{}, draw.Src)
	return canvas
}

func encodedJPEG(t *testing.T, width int, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, picture(width, height), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encodedPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture(width, height)); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encodedGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	frames := []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 4, 3), palette), image.NewPaletted(image.Rect(0, 0, 4, 3), palette)}
	var buffer bytes.Buffer
	if err := gif.EncodeAll(&buffer, &gif.GIF{Image: frames, Delay: []int{10, 10}, LoopCount: 0}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func jpegSegment(t *testing.T, marker byte, payload []byte) []byte {
	t.Helper()
	length, fits := uint16Of(len(payload) + 2)
	if !fits {
		t.Fatal("a fixture segment is too long")
	}
	segment := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], length)
	return append(segment, payload...)
}

func exifWithLocation(orientation uint16) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 2, 0}
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x8825)
	tiff = binary.LittleEndian.AppendUint16(tiff, 4)
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)
	tiff = binary.LittleEndian.AppendUint32(tiff, 38)
	tiff = binary.LittleEndian.AppendUint16(tiff, exifOrientationTag)
	tiff = binary.LittleEndian.AppendUint16(tiff, tiffShortType)
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	tiff = append(tiff, []byte("GPS 55.7558N 37.6173E")...)
	return append(bytes.Clone(exifIdentifier), tiff...)
}

func withSegmentsAfterStart(original []byte, segments ...[]byte) []byte {
	result := bytes.Clone(original[:2])
	for _, segment := range segments {
		result = append(result, segment...)
	}
	return append(result, original[2:]...)
}

func pngChunk(t *testing.T, kind string, body []byte) []byte {
	t.Helper()
	chunk := binary.BigEndian.AppendUint32(nil, size32(t, len(body)))
	chunk = append(chunk, kind...)
	chunk = append(chunk, body...)
	return binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(append([]byte(kind), body...)))
}

func pngWithChunkAfterHeader(original []byte, chunk []byte) []byte {
	headerEnd := 8 + 8 + 13 + 4
	result := bytes.Clone(original[:headerEnd])
	result = append(result, chunk...)
	return append(result, original[headerEnd:]...)
}

func webpChunk(t *testing.T, fourCC string, body []byte) []byte {
	t.Helper()
	chunk := append([]byte(fourCC), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(chunk[4:], size32(t, len(body)))
	chunk = append(chunk, body...)
	if len(body)%2 == 1 {
		chunk = append(chunk, 0)
	}
	return chunk
}

func webpOf(t *testing.T, chunks ...[]byte) []byte {
	t.Helper()
	data := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, chunk := range chunks {
		data = append(data, chunk...)
	}
	binary.LittleEndian.PutUint32(data[4:], size32(t, len(data)-8))
	return data
}

func extendedHeader(t *testing.T, flags byte, width int, height int) []byte {
	t.Helper()
	body := []byte{flags, 0, 0, 0}
	body = append(body, binary.LittleEndian.AppendUint32(nil, size32(t, width-1))[:3]...)
	return append(body, binary.LittleEndian.AppendUint32(nil, size32(t, height-1))[:3]...)
}

func losslessHeader(t *testing.T, width int, height int) []byte {
	t.Helper()
	bits := size32(t, width-1) | size32(t, height-1)<<14
	return binary.LittleEndian.AppendUint32([]byte{0x2F}, bits)
}

func filtered(t *testing.T, kind fileKind, data []byte) ([]byte, pictureSize, error) {
	t.Helper()
	if kind == kindWebP {
		return filterWebP(bytes.Clone(data))
	}
	var output bytes.Buffer
	size, err := filterPicture(kind, bufio.NewReaderSize(bytes.NewReader(data), detectionBytes), &output)
	return output.Bytes(), size, err
}

func TestPictureDimensionsAreReadFromHeaders(t *testing.T) {
	t.Parallel()
	huge := pngChunk(t, pngHeaderChunk, append(binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, 10_000), 6_000), 8, 2, 0, 0, 0))
	cases := []struct {
		name        string
		kind        fileKind
		data        []byte
		width       int
		height      int
		previewable bool
	}{
		{"a JPEG", kindJPEG, encodedJPEG(t, 33, 21), 33, 21, true},
		{"a PNG", kindPNG, encodedPNG(t, 17, 9), 17, 9, true},
		{"a GIF", kindGIF, encodedGIF(t), 4, 3, true},
		{"an extended WebP", kindWebP, webpOf(t, webpChunk(t, "VP8X", extendedHeader(t, 0, 640, 480)), webpChunk(t, "VP8L", losslessHeader(t, 640, 480))), 640, 480, true},
		{"a lossless WebP", kindWebP, webpOf(t, webpChunk(t, "VP8L", losslessHeader(t, 300, 200))), 300, 200, true},
		{"a lossy WebP", kindWebP, webpOf(t, webpChunk(t, "VP8 ", []byte{0x10, 0x02, 0x00, 0x9D, 0x01, 0x2A, 0x20, 0x03, 0x58, 0x02})), 800, 600, true},
		{"a PNG above 50 megapixels", kindPNG, append(append(bytes.Clone(pngSignature), huge...), pngChunk(t, pngEndChunk, nil)...), 10_000, 6_000, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, size, err := filtered(t, testCase.kind, testCase.data)

			if err != nil || size.width != testCase.width || size.height != testCase.height || size.previewable() != testCase.previewable {
				t.Fatalf("read %+v previewable %t: %v", size, size.previewable(), err)
			}
		})
	}
}

func TestPictureFilterKeepsOnlyAllowedStructures(t *testing.T) {
	t.Parallel()
	base := encodedJPEG(t, 24, 16)
	mpfSecond := append(jpegSegment(t, markerApplication2, []byte("MPF\x00second picture index")), encodedJPEG(t, 8, 8)...)
	jpegWithMetadata := withSegmentsAfterStart(base,
		jpegSegment(t, markerApplication0, []byte("JFIF\x00\x01\x02\x00\x00\x01\x00\x01\x02\x01THUMBY")),
		jpegSegment(t, markerApplication1, exifWithLocation(6)),
		jpegSegment(t, markerApplication1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>creator</x:xmpmeta>")),
		jpegSegment(t, 0xED, []byte("Photoshop 3.0\x00IPTC caption")),
		jpegSegment(t, 0xFE, []byte("a private comment")),
		jpegSegment(t, markerApplication2, []byte("ICC_PROFILE\x00\x01\x01profile")),
	)
	jpegWithMetadata = append(jpegWithMetadata, mpfSecond...)
	pngWithText := append(pngWithChunkAfterHeader(encodedPNG(t, 6, 6), pngChunk(t, "tEXt", []byte("Location\x00Moscow"))), []byte("appended data")...)
	pngWithText = pngWithChunkAfterHeader(pngWithText, pngChunk(t, "eXIf", []byte("MM\x00*exif")))
	gifData := encodedGIF(t)
	gifWithComments := append(bytes.Clone(gifData[:len(gifData)-1]),
		0x21, 0xFE, 5, 'h', 'e', 'l', 'l', 'o', 0,
		0x21, 0xFF, 11, 'X', 'M', 'P', ' ', 'D', 'a', 't', 'a', 'X', 'M', 'P', 3, 'x', 'm', 'p', 0,
		0x21, 0x01, 12, 0, 0, 0, 0, 1, 0, 1, 0, 8, 8, 0, 0, 2, 'h', 'i', 0,
		0x3B)
	gifWithComments = append(gifWithComments, []byte("after the trailer")...)
	webp := webpOf(t,
		webpChunk(t, "VP8X", extendedHeader(t, webpFlagICC|webpFlagExif|webpFlagXMP, 64, 48)),
		webpChunk(t, "ICCP", []byte("profile")),
		webpChunk(t, "VP8L", losslessHeader(t, 64, 48)),
		webpChunk(t, "EXIF", []byte("GPS 55.7558N")),
		webpChunk(t, "XMP ", []byte("<x:xmpmeta>creator</x:xmpmeta>")),
	)
	webp = append(webp, []byte("trailing")...)
	cases := []struct {
		name      string
		kind      fileKind
		data      []byte
		forbidden []string
		required  []string
	}{
		{"a JPEG with EXIF location, XMP, IPTC, a comment and an MPF second picture", kindJPEG, jpegWithMetadata,
			[]string{"GPS", "xmpmeta", "IPTC", "private comment", "MPF", "second picture", "THUMBY"}, []string{"ICC_PROFILE", "JFIF"}},
		{"a PNG with text and EXIF chunks and appended data", kindPNG, pngWithText,
			[]string{"tEXt", "Moscow", "eXIf", "appended data"}, []string{"IHDR", "IDAT", "IEND"}},
		{"a GIF with comments, a foreign application block, plain text and trailing data", kindGIF, gifWithComments,
			[]string{"hello", "XMP Data", "after the trailer"}, []string{"NETSCAPE2.0"}},
		{"a WebP with EXIF and XMP chunks and trailing data", kindWebP, webp,
			[]string{"EXIF", "XMP ", "GPS", "trailing"}, []string{"VP8X", "ICCP", "VP8L"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output, _, err := filtered(t, testCase.kind, testCase.data)

			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range testCase.forbidden {
				if bytes.Contains(output, []byte(forbidden)) {
					t.Errorf("the filtered picture keeps %q", forbidden)
				}
			}
			for _, required := range testCase.required {
				if !bytes.Contains(output, []byte(required)) {
					t.Errorf("the filtered picture lost %q", required)
				}
			}
		})
	}
}

func TestFilteredPicturesStillDecodeAndKeepTheirOrientation(t *testing.T) {
	t.Parallel()
	jpegData := withSegmentsAfterStart(encodedJPEG(t, 24, 16), jpegSegment(t, markerApplication1, exifWithLocation(6)), jpegSegment(t, 0xFE, []byte("note")))
	pngData := pngWithChunkAfterHeader(encodedPNG(t, 6, 6), pngChunk(t, "tEXt", []byte("Location\x00Moscow")))

	jpegOutput, _, jpegErr := filtered(t, kindJPEG, jpegData)
	pngOutput, _, pngErr := filtered(t, kindPNG, pngData)
	gifOutput, _, gifErr := filtered(t, kindGIF, encodedGIF(t))

	if jpegErr != nil || pngErr != nil || gifErr != nil {
		t.Fatal(jpegErr, pngErr, gifErr)
	}
	if _, err := jpeg.Decode(bytes.NewReader(jpegOutput)); err != nil {
		t.Fatalf("the filtered JPEG does not decode: %v", err)
	}
	if !bytes.Contains(jpegOutput, orientationSegment(6)) || !bytes.HasSuffix(jpegOutput, []byte{0xFF, markerEndOfImage}) {
		t.Fatal("the filtered JPEG lost its orientation or its end")
	}
	if _, err := png.Decode(bytes.NewReader(pngOutput)); err != nil {
		t.Fatalf("the filtered PNG does not decode: %v", err)
	}
	if animation, err := gif.DecodeAll(bytes.NewReader(gifOutput)); err != nil || len(animation.Image) != 2 || animation.LoopCount != 0 {
		t.Fatalf("the filtered GIF decodes as %v: %v", animation, err)
	}
}

func TestWebPFilterCorrectsItsSizeAndFlags(t *testing.T) {
	t.Parallel()
	data := webpOf(t,
		webpChunk(t, "VP8X", extendedHeader(t, webpFlagExif|webpFlagXMP|webpFlagICC, 64, 48)),
		webpChunk(t, "VP8L", losslessHeader(t, 64, 48)),
		webpChunk(t, "EXIF", []byte("GPS")),
	)

	output, _, err := filtered(t, kindWebP, data)

	if err != nil {
		t.Fatal(err)
	}
	if declared := binary.LittleEndian.Uint32(output[4:8]); int(declared) != len(output)-8 {
		t.Fatalf("the RIFF size is %d for %d bytes", declared, len(output))
	}
	if flags := output[12+webpChunkHeader]; flags&(webpFlagExif|webpFlagXMP|webpFlagICC) != 0 {
		t.Fatalf("the extended flags are %08b without EXIF, XMP or ICC chunks", flags)
	}
}

func TestPicturesThatDoNotParseAreRefused(t *testing.T) {
	t.Parallel()
	jpegData := encodedJPEG(t, 8, 8)
	cases := []struct {
		name string
		kind fileKind
		data []byte
	}{
		{"a JPEG without its end", kindJPEG, jpegData[:len(jpegData)-2]},
		{"a JPEG cut inside a segment", kindJPEG, jpegData[:30]},
		{"a PNG without its end", kindPNG, encodedPNG(t, 4, 4)[:40]},
		{"a PNG with an unknown critical chunk", kindPNG, pngWithChunkAfterHeader(encodedPNG(t, 4, 4), pngChunk(t, "ZZZZ", []byte("x")))},
		{"a GIF without its trailer", kindGIF, encodedGIF(t)[:20]},
		{"a WebP whose RIFF size passes its data", kindWebP, append([]byte("RIFF\xff\x00\x00\x00WEBP"), webpChunk(t, "VP8L", losslessHeader(t, 2, 2))...)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := filtered(t, testCase.kind, testCase.data)

			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

package files

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	relationshipsNamespace = "http://schemas.openxmlformats.org/package/2006/relationships"
	officeRelationships    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	wordMainType           = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	sheetMainType          = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	slideMainType          = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	textMimetype           = "application/vnd.oasis.opendocument.text"
)

type entry struct {
	name   string
	body   string
	stored bool
}

func containerOf(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, item := range entries {
		method := zip.Deflate
		if item.stored {
			method = zip.Store
		}
		part, err := writer.CreateHeader(&zip.FileHeader{Name: item.name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, item.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func relationships(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + relationshipsNamespace + `">` + strings.Join(items, "") + `</Relationships>`
}

func relationship(id string, kind string, target string, external bool) string {
	mode := ""
	if external {
		mode = ` TargetMode="External"`
	}
	return fmt.Sprintf(`<Relationship Id=%q Type=%q Target=%q%s/>`, id, officeRelationships+kind, target, mode)
}

func contentTypesWith(mainPart string, mainType string, extra ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/` + mainPart + `" ContentType="` + mainType + `"/>` + strings.Join(extra, "") + `</Types>`
}

func wordDocument(extraTypes []string, documentRelationships []string, extraEntries ...entry) []entry {
	entries := []entry{
		{name: contentTypesEntry, body: contentTypesWith("word/document.xml", wordMainType, extraTypes...)},
		{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "word/document.xml", false))},
		{name: "word/document.xml", body: `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`},
		{name: "word/_rels/document.xml.rels", body: relationships(documentRelationships...)},
	}
	return append(entries, extraEntries...)
}

func odfText(extra ...entry) []entry {
	entries := []entry{
		{name: odfMimetypeEntry, body: textMimetype, stored: true},
		{name: "content.xml", body: `<office:document-content/>`},
		{name: odfManifestEntry, body: manifestOf(`<manifest:file-entry manifest:full-path="/" manifest:media-type="` + textMimetype + `"/>` +
			`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>`)},
	}
	return append(entries, extra...)
}

func manifestOf(entries string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">` +
		entries + `</manifest:manifest>`
}

type countingReader struct {
	source io.ReaderAt
	reads  atomic.Int64
	bytes  atomic.Int64
}

func (c *countingReader) ReadAt(buffer []byte, offset int64) (int, error) {
	c.reads.Add(1)
	c.bytes.Add(int64(len(buffer)))
	return c.source.ReadAt(buffer, offset)
}

func inspect(data []byte, kind fileKind) (int64, error) {
	reader := &countingReader{source: bytes.NewReader(data)}
	err := inspectContainer(reader, int64(len(data)), kind)
	return reader.reads.Load(), err
}

func endOfDirectory(data []byte) int {
	return bytes.LastIndex(data, []byte{'P', 'K', 0x05, 0x06})
}

func patched(data []byte, mutate func(record []byte)) []byte {
	clone := bytes.Clone(data)
	mutate(clone[endOfDirectory(clone):])
	return clone
}

func paddedRelationships(bytesWanted int) string {
	item := relationship("rIdX", "styles", "styles.xml", false)
	count := bytesWanted / len(item)
	return relationships(strings.Repeat(item, count))
}

func TestOfficeContainersAreInspectedWithinTheirBounds(t *testing.T) {
	t.Parallel()
	plain := containerOf(t, wordDocument(nil, nil))
	many := make([]entry, 0, 2001)
	for index := range 2001 {
		many = append(many, entry{name: fmt.Sprintf("word/_rels/part%d.xml.rels", index), body: relationships()})
	}
	cases := []struct {
		name     string
		data     []byte
		kind     fileKind
		accepted bool
	}{
		{"a word document", plain, kindDOCX, true},
		{"a spreadsheet", containerOf(t, []entry{
			{name: contentTypesEntry, body: contentTypesWith("xl/workbook.xml", sheetMainType)},
			{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "xl/workbook.xml", false))},
			{name: "xl/workbook.xml", body: "<workbook/>"},
		}), kindXLSX, true},
		{"a presentation", containerOf(t, []entry{
			{name: contentTypesEntry, body: contentTypesWith("ppt/presentation.xml", slideMainType)},
			{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "/ppt/presentation.xml", false))},
			{name: "ppt/presentation.xml", body: "<presentation/>"},
		}), kindPPTX, true},
		{"an ODF text", containerOf(t, odfText()), kindODT, true},
		{"an ODF spreadsheet", containerOf(t, append([]entry{
			{name: odfMimetypeEntry, body: "application/vnd.oasis.opendocument.spreadsheet", stored: true},
		}, odfText()[1:]...)), kindODS, true},
		{"an ODF presentation", containerOf(t, append([]entry{
			{name: odfMimetypeEntry, body: "application/vnd.oasis.opendocument.presentation", stored: true},
		}, odfText()[1:]...)), kindODP, true},
		{"an ODF spreadsheet named as text", containerOf(t, append([]entry{
			{name: odfMimetypeEntry, body: "application/vnd.oasis.opendocument.spreadsheet", stored: true},
		}, odfText()[1:]...)), kindODT, false},
		{"a generic ZIP", containerOf(t, []entry{{name: "photos/one.jpg", body: "x"}}), kindDOCX, false},
		{"a declared entry count over 10,000", patched(plain, func(record []byte) {
			binary.LittleEndian.PutUint16(record[8:], 10_001)
			binary.LittleEndian.PutUint16(record[10:], 10_001)
		}), kindDOCX, false},
		{"a directory over 4 MiB", patched(plain, func(record []byte) {
			binary.LittleEndian.PutUint32(record[12:], containerMaxDirectoryBytes+1)
		}), kindDOCX, false},
		{"a directory outside the object", patched(plain, func(record []byte) {
			binary.LittleEndian.PutUint32(record[16:], size32(t, len(plain)))
		}), kindDOCX, false},
		{"duplicate names differing in case", containerOf(t, wordDocument(nil, nil, entry{name: "WORD/document.xml", body: "<x/>"})), kindDOCX, false},
		{"more than 2,000 relationship entries", containerOf(t, wordDocument(nil, nil, many...)), kindDOCX, false},
		{"more than 8 MiB of inflated relationship data", containerOf(t, wordDocument(nil, nil,
			entry{name: "word/_rels/one.xml.rels", body: paddedRelationships(5 << 20)},
			entry{name: "word/_rels/two.xml.rels", body: paddedRelationships(5 << 20)},
		)), kindDOCX, false},
		{"a single relationship file close to 8 MiB", containerOf(t, wordDocument(nil, nil,
			entry{name: "word/_rels/large.xml.rels", body: paddedRelationships(8<<20 - 64<<10)},
		)), kindDOCX, true},
		{"an oversized content types part", containerOf(t, []entry{
			{name: contentTypesEntry, body: contentTypesWith("word/document.xml", wordMainType, strings.Repeat(" ", contentTypesMaxBytes))},
			{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "word/document.xml", false))},
		}), kindDOCX, false},
		{"a manifest over 1 MiB", containerOf(t, []entry{
			{name: odfMimetypeEntry, body: textMimetype, stored: true},
			{name: odfManifestEntry, body: manifestOf(strings.Repeat(" ", manifestMaxBytes))},
		}), kindODT, false},
		{"a compressed mimetype", containerOf(t, append([]entry{{name: odfMimetypeEntry, body: textMimetype}}, odfText()[1:]...)), kindODT, false},
		{"a misplaced mimetype", containerOf(t, []entry{odfText()[2], odfText()[0], odfText()[1]}), kindODT, false},
		{"an oversized mimetype", containerOf(t, append([]entry{
			{name: odfMimetypeEntry, body: textMimetype + strings.Repeat(" ", odfMimetypeMaxBytes), stored: true},
		}, odfText()[1:]...)), kindODT, false},
		{"a truncated object", plain[:len(plain)/2], kindDOCX, false},
		{"entries whose local headers overlap", overlapping(plain), kindDOCX, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := inspect(testCase.data, testCase.kind)

			if (err == nil) != testCase.accepted || (err != nil && !errors.Is(err, ErrContainer)) {
				t.Fatalf("accepted %t: %v", testCase.accepted, err)
			}
		})
	}
}

func TestAnOrdinaryDocumentIsInspectedWithAHandfulOfReads(t *testing.T) {
	t.Parallel()
	data := containerOf(t, wordDocument(nil, []string{relationship("rId2", "hyperlink", "https://example.org", true)}))

	reads, err := inspect(data, kindDOCX)

	if err != nil || reads > 4 {
		t.Fatalf("the inspection made %d reads: %v", reads, err)
	}
}

func TestAHundredSlideDeckWithNotesIsAccepted(t *testing.T) {
	t.Parallel()
	entries := []entry{
		{name: contentTypesEntry, body: contentTypesWith("ppt/presentation.xml", slideMainType)},
		{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "ppt/presentation.xml", false))},
		{name: "ppt/presentation.xml", body: "<presentation/>"},
	}
	deckRelationships := make([]string, 0, 100)
	for slide := 1; slide <= 100; slide++ {
		deckRelationships = append(deckRelationships, relationship(fmt.Sprintf("rId%d", slide+1), "slide", fmt.Sprintf("slides/slide%d.xml", slide), false))
		entries = append(entries,
			entry{name: fmt.Sprintf("ppt/slides/slide%d.xml", slide), body: strings.Repeat("<p:sp/>", 400)},
			entry{name: fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slide), body: relationships(
				relationship("rId1", "slideLayout", "../slideLayouts/slideLayout1.xml", false),
				relationship("rId2", "notesSlide", fmt.Sprintf("../notesSlides/notesSlide%d.xml", slide), false),
				relationship("rId3", "image", "../media/image1.png", false),
				relationship("rId4", "hyperlink", "https://example.org/reading", true),
			)},
			entry{name: fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", slide), body: strings.Repeat("<p:sp/>", 100)},
			entry{name: fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", slide), body: relationships(
				relationship("rId1", "notesMaster", "../notesMasters/notesMaster1.xml", false),
				relationship("rId2", "slide", fmt.Sprintf("../slides/slide%d.xml", slide), false),
			)},
		)
	}
	entries = append(entries, entry{name: "ppt/_rels/presentation.xml.rels", body: relationships(deckRelationships...)})
	data := containerOf(t, entries)

	_, err := inspect(data, kindPPTX)

	if err != nil {
		t.Fatalf("a hundred-slide deck with notes was refused: %v", err)
	}
}

func TestRelationshipFilesAreBoundedInTotal(t *testing.T) {
	t.Parallel()
	exactly := make([]entry, 0, relationshipsMaxEntries-2)
	for index := range relationshipsMaxEntries - 2 {
		exactly = append(exactly, entry{name: fmt.Sprintf("word/_rels/p%d.xml.rels", index), body: relationships()})
	}
	cases := []struct {
		name     string
		entries  []entry
		accepted bool
	}{
		{"exactly 2,000 relationship entries", wordDocument(nil, nil, exactly...), true},
		{"2,001 relationship entries", wordDocument(nil, nil, append(exactly, entry{name: "word/_rels/extra.xml.rels", body: relationships()})...), false},
		{"three files summing to over 8 MiB", wordDocument(nil, nil,
			entry{name: "word/_rels/a.xml.rels", body: paddedRelationships(3 << 20)},
			entry{name: "word/_rels/b.xml.rels", body: paddedRelationships(3 << 20)},
			entry{name: "word/_rels/c.xml.rels", body: paddedRelationships(3 << 20)},
		), false},
		{"two files summing to under 8 MiB", wordDocument(nil, nil,
			entry{name: "word/_rels/a.xml.rels", body: paddedRelationships(3 << 20)},
			entry{name: "word/_rels/b.xml.rels", body: paddedRelationships(3 << 20)},
		), true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			data := containerOf(t, testCase.entries)

			_, err := inspect(data, kindDOCX)

			if (err == nil) != testCase.accepted {
				t.Fatalf("accepted %t: %v", testCase.accepted, err)
			}
		})
	}
}

func TestOfficeContentRulesFollowTheDecision(t *testing.T) {
	t.Parallel()
	vbaType := `<Default Extension="bin" ContentType="application/vnd.ms-office.vbaProject"/>`
	cases := []struct {
		name     string
		entries  []entry
		kind     fileKind
		accepted bool
	}{
		{"a macro by part name", wordDocument(nil, nil, entry{name: "word/vbaProject.bin", body: "x"}), kindDOCX, false},
		{"a macro by content type", wordDocument([]string{vbaType}, nil), kindDOCX, false},
		{"a macro by relationship type", wordDocument(nil, []string{
			`<Relationship Id="rId9" Type="http://schemas.microsoft.com/office/2006/relationships/vbaProject" Target="macros.bin"/>`,
		}), kindDOCX, false},
		{"a macro-enabled main type", []entry{
			{name: contentTypesEntry, body: contentTypesWith("word/document.xml", "application/vnd.ms-word.document.macroEnabled.main+xml")},
			{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "word/document.xml", false))},
		}, kindDOCX, false},
		{"a main part that disagrees with the extension", []entry{
			{name: contentTypesEntry, body: contentTypesWith("xl/workbook.xml", sheetMainType)},
			{name: packageRelationshipsEntry, body: relationships(relationship("rId1", "officeDocument", "xl/workbook.xml", false))},
		}, kindDOCX, false},
		{"an ActiveX part", wordDocument(nil, nil, entry{name: "word/activeX/activeX1.xml", body: "<ax/>"}), kindDOCX, false},
		{"an ActiveX relationship", wordDocument(nil, []string{relationship("rId9", "control", "activeX/activeX1.xml", false)}), kindDOCX, false},
		{"an OLE embedding", wordDocument(nil, nil, entry{name: "word/embeddings/oleObject1.bin", body: "x"}), kindDOCX, false},
		{"an OLE relationship", wordDocument(nil, []string{relationship("rId9", "oleObject", "embeddings/object.bin", false)}), kindDOCX, false},
		{"an external relationship that is not a hyperlink", wordDocument(nil, []string{
			relationship("rId9", "image", "https://tracker.example/pixel.png", true),
		}), kindDOCX, false},
		{"an external attached template", wordDocument(nil, []string{
			relationship("rId9", "attachedTemplate", "https://templates.example/normal.dotm", true),
		}), kindDOCX, false},
		{"an alternative format chunk", wordDocument(nil, []string{relationship("rId9", "aFChunk", "page.html", false)},
			entry{name: "word/page.html", body: "<html></html>"}), kindDOCX, false},
		{"an allowed hyperlink", wordDocument(nil, []string{relationship("rId9", "hyperlink", "https://example.org", true)}), kindDOCX, true},
		{"a native chart with its package workbook", wordDocument(
			[]string{`<Default Extension="xlsx" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"/>`},
			[]string{relationship("rId9", "chart", "charts/chart1.xml", false)},
			entry{name: "word/charts/chart1.xml", body: "<c:chartSpace/>"},
			entry{name: "word/charts/_rels/chart1.xml.rels", body: relationships(
				relationship("rId1", "package", "../embeddings/Microsoft_Excel_Worksheet.xlsx", false),
			)},
			entry{name: "word/embeddings/Microsoft_Excel_Worksheet.xlsx", body: "workbook"},
		), kindDOCX, true},
		{"an ODF Basic folder", odfText(entry{name: "Basic/Standard/Module1.xml", body: "<script/>"}), kindODT, false},
		{"an ODF Scripts folder", odfText(entry{name: "Scripts/python/run.py", body: "print()"}), kindODT, false},
		{"an ODF manifest script entry", []entry{
			odfText()[0], odfText()[1],
			{name: odfManifestEntry, body: manifestOf(`<manifest:file-entry manifest:full-path="run.bas" manifest:media-type="application/vnd.sun.star.basic"/>`)},
		}, kindODT, false},
		{"an ODF manifest OLE entry", []entry{
			odfText()[0], odfText()[1],
			{name: odfManifestEntry, body: manifestOf(`<manifest:file-entry manifest:full-path="Object 1" manifest:media-type="application/vnd.sun.star.oleobject"/>`)},
		}, kindODT, false},
		{"an ODF manifest entry outside the container", []entry{
			odfText()[0], odfText()[1],
			{name: odfManifestEntry, body: manifestOf(`<manifest:file-entry manifest:full-path="../outside.xml" manifest:media-type="text/xml"/>`)},
		}, kindODT, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			data := containerOf(t, testCase.entries)

			_, err := inspect(data, testCase.kind)

			if (err == nil) != testCase.accepted || (err != nil && !errors.Is(err, ErrContainer)) {
				t.Fatalf("accepted %t: %v", testCase.accepted, err)
			}
		})
	}
}

func manyEntries(t *testing.T, count int) []byte {
	t.Helper()
	entries := wordDocument(nil, nil)
	for index := range count {
		entries = append(entries, entry{name: fmt.Sprintf("word/media/p%d.bin", index), body: "x"})
	}
	return containerOf(t, entries)
}

func directoryRecordLength(data []byte, offset int) uint32 {
	name := binary.LittleEndian.Uint16(data[offset+28:])
	extra := binary.LittleEndian.Uint16(data[offset+30:])
	comment := binary.LittleEndian.Uint16(data[offset+32:])
	return uint32(directoryRecordBytes) + uint32(name) + uint32(extra) + uint32(comment)
}

func TestAnUnderstatedCentralDirectoryIsRefusedWithoutParsingItsHeaders(t *testing.T) {
	large := manyEntries(t, 9_000)
	record := endOfDirectory(large)
	directoryEnd := size32(t, record)
	directoryStart := binary.LittleEndian.Uint32(large[record+16:])
	lastRecordStart := bytes.LastIndex(large[:record], []byte{'P', 'K', 0x01, 0x02})
	firstRecord := directoryRecordLength(large, int(directoryStart))
	lastRecord := directoryRecordLength(large, lastRecordStart)
	understatedSize := patched(large, func(eocd []byte) {
		binary.LittleEndian.PutUint16(eocd[8:], 1)
		binary.LittleEndian.PutUint16(eocd[10:], 1)
		binary.LittleEndian.PutUint32(eocd[12:], firstRecord)
	})
	understatedCount := patched(large, func(eocd []byte) {
		binary.LittleEndian.PutUint16(eocd[8:], 1)
		binary.LittleEndian.PutUint16(eocd[10:], 1)
	})
	tooSmallForEntries := patched(large, func(eocd []byte) {
		binary.LittleEndian.PutUint32(eocd[12:], lastRecord)
		binary.LittleEndian.PutUint32(eocd[16:], directoryEnd-lastRecord)
	})
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, sizeErr := inspect(understatedSize, kindDOCX)
	_, countErr := inspect(understatedCount, kindDOCX)
	_, smallErr := inspect(tooSmallForEntries, kindDOCX)
	runtime.ReadMemStats(&after)

	for name, err := range map[string]error{"an understated size": sizeErr, "an understated count": countErr, "a directory too small for its entries": smallErr} {
		if !errors.Is(err, ErrContainer) {
			t.Errorf("%s was not refused: %v", name, err)
		}
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4<<20 {
		t.Fatalf("refusing the understated directories allocated %d bytes", allocated)
	}
}

func TestInspectionReadsEachEntryOnceAndStopsAtItsBudget(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("dense data that does not compress well ", 1)
	entries := wordDocument(nil, nil)
	for index := range 40 {
		entries = append(entries, entry{name: fmt.Sprintf("word/_rels/p%d.xml.rels", index), body: relationships(strings.Repeat(
			relationship(fmt.Sprintf("rId%d", index), "styles", payload, false), 200))})
	}
	data := containerOf(t, entries)
	reader := &countingReader{source: bytes.NewReader(data)}

	err := inspectContainer(reader, int64(len(data)), kindDOCX)
	refusedByBudget := inspectWithinBudget(bytes.NewReader(data), int64(len(data)), kindDOCX, int64(len(data))/2)

	if err != nil || reader.bytes.Load() > int64(len(data))+containerTailBytes {
		t.Fatalf("the inspection read %d bytes of a %d-byte object: %v", reader.bytes.Load(), len(data), err)
	}
	if !errors.Is(refusedByBudget, ErrContainer) {
		t.Fatalf("an inspection over its budget answered %v", refusedByBudget)
	}
}

func overlapping(data []byte) []byte {
	clone := bytes.Clone(data)
	directoryStart := int(binary.LittleEndian.Uint32(clone[endOfDirectory(clone)+16:]))
	first := bytes.Index(clone[directoryStart:], []byte{'P', 'K', 0x01, 0x02})
	second := directoryStart + first + 4 + bytes.Index(clone[directoryStart+first+4:], []byte{'P', 'K', 0x01, 0x02})
	binary.LittleEndian.PutUint32(clone[second+42:], 0)
	return clone
}

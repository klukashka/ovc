package files

import (
	"archive/zip"
	"cmp"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strings"
)

const (
	containerTailBytes         = 64<<10 + 22
	containerMaxEntries        = 10_000
	containerMaxDirectoryBytes = 4 << 20
	containerReadAhead         = 1 << 20
	containerInspectionBytes   = 32 << 20
	directoryRecordBytes       = 46
	directoryRecordSignature   = 0x02014b50
	zip64Marker32              = 0xFFFFFFFF
	zip64ExtraID               = 0x0001
	localHeaderBytes           = 30
	contentTypesMaxBytes       = 1 << 20
	manifestMaxBytes           = 1 << 20
	relationshipsMaxBytes      = 8 << 20
	relationshipsMaxEntries    = 2_000
	odfMimetypeMaxBytes        = 256
	endOfDirectoryBytes        = 22
	endOfDirectorySignature    = 0x06054b50
	zip64LocatorSignature      = 0x07064b50
	zip64LocatorBytes          = 20
	zip64EndSignature          = 0x06064b50
	zip64EndBytes              = 56
	contentTypesEntry          = "[Content_Types].xml"
	packageRelationshipsEntry  = "_rels/.rels"
	odfMimetypeEntry           = "mimetype"
	odfManifestEntry           = "META-INF/manifest.xml"
	externalTargetMode         = "External"
	officeDocumentRelationship = "officeDocument"
	hyperlinkRelationship      = "hyperlink"
	relationshipsSuffix        = ".rels"
	contentTypeOverride        = "Override"
	contentTypeDefault         = "Default"
	relationshipElement        = "Relationship"
	manifestEntryElement       = "file-entry"
	manifestPathAttribute      = "full-path"
	manifestMediaTypeAttribute = "media-type"
)

var ErrContainer = errors.New("the office container is refused")

var (
	ooxmlMainTypes = map[fileKind]string{
		kindDOCX: "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
		kindXLSX: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
		kindPPTX: "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml",
	}
	odfMimetypes = map[fileKind]string{
		kindODT: "application/vnd.oasis.opendocument.text",
		kindODS: "application/vnd.oasis.opendocument.spreadsheet",
		kindODP: "application/vnd.oasis.opendocument.presentation",
	}
	refusedRelationshipTypes = map[string]bool{
		"vbaProject": true, "vbaProjectSignature": true, "wordVbaData": true, "oleObject": true, "control": true,
		"activeXControl": true, "activeXControlBinary": true, "aFChunk": true,
	}
	refusedContentTypeFragments = []string{"macroenabled", "vbaproject", "activex", "oleobject"}
	refusedPartFragments        = []string{"vbaproject", "/activex/"}
	refusedManifestTypes        = []string{"script", "basic", "oleobject", "x-ole"}
	refusedODFFolders           = []string{"basic/", "scripts/"}
)

func refused(reason string) error {
	return fmt.Errorf("%w: %s", ErrContainer, reason)
}

type rangedObject struct {
	source     io.ReaderAt
	size       int64
	tailStart  int64
	tail       []byte
	dirStart   int64
	dirEnd     int64
	directory  []byte
	closing    []byte
	boundaries []int64
	window     []byte
	windowFrom int64
	budget     int64
	exhausted  bool
	failure    error
}

func (o *rangedObject) fetch(buffer []byte, offset int64) error {
	o.budget -= int64(len(buffer))
	if o.budget < 0 {
		o.exhausted = true
		return refused("too much data to inspect")
	}
	read, err := o.source.ReadAt(buffer, offset)
	if read == len(buffer) && (err == nil || errors.Is(err, io.EOF)) {
		return nil
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	o.failure = err
	return err
}

func (o *rangedObject) viewSize() int64 {
	return o.dirEnd + int64(len(o.closing))
}

func (o *rangedObject) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < 0 || offset >= o.viewSize() {
		return 0, io.EOF
	}
	wanted := min(int64(len(buffer)), o.viewSize()-offset)
	if offset >= o.dirEnd {
		return o.ended(copy(buffer[:wanted], o.closing[offset-o.dirEnd:]), len(buffer))
	}
	if offset+wanted > o.dirEnd {
		below, err := o.ReadAt(buffer[:o.dirEnd-offset], offset)
		if err != nil {
			return below, err
		}
		above := copy(buffer[below:wanted], o.closing)
		return o.ended(below+above, len(buffer))
	}
	for _, region := range []struct {
		start int64
		data  []byte
	}{{o.dirStart, o.directory}, {o.tailStart, o.tail}, {o.windowFrom, o.window}} {
		if offset >= region.start && offset+wanted <= region.start+int64(len(region.data)) {
			return o.ended(copy(buffer[:wanted], region.data[offset-region.start:]), len(buffer))
		}
	}
	span := min(int64(containerReadAhead), o.spanEnd(offset)-offset)
	if wanted > span {
		if err := o.fetch(buffer[:wanted], offset); err != nil {
			return 0, err
		}
		return o.ended(int(wanted), len(buffer))
	}
	o.window, o.windowFrom = make([]byte, span), offset
	if err := o.fetch(o.window, offset); err != nil {
		o.window = nil
		return 0, err
	}
	return o.ended(copy(buffer[:wanted], o.window), len(buffer))
}

func (o *rangedObject) spanEnd(offset int64) int64 {
	index, _ := slices.BinarySearch(o.boundaries, offset+1)
	if index < len(o.boundaries) {
		return o.boundaries[index]
	}
	return o.dirEnd
}

func (o *rangedObject) ended(copied int, requested int) (int, error) {
	if copied < requested {
		return copied, io.EOF
	}
	return copied, nil
}

type directoryBounds struct {
	entries uint64
	size    uint64
	offset  uint64
	end     int64
}

func inspectContainer(source io.ReaderAt, size int64, kind fileKind) error {
	return inspectWithinBudget(source, size, kind, containerInspectionBytes)
}

func inspectWithinBudget(source io.ReaderAt, size int64, kind fileKind, budget int64) error {
	object := &rangedObject{source: source, size: size, budget: budget}
	err := inspectWithin(object, kind)
	switch {
	case object.failure != nil:
		return object.failure
	case object.exhausted:
		return refused("too much data to inspect")
	}
	return err
}

func inspectWithin(object *rangedObject, kind fileKind) error {
	if object.size < endOfDirectoryBytes {
		return refused("too short")
	}
	tailBytes := min(object.size, containerTailBytes)
	object.tailStart = object.size - tailBytes
	object.tail = make([]byte, tailBytes)
	if err := object.fetch(object.tail, object.tailStart); err != nil {
		return err
	}
	bounds, err := locateDirectory(object)
	if err != nil {
		return err
	}
	if bounds.entries > containerMaxEntries {
		return refused("too many entries")
	}
	if bounds.size > containerMaxDirectoryBytes {
		return refused("directory too large")
	}
	end := unsigned(bounds.end)
	if bounds.offset > end || bounds.size != end-bounds.offset {
		return refused("directory does not end where the object says")
	}
	if bounds.entries*directoryRecordBytes > bounds.size {
		return refused("directory too small for its entries")
	}
	object.dirStart, object.dirEnd = signed(bounds.offset), bounds.end
	if object.dirStart >= object.tailStart {
		object.directory = object.tail[object.dirStart-object.tailStart : object.dirEnd-object.tailStart]
	} else {
		object.directory = make([]byte, bounds.size)
		if err := object.fetch(object.directory, object.dirStart); err != nil {
			return err
		}
	}
	if err := object.walkDirectory(bounds.entries); err != nil {
		return err
	}
	archive, err := zip.NewReader(object, object.viewSize())
	if err != nil {
		return refused("unreadable directory")
	}
	if uint64(len(archive.File)) != bounds.entries {
		return refused("entry count mismatch")
	}
	seen := make(map[string]bool, len(archive.File))
	for _, entry := range archive.File {
		folded := strings.ToLower(entry.Name)
		if seen[folded] {
			return refused("duplicate entry")
		}
		seen[folded] = true
	}
	if odfMimetypes[kind] != "" {
		return inspectODF(archive, kind)
	}
	return inspectOOXML(archive, kind)
}

func locateDirectory(object *rangedObject) (directoryBounds, error) {
	tail := object.tail
	for position := len(tail) - endOfDirectoryBytes; position >= 0; position-- {
		if binary.LittleEndian.Uint32(tail[position:]) != endOfDirectorySignature {
			continue
		}
		comment := int(binary.LittleEndian.Uint16(tail[position+20:]))
		if position+endOfDirectoryBytes+comment > len(tail) {
			continue
		}
		bounds := directoryBounds{
			entries: uint64(binary.LittleEndian.Uint16(tail[position+10:])),
			size:    uint64(binary.LittleEndian.Uint32(tail[position+12:])),
			offset:  uint64(binary.LittleEndian.Uint32(tail[position+16:])),
			end:     object.tailStart + int64(position),
		}
		if bounds.entries != 0xFFFF && bounds.size != 0xFFFFFFFF && bounds.offset != 0xFFFFFFFF {
			return bounds, nil
		}
		return locateZip64(object, position)
	}
	return directoryBounds{}, refused("no end of directory")
}

func locateZip64(object *rangedObject, endPosition int) (directoryBounds, error) {
	locator := endPosition - zip64LocatorBytes
	if locator < 0 || binary.LittleEndian.Uint32(object.tail[locator:]) != zip64LocatorSignature {
		return directoryBounds{}, refused("no zip64 locator")
	}
	recordOffset := binary.LittleEndian.Uint64(object.tail[locator+8:])
	recordEnd := unsigned(object.tailStart + int64(locator))
	if recordOffset > recordEnd || zip64EndBytes > recordEnd-recordOffset {
		return directoryBounds{}, refused("zip64 record outside the object")
	}
	record := make([]byte, zip64EndBytes)
	start := signed(recordOffset)
	if start >= object.tailStart {
		copy(record, object.tail[start-object.tailStart:])
	} else if err := object.fetch(record, start); err != nil {
		return directoryBounds{}, err
	}
	if binary.LittleEndian.Uint32(record) != zip64EndSignature {
		return directoryBounds{}, refused("no zip64 record")
	}
	return directoryBounds{
		entries: binary.LittleEndian.Uint64(record[32:]),
		size:    binary.LittleEndian.Uint64(record[40:]),
		offset:  binary.LittleEndian.Uint64(record[48:]),
		end:     start,
	}, nil
}

func findEntry(archive *zip.Reader, name string) *zip.File {
	for _, entry := range archive.File {
		if strings.EqualFold(entry.Name, name) {
			return entry
		}
	}
	return nil
}

func readBounded(entry *zip.File, limit int64) ([]byte, error) {
	if entry.UncompressedSize64 > unsigned(limit) {
		return nil, refused("entry too large")
	}
	opened, err := entry.Open()
	if err != nil {
		return nil, refused("unreadable entry")
	}
	data, readErr := io.ReadAll(io.LimitReader(opened, limit+1))
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil {
		return nil, refused("unreadable entry")
	}
	if int64(len(data)) > limit {
		return nil, refused("entry too large")
	}
	return data, nil
}

func lastSegment(relationshipType string) string {
	return relationshipType[strings.LastIndex(relationshipType, "/")+1:]
}

func containsAny(text string, fragments []string) bool {
	lowered := strings.ToLower(text)
	for _, fragment := range fragments {
		if strings.Contains(lowered, fragment) {
			return true
		}
	}
	return false
}

type contentTypes struct {
	defaults  map[string]string
	overrides map[string]string
}

func parseContentTypes(data []byte) (contentTypes, error) {
	types := contentTypes{defaults: map[string]string{}, overrides: map[string]string{}}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return types, nil
		}
		if err != nil {
			return contentTypes{}, refused("malformed content types")
		}
		element, opened := token.(xml.StartElement)
		if !opened {
			continue
		}
		contentType := attribute(element, "ContentType")
		if containsAny(contentType, refusedContentTypeFragments) {
			return contentTypes{}, refused("macro, ActiveX or OLE content type")
		}
		switch element.Name.Local {
		case contentTypeDefault:
			types.defaults[strings.ToLower(attribute(element, "Extension"))] = contentType
		case contentTypeOverride:
			types.overrides[strings.ToLower(strings.TrimPrefix(attribute(element, "PartName"), "/"))] = contentType
		}
	}
}

func (t contentTypes) of(part string) string {
	normalised := strings.ToLower(strings.TrimPrefix(part, "/"))
	if contentType, found := t.overrides[normalised]; found {
		return contentType
	}
	return t.defaults[strings.TrimPrefix(path.Ext(normalised), ".")]
}

func attribute(element xml.StartElement, name string) string {
	for _, candidate := range element.Attr {
		if candidate.Name.Local == name {
			return candidate.Value
		}
	}
	return ""
}

func inspectOOXML(archive *zip.Reader, kind fileKind) error {
	expected, office := ooxmlMainTypes[kind]
	if !office {
		return refused("not an office kind")
	}
	typesEntry := findEntry(archive, contentTypesEntry)
	if typesEntry == nil {
		return refused("no content types")
	}
	data, err := readBounded(typesEntry, contentTypesMaxBytes)
	if err != nil {
		return err
	}
	types, err := parseContentTypes(data)
	if err != nil {
		return err
	}
	relationshipFiles := 0
	for _, entry := range archive.File {
		if containsAny("/"+entry.Name, refusedPartFragments) || strings.Contains(strings.ToLower("/"+entry.Name), "/embeddings/oleobject") {
			return refused("macro, ActiveX or OLE part")
		}
		if strings.HasSuffix(strings.ToLower(entry.Name), relationshipsSuffix) {
			relationshipFiles++
		}
	}
	if relationshipFiles > relationshipsMaxEntries {
		return refused("too many relationship files")
	}
	budget := int64(relationshipsMaxBytes)
	mainPart := ""
	for _, entry := range archive.File {
		if !strings.HasSuffix(strings.ToLower(entry.Name), relationshipsSuffix) {
			continue
		}
		main, err := inspectRelationships(entry, &budget)
		if err != nil {
			return err
		}
		if strings.EqualFold(entry.Name, packageRelationshipsEntry) {
			mainPart = main
		}
	}
	if mainPart == "" || types.of(mainPart) != expected {
		return refused("main part disagrees with the extension")
	}
	return nil
}

type budgetedReader struct {
	source io.Reader
	budget *int64
}

func (r budgetedReader) Read(buffer []byte) (int, error) {
	if *r.budget < 0 {
		return 0, refused("relationship data too large")
	}
	read, err := r.source.Read(buffer[:min(int64(len(buffer)), *r.budget+1)])
	*r.budget -= int64(read)
	if *r.budget < 0 {
		return read, refused("relationship data too large")
	}
	return read, err
}

func inspectRelationships(entry *zip.File, budget *int64) (string, error) {
	opened, err := entry.Open()
	if err != nil {
		return "", refused("unreadable relationships")
	}
	main, inspectErr := relationshipsOf(xml.NewDecoder(budgetedReader{source: opened, budget: budget}))
	closeErr := opened.Close()
	if inspectErr != nil {
		return "", inspectErr
	}
	if closeErr != nil {
		return "", refused("unreadable relationships")
	}
	return main, nil
}

func relationshipsOf(decoder *xml.Decoder) (string, error) {
	main := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return main, nil
		}
		if errors.Is(err, ErrContainer) {
			return "", err
		}
		if err != nil {
			return "", refused("malformed relationships")
		}
		element, opened := token.(xml.StartElement)
		if !opened || element.Name.Local != relationshipElement {
			continue
		}
		relationshipType := lastSegment(attribute(element, "Type"))
		if refusedRelationshipTypes[relationshipType] {
			return "", refused("macro, ActiveX or OLE relationship")
		}
		if strings.EqualFold(attribute(element, "TargetMode"), externalTargetMode) && relationshipType != hyperlinkRelationship {
			return "", refused("external relationship")
		}
		if relationshipType == officeDocumentRelationship {
			main = attribute(element, "Target")
		}
	}
}

func inspectODF(archive *zip.Reader, kind fileKind) error {
	if len(archive.File) == 0 {
		return refused("empty")
	}
	first := archive.File[0]
	if first.Name != odfMimetypeEntry || first.Method != zip.Store || first.UncompressedSize64 > odfMimetypeMaxBytes {
		return refused("misplaced mimetype")
	}
	mimetype, err := readBounded(first, odfMimetypeMaxBytes)
	if err != nil {
		return err
	}
	if string(mimetype) != odfMimetypes[kind] {
		return refused("mimetype disagrees with the extension")
	}
	for _, entry := range archive.File {
		lowered := strings.ToLower(entry.Name)
		for _, folder := range refusedODFFolders {
			if strings.HasPrefix(lowered, folder) {
				return refused("script folder")
			}
		}
	}
	manifest := findEntry(archive, odfManifestEntry)
	if manifest == nil {
		return refused("no manifest")
	}
	data, err := readBounded(manifest, manifestMaxBytes)
	if err != nil {
		return err
	}
	return inspectManifest(data)
}

func inspectManifest(data []byte) error {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return refused("malformed manifest")
		}
		element, opened := token.(xml.StartElement)
		if !opened || element.Name.Local != manifestEntryElement {
			continue
		}
		if containsAny(attribute(element, manifestMediaTypeAttribute), refusedManifestTypes) {
			return refused("script or OLE manifest entry")
		}
		if outsideContainer(attribute(element, manifestPathAttribute)) {
			return refused("manifest entry outside the container")
		}
	}
}

func outsideContainer(entry string) bool {
	if entry == "/" {
		return false
	}
	normalised := strings.ReplaceAll(entry, `\`, "/")
	if strings.HasPrefix(normalised, "/") || strings.Contains(normalised, ":") {
		return true
	}
	for _, segment := range strings.Split(normalised, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

type entrySpan struct {
	offset  int64
	minimum int64
}

func (o *rangedObject) walkDirectory(entries uint64) error {
	spans := make([]entrySpan, 0, entries)
	var count uint64
	for position := 0; position < len(o.directory); {
		record := o.directory[position:]
		if len(record) < directoryRecordBytes || binary.LittleEndian.Uint32(record) != directoryRecordSignature {
			return refused("malformed directory record")
		}
		count++
		if count > entries {
			return refused("more entries than declared")
		}
		name := int(binary.LittleEndian.Uint16(record[28:]))
		extra := int(binary.LittleEndian.Uint16(record[30:]))
		comment := int(binary.LittleEndian.Uint16(record[32:]))
		total := directoryRecordBytes + name + extra + comment
		if total > len(record) {
			return refused("directory record outside the directory")
		}
		located, found := locateEntry(record, record[directoryRecordBytes+name:directoryRecordBytes+name+extra])
		if !found || located.offset >= unsigned(o.dirStart) || located.compressed > unsigned(o.dirStart) {
			return refused("entry outside the object")
		}
		offset := signed(located.offset)
		spans = append(spans, entrySpan{offset: offset, minimum: offset + localHeaderBytes + int64(name) + signed(located.compressed)})
		position += total
	}
	if count != entries {
		return refused("fewer entries than declared")
	}
	if o.dirStart > math.MaxUint32 || int64(len(o.directory)) > math.MaxUint32 {
		return refused("directory offset beyond the supported range")
	}
	slices.SortFunc(spans, func(left entrySpan, right entrySpan) int { return cmp.Compare(left.offset, right.offset) })
	o.boundaries = make([]int64, 0, len(spans)+1)
	for index, span := range spans {
		next := o.dirStart
		if index+1 < len(spans) {
			next = spans[index+1].offset
		}
		if span.minimum > next {
			return refused("overlapping entries")
		}
		o.boundaries = append(o.boundaries, span.offset)
	}
	o.boundaries = append(o.boundaries, o.dirStart)
	o.closing = closingRecord(entries, unsigned(int64(len(o.directory))), unsigned(o.dirStart))
	return nil
}

type locatedEntry struct {
	offset     uint64
	compressed uint64
}

func locateEntry(record []byte, extra []byte) (locatedEntry, bool) {
	located := locatedEntry{
		offset: uint64(binary.LittleEndian.Uint32(record[42:])), compressed: uint64(binary.LittleEndian.Uint32(record[20:])),
	}
	uncompressedMarked := binary.LittleEndian.Uint32(record[24:]) == zip64Marker32
	compressedMarked := located.compressed == zip64Marker32
	offsetMarked := located.offset == zip64Marker32
	if !compressedMarked && !offsetMarked {
		return located, true
	}
	for len(extra) >= 4 {
		id, size := binary.LittleEndian.Uint16(extra), int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			return locatedEntry{}, false
		}
		if id == zip64ExtraID {
			fields := extra[4 : 4+size]
			take := func(marked bool, target *uint64) bool {
				if !marked {
					return true
				}
				if len(fields) < 8 {
					return false
				}
				*target = binary.LittleEndian.Uint64(fields)
				fields = fields[8:]
				return true
			}
			var uncompressed uint64
			if take(uncompressedMarked, &uncompressed) && take(compressedMarked, &located.compressed) && take(offsetMarked, &located.offset) {
				return located, true
			}
			return locatedEntry{}, false
		}
		extra = extra[4+size:]
	}
	return locatedEntry{}, false
}

func closingRecord(entries uint64, size uint64, offset uint64) []byte {
	record := binary.LittleEndian.AppendUint32(nil, endOfDirectorySignature)
	record = append(record, 0, 0, 0, 0)
	count := binary.LittleEndian.AppendUint64(nil, entries)[:2]
	record = append(record, count...)
	record = append(record, count...)
	record = append(record, binary.LittleEndian.AppendUint64(nil, size)[:4]...)
	record = append(record, binary.LittleEndian.AppendUint64(nil, offset)[:4]...)
	return append(record, 0, 0)
}

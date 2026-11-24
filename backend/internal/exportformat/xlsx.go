package exportformat

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	RowsPerSheet         = 1_000_000
	MaxCellUnits         = 32767
	maxSheetTitleRunes   = 31
	maxSheetSuffixRunes  = len(" (99)")
	truncationMark       = "…"
	spreadsheetNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	relationsNamespace   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	packageRelations     = "http://schemas.openxmlformats.org/package/2006/relationships"
	xmlDeclaration       = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	dateStyle            = 1
	textStyle            = 2
	maxSheetParts        = 99
)

var (
	ErrUnsafeSheetTitle  = errors.New("the sheet title is empty, too long, repeated or holds a character a sheet name may not carry")
	ErrNoSheets          = errors.New("a workbook needs at least one sheet")
	ErrNumberNotFinite   = errors.New("a number cell must be finite")
	ErrSheetClosed       = errors.New("the sheet was closed by a later sheet")
	ErrTooManySheetParts = errors.New("a sheet passed the number of continuation parts a sheet title can name")
	spreadsheetEpoch     = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)
)

type cellKind int

const (
	cellEmpty cellKind = iota
	cellText
	cellNumber
	cellDate
)

type Cell struct {
	kind   cellKind
	text   string
	number float64
	date   time.Time
}

func Text(text string) Cell {
	return Cell{kind: cellText, text: text}
}

func Number(number float64) Cell {
	return Cell{kind: cellNumber, number: number}
}

func Integer(number int64) Cell {
	return Cell{kind: cellNumber, number: float64(number)}
}

func Date(wallClock time.Time) Cell {
	return Cell{kind: cellDate, date: wallClock}
}

func Empty() Cell {
	return Cell{kind: cellEmpty}
}

type Workbook struct {
	archive      *zip.Writer
	modified     time.Time
	rowsPerSheet int
	titles       []string
	taken        map[string]struct{}
	current      *Sheet
}

type Sheet struct {
	workbook *Workbook
	title    string
	headers  []string
	part     int
	rows     int
	output   *bufio.Writer
	closed   bool
}

func NewWorkbook(destination io.Writer, modified time.Time, rowsPerSheet int) *Workbook {
	if rowsPerSheet < 1 {
		panic("exportformat: a workbook needs at least one row per sheet")
	}
	archive := zip.NewWriter(destination)
	archive.RegisterCompressor(zip.Deflate, fastDeflate)
	return &Workbook{archive: archive, modified: modified.UTC(), rowsPerSheet: rowsPerSheet, taken: map[string]struct{}{}}
}

func (workbook *Workbook) StartSheet(title string, headers []string) (*Sheet, error) {
	if !safeSheetTitle(title) {
		return nil, ErrUnsafeSheetTitle
	}
	if _, repeated := workbook.taken[strings.ToLower(title)]; repeated {
		return nil, ErrUnsafeSheetTitle
	}
	if err := workbook.finishCurrent(); err != nil {
		return nil, err
	}
	sheet := &Sheet{workbook: workbook, title: title, headers: headers}
	if err := sheet.startPart(); err != nil {
		return nil, err
	}
	workbook.current = sheet
	return sheet, nil
}

func safeSheetTitle(title string) bool {
	count := utf8.RuneCountInString(title)
	if count == 0 || count > maxSheetTitleRunes-maxSheetSuffixRunes || !utf8.ValidString(title) {
		return false
	}
	if strings.ContainsAny(title, `[]:*?/\`) || strings.HasPrefix(title, "'") || strings.HasSuffix(title, "'") {
		return false
	}
	for _, letter := range title {
		if letter < ' ' {
			return false
		}
	}
	return true
}

func (sheet *Sheet) startPart() error {
	if sheet.part == maxSheetParts {
		return ErrTooManySheetParts
	}
	sheet.part++
	title := sheet.title
	if sheet.part > 1 {
		title = sheet.title + " (" + strconv.Itoa(sheet.part) + ")"
	}
	workbook := sheet.workbook
	if _, repeated := workbook.taken[strings.ToLower(title)]; repeated {
		return ErrUnsafeSheetTitle
	}
	workbook.taken[strings.ToLower(title)] = struct{}{}
	workbook.titles = append(workbook.titles, title)
	entry, err := workbook.create("xl/worksheets/sheet" + strconv.Itoa(len(workbook.titles)) + ".xml")
	if err != nil {
		return err
	}
	sheet.output = bufio.NewWriter(entry)
	sheet.rows = 0
	if _, err := sheet.output.WriteString(xmlDeclaration + `<worksheet xmlns="` + spreadsheetNamespace + `"><sheetData>`); err != nil {
		return fmt.Errorf("write a worksheet: %w", err)
	}
	if len(sheet.headers) == 0 {
		return nil
	}
	headers := make([]Cell, len(sheet.headers))
	for index, header := range sheet.headers {
		headers[index] = Text(header)
	}
	return sheet.writeRow(headers)
}

func (sheet *Sheet) WriteRow(cells ...Cell) error {
	if sheet.closed {
		return ErrSheetClosed
	}
	if sheet.rows-sheet.headerRows() >= sheet.workbook.rowsPerSheet {
		finished, started := sheet.finishPart(), error(nil)
		if finished == nil {
			started = sheet.startPart()
		}
		if err := errors.Join(finished, started); err != nil {
			sheet.closed = true
			sheet.workbook.current = nil
			return err
		}
	}
	return sheet.writeRow(cells)
}

func (sheet *Sheet) headerRows() int {
	if len(sheet.headers) == 0 {
		return 0
	}
	return 1
}

func (sheet *Sheet) writeRow(cells []Cell) error {
	sheet.rows++
	var row bytes.Buffer
	row.WriteString(`<row r="` + strconv.Itoa(sheet.rows) + `">`)
	for index, cell := range cells {
		if err := writeCell(&row, columnName(index)+strconv.Itoa(sheet.rows), cell); err != nil {
			return err
		}
	}
	row.WriteString(`</row>`)
	if _, err := sheet.output.Write(row.Bytes()); err != nil {
		return fmt.Errorf("write a worksheet row: %w", err)
	}
	return nil
}

func writeCell(row *bytes.Buffer, reference string, cell Cell) error {
	switch cell.kind {
	case cellEmpty:
		return nil
	case cellText:
		row.WriteString(`<c r="` + reference + `" s="` + strconv.Itoa(textStyle) + `" t="inlineStr"><is><t xml:space="preserve">`)
		if err := xml.EscapeText(row, []byte(escapeSpreadsheetEscapes(CutCellText(cell.text)))); err != nil {
			return fmt.Errorf("escape a cell: %w", err)
		}
		row.WriteString(`</t></is></c>`)
	case cellNumber:
		if math.IsNaN(cell.number) || math.IsInf(cell.number, 0) {
			return ErrNumberNotFinite
		}
		row.WriteString(`<c r="` + reference + `"><v>` + strconv.FormatFloat(cell.number, 'g', -1, 64) + `</v></c>`)
	case cellDate:
		row.WriteString(`<c r="` + reference + `" s="` + strconv.Itoa(dateStyle) + `"><v>` +
			strconv.FormatFloat(spreadsheetSerial(cell.date), 'f', -1, 64) + `</v></c>`)
	}
	return nil
}

func escapeSpreadsheetEscapes(text string) string {
	var escaped strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] == '_' && startsEscape(text[index+1:]) {
			escaped.WriteString("_x005F")
		}
		escaped.WriteByte(text[index])
	}
	return escaped.String()
}

func startsEscape(rest string) bool {
	if len(rest) < 6 || rest[0] != 'x' || rest[5] != '_' {
		return false
	}
	for _, digit := range rest[1:5] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", digit) {
			return false
		}
	}
	return true
}

func CutCellText(text string) string {
	text = strings.ToValidUTF8(text, string(utf8.RuneError))
	if len(utf16.Encode([]rune(text))) <= MaxCellUnits {
		return text
	}
	units := 0
	limit := MaxCellUnits - len(utf16.Encode([]rune(truncationMark)))
	for offset, letter := range text {
		size := utf16.RuneLen(letter)
		if units+size > limit {
			return text[:offset] + truncationMark
		}
		units += size
	}
	return text
}

func spreadsheetSerial(wallClock time.Time) float64 {
	clock := time.Date(wallClock.Year(), wallClock.Month(), wallClock.Day(), wallClock.Hour(), wallClock.Minute(),
		wallClock.Second(), 0, time.UTC)
	return clock.Sub(spreadsheetEpoch).Seconds() / (24 * 60 * 60)
}

func columnName(index int) string {
	name := ""
	for number := index + 1; number > 0; number = (number - 1) / 26 {
		name = string(rune('A'+(number-1)%26)) + name
	}
	return name
}

func (sheet *Sheet) finishPart() error {
	if _, err := sheet.output.WriteString(`</sheetData></worksheet>`); err != nil {
		return fmt.Errorf("finish a worksheet: %w", err)
	}
	if err := sheet.output.Flush(); err != nil {
		return fmt.Errorf("finish a worksheet: %w", err)
	}
	return nil
}

func (workbook *Workbook) finishCurrent() error {
	if workbook.current == nil {
		return nil
	}
	current := workbook.current
	workbook.current = nil
	current.closed = true
	return current.finishPart()
}

func (workbook *Workbook) create(name string) (io.Writer, error) {
	entry, err := workbook.archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: workbook.modified})
	if err != nil {
		return nil, fmt.Errorf("start a workbook part: %w", err)
	}
	return entry, nil
}

func (workbook *Workbook) Close() error {
	if err := workbook.finishCurrent(); err != nil {
		return err
	}
	if len(workbook.titles) == 0 {
		return ErrNoSheets
	}
	parts := []struct {
		name    string
		content func() (string, error)
	}{
		{"xl/workbook.xml", workbook.workbookPart},
		{"xl/_rels/workbook.xml.rels", workbook.workbookRelations},
		{"xl/styles.xml", stylesPart},
		{"_rels/.rels", packageRelationsPart},
		{"[Content_Types].xml", workbook.contentTypes},
	}
	for _, part := range parts {
		content, err := part.content()
		if err != nil {
			return err
		}
		entry, err := workbook.create(part.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(entry, content); err != nil {
			return fmt.Errorf("write a workbook part: %w", err)
		}
	}
	if err := workbook.archive.Close(); err != nil {
		return fmt.Errorf("finish the workbook: %w", err)
	}
	return nil
}

func (workbook *Workbook) workbookPart() (string, error) {
	var part bytes.Buffer
	part.WriteString(xmlDeclaration + `<workbook xmlns="` + spreadsheetNamespace + `" xmlns:r="` + relationsNamespace + `"><sheets>`)
	for index, title := range workbook.titles {
		part.WriteString(`<sheet name="`)
		if err := xml.EscapeText(&part, []byte(title)); err != nil {
			return "", fmt.Errorf("escape a sheet title: %w", err)
		}
		number := strconv.Itoa(index + 1)
		part.WriteString(`" sheetId="` + number + `" r:id="rId` + number + `"/>`)
	}
	part.WriteString(`</sheets></workbook>`)
	return part.String(), nil
}

func (workbook *Workbook) workbookRelations() (string, error) {
	var part strings.Builder
	part.WriteString(xmlDeclaration + `<Relationships xmlns="` + packageRelations + `">`)
	for index := range workbook.titles {
		number := strconv.Itoa(index + 1)
		part.WriteString(`<Relationship Id="rId` + number + `" Type="` + relationsNamespace +
			`/worksheet" Target="worksheets/sheet` + number + `.xml"/>`)
	}
	part.WriteString(`<Relationship Id="rId` + strconv.Itoa(len(workbook.titles)+1) + `" Type="` + relationsNamespace +
		`/styles" Target="styles.xml"/></Relationships>`)
	return part.String(), nil
}

func stylesPart() (string, error) {
	return xmlDeclaration + `<styleSheet xmlns="` + spreadsheetNamespace + `">` +
		`<numFmts count="1"><numFmt numFmtId="164" formatCode="yyyy-mm-dd hh:mm"/></numFmts>` +
		`<fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>` +
		`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="3"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
		`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
		`<xf numFmtId="49" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" quotePrefix="1"/></cellXfs>` +
		`</styleSheet>`, nil
}

func packageRelationsPart() (string, error) {
	return xmlDeclaration + `<Relationships xmlns="` + packageRelations + `"><Relationship Id="rId1" Type="` +
		relationsNamespace + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`, nil
}

func (workbook *Workbook) contentTypes() (string, error) {
	var part strings.Builder
	part.WriteString(xmlDeclaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for index := range workbook.titles {
		part.WriteString(`<Override PartName="/xl/worksheets/sheet` + strconv.Itoa(index+1) +
			`.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
	}
	part.WriteString(`</Types>`)
	return part.String(), nil
}

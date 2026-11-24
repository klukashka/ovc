package exportformat_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"ovc/backend/internal/exportformat"
)

type sheetCell struct {
	Reference string  `xml:"r,attr"`
	Type      string  `xml:"t,attr"`
	Style     string  `xml:"s,attr"`
	Formula   *string `xml:"f"`
	Value     string  `xml:"v"`
	Inline    string  `xml:"is>t"`
}

type sheetRow struct {
	Number string      `xml:"r,attr"`
	Cells  []sheetCell `xml:"c"`
}

type worksheet struct {
	Rows []sheetRow `xml:"sheetData>row"`
}

type workbookSheets struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
	} `xml:"sheets>sheet"`
}

type openedWorkbook struct {
	parts map[string][]byte
}

func buildWorkbook(t *testing.T, rowsPerSheet int, fill func(*exportformat.Workbook) error) openedWorkbook {
	t.Helper()
	var output bytes.Buffer
	workbook := exportformat.NewWorkbook(&output, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), rowsPerSheet)
	if err := fill(workbook); err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	opened := openedWorkbook{parts: map[string][]byte{}}
	for _, file := range reader.File {
		content, err := readZipFile(file)
		if err != nil {
			t.Fatal(err)
		}
		opened.parts[file.Name] = content
	}
	return opened
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(reader)
	return content, errors.Join(readErr, reader.Close())
}

func (opened openedWorkbook) sheet(t *testing.T, number string) worksheet {
	t.Helper()
	var parsed worksheet
	if err := xml.Unmarshal(opened.parts["xl/worksheets/sheet"+number+".xml"], &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

func (opened openedWorkbook) sheetNames(t *testing.T) []string {
	t.Helper()
	var parsed workbookSheets
	if err := xml.Unmarshal(opened.parts["xl/workbook.xml"], &parsed); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(parsed.Sheets))
	for _, sheet := range parsed.Sheets {
		names = append(names, sheet.Name)
	}
	return names
}

func writeOneRow(title string, cells ...exportformat.Cell) func(*exportformat.Workbook) error {
	return func(workbook *exportformat.Workbook) error {
		sheet, err := workbook.StartSheet(title, []string{"Text", "Number", "Date"})
		if err != nil {
			return err
		}
		return sheet.WriteRow(cells...)
	}
}

func TestWorkbookStoresFormulaLikeTextAsInlineStrings(t *testing.T) {
	texts := []string{"=1+1", "+cmd|' /C calc'!A0", "-2+3", "@SUM(A1)", `=HYPERLINK("https://evil.example")`}
	cells := make([]exportformat.Cell, 0, len(texts))
	for _, text := range texts {
		cells = append(cells, exportformat.Text(text))
	}

	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", cells...))

	row := opened.sheet(t, "1").Rows[1]
	for index, cell := range row.Cells {
		if cell.Type != "inlineStr" || cell.Formula != nil || cell.Inline != texts[index] {
			t.Errorf("cell %s = %+v; want the inline string %q", cell.Reference, cell, texts[index])
		}
	}
}

func TestWorkbookRoundTripsMarkupCharactersAsText(t *testing.T) {
	text := `<b>bold</b> & </t></is></c><c><f>1+1</f> "quoted" 'single'` + "\nsecond line  "

	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", exportformat.Text(text)))

	cell := opened.sheet(t, "1").Rows[1].Cells[0]
	if cell.Inline != text || cell.Formula != nil {
		t.Fatalf("cell = %+v; want the text %q unchanged", cell, text)
	}
}

func TestWorkbookHoldsNoActiveOrExternalParts(t *testing.T) {
	forbidden := []string{"<f>", "<f ", "hyperlink", "definedName", "externalLink", "comments", "vbaProject", "connections",
		"oleObject", "TargetMode"}

	opened := buildWorkbook(t, exportformat.RowsPerSheet,
		writeOneRow("Messages", exportformat.Text("=HYPERLINK(\"x\")"), exportformat.Integer(7), exportformat.Date(time.Date(2030, 3, 11, 18, 30, 0, 0, time.UTC))))

	wantParts := []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml",
		"xl/worksheets/sheet1.xml"}
	if len(opened.parts) != len(wantParts) {
		t.Fatalf("parts = %d; want exactly %v", len(opened.parts), wantParts)
	}
	for _, name := range wantParts {
		content, present := opened.parts[name]
		if !present {
			t.Fatalf("part %s missing", name)
		}
		for _, marker := range forbidden {
			if strings.Contains(string(content), marker) {
				t.Errorf("part %s holds %q", name, marker)
			}
		}
	}
}

func TestWorkbookWritesNumbersAndDatesAsValues(t *testing.T) {
	wallClock := time.Date(2030, 3, 11, 18, 30, 0, 0, time.FixedZone("Moscow", 3*60*60))

	opened := buildWorkbook(t, exportformat.RowsPerSheet,
		writeOneRow("Submissions", exportformat.Number(2.5), exportformat.Integer(-4), exportformat.Date(wallClock), exportformat.Empty()))

	cells := opened.sheet(t, "1").Rows[1].Cells
	if len(cells) != 3 {
		t.Fatalf("cells = %+v; want three, the empty cell left out", cells)
	}
	if cells[0].Value != "2.5" || cells[0].Type != "" || cells[1].Value != "-4" {
		t.Errorf("number cells = %+v, %+v", cells[0], cells[1])
	}
	wantSerial := 47553 + 18.5/24
	if cells[2].Style != "1" || !closeTo(cells[2].Value, wantSerial) {
		t.Errorf("date cell = %+v; want style 1 and serial %v of the wall clock", cells[2], wantSerial)
	}
}

func closeTo(text string, want float64) bool {
	var value float64
	if err := xml.Unmarshal([]byte("<v>"+text+"</v>"), &value); err != nil {
		return false
	}
	return math.Abs(value-want) < 1e-9
}

func TestWorkbookRefusesNumbersThatAreNotFinite(t *testing.T) {
	var output bytes.Buffer
	workbook := exportformat.NewWorkbook(&output, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), exportformat.RowsPerSheet)
	sheet, err := workbook.StartSheet("Submissions", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = sheet.WriteRow(exportformat.Number(math.NaN()))

	if !errors.Is(err, exportformat.ErrNumberNotFinite) {
		t.Fatalf("WriteRow(NaN) = %v; want ErrNumberNotFinite", err)
	}
}

func TestWorkbookReplacesCharactersXMLForbids(t *testing.T) {
	text := "bell\x07 nul\x00 form\x0c end￾"

	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", exportformat.Text(text)))

	cell := opened.sheet(t, "1").Rows[1].Cells[0]
	if cell.Inline != "bell� nul� form� end�" {
		t.Fatalf("cell text = %q; want each forbidden character replaced by U+FFFD", cell.Inline)
	}
}

func TestWorkbookCutsLongCellTextAtTheCellLimit(t *testing.T) {
	text := strings.Repeat("😀", 20000)

	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", exportformat.Text(text)))

	cell := opened.sheet(t, "1").Rows[1].Cells[0]
	units := len(utf16.Encode([]rune(cell.Inline)))
	if units > exportformat.MaxCellUnits || units < exportformat.MaxCellUnits-2 || !strings.HasSuffix(cell.Inline, "…") {
		t.Fatalf("cut cell holds %d UTF-16 units, suffix %q; want at most %d ending with …", units,
			cell.Inline[len(cell.Inline)-6:], exportformat.MaxCellUnits)
	}
}

func TestWorkbookKeepsTextAtTheCellLimitWhole(t *testing.T) {
	text := strings.Repeat("a", exportformat.MaxCellUnits)

	got := exportformat.CutCellText(text)

	if got != text {
		t.Fatalf("text of exactly the limit was cut to %d bytes", len(got))
	}
}

func TestWorkbookContinuesALongSheetUnderTheSameNameWithItsHeaders(t *testing.T) {
	fill := func(workbook *exportformat.Workbook) error {
		sheet, err := workbook.StartSheet("Messages", []string{"Body"})
		if err != nil {
			return err
		}
		for index := 0; index < 5; index++ {
			if err := sheet.WriteRow(exportformat.Integer(int64(index))); err != nil {
				return err
			}
		}
		files, err := workbook.StartSheet("Files", []string{"Path"})
		if err != nil {
			return err
		}
		return files.WriteRow(exportformat.Text("files/chat/x"))
	}

	opened := buildWorkbook(t, 2, fill)

	if got := strings.Join(opened.sheetNames(t), "|"); got != "Messages|Messages (2)|Messages (3)|Files" {
		t.Fatalf("sheets = %s", got)
	}
	last := opened.sheet(t, "3")
	if len(last.Rows) != 2 || last.Rows[0].Cells[0].Inline != "Body" || last.Rows[1].Cells[0].Value != "4" {
		t.Fatalf("third part = %+v; want the header and the fifth row", last.Rows)
	}
}

func TestWorkbookRefusesUnsafeOrRepeatedSheetTitles(t *testing.T) {
	titles := []string{"", "a/b", "a[1]", "what?", "'quoted'", "tab\there", strings.Repeat("x", 27), "messages"}
	var output bytes.Buffer
	workbook := exportformat.NewWorkbook(&output, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), exportformat.RowsPerSheet)
	if _, err := workbook.StartSheet("Messages", nil); err != nil {
		t.Fatal(err)
	}

	for _, title := range titles {
		_, err := workbook.StartSheet(title, nil)

		if !errors.Is(err, exportformat.ErrUnsafeSheetTitle) {
			t.Errorf("StartSheet(%q) = %v; want ErrUnsafeSheetTitle", title, err)
		}
	}
}

func TestWorkbookRefusesRowsForASheetALaterSheetClosed(t *testing.T) {
	var output bytes.Buffer
	workbook := exportformat.NewWorkbook(&output, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), exportformat.RowsPerSheet)
	first, err := workbook.StartSheet("Students", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workbook.StartSheet("Groups", nil); err != nil {
		t.Fatal(err)
	}

	err = first.WriteRow(exportformat.Text("late"))

	if !errors.Is(err, exportformat.ErrSheetClosed) {
		t.Fatalf("WriteRow on a closed sheet = %v; want ErrSheetClosed", err)
	}
}

func TestWorkbookWithoutSheetsIsRefused(t *testing.T) {
	workbook := exportformat.NewWorkbook(&bytes.Buffer{}, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), exportformat.RowsPerSheet)

	err := workbook.Close()

	if !errors.Is(err, exportformat.ErrNoSheets) {
		t.Fatalf("Close = %v; want ErrNoSheets", err)
	}
}

func TestWorkbookKeepsEscapeLikeTextFromBeingDecoded(t *testing.T) {
	text := "_x0041_ and _x000D_ and _x0041_x0042_ and __x0043__ stay as typed"

	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", exportformat.Text(text)))

	cell := opened.sheet(t, "1").Rows[1].Cells[0]
	if got := decodeSpreadsheetEscapes(cell.Inline); got != text {
		t.Fatalf("a spreadsheet program shows %q; want %q", got, text)
	}
}

func decodeSpreadsheetEscapes(text string) string {
	var decoded strings.Builder
	for index := 0; index < len(text); {
		if index+7 <= len(text) && text[index] == '_' && text[index+1] == 'x' && text[index+6] == '_' {
			if letter, err := strconv.Unquote(`"\u` + text[index+2:index+6] + `"`); err == nil {
				decoded.WriteString(letter)
				index += 7
				continue
			}
		}
		decoded.WriteByte(text[index])
		index++
	}
	return decoded.String()
}

func TestWorkbookTextCellsCarryTheQuotePrefixStyle(t *testing.T) {
	opened := buildWorkbook(t, exportformat.RowsPerSheet, writeOneRow("Messages", exportformat.Text("=1+1")))

	cell := opened.sheet(t, "1").Rows[1].Cells[0]
	if cell.Style != "2" || !strings.Contains(string(opened.parts["xl/styles.xml"]), `quotePrefix="1"/></cellXfs>`) {
		t.Fatalf("text cell style %q; want the third style, which sets the quote prefix", cell.Style)
	}
}

func TestWorkbookRefusesASheetPastItsLastContinuationPart(t *testing.T) {
	workbook := exportformat.NewWorkbook(&bytes.Buffer{}, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), 1)
	sheet, err := workbook.StartSheet("Messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 99; index++ {
		if err := sheet.WriteRow(exportformat.Integer(int64(index))); err != nil {
			t.Fatal(err)
		}
	}

	err = sheet.WriteRow(exportformat.Integer(99))

	if !errors.Is(err, exportformat.ErrTooManySheetParts) {
		t.Fatalf("the hundredth part = %v; want ErrTooManySheetParts", err)
	}
}

func TestWorkbookWithoutRowsPerSheetIsInvalidConfiguration(t *testing.T) {
	build := func() { exportformat.NewWorkbook(&bytes.Buffer{}, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), 0) }

	panicked := panics(build)

	if !panicked {
		t.Fatal("NewWorkbook with no rows per sheet did not panic")
	}
}

func panics(call func()) (panicked bool) {
	defer func() {
		panicked = recover() != nil
	}()
	call()
	return false
}

package exportformat_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/exportformat"
)

const lessonText = "3c5a7e91-2b4d-4f60-8a1c-9e0b7d6f5a42"

func lessonID(t *testing.T) domain.LessonID {
	t.Helper()
	id, ok := domain.ParseUUID[domain.LessonID](lessonText)
	if !ok {
		t.Fatal("bad lesson identifier")
	}
	return id
}

func writeCalendar(t *testing.T, events ...exportformat.CalendarEvent) string {
	t.Helper()
	var output strings.Builder
	calendar, err := exportformat.NewCalendar(&output, "ovc.example", time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := calendar.WriteEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := calendar.Close(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func unfoldedLines(text string) []string {
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(text, "\r\n ", ""), "\r\n"), "\r\n")
}

func propertyNames(lines []string) []string {
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		names = append(names, strings.SplitN(line, ":", 2)[0])
	}
	return names
}

func TestCalendarEventKeepsHostileTitleTextInsideSummary(t *testing.T) {
	moscow := time.FixedZone("Moscow", 3*60*60)
	event := exportformat.CalendarEvent{
		Lesson:      lessonID(t),
		Start:       time.Date(2030, 3, 11, 18, 0, 0, 0, moscow),
		End:         time.Date(2030, 3, 11, 19, 0, 0, 0, moscow),
		Summary:     "Algebra\nATTACH:https://evil.example/x\r\nURL:x;y,z\\w",
		Description: "Group lesson\n\x07bell",
		Cancelled:   true,
	}

	lines := unfoldedLines(writeCalendar(t, event))

	wantNames := []string{"BEGIN", "VERSION", "PRODID", "CALSCALE", "BEGIN", "UID", "DTSTAMP", "DTSTART", "DTEND", "SUMMARY",
		"DESCRIPTION", "STATUS", "END", "END"}
	if got := propertyNames(lines); strings.Join(got, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("properties = %v; want %v", got, wantNames)
	}
	wantLines := map[int]string{
		5:  "UID:" + lessonText + "@ovc.example",
		6:  "DTSTAMP:20300310T090000Z",
		7:  "DTSTART:20300311T150000Z",
		8:  "DTEND:20300311T160000Z",
		9:  `SUMMARY:Algebra\nATTACH:https://evil.example/x\nURL:x\;y\,z\\w`,
		10: `DESCRIPTION:Group lesson\nbell`,
		11: "STATUS:CANCELLED",
	}
	for index, want := range wantLines {
		if lines[index] != want {
			t.Errorf("line %d = %q; want %q", index, lines[index], want)
		}
	}
}

func TestCalendarLeavesStatusOutForALessonThatTakesPlace(t *testing.T) {
	event := exportformat.CalendarEvent{
		Lesson: lessonID(t),
		Start:  time.Date(2030, 3, 11, 15, 0, 0, 0, time.UTC),
		End:    time.Date(2030, 3, 11, 16, 0, 0, 0, time.UTC),
	}

	text := writeCalendar(t, event)

	if strings.Contains(text, "STATUS") {
		t.Fatalf("calendar of a lesson that takes place holds a status:\n%s", text)
	}
}

func TestCalendarLinesFoldAt75OctetsOnCharacterBoundaries(t *testing.T) {
	event := exportformat.CalendarEvent{
		Lesson:  lessonID(t),
		Start:   time.Date(2030, 3, 11, 15, 0, 0, 0, time.UTC),
		End:     time.Date(2030, 3, 11, 16, 0, 0, 0, time.UTC),
		Summary: strings.Repeat("Урок английского 🙂 ", 12),
	}

	text := writeCalendar(t, event)

	for _, line := range strings.Split(strings.TrimSuffix(text, "\r\n"), "\r\n") {
		if len(line) > 75 || !utf8.ValidString(line) {
			t.Errorf("line of %d octets, valid %v: %q", len(line), utf8.ValidString(line), line)
		}
	}
	if !strings.Contains(strings.Join(unfoldedLines(text), "\n"), "SUMMARY:"+event.Summary) {
		t.Fatal("unfolding does not restore the summary")
	}
}

func TestCalendarEscapesEachSpecialCharacterAndDropsControls(t *testing.T) {
	cases := map[string]string{
		`a\b`:          `a\\b`,
		"a;b":          `a\;b`,
		"a,b":          `a\,b`,
		"a\r\nb":       `a\nb`,
		"a\rb":         "ab",
		"a\x00\x1b\tb": "ab",
		"a\u0085b":     "ab",
	}

	for text, want := range cases {
		got := exportformat.EscapeCalendarText(text)

		if got != want {
			t.Errorf("EscapeCalendarText(%q) = %q; want %q", text, got, want)
		}
	}
}

func TestCalendarRefusesAHostThatCouldBreakTheUID(t *testing.T) {
	hosts := []string{"", "evil.example\r\nATTACH:x", "a b", "host;x"}

	for _, host := range hosts {
		_, err := exportformat.NewCalendar(&strings.Builder{}, host, time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC))

		if !errors.Is(err, exportformat.ErrUnsafeCalendarHost) {
			t.Errorf("NewCalendar(%q) = %v; want ErrUnsafeCalendarHost", host, err)
		}
	}
}

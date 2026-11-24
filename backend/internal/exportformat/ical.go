package exportformat

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/domain"
)

const (
	maxCalendarLineOctets = 75
	calendarTimeLayout    = "20060102T150405Z"
	calendarLineEnd       = "\r\n"
)

var ErrUnsafeCalendarHost = errors.New("the calendar host holds characters a UID may not carry")

type CalendarEvent struct {
	Lesson      domain.LessonID
	Start       time.Time
	End         time.Time
	Summary     string
	Description string
	Cancelled   bool
}

type Calendar struct {
	output *bufio.Writer
	host   string
	stamp  time.Time
}

func NewCalendar(destination io.Writer, host string, stamp time.Time) (*Calendar, error) {
	if !safeCalendarHost(host) {
		return nil, ErrUnsafeCalendarHost
	}
	calendar := &Calendar{output: bufio.NewWriter(destination), host: host, stamp: stamp.UTC()}
	for _, line := range []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//OVC//Export//EN", "CALSCALE:GREGORIAN"} {
		if err := calendar.writeLine(line); err != nil {
			return nil, err
		}
	}
	return calendar, nil
}

func safeCalendarHost(host string) bool {
	if host == "" {
		return false
	}
	for _, letter := range host {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-:[]", letter) {
			return false
		}
	}
	return true
}

func (calendar *Calendar) WriteEvent(event CalendarEvent) error {
	lines := []string{
		"BEGIN:VEVENT",
		"UID:" + event.Lesson.String() + "@" + calendar.host,
		"DTSTAMP:" + calendar.stamp.Format(calendarTimeLayout),
		"DTSTART:" + event.Start.UTC().Format(calendarTimeLayout),
		"DTEND:" + event.End.UTC().Format(calendarTimeLayout),
		"SUMMARY:" + EscapeCalendarText(event.Summary),
		"DESCRIPTION:" + EscapeCalendarText(event.Description),
	}
	if event.Cancelled {
		lines = append(lines, "STATUS:CANCELLED")
	}
	lines = append(lines, "END:VEVENT")
	for _, line := range lines {
		if err := calendar.writeLine(line); err != nil {
			return err
		}
	}
	return nil
}

func (calendar *Calendar) Close() error {
	if err := calendar.writeLine("END:VCALENDAR"); err != nil {
		return err
	}
	if err := calendar.output.Flush(); err != nil {
		return fmt.Errorf("finish the calendar: %w", err)
	}
	return nil
}

func (calendar *Calendar) writeLine(line string) error {
	if _, err := calendar.output.WriteString(FoldCalendarLine(line)); err != nil {
		return fmt.Errorf("write the calendar: %w", err)
	}
	return nil
}

func EscapeCalendarText(text string) string {
	var escaped strings.Builder
	for _, letter := range strings.ToValidUTF8(text, string(utf8.RuneError)) {
		switch {
		case letter == '\\' || letter == ';' || letter == ',':
			escaped.WriteRune('\\')
			escaped.WriteRune(letter)
		case letter == '\n':
			escaped.WriteString(`\n`)
		case unicode.IsControl(letter):
		default:
			escaped.WriteRune(letter)
		}
	}
	return escaped.String()
}

func FoldCalendarLine(line string) string {
	var folded strings.Builder
	octets := 0
	for _, letter := range line {
		size := utf8.RuneLen(letter)
		if octets+size > maxCalendarLineOctets {
			folded.WriteString(calendarLineEnd + " ")
			octets = 1
		}
		folded.WriteRune(letter)
		octets += size
	}
	folded.WriteString(calendarLineEnd)
	return folded.String()
}

package files

import (
	"unicode"
	"unicode/utf8"
)

type plainTextCheck struct {
	pending []byte
	valid   bool
}

func newPlainTextCheck() *plainTextCheck {
	return &plainTextCheck{valid: true, pending: make([]byte, 0, utf8.UTFMax)}
}

func (c *plainTextCheck) Write(data []byte) (int, error) {
	if !c.valid {
		return 0, ErrUnsupported
	}
	remaining := data
	for len(c.pending) > 0 && len(remaining) > 0 {
		c.pending = append(c.pending, remaining[0])
		remaining = remaining[1:]
		if utf8.FullRune(c.pending) {
			r, size := utf8.DecodeRune(c.pending)
			if !acceptedRune(r, size) || size != len(c.pending) {
				c.valid = false
				return 0, ErrUnsupported
			}
			c.pending = c.pending[:0]
		}
	}
	for len(remaining) > 0 {
		if !utf8.FullRune(remaining) {
			c.pending = append(c.pending, remaining...)
			break
		}
		r, size := utf8.DecodeRune(remaining)
		if !acceptedRune(r, size) {
			c.valid = false
			return 0, ErrUnsupported
		}
		remaining = remaining[size:]
	}
	return len(data), nil
}

func (c *plainTextCheck) finish() error {
	if !c.valid || len(c.pending) > 0 {
		return ErrUnsupported
	}
	return nil
}

func acceptedRune(r rune, size int) bool {
	if r == utf8.RuneError && size <= 1 {
		return false
	}
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return !unicode.IsControl(r)
}

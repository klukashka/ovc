package core

import (
	"fmt"
	"io"
	"log/slog"
)

const redacted = "[redacted]"

type Secret struct {
	reveal func() string
}

func NewSecret(value string) Secret {
	return Secret{reveal: func() string { return value }}
}

func (s Secret) Reveal() string {
	if s.reveal == nil {
		return ""
	}
	return s.reveal()
}

func (s Secret) IsZero() bool {
	return s.Reveal() == ""
}

func (Secret) String() string {
	return redacted
}

func (Secret) GoString() string {
	return redacted
}

func (Secret) Format(state fmt.State, _ rune) {
	writeRedacted(state)
}

func writeRedacted(state fmt.State) {
	if _, err := io.WriteString(state, redacted); err != nil {
		panic("the formatter state refused the redaction marker: " + ErrorKind(err))
	}
}

func (Secret) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

func (Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

func (Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}

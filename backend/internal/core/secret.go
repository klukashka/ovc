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
	_, err := io.WriteString(state, redacted)
	if err != nil {
		return
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

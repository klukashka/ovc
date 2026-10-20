package core

import (
	"fmt"
	"io"
	"log/slog"
)

const redacted = "[redacted]"

type Secret struct {
	value string
}

func NewSecret(value string) Secret {
	return Secret{value: value}
}

func (s Secret) Reveal() string {
	return s.value
}

func (s Secret) IsZero() bool {
	return s.value == ""
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

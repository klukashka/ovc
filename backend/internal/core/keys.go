package core

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"strconv"
)

const derivedKeyBytes = 32

type Keys struct {
	access   func() []byte
	throttle func() []byte
	refresh  func() []byte
	download func() []byte
}

func DeriveKeys(secret Secret) (Keys, error) {
	derive := func(info string) (func() []byte, error) {
		key, err := hkdf.Key(sha256.New, []byte(secret.Reveal()), nil, info, derivedKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("derive the %s key: %w", info, err)
		}
		return func() []byte { return key }, nil
	}
	access, err := derive("access-token")
	if err != nil {
		return Keys{}, err
	}
	throttle, err := derive("throttle")
	if err != nil {
		return Keys{}, err
	}
	refresh, err := derive("refresh-generation")
	if err != nil {
		return Keys{}, err
	}
	download, err := derive("download-url")
	if err != nil {
		return Keys{}, err
	}
	return Keys{access: access, throttle: throttle, refresh: refresh, download: download}, nil
}

func (k Keys) ThrottleSubject(purpose string, value string) []byte {
	mac := hmac.New(sha256.New, k.throttle())
	mac.Write([]byte(purpose))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (k Keys) RefreshMAC(session [16]byte, generation int64) []byte {
	mac := hmac.New(sha256.New, k.refresh())
	mac.Write(session[:])
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.FormatInt(generation, 10)))
	return mac.Sum(nil)
}

func (k Keys) DownloadMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, k.download())
	mac.Write(payload)
	return mac.Sum(nil)
}

func (Keys) String() string {
	return redacted
}

func (Keys) GoString() string {
	return redacted
}

func (Keys) Format(state fmt.State, _ rune) {
	if _, err := io.WriteString(state, redacted); err != nil {
		return
	}
}

func (Keys) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

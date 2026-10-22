package core

import (
	"crypto/rand"
	"fmt"
	"io"

	"ovc/backend/internal/domain"
)

type RandomSource io.Reader

func CryptoRandom() RandomSource {
	return rand.Reader
}

func NewID[ID ~[16]byte](random RandomSource) (ID, error) {
	var id ID
	if _, err := io.ReadFull(random, id[:]); err != nil {
		return id, fmt.Errorf("read random identifier: %w", err)
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return id, nil
}

func RandomFraction(random RandomSource) (float64, error) {
	var buffer [8]byte
	if _, err := io.ReadFull(random, buffer[:]); err != nil {
		return 0, fmt.Errorf("read random fraction: %w", err)
	}
	var value uint64
	for _, part := range buffer {
		value = value<<8 | uint64(part)
	}
	return float64(value>>11) / float64(uint64(1)<<53), nil
}

func NewTutorCode(random RandomSource) (domain.TutorCode, error) {
	alphabet := domain.TutorCodeAlphabet
	unbiasedBelow := 256 - 256%len(alphabet)
	code := make([]byte, 0, domain.TutorCodeLength)
	var drawn [1]byte
	for len(code) < domain.TutorCodeLength {
		if _, err := io.ReadFull(random, drawn[:]); err != nil {
			return "", fmt.Errorf("read random tutor code: %w", err)
		}
		if int(drawn[0]) < unbiasedBelow {
			code = append(code, alphabet[int(drawn[0])%len(alphabet)])
		}
	}
	return domain.TutorCode(code), nil
}

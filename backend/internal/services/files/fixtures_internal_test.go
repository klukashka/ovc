package files

import (
	"math"
	"testing"
)

func size32(t *testing.T, value int) uint32 {
	t.Helper()
	if value < 0 || value > math.MaxUint32 {
		t.Fatalf("a fixture length %d does not fit 32 bits", value)
		return 0
	}
	return uint32(value)
}

func size8(t *testing.T, value int) byte {
	t.Helper()
	if value < 0 || value > math.MaxUint8 {
		t.Fatalf("a fixture length %d does not fit a byte", value)
		return 0
	}
	return byte(value)
}

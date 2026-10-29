package files

import "math"

func unsigned(value int64) uint64 {
	if value < 0 {
		panic("a negative offset or instant reached an unsigned field")
	}
	return uint64(value)
}

func signed(value uint64) int64 {
	if value > math.MaxInt64 {
		panic("an unchecked unsigned value reached a signed field")
	}
	return int64(value)
}

func uint16Of(value int) (uint16, bool) {
	if value < 0 || value > math.MaxUint16 {
		return 0, false
	}
	return uint16(value), true
}

func int32Of(value int) (int32, bool) {
	if value < 0 || value > math.MaxInt32 {
		return 0, false
	}
	return int32(value), true
}

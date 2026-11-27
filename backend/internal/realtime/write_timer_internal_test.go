package realtime

import "testing"

type staleWriteTimer struct {
	name     string
	closed   bool
	writing  bool
	writeSeq uint64
	firedFor uint64
}

func TestAStaleWriteTimerLeavesTheSocketUntouched(t *testing.T) {
	cases := []staleWriteTimer{
		{name: "its write completed", writing: false, writeSeq: 1, firedFor: 1},
		{name: "a later write is in flight", writing: true, writeSeq: 2, firedFor: 1},
		{name: "the socket closed for another reason", closed: true, writing: true, writeSeq: 1, firedFor: 1},
	}
	for _, timer := range cases {
		t.Run(timer.name, func(t *testing.T) {
			s := &socket{g: &Gateway{}, closed: timer.closed, writing: timer.writing, writeSeq: timer.writeSeq}

			dropped := s.dropTimedOutWrite(timer.firedFor)

			if dropped || s.code != 0 || s.reason != "" {
				t.Fatalf("a stale timer dropped the socket: %t %d %q", dropped, s.code, s.reason)
			}
		})
	}
}

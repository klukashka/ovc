package realtime

import (
	"bytes"
	"testing"
)

func filledQueue(frames int, size int) (*outbound, bool) {
	queue := &outbound{}
	accepted := true
	for range frames {
		accepted = queue.push(bytes.Repeat([]byte{'x'}, size)) && accepted
	}
	return queue, accepted
}

func TestOutboundQueueRefusesBeyond256FramesOr1MiB(t *testing.T) {
	t.Parallel()
	byCount, countAccepted := filledQueue(maxQueuedFrames, 10)
	byBytes, bytesAccepted := filledQueue(4, maxQueuedBytes/4)

	extraFrame := byCount.push([]byte{'y'})
	extraByte := byBytes.push([]byte{'y'})
	first, _ := byCount.pop()
	afterPop := byCount.push([]byte{'z'})

	if !countAccepted || !bytesAccepted {
		t.Fatal("a queue refused a frame inside its bounds")
	}
	if extraFrame || extraByte {
		t.Fatalf("the 257th frame was accepted: %t; the byte beyond 1 MiB was accepted: %t", extraFrame, extraByte)
	}
	if len(first) != 10 || !afterPop {
		t.Fatal("a popped frame did not free its slot")
	}
}

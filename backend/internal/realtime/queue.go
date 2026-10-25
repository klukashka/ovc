package realtime

const (
	maxQueuedFrames = 256
	maxQueuedBytes  = 1 << 20
)

type outbound struct {
	frames [][]byte
	bytes  int
}

func (q *outbound) push(frame []byte) bool {
	if len(q.frames)+1 > maxQueuedFrames || q.bytes+len(frame) > maxQueuedBytes {
		return false
	}
	q.frames = append(q.frames, frame)
	q.bytes += len(frame)
	return true
}

func (q *outbound) pop() ([]byte, bool) {
	if len(q.frames) == 0 {
		return nil, false
	}
	frame := q.frames[0]
	q.frames[0] = nil
	q.frames = q.frames[1:]
	q.bytes -= len(frame)
	return frame, true
}

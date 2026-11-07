package core

import "sync/atomic"

type Switch struct {
	on atomic.Bool
}

func (s *Switch) On() bool {
	return s != nil && s.on.Load()
}

func (s *Switch) Set(on bool) {
	s.on.Store(on)
}

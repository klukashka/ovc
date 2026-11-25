package core

import "time"

type Timer interface {
	Stop() bool
}

type Ticker interface {
	Ticks() <-chan time.Time
	Stop()
}

type Scheduler interface {
	AfterFunc(after time.Duration, run func()) Timer
	NewTicker(every time.Duration) Ticker
}

type SystemScheduler struct{}

func (SystemScheduler) AfterFunc(after time.Duration, run func()) Timer {
	return time.AfterFunc(after, run)
}

func (SystemScheduler) NewTicker(every time.Duration) Ticker {
	return systemTicker{ticker: time.NewTicker(every)}
}

type systemTicker struct {
	ticker *time.Ticker
}

func (t systemTicker) Ticks() <-chan time.Time {
	return t.ticker.C
}

func (t systemTicker) Stop() {
	t.ticker.Stop()
}

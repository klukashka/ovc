package listener

import (
	"syscall"
	"time"
)

func socketControl(time.Duration) func(string, string, syscall.RawConn) error {
	return nil
}

package listener

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func socketControl(userTimeout time.Duration) func(string, string, syscall.RawConn) error {
	milliseconds := int(userTimeout.Milliseconds())
	return func(network string, _ string, raw syscall.RawConn) error {
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil
		}
		var optionErr error
		if err := raw.Control(func(descriptor uintptr) {
			optionErr = unix.SetsockoptInt(int(descriptor), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, milliseconds)
		}); err != nil {
			return fmt.Errorf("reach the listener socket: %w", err)
		}
		if optionErr != nil {
			return fmt.Errorf("set tcp_user_timeout on the listener socket: %w", optionErr)
		}
		return nil
	}
}

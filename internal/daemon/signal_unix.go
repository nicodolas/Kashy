//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

// sendStop sends SIGTERM on Unix/macOS.
func sendStop(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

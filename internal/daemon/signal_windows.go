//go:build windows

package daemon

import (
	"os"
)

// sendStop on Windows: os.Kill is the only reliable cross-process signal.
// SIGTERM is not supported for external processes on Windows.
func sendStop(proc *os.Process) error {
	return proc.Kill()
}

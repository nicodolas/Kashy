// Package daemon manages a PID file for the kashy start process,
// enabling kashy stop to find and terminate the running daemon.
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultPIDPath returns the path to the kashy pidfile (~/.kashy/kashy.pid).
func DefaultPIDPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kashy", "kashy.pid")
}

// pidPath returns the pidfile path inside the given directory (used in tests).
func pidPath(dir string) string {
	return filepath.Join(dir, "kashy.pid")
}

// WriteSelf writes the current process PID to the default pidfile.
func WriteSelf() error {
	return writePID(DefaultPIDPath(), os.Getpid())
}

// writePID writes pid to path (creates directories as needed).
func writePID(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("daemon: mkdir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// readPID reads and parses the pid from path.
func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("daemon: read pidfile: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("daemon: invalid pidfile content: %w", err)
	}
	return pid, nil
}

// removePID deletes the pidfile, ignoring errors (best effort cleanup).
func removePID(path string) {
	_ = os.Remove(path)
}

// RemoveSelf deletes the default pidfile (called on clean shutdown).
func RemoveSelf() {
	removePID(DefaultPIDPath())
}

// StopDaemon reads the default pidfile and sends SIGTERM to the process.
// Returns an error if the pidfile is missing or the process cannot be signalled.
func StopDaemon() error {
	path := DefaultPIDPath()
	pid, err := readPID(path)
	if err != nil {
		return fmt.Errorf("kashy is not running (no pidfile found at %s)", path)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("daemon: find process %d: %w", pid, err)
	}

	if err := sendStop(proc); err != nil {
		return fmt.Errorf("daemon: signal process %d: %w", pid, err)
	}

	removePID(path)
	return nil
}

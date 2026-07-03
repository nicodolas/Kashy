package daemon

import (
	"os"
	"strconv"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	dir := t.TempDir()
	path := pidPath(dir)

	if err := writePID(path, 12345); err != nil {
		t.Fatalf("writePID: %v", err)
	}

	pid, err := readPID(path)
	if err != nil {
		t.Fatalf("readPID: %v", err)
	}
	if pid != 12345 {
		t.Errorf("got pid %d, want 12345", pid)
	}
}

func TestReadPIDNoFile(t *testing.T) {
	dir := t.TempDir()
	path := pidPath(dir)

	_, err := readPID(path)
	if err == nil {
		t.Error("expected error reading non-existent pidfile")
	}
}

func TestReadPIDInvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := pidPath(dir)

	os.WriteFile(path, []byte("not-a-number\n"), 0644)
	_, err := readPID(path)
	if err == nil {
		t.Error("expected error for non-numeric pidfile content")
	}
}

func TestRemovePID(t *testing.T) {
	dir := t.TempDir()
	path := pidPath(dir)

	os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
	removePID(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("pidfile should be removed after removePID")
	}
}

func TestDefaultPIDPath(t *testing.T) {
	p := DefaultPIDPath()
	if p == "" {
		t.Error("DefaultPIDPath() should not be empty")
	}
}

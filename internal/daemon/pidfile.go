package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePID writes the current process PID to path, creating or overwriting it.
func WritePID(path string) error {
	data := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("WritePID: %w", err)
	}
	return nil
}

// ReadPID reads and returns the PID stored in path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("ReadPID: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("ReadPID: malformed PID in %s: %w", path, err)
	}
	return pid, nil
}

// RemovePID removes the PID file at path. It tolerates the file not existing.
func RemovePID(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("RemovePID: %w", err)
	}
	return nil
}

// IsRunning returns true if path contains a PID for a currently running process.
func IsRunning(path string) bool {
	pid, err := ReadPID(path)
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// signal 0 probes existence without sending a real signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

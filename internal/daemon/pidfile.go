package daemon

import (
	"fmt"
	"os"
	"strconv"
)

// WritePID writes the current process PID to path, creating or overwriting it.
func WritePID(path string) error {
	data := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("WritePID: %w", err)
	}
	return nil
}

// RemovePID removes the PID file at path. It tolerates the file not existing.
func RemovePID(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("RemovePID: %w", err)
	}
	return nil
}

package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Lock represents a file-based lock that prevents concurrent dotfather runs.
type Lock struct {
	path string
	file *os.File
}

// Acquire creates an exclusive lock file in the given directory.
// Returns an error if another instance already holds the lock.
func Acquire(dir string) (*Lock, error) {
	lockPath := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			if removeStaleLock(lockPath) {
				f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return nil, fmt.Errorf("acquire lock after stale removal: %w", err)
				}
			} else {
				return nil, fmt.Errorf("dotfather is already running (lock: %s); if not, remove the lock file", lockPath)
			}
		} else {
			return nil, fmt.Errorf("acquire lock: %w", err)
		}
	}
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	return &Lock{path: lockPath, file: f}, nil
}

// Release removes the lock file.
func (l *Lock) Release() error {
	_ = l.file.Close()
	return os.Remove(l.path)
}

// removeStaleLock checks if the lock file references a dead process and removes it if so.
func removeStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return false // process is alive
	}
	// EPERM means the process exists but we lack permission to signal it.
	if errors.Is(err, syscall.EPERM) {
		return false
	}
	return os.Remove(lockPath) == nil
}

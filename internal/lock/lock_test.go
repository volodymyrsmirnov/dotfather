package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()

	lk, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}

	// Lock file should exist.
	if _, err := os.Stat(filepath.Join(dir, ".lock")); err != nil {
		t.Error("lock file should exist")
	}

	if err := lk.Release(); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	// Lock file should be gone.
	if _, err := os.Stat(filepath.Join(dir, ".lock")); !os.IsNotExist(err) {
		t.Error("lock file should not exist after release")
	}
}

func TestAcquireTwiceFails(t *testing.T) {
	dir := t.TempDir()

	lk, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	defer lk.Release()

	_, err = Acquire(dir)
	if err == nil {
		t.Error("second Acquire() should fail")
	}
}

func TestAcquire_RemovesStaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	// Write a lock file with a PID that does not exist.
	if err := os.WriteFile(lockPath, []byte("99999999\n"), 0600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	lk, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire() should succeed after stale lock removal: %v", err)
	}
	defer lk.Release()
}

func TestAcquire_DoesNotRemoveLiveLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	// Write a lock file with the current process PID (definitely alive).
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	_, err := Acquire(dir)
	if err == nil {
		t.Error("Acquire() should fail when lock is held by a live process")
	}

	// Clean up manually since we didn't use Acquire to create it.
	os.Remove(lockPath)
}

func TestAcquireAfterRelease(t *testing.T) {
	dir := t.TempDir()

	lk, err := Acquire(dir)
	if err != nil {
		t.Fatalf("first Acquire() error: %v", err)
	}
	if err := lk.Release(); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	lk2, err := Acquire(dir)
	if err != nil {
		t.Fatalf("re-Acquire() error: %v", err)
	}
	defer lk2.Release()
}

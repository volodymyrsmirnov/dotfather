package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// AtomicWriteFile writes to path atomically by first writing to a temporary
// file in the same directory, then renaming it into place. If fn returns an
// error the temp file is removed and the original is untouched.
func AtomicWriteFile(path string, perm os.FileMode, fn func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	tmp := path + ".dotfather-tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if err := fn(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// SafeWriteTarget is like AtomicWriteFile but also rejects symlinks at
// the destination path. If the destination is a symlink it is removed
// before writing so that decrypted content never leaks through a symlink.
func SafeWriteTarget(path string, perm os.FileMode, fn func(w io.Writer) error) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove symlink at target: %w", err)
			}
		}
	}
	return AtomicWriteFile(path, perm, fn)
}

// UniqueBackupPath returns path+suffix if that path does not exist,
// otherwise appends a timestamp to avoid overwriting an existing backup.
func UniqueBackupPath(path, suffix string) string {
	base := path + suffix
	if _, err := os.Lstat(base); os.IsNotExist(err) {
		return base
	}
	return fmt.Sprintf("%s.%s", base, time.Now().Format("20060102-150405"))
}

// FileHash returns the hex-encoded SHA-256 hash of a file's contents.
func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

package linker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LinkState describes the state of a symlink.
type LinkState int

const (
	OK       LinkState = iota // Symlink exists and points to repo file.
	Broken                    // Symlink exists but target is missing.
	Missing                   // No symlink and no file at target.
	Unlinked                  // Regular file exists at target (not a symlink to repo).
	Conflict                  // Symlink exists but points somewhere else.
)

func (s LinkState) String() string {
	switch s {
	case OK:
		return "OK"
	case Broken:
		return "BROKEN"
	case Missing:
		return "MISSING"
	case Unlinked:
		return "UNLINKED"
	case Conflict:
		return "CONFLICT"
	default:
		return "UNKNOWN"
	}
}

// LinkStatus holds the state of a managed file's symlink.
type LinkStatus struct {
	RepoPath   string    // Absolute path in the repo.
	TargetPath string    // Absolute path where the symlink should be (in ~/).
	RelPath    string    // Path relative to home (for display).
	State      LinkState // Current state.
}

// Link creates a symlink from targetPath to repoFile.
// It creates parent directories of targetPath as needed.
func Link(repoFile, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.Symlink(repoFile, targetPath); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

// Unlink removes a symlink at targetPath if it points to repoFile.
func Unlink(targetPath, repoFile string) error {
	linkTarget, err := os.Readlink(targetPath)
	if err != nil {
		return fmt.Errorf("read symlink: %w", err)
	}

	if linkTarget != repoFile {
		return fmt.Errorf("%s is not a dotfather symlink (points to %s)", targetPath, linkTarget)
	}

	return os.Remove(targetPath)
}

// Check examines the symlink state for a managed file.
func Check(repoFile, targetPath string) LinkState {
	info, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Missing
		}
		return Missing
	}

	// Target exists. Is it a symlink?
	if info.Mode()&os.ModeSymlink == 0 {
		return Unlinked
	}

	// It's a symlink. Where does it point?
	linkTarget, err := os.Readlink(targetPath)
	if err != nil {
		return Broken
	}

	if linkTarget == repoFile {
		// Verify the repo file actually exists.
		if _, err := os.Stat(repoFile); err != nil {
			return Broken
		}
		return OK
	}

	return Conflict
}

// IsOurSymlink returns true if targetPath is a symlink pointing to repoFile.
func IsOurSymlink(targetPath, repoFile string) bool {
	return Check(repoFile, targetPath) == OK
}

// MoveFile moves src to dst, falling back to copy+remove for cross-device moves.
func MoveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback: copy + remove (cross-device).
	if err := CopyFile(src, dst); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return os.Remove(src)
}

// CopyFile copies src to dst, preserving permissions.
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(dstFile, srcFile)
	closeErr := dstFile.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// CleanEmptyDirs removes empty parent directories from path up to (but not including) root.
// Returns any errors encountered (non-fatal, directories may be in use).
func CleanEmptyDirs(path, root string) error {
	dir := filepath.Dir(path)
	for dir != root && dir != filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("remove empty dir %s: %w", dir, err)
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

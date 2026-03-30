package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HomeDir returns the current user's home directory.
func HomeDir() (string, error) {
	return os.UserHomeDir()
}

// ExpandPath expands ~ to the home directory and returns an absolute, clean path.
func ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := HomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		p = filepath.Join(home, p[1:])
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	return filepath.Clean(abs), nil
}

// NormalizePath resolves a path to its absolute, cleaned form.
// It expands ~, resolves symlinks in parent directories (but not the final component),
// and returns the result.
func NormalizePath(p string) (string, error) {
	expanded, err := ExpandPath(p)
	if err != nil {
		return "", err
	}

	// Resolve symlinks in parent dir (not the file itself, which might be our symlink).
	dir := filepath.Dir(expanded)
	base := filepath.Base(expanded)

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// Parent doesn't exist yet — return the expanded path as-is.
		return expanded, nil
	}

	return filepath.Join(resolvedDir, base), nil
}

// RelToHome returns the path relative to the home directory.
func RelToHome(absPath string) (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Rel(home, absPath)
}

// IsUnderHome checks whether the given absolute path is under the home directory.
func IsUnderHome(absPath string) (bool, error) {
	home, err := HomeDir()
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(home, absPath)
	if err != nil {
		return false, err
	}

	return !strings.HasPrefix(rel, ".."), nil
}

// TildePath returns a path with ~ prefix for display (e.g., ~/.config/foo).
func TildePath(absPath string) string {
	home, err := HomeDir()
	if err != nil {
		return absPath
	}

	rel, err := filepath.Rel(home, absPath)
	if err != nil {
		return absPath
	}

	if strings.HasPrefix(rel, "..") {
		return absPath
	}

	return filepath.Join("~", rel)
}

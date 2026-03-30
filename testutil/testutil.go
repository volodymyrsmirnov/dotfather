package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// SetupTestHome creates a temporary directory to use as $HOME and $DOTFATHER_DIR.
// Returns the home dir path (symlink-resolved for macOS compatibility).
func SetupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	// Resolve symlinks (on macOS, /var -> /private/var).
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("resolve home path: %v", err)
	}
	t.Setenv("HOME", resolved)
	t.Setenv("DOTFATHER_DIR", filepath.Join(resolved, ".dotfather"))
	return resolved
}

// CreateFile creates a file with the given content inside the base directory.
func CreateFile(t *testing.T, base, relPath, content string) string {
	t.Helper()
	absPath := filepath.Join(base, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("create parent dirs: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return absPath
}

// InitGitRepo initializes a git repo with an initial commit.
func InitGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	// Create an initial commit so the repo has a branch.
	CreateFile(t, dir, ".gitkeep", "")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
}

// GitCommitAll stages and commits all changes.
func GitCommitAll(t *testing.T, dir, message string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", message)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/volodymyrsmirnov/dotfather/testutil"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func TestNew_DefaultPath(t *testing.T) {
	home := testutil.SetupTestHome(t)

	// Unset DOTFATHER_DIR to use default.
	t.Setenv("DOTFATHER_DIR", "")

	r, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	want := filepath.Join(home, ".dotfather")
	if r.Path() != want {
		t.Errorf("Path() = %q, want %q", r.Path(), want)
	}
}

func TestNew_CustomDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	customDir := filepath.Join(home, "custom_dotfiles")
	t.Setenv("DOTFATHER_DIR", customDir)

	r, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if r.Path() != customDir {
		t.Errorf("Path() = %q, want %q", r.Path(), customDir)
	}
}

func TestExists(t *testing.T) {
	home := testutil.SetupTestHome(t)

	r, _ := New()

	// Doesn't exist yet.
	if r.Exists() {
		t.Error("Exists() should be false for non-existent dir")
	}

	// Create it.
	mustMkdir(t, filepath.Join(home, ".dotfather"))

	if !r.Exists() {
		t.Error("Exists() should be true after creating dir")
	}
}

func TestIsGitRepo(t *testing.T) {
	home := testutil.SetupTestHome(t)

	r, _ := New()

	// Not a git repo.
	repoDir := filepath.Join(home, ".dotfather")
	mustMkdir(t, repoDir)
	if r.IsGitRepo() {
		t.Error("IsGitRepo() should be false for non-git dir")
	}

	// Initialize as git repo.
	testutil.InitGitRepo(t, repoDir)
	if !r.IsGitRepo() {
		t.Error("IsGitRepo() should be true after git init")
	}
}

func TestEnsureExists(t *testing.T) {
	home := testutil.SetupTestHome(t)

	r, _ := New()

	// Doesn't exist.
	if err := r.EnsureExists(); err == nil {
		t.Error("EnsureExists() should error when repo doesn't exist")
	}

	// Exists but not git repo.
	mustMkdir(t, filepath.Join(home, ".dotfather"))
	if err := r.EnsureExists(); err == nil {
		t.Error("EnsureExists() should error when dir is not a git repo")
	}

	// Valid git repo.
	testutil.InitGitRepo(t, filepath.Join(home, ".dotfather"))
	if err := r.EnsureExists(); err != nil {
		t.Errorf("EnsureExists() unexpected error: %v", err)
	}
}

func TestManagedFiles(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	r, _ := New()

	// Add some files.
	testutil.CreateFile(t, repoDir, ".bashrc", "# bash")
	testutil.CreateFile(t, repoDir, ".config/nvim/init.lua", "-- nvim")
	testutil.CreateFile(t, repoDir, ".zshrc", "# zsh")

	files, err := r.ManagedFiles()
	if err != nil {
		t.Fatalf("ManagedFiles() error: %v", err)
	}

	// Should NOT include .gitkeep from InitGitRepo, but SHOULD include our files.
	// Files are sorted alphabetically.
	expected := []string{".bashrc", ".config/nvim/init.lua", ".gitkeep", ".zshrc"}
	if len(files) != len(expected) {
		t.Fatalf("ManagedFiles() returned %d files, want %d: %v", len(files), len(expected), files)
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %q, want %q", i, f, expected[i])
		}
	}
}

func TestManagedFiles_ExcludesGitDir(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	r, _ := New()

	files, err := r.ManagedFiles()
	if err != nil {
		t.Fatalf("ManagedFiles() error: %v", err)
	}

	for _, f := range files {
		if strings.HasPrefix(f, ".git/") || f == ".git" {
			t.Errorf("ManagedFiles() should exclude .git, found: %s", f)
		}
	}
}

func TestRepoPathFor(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	mustMkdir(t, repoDir)

	r, _ := New()

	got, err := r.RepoPathFor(filepath.Join(home, ".config", "foo.yaml"))
	if err != nil {
		t.Fatalf("RepoPathFor() error: %v", err)
	}

	want := filepath.Join(repoDir, ".config", "foo.yaml")
	if got != want {
		t.Errorf("RepoPathFor() = %q, want %q", got, want)
	}
}

func TestTargetPathFor(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	mustMkdir(t, repoDir)

	r, _ := New()

	got, err := r.TargetPathFor(".config/foo.yaml")
	if err != nil {
		t.Fatalf("TargetPathFor() error: %v", err)
	}

	want := filepath.Join(home, ".config", "foo.yaml")
	if got != want {
		t.Errorf("TargetPathFor() = %q, want %q", got, want)
	}
}

func TestIsManaged(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	r, _ := New()

	// File not in repo.
	managed, err := r.IsManaged(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("IsManaged() error: %v", err)
	}
	if managed {
		t.Error("IsManaged() should be false for unmanaged file")
	}

	// Add file to repo.
	testutil.CreateFile(t, repoDir, ".bashrc", "# bash")

	managed, err = r.IsManaged(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("IsManaged() error: %v", err)
	}
	if !managed {
		t.Error("IsManaged() should be true for managed file")
	}
}

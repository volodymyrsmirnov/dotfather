package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "test@test.com")
	runTestGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runTestGit(t, dir, "add", ".")
	runTestGit(t, dir, "commit", "-m", "initial")
	return dir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(context.Background(), dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Verify .git exists.
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		t.Error(".git directory should exist")
	}
}

func TestIsGitRepo(t *testing.T) {
	dir := t.TempDir()

	if IsGitRepo(context.Background(), dir) {
		t.Error("IsGitRepo() should be false for non-git dir")
	}

	if err := Init(context.Background(), dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if !IsGitRepo(context.Background(), dir) {
		t.Error("IsGitRepo() should be true after init")
	}
}

func TestAddAndCommit(t *testing.T) {
	dir := initTestRepo(t)

	// Create a new file.
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Add(context.Background(), dir, "test.txt"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	if err := Commit(context.Background(), dir, "add test file"); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	// Verify the commit exists.
	out, err := run(context.Background(), dir, "log", "--oneline", "-1")
	if err != nil {
		t.Fatalf("git log error: %v", err)
	}
	if out == "" {
		t.Error("expected a commit in the log")
	}
}

func TestAddAll(t *testing.T) {
	dir := initTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := AddAll(context.Background(), dir); err != nil {
		t.Fatalf("AddAll() error: %v", err)
	}

	if err := Commit(context.Background(), dir, "add all"); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}
}

func TestStatus(t *testing.T) {
	dir := initTestRepo(t)

	// Clean repo.
	status, err := Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status != "" {
		t.Errorf("Status() on clean repo should be empty, got %q", status)
	}

	// Add untracked file.
	writeTestFile(t, filepath.Join(dir, "new.txt"), "new")

	status, err = Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status == "" {
		t.Error("Status() should show untracked file")
	}
}

func TestHasUncommitted(t *testing.T) {
	dir := initTestRepo(t)

	has, err := HasUncommitted(context.Background(), dir)
	if err != nil {
		t.Fatalf("HasUncommitted() error: %v", err)
	}
	if has {
		t.Error("HasUncommitted() should be false on clean repo")
	}

	writeTestFile(t, filepath.Join(dir, "new.txt"), "new")

	has, err = HasUncommitted(context.Background(), dir)
	if err != nil {
		t.Fatalf("HasUncommitted() error: %v", err)
	}
	if !has {
		t.Error("HasUncommitted() should be true with untracked file")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initTestRepo(t)

	branch, err := CurrentBranch(context.Background(), dir)
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}

	// Should be main or master depending on git config.
	if branch == "" {
		t.Error("CurrentBranch() returned empty string")
	}
}

func TestHasRemote(t *testing.T) {
	dir := initTestRepo(t)

	if HasRemote(context.Background(), dir) {
		t.Error("HasRemote() should be false with no remote")
	}

	runTestGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")

	if !HasRemote(context.Background(), dir) {
		t.Error("HasRemote() should be true after adding remote")
	}
}

func TestRemoteAdd(t *testing.T) {
	dir := initTestRepo(t)

	if err := RemoteAdd(context.Background(), dir, "origin", "https://example.com/repo.git"); err != nil {
		t.Fatalf("RemoteAdd() error: %v", err)
	}

	if !HasRemote(context.Background(), dir) {
		t.Error("remote should exist after RemoteAdd")
	}
}

func TestDiff(t *testing.T) {
	dir := initTestRepo(t)

	// No diff on clean repo.
	diff, err := Diff(context.Background(), dir)
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff() on clean repo should be empty, got %q", diff)
	}

	// Modify tracked file.
	writeTestFile(t, filepath.Join(dir, ".gitkeep"), "modified")

	diff, err = Diff(context.Background(), dir)
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff == "" {
		t.Error("Diff() should show changes")
	}
}

func TestDiffCached(t *testing.T) {
	dir := initTestRepo(t)

	writeTestFile(t, filepath.Join(dir, "new.txt"), "new")
	runTestGit(t, dir, "add", "new.txt")

	diff, err := DiffCached(context.Background(), dir)
	if err != nil {
		t.Fatalf("DiffCached() error: %v", err)
	}
	if diff == "" {
		t.Error("DiffCached() should show staged changes")
	}
}

func TestGitError(t *testing.T) {
	dir := t.TempDir()

	// Running git status in non-git dir should return GitError.
	_, err := Status(context.Background(), dir)
	if err == nil {
		t.Fatal("Status() should error in non-git dir")
	}

	gitErr, ok := err.(*GitError)
	if !ok {
		t.Fatalf("expected *GitError, got %T", err)
	}
	if gitErr.Command != "status" {
		t.Errorf("GitError.Command = %q, want %q", gitErr.Command, "status")
	}
	if gitErr.ExitCode == 0 {
		t.Error("GitError.ExitCode should be non-zero")
	}
}

func TestClone(t *testing.T) {
	// Create a source repo.
	src := initTestRepo(t)
	writeTestFile(t, filepath.Join(src, "dotfile"), "content")
	runTestGit(t, src, "add", ".")
	runTestGit(t, src, "commit", "-m", "add dotfile")

	// Clone it.
	dst := filepath.Join(t.TempDir(), "cloned")
	if err := Clone(context.Background(), src, dst); err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Verify the file exists in clone.
	content, err := os.ReadFile(filepath.Join(dst, "dotfile"))
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("cloned file content = %q, want %q", content, "content")
	}
}

func TestClone_BadURL(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "bad_clone")
	err := Clone(context.Background(), "https://nonexistent.invalid/repo.git", dst)
	if err == nil {
		t.Fatal("Clone() should error with bad URL")
	}
}

func TestCommit_NoSigning(t *testing.T) {
	dir := initTestRepo(t)

	// Enable commit signing — Commit() should still succeed because it
	// passes -c commit.gpgsign=false.
	runTestGit(t, dir, "config", "commit.gpgsign", "true")

	writeTestFile(t, filepath.Join(dir, "signed.txt"), "data")
	if err := Add(context.Background(), dir, "signed.txt"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	if err := Commit(context.Background(), dir, "should not require signing"); err != nil {
		t.Fatalf("Commit() error with gpgsign=true: %v", err)
	}
}

func TestGitAvailable(t *testing.T) {
	// Git should be available in test environment.
	if !GitAvailable() {
		t.Error("GitAvailable() should be true")
	}
}

package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func setupFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func setupSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}

func TestLink(t *testing.T) {
	dir := t.TempDir()

	repoFile := filepath.Join(dir, "repo", ".bashrc")
	targetPath := filepath.Join(dir, "home", ".bashrc")

	// Create the repo file.
	setupFile(t, repoFile, "# bashrc")

	// Link.
	if err := Link(repoFile, targetPath); err != nil {
		t.Fatalf("Link() error: %v", err)
	}

	// Verify it's a symlink pointing to repo file.
	linkTarget, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Readlink() error: %v", err)
	}
	if linkTarget != repoFile {
		t.Errorf("symlink target = %q, want %q", linkTarget, repoFile)
	}

	// Verify content is accessible through symlink.
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(content) != "# bashrc" {
		t.Errorf("content = %q, want %q", content, "# bashrc")
	}
}

func TestLink_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()

	repoFile := filepath.Join(dir, "repo", "file")
	setupFile(t, repoFile, "data")

	targetPath := filepath.Join(dir, "home", "deep", "nested", "file")

	if err := Link(repoFile, targetPath); err != nil {
		t.Fatalf("Link() error: %v", err)
	}

	// Verify parent dirs were created.
	info, err := os.Stat(filepath.Dir(targetPath))
	if err != nil {
		t.Fatalf("parent dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("parent should be a directory")
	}
}

func TestUnlink(t *testing.T) {
	dir := t.TempDir()

	repoFile := filepath.Join(dir, "repo", ".bashrc")
	targetPath := filepath.Join(dir, "home", ".bashrc")

	setupFile(t, repoFile, "content")
	setupSymlink(t, repoFile, targetPath)

	// Unlink.
	if err := Unlink(targetPath, repoFile); err != nil {
		t.Fatalf("Unlink() error: %v", err)
	}

	// Verify symlink is gone.
	_, err := os.Lstat(targetPath)
	if !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
}

func TestUnlink_WrongTarget(t *testing.T) {
	dir := t.TempDir()

	wrongTarget := filepath.Join(dir, "wrong")
	setupFile(t, wrongTarget, "wrong")

	targetPath := filepath.Join(dir, "link")
	setupSymlink(t, wrongTarget, targetPath)

	err := Unlink(targetPath, "/expected/repo/file")
	if err == nil {
		t.Fatal("Unlink() should error when symlink points elsewhere")
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()

	repoFile := filepath.Join(dir, "repo", "file")
	setupFile(t, repoFile, "content")

	targetDir := filepath.Join(dir, "home")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Run("OK", func(t *testing.T) {
		target := filepath.Join(targetDir, "ok_link")
		setupSymlink(t, repoFile, target)

		if got := Check(repoFile, target); got != OK {
			t.Errorf("Check() = %v, want OK", got)
		}
	})

	t.Run("Missing", func(t *testing.T) {
		target := filepath.Join(targetDir, "nonexistent")

		if got := Check(repoFile, target); got != Missing {
			t.Errorf("Check() = %v, want Missing", got)
		}
	})

	t.Run("Unlinked", func(t *testing.T) {
		target := filepath.Join(targetDir, "regular_file")
		setupFile(t, target, "not a symlink")

		if got := Check(repoFile, target); got != Unlinked {
			t.Errorf("Check() = %v, want Unlinked", got)
		}
	})

	t.Run("Conflict", func(t *testing.T) {
		otherFile := filepath.Join(dir, "other")
		setupFile(t, otherFile, "other")

		target := filepath.Join(targetDir, "conflict_link")
		setupSymlink(t, otherFile, target)

		if got := Check(repoFile, target); got != Conflict {
			t.Errorf("Check() = %v, want Conflict", got)
		}
	})

	t.Run("Broken", func(t *testing.T) {
		// Symlink to correct path, but the repo file doesn't exist at that path.
		nonexistentRepo := filepath.Join(dir, "repo", "gone")
		target := filepath.Join(targetDir, "broken_link")
		setupSymlink(t, nonexistentRepo, target)

		if got := Check(nonexistentRepo, target); got != Broken {
			t.Errorf("Check() = %v, want Broken", got)
		}
	})
}

func TestIsOurSymlink(t *testing.T) {
	dir := t.TempDir()

	repoFile := filepath.Join(dir, "repo", "file")
	setupFile(t, repoFile, "content")

	target := filepath.Join(dir, "link")
	setupSymlink(t, repoFile, target)

	if !IsOurSymlink(target, repoFile) {
		t.Error("IsOurSymlink() should return true")
	}

	// Not our symlink.
	otherTarget := filepath.Join(dir, "other_link")
	setupSymlink(t, "/tmp/other", otherTarget)

	if IsOurSymlink(otherTarget, repoFile) {
		t.Error("IsOurSymlink() should return false for wrong target")
	}
}

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src", "file")
	dst := filepath.Join(dir, "dst", "file")

	setupFile(t, src, "content")

	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile() error: %v", err)
	}

	// Source should be gone.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file should be removed")
	}

	// Destination should have the content.
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("dst content = %q, want %q", content, "content")
	}
}

func TestMoveFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "file")
	dst := filepath.Join(dir, "a", "b", "c", "file")

	setupFile(t, src, "data")

	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile() error: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(content) != "data" {
		t.Errorf("content = %q, want %q", content, "data")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	setupFile(t, src, "hello")

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error: %v", err)
	}

	// Both should exist.
	srcContent, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	dstContent, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}

	if string(srcContent) != "hello" {
		t.Error("source should be unchanged")
	}
	if string(dstContent) != "hello" {
		t.Error("destination should have same content")
	}
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("#!/bin/bash"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error: %v", err)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat src: %v", err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}

	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("permissions: src=%v, dst=%v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestCleanEmptyDirs(t *testing.T) {
	root := t.TempDir()

	// Create nested empty dirs.
	deepDir := filepath.Join(root, "a", "b", "c")
	setupFile(t, filepath.Join(deepDir, "file"), "x")
	filePath := filepath.Join(deepDir, "file")
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if err := CleanEmptyDirs(filePath, root); err != nil {
		t.Fatalf("CleanEmptyDirs() error: %v", err)
	}

	// All empty dirs should be removed.
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Error("empty parent dirs should be removed")
	}
}

func TestCleanEmptyDirs_StopsAtNonEmpty(t *testing.T) {
	root := t.TempDir()

	// Create: root/a/b/c/ with a file in root/a/
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	setupFile(t, filepath.Join(root, "a", "keep.txt"), "keep")

	// Trigger cleanup from root/a/b/c/deleted
	if err := CleanEmptyDirs(filepath.Join(root, "a", "b", "c", "deleted"), root); err != nil {
		t.Fatalf("CleanEmptyDirs() error: %v", err)
	}

	// root/a/ should remain (has keep.txt), but root/a/b/ and root/a/b/c/ should be gone.
	if _, err := os.Stat(filepath.Join(root, "a")); os.IsNotExist(err) {
		t.Error("root/a/ should remain (has a file)")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b")); !os.IsNotExist(err) {
		t.Error("root/a/b/ should be removed (empty)")
	}
}

func TestCleanEmptyDirs_DoesNotRemoveRoot(t *testing.T) {
	root := t.TempDir()

	// Root itself is empty except for the file we "deleted".
	if err := CleanEmptyDirs(filepath.Join(root, "file"), root); err != nil {
		t.Fatalf("CleanEmptyDirs() error: %v", err)
	}

	// Root should still exist.
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Error("root should not be removed")
	}
}

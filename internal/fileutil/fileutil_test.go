package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	err := AtomicWriteFile(path, 0644, func(w io.Writer) error {
		_, err := w.Write([]byte("hello"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", data, "hello")
	}
}

func TestAtomicWriteFile_ErrorCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	// Write initial content.
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	err := AtomicWriteFile(path, 0644, func(w io.Writer) error {
		return fmt.Errorf("simulated failure")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// Original file should be untouched.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("original file was modified: got %q", data)
	}

	// Temp file should not exist.
	tmp := path + ".dotfather-tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file was not cleaned up")
	}
}

func TestAtomicWriteFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	err := AtomicWriteFile(path, 0644, func(w io.Writer) error {
		_, err := w.Write([]byte("nested"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Fatalf("got %q, want %q", data, "nested")
	}
}

func TestSafeWriteTarget_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := SafeWriteTarget(link, 0600, func(w io.Writer) error {
		_, err := w.Write([]byte("decrypted"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// link should now be a regular file, not a symlink.
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("destination is still a symlink")
	}

	// The original target should be unchanged.
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatalf("symlink target was modified: got %q", data)
	}

	// link should contain the new content.
	data, err = os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "decrypted" {
		t.Fatalf("got %q, want %q", data, "decrypted")
	}
}

func TestSafeWriteTarget_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	err := SafeWriteTarget(path, 0600, func(w io.Writer) error {
		_, err := w.Write([]byte("new"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("got %q, want %q", data, "new")
	}
}

func TestUniqueBackupPath_NoCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	got := UniqueBackupPath(path, ".dotfather-backup")
	want := path + ".dotfather-backup"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUniqueBackupPath_Collision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	existing := path + ".dotfather-backup"

	// Create the plain backup so it collides.
	if err := os.WriteFile(existing, []byte("old backup"), 0644); err != nil {
		t.Fatal(err)
	}

	got := UniqueBackupPath(path, ".dotfather-backup")
	if got == existing {
		t.Fatal("should not return the colliding path")
	}
	if !strings.HasPrefix(got, existing+".") {
		t.Fatalf("got %q, want prefix %q", got, existing+".")
	}
}

func TestFileHash_Consistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")

	if err := os.WriteFile(path, []byte("test data"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := FileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := FileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("hashes should be identical for same content")
	}
}

func TestFileHash_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	p2 := filepath.Join(dir, "b.txt")

	if err := os.WriteFile(p1, []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := FileHash(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := FileHash(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("hashes should differ for different content")
	}
}

func TestFileHash_NotFound(t *testing.T) {
	_, err := FileHash("/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

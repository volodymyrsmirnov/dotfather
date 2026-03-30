package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return resolved
}

func TestHomeDir(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	got, err := HomeDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != home {
		t.Errorf("HomeDir() = %q, want %q", got, home)
	}
}

func TestExpandPath(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "tilde prefix",
			input: "~/foo/bar",
			want:  filepath.Join(home, "foo/bar"),
		},
		{
			name:  "bare tilde",
			input: "~",
			want:  home,
		},
		{
			name:  "absolute path",
			input: "/tmp/foo",
			want:  "/tmp/foo",
		},
		{
			name:  "relative path",
			input: "foo/bar",
			// Relative to cwd — we just check it's absolute.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExpandPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("ExpandPath(%q) = %q, want absolute path", tt.input, got)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	// Create a real directory so EvalSymlinks works.
	dir := filepath.Join(home, ".config")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := NormalizePath("~/.config/foo.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, ".config", "foo.yaml")
	if got != want {
		t.Errorf("NormalizePath() = %q, want %q", got, want)
	}
}

func TestNormalizePath_SymlinkedParent(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	// Create real dir and symlink to it.
	realDir := filepath.Join(home, "real_config")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	symlinkDir := filepath.Join(home, "linked_config")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := NormalizePath(filepath.Join(symlinkDir, "foo.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parent should be resolved to real path.
	want := filepath.Join(realDir, "foo.yaml")
	if got != want {
		t.Errorf("NormalizePath() = %q, want %q", got, want)
	}
}

func TestNormalizePath_NonexistentParent(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	// Parent doesn't exist — should still return expanded path.
	got, err := NormalizePath("~/nonexistent/dir/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "nonexistent", "dir", "file.txt")
	if got != want {
		t.Errorf("NormalizePath() = %q, want %q", got, want)
	}
}

func TestRelToHome(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	got, err := RelToHome(filepath.Join(home, ".config", "foo.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(".config", "foo.yaml")
	if got != want {
		t.Errorf("RelToHome() = %q, want %q", got, want)
	}
}

func TestIsUnderHome(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"under home", filepath.Join(home, ".bashrc"), true},
		{"nested under home", filepath.Join(home, ".config/foo/bar"), true},
		{"home itself", home, true},
		{"outside home", "/tmp/foo", false},
		{"relative escape", filepath.Join(home, "..", "other"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsUnderHome(tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("IsUnderHome(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestTildePath(t *testing.T) {
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"under home", filepath.Join(home, ".bashrc"), "~/.bashrc"},
		{"nested", filepath.Join(home, ".config/foo"), filepath.Join("~", ".config/foo")},
		{"outside home", "/tmp/foo", "/tmp/foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TildePath(tt.path)
			if got != tt.want {
				t.Errorf("TildePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

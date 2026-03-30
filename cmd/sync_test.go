package cmd

import (
	"testing"
)

func TestGenerateCommitMessage_SingleAdd(t *testing.T) {
	porcelain := "A  .bashrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Add .bashrc" {
		t.Errorf("got %q, want %q", msg, "Add .bashrc")
	}
}

func TestGenerateCommitMessage_SingleModify(t *testing.T) {
	porcelain := "M  .bashrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Update .bashrc" {
		t.Errorf("got %q, want %q", msg, "Update .bashrc")
	}
}

func TestGenerateCommitMessage_SingleDelete(t *testing.T) {
	porcelain := "D  .bashrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Remove .bashrc" {
		t.Errorf("got %q, want %q", msg, "Remove .bashrc")
	}
}

func TestGenerateCommitMessage_Mixed(t *testing.T) {
	porcelain := "A  .bashrc\nM  .zshrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Add .bashrc, Update .zshrc" {
		t.Errorf("got %q, want %q", msg, "Add .bashrc, Update .zshrc")
	}
}

func TestGenerateCommitMessage_ThreeFiles(t *testing.T) {
	porcelain := "A  .bashrc\nM  .zshrc\nD  .vimrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Add .bashrc, Update .zshrc, Remove .vimrc" {
		t.Errorf("got %q, want %q", msg, "Add .bashrc, Update .zshrc, Remove .vimrc")
	}
}

func TestGenerateCommitMessage_ManyFiles(t *testing.T) {
	porcelain := "A  a\nA  b\nM  c\nD  d\n"
	msg := generateCommitMessage(porcelain)
	// 4 files -> uses the summary format.
	if msg == "" {
		t.Error("message should not be empty")
	}
	if len(msg) > 100 {
		t.Errorf("message too long: %q", msg)
	}
	// Should contain the count.
	if !contains(msg, "4 dotfiles") {
		t.Errorf("expected '4 dotfiles' in message, got %q", msg)
	}
}

func TestGenerateCommitMessage_Untracked(t *testing.T) {
	porcelain := "?? .bashrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Add .bashrc" {
		t.Errorf("got %q, want %q", msg, "Add .bashrc")
	}
}

func TestGenerateCommitMessage_Empty(t *testing.T) {
	porcelain := ""
	msg := generateCommitMessage(porcelain)
	if msg != "Update dotfiles" {
		t.Errorf("got %q, want %q", msg, "Update dotfiles")
	}
}

func TestGenerateCommitMessage_ModifiedUnstaged(t *testing.T) {
	// "M " = unstaged modification
	porcelain := " M .bashrc\n"
	msg := generateCommitMessage(porcelain)
	if msg != "Update .bashrc" {
		t.Errorf("got %q, want %q", msg, "Update .bashrc")
	}
}

func TestShellescape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"has space", "'has space'"},
		{"has'quote", "'has'\\''quote'"},
		{"", "''"},
	}

	for _, tt := range tests {
		got := shellescape(tt.input)
		if got != tt.want {
			t.Errorf("shellescape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

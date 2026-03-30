package shellinit

import (
	"strings"
	"testing"
)

func TestForShell_Bash(t *testing.T) {
	script, err := ForShell("bash")
	if err != nil {
		t.Fatalf("ForShell(bash) error: %v", err)
	}
	if !strings.Contains(script, "dotfather()") {
		t.Error("bash init should contain dotfather function")
	}
	if !strings.Contains(script, `command dotfather cd`) {
		t.Error("bash init should reference 'command dotfather cd'")
	}
}

func TestForShell_Zsh(t *testing.T) {
	script, err := ForShell("zsh")
	if err != nil {
		t.Fatalf("ForShell(zsh) error: %v", err)
	}
	if !strings.Contains(script, "dotfather()") {
		t.Error("zsh init should contain dotfather function")
	}
}

func TestForShell_Fish(t *testing.T) {
	script, err := ForShell("fish")
	if err != nil {
		t.Fatalf("ForShell(fish) error: %v", err)
	}
	if !strings.Contains(script, "function dotfather") {
		t.Error("fish init should contain 'function dotfather'")
	}
	if !strings.Contains(script, "command dotfather cd") {
		t.Error("fish init should reference 'command dotfather cd'")
	}
}

func TestForShell_Unsupported(t *testing.T) {
	_, err := ForShell("powershell")
	if err == nil {
		t.Error("ForShell(powershell) should return error")
	}
}

func TestForShell_BashAndZshAreSame(t *testing.T) {
	bash, _ := ForShell("bash")
	zsh, _ := ForShell("zsh")
	if bash != zsh {
		t.Error("bash and zsh init scripts should be identical")
	}
}

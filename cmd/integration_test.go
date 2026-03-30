package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
	"github.com/volodymyrsmirnov/dotfather/testutil"
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

func TestInitFreshWorkflow(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "init"})
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify repo created.
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		t.Error("git repo should be initialized")
	}
}

func TestInitIdempotent(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "init"})
	if err != nil {
		t.Fatalf("init should be idempotent: %v", err)
	}
}

func TestInitExistingNonGitDir(t *testing.T) {
	home := testutil.SetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".dotfather"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "init"})
	if err == nil {
		t.Error("init should fail when dir exists but is not a git repo")
	}
}

func TestAddAndListWorkflow(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Create a file to add.
	testutil.CreateFile(t, home, ".bashrc", "# my bashrc")

	app := NewApp()

	// Add the file.
	err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")})
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify: original is now a symlink.
	linkTarget, err := os.Readlink(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	expectedTarget := filepath.Join(repoDir, ".bashrc")
	if linkTarget != expectedTarget {
		t.Errorf("symlink target = %q, want %q", linkTarget, expectedTarget)
	}

	// Verify: file exists in repo with correct content.
	content, err := os.ReadFile(filepath.Join(repoDir, ".bashrc"))
	if err != nil {
		t.Fatalf("read repo file: %v", err)
	}
	if string(content) != "# my bashrc" {
		t.Errorf("repo file content = %q, want %q", content, "# my bashrc")
	}

	// Verify: content accessible through symlink.
	symlinkContent, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("read through symlink: %v", err)
	}
	if string(symlinkContent) != "# my bashrc" {
		t.Errorf("symlink content = %q, want %q", symlinkContent, "# my bashrc")
	}
}

func TestAddWithKeepFlag(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".bashrc", "# my bashrc")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", "--keep", filepath.Join(home, ".bashrc")})
	if err != nil {
		t.Fatalf("add --keep failed: %v", err)
	}

	// Verify: .bak file exists.
	bakContent, err := os.ReadFile(filepath.Join(home, ".bashrc.bak"))
	if err != nil {
		t.Fatalf("read .bak file: %v", err)
	}
	if string(bakContent) != "# my bashrc" {
		t.Errorf(".bak content = %q, want %q", bakContent, "# my bashrc")
	}

	// Verify: original is a symlink.
	linkTarget, err := os.Readlink(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != filepath.Join(repoDir, ".bashrc") {
		t.Errorf("symlink target = %q, want %q", linkTarget, filepath.Join(repoDir, ".bashrc"))
	}
}

func TestAddNestedFile(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".config/nvim/init.lua", "-- nvim config")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".config/nvim/init.lua")})
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify nested structure in repo.
	content, err := os.ReadFile(filepath.Join(repoDir, ".config/nvim/init.lua"))
	if err != nil {
		t.Fatalf("read nested file: %v", err)
	}
	if string(content) != "-- nvim config" {
		t.Errorf("content = %q, want %q", content, "-- nvim config")
	}

	// Verify symlink.
	state := linker.Check(
		filepath.Join(repoDir, ".config/nvim/init.lua"),
		filepath.Join(home, ".config/nvim/init.lua"),
	)
	if state != linker.OK {
		t.Errorf("symlink state = %v, want OK", state)
	}
}

func TestAddMultipleFiles(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".bashrc", "bash")
	testutil.CreateFile(t, home, ".zshrc", "zsh")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add",
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
	})
	if err != nil {
		t.Fatalf("add multiple failed: %v", err)
	}

	// Both should be symlinks.
	for _, name := range []string{".bashrc", ".zshrc"} {
		if _, err := os.Readlink(filepath.Join(home, name)); err != nil {
			t.Errorf("%s should be a symlink: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(repoDir, name)); os.IsNotExist(err) {
			t.Errorf("%s should exist in repo", name)
		}
	}
}

func TestAddAlreadyManaged(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".bashrc", "bash")

	app := NewApp()
	// Add first time.
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")}); err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Add again — should not error.
	err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")})
	if err != nil {
		t.Errorf("re-adding managed file should not error: %v", err)
	}
}

func TestAddDirectory(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".config/myapp/a.yaml", "a")
	testutil.CreateFile(t, home, ".config/myapp/b.yaml", "b")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".config/myapp")})
	if err != nil {
		t.Fatalf("add directory failed: %v", err)
	}

	// Both files should be in repo.
	for _, name := range []string{"a.yaml", "b.yaml"} {
		path := filepath.Join(repoDir, ".config/myapp", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s should exist in repo", name)
		}
	}
}

func TestAddFileOutsideHome(t *testing.T) {
	testutil.SetupTestHome(t)

	tmpFile := filepath.Join(resolvedTempDir(t), "outside.txt")
	if err := os.WriteFile(tmpFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", tmpFile})
	if err == nil {
		t.Error("adding file outside home should fail")
	}
}

func TestForgetOutsideHome(t *testing.T) {
	testutil.SetupTestHome(t)

	app := NewApp()
	_ = app.Run(context.Background(), []string{"dotfather", "init"})

	// Create a file outside the test home.
	tmpFile := filepath.Join(resolvedTempDir(t), "outside.txt")
	if err := os.WriteFile(tmpFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := app.Run(context.Background(), []string{"dotfather", "forget", tmpFile})
	if err == nil {
		t.Error("forgetting file outside home should fail")
	}
}

func TestAddNoRepo(t *testing.T) {
	testutil.SetupTestHome(t)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", "/some/file"})
	if err == nil {
		t.Error("add without repo should fail")
	}
}

func TestAddFile_UnlinkedRefusesWithoutForce(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Add a file to get it managed.
	bashrc := filepath.Join(home, ".bashrc")
	testutil.CreateFile(t, home, ".bashrc", "# my bashrc")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", bashrc}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Replace the symlink with a regular file (simulating UNLINKED state).
	if err := os.Remove(bashrc); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := os.WriteFile(bashrc, []byte("local edit"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Re-add without --force should fail.
	err := app.Run(context.Background(), []string{"dotfather", "add", bashrc})
	if err == nil {
		t.Error("re-add of unlinked file without --force should fail")
	}
}

func TestAddFile_UnlinkedSucceedsWithForce(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	bashrc := filepath.Join(home, ".bashrc")
	testutil.CreateFile(t, home, ".bashrc", "# my bashrc")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", bashrc}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Replace symlink with regular file.
	if err := os.Remove(bashrc); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(bashrc, []byte("local edit"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Re-add with --force should succeed.
	if err := app.Run(context.Background(), []string{"dotfather", "add", "--force", bashrc}); err != nil {
		t.Fatalf("add --force should succeed: %v", err)
	}

	// Verify it's a symlink again.
	info, err := os.Lstat(bashrc)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("file should be a symlink after force re-add")
	}
}

func TestForgetWorkflow(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Add a file first.
	testutil.CreateFile(t, home, ".bashrc", "# my bashrc")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Forget it.
	err := app.Run(context.Background(), []string{"dotfather", "forget", filepath.Join(home, ".bashrc")})
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	// Verify: original is a regular file, not a symlink.
	info, err := os.Lstat(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("file should not be a symlink after forget")
	}

	// Verify: content preserved.
	content, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "# my bashrc" {
		t.Errorf("content = %q, want %q", content, "# my bashrc")
	}

	// Verify: file removed from repo.
	if _, err := os.Stat(filepath.Join(repoDir, ".bashrc")); !os.IsNotExist(err) {
		t.Error("file should be removed from repo")
	}
}

func TestForgetNotManaged(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "forget", filepath.Join(home, ".bashrc")})
	if err == nil {
		t.Error("forget unmanaged file should fail")
	}
}

func TestForgetCleansEmptyDirs(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Add a nested file.
	testutil.CreateFile(t, home, ".config/app/config.yaml", "config")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".config/app/config.yaml")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Forget it.
	if err := app.Run(context.Background(), []string{"dotfather", "forget", filepath.Join(home, ".config/app/config.yaml")}); err != nil {
		t.Fatalf("forget: %v", err)
	}

	// Empty .config/app/ dir in repo should be cleaned up.
	if _, err := os.Stat(filepath.Join(repoDir, ".config/app")); !os.IsNotExist(err) {
		t.Error("empty nested dir in repo should be cleaned up")
	}
}

func TestSyncLocalOnly(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Add a file.
	testutil.CreateFile(t, home, ".bashrc", "# bash")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Sync (no remote).
	err := app.Run(context.Background(), []string{"dotfather", "sync"})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify a commit was made.
	// Check git log has a commit mentioning .bashrc.
}

func TestSyncNoChanges(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	// Sync with no changes.
	err := app.Run(context.Background(), []string{"dotfather", "sync"})
	if err != nil {
		t.Fatalf("sync with no changes should not error: %v", err)
	}
}

func TestStatusEmpty(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "status"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
}

func TestStatusWithFiles(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".bashrc", "bash")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := app.Run(context.Background(), []string{"dotfather", "status"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
}

func TestStatusJSON(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	testutil.CreateFile(t, home, ".bashrc", "bash")
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".bashrc")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := app.Run(context.Background(), []string{"dotfather", "status", "--json"})
	if err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
}

func TestListEmpty(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
}

func TestDiffNoChanges(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "diff"})
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
}

func TestCDPrintsPath(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "cd"})
	if err != nil {
		t.Fatalf("cd failed: %v", err)
	}
}

func TestCDShellInit(t *testing.T) {
	app := NewApp()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			err := app.Run(context.Background(), []string{"dotfather", "cd", "--shell-init", shell})
			if err != nil {
				t.Fatalf("cd --shell-init %s failed: %v", shell, err)
			}
		})
	}
}

func TestCDShellInitUnsupported(t *testing.T) {
	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "cd", "--shell-init", "powershell"})
	if err == nil {
		t.Error("cd --shell-init powershell should fail")
	}
}

func TestFullWorkflow(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")

	app := NewApp()

	// 1. Init.
	if err := app.Run(context.Background(), []string{"dotfather", "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// 2. Add files.
	testutil.CreateFile(t, home, ".bashrc", "# bash config")
	testutil.CreateFile(t, home, ".config/starship.toml", "[prompt]\nformat = 'bold'")

	if err := app.Run(context.Background(), []string{"dotfather", "add",
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".config/starship.toml"),
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// 3. Verify symlinks.
	for _, f := range []string{".bashrc", ".config/starship.toml"} {
		target, err := os.Readlink(filepath.Join(home, f))
		if err != nil {
			t.Errorf("%s should be symlink: %v", f, err)
		}
		expected := filepath.Join(repoDir, f)
		if target != expected {
			t.Errorf("%s symlink = %q, want %q", f, target, expected)
		}
	}

	// 4. Edit a file through the symlink.
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# updated bash config"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 5. Sync.
	if err := app.Run(context.Background(), []string{"dotfather", "sync"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// 6. Verify repo file was updated.
	content, _ := os.ReadFile(filepath.Join(repoDir, ".bashrc"))
	if string(content) != "# updated bash config" {
		t.Errorf("repo .bashrc = %q, want updated content", content)
	}

	// 7. Status should show all OK.
	if err := app.Run(context.Background(), []string{"dotfather", "status"}); err != nil {
		t.Fatalf("status: %v", err)
	}

	// 8. Forget one file.
	if err := app.Run(context.Background(), []string{"dotfather", "forget",
		filepath.Join(home, ".bashrc"),
	}); err != nil {
		t.Fatalf("forget: %v", err)
	}

	// 9. Verify .bashrc is back to a regular file.
	info, _ := os.Lstat(filepath.Join(home, ".bashrc"))
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error(".bashrc should be a regular file after forget")
	}

	// 10. List should only show starship.toml (and .gitkeep from test setup).
	// Verify .bashrc is no longer in repo.
	if _, err := os.Stat(filepath.Join(repoDir, ".bashrc")); !os.IsNotExist(err) {
		t.Error(".bashrc should not exist in repo after forget")
	}
}

func TestInitCloneWorkflow(t *testing.T) {
	home := testutil.SetupTestHome(t)

	// Create a "remote" repo with some files.
	remoteDir := filepath.Join(resolvedTempDir(t), "remote")
	testutil.InitGitRepo(t, remoteDir)
	testutil.CreateFile(t, remoteDir, ".bashrc", "# from remote")
	testutil.CreateFile(t, remoteDir, ".config/app.yaml", "key: value")
	testutil.GitCommitAll(t, remoteDir, "add dotfiles")

	// Init from clone.
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "init", remoteDir}); err != nil {
		t.Fatalf("init clone: %v", err)
	}

	repoDir := filepath.Join(home, ".dotfather")

	// Verify files exist in repo.
	for _, f := range []string{".bashrc", ".config/app.yaml"} {
		if _, err := os.Stat(filepath.Join(repoDir, f)); os.IsNotExist(err) {
			t.Errorf("%s should exist in cloned repo", f)
		}
	}

	// Verify symlinks created.
	for _, f := range []string{".bashrc", ".config/app.yaml"} {
		target, err := os.Readlink(filepath.Join(home, f))
		if err != nil {
			t.Errorf("%s should be a symlink: %v", f, err)
			continue
		}
		if target != filepath.Join(repoDir, f) {
			t.Errorf("%s symlink = %q, want %q", f, target, filepath.Join(repoDir, f))
		}
	}

	// Verify content accessible through symlinks.
	content, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(content) != "# from remote" {
		t.Errorf("symlink content = %q, want %q", content, "# from remote")
	}
}

func TestInitCloneWithConflicts(t *testing.T) {
	home := testutil.SetupTestHome(t)

	// Create a "remote" repo.
	remoteDir := filepath.Join(resolvedTempDir(t), "remote")
	testutil.InitGitRepo(t, remoteDir)
	testutil.CreateFile(t, remoteDir, ".bashrc", "# remote version")
	testutil.GitCommitAll(t, remoteDir, "add bashrc")

	// Create an existing file at the target.
	testutil.CreateFile(t, home, ".bashrc", "# local version")

	// Init from clone — should back up existing file.
	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "init", remoteDir}); err != nil {
		t.Fatalf("init clone: %v", err)
	}

	// Verify backup exists.
	backupContent, err := os.ReadFile(filepath.Join(home, ".bashrc.dotfather-backup"))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != "# local version" {
		t.Errorf("backup content = %q, want %q", backupContent, "# local version")
	}

	// Verify symlink points to remote version.
	content, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(content) != "# remote version" {
		t.Errorf("symlink content = %q, want %q", content, "# remote version")
	}
}

func TestAddExistingSymlink(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Create a file and a symlink to it.
	realFile := testutil.CreateFile(t, home, "real_config", "real content")
	symlinkPath := filepath.Join(home, ".config_link")
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", symlinkPath})
	if err != nil {
		t.Fatalf("add symlink failed: %v", err)
	}

	// Verify: the symlink now points to the repo.
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if !strings.HasPrefix(target, repoDir) {
		t.Errorf("symlink should point to repo, got %q", target)
	}

	// Verify: content preserved.
	content, _ := os.ReadFile(symlinkPath)
	if string(content) != "real content" {
		t.Errorf("content = %q, want %q", content, "real content")
	}
}

func TestForgetWithForceFlag(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Put a file in the repo manually.
	testutil.CreateFile(t, repoDir, ".bashrc", "repo version")
	testutil.GitCommitAll(t, repoDir, "add bashrc")

	// Create a regular file (not a symlink) at the target.
	testutil.CreateFile(t, home, ".bashrc", "local version")

	app := NewApp()

	// Without force — should fail.
	err := app.Run(context.Background(), []string{"dotfather", "forget", filepath.Join(home, ".bashrc")})
	if err == nil {
		t.Error("forget without --force should fail when target has non-symlink file")
	}

	// With force — should succeed.
	err = app.Run(context.Background(), []string{"dotfather", "forget", "--force", filepath.Join(home, ".bashrc")})
	if err != nil {
		t.Fatalf("forget --force failed: %v", err)
	}

	// Verify content is from repo.
	content, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(content) != "repo version" {
		t.Errorf("content = %q, want repo version", content)
	}
}

func TestAddRejectsRepoPath(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Create a file inside the repo.
	testutil.CreateFile(t, repoDir, "somefile.txt", "repo internal")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(repoDir, "somefile.txt")})
	if err == nil {
		t.Error("add should reject files inside the dotfather repo")
	}
}

func TestAddEncryptedRejectsRepoPath(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	if err := crypto.GenerateKey(repoDir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	testutil.CreateFile(t, repoDir, "somefile.txt", "repo internal")

	app := NewApp()
	err := app.Run(context.Background(), []string{"dotfather", "add", "--encrypt", filepath.Join(repoDir, "somefile.txt")})
	if err == nil {
		t.Error("add --encrypt should reject files inside the dotfather repo")
	}
}

func TestForgetEncrypted_DecryptsIfMissing(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	if err := crypto.GenerateKey(repoDir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create a file and add it encrypted.
	secretPath := filepath.Join(home, ".secret")
	testutil.CreateFile(t, home, ".secret", "my secret data")

	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", "--encrypt", secretPath}); err != nil {
		t.Fatalf("add --encrypt failed: %v", err)
	}

	// Verify .age file exists in repo.
	encFile := filepath.Join(repoDir, ".secret.age")
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("encrypted file should exist: %v", err)
	}

	// Delete the plaintext target.
	if err := os.Remove(secretPath); err != nil {
		t.Fatalf("remove plaintext: %v", err)
	}

	// Forget should decrypt before removing.
	if err := app.Run(context.Background(), []string{"dotfather", "forget", secretPath}); err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	// Verify plaintext was restored.
	content, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatalf("plaintext should be restored: %v", err)
	}
	if string(content) != "my secret data" {
		t.Errorf("content = %q, want %q", content, "my secret data")
	}

	// Verify .age file was removed.
	if _, err := os.Stat(encFile); !os.IsNotExist(err) {
		t.Error(".age file should be removed from repo")
	}
}

func TestForgetEncrypted_TargetExists(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	if err := crypto.GenerateKey(repoDir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	secretPath := filepath.Join(home, ".secret")
	testutil.CreateFile(t, home, ".secret", "my secret data")

	app := NewApp()
	if err := app.Run(context.Background(), []string{"dotfather", "add", "--encrypt", secretPath}); err != nil {
		t.Fatalf("add --encrypt failed: %v", err)
	}

	// Target still exists — forget should work without decrypting.
	if err := app.Run(context.Background(), []string{"dotfather", "forget", secretPath}); err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	// Verify .age removed.
	encFile := filepath.Join(repoDir, ".secret.age")
	if _, err := os.Stat(encFile); !os.IsNotExist(err) {
		t.Error(".age file should be removed from repo")
	}

	// Verify plaintext still exists.
	if _, err := os.Stat(secretPath); err != nil {
		t.Error("plaintext target should still exist")
	}
}

func TestForgetDirectory(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	// Create multiple files under a directory.
	testutil.CreateFile(t, home, ".config/myapp/config.yaml", "app config")
	testutil.CreateFile(t, home, ".config/myapp/theme.json", "theme data")

	app := NewApp()

	// Add both files.
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".config/myapp/config.yaml")}); err != nil {
		t.Fatalf("add config: %v", err)
	}
	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".config/myapp/theme.json")}); err != nil {
		t.Fatalf("add theme: %v", err)
	}

	// Both should be symlinks now.
	if linker.Check(filepath.Join(repoDir, ".config/myapp/config.yaml"), filepath.Join(home, ".config/myapp/config.yaml")) != linker.OK {
		t.Error("config.yaml should be linked")
	}
	if linker.Check(filepath.Join(repoDir, ".config/myapp/theme.json"), filepath.Join(home, ".config/myapp/theme.json")) != linker.OK {
		t.Error("theme.json should be linked")
	}

	// Forget the directory.
	if err := app.Run(context.Background(), []string{"dotfather", "forget", filepath.Join(home, ".config/myapp")}); err != nil {
		t.Fatalf("forget directory failed: %v", err)
	}

	// Both files should be restored as regular files.
	for _, name := range []string{"config.yaml", "theme.json"} {
		fpath := filepath.Join(home, ".config/myapp", name)
		info, err := os.Lstat(fpath)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a regular file, not a symlink", name)
		}
	}

	// Repo should not have those files.
	if _, err := os.Stat(filepath.Join(repoDir, ".config/myapp/config.yaml")); !os.IsNotExist(err) {
		t.Error("config.yaml should be removed from repo")
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".config/myapp/theme.json")); !os.IsNotExist(err) {
		t.Error("theme.json should be removed from repo")
	}
}

func TestConvertToEncrypted(t *testing.T) {
	home := testutil.SetupTestHome(t)
	repoDir := filepath.Join(home, ".dotfather")
	testutil.InitGitRepo(t, repoDir)

	if err := crypto.GenerateKey(repoDir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Add file normally (creates symlink).
	testutil.CreateFile(t, home, ".secret", "secret content")
	app := NewApp()

	if err := app.Run(context.Background(), []string{"dotfather", "add", filepath.Join(home, ".secret")}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify it's a symlink.
	if linker.Check(filepath.Join(repoDir, ".secret"), filepath.Join(home, ".secret")) != linker.OK {
		t.Fatal("should be linked after add")
	}

	// Re-add with --encrypt to convert.
	if err := app.Run(context.Background(), []string{"dotfather", "add", "--encrypt", filepath.Join(home, ".secret")}); err != nil {
		t.Fatalf("add --encrypt (convert) failed: %v", err)
	}

	// Target should be a regular file, not a symlink.
	info, err := os.Lstat(filepath.Join(home, ".secret"))
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("target should be a regular file after conversion, not a symlink")
	}

	// Content should be preserved.
	content, _ := os.ReadFile(filepath.Join(home, ".secret"))
	if string(content) != "secret content" {
		t.Errorf("content = %q, want %q", content, "secret content")
	}

	// .age file should exist in repo.
	if _, err := os.Stat(filepath.Join(repoDir, ".secret.age")); err != nil {
		t.Error(".age file should exist in repo")
	}

	// Plain file should NOT exist in repo.
	if _, err := os.Stat(filepath.Join(repoDir, ".secret")); !os.IsNotExist(err) {
		t.Error("plain file should be removed from repo after conversion")
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestResolveConflicts_AcceptLocal_DuringRebase(t *testing.T) {
	home := testutil.SetupTestHome(t)
	ctx := context.Background()

	// Create a bare "remote" repo.
	remoteDir := resolvedTempDir(t)
	runGitCmd(t, remoteDir, "init", "--bare")

	// Clone remote into dotfather repo location.
	repoDir := filepath.Join(home, ".dotfather")
	runGitCmd(t, home, "clone", remoteDir, repoDir)
	runGitCmd(t, repoDir, "config", "user.email", "test@test.com")
	runGitCmd(t, repoDir, "config", "user.name", "Test")

	// Create a file, commit, push.
	conflictFile := ".config/app/config.yaml"
	testutil.CreateFile(t, repoDir, conflictFile, "original content")
	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "initial")
	runGitCmd(t, repoDir, "push", "origin", "HEAD")

	// Simulate remote change: clone the remote again, modify the file, push.
	secondClone := resolvedTempDir(t)
	runGitCmd(t, home, "clone", remoteDir, secondClone)
	runGitCmd(t, secondClone, "config", "user.email", "other@test.com")
	runGitCmd(t, secondClone, "config", "user.name", "Other")
	if err := os.WriteFile(filepath.Join(secondClone, conflictFile), []byte("remote change"), 0644); err != nil {
		t.Fatalf("write remote change: %v", err)
	}
	runGitCmd(t, secondClone, "add", ".")
	runGitCmd(t, secondClone, "commit", "-m", "remote edit")
	runGitCmd(t, secondClone, "push", "origin", "HEAD")

	// Now make a divergent local change.
	if err := os.WriteFile(filepath.Join(repoDir, conflictFile), []byte("local change"), 0644); err != nil {
		t.Fatalf("write local change: %v", err)
	}
	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "local edit")

	// Pull --rebase will now conflict. Start the pull manually.
	pullCmd := exec.Command("git", "pull", "--rebase", "origin", "HEAD")
	pullCmd.Dir = repoDir
	_ = pullCmd.Run() // expected to fail with conflict

	// Verify there is a conflict.
	conflicted, err := git.ConflictedFiles(ctx, repoDir)
	if err != nil {
		t.Fatalf("ConflictedFiles: %v", err)
	}
	if len(conflicted) == 0 {
		t.Fatal("expected a rebase conflict")
	}

	// Resolve with "l" (accept local). Provide input via io.Reader.
	r, _ := repo.New()
	input := strings.NewReader("l\n")
	if err := resolveConflicts(ctx, r, conflicted, input); err != nil {
		t.Fatalf("resolveConflicts: %v", err)
	}

	// The file should now contain the LOCAL content, not the remote content.
	content, err := os.ReadFile(filepath.Join(repoDir, conflictFile))
	if err != nil {
		t.Fatalf("read resolved file: %v", err)
	}
	if got := string(content); got != "local change" {
		t.Errorf("after accepting local: content = %q, want %q", got, "local change")
	}
}

func TestSyncEncryptedConflictDetection(t *testing.T) {
	home := testutil.SetupTestHome(t)
	ctx := context.Background()

	// Set up a non-bare "remote" repo with an initial commit so branch exists.
	remoteDir := resolvedTempDir(t)
	testutil.InitGitRepo(t, remoteDir)

	// Clone remote into dotfather repo location.
	repoDir := filepath.Join(home, ".dotfather")
	runGitCmd(t, home, "clone", remoteDir, repoDir)
	runGitCmd(t, repoDir, "config", "user.email", "test@test.com")
	runGitCmd(t, repoDir, "config", "user.name", "Test")
	// Allow pushes to checked-out branch on the remote.
	runGitCmd(t, remoteDir, "config", "receive.denyCurrentBranch", "ignore")

	// Generate encryption keys.
	if err := crypto.GenerateKey(repoDir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create a secret file and encrypt it into the repo.
	secretPath := filepath.Join(home, ".secret")
	testutil.CreateFile(t, home, ".secret", "original secret")

	app := NewApp()
	if err := app.Run(ctx, []string{"dotfather", "add", "--encrypt", secretPath}); err != nil {
		t.Fatalf("add --encrypt: %v", err)
	}

	// Commit and push.
	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "add encrypted secret")
	branch, err := git.CurrentBranch(ctx, repoDir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	runGitCmd(t, repoDir, "push", "origin", branch)

	// Simulate remote change: clone again, re-encrypt with different content, push.
	secondClone := resolvedTempDir(t)
	runGitCmd(t, home, "clone", remoteDir, secondClone)
	runGitCmd(t, secondClone, "config", "user.email", "other@test.com")
	runGitCmd(t, secondClone, "config", "user.name", "Other")

	// Copy the age keys to the second clone so it can encrypt.
	identData, _ := os.ReadFile(filepath.Join(repoDir, ".age-identity"))
	recipData, _ := os.ReadFile(filepath.Join(repoDir, ".age-recipients"))
	if err := os.WriteFile(filepath.Join(secondClone, ".age-identity"), identData, 0600); err != nil {
		t.Fatalf("write .age-identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secondClone, ".age-recipients"), recipData, 0644); err != nil {
		t.Fatalf("write .age-recipients: %v", err)
	}

	// Encrypt a different version of the secret.
	remoteTmp := filepath.Join(t.TempDir(), "remote_secret")
	if err := os.WriteFile(remoteTmp, []byte("remote secret"), 0600); err != nil {
		t.Fatalf("write remote secret: %v", err)
	}
	if err := crypto.EncryptFile(secondClone, remoteTmp, filepath.Join(secondClone, ".secret.age")); err != nil {
		t.Fatalf("encrypt remote secret: %v", err)
	}
	runGitCmd(t, secondClone, "add", ".secret.age")
	runGitCmd(t, secondClone, "commit", "-m", "update encrypted secret from remote")
	runGitCmd(t, secondClone, "push", "origin", branch)

	// Now modify the local plaintext (simulating local edits).
	if err := os.WriteFile(secretPath, []byte("local secret edit"), 0600); err != nil {
		t.Fatalf("write local edit: %v", err)
	}

	// Snapshot hashes and detect local edits BEFORE pull.
	r, err := repo.New()
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	preAgeHashes := hashAgeFiles(r)
	localEdits := detectLocalEdits(r)

	// Pull from remote (should update .secret.age without conflict).
	if _, pullErr := git.Pull(ctx, repoDir, branch); pullErr != nil {
		t.Fatalf("pull should succeed (only .age changed on remote): %v", pullErr)
	}

	// Detect encrypted conflicts.
	postAgeHashes := hashAgeFiles(r)
	encConflicts := detectEncryptedConflicts(r, preAgeHashes, localEdits, postAgeHashes)

	if !encConflicts[".secret.age"] {
		t.Fatal("expected encrypted conflict for .secret.age")
	}

	// Verify the local backup was created.
	backupPath := secretPath + ".dotfather-local"
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup should exist: %v", err)
	}
	if string(backupContent) != "local secret edit" {
		t.Errorf("backup content = %q, want %q", backupContent, "local secret edit")
	}

	// Decrypt (remote wins).
	if err := decryptEncryptedFiles(r, encConflicts, preAgeHashes, postAgeHashes); err != nil {
		t.Fatalf("decryptEncryptedFiles: %v", err)
	}

	// The plaintext should now contain the remote version.
	content, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatalf("read decrypted secret: %v", err)
	}
	if string(content) != "remote secret" {
		t.Errorf("decrypted content = %q, want %q", content, "remote secret")
	}
}

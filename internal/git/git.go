package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitError wraps a failed git command with its stderr output.
type GitError struct {
	Command  string
	Args     []string
	Stderr   string
	ExitCode int
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed (exit %d): %s", e.Command, e.ExitCode, strings.TrimSpace(e.Stderr))
}

// run executes a git command in the given directory and returns stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		subcmd := ""
		if len(args) > 0 {
			subcmd = args[0]
		}
		return stdout.String(), &GitError{
			Command:  subcmd,
			Args:     args,
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}
	}

	return stdout.String(), nil
}

// runCombined executes a git command and returns combined stdout+stderr.
func runCombined(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		subcmd := ""
		if len(args) > 0 {
			subcmd = args[0]
		}
		return string(out), &GitError{
			Command:  subcmd,
			Args:     args,
			Stderr:   string(out),
			ExitCode: exitCode,
		}
	}

	return string(out), nil
}

// Init initializes a new git repository.
func Init(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "init")
	return err
}

// Clone clones a repository into the given directory.
func Clone(ctx context.Context, url, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", url, dir)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &GitError{
			Command:  "clone",
			Args:     []string{"clone", url, dir},
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}
	}
	return nil
}

// Add stages files.
func Add(ctx context.Context, dir string, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := run(ctx, dir, args...)
	return err
}

// AddAll stages all changes.
func AddAll(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "add", "-A")
	return err
}

// Commit creates a commit with the given message.
// Signing is disabled to avoid depending on the caller's GPG/SSH agent.
func Commit(ctx context.Context, dir, message string) error {
	_, err := run(ctx, dir, "-c", "commit.gpgsign=false", "commit", "-m", message)
	return err
}

// Pull runs git pull --rebase for the given branch.
func Pull(ctx context.Context, dir, branch string) (string, error) {
	return runCombined(ctx, dir, "pull", "--rebase", "origin", branch)
}

// Push pushes to origin for the given branch.
func Push(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, dir, "push", "origin", branch)
	return err
}

// Status returns the porcelain status output.
func Status(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "status", "--porcelain")
}

// Diff returns the diff output.
func Diff(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "diff")
}

// DiffCached returns the staged diff output.
func DiffCached(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "diff", "--cached")
}

// HasRemote checks if the "origin" remote is configured.
func HasRemote(ctx context.Context, dir string) bool {
	_, err := run(ctx, dir, "remote", "get-url", "origin")
	return err == nil
}

// RemoteGetURL returns the URL of the "origin" remote.
func RemoteGetURL(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "remote", "get-url", "origin")
}

// RemoteAdd adds a remote.
func RemoteAdd(ctx context.Context, dir, name, url string) error {
	_, err := run(ctx, dir, "remote", "add", name, url)
	return err
}

// CurrentBranch returns the current branch name.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// HasUncommitted returns true if there are uncommitted changes.
func HasUncommitted(ctx context.Context, dir string) (bool, error) {
	out, err := Status(ctx, dir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// ConflictedFiles returns the list of files with merge conflicts.
func ConflictedFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}

	return strings.Split(trimmed, "\n"), nil
}

// CheckoutOurs checks out the local version of a conflicting file.
func CheckoutOurs(ctx context.Context, dir, path string) error {
	_, err := run(ctx, dir, "checkout", "--ours", "--", path)
	return err
}

// CheckoutTheirs checks out the remote version of a conflicting file.
func CheckoutTheirs(ctx context.Context, dir, path string) error {
	_, err := run(ctx, dir, "checkout", "--theirs", "--", path)
	return err
}

// RebaseContinue continues a rebase in progress.
func RebaseContinue(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "rebase", "--continue")
	return err
}

// RebaseAbort aborts a rebase in progress.
func RebaseAbort(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "rebase", "--abort")
	return err
}

// IsGitRepo checks if the directory is a git repository.
func IsGitRepo(ctx context.Context, dir string) bool {
	_, err := run(ctx, dir, "rev-parse", "--git-dir")
	return err == nil
}

// GitAvailable checks if git is available in PATH.
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// AheadBehind returns how many commits ahead and behind the current branch is
// relative to its upstream.
func AheadBehind(ctx context.Context, dir string) (ahead, behind int, err error) {
	out, err := run(ctx, dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0, err
	}
	_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &ahead, &behind)
	return ahead, behind, nil
}

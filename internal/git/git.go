package git

import (
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
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
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
func runCombined(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
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
func Init(dir string) error {
	_, err := run(dir, "init")
	return err
}

// Clone clones a repository into the given directory.
func Clone(url, dir string) error {
	cmd := exec.Command("git", "clone", url, dir)
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
func Add(dir string, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := run(dir, args...)
	return err
}

// AddAll stages all changes.
func AddAll(dir string) error {
	_, err := run(dir, "add", "-A")
	return err
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	_, err := run(dir, "commit", "-m", message)
	return err
}

// Pull runs git pull --rebase for the given branch.
func Pull(dir, branch string) (string, error) {
	return runCombined(dir, "pull", "--rebase", "origin", branch)
}

// Push pushes to origin for the given branch.
func Push(dir, branch string) error {
	_, err := run(dir, "push", "origin", branch)
	return err
}

// Status returns the porcelain status output.
func Status(dir string) (string, error) {
	return run(dir, "status", "--porcelain")
}

// Diff returns the diff output.
func Diff(dir string) (string, error) {
	return run(dir, "diff")
}

// DiffCached returns the staged diff output.
func DiffCached(dir string) (string, error) {
	return run(dir, "diff", "--cached")
}

// HasRemote checks if the "origin" remote is configured.
func HasRemote(dir string) bool {
	_, err := run(dir, "remote", "get-url", "origin")
	return err == nil
}

// RemoteGetURL returns the URL of the "origin" remote.
func RemoteGetURL(dir string) (string, error) {
	return run(dir, "remote", "get-url", "origin")
}

// RemoteAdd adds a remote.
func RemoteAdd(dir, name, url string) error {
	_, err := run(dir, "remote", "add", name, url)
	return err
}

// CurrentBranch returns the current branch name.
func CurrentBranch(dir string) (string, error) {
	out, err := run(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// HasUncommitted returns true if there are uncommitted changes.
func HasUncommitted(dir string) (bool, error) {
	out, err := Status(dir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// ConflictedFiles returns the list of files with merge conflicts.
func ConflictedFiles(dir string) ([]string, error) {
	out, err := run(dir, "diff", "--name-only", "--diff-filter=U")
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
func CheckoutOurs(dir, path string) error {
	_, err := run(dir, "checkout", "--ours", "--", path)
	return err
}

// CheckoutTheirs checks out the remote version of a conflicting file.
func CheckoutTheirs(dir, path string) error {
	_, err := run(dir, "checkout", "--theirs", "--", path)
	return err
}

// RebaseContinue continues a rebase in progress.
func RebaseContinue(dir string) error {
	_, err := run(dir, "rebase", "--continue")
	return err
}

// RebaseAbort aborts a rebase in progress.
func RebaseAbort(dir string) error {
	_, err := run(dir, "rebase", "--abort")
	return err
}

// IsGitRepo checks if the directory is a git repository.
func IsGitRepo(dir string) bool {
	_, err := run(dir, "rev-parse", "--git-dir")
	return err == nil
}

// GitAvailable checks if git is available in PATH.
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// AheadBehind returns how many commits ahead and behind the current branch is
// relative to its upstream.
func AheadBehind(dir string) (ahead, behind int, err error) {
	out, err := run(dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0, err
	}
	_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &ahead, &behind)
	return ahead, behind, nil
}

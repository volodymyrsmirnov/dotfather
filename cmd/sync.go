package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	cli "github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/fileutil"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
	"github.com/volodymyrsmirnov/dotfather/internal/lock"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newSyncCommand() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Pull, commit, and push dotfile changes",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "interactive",
				Aliases: []string{"i"},
				Usage:   "Interactively resolve merge conflicts",
			},
		},
		Action: runSync,
	}
}

func runSync(ctx context.Context, c *cli.Command) error {
	r, err := repo.New()
	if err != nil {
		return err
	}
	if err := r.EnsureExists(); err != nil {
		return err
	}

	lk, err := lock.Acquire(r.Path())
	if err != nil {
		return err
	}
	defer lk.Release()

	interactive := c.Bool("interactive")
	hasRemote := git.HasRemote(ctx, r.Path())

	// Pull from remote if configured.
	if hasRemote {
		branch, err := git.CurrentBranch(ctx, r.Path())
		if err != nil {
			return fmt.Errorf("detect branch: %w", err)
		}

		// If the branch has no commits yet (unborn), commit staged changes
		// first so pull --rebase can work.
		if !git.HasCommits(ctx, r.Path()) {
			if err := git.AddAll(ctx, r.Path()); err != nil {
				return fmt.Errorf("git add: %w", err)
			}
			if err := git.Commit(ctx, r.Path(), "Initialize dotfather repository"); err != nil {
				return fmt.Errorf("initial commit: %w", err)
			}
			fmt.Println("Created initial commit.")
		}

		// Stash local changes so pull --rebase can proceed.
		stashed, stashErr := git.Stash(ctx, r.Path())
		if stashErr != nil {
			return fmt.Errorf("git stash: %w", stashErr)
		}

		// Snapshot .age file hashes and detect local plaintext edits before pull.
		preHashes := hashAgeFiles(r)
		localEdits := detectLocalEdits(r)

		fmt.Printf("Pulling from origin/%s...\n", branch)
		_, pullErr := git.Pull(ctx, r.Path(), branch)

		if pullErr != nil {
			// Check if it's a conflict.
			conflicted, _ := git.ConflictedFiles(ctx, r.Path())
			if len(conflicted) > 0 {
				if interactive {
					if err := resolveConflicts(ctx, r, conflicted, os.Stdin); err != nil {
						return err
					}
				} else {
					fmt.Fprintf(os.Stderr, "Merge conflicts in:\n")
					for _, f := range conflicted {
						fmt.Fprintf(os.Stderr, "  %s\n", f)
					}
					fmt.Fprintf(os.Stderr, "\nResolve manually in %s or re-run with --interactive\n",
						pathutil.TildePath(r.Path()))
					return cli.Exit("", 2)
				}
			} else {
				return fmt.Errorf("git pull: %w", pullErr)
			}
		}

		// Detect encrypted files that changed on remote while local target has edits.
		postHashes := hashAgeFiles(r)
		encConflicts := detectEncryptedConflicts(r, preHashes, localEdits, postHashes)

		// Reconcile symlinks after pull.
		if err := reconcileSymlinks(r); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: symlink reconciliation: %v\n", err)
		}

		// Decrypt encrypted files after pull, handling conflicts.
		if err := decryptEncryptedFiles(r, encConflicts, preHashes, postHashes); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: decrypt after pull: %v\n", err)
		}

		// Restore stashed changes.
		if stashed {
			if err := git.StashPop(ctx, r.Path()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: git stash pop failed: %v\n", err)
				fmt.Fprintf(os.Stderr, "  Your changes are still in the stash. Run: git -C %s stash pop\n",
					pathutil.TildePath(r.Path()))
			}
		}
	} else {
		fmt.Println("No remote configured. Use 'git -C " + pathutil.TildePath(r.Path()) + " remote add origin <url>' to set one.")
	}

	// Re-encrypt changed target files before commit.
	if err := reencryptChangedFiles(r); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: re-encrypt: %v\n", err)
	}

	// Check for uncommitted changes.
	porcelain, err := git.Status(ctx, r.Path())
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if strings.TrimSpace(porcelain) == "" {
		fmt.Println("Already up to date.")
		return nil
	}

	// Stage all changes.
	if err := git.AddAll(ctx, r.Path()); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Re-check status after staging.
	porcelain, err = git.Status(ctx, r.Path())
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if strings.TrimSpace(porcelain) == "" {
		fmt.Println("Already up to date.")
		return nil
	}

	// Generate commit message.
	message := generateCommitMessage(porcelain)

	// Commit.
	if err := git.Commit(ctx, r.Path(), message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	fmt.Printf("Committed: %s\n", message)

	// Push if remote configured.
	if hasRemote {
		branch, _ := git.CurrentBranch(ctx, r.Path())
		fmt.Printf("Pushing to origin/%s...\n", branch)
		if err := git.Push(ctx, r.Path(), branch); err != nil {
			return fmt.Errorf("git push failed: %w\nPush manually with: git -C %s push",
				err, pathutil.TildePath(r.Path()))
		}
		fmt.Println("Pushed successfully.")
	}

	return nil
}

func generateCommitMessage(porcelain string) string {
	trimmed := strings.TrimRight(porcelain, "\n")
	if trimmed == "" {
		return "Update dotfiles"
	}
	lines := strings.Split(trimmed, "\n")

	var added, modified, deleted []string
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		file := strings.TrimSpace(line[3:])

		switch {
		case status == "??" || status[0] == 'A':
			added = append(added, file)
		case status[0] == 'R' || status[0] == 'C':
			modified = append(modified, file)
		case status[0] == 'M' || status[1] == 'M':
			modified = append(modified, file)
		case status[0] == 'D' || status[1] == 'D':
			deleted = append(deleted, file)
		}
	}

	total := len(added) + len(modified) + len(deleted)
	if total == 0 {
		return "Update dotfiles"
	}

	if total <= 3 {
		var parts []string
		for _, f := range added {
			parts = append(parts, "Add "+f)
		}
		for _, f := range modified {
			parts = append(parts, "Update "+f)
		}
		for _, f := range deleted {
			parts = append(parts, "Remove "+f)
		}
		return strings.Join(parts, ", ")
	}

	return fmt.Sprintf("Update %d dotfiles (%s)", total, time.Now().Format("2006-01-02 15:04"))
}

func resolveConflicts(ctx context.Context, r *repo.Repo, conflicted []string, input io.Reader) error {
	reader := bufio.NewReader(input)

	for _, file := range conflicted {
		absFile := filepath.Join(r.Path(), file)
		fmt.Printf("\nConflict in %s:\n", file)
		fmt.Printf("  [l] Accept local  [r] Accept remote  [m] Merge in editor\n")
		fmt.Printf("  Choice: ")

		line, err := reader.ReadString('\n')
		if err != nil {
			if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
			}
			return fmt.Errorf("read input: %w", err)
		}

		choice := strings.TrimSpace(strings.ToLower(line))

		// During rebase, git's ours/theirs semantics are swapped:
		// --theirs is the local commit being replayed,
		// --ours is the upstream (remote) branch being rebased onto.
		switch choice {
		case "l":
			if err := git.CheckoutTheirs(ctx, r.Path(), file); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("checkout local: %w", err)
			}
			if err := git.Add(ctx, r.Path(), file); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("stage file: %w", err)
			}
			fmt.Printf("  Accepted local version of %s\n", file)

		case "r":
			if err := git.CheckoutOurs(ctx, r.Path(), file); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("checkout remote: %w", err)
			}
			if err := git.Add(ctx, r.Path(), file); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("stage file: %w", err)
			}
			fmt.Printf("  Accepted remote version of %s\n", file)

		case "m":
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			// Use sh -c to handle editors with flags (e.g., "zed --wait").
			cmd := exec.Command("sh", "-c", editor+" "+shellescape(absFile))
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			fmt.Printf("  Opening %s in %s...\n", file, editor)
			if err := cmd.Run(); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("editor: %w", err)
			}

			if err := git.Add(ctx, r.Path(), file); err != nil {
				if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
				}
				return fmt.Errorf("stage file: %w", err)
			}
			fmt.Printf("  Resolved %s\n", file)

		default:
			if abortErr := git.RebaseAbort(ctx, r.Path()); abortErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: rebase abort failed: %v\n", abortErr)
			}
			return fmt.Errorf("invalid choice: %s (expected l, r, or m)", choice)
		}
	}

	fmt.Println("\nAll conflicts resolved. Continuing rebase...")
	if err := git.RebaseContinue(ctx, r.Path()); err != nil {
		return fmt.Errorf("rebase continue: %w", err)
	}

	return nil
}

// shellescape wraps a path in single quotes for safe shell usage.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func reconcileSymlinks(r *repo.Repo) error {
	files, err := r.ManagedFiles()
	if err != nil {
		return err
	}

	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	for _, relPath := range files {
		// Skip encrypted files — handled by decrypt/re-encrypt functions.
		if crypto.IsEncrypted(relPath) {
			continue
		}

		repoFile := filepath.Join(r.Path(), relPath)
		targetPath := filepath.Join(home, relPath)

		state := linker.Check(repoFile, targetPath)
		switch state {
		case linker.OK:
			// Already correct.
		case linker.Missing:
			// New file from remote — create symlink.
			if err := linker.Link(repoFile, targetPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not link new file %s: %v\n",
					pathutil.TildePath(targetPath), err)
				continue
			}
			fmt.Printf("Linked new file: %s\n", pathutil.TildePath(targetPath))
		case linker.Broken:
			// Broken symlink — re-create.
			if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: could not remove broken symlink %s: %v\n",
					pathutil.TildePath(targetPath), err)
			}
			if err := linker.Link(repoFile, targetPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not re-link %s: %v\n",
					pathutil.TildePath(targetPath), err)
			}
		default:
			// Unlinked or conflict — don't touch, user should handle.
		}
	}

	return nil
}

// reencryptChangedFiles checks encrypted file targets and re-encrypts if modified.
// Note: uses mtime comparison which can cause unnecessary re-encryption after pull,
// but this errs on the side of safety (re-encrypting is idempotent, not data-losing).
func reencryptChangedFiles(r *repo.Repo) error {
	if !crypto.HasRecipient(r.Path()) || !crypto.HasIdentity(r.Path()) {
		return nil
	}

	files, err := r.ManagedFiles()
	if err != nil {
		return err
	}

	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	g := new(errgroup.Group)
	for _, relPath := range files {
		if !crypto.IsEncrypted(relPath) {
			continue
		}

		g.Go(func() error {
			plaintextRel := crypto.PlaintextPath(relPath)
			targetPath := filepath.Join(home, plaintextRel)
			encFile := filepath.Join(r.Path(), relPath)

			targetInfo, err := os.Stat(targetPath)
			if err != nil {
				return nil // Target doesn't exist — nothing to re-encrypt.
			}

			encInfo, err := os.Stat(encFile)
			if err != nil {
				return nil
			}

			if targetInfo.ModTime().After(encInfo.ModTime()) {
				if err := crypto.EncryptFile(r.Path(), targetPath, encFile); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not re-encrypt %s: %v\n",
						pathutil.TildePath(targetPath), err)
					return err
				}
				fmt.Printf("Re-encrypted %s\n", pathutil.TildePath(targetPath))
			}
			return nil
		})
	}

	return g.Wait()
}

// hashAgeFiles computes SHA-256 hashes of all .age files in the repo.
func hashAgeFiles(r *repo.Repo) map[string]string {
	files, err := r.ManagedFiles()
	if err != nil {
		return nil
	}
	hashes := make(map[string]string)
	for _, relPath := range files {
		if !crypto.IsEncrypted(relPath) {
			continue
		}
		absPath := filepath.Join(r.Path(), relPath)
		h, err := fileutil.FileHash(absPath)
		if err != nil {
			continue
		}
		hashes[relPath] = h
	}
	return hashes
}

// detectLocalEdits finds .age files whose corresponding plaintext target has
// been modified since the last sync. It decrypts each .age file in memory and
// compares the result to the current plaintext content.
func detectLocalEdits(r *repo.Repo) map[string]bool {
	if !crypto.HasIdentity(r.Path()) {
		return nil
	}
	files, err := r.ManagedFiles()
	if err != nil {
		return nil
	}
	home, err := pathutil.HomeDir()
	if err != nil {
		return nil
	}
	edits := make(map[string]bool)
	for _, relPath := range files {
		if !crypto.IsEncrypted(relPath) {
			continue
		}
		encFile := filepath.Join(r.Path(), relPath)
		plaintextRel := crypto.PlaintextPath(relPath)
		targetPath := filepath.Join(home, plaintextRel)

		targetHash, err := fileutil.FileHash(targetPath)
		if err != nil {
			continue // target doesn't exist
		}

		decrypted, err := crypto.DecryptToBytes(r.Path(), encFile)
		if err != nil {
			continue
		}
		decryptedHash := fileutil.BytesHash(decrypted)

		if targetHash != decryptedHash {
			edits[relPath] = true
		}
	}
	return edits
}

// detectEncryptedConflicts finds .age files that changed on remote while the
// local plaintext target has been modified. localEdits contains files where the
// plaintext differs from the decrypted .age content (detected before pull).
// For each conflict, the local plaintext is saved as a backup so the remote
// version can be decrypted.
func detectEncryptedConflicts(r *repo.Repo, preAgeHashes map[string]string, localEdits map[string]bool, postAgeHashes map[string]string) map[string]bool {
	if len(preAgeHashes) == 0 {
		return nil
	}

	home, err := pathutil.HomeDir()
	if err != nil {
		return nil
	}

	conflicts := make(map[string]bool)

	for relPath, postHash := range postAgeHashes {
		preHash, existed := preAgeHashes[relPath]
		if !existed || preHash == postHash {
			continue // new file or unchanged on remote
		}

		// Remote changed this .age file. Did local plaintext also have edits?
		if !localEdits[relPath] {
			continue
		}

		plaintextRel := crypto.PlaintextPath(relPath)
		targetPath := filepath.Join(home, plaintextRel)

		// Save local version before remote overwrites it.
		backupPath := targetPath + ".dotfather-local"
		if cpErr := linker.CopyFile(targetPath, backupPath); cpErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save local version of %s: %v\n",
				pathutil.TildePath(targetPath), cpErr)
		} else {
			fmt.Fprintf(os.Stderr, "Conflict: %s changed both locally and remotely\n", relPath)
			fmt.Fprintf(os.Stderr, "  Local edits saved to %s\n", pathutil.TildePath(backupPath))
			fmt.Fprintf(os.Stderr, "  Remote version will be decrypted\n")
		}
		conflicts[relPath] = true
	}

	return conflicts
}

// decryptEncryptedFiles decrypts all .age files in the repo to their target paths.
// Files in the conflicted set are always decrypted (remote wins, local was backed up).
// Non-conflicting files are skipped if their .age hash is unchanged from pre-pull
// (no new content from remote). Uses content hashing instead of mtime for reliability.
func decryptEncryptedFiles(r *repo.Repo, conflicted map[string]bool, preAgeHashes map[string]string, postAgeHashes map[string]string) error {
	if !crypto.HasIdentity(r.Path()) {
		return nil
	}

	files, err := r.ManagedFiles()
	if err != nil {
		return err
	}

	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	var firstErr error
	for _, relPath := range files {
		if !crypto.IsEncrypted(relPath) {
			continue
		}

		plaintextRel := crypto.PlaintextPath(relPath)
		targetPath := filepath.Join(home, plaintextRel)
		encFile := filepath.Join(r.Path(), relPath)

		// For conflicting files, always decrypt (remote wins, local was backed up).
		// For non-conflicting files, skip if the .age file is unchanged on remote.
		if !conflicted[relPath] {
			preHash, existed := preAgeHashes[relPath]
			postHash := postAgeHashes[relPath]
			if existed && preHash == postHash {
				continue // .age unchanged on remote, preserve local plaintext
			}
		}

		if err := crypto.DecryptFile(r.Path(), encFile, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not decrypt %s: %v\n",
				pathutil.TildePath(targetPath), err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		fmt.Printf("Decrypted %s\n", pathutil.TildePath(targetPath))
	}

	return firstErr
}

package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/fileutil"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
	"github.com/volodymyrsmirnov/dotfather/internal/lock"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newAddCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Add files to dotfather management",
		ArgsUsage: "<path> [path...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "keep",
				Aliases: []string{"k"},
				Usage:   "Keep a .bak copy of the original file",
			},
			&cli.BoolFlag{
				Name:    "encrypt",
				Aliases: []string{"e"},
				Usage:   "Encrypt file with age (stored as .age, copied instead of symlinked)",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite existing files at target when re-linking",
			},
		},
		Action: runAdd,
	}
}

func runAdd(ctx context.Context, c *cli.Command) error {
	if c.NArg() == 0 {
		return fmt.Errorf("at least one path is required")
	}

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

	keep := c.Bool("keep")
	encrypt := c.Bool("encrypt")
	force := c.Bool("force")

	if encrypt && !crypto.HasRecipient(r.Path()) {
		return fmt.Errorf("no age recipient key found; run 'dotfather init' to generate keys")
	}

	var errors []error

	for i := 0; i < c.NArg(); i++ {
		arg := c.Args().Get(i)
		if encrypt {
			if err := addEncryptedPath(ctx, r, arg); err != nil {
				errors = append(errors, fmt.Errorf("%s: %w", arg, err))
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
			}
		} else {
			if err := addPath(ctx, r, arg, keep, force); err != nil {
				errors = append(errors, fmt.Errorf("%s: %w", arg, err))
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) failed", len(errors))
	}

	return nil
}

// addEncryptedPath encrypts a file and stores it as .age in the repo.
func addEncryptedPath(ctx context.Context, r *repo.Repo, path string) error {
	absPath, err := pathutil.NormalizePath(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	underHome, err := pathutil.IsUnderHome(absPath)
	if err != nil {
		return err
	}
	if !underHome {
		return fmt.Errorf("only files under your home directory can be managed")
	}

	if pathutil.IsUnderPath(absPath, r.Path()) {
		return fmt.Errorf("cannot add files inside the dotfather repo (%s)", pathutil.TildePath(r.Path()))
	}

	// Check if the file is already managed as a regular (symlinked) file.
	// If so, convert it to encrypted instead of adding from scratch.
	if repoPath, err := r.RepoPathFor(absPath); err == nil {
		if _, err := os.Stat(repoPath); err == nil {
			return convertToEncrypted(ctx, r, absPath, repoPath)
		}
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return addEncryptedDirectory(ctx, r, absPath)
	}

	// Follow symlinks to get real file.
	if info.Mode()&os.ModeSymlink != 0 {
		absPath, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			return fmt.Errorf("follow symlink: %w", err)
		}
	}

	return addEncryptedFile(ctx, r, absPath)
}

func addEncryptedFile(ctx context.Context, r *repo.Repo, absPath string) error {
	relPath, err := pathutil.RelToHome(absPath)
	if err != nil {
		return err
	}

	encRelPath := crypto.EncryptedPath(relPath)
	encRepoPath := filepath.Join(r.Path(), encRelPath)

	// Encrypt the file.
	if err := crypto.EncryptFile(r.Path(), absPath, encRepoPath); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	// Stage in git.
	if err := git.Add(ctx, r.Path(), encRelPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	fmt.Printf("Added (encrypted) %s\n", pathutil.TildePath(absPath))
	return nil
}

func convertToEncrypted(ctx context.Context, r *repo.Repo, absPath, repoPath string) error {
	relPath, err := pathutil.RelToHome(absPath)
	if err != nil {
		return err
	}
	encRelPath := crypto.EncryptedPath(relPath)
	encRepoPath := filepath.Join(r.Path(), encRelPath)

	// Copy repo file to a temp location, then replace the symlink atomically.
	// This avoids a crash window where neither the symlink nor the file exists.
	tmpFile := absPath + ".dotfather-tmp"
	if err := linker.CopyFile(repoPath, tmpFile); err != nil {
		return fmt.Errorf("copy to temp: %w", err)
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("remove symlink: %w", err)
	}
	if err := os.Rename(tmpFile, absPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename temp to target: %w", err)
	}

	// Encrypt the file.
	if err := crypto.EncryptFile(r.Path(), absPath, encRepoPath); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	// Remove unencrypted repo copy.
	if err := os.Remove(repoPath); err != nil {
		return fmt.Errorf("remove plaintext from repo: %w", err)
	}
	linker.CleanEmptyDirs(repoPath, r.Path())

	// Stage both changes.
	if err := git.Add(ctx, r.Path(), encRelPath); err != nil {
		return fmt.Errorf("git add encrypted: %w", err)
	}
	if err := git.Add(ctx, r.Path(), relPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not stage removal: %v\n", err)
	}

	fmt.Printf("Converted %s to encrypted\n", pathutil.TildePath(absPath))
	return nil
}

func addEncryptedDirectory(ctx context.Context, r *repo.Repo, dirPath string) error {
	var errors []error

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if pathutil.IsUnderPath(path, r.Path()) {
				return filepath.SkipDir
			}
			return nil
		}
		if err := addEncryptedPath(ctx, r, path); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", path, err))
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", pathutil.TildePath(path), err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) in directory failed", len(errors))
	}
	return nil
}

func addPath(ctx context.Context, r *repo.Repo, path string, keep, force bool) error {
	absPath, err := pathutil.NormalizePath(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	underHome, err := pathutil.IsUnderHome(absPath)
	if err != nil {
		return err
	}
	if !underHome {
		return fmt.Errorf("only files under your home directory can be managed")
	}

	if pathutil.IsUnderPath(absPath, r.Path()) {
		return fmt.Errorf("cannot add files inside the dotfather repo (%s)", pathutil.TildePath(r.Path()))
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// If it's a directory, recursively add all files.
	if info.IsDir() {
		return addDirectory(ctx, r, absPath, keep, force)
	}

	// If it's a symlink, check if it's already ours.
	if info.Mode()&os.ModeSymlink != 0 {
		repoPath, _ := r.RepoPathFor(absPath)
		if linker.IsOurSymlink(absPath, repoPath) {
			fmt.Printf("Already managed: %s\n", pathutil.TildePath(absPath))
			return nil
		}
		// Follow the symlink to get the real file.
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return fmt.Errorf("follow symlink: %w", err)
		}
		// Remove the existing symlink first, then treat the real file as source.
		if err := os.Remove(absPath); err != nil {
			return fmt.Errorf("remove existing symlink: %w", err)
		}
		// Copy the real file to where the symlink was, then proceed normally.
		if err := linker.CopyFile(realPath, absPath); err != nil {
			return fmt.Errorf("copy real file: %w", err)
		}
		fmt.Printf("Resolved existing symlink at %s\n", pathutil.TildePath(absPath))
	}

	return addFile(ctx, r, absPath, keep, force)
}

func addFile(ctx context.Context, r *repo.Repo, absPath string, keep, force bool) error {
	repoPath, err := r.RepoPathFor(absPath)
	if err != nil {
		return err
	}

	// Check if already in repo.
	if _, err := os.Stat(repoPath); err == nil {
		state := linker.Check(repoPath, absPath)
		switch state {
		case linker.OK:
			fmt.Printf("Already managed: %s\n", pathutil.TildePath(absPath))
			return nil
		case linker.Broken, linker.Missing:
			// Safe to re-link — no user data at risk.
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", pathutil.TildePath(absPath), err)
			}
			if err := linker.Link(repoPath, absPath); err != nil {
				return fmt.Errorf("create symlink: %w", err)
			}
			fmt.Printf("Re-linked %s\n", pathutil.TildePath(absPath))
			return nil
		case linker.Unlinked, linker.Conflict, linker.Inaccessible:
			if !force {
				return fmt.Errorf("file exists at %s (%s); use --force to overwrite",
					pathutil.TildePath(absPath), state.String())
			}
			if err := os.Remove(absPath); err != nil {
				return fmt.Errorf("remove existing file: %w", err)
			}
			if err := linker.Link(repoPath, absPath); err != nil {
				return fmt.Errorf("create symlink: %w", err)
			}
			fmt.Printf("Re-linked (forced) %s\n", pathutil.TildePath(absPath))
			return nil
		}
	}

	// Warn if adding a potentially sensitive file without encryption.
	if relPath, err := pathutil.RelToHome(absPath); err == nil && isSensitivePath(relPath) {
		fmt.Fprintf(os.Stderr, "Warning: %s looks like a sensitive file; consider using --encrypt\n",
			pathutil.TildePath(absPath))
	}

	if keep {
		// Copy to repo, create symlink at temp path, backup original, swap in symlink.
		if err := linker.CopyFile(absPath, repoPath); err != nil {
			return fmt.Errorf("copy to repo: %w", err)
		}
		tmpLink := absPath + ".dotfather-link"
		if err := linker.Link(repoPath, tmpLink); err != nil {
			_ = os.Remove(repoPath)
			return fmt.Errorf("create symlink: %w", err)
		}
		bakPath := fileutil.UniqueBackupPath(absPath, ".bak")
		if err := os.Rename(absPath, bakPath); err != nil {
			_ = os.Remove(tmpLink)
			_ = os.Remove(repoPath)
			return fmt.Errorf("create backup: %w", err)
		}
		if err := os.Rename(tmpLink, absPath); err != nil {
			_ = os.Rename(bakPath, absPath) // restore original
			_ = os.Remove(tmpLink)
			_ = os.Remove(repoPath)
			return fmt.Errorf("replace original with symlink: %w", err)
		}
		fmt.Printf("Added %s (backup at %s)\n", pathutil.TildePath(absPath), pathutil.TildePath(bakPath))
	} else {
		// Copy to repo, create symlink at temp path, atomically replace original.
		if err := linker.CopyFile(absPath, repoPath); err != nil {
			return fmt.Errorf("copy to repo: %w", err)
		}
		tmpLink := absPath + ".dotfather-link"
		if err := linker.Link(repoPath, tmpLink); err != nil {
			_ = os.Remove(repoPath)
			return fmt.Errorf("create symlink: %w", err)
		}
		if err := os.Rename(tmpLink, absPath); err != nil {
			_ = os.Remove(tmpLink)
			_ = os.Remove(repoPath)
			return fmt.Errorf("replace original with symlink: %w", err)
		}
		fmt.Printf("Added %s\n", pathutil.TildePath(absPath))
	}

	// Stage in git.
	relPath, _ := pathutil.RelToHome(absPath)
	return git.Add(ctx, r.Path(), relPath)
}

func addDirectory(ctx context.Context, r *repo.Repo, dirPath string, keep, force bool) error {
	var errors []error

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if pathutil.IsUnderPath(path, r.Path()) {
				return filepath.SkipDir
			}
			return nil
		}
		if err := addPath(ctx, r, path, keep, force); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", path, err))
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", pathutil.TildePath(path), err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) in directory failed", len(errors))
	}
	return nil
}

var sensitivePatterns = []string{
	".ssh/",
	".gnupg/",
	".aws/credentials",
	".kube/config",
	".netrc",
	".docker/config.json",
}

func isSensitivePath(relPath string) bool {
	for _, pattern := range sensitivePatterns {
		if strings.HasPrefix(relPath, pattern) || relPath == strings.TrimSuffix(pattern, "/") {
			return true
		}
	}
	return false
}

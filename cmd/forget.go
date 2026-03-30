package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
	"github.com/volodymyrsmirnov/dotfather/internal/lock"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newForgetCommand() *cli.Command {
	return &cli.Command{
		Name:      "forget",
		Usage:     "Remove files from dotfather management",
		ArgsUsage: "<path> [path...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite existing non-symlink files at target",
			},
		},
		Action: runForget,
	}
}

func runForget(ctx context.Context, c *cli.Command) error {
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

	force := c.Bool("force")
	var errors []error

	for i := 0; i < c.NArg(); i++ {
		arg := c.Args().Get(i)
		if err := forgetFile(ctx, r, arg, force); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", arg, err))
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) failed", len(errors))
	}

	return nil
}

func forgetFile(ctx context.Context, r *repo.Repo, path string, force bool) error {
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

	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return forgetDirectory(ctx, r, absPath, force)
	}

	return forgetSingleFile(ctx, r, absPath, force)
}

func forgetDirectory(ctx context.Context, r *repo.Repo, dirPath string, force bool) error {
	relDir, err := pathutil.RelToHome(dirPath)
	if err != nil {
		return err
	}

	files, err := r.ManagedFiles()
	if err != nil {
		return err
	}

	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	var errors []error
	found := false
	for _, relPath := range files {
		plaintextRel := relPath
		if crypto.IsEncrypted(relPath) {
			plaintextRel = crypto.PlaintextPath(relPath)
		}
		if !strings.HasPrefix(plaintextRel, relDir+"/") {
			continue
		}
		found = true
		targetPath := filepath.Join(home, plaintextRel)
		if err := forgetSingleFile(ctx, r, targetPath, force); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", pathutil.TildePath(targetPath), err))
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", pathutil.TildePath(targetPath), err)
		}
	}

	if !found {
		return fmt.Errorf("no managed files found under %s", pathutil.TildePath(dirPath))
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) failed", len(errors))
	}
	return nil
}

func forgetSingleFile(ctx context.Context, r *repo.Repo, absPath string, force bool) error {
	relPath, err := pathutil.RelToHome(absPath)
	if err != nil {
		return err
	}

	repoFile, err := r.RepoPathFor(absPath)
	if err != nil {
		return err
	}

	// Check file exists in repo — try plain path first, then encrypted.
	encRelPath := crypto.EncryptedPath(relPath)
	encRepoFile := filepath.Join(r.Path(), encRelPath)

	isEncrypted := false
	if _, err := os.Stat(repoFile); os.IsNotExist(err) {
		// Try encrypted variant.
		if _, err := os.Stat(encRepoFile); os.IsNotExist(err) {
			return fmt.Errorf("not managed by dotfather: %s", pathutil.TildePath(absPath))
		}
		isEncrypted = true
	}

	// Handle encrypted file forget.
	if isEncrypted {
		// Ensure target file exists before removing the .age source.
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			if err := crypto.DecryptFile(r.Path(), encRepoFile, absPath); err != nil {
				return fmt.Errorf("decrypt before forget: %w", err)
			}
			fmt.Printf("Decrypted %s before forgetting\n", pathutil.TildePath(absPath))
		}

		if err := os.Remove(encRepoFile); err != nil {
			return fmt.Errorf("remove from repo: %w", err)
		}
		if err := linker.CleanEmptyDirs(encRepoFile, r.Path()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		if err := git.Add(ctx, r.Path(), encRelPath); err != nil {
			return fmt.Errorf("git stage: %w", err)
		}
		fmt.Printf("Forgotten (encrypted) %s\n", pathutil.TildePath(absPath))
		return nil
	}

	// Handle the target location.
	info, err := os.Lstat(absPath)
	if err == nil {
		// Something exists at target.
		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink. Check if it's ours.
			linkTarget, _ := os.Readlink(absPath)
			if linkTarget == repoFile {
				// Our symlink — remove it.
				if err := os.Remove(absPath); err != nil {
					return fmt.Errorf("remove symlink: %w", err)
				}
			} else if force {
				if err := os.Remove(absPath); err != nil {
					return fmt.Errorf("remove existing symlink: %w", err)
				}
			} else {
				return fmt.Errorf("unexpected symlink at %s (points to %s, not our repo). Use --force to overwrite",
					pathutil.TildePath(absPath), linkTarget)
			}
		} else {
			// Regular file exists.
			if force {
				if err := os.Remove(absPath); err != nil {
					return fmt.Errorf("remove existing file: %w", err)
				}
			} else {
				return fmt.Errorf("unexpected file at %s (not a dotfather symlink). Use --force to overwrite",
					pathutil.TildePath(absPath))
			}
		}
	}

	// Copy file from repo to target location.
	if err := linker.CopyFile(repoFile, absPath); err != nil {
		return fmt.Errorf("restore file: %w", err)
	}

	// Remove from repo.
	if err := os.Remove(repoFile); err != nil {
		return fmt.Errorf("remove from repo: %w", err)
	}

	// Clean up empty parent directories in repo.
	if err := linker.CleanEmptyDirs(repoFile, r.Path()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	// Stage the deletion in git.
	if err := git.Add(ctx, r.Path(), relPath); err != nil {
		return fmt.Errorf("git stage: %w", err)
	}

	fmt.Printf("Forgotten %s\n", pathutil.TildePath(absPath))
	return nil
}

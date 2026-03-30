package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
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

func runForget(_ context.Context, c *cli.Command) error {
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

	force := c.Bool("force")
	var errors []error

	for i := 0; i < c.NArg(); i++ {
		arg := c.Args().Get(i)
		if err := forgetFile(r, arg, force); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", arg, err))
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d file(s) failed", len(errors))
	}

	return nil
}

func forgetFile(r *repo.Repo, path string, force bool) error {
	absPath, err := pathutil.NormalizePath(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

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
		if err := os.Remove(encRepoFile); err != nil {
			return fmt.Errorf("remove from repo: %w", err)
		}
		if err := linker.CleanEmptyDirs(encRepoFile, r.Path()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		if err := git.Add(r.Path(), encRelPath); err != nil {
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
	if err := git.Add(r.Path(), relPath); err != nil {
		return fmt.Errorf("git stage: %w", err)
	}

	fmt.Printf("Forgotten %s\n", pathutil.TildePath(absPath))
	return nil
}

package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
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
		},
		Action: runAdd,
	}
}

func runAdd(_ context.Context, c *cli.Command) error {
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

	keep := c.Bool("keep")
	encrypt := c.Bool("encrypt")

	if encrypt && !crypto.HasRecipient(r.Path()) {
		return fmt.Errorf("no age recipient key found; run 'dotfather init' to generate keys")
	}

	var errors []error

	for i := 0; i < c.NArg(); i++ {
		arg := c.Args().Get(i)
		if encrypt {
			if err := addEncryptedPath(r, arg); err != nil {
				errors = append(errors, fmt.Errorf("%s: %w", arg, err))
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
			}
		} else {
			if err := addPath(r, arg, keep); err != nil {
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
func addEncryptedPath(r *repo.Repo, path string) error {
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

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return addEncryptedDirectory(r, absPath)
	}

	// Follow symlinks to get real file.
	if info.Mode()&os.ModeSymlink != 0 {
		absPath, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			return fmt.Errorf("follow symlink: %w", err)
		}
	}

	return addEncryptedFile(r, absPath)
}

func addEncryptedFile(r *repo.Repo, absPath string) error {
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
	if err := git.Add(r.Path(), encRelPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	fmt.Printf("Added (encrypted) %s\n", pathutil.TildePath(absPath))
	return nil
}

func addEncryptedDirectory(r *repo.Repo, dirPath string) error {
	var errors []error

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if err := addEncryptedFile(r, path); err != nil {
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

func addPath(r *repo.Repo, path string, keep bool) error {
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

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// If it's a directory, recursively add all files.
	if info.IsDir() {
		return addDirectory(r, absPath, keep)
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

	return addFile(r, absPath, keep)
}

func addFile(r *repo.Repo, absPath string, keep bool) error {
	repoPath, err := r.RepoPathFor(absPath)
	if err != nil {
		return err
	}

	// Check if already in repo.
	if _, err := os.Stat(repoPath); err == nil {
		// File exists in repo. Check if symlink is correct.
		if linker.Check(repoPath, absPath) == linker.OK {
			fmt.Printf("Already managed: %s\n", pathutil.TildePath(absPath))
			return nil
		}
		// File in repo but symlink broken — re-create it.
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", pathutil.TildePath(absPath), err)
		}
		if err := linker.Link(repoPath, absPath); err != nil {
			return fmt.Errorf("create symlink: %w", err)
		}
		fmt.Printf("Re-linked %s\n", pathutil.TildePath(absPath))
		return nil
	}

	if keep {
		// Copy to repo, rename original to .bak, create symlink.
		if err := linker.CopyFile(absPath, repoPath); err != nil {
			return fmt.Errorf("copy to repo: %w", err)
		}
		bakPath := absPath + ".bak"
		if err := os.Rename(absPath, bakPath); err != nil {
			return fmt.Errorf("create backup: %w", err)
		}
		if err := linker.Link(repoPath, absPath); err != nil {
			return fmt.Errorf("create symlink: %w", err)
		}
		fmt.Printf("Added %s (backup at %s)\n", pathutil.TildePath(absPath), pathutil.TildePath(bakPath))
	} else {
		// Move to repo, create symlink.
		if err := linker.MoveFile(absPath, repoPath); err != nil {
			return fmt.Errorf("move to repo: %w", err)
		}
		if err := linker.Link(repoPath, absPath); err != nil {
			return fmt.Errorf("create symlink: %w", err)
		}
		fmt.Printf("Added %s\n", pathutil.TildePath(absPath))
	}

	// Stage in git.
	relPath, _ := pathutil.RelToHome(absPath)
	return git.Add(r.Path(), relPath)
}

func addDirectory(r *repo.Repo, dirPath string, keep bool) error {
	var errors []error

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if err := addFile(r, path, keep); err != nil {
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

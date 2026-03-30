package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/fileutil"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/linker"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newInitCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Initialize a new dotfather repository",
		ArgsUsage: "[url]",
		Action:    runInit,
	}
}

func runInit(ctx context.Context, c *cli.Command) error {
	if !git.GitAvailable() {
		return fmt.Errorf("git is required but not found in PATH")
	}

	r, err := repo.New()
	if err != nil {
		return err
	}

	// If repo already exists and is valid, be idempotent.
	if r.IsGitRepo() {
		fmt.Printf("Dotfather repository already exists at %s\n", pathutil.TildePath(r.Path()))
		return nil
	}

	// If directory exists but isn't a git repo, error.
	if r.Exists() {
		return fmt.Errorf("%s exists but is not a git repository", r.Path())
	}

	url := c.Args().First()

	if url == "" {
		return initFresh(ctx, r)
	}

	return initFromClone(ctx, r, url)
}

func initFresh(ctx context.Context, r *repo.Repo) error {
	if err := os.MkdirAll(r.Path(), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := git.Init(ctx, r.Path()); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Generate age keypair.
	if err := crypto.GenerateKey(r.Path()); err != nil {
		return fmt.Errorf("generate age key: %w", err)
	}

	// Create .gitignore (excludes .age-identity).
	if err := r.WriteGitignore(); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}

	// Create README.md.
	if err := r.WriteREADME(); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	// Stage meta files and create initial commit.
	if err := git.Add(ctx, r.Path(), ".gitignore", crypto.RecipientFile, "README.md"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := git.Commit(ctx, r.Path(), "Initialize dotfather repository"); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	fmt.Printf("Initialized dotfather repository at %s\n", pathutil.TildePath(r.Path()))
	return nil
}

func initFromClone(ctx context.Context, r *repo.Repo, url string) error {
	if err := git.Clone(ctx, url, r.Path()); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	// Generate age key if not present.
	if !crypto.HasIdentity(r.Path()) {
		if crypto.HasRecipient(r.Path()) {
			// Repo has encrypted files but we don't have the key.
			fmt.Println("Warning: Encrypted files found but no age identity key.")
			fmt.Println("Copy your key from another machine:")
			fmt.Printf("  scp other-machine:%s %s\n",
				pathutil.TildePath(filepath.Join(r.Path(), crypto.IdentityFile)),
				pathutil.TildePath(filepath.Join(r.Path(), crypto.IdentityFile)))
			fmt.Println("Then run: dotfather sync")
		} else {
			// Fresh clone, no encryption set up yet — generate a key.
			if err := crypto.GenerateKey(r.Path()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not generate age key: %v\n", err)
			}
		}
	}

	// Ensure .gitignore exists.
	if _, err := os.Stat(filepath.Join(r.Path(), ".gitignore")); os.IsNotExist(err) {
		if err := r.WriteGitignore(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not write .gitignore: %v\n", err)
		}
	}

	files, err := r.ManagedFiles()
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	linked := 0
	decrypted := 0
	backups := 0
	hasEncrypted := false

	for _, relPath := range files {
		// Handle encrypted files.
		if crypto.IsEncrypted(relPath) {
			hasEncrypted = true
			if !crypto.HasIdentity(r.Path()) {
				continue // Skip — no key yet.
			}
			plaintextRel := crypto.PlaintextPath(relPath)
			targetPath, err := r.TargetPathFor(plaintextRel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", relPath, err)
				continue
			}
			encFile := filepath.Join(r.Path(), relPath)

			// Back up existing target before decrypting over it.
			if _, err := os.Lstat(targetPath); err == nil {
				backupPath := fileutil.UniqueBackupPath(targetPath, ".dotfather-backup")
				if err := os.Rename(targetPath, backupPath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not back up %s: %v\n",
						pathutil.TildePath(targetPath), err)
					continue
				}
				fmt.Printf("Backed up %s to %s\n",
					pathutil.TildePath(targetPath), pathutil.TildePath(backupPath))
				backups++
			}

			if err := crypto.DecryptFile(r.Path(), encFile, targetPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not decrypt %s: %v\n",
					pathutil.TildePath(targetPath), err)
				continue
			}
			fmt.Printf("Decrypted %s\n", pathutil.TildePath(targetPath))
			decrypted++
			continue
		}

		// Handle regular files — symlink them.
		targetPath, err := r.TargetPathFor(relPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", relPath, err)
			continue
		}

		repoFile := filepath.Join(r.Path(), relPath)

		if state := linker.Check(repoFile, targetPath); state == linker.OK {
			linked++
			continue
		}

		if _, err := os.Lstat(targetPath); err == nil {
			backupPath := fileutil.UniqueBackupPath(targetPath, ".dotfather-backup")
			if err := os.Rename(targetPath, backupPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not back up %s: %v\n",
					pathutil.TildePath(targetPath), err)
				continue
			}
			fmt.Printf("Backed up %s to %s\n",
				pathutil.TildePath(targetPath), pathutil.TildePath(backupPath))
			backups++
		}

		if err := linker.Link(repoFile, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not link %s: %v\n",
				pathutil.TildePath(targetPath), err)
			continue
		}
		linked++
	}

	parts := []string{fmt.Sprintf("linked %d files", linked)}
	if decrypted > 0 {
		parts = append(parts, fmt.Sprintf("%d decrypted", decrypted))
	}
	if backups > 0 {
		parts = append(parts, fmt.Sprintf("%d backups created", backups))
	}
	fmt.Printf("Cloned repository (%s)\n", strings.Join(parts, ", "))

	if hasEncrypted && !crypto.HasIdentity(r.Path()) {
		fmt.Println("\nEncrypted files were skipped — copy your age identity key to decrypt them.")
	}

	return nil
}

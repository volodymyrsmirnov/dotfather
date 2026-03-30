package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newListCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List all managed files",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "paths",
				Aliases: []string{"p"},
				Usage:   "Print absolute paths instead of ~/... paths",
			},
		},
		Action: runList,
	}
}

func runList(_ context.Context, c *cli.Command) error {
	r, err := repo.New()
	if err != nil {
		return err
	}
	if err := r.EnsureExists(); err != nil {
		return err
	}

	files, err := r.ManagedFiles()
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No managed files.")
		return nil
	}

	absPaths := c.Bool("paths")
	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	for _, relPath := range files {
		displayRel := relPath
		suffix := ""
		if crypto.IsEncrypted(relPath) {
			displayRel = crypto.PlaintextPath(relPath)
			suffix = " [encrypted]"
		}
		if absPaths {
			fmt.Printf("%s%s\n", filepath.Join(home, displayRel), suffix)
		} else {
			fmt.Printf("%s%s\n", filepath.Join("~", displayRel), suffix)
		}
	}

	return nil
}

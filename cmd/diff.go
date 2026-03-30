package cmd

import (
	"context"
	"fmt"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/repo"
)

func newDiffCommand() *cli.Command {
	return &cli.Command{
		Name:   "diff",
		Usage:  "Show uncommitted changes in the dotfiles repo",
		Action: runDiff,
	}
}

func runDiff(_ context.Context, _ *cli.Command) error {
	r, err := repo.New()
	if err != nil {
		return err
	}
	if err := r.EnsureExists(); err != nil {
		return err
	}

	unstaged, err := git.Diff(r.Path())
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	staged, err := git.DiffCached(r.Path())
	if err != nil {
		return fmt.Errorf("git diff --cached: %w", err)
	}

	combined := strings.TrimSpace(unstaged) + strings.TrimSpace(staged)
	if combined == "" {
		fmt.Println("No uncommitted changes.")
		return nil
	}

	if strings.TrimSpace(staged) != "" {
		fmt.Println(staged)
	}
	if strings.TrimSpace(unstaged) != "" {
		fmt.Println(unstaged)
	}

	return nil
}

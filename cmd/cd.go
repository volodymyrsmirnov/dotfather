package cmd

import (
	"context"
	"fmt"

	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/repo"
	"github.com/volodymyrsmirnov/dotfather/shellinit"
)

func newCDCommand() *cli.Command {
	return &cli.Command{
		Name:  "cd",
		Usage: "Print the dotfather repo path (use --shell-init for shell integration)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "shell-init",
				Usage: "Print shell init function for the given shell (bash, zsh, fish)",
			},
		},
		Action: runCD,
	}
}

func runCD(_ context.Context, c *cli.Command) error {
	shell := c.String("shell-init")
	if shell != "" {
		script, err := shellinit.ForShell(shell)
		if err != nil {
			return err
		}
		fmt.Print(script)
		return nil
	}

	r, err := repo.New()
	if err != nil {
		return err
	}
	if err := r.EnsureExists(); err != nil {
		return err
	}

	fmt.Println(r.Path())
	return nil
}

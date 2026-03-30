package cmd

import (
	cli "github.com/urfave/cli/v3"

	"github.com/volodymyrsmirnov/dotfather/internal/version"
)

// NewApp creates the root dotfather CLI command with all subcommands.
func NewApp() *cli.Command {
	return &cli.Command{
		Name:    "dotfather",
		Usage:   "Lightweight symlink-based dotfile manager",
		Version: version.String(),
		Commands: []*cli.Command{
			newInitCommand(),
			newAddCommand(),
			newForgetCommand(),
			newSyncCommand(),
			newStatusCommand(),
			newListCommand(),
			newDiffCommand(),
			newCDCommand(),
		},
	}
}

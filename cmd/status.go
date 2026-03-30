package cmd

import (
	"context"
	"encoding/json"
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

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show status of managed files",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Output as JSON",
			},
		},
		Action: runStatus,
	}
}

type fileStatus struct {
	Path  string `json:"path"`
	State string `json:"state"`
}

type statusOutput struct {
	Files       []fileStatus `json:"files"`
	Total       int          `json:"total"`
	OK          int          `json:"ok"`
	Broken      int          `json:"broken"`
	Missing     int          `json:"missing"`
	Unlinked    int          `json:"unlinked"`
	Conflict    int          `json:"conflict"`
	Uncommitted bool         `json:"uncommitted"`
	Ahead       int          `json:"ahead,omitempty"`
	Behind      int          `json:"behind,omitempty"`
}

func runStatus(_ context.Context, c *cli.Command) error {
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

	home, err := pathutil.HomeDir()
	if err != nil {
		return err
	}

	asJSON := c.Bool("json")

	output := statusOutput{}
	var statuses []linker.LinkStatus

	for _, relPath := range files {
		// Handle encrypted files differently.
		if crypto.IsEncrypted(relPath) {
			plaintextRel := crypto.PlaintextPath(relPath)
			targetPath := filepath.Join(home, plaintextRel)
			displayPath := pathutil.TildePath(targetPath)

			stateStr := "ENCRYPTED"
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				stateStr = "ENCRYPTED (missing)"
			}

			statuses = append(statuses, linker.LinkStatus{
				RepoPath:   filepath.Join(r.Path(), relPath),
				TargetPath: targetPath,
				RelPath:    plaintextRel,
				State:      linker.OK, // Count as OK for summary.
			})
			output.Files = append(output.Files, fileStatus{
				Path:  displayPath,
				State: stateStr,
			})
			output.Total++
			output.OK++
			continue
		}

		repoFile := filepath.Join(r.Path(), relPath)
		targetPath := filepath.Join(home, relPath)

		state := linker.Check(repoFile, targetPath)
		statuses = append(statuses, linker.LinkStatus{
			RepoPath:   repoFile,
			TargetPath: targetPath,
			RelPath:    relPath,
			State:      state,
		})

		output.Files = append(output.Files, fileStatus{
			Path:  pathutil.TildePath(targetPath),
			State: state.String(),
		})

		output.Total++
		switch state {
		case linker.OK:
			output.OK++
		case linker.Broken:
			output.Broken++
		case linker.Missing:
			output.Missing++
		case linker.Unlinked:
			output.Unlinked++
		case linker.Conflict:
			output.Conflict++
		}
	}

	// Git status.
	uncommitted, _ := git.HasUncommitted(r.Path())
	output.Uncommitted = uncommitted

	if git.HasRemote(r.Path()) {
		ahead, behind, err := git.AheadBehind(r.Path())
		if err == nil {
			output.Ahead = ahead
			output.Behind = behind
		}
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Text output.
	if len(statuses) == 0 {
		fmt.Println("No managed files.")
		return nil
	}

	// Find max path length for alignment.
	maxLen := 0
	for _, s := range statuses {
		tilde := pathutil.TildePath(s.TargetPath)
		if len(tilde) > maxLen {
			maxLen = len(tilde)
		}
	}

	for _, s := range statuses {
		tilde := pathutil.TildePath(s.TargetPath)
		stateStr := s.State.String()
		fmt.Printf("  %-*s  %s\n", maxLen, tilde, stateStr)
	}

	fmt.Println()

	// Summary.
	summary := fmt.Sprintf("%d files managed", output.Total)
	if output.OK > 0 {
		summary += fmt.Sprintf(", %d ok", output.OK)
	}
	if output.Broken > 0 {
		summary += fmt.Sprintf(", %d broken", output.Broken)
	}
	if output.Missing > 0 {
		summary += fmt.Sprintf(", %d missing", output.Missing)
	}
	if output.Unlinked > 0 {
		summary += fmt.Sprintf(", %d unlinked", output.Unlinked)
	}
	if output.Conflict > 0 {
		summary += fmt.Sprintf(", %d conflict", output.Conflict)
	}
	fmt.Println(summary)

	if output.Uncommitted {
		fmt.Println("Uncommitted changes present.")
	}
	if output.Ahead > 0 {
		fmt.Printf("Ahead of remote by %d commit(s).\n", output.Ahead)
	}
	if output.Behind > 0 {
		fmt.Printf("Behind remote by %d commit(s).\n", output.Behind)
	}

	return nil
}

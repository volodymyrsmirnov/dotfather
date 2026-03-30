package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cli "github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

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
	OK           int          `json:"ok"`
	Encrypted    int          `json:"encrypted"`
	Broken       int          `json:"broken"`
	Missing      int          `json:"missing"`
	Unlinked     int          `json:"unlinked"`
	Conflict     int          `json:"conflict"`
	Inaccessible int          `json:"inaccessible,omitempty"`
	Uncommitted bool         `json:"uncommitted"`
	Ahead       int          `json:"ahead,omitempty"`
	Behind      int          `json:"behind,omitempty"`
}

func runStatus(ctx context.Context, c *cli.Command) error {
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

	// Check file statuses in parallel (stat operations are I/O bound).
	results := make([]fileStatus, len(files))
	g, _ := errgroup.WithContext(ctx)

	for i, relPath := range files {
		i, relPath := i, relPath
		g.Go(func() error {
			if crypto.IsEncrypted(relPath) {
				plaintextRel := crypto.PlaintextPath(relPath)
				targetPath := filepath.Join(home, plaintextRel)
				stateStr := "ENCRYPTED"
				if _, err := os.Stat(targetPath); os.IsNotExist(err) {
					stateStr = "ENCRYPTED (missing)"
				}
				results[i] = fileStatus{
					Path:  pathutil.TildePath(targetPath),
					State: stateStr,
				}
				return nil
			}

			repoFile := filepath.Join(r.Path(), relPath)
			targetPath := filepath.Join(home, relPath)
			state := linker.Check(repoFile, targetPath)
			results[i] = fileStatus{
				Path:  pathutil.TildePath(targetPath),
				State: state.String(),
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Collect results and compute counters.
	output := statusOutput{}
	for _, f := range results {
		output.Files = append(output.Files, f)
		output.Total++
		switch f.State {
		case linker.OK.String():
			output.OK++
		case linker.Broken.String():
			output.Broken++
		case linker.Missing.String():
			output.Missing++
		case linker.Unlinked.String():
			output.Unlinked++
		case linker.Conflict.String():
			output.Conflict++
		case linker.Inaccessible.String():
			output.Inaccessible++
		case "ENCRYPTED", "ENCRYPTED (missing)":
			output.Encrypted++
		}
	}

	// Git status.
	uncommitted, _ := git.HasUncommitted(ctx, r.Path())
	output.Uncommitted = uncommitted

	if git.HasRemote(ctx, r.Path()) {
		ahead, behind, err := git.AheadBehind(ctx, r.Path())
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
	if len(output.Files) == 0 {
		fmt.Println("No managed files.")
		return nil
	}

	// Find max path length for alignment.
	maxLen := 0
	for _, f := range output.Files {
		if len(f.Path) > maxLen {
			maxLen = len(f.Path)
		}
	}

	for _, f := range output.Files {
		fmt.Printf("  %-*s  %s\n", maxLen, f.Path, f.State)
	}

	fmt.Println()

	// Summary.
	summary := fmt.Sprintf("%d files managed", output.Total)
	if output.OK > 0 {
		summary += fmt.Sprintf(", %d ok", output.OK)
	}
	if output.Encrypted > 0 {
		summary += fmt.Sprintf(", %d encrypted", output.Encrypted)
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
	if output.Inaccessible > 0 {
		summary += fmt.Sprintf(", %d inaccessible", output.Inaccessible)
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

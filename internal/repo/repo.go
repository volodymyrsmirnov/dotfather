package repo

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/volodymyrsmirnov/dotfather/internal/crypto"
	"github.com/volodymyrsmirnov/dotfather/internal/git"
	"github.com/volodymyrsmirnov/dotfather/internal/pathutil"
)

// metaFiles are repo files that should not be symlinked to ~.
var metaFiles = map[string]bool{
	"README.md":            true,
	".gitignore":           true,
	".dotfather-ignore":    true,
	crypto.RecipientFile:   true,
	crypto.IdentityFile:    true,
}

const defaultDirName = ".dotfather"

// Repo represents the dotfather repository.
type Repo struct {
	path           string
	extraMetaFiles map[string]bool
}

// New creates a Repo instance using $DOTFATHER_DIR or ~/.dotfather/.
func New() (*Repo, error) {
	dir := os.Getenv("DOTFATHER_DIR")
	if dir == "" {
		home, err := pathutil.HomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home: %w", err)
		}
		dir = filepath.Join(home, defaultDirName)
	}

	r := &Repo{path: dir}
	r.loadIgnoreFile()
	return r, nil
}

// Path returns the absolute path to the repository.
func (r *Repo) Path() string {
	return r.path
}

// Exists returns true if the repo directory exists.
func (r *Repo) Exists() bool {
	info, err := os.Stat(r.path)
	return err == nil && info.IsDir()
}

// IsGitRepo returns true if the repo is a valid git repository.
func (r *Repo) IsGitRepo() bool {
	return r.Exists() && git.IsGitRepo(context.Background(), r.path)
}

// EnsureExists returns an error if the repo doesn't exist or isn't a git repo.
func (r *Repo) EnsureExists() error {
	if !r.Exists() {
		return fmt.Errorf("no dotfather repository found; run 'dotfather init' first")
	}
	if !r.IsGitRepo() {
		return fmt.Errorf("%s exists but is not a git repository", r.path)
	}
	return nil
}

// IsMetaFile returns true if the repo-relative path is a meta file (not a dotfile).
func (r *Repo) IsMetaFile(relPath string) bool {
	if metaFiles[relPath] {
		return true
	}
	return r.extraMetaFiles[relPath]
}

func (r *Repo) loadIgnoreFile() {
	data, err := os.ReadFile(filepath.Join(r.path, ".dotfather-ignore"))
	if err != nil {
		return
	}
	r.extraMetaFiles = make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r.extraMetaFiles[line] = true
	}
}

// ManagedFiles returns all dotfiles in the repo (relative paths),
// excluding .git/ and meta files (README.md, .gitignore, .age-*).
func (r *Repo) ManagedFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(r.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(r.path, path)
			if err != nil {
				return err
			}
			if !r.IsMetaFile(rel) {
				files = append(files, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

// WriteREADME creates a README.md in the repo with setup instructions.
func (r *Repo) WriteREADME() error {
	originURL := "<your-dotfiles-repo-url>"
	if git.HasRemote(context.Background(), r.path) {
		if url, err := git.RemoteGetURL(context.Background(), r.path); err == nil {
			originURL = strings.TrimSpace(url)
		}
	}

	content := fmt.Sprintf(`# Dotfiles

Managed by [dotfather](https://github.com/volodymyrsmirnov/dotfather).

## Setup on a new machine

1. Install dotfather:

%s

2. Clone and set up dotfiles:

%s

3. All dotfiles will be automatically symlinked.

## Commands

%s
`,
		"   ```bash\n   brew install volodymyrsmirnov/tap/dotfather\n   ```",
		"   ```bash\n   dotfather init "+originURL+"\n   ```",
		"```bash\ndotfather add ~/.bashrc              # Add a file\ndotfather add --encrypt ~/.ssh/key   # Add encrypted\ndotfather sync                       # Pull + commit + push\ndotfather status                     # Check health\ndotfather forget ~/.bashrc           # Stop managing\n```",
	)

	return os.WriteFile(filepath.Join(r.path, "README.md"), []byte(content), 0644)
}

// WriteGitignore creates a .gitignore in the repo that excludes the age identity.
func (r *Repo) WriteGitignore() error {
	content := ".age-identity\n.lock\n"
	return os.WriteFile(filepath.Join(r.path, ".gitignore"), []byte(content), 0644)
}

// RepoPathFor converts an absolute home-relative path to its repo path.
// e.g., /Users/vol/.bashrc -> /Users/vol/.dotfather/.bashrc
func (r *Repo) RepoPathFor(absPath string) (string, error) {
	rel, err := pathutil.RelToHome(absPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(r.path, rel), nil
}

// TargetPathFor converts a repo-relative path to its home directory target.
// e.g., .bashrc -> /Users/vol/.bashrc
func (r *Repo) TargetPathFor(relPath string) (string, error) {
	home, err := pathutil.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, relPath), nil
}

// IsManaged checks if a file (by absolute path) is already managed.
func (r *Repo) IsManaged(absPath string) (bool, error) {
	repoPath, err := r.RepoPathFor(absPath)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

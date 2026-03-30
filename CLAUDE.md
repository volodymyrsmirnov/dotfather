# dotfather

Lightweight symlink-based dotfile manager written in Go.

## Project overview

dotfather manages dotfiles by mirroring the home directory structure inside a git repository (`~/.dotfather/`). Original file locations become symlinks pointing into the repo. The tool handles git operations (pull, commit, push) automatically via `dotfather sync`.

Key design principle: **no manifest file**. The repo directory structure IS the configuration. A file at `~/.dotfather/.config/nvim/init.lua` is symlinked to `~/.config/nvim/init.lua`.

## Package structure

```
main.go                          # Entry point, calls cmd.NewApp().Run()
cmd/
  app.go                         # NewApp() - assembles CLI with all subcommands
  init.go                        # dotfather init [url]
  add.go                         # dotfather add <path> [--keep]
  forget.go                      # dotfather forget <path> [--force]
  sync.go                        # dotfather sync [--interactive]
  status.go                      # dotfather status [--json]
  list.go                        # dotfather list [--paths]
  diff.go                        # dotfather diff
  cd.go                          # dotfather cd [--shell-init <shell>]
internal/
  pathutil/pathutil.go           # HomeDir, ExpandPath, NormalizePath, RelToHome, IsUnderHome, TildePath
  git/git.go                     # Thin wrappers around exec.Command("git", ...) - Init, Clone, Add, Commit, Pull, Push, Status, etc.
  repo/repo.go                   # Repo struct - path resolution, ManagedFiles() (excludes meta files), WriteREADME(), WriteGitignore()
  linker/linker.go               # Symlink engine - Link, Unlink, Check, MoveFile, CopyFile, CleanEmptyDirs
  crypto/crypto.go               # age encryption - GenerateKey, EncryptFile, DecryptFile, IsEncrypted, PlaintextPath
  version/version.go             # Build-time version injection (Version, Commit, Date)
shellinit/shellinit.go           # Shell wrapper functions for bash/zsh/fish
testutil/testutil.go             # Test helpers: SetupTestHome, CreateFile, InitGitRepo
```

## Key types

- `repo.Repo` - Represents the dotfather repository. Created via `repo.New()`. Core methods: `Path()`, `Exists()`, `IsGitRepo()`, `EnsureExists()`, `ManagedFiles()`, `RepoPathFor()`, `TargetPathFor()`, `IsManaged()`.
- `linker.LinkState` - Enum: `OK`, `Broken`, `Missing`, `Unlinked`, `Conflict`.
- `linker.LinkStatus` - Struct with `RepoPath`, `TargetPath`, `RelPath`, `State`.
- `git.GitError` - Wraps failed git commands with `Command`, `Args`, `Stderr`, `ExitCode`.

## Data flow

### `dotfather add ~/.bashrc`
1. `pathutil.NormalizePath()` resolves to absolute path
2. `pathutil.IsUnderHome()` validates path is under $HOME
3. `repo.RepoPathFor()` computes repo destination
4. `linker.MoveFile()` moves file to repo (cross-device fallback: copy+remove)
5. `linker.Link()` creates symlink from original location to repo file
6. `git.Add()` stages the new file

### `dotfather sync`
1. `git.HasRemote()` checks for origin
2. `git.Pull()` with rebase for linear history
3. `reconcileSymlinks()` links new files, removes stale symlinks
4. `git.Status()` porcelain output parsed for commit message
5. `generateCommitMessage()` creates descriptive message
6. `git.Commit()` + `git.Push()`

### `dotfather init <url>`
1. `git.Clone()` clones to repo path
2. `repo.ManagedFiles()` walks repo tree
3. For each file: backup existing target, `linker.Link()` creates symlink

## Build and test

```bash
make build       # Build with version info injected via ldflags
make test        # Run tests with race detector and coverage
make fmt         # Format code
make fmt-check   # Check formatting
make lint        # Run vet + golangci-lint
make vulncheck   # Run govulncheck
```

## Key types

- `crypto.IdentityFile` / `crypto.RecipientFile` — filenames for age key storage in repo
- `crypto.EncryptedExt` — `.age` extension used to detect encrypted files
- `repo.IsMetaFile()` — identifies repo files that should not be symlinked (README.md, .gitignore, .age-*)

## Encrypted files flow

- `add --encrypt` encrypts file with age, stores as `<path>.age` in repo, keeps original in place
- `sync` before commit: re-encrypts targets newer than their `.age` file
- `sync` after pull: decrypts all `.age` files to target paths
- `forget` on encrypted file: removes `.age` from repo, leaves target file
- Detection: any file ending in `.age` is treated as encrypted

## Dependencies

- `github.com/urfave/cli/v3` - CLI framework
- `filippo.io/age` - age encryption library (compiled in, no external age CLI needed)
- Git binary must be in PATH (all git operations shell out via `exec.Command`)

## Code style

- Imports grouped: stdlib, blank line, external deps, blank line, internal packages
- Error messages: lowercase, no trailing punctuation
- Functions in cmd/ files are thin: validate input, call internal/, format output
- All path handling goes through `internal/pathutil`
- All git operations go through `internal/git`
- No interactive prompts except `sync --interactive` conflict resolution

## Testing

- Tests use `testutil.SetupTestHome()` which creates a temp dir with resolved symlinks (macOS `/var` -> `/private/var`)
- `testutil.InitGitRepo()` creates a git repo with initial commit
- Integration tests in `cmd/integration_test.go` test full workflows end-to-end
- Unit tests alongside each package in `*_test.go` files
- Run with `go test ./...` or `make test`

## Conventions

- Business logic lives in `internal/`, CLI wiring in `cmd/`
- Exit codes: 0 (success), 1 (general error), 2 (merge conflict)
- Multi-path commands (`add`, `forget`) process all args, collect errors, report at end
- Symlinks are absolute (repo file path -> target)
- Version injected at build time via `-ldflags -X`
- `$DOTFATHER_DIR` env var overrides default `~/.dotfather/` location

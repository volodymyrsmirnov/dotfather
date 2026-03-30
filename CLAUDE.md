# dotfather

Lightweight symlink-based dotfile manager written in Go.

## Project overview

dotfather manages dotfiles by mirroring the home directory structure inside a git repository (`~/.dotfather/`). Regular files become symlinks pointing into the repo. Sensitive files can be added with `--encrypt` — they are stored as `.age` files in the repo and decrypted as regular copies (not symlinks) at target paths.

Key design principles:
- **No manifest file** — the repo directory structure IS the configuration
- **Meta files excluded** — `README.md`, `.gitignore`, `.age-recipients`, `.age-identity`, `.dotfather-ignore`, `.lock` live in the repo but are never symlinked
- **Custom ignore** — `.dotfather-ignore` file can list additional repo files to exclude from management (one per line, `#` comments)
- **Encrypted files are copies** — `.age` files in the repo are decrypted to target paths, not symlinked

## Package structure

```
main.go                          # Entry point, calls cmd.NewApp().Run()
cmd/
  app.go                         # NewApp() - assembles CLI with all subcommands
  init.go                        # dotfather init [url] - generates age keys, README, .gitignore
  add.go                         # dotfather add <path> [--keep] [--encrypt] [--force]
  forget.go                      # dotfather forget <path> [--force] - handles both regular and encrypted
  sync.go                        # dotfather sync [--interactive] - re-encrypts before commit, decrypts after pull
  status.go                      # dotfather status [--json] - shows ENCRYPTED state for .age files
  list.go                        # dotfather list [--paths] - shows [encrypted] suffix
  diff.go                        # dotfather diff
  cd.go                          # dotfather cd [--shell-init <shell>]
internal/
  pathutil/pathutil.go           # HomeDir, ExpandPath, NormalizePath, RelToHome, IsUnderHome, IsUnderPath, TildePath
  git/git.go                     # Thin wrappers around exec.Command("git", ...) - Init, Clone, Add, Commit, Pull, Push, Status, Diff, RemoteGetURL, etc.
  repo/repo.go                   # Repo struct - path resolution ($DOTFATHER_DIR or ~/.dotfather/), ManagedFiles() (filters meta files), WriteREADME(), WriteGitignore(), IsMetaFile()
  fileutil/fileutil.go           # AtomicWriteFile, SafeWriteTarget, UniqueBackupPath, FileHash, BytesHash
  lock/lock.go                   # File-based lock (Acquire/Release) to prevent concurrent dotfather runs; stale PID detection
  linker/linker.go               # Symlink engine - Link, Check (6 states), IsOurSymlink, CopyFile, CleanEmptyDirs
  crypto/crypto.go               # age encryption - GenerateKey, EncryptFile, DecryptFile, HasRecipient, HasIdentity, IsEncrypted, PlaintextPath, EncryptedPath
  version/version.go             # Build-time version injection (Version, Commit, Date)
shellinit/shellinit.go           # Shell wrapper functions for bash/zsh/fish (makes `dotfather cd` change directory)
testutil/testutil.go             # Test helpers: SetupTestHome (resolves macOS symlinks), CreateFile, InitGitRepo, GitCommitAll
```

## Key types

- `repo.Repo` — represents the dotfather repository. Created via `repo.New()`. Core methods: `Path()`, `Exists()`, `IsGitRepo()`, `EnsureExists()`, `ManagedFiles()`, `RepoPathFor()`, `TargetPathFor()`, `IsManaged()`, `WriteREADME()`, `WriteGitignore()`
- `repo.Repo.IsMetaFile(relPath)` — method, returns true for repo files that should not be symlinked (includes hardcoded meta files + `.dotfather-ignore` entries)
- `linker.LinkState` — enum: `OK`, `Broken`, `Missing`, `Unlinked`, `Conflict`, `Inaccessible`
- `lock.Lock` — file-based mutual exclusion lock with `Acquire(dir)` and `Release()`
- `git.GitError` — wraps failed git commands: `Command`, `Args`, `Stderr`, `ExitCode`
- `crypto.IdentityFile` / `crypto.RecipientFile` — constants for age key filenames in repo
- `crypto.EncryptedExt` — `.age` extension constant

## Data flow

### `dotfather init`
1. Create `~/.dotfather/`, run `git init`
2. `crypto.GenerateKey()` writes `.age-identity` (0600) + `.age-recipients`
3. `repo.WriteGitignore()` creates `.gitignore` (excludes `.age-identity` and `.lock`)
4. `repo.WriteREADME()` creates README.md (includes origin URL if available)
5. Stage meta files

### `dotfather init <url>`
1. `git.Clone()` clones to repo path
2. If `.age-recipients` exists but `.age-identity` missing → print key copy instructions
3. `repo.ReloadIgnoreFile()` re-reads `.dotfather-ignore` from cloned repo
4. `repo.ManagedFiles()` walks repo tree (excludes meta files)
5. For each regular file: backup existing target, create symlink
6. For each `.age` file: decrypt to target path (if identity available)

### `dotfather add ~/.bashrc`
1. `pathutil.NormalizePath()` resolves to absolute path
2. `pathutil.IsUnderHome()` validates path is under $HOME
3. `repo.RepoPathFor()` computes repo destination
4. `lock.Acquire()` prevents concurrent operations
5. `linker.CopyFile()` copies file to repo
6. `linker.Link()` creates symlink at temp path (`.dotfather-link`)
7. `os.Rename()` atomically replaces original with symlink
8. `git.Add()` stages the new file

### `dotfather add --encrypt ~/.ssh/id_rsa`
1. Resolve and validate path (same as regular add)
2. `crypto.EncryptFile()` reads recipient from `.age-recipients`, encrypts to `<path>.age` in repo
3. Original file stays in place (no symlink)
4. `git.Add()` stages the `.age` file

### `dotfather sync`
1. `lock.Acquire()` prevents concurrent operations
2. `git.Stash()` saves uncommitted changes before pull
3. `hashAgeFiles()` + `detectLocalEdits()` snapshot pre-pull state
4. `git.Pull()` with rebase for linear history
5. `detectEncryptedConflicts()` finds files changed both locally and remotely; saves local edits as `.dotfather-local` backups
6. `reconcileSymlinks()` links new regular files, re-creates broken symlinks (skips `.age` files)
7. `decryptEncryptedFiles()` decrypts changed `.age` files to target paths (skips unchanged)
8. `git.StashPop()` restores stashed changes
9. `reencryptChangedFiles()` re-encrypts targets newer than their `.age` file
10. `git.AddAll()` + `generateCommitMessage()` from porcelain output
11. `git.Commit()` + `git.Push()`

### `dotfather forget ~/.ssh/id_rsa`
1. `lock.Acquire()` prevents concurrent operations
2. Check both `<relPath>` and `<relPath>.age` in repo
3. If encrypted: decrypt to target if missing, remove `.age` file from repo
4. If regular: copy from repo to temp, replace symlink atomically, remove from repo

## Build and test

```bash
make build       # Build with version info injected via ldflags
make test        # Run tests with race detector and coverage
make fmt         # Format code
make fmt-check   # Check formatting
make lint        # Run vet + golangci-lint
make vulncheck   # Run govulncheck
```

## Dependencies

- `github.com/urfave/cli/v3` — CLI framework
- `filippo.io/age` — age encryption library (compiled in, no external age CLI needed); includes `agessh` for SSH key support as recipients/identities
- `golang.org/x/sync` — errgroup for parallel file operations (status checks, re-encryption)
- Git binary must be in PATH (all git operations shell out via `exec.Command`)

## Code style

- Imports grouped: stdlib, blank line, external deps, blank line, internal packages
- Error messages: lowercase, no trailing punctuation
- Functions in `cmd/` are thin: validate input, call `internal/`, format output
- All path handling goes through `internal/pathutil`
- All git operations go through `internal/git`
- All encryption through `internal/crypto`
- No interactive prompts except `sync --interactive` conflict resolution
- Filesystem errors (os.Remove, os.Symlink, etc.) are checked and reported as warnings when non-fatal

## Testing

- Tests use `testutil.SetupTestHome()` which creates a temp dir with resolved symlinks (macOS `/var` -> `/private/var`)
- `testutil.InitGitRepo()` creates a git repo with initial commit and git user config
- Integration tests in `cmd/integration_test.go` test full workflows end-to-end (init, add, forget, sync, status, list, diff, cd, clone with conflicts, encrypted files)
- Unit tests alongside each package in `*_test.go` files
- Crypto tests cover: key generation, encrypt/decrypt roundtrip, binary data, empty files, missing keys
- Run with `go test ./...` or `make test`
- Lint with `golangci-lint run ./...` or `make lint`

## Conventions

- Business logic in `internal/`, CLI wiring in `cmd/`
- Exit codes: 0 (success), 1 (general error), 2 (merge conflict)
- Multi-path commands (`add`, `forget`) process all args, collect errors, report at end
- Symlinks are absolute paths (repo file path → target)
- Encrypted files detected by `.age` extension — no separate tracking
- Version injected at build time via `-ldflags -X`
- `$DOTFATHER_DIR` env var overrides default `~/.dotfather/` location (resolved to absolute if relative)
- `$EDITOR` for conflict resolution, supports editors with flags (e.g., `zed --wait`) via `sh -c`

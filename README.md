# dotfather

Lightweight symlink-based dotfile manager. Like chezmoi, but without the complexity.

dotfather uses a simple approach: your dotfiles live in a git repository (`~/.dotfather/`), and their original locations become symlinks pointing into that repo. Sensitive files can be encrypted with [age](https://github.com/FiloSottile/age). One command to sync everything.

## Install

### Homebrew (macOS / Linux)

```bash
brew install volodymyrsmirnov/tap/dotfather
```

### Shell script

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/volodymyrsmirnov/dotfather/main/install.sh)"
```

### GitHub Releases

Download the latest binary from [Releases](https://github.com/volodymyrsmirnov/dotfather/releases).

### From source

```bash
go install github.com/volodymyrsmirnov/dotfather@latest
```

## Quick start

```bash
# Initialize a new dotfiles repo
dotfather init

# Or clone an existing one (auto-creates all symlinks)
dotfather init git@github.com:yourname/dotfiles.git

# Add files to management
dotfather add ~/.bashrc
dotfather add ~/.config/nvim/init.lua
dotfather add ~/.config/starship.toml

# Add an entire directory
dotfather add ~/.config/kitty

# Add sensitive files as encrypted
dotfather add --encrypt ~/.ssh/id_rsa
dotfather add --encrypt ~/.config/secrets/

# Check status
dotfather status

# Sync to remote (pull + commit + push)
dotfather sync

# Stop managing a file (restores the original)
dotfather forget ~/.bashrc
```

## How it works

dotfather mirrors your home directory structure inside `~/.dotfather/`:

```
~/.dotfather/                      <- git repo
├── README.md                      <- auto-generated setup instructions
├── .gitignore                     <- excludes age identity key
├── .age-recipients                <- age public key (committed)
├── .age-identity                  <- age private key (gitignored)
├── .bashrc                        <- real file (git-tracked)
├── .config/
│   ├── nvim/
│   │   └── init.lua
│   └── starship.toml
└── .ssh/
    ├── config
    └── id_rsa.age                 <- encrypted file (git-tracked)

~/                                 <- your home directory
├── .bashrc       -> ~/.dotfather/.bashrc             (symlink)
├── .config/
│   ├── nvim/
│   │   └── init.lua -> ~/.dotfather/.config/nvim/init.lua
│   └── starship.toml -> ~/.dotfather/.config/starship.toml
└── .ssh/
    ├── config    -> ~/.dotfather/.ssh/config          (symlink)
    └── id_rsa                                         (decrypted copy)
```

**Regular files** are symlinked — edits go directly to the repo.
**Encrypted files** (`.age`) are decrypted copies — re-encrypted automatically on `sync`.
**Meta files** (`README.md`, `.gitignore`, `.age-recipients`, `.age-identity`) stay in the repo only and are never symlinked.

No manifest files, no templates, no encoding. The directory structure IS the configuration.

## Commands

### `dotfather init [url]`

Initialize a new dotfather repository. Generates an age keypair, creates a README.md with setup instructions, and sets up `.gitignore`.

```bash
# Empty repo
dotfather init

# Clone existing dotfiles and set up all symlinks
dotfather init git@github.com:yourname/dotfiles.git
```

When cloning, existing files at target locations are backed up as `<file>.dotfather-backup`. Encrypted files are decrypted automatically if the age identity key is present.

### `dotfather add <path> [path...]`

Add files to dotfather management. Moves the file into the repo and replaces the original with a symlink.

```bash
dotfather add ~/.bashrc ~/.zshrc
dotfather add ~/.config/nvim       # adds all files in directory

# Keep a .bak copy of the original (safety net)
dotfather add --keep ~/.bashrc

# Add as encrypted (stored as .age, copied instead of symlinked)
dotfather add --encrypt ~/.ssh/id_rsa
dotfather add --encrypt ~/.config/secrets/
```

| Flag | Description |
|------|-------------|
| `--keep`, `-k` | Keep a `.bak` copy of the original file |
| `--encrypt`, `-e` | Encrypt with age (stored as `.age`, original stays in place) |

### `dotfather forget <path> [path...]`

Remove files from dotfather management. Copies the file back to its original location and removes it from the repo. Works for both regular and encrypted files.

```bash
dotfather forget ~/.bashrc
dotfather forget ~/.ssh/id_rsa     # removes .ssh/id_rsa.age from repo

# Overwrite existing non-symlink files at target
dotfather forget --force ~/.bashrc
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Overwrite existing non-symlink files at target |

### `dotfather sync`

Pull remote changes, commit local changes with auto-generated messages, and push.

For encrypted files: re-encrypts any targets that were modified locally before committing, then decrypts any updated `.age` files after pulling.

```bash
dotfather sync

# Interactively resolve merge conflicts
dotfather sync --interactive
```

Auto-generated commit messages:
- `Add .bashrc` (new file)
- `Update .config/nvim/init.lua` (modified file)
- `Add .bashrc, Update .zshrc` (2-3 files)
- `Update 7 dotfiles (2026-03-30 14:22)` (4+ files)

**Interactive conflict resolution** (`--interactive`, `-i`):

When merge conflicts occur, you're prompted for each file:
- `l` — Accept local version
- `r` — Accept remote version
- `m` — Open in `$EDITOR` for manual merge (supports editors with flags like `zed --wait`)

| Flag | Description |
|------|-------------|
| `--interactive`, `-i` | Interactively resolve merge conflicts |

### `dotfather status`

Show the health of all managed files and sync state.

```bash
dotfather status

# Output:
#   ~/.bashrc                OK
#   ~/.config/nvim/init.lua  OK
#   ~/.ssh/id_rsa            ENCRYPTED
#   ~/.config/old.yaml       BROKEN
#
# 4 files managed, 3 ok, 1 broken

# JSON output for scripting
dotfather status --json
```

States: `OK`, `BROKEN`, `MISSING`, `UNLINKED`, `CONFLICT`, `ENCRYPTED`, `ENCRYPTED (missing)`

| Flag | Description |
|------|-------------|
| `--json`, `-j` | Output as JSON |

### `dotfather list`

List all managed files. Encrypted files are shown with `[encrypted]` suffix.

```bash
dotfather list
# ~/.bashrc
# ~/.config/nvim/init.lua
# ~/.ssh/id_rsa [encrypted]

dotfather list --paths  # absolute paths
```

| Flag | Description |
|------|-------------|
| `--paths`, `-p` | Print absolute paths instead of `~/...` |

### `dotfather diff`

Show uncommitted changes in the dotfiles repo.

```bash
dotfather diff
```

### `dotfather cd`

Print the dotfather repo path, or set up shell integration.

```bash
# Print repo path
dotfather cd

# Set up shell integration (add to your rc file)
eval "$(dotfather cd --shell-init bash)"   # bash
eval "$(dotfather cd --shell-init zsh)"    # zsh
dotfather cd --shell-init fish | source    # fish
```

With shell integration, `dotfather cd` will change your shell's working directory to the repo.

## Encrypted files

Sensitive files (SSH keys, tokens, API credentials) can be stored encrypted in the repo using [age](https://github.com/FiloSottile/age) encryption. The age library is compiled into the dotfather binary — no external tools needed.

```bash
dotfather add --encrypt ~/.ssh/id_rsa
dotfather add --encrypt ~/.config/secrets/api_token
```

### How it works

- Encrypted files are stored as `<path>.age` in the repo (e.g., `.ssh/id_rsa.age`)
- The original file stays in place as a regular file (not a symlink)
- `dotfather sync` automatically:
  - **Re-encrypts** local files that changed since the last sync
  - **Decrypts** files updated from the remote after pulling

### Key management

Keys are generated automatically on `dotfather init`:

| File | Purpose | Git status |
|------|---------|------------|
| `.age-recipients` | Public key (used for encryption) | Committed |
| `.age-identity` | Private key (used for decryption) | Gitignored |

The public key is shared via the repo so any clone can encrypt new files. The private key must be transferred manually to new machines.

### New machine with encrypted files

```bash
# 1. Clone dotfiles (regular files are linked, encrypted files are skipped)
dotfather init git@github.com:yourname/dotfiles.git

# 2. Copy your age identity key from an existing machine
scp other-machine:~/.dotfather/.age-identity ~/.dotfather/.age-identity

# 3. Decrypt encrypted files
dotfather sync
```

> **Tip**: Install the `age` CLI (`brew install age`) if you want to manually inspect or decrypt files outside of dotfather.

## New machine setup

```bash
# 1. Install dotfather
brew install volodymyrsmirnov/tap/dotfather

# 2. Clone your dotfiles (auto-creates all symlinks)
dotfather init git@github.com:yourname/dotfiles.git

# 3. Done! All your dotfiles are linked.
dotfather status
```

Existing files are automatically backed up as `.dotfather-backup` during clone.

## Configuration

dotfather uses `~/.dotfather/` by default. Override with the `DOTFATHER_DIR` environment variable:

```bash
export DOTFATHER_DIR=~/my-dotfiles
```

## Typical workflow

```bash
# Edit dotfiles as usual (symlinks are transparent)
vim ~/.bashrc

# Periodically sync
dotfather sync
# -> Committed: Update .bashrc
# -> Pushed to origin/main
```

## License

MIT

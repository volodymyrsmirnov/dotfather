# dotfather

Lightweight symlink-based dotfile manager. Like chezmoi, but without the complexity.

dotfather uses a simple approach: your dotfiles live in a git repository (`~/.dotfather/`), and their original locations become symlinks pointing into that repo. One command to sync everything.

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
~/.dotfather/              <- git repo
├── .bashrc                <- real file (git-tracked)
├── .config/
│   ├── nvim/
│   │   └── init.lua
│   └── starship.toml
└── .ssh/
    └── config

~/                         <- your home directory
├── .bashrc        -> ~/.dotfather/.bashrc          (symlink)
├── .config/
│   ├── nvim/
│   │   └── init.lua -> ~/.dotfather/.config/nvim/init.lua
│   └── starship.toml -> ~/.dotfather/.config/starship.toml
└── .ssh/
    └── config     -> ~/.dotfather/.ssh/config
```

No manifest files, no templates, no encoding. The directory structure IS the configuration.

Encrypted files (added with `--encrypt`) are stored as `.age` files and decrypted on sync:
```
~/.dotfather/
└── .ssh/
    └── id_rsa.age      <- encrypted (git-tracked)

~/
└── .ssh/
    └── id_rsa          <- decrypted copy (not a symlink)
```

## Commands

### `dotfather init [url]`

Initialize a new dotfather repository.

```bash
# Empty repo
dotfather init

# Clone existing dotfiles and set up all symlinks
dotfather init git@github.com:yourname/dotfiles.git
```

When cloning, existing files at target locations are backed up as `<file>.dotfather-backup`.

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
| `--encrypt`, `-e` | Encrypt with age (stored as `.age`, copied not symlinked) |

### `dotfather forget <path> [path...]`

Remove files from dotfather management. Copies the file back to its original location and removes it from the repo.

```bash
dotfather forget ~/.bashrc

# Overwrite existing non-symlink files at target
dotfather forget --force ~/.bashrc
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Overwrite existing non-symlink files at target |

### `dotfather sync`

Pull remote changes, commit local changes with auto-generated messages, and push.

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
- `m` — Open in `$EDITOR` for manual merge

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
#   ~/.config/old.yaml       BROKEN
#
# 3 files managed, 2 ok, 1 broken

# JSON output for scripting
dotfather status --json
```

States: `OK`, `BROKEN`, `MISSING`, `UNLINKED`, `CONFLICT`

| Flag | Description |
|------|-------------|
| `--json`, `-j` | Output as JSON |

### `dotfather list`

List all managed files.

```bash
dotfather list        # ~/.bashrc, ~/.config/nvim/init.lua, ...
dotfather list --paths  # /home/user/.bashrc, /home/user/.config/nvim/init.lua, ...
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

Sensitive files (SSH keys, tokens, etc.) can be encrypted with [age](https://github.com/FiloSottile/age):

```bash
# Add encrypted files
dotfather add --encrypt ~/.ssh/id_rsa
dotfather add --encrypt ~/.config/secrets/api_token

# Sync works transparently — re-encrypts changed files, decrypts after pull
dotfather sync
```

Encryption is handled by the built-in age library (no external tools needed). Keys are generated automatically on `dotfather init`:
- **Public key** (`.age-recipients`): committed to the repo, used for encryption
- **Private key** (`.age-identity`): gitignored, must be copied manually to new machines

### New machine with encrypted files

After cloning, copy your age identity key from an existing machine:

```bash
scp other-machine:~/.dotfather/.age-identity ~/.dotfather/.age-identity
dotfather sync  # decrypts all encrypted files
```

> **Tip**: Install the `age` CLI (`brew install age`) if you want to manually inspect encrypted files outside of dotfather.

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

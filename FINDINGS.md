# Findings

## 1. High: `sync` can silently overwrite remote changes for encrypted files

- References: `cmd/sync.go:57`, `cmd/sync.go:94`, `cmd/sync.go:102`, `cmd/sync.go:352`, `cmd/sync.go:405`
- `dotfather sync` pulls first, decrypts pulled `.age` files, and only then re-encrypts locally edited plaintext targets back into the repo.
- Local edits to encrypted files live outside git until `reencryptChangedFiles()` runs. That means `git pull --rebase` cannot detect conflicts against those local plaintext edits.
- If the same encrypted file changed remotely and locally, the remote ciphertext is pulled first, then the newer local plaintext copy causes `reencryptChangedFiles()` to overwrite the pulled ciphertext with no merge conflict and no warning.
- Impact: remote changes to encrypted files can be lost silently.
- Recommendation: materialize local encrypted-file edits into the repo before any pull/rebase, or store and compare content hashes so encrypted targets participate in conflict detection.

## 2. High: cloned repo symlinks are trusted and can project arbitrary targets into `~`

- References: `internal/repo/repo.go:102`, `cmd/init.go:169`, `cmd/sync.go:302`, `internal/linker/linker.go:48`
- `ManagedFiles()` treats any non-directory entry in the repo as a managed file, including symlinks.
- During `init <url>` and later `sync`, regular managed entries are linked into the home directory without checking whether the repo entry itself is a symlink or whether it resolves outside the repo.
- A malicious or just careless repository can commit something like `.bashrc -> /etc/passwd` inside the repo. Dotfather will then create `~/.bashrc -> ~/.dotfather/.bashrc`, and that second hop resolves to `/etc/passwd`.
- Impact: untrusted repositories can cause home-directory paths to resolve to arbitrary filesystem locations.
- Recommendation: reject symlink entries in the repo, or resolve them and require the final target to stay inside the repo before linking anything into `~`.

## 3. Medium: decryption writes through existing symlinks at the target path

- References: `internal/crypto/crypto.go:103`, `internal/crypto/crypto.go:131`, `internal/crypto/crypto.go:135`, `cmd/init.go:146`, `cmd/sync.go:427`
- `DecryptFile()` uses `os.Stat()` and then `os.WriteFile()` on `dstFile`. Both follow symlinks.
- If the destination path for an encrypted file is already a symlink, decryption will write to the symlink target instead of replacing or rejecting the symlink.
- `init` backs up whatever exists at the target path, but `sync` does not, and neither path verifies that the destination is a plain file owned by dotfather.
- Impact: an unexpected symlink at a managed encrypted-file path can redirect decrypted secrets into another file.
- Recommendation: switch to `os.Lstat()` plus explicit symlink rejection, or replace the destination atomically with a regular file after validating it.

## 4. Medium: `add` has partial-failure data-loss windows when relinking fails

- References: `cmd/add.go:342`, `cmd/add.go:347`, `cmd/add.go:351`, `cmd/add.go:357`, `cmd/add.go:360`
- In `--keep` mode, the command copies the file into the repo, renames the original to `.bak`, and only then creates the symlink.
- In normal mode, it moves the original into the repo and only then creates the symlink.
- If `linker.Link()` fails after the move/rename, the command returns an error but leaves the user's original path missing or renamed.
- Impact: the operation is not rollback-safe; a transient filesystem error leaves the repo updated but the original location broken.
- Recommendation: create the replacement symlink via a temp path and rename it into place, or roll back the move/rename when symlink creation fails.

## 5. Low: backup names are fixed and can overwrite older backups

- References: `cmd/init.go:148`, `cmd/init.go:184`, `cmd/add.go:347`
- Clone/setup backups always use `<path>.dotfather-backup`, and `add --keep` always uses `<path>.bak`.
- On Unix, `os.Rename()` replaces an existing destination file, so an older backup at the same path can be silently clobbered.
- Impact: repeated recovery/setup attempts can destroy the only earlier backup copy.
- Recommendation: fail if the backup path already exists, or generate unique names with a timestamp/random suffix.

## Suggested test gaps

- Add an integration test that reproduces concurrent local+remote edits to the same encrypted file during `sync`.
- Add repository-loading tests that ensure repo-side symlinks are rejected.
- Add decryption tests that confirm destination symlinks are refused rather than followed.

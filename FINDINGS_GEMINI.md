# dotfather - Code Review Findings

## Overview
`dotfather` is a robust and well-structured dotfile manager. It leverages modern encryption (`age`) and symlink-based management. The code is modular and generally follows Go best practices. However, there are several areas related to atomicity, edge-case handling, and missing features that could be improved.

---

## 1. Security Findings

### 1.1 Positive Security Practices
- **Encryption:** Uses `filippo.io/age` for encryption, which is modern, secure, and easy to use.
- **Permissions:** Correctly sets `0600` (read/write for owner only) permissions for sensitive files like the age identity key and decrypted files.
- **Sensitive File Warnings:** The `add` command warns users when they are adding known sensitive files (e.g., `.ssh/`, `.aws/credentials`) without the `--encrypt` flag.
- **Git Safety:** The `.age-identity` file is automatically added to `.gitignore` to prevent accidental commits.

### 1.2 Potential Issues
- **Memory Safety (OOM Risk):** In `internal/crypto/crypto.go`, `DecryptFile` uses `io.ReadAll(r)` to read the entire decrypted content into memory. While dotfiles are usually small, a very large encrypted file (e.g., a binary blob or large history file) could cause an Out-Of-Memory (OOM) crash.
  - **Suggestion:** Use `io.Copy(dst, r)` instead of `io.ReadAll(r)`.
- **First Recipient Only:** `EncryptFile` only uses the first recipient found in the `.age-recipients` file. While typically there is only one, `age` supports multiple recipients.
  - **Suggestion:** Load all valid recipients and encrypt for all of them.

---

## 2. Atomicity & Reliability Findings

### 2.1 Non-Atomic File Operations (Major Concern)
Many file operations in `dotfather` are performed in multiple steps without rollback mechanisms. If the process is interrupted (e.g., crash, power loss, Ctrl+C) between steps, the system can be left in an inconsistent state.

- **`linker.MoveFile` (Fallback Path):** If `os.Rename` fails (e.g., cross-device move), it falls back to `CopyFile` followed by `os.Remove`. If it fails after copying but before removing, two copies exist. If it fails halfway through copying, the destination is a partial/corrupted file.
- **`cmd/add.go` (addFile):**
  ```go
  if err := linker.MoveFile(absPath, repoPath); err != nil { ... }
  if err := linker.Link(repoFile, absPath); err != nil { ... }
  ```
  If `MoveFile` succeeds but `Link` fails, the original file is gone from home and moved to the repo, but no symlink is created. The user's home directory is missing the file.
- **`internal/crypto/crypto.go` (Encrypt/Decrypt):** Both `EncryptFile` and `DecryptFile` use `os.OpenFile` with `os.O_TRUNC`. If the operation fails halfway, the target file (whether in the repo or the home directory) is truncated and corrupted.
  - **Suggestion:** Write to a temporary file (e.g., `.filename.tmp`) and use `os.Rename` to replace the target atomically.

### 2.2 Cross-Device Linkage
Symlinks across different file systems (partitions) are supported by most OSs, but `os.Rename` is not. While `linker.MoveFile` handles this with a copy-fallback, the overall architecture assumes the repo and the home directory are on the same filesystem for many operations.

---

## 3. Functional Edge Cases & Bugs

### 3.1 Duplicate Modes in Repo
If a file is added as plain text (`add ~/.bashrc`) and then later added as encrypted (`add --encrypt ~/.bashrc`), the `RepoPathFor` function (in `internal/repo/repo.go`) only looks for the plain path. This can lead to a state where the repo contains both `.bashrc` and `.bashrc.age`.
- **`cmd/add.go`: `convertToEncrypted`** tries to handle this, but the logic for detecting already managed files needs to be more robust to handle both encrypted and unencrypted variants.

### 3.2 Symlink Resolution in `NormalizePath`
In `internal/pathutil/pathutil.go`, `NormalizePath` resolves symlinks in the parent directory but not the final component. This is clever (to avoid resolving the dotfather-managed symlink itself), but it might fail if a parent directory is *itself* a symlink managed by dotfather (though dotfather typically manages files).

### 3.3 Conflict Handling for Encrypted Files
In `cmd/sync.go`, `decryptEncryptedFiles` skips decryption if the target file is newer than the encrypted file in the repo:
```go
if targetInfo.ModTime().After(encInfo.ModTime()) { continue }
```
This avoids overwriting local edits. However, if a `git pull` brings a NEWER encrypted file from remote, but the user also has local edits that are EVEN NEWER, the local edits "win" without any warning to the user that the remote version was ignored.

---

## 4. Missing Functionality & Suggestions

### 4.1 `diff` Command for Encrypted Files
Currently, `dotfather diff` simply calls `git diff`. For encrypted `.age` files, this is useless as it shows binary diffs (or just "binary files differ").
- **Suggestion:** `dotfather diff` should decrypt the repo version to a temporary location and show a diff against the current plaintext file in the home directory.

### 4.2 `doctor` Command
There is no easy way to fix a broken environment.
- **Suggestion:** Add a `doctor` command that:
  - Validates all symlinks.
  - Re-links missing symlinks.
  - Checks for out-of-sync encrypted files.
  - Validates age keys.

### 4.3 SSH Key Integration
`age` supports using SSH keys for encryption/decryption. While the code has some support for parsing SSH keys, the `init` command always generates a new X25519 key.
- **Suggestion:** Allow users to specify an existing SSH public key (`~/.ssh/id_ed25519.pub`) as the recipient during `init`.

### 4.4 Template Support
The README mentions templates, but there is no implementation for them in the current codebase.

---

## 5. Technical Debt & Code Quality

### 5.1 Path Handling
- **Windows Support:** The use of `~/` and hardcoded path separators in some places might hinder future Windows support. Using `filepath` more consistently and avoiding manual `~/` string manipulation would help.
- **Error Handling:** `os.UserHomeDir()` is called in many places. Centralizing this or caching the result in the `Repo` struct would be cleaner.

### 5.2 Error Reporting
In recursive operations (like `add` or `sync`), errors are collected into a slice and reported at the end. This is good, but some intermediate warnings are printed to `os.Stderr` immediately, which can be noisy or inconsistent.

---

## Conclusion
`dotfather` is a solid tool with a clean codebase. The most critical improvement would be ensuring **atomicity** in file operations to prevent data loss or corruption during interruptions. Enhancing the `diff` and `status` commands to better handle encrypted files would also significantly improve the user experience.

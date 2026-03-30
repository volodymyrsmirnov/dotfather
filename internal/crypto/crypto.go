package crypto

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

const (
	IdentityFile  = ".age-identity"
	RecipientFile = ".age-recipients"
	EncryptedExt  = ".age"
)

// GenerateKey creates a new age X25519 keypair and writes the identity
// and recipient files to the repo directory.
func GenerateKey(repoPath string) error {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	identityPath := filepath.Join(repoPath, IdentityFile)
	recipientPath := filepath.Join(repoPath, RecipientFile)

	identityContent := fmt.Sprintf("# dotfather age identity — do NOT commit this file\n# public key: %s\n%s\n",
		identity.Recipient().String(), identity.String())

	if err := os.WriteFile(identityPath, []byte(identityContent), 0600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}

	recipientContent := identity.Recipient().String() + "\n"
	if err := os.WriteFile(recipientPath, []byte(recipientContent), 0644); err != nil {
		return fmt.Errorf("write recipient: %w", err)
	}

	return nil
}

// HasRecipient checks if the recipient file exists in the repo.
func HasRecipient(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, RecipientFile))
	return err == nil
}

// HasIdentity checks if the identity file exists in the repo.
func HasIdentity(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, IdentityFile))
	return err == nil
}

// EncryptFile encrypts srcFile and writes the result to dstFile,
// using the recipient from the repo's .age-recipients file.
func EncryptFile(repoPath, srcFile, dstFile string) error {
	recipient, err := loadRecipient(repoPath)
	if err != nil {
		return err
	}

	src, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	dst, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}

	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		_ = dst.Close()
		return fmt.Errorf("encrypt: %w", err)
	}

	if _, err := io.Copy(w, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("write encrypted data: %w", err)
	}

	if err := w.Close(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("finalize encryption: %w", err)
	}

	return dst.Close()
}

// DecryptFile decrypts srcFile and writes the result to dstFile,
// using the identity from the repo's .age-identity file.
func DecryptFile(repoPath, srcFile, dstFile string) error {
	identities, err := loadIdentities(repoPath)
	if err != nil {
		return err
	}

	f, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open encrypted file: %w", err)
	}
	defer func() { _ = f.Close() }()

	r, err := age.Decrypt(f, identities...)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read decrypted data: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	// Preserve existing file permissions; default to 0600 for new files.
	mode := os.FileMode(0600)
	if info, err := os.Stat(dstFile); err == nil {
		mode = info.Mode().Perm()
	}

	if err := os.WriteFile(dstFile, plaintext, mode); err != nil {
		return fmt.Errorf("write decrypted file: %w", err)
	}

	return nil
}

// IsEncrypted returns true if the repo-relative path has the .age extension.
func IsEncrypted(relPath string) bool {
	return strings.HasSuffix(relPath, EncryptedExt)
}

// PlaintextPath strips the .age extension from a repo-relative path.
func PlaintextPath(relPath string) string {
	return strings.TrimSuffix(relPath, EncryptedExt)
}

// EncryptedPath adds the .age extension to a repo-relative path.
func EncryptedPath(relPath string) string {
	return relPath + EncryptedExt
}

func loadRecipient(repoPath string) (age.Recipient, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, RecipientFile))
	if err != nil {
		return nil, fmt.Errorf("read recipient file: %w (run 'dotfather init' to generate keys)", err)
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		return nil, fmt.Errorf("recipient file is empty")
	}

	// Take first non-comment line; try X25519, then SSH public key.
	for _, l := range strings.Split(line, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if r, err := age.ParseX25519Recipient(l); err == nil {
			return r, nil
		}
		if r, err := agessh.ParseRecipient(l); err == nil {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no recipient found in %s", RecipientFile)
}

func loadIdentities(repoPath string) ([]age.Identity, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, IdentityFile))
	if err != nil {
		return nil, fmt.Errorf("open identity file: %w (copy from another machine or run 'dotfather init')", err)
	}

	// Try native age identities first.
	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err == nil && len(identities) > 0 {
		return identities, nil
	}

	// Try SSH private key.
	if id, sshErr := agessh.ParseIdentity(data); sshErr == nil {
		return []age.Identity{id}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("parse identities: %w", err)
	}
	return nil, fmt.Errorf("no identities found in %s", IdentityFile)
}

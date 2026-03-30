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

	"github.com/volodymyrsmirnov/dotfather/internal/fileutil"
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
// using all recipients from the repo's .age-recipients file.
// The write is atomic: a temp file is used and renamed into place.
func EncryptFile(repoPath, srcFile, dstFile string) error {
	recipients, err := loadRecipients(repoPath)
	if err != nil {
		return err
	}

	src, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	return fileutil.AtomicWriteFile(dstFile, 0600, func(dst io.Writer) error {
		w, err := age.Encrypt(dst, recipients...)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}
		if _, err := io.Copy(w, src); err != nil {
			return fmt.Errorf("write encrypted data: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("finalize encryption: %w", err)
		}
		return nil
	})
}

// DecryptFile decrypts srcFile and writes the result to dstFile,
// using the identity from the repo's .age-identity file.
// The write is atomic (temp file + rename) and rejects symlinks at the
// destination to prevent decrypted secrets from leaking through a symlink.
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

	// Preserve existing file permissions; default to 0600 for new files.
	mode := os.FileMode(0600)
	if info, statErr := os.Lstat(dstFile); statErr == nil {
		if info.Mode().IsRegular() {
			mode = info.Mode().Perm()
		}
	}

	return fileutil.SafeWriteTarget(dstFile, mode, func(dst io.Writer) error {
		if _, err := io.Copy(dst, r); err != nil {
			return fmt.Errorf("write decrypted data: %w", err)
		}
		return nil
	})
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

func loadRecipients(repoPath string) ([]age.Recipient, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, RecipientFile))
	if err != nil {
		return nil, fmt.Errorf("read recipient file: %w (run 'dotfather init' to generate keys)", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil, fmt.Errorf("recipient file is empty")
	}

	var recipients []age.Recipient
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if r, err := age.ParseX25519Recipient(l); err == nil {
			recipients = append(recipients, r)
			continue
		}
		if r, err := agessh.ParseRecipient(l); err == nil {
			recipients = append(recipients, r)
			continue
		}
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients found in %s", RecipientFile)
	}
	return recipients, nil
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

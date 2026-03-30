package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	dir := t.TempDir()

	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	// Identity file should exist with restricted permissions.
	identPath := filepath.Join(dir, IdentityFile)
	info, err := os.Stat(identPath)
	if err != nil {
		t.Fatalf("identity file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("identity permissions = %v, want 0600", info.Mode().Perm())
	}

	// Recipient file should exist.
	recipPath := filepath.Join(dir, RecipientFile)
	if _, err := os.Stat(recipPath); err != nil {
		t.Fatalf("recipient file not created: %v", err)
	}

	// Recipient should start with "age1".
	data, err := os.ReadFile(recipPath)
	if err != nil {
		t.Fatalf("read recipient: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "age1" {
		t.Errorf("recipient should start with 'age1', got %q", string(data[:10]))
	}
}

func TestHasRecipient(t *testing.T) {
	dir := t.TempDir()

	if HasRecipient(dir) {
		t.Error("HasRecipient() should be false before key generation")
	}

	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if !HasRecipient(dir) {
		t.Error("HasRecipient() should be true after key generation")
	}
}

func TestHasIdentity(t *testing.T) {
	dir := t.TempDir()

	if HasIdentity(dir) {
		t.Error("HasIdentity() should be false before key generation")
	}

	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if !HasIdentity(dir) {
		t.Error("HasIdentity() should be true after key generation")
	}
}

func TestEncryptDecryptFile(t *testing.T) {
	dir := t.TempDir()

	// Generate keys.
	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create a plaintext file.
	srcFile := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(srcFile, []byte("super secret content"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// Encrypt.
	encFile := filepath.Join(dir, "secret.txt.age")
	if err := EncryptFile(dir, srcFile, encFile); err != nil {
		t.Fatalf("EncryptFile() error: %v", err)
	}

	// Encrypted file should exist.
	if _, err := os.Stat(encFile); err != nil {
		t.Fatalf("encrypted file not created: %v", err)
	}

	// Encrypted file content should differ from plaintext.
	encData, err := os.ReadFile(encFile)
	if err != nil {
		t.Fatalf("read encrypted: %v", err)
	}
	if string(encData) == "super secret content" {
		t.Error("encrypted file should not contain plaintext")
	}

	// Decrypt to a new file.
	dstFile := filepath.Join(dir, "decrypted.txt")
	if err := DecryptFile(dir, encFile, dstFile); err != nil {
		t.Fatalf("DecryptFile() error: %v", err)
	}

	// Decrypted content should match original.
	decrypted, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read decrypted: %v", err)
	}
	if string(decrypted) != "super secret content" {
		t.Errorf("decrypted = %q, want %q", decrypted, "super secret content")
	}

	// Decrypted file should have restricted permissions.
	info, err := os.Stat(dstFile)
	if err != nil {
		t.Fatalf("stat decrypted: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("decrypted permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestEncryptFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	srcFile := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	encFile := filepath.Join(dir, "deep", "nested", "src.txt.age")
	if err := EncryptFile(dir, srcFile, encFile); err != nil {
		t.Fatalf("EncryptFile() error: %v", err)
	}

	if _, err := os.Stat(encFile); err != nil {
		t.Error("encrypted file should exist in nested directory")
	}
}

func TestDecryptFile_NoIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	srcFile := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	encFile := filepath.Join(dir, "src.txt.age")
	if err := EncryptFile(dir, srcFile, encFile); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// Remove the identity file.
	if err := os.Remove(filepath.Join(dir, IdentityFile)); err != nil {
		t.Fatalf("remove identity: %v", err)
	}

	// Decrypt should fail.
	dstFile := filepath.Join(dir, "out.txt")
	if err := DecryptFile(dir, encFile, dstFile); err == nil {
		t.Error("DecryptFile() should fail without identity")
	}
}

func TestEncryptFile_NoRecipient(t *testing.T) {
	dir := t.TempDir()

	srcFile := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	encFile := filepath.Join(dir, "src.txt.age")
	err := EncryptFile(dir, srcFile, encFile)
	if err == nil {
		t.Error("EncryptFile() should fail without recipient")
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".ssh/config.age", true},
		{".bashrc", false},
		{".config/app.yaml.age", true},
		{"age", false},
		{".age", true},
	}

	for _, tt := range tests {
		if got := IsEncrypted(tt.path); got != tt.want {
			t.Errorf("IsEncrypted(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPlaintextPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{".ssh/config.age", ".ssh/config"},
		{".bashrc.age", ".bashrc"},
		{".bashrc", ".bashrc"},
	}

	for _, tt := range tests {
		if got := PlaintextPath(tt.input); got != tt.want {
			t.Errorf("PlaintextPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEncryptedPath(t *testing.T) {
	if got := EncryptedPath(".ssh/config"); got != ".ssh/config.age" {
		t.Errorf("EncryptedPath() = %q, want %q", got, ".ssh/config.age")
	}
}

func TestEncryptDecryptRoundtrip_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	srcFile := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(srcFile, []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	encFile := filepath.Join(dir, "empty.txt.age")
	if err := EncryptFile(dir, srcFile, encFile); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	dstFile := filepath.Join(dir, "out.txt")
	if err := DecryptFile(dir, encFile, dstFile); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "" {
		t.Errorf("expected empty file, got %q", data)
	}
}

func TestEncryptDecryptRoundtrip_BinaryData(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateKey(dir); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create file with binary data.
	binaryData := make([]byte, 256)
	for i := range binaryData {
		binaryData[i] = byte(i)
	}

	srcFile := filepath.Join(dir, "binary.dat")
	if err := os.WriteFile(srcFile, binaryData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	encFile := filepath.Join(dir, "binary.dat.age")
	if err := EncryptFile(dir, srcFile, encFile); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	dstFile := filepath.Join(dir, "out.dat")
	if err := DecryptFile(dir, encFile, dstFile); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != len(binaryData) {
		t.Fatalf("length = %d, want %d", len(data), len(binaryData))
	}
	for i, b := range data {
		if b != binaryData[i] {
			t.Errorf("byte[%d] = %d, want %d", i, b, binaryData[i])
			break
		}
	}
}

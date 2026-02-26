package config

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"my-secret-token",
		"",
		"a",
		"hello world! 🌍",
		"vercel_token_abc123xyz",
	}

	for _, plaintext := range tests {
		encrypted, err := Encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plaintext, err)
		}

		if !IsEncrypted(encrypted) {
			t.Errorf("encrypted string should have ENC: prefix, got %q", encrypted)
		}

		decrypted, err := Decrypt(key, encrypted)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}

		if decrypted != plaintext {
			t.Errorf("got %q, want %q", decrypted, plaintext)
		}
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}

	enc1, _ := Encrypt(key, "same-token")
	enc2, _ := Encrypt(key, "same-token")

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestDecryptInvalidPrefix(t *testing.T) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}

	_, err := Decrypt(key, "not-encrypted")
	if err == nil {
		t.Error("expected error for missing prefix")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, keySize)
	key2 := make([]byte, keySize)
	io.ReadFull(rand.Reader, key1)
	io.ReadFull(rand.Reader, key2)

	encrypted, _ := Encrypt(key1, "secret")
	_, err := Decrypt(key2, encrypted)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestLoadOrCreateKey(t *testing.T) {
	// Use a temp home dir to avoid touching the real ~/.orbit/
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// First call should create the key
	key1, err := LoadOrCreateKey()
	if err != nil {
		t.Fatalf("LoadOrCreateKey (create): %v", err)
	}
	if len(key1) != keySize {
		t.Fatalf("key length: got %d, want %d", len(key1), keySize)
	}

	// Verify file permissions
	keyPath := filepath.Join(tmpHome, ".orbit", "key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("key file permissions: got %o, want 0600", perm)
	}

	// Second call should load the same key
	key2, err := LoadOrCreateKey()
	if err != nil {
		t.Fatalf("LoadOrCreateKey (load): %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("loaded key differs from created key")
	}
}

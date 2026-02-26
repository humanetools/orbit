package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	encPrefix = "ENC:"
	keySize   = 32 // AES-256
)

func keyFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".orbit", "key"), nil
}

// LoadOrCreateKey reads the AES-256 key from ~/.orbit/key.
// If the file does not exist, a new random key is generated and saved with 0600 permissions.
func LoadOrCreateKey() ([]byte, error) {
	path, err := keyFilePath()
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create orbit dir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("decode key file: %w", err)
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("invalid key length: got %d, want %d", len(key), keySize)
		}
		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Generate new key
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}

	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a string prefixed with "ENC:".
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return encPrefix + encoded, nil
}

// Decrypt decrypts a string previously encrypted with Encrypt.
// The input must be prefixed with "ENC:".
func Decrypt(key []byte, encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, encPrefix) {
		return "", fmt.Errorf("invalid encrypted string: missing %q prefix", encPrefix)
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted checks if a string has the encryption prefix.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, encPrefix)
}

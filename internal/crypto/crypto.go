package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	ErrInvalidKey        = errors.New("encryption key must be 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

var (
	// ErrEncryptionKeyRequired is returned when no encryption key is configured
	ErrEncryptionKeyRequired = errors.New("ENCRYPTION_KEY environment variable is required")
)

// DefaultKey returns the encryption key from environment.
// In production, this MUST be set via ENCRYPTION_KEY environment variable.
// The key must be exactly 32 bytes (for AES-256) and should be base64 encoded.
func DefaultKey() []byte {
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		// In development mode only, allow a generated key with warning
		if os.Getenv("APP_ENV") == "development" || os.Getenv("ENV") == "development" {
			// Generate a deterministic dev key based on hostname for consistency
			// WARNING: This is NOT secure and should never be used in production
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "localhost"
			}
			devKey := make([]byte, 32)
			copy(devKey, []byte("dev-"+hostname+"-testforge-key!!"))
			return devKey
		}
		// In production/staging, panic if no key is set
		panic(ErrEncryptionKeyRequired)
	}
	// If key is base64 encoded
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		return decoded
	}
	// If key is raw string of exactly 32 bytes
	if len(key) == 32 {
		return []byte(key)
	}
	// Key is invalid length
	panic(fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (got %d bytes), preferably base64 encoded", len(key)))
}

// MustDefaultKey returns the default key or panics with a clear message
func MustDefaultKey() []byte {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Sprintf("failed to get encryption key: %v", r))
		}
	}()
	return DefaultKey()
}

// Encrypt encrypts plaintext using AES-256-GCM
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func Decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}

// EncryptIfNotEmpty encrypts only if the value is not empty
func EncryptIfNotEmpty(value string, key []byte) (string, error) {
	if value == "" {
		return "", nil
	}
	return Encrypt(value, key)
}

// DecryptIfNotEmpty decrypts only if the value is not empty
func DecryptIfNotEmpty(value string, key []byte) (string, error) {
	if value == "" {
		return "", nil
	}
	return Decrypt(value, key)
}

// MustEncrypt encrypts and panics on error (use only in tests)
func MustEncrypt(plaintext string, key []byte) string {
	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		panic(err)
	}
	return encrypted
}

// MustDecrypt decrypts and panics on error (use only in tests)
func MustDecrypt(ciphertext string, key []byte) string {
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		panic(err)
	}
	return decrypted
}

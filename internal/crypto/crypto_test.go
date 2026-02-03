package crypto

import (
	"os"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "simple text",
			plaintext: "hello world",
		},
		{
			name:      "empty string",
			plaintext: "",
		},
		{
			name:      "unicode text",
			plaintext: "Hello, 世界! 🌍",
		},
		{
			name:      "long text",
			plaintext: "This is a longer piece of text that should still encrypt and decrypt properly without any issues whatsoever.",
		},
		{
			name:      "special characters",
			plaintext: "password123!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if tt.plaintext != "" && encrypted == tt.plaintext {
				t.Error("Encrypt() returned plaintext without encryption")
			}

			decrypted, err := Decrypt(encrypted, key)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncrypt_InvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		key     []byte
		wantErr error
	}{
		{
			name:    "key too short",
			key:     []byte("short"),
			wantErr: ErrInvalidKey,
		},
		{
			name:    "key too long",
			key:     make([]byte, 64),
			wantErr: ErrInvalidKey,
		},
		{
			name:    "empty key",
			key:     []byte{},
			wantErr: ErrInvalidKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Encrypt("test", tt.key)
			if err != tt.wantErr {
				t.Errorf("Encrypt() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecrypt_InvalidKey(t *testing.T) {
	validKey := make([]byte, 32)
	copy(validKey, []byte("12345678901234567890123456789012"))

	// First encrypt something
	encrypted, err := Encrypt("test data", validKey)
	if err != nil {
		t.Fatalf("Setup Encrypt() error = %v", err)
	}

	tests := []struct {
		name    string
		key     []byte
		wantErr error
	}{
		{
			name:    "key too short",
			key:     []byte("short"),
			wantErr: ErrInvalidKey,
		},
		{
			name:    "key too long",
			key:     make([]byte, 64),
			wantErr: ErrInvalidKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(encrypted, tt.key)
			if err != tt.wantErr {
				t.Errorf("Decrypt() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	tests := []struct {
		name       string
		ciphertext string
		wantErr    error
	}{
		{
			name:       "not base64",
			ciphertext: "not valid base64!!!",
			wantErr:    ErrInvalidCiphertext,
		},
		{
			name:       "base64 but too short",
			ciphertext: "YWJj", // "abc" in base64, too short for nonce
			wantErr:    ErrInvalidCiphertext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(tt.ciphertext, key)
			if err != tt.wantErr {
				t.Errorf("Decrypt() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	copy(key1, []byte("12345678901234567890123456789012"))

	key2 := make([]byte, 32)
	copy(key2, []byte("different-key-for-testing!!!!!!!"))

	encrypted, err := Encrypt("secret data", key1)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err != ErrDecryptionFailed {
		t.Errorf("Decrypt() with wrong key: error = %v, want %v", err, ErrDecryptionFailed)
	}
}

func TestEncryptIfNotEmpty(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	t.Run("empty string", func(t *testing.T) {
		result, err := EncryptIfNotEmpty("", key)
		if err != nil {
			t.Errorf("EncryptIfNotEmpty() error = %v", err)
		}
		if result != "" {
			t.Errorf("EncryptIfNotEmpty() = %v, want empty string", result)
		}
	})

	t.Run("non-empty string", func(t *testing.T) {
		result, err := EncryptIfNotEmpty("data", key)
		if err != nil {
			t.Errorf("EncryptIfNotEmpty() error = %v", err)
		}
		if result == "" {
			t.Error("EncryptIfNotEmpty() returned empty string for non-empty input")
		}
		if result == "data" {
			t.Error("EncryptIfNotEmpty() returned plaintext without encryption")
		}
	})
}

func TestDecryptIfNotEmpty(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	t.Run("empty string", func(t *testing.T) {
		result, err := DecryptIfNotEmpty("", key)
		if err != nil {
			t.Errorf("DecryptIfNotEmpty() error = %v", err)
		}
		if result != "" {
			t.Errorf("DecryptIfNotEmpty() = %v, want empty string", result)
		}
	})

	t.Run("non-empty string", func(t *testing.T) {
		encrypted, _ := Encrypt("secret", key)
		result, err := DecryptIfNotEmpty(encrypted, key)
		if err != nil {
			t.Errorf("DecryptIfNotEmpty() error = %v", err)
		}
		if result != "secret" {
			t.Errorf("DecryptIfNotEmpty() = %v, want 'secret'", result)
		}
	})
}

func TestMustEncrypt(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	t.Run("valid encryption", func(t *testing.T) {
		result := MustEncrypt("test", key)
		if result == "" {
			t.Error("MustEncrypt() returned empty string")
		}
	})

	t.Run("invalid key panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustEncrypt() did not panic with invalid key")
			}
		}()
		MustEncrypt("test", []byte("short"))
	})
}

func TestMustDecrypt(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	encrypted := MustEncrypt("test", key)

	t.Run("valid decryption", func(t *testing.T) {
		result := MustDecrypt(encrypted, key)
		if result != "test" {
			t.Errorf("MustDecrypt() = %v, want 'test'", result)
		}
	})

	t.Run("invalid ciphertext panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustDecrypt() did not panic with invalid ciphertext")
			}
		}()
		MustDecrypt("invalid", key)
	})
}

func TestDefaultKey_Development(t *testing.T) {
	// Save original env vars
	originalKey := os.Getenv("ENCRYPTION_KEY")
	originalAppEnv := os.Getenv("APP_ENV")
	originalEnv := os.Getenv("ENV")

	// Restore after test
	defer func() {
		os.Setenv("ENCRYPTION_KEY", originalKey)
		os.Setenv("APP_ENV", originalAppEnv)
		os.Setenv("ENV", originalEnv)
	}()

	t.Run("development mode with APP_ENV", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "")
		os.Setenv("APP_ENV", "development")
		os.Setenv("ENV", "")

		key := DefaultKey()
		if len(key) != 32 {
			t.Errorf("DefaultKey() length = %d, want 32", len(key))
		}
	})

	t.Run("development mode with ENV", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "")
		os.Setenv("APP_ENV", "")
		os.Setenv("ENV", "development")

		key := DefaultKey()
		if len(key) != 32 {
			t.Errorf("DefaultKey() length = %d, want 32", len(key))
		}
	})

	t.Run("with base64 key", func(t *testing.T) {
		// 32 bytes base64 encoded
		os.Setenv("ENCRYPTION_KEY", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
		os.Setenv("APP_ENV", "")
		os.Setenv("ENV", "")

		key := DefaultKey()
		if len(key) != 32 {
			t.Errorf("DefaultKey() length = %d, want 32", len(key))
		}
	})

	t.Run("with raw 32-byte key", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
		os.Setenv("APP_ENV", "")
		os.Setenv("ENV", "")

		key := DefaultKey()
		if len(key) != 32 {
			t.Errorf("DefaultKey() length = %d, want 32", len(key))
		}
		if string(key) != "12345678901234567890123456789012" {
			t.Errorf("DefaultKey() = %s, want raw key", string(key))
		}
	})
}

func TestDefaultKey_Production_Panics(t *testing.T) {
	// Save original env vars
	originalKey := os.Getenv("ENCRYPTION_KEY")
	originalAppEnv := os.Getenv("APP_ENV")
	originalEnv := os.Getenv("ENV")

	// Restore after test
	defer func() {
		os.Setenv("ENCRYPTION_KEY", originalKey)
		os.Setenv("APP_ENV", originalAppEnv)
		os.Setenv("ENV", originalEnv)
	}()

	t.Run("no key in production panics", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "")
		os.Setenv("APP_ENV", "production")
		os.Setenv("ENV", "production")

		defer func() {
			if r := recover(); r == nil {
				t.Error("DefaultKey() did not panic in production without key")
			}
		}()
		DefaultKey()
	})

	t.Run("invalid key length panics", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "too-short")
		os.Setenv("APP_ENV", "")
		os.Setenv("ENV", "")

		defer func() {
			if r := recover(); r == nil {
				t.Error("DefaultKey() did not panic with invalid key length")
			}
		}()
		DefaultKey()
	})
}

func TestMustDefaultKey(t *testing.T) {
	// Save original env vars
	originalKey := os.Getenv("ENCRYPTION_KEY")
	originalAppEnv := os.Getenv("APP_ENV")

	defer func() {
		os.Setenv("ENCRYPTION_KEY", originalKey)
		os.Setenv("APP_ENV", originalAppEnv)
	}()

	t.Run("valid key", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
		os.Setenv("APP_ENV", "")

		key := MustDefaultKey()
		if len(key) != 32 {
			t.Errorf("MustDefaultKey() length = %d, want 32", len(key))
		}
	})

	t.Run("panics with wrapped message", func(t *testing.T) {
		os.Setenv("ENCRYPTION_KEY", "short")
		os.Setenv("APP_ENV", "")

		defer func() {
			r := recover()
			if r == nil {
				t.Error("MustDefaultKey() did not panic")
				return
			}
			panicMsg, ok := r.(string)
			if !ok {
				t.Error("MustDefaultKey() panic was not a string")
				return
			}
			if panicMsg == "" {
				t.Error("MustDefaultKey() panic message was empty")
			}
		}()
		MustDefaultKey()
	})
}

func TestEncryptDecrypt_Consistency(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("12345678901234567890123456789012"))

	// Same plaintext should produce different ciphertexts (due to random nonce)
	plaintext := "test data"
	encrypted1, _ := Encrypt(plaintext, key)
	encrypted2, _ := Encrypt(plaintext, key)

	if encrypted1 == encrypted2 {
		t.Error("Encrypt() produced identical ciphertexts for same input (should use random nonce)")
	}

	// But both should decrypt to the same value
	decrypted1, _ := Decrypt(encrypted1, key)
	decrypted2, _ := Decrypt(encrypted2, key)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Decrypt() did not return original plaintext")
	}
}

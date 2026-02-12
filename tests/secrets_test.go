package tests

import (
	"testing"

	"github.com/warrentherabbit/alexandria/internal/encryption"
	"github.com/warrentherabbit/alexandria/internal/store"
)

func TestFernetEncryptDecrypt(t *testing.T) {
	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	enc, err := encryption.NewEncryptor(key.Encode())
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	plaintext := "super-secret-api-key-12345"
	token, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if token == plaintext {
		t.Error("encrypted value should differ from plaintext")
	}

	decrypted, err := enc.Decrypt(token)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected '%s', got '%s'", plaintext, decrypted)
	}
}

func TestFernetWrongKey(t *testing.T) {
	key1, _ := encryption.GenerateKey()
	key2, _ := encryption.GenerateKey()

	enc1, _ := encryption.NewEncryptor(key1.Encode())
	enc2, _ := encryption.NewEncryptor(key2.Encode())

	token, _ := enc1.Encrypt("test-value")

	_, err := enc2.Decrypt(token)
	if err == nil {
		t.Error("decrypting with wrong key should fail")
	}
}

func TestFernetEmptyKey(t *testing.T) {
	_, err := encryption.NewEncryptor("")
	if err == nil {
		t.Error("empty key should fail")
	}
}

func TestSecretScopeAccess(t *testing.T) {
	ss := &store.SecretStore{}

	tests := []struct {
		name    string
		scope   []string
		agent   string
		allowed bool
	}{
		{"admin always allowed", []string{}, "warren", true},
		{"empty scope denies non-admin", []string{}, "kai", false},
		{"agent in scope", []string{"kai", "lily"}, "kai", true},
		{"agent not in scope", []string{"kai"}, "lily", false},
		{"wildcard allows all", []string{"*"}, "celebrimbor", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &store.Secret{Scope: tt.scope}
			result := ss.CanAccess(secret, tt.agent)
			if result != tt.allowed {
				t.Errorf("expected %v, got %v", tt.allowed, result)
			}
		})
	}
}

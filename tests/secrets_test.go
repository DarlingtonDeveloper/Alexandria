package tests

import (
	"os"
	"testing"

	"github.com/warrentherabbit/alexandria/internal/config"
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

func TestFernetRejectsInvalidKeyStrings(t *testing.T) {
	// Reproduces root cause: a human-readable string was used as ENCRYPTION_KEY
	// instead of a valid URL-safe base64-encoded 32-byte Fernet key.
	invalidKeys := []string{
		"kai-alexandria-fernet-key-2026", // the actual bad value from production
		"not-a-valid-base64-key",
		"too-short",
		"AAAA", // valid base64 but wrong length (3 bytes)
	}
	for _, k := range invalidKeys {
		t.Run(k, func(t *testing.T) {
			_, err := encryption.NewEncryptor(k)
			if err == nil {
				t.Errorf("expected error for invalid key %q, got nil", k)
			}
		})
	}
}

func TestFernetKeyConsistencyAcrossInstances(t *testing.T) {
	// With a persistent key, secrets encrypted by one encryptor instance
	// must be decryptable by another instance using the same key.
	// This simulates container restarts with a stable ENCRYPTION_KEY.
	key, _ := encryption.GenerateKey()
	keyStr := key.Encode()

	enc1, _ := encryption.NewEncryptor(keyStr)
	token, _ := enc1.Encrypt("persistent-secret")

	// Simulate restart — new encryptor instance, same key
	enc2, _ := encryption.NewEncryptor(keyStr)
	decrypted, err := enc2.Decrypt(token)
	if err != nil {
		t.Fatalf("same key should decrypt across instances: %v", err)
	}
	if decrypted != "persistent-secret" {
		t.Errorf("expected 'persistent-secret', got '%s'", decrypted)
	}
}

func TestEphemeralKeysCannotDecryptEachOther(t *testing.T) {
	// Demonstrates the failure mode: if each container start generates
	// a new ephemeral key, secrets from previous runs are unrecoverable.
	key1, _ := encryption.GenerateKey()
	key2, _ := encryption.GenerateKey()

	enc1, _ := encryption.NewEncryptor(key1.Encode())
	enc2, _ := encryption.NewEncryptor(key2.Encode())

	token, _ := enc1.Encrypt("will-be-lost")

	_, err := enc2.Decrypt(token)
	if err == nil {
		t.Error("different ephemeral keys must not decrypt each other's tokens")
	}
}

func TestConfigLoadsEncryptionKeyFromEnv(t *testing.T) {
	key, _ := encryption.GenerateKey()
	keyStr := key.Encode()

	os.Setenv("ENCRYPTION_KEY", keyStr)
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	defer os.Unsetenv("ENCRYPTION_KEY")
	defer os.Unsetenv("DATABASE_URL")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	if cfg.EncryptionKey != keyStr {
		t.Errorf("expected EncryptionKey=%q, got %q", keyStr, cfg.EncryptionKey)
	}
}

func TestConfigDoesNotReadVaultEncryptionKey(t *testing.T) {
	// The bug: stack.yaml set VAULT_ENCRYPTION_KEY but config.go reads ENCRYPTION_KEY.
	// This test ensures the wrong env var name is NOT silently accepted.
	os.Setenv("VAULT_ENCRYPTION_KEY", "some-key")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	os.Unsetenv("ENCRYPTION_KEY")
	defer os.Unsetenv("VAULT_ENCRYPTION_KEY")
	defer os.Unsetenv("DATABASE_URL")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	if cfg.EncryptionKey == "some-key" {
		t.Error("config should not read VAULT_ENCRYPTION_KEY; it reads ENCRYPTION_KEY")
	}
}

func TestSecretOwnerAccess(t *testing.T) {
	ss := &store.SecretStore{}

	agentType := "agent"
	agentKai := "kai"

	tests := []struct {
		name    string
		secret  *store.Secret
		agent   string
		allowed bool
	}{
		{
			"owner agent has access via AgentID field",
			&store.Secret{AgentID: &agentKai},
			"kai",
			true,
		},
		{
			"non-owner agent denied via AgentID field",
			&store.Secret{AgentID: &agentKai},
			"lily",
			false,
		},
		{
			"owner_type agent with matching owner_id is not checked by CanAccess",
			&store.Secret{OwnerType: &agentType, OwnerID: &agentKai},
			"kai",
			false, // CanAccess only checks Scope and AgentID, not OwnerType/OwnerID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ss.CanAccess(tt.secret, tt.agent)
			if result != tt.allowed {
				t.Errorf("expected %v, got %v", tt.allowed, result)
			}
		})
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

// Package encryption provides Fernet-based symmetric encryption for secrets.
package encryption

import (
	"fmt"
	"strings"

	"github.com/fernet/fernet-go"
)

// Encryptor provides encrypt/decrypt operations using Fernet.
type Encryptor struct {
	key *fernet.Key
}

// NewEncryptor creates a new Fernet encryptor with the given key string.
// The key should be a URL-safe base64-encoded 32-byte key.
func NewEncryptor(keyStr string) (*Encryptor, error) {
	keyStr = strings.TrimSpace(keyStr)
	if keyStr == "" {
		return nil, fmt.Errorf("encryption key is empty")
	}

	k, err := fernet.DecodeKey(keyStr)
	if err != nil {
		return nil, fmt.Errorf("decoding fernet key: %w", err)
	}

	return &Encryptor{key: k}, nil
}

// GenerateKey creates a new random Fernet key.
func GenerateKey() (*fernet.Key, error) {
	k := new(fernet.Key)
	if err := k.Generate(); err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	return k, nil
}

// Encrypt encrypts plaintext and returns a Fernet token string.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	tok, err := fernet.EncryptAndSign([]byte(plaintext), e.key)
	if err != nil {
		return "", fmt.Errorf("encrypting: %w", err)
	}
	return string(tok), nil
}

// Decrypt decrypts a Fernet token and returns the plaintext.
func (e *Encryptor) Decrypt(token string) (string, error) {
	msg := fernet.VerifyAndDecrypt([]byte(token), 0, []*fernet.Key{e.key})
	if msg == nil {
		return "", fmt.Errorf("decryption failed: invalid token or key")
	}
	return string(msg), nil
}

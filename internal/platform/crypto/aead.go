package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// AEAD encrypts and decrypts small secrets (integration credentials, GST
// API tokens, WhatsApp/SMTP secrets) at rest using AES-256-GCM. The key is
// expected to come from environment/secrets management (brief §60) —
// never from a database column and never logged.
type AEAD struct {
	gcm cipher.AEAD
}

// NewAEAD builds an AEAD from a 32-byte (AES-256) key.
func NewAEAD(key []byte) (*AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: AEAD key must be 32 bytes (AES-256), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: building AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: building GCM: %w", err)
	}
	return &AEAD{gcm: gcm}, nil
}

// Seal encrypts plaintext, returning nonce||ciphertext||tag as a single
// byte slice. additionalData is authenticated but not encrypted (e.g. a
// record ID, to bind the ciphertext to the row it belongs to and detect a
// ciphertext copied between rows).
func (a *AEAD) Seal(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generating nonce: %w", err)
	}
	return a.gcm.Seal(nonce, nonce, plaintext, additionalData), nil
}

// Open decrypts a value produced by Seal. additionalData must match what
// was passed to Seal.
func (a *AEAD) Open(sealed, additionalData []byte) ([]byte, error) {
	nonceSize := a.gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("crypto: sealed value shorter than nonce size")
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := a.gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("crypto: decryption failed: %w", err)
	}
	return plaintext, nil
}

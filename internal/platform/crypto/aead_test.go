package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestAEADSealOpenRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	a, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD: %v", err)
	}
	plaintext := []byte("gsp-client-secret-value")
	aad := []byte("integration-credential-row-id-123")

	sealed, err := a.Seal(plaintext, aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("sealed output must not contain the plaintext verbatim")
	}

	opened, err := a.Open(sealed, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("Open = %q, want %q", opened, plaintext)
	}
}

func TestAEADOpenFailsWithWrongAAD(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	a, _ := NewAEAD(key)
	sealed, _ := a.Seal([]byte("secret"), []byte("row-1"))

	if _, err := a.Open(sealed, []byte("row-2")); err == nil {
		t.Fatal("expected Open to fail when additionalData does not match")
	}
}

func TestAEADOpenFailsWithTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	a, _ := NewAEAD(key)
	sealed, _ := a.Seal([]byte("secret"), nil)
	sealed[len(sealed)-1] ^= 0xFF

	if _, err := a.Open(sealed, nil); err == nil {
		t.Fatal("expected Open to fail on tampered ciphertext")
	}
}

func TestNewAEADRejectsWrongKeyLength(t *testing.T) {
	if _, err := NewAEAD(make([]byte, 16)); err == nil {
		t.Fatal("expected error for a non-32-byte key")
	}
}

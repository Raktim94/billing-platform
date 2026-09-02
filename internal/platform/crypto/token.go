package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// RandomToken generates a cryptographically secure random token, returned
// both as raw bytes and as a URL-safe base64 string suitable for embedding
// in a link (password reset, session bearer value, API key). byteLength
// should be at least 32 (256 bits) for anything security-sensitive.
func RandomToken(byteLength int) (raw []byte, encoded string, err error) {
	if byteLength < 16 {
		return nil, "", fmt.Errorf("crypto: token length %d is too short to be secure", byteLength)
	}
	raw = make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("crypto: generating random token: %w", err)
	}
	return raw, base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashToken returns a SHA-256 hex digest of a token, for storage. Tokens
// produced by RandomToken already carry 256 bits of entropy, so a fast
// hash (unlike password hashing) is appropriate here — the threat this
// guards against is "database dump reveals a still-valid, reusable
// bearer token," not offline brute force of a low-entropy secret (brief
// §27: reset tokens "store only hashed", single-use, expiring).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokensEqual does a constant-time comparison of two token hash strings.
// Use this instead of == when comparing a caller-supplied hash against a
// stored one, to avoid leaking match-length via timing.
func TokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

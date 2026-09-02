// Package crypto provides the password-hashing and at-rest encryption
// primitives used across the platform. This is the only package permitted
// to call a hashing/encryption primitive directly (brief §61 — "No custom
// encryption schemes"; everything here wraps golang.org/x/crypto or the
// standard library, nothing is implemented from scratch).
package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PasswordParams are the Argon2id cost parameters. See
// docs/adr/0001-argon2id-parameters.md for how the defaults were chosen and
// benchmarked. The brief's stated floor is memory>=19MiB, iterations>=2,
// parallelism>=1; PasswordHasher enforces that floor at construction time
// so a misconfigured deployment cannot silently weaken password storage.
type PasswordParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

const argon2MinMemoryKiB = 19 * 1024

// PasswordHasher hashes and verifies passwords with Argon2id.
type PasswordHasher struct {
	params PasswordParams
}

// NewPasswordHasher validates params against the brief's floor and returns
// a PasswordHasher. Construct one per process at startup, not per request.
func NewPasswordHasher(params PasswordParams) (*PasswordHasher, error) {
	if params.MemoryKiB < argon2MinMemoryKiB {
		return nil, fmt.Errorf("crypto: Argon2 memory %d KiB is below the required floor of %d KiB (19 MiB)",
			params.MemoryKiB, argon2MinMemoryKiB)
	}
	if params.Iterations < 2 {
		return nil, fmt.Errorf("crypto: Argon2 iterations %d is below the required floor of 2", params.Iterations)
	}
	if params.Parallelism < 1 {
		return nil, fmt.Errorf("crypto: Argon2 parallelism %d is below the required floor of 1", params.Parallelism)
	}
	if params.SaltLength == 0 {
		params.SaltLength = 16
	}
	if params.KeyLength == 0 {
		params.KeyLength = 32
	}
	return &PasswordHasher{params: params}, nil
}

// Hash returns a self-describing encoded hash string in the standard
// Argon2id reference format:
//
//	$argon2id$v=19$m=<mem>,t=<iter>,p=<par>$<salt-b64>$<hash-b64>
//
// Encoding the parameters alongside the hash means a future change to the
// default cost parameters does not invalidate already-stored hashes:
// Verify reads the parameters back out of the stored string rather than
// assuming the process's current defaults.
func (h *PasswordHasher) Hash(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("crypto: password must not be empty")
	}
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("crypto: generating salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.MemoryKiB, h.params.Parallelism, h.params.KeyLength)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.MemoryKiB, h.params.Iterations, h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

// Verify reports whether password matches the given encoded hash. It uses
// the cost parameters embedded in the encoded hash, not the hasher's
// current defaults, so rotating ARGON2_* env vars does not break existing
// users' stored hashes — they keep verifying against what they were
// hashed with until the next password change re-hashes at the new cost.
func Verify(password, encoded string) (bool, error) {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, uint32(len(want)))
	// Constant-time comparison: a timing difference here would leak
	// information about how much of the hash matched.
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return true, nil
	}
	return false, nil
}

func decodeHash(encoded string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// parts[0] is "" (leading $), [1]=argon2id, [2]=v=.., [3]=m=..,t=..,p=..,
	// [4]=salt, [5]=hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: malformed version segment: %w", err)
	}
	if version != argon2.Version {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: unsupported argon2 version %d", version)
	}
	var params PasswordParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.MemoryKiB, &params.Iterations, &params.Parallelism); err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: malformed params segment: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: malformed salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("crypto: malformed hash: %w", err)
	}
	return params, salt, hash, nil
}

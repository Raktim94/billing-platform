package app

import (
	"fmt"
	"unicode/utf8"
)

// Password policy per brief §27: "minimum length policy, password
// strength validation, maximum sensible length, Unicode handling."
// Deliberately not a composition-rule policy (no forced
// uppercase/digit/symbol mix) — NIST SP 800-63B and current OWASP
// guidance both recommend length over composition rules, which tend to
// push users toward predictable substitutions (Password1! patterns)
// rather than genuinely stronger secrets.
const (
	minPasswordLength = 12
	// Argon2 hashes the whole input; an unbounded password is a cheap
	// denial-of-service vector against the hashing step itself, so cap
	// at a generous but finite length.
	maxPasswordLengthBytes = 256
)

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	if len(password) > maxPasswordLengthBytes {
		return fmt.Errorf("password must be at most %d bytes", maxPasswordLengthBytes)
	}
	return nil
}

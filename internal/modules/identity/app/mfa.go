package app

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"billing-platform/internal/modules/identity/domain"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/crypto"
	"billing-platform/internal/platform/permissions"
)

const recoveryCodeCount = 10

// verifyMFAForLogin checks a TOTP code first, and — since a recovery code
// and a 6-digit TOTP code are never valid values for each other for any
// remotely correct configuration — falls back to treating the supplied
// value as a recovery code, so a user who has lost their authenticator
// app can still log in with brief §28's recovery codes without a
// separate "use recovery code" UI mode.
func (s *Service) verifyMFAForLogin(ctx context.Context, userID uuid.UUID, code string, now time.Time) error {
	if code == "" {
		return domain.ErrMFARequired
	}

	secretRow, err := s.mfa.GetSecret(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading mfa secret: %w", err)
	}
	if !secretRow.Enabled {
		return domain.ErrMFAInvalid
	}

	secret, err := s.aead.Open(secretRow.SecretEncrypted, userID[:])
	if err != nil {
		return fmt.Errorf("decrypting mfa secret: %w", err)
	}

	if totp.Validate(code, string(secret)) {
		return nil
	}

	consumed, err := s.mfa.ConsumeRecoveryCode(ctx, userID, crypto.HashToken(code), now)
	if err != nil {
		return fmt.Errorf("checking recovery code: %w", err)
	}
	if consumed {
		return nil
	}

	return domain.ErrMFAInvalid
}

type EnrollMFAResult struct {
	Secret          string
	ProvisioningURI string
}

// EnrollMFA generates a new TOTP secret and stores it disabled — it only
// takes effect once VerifyMFAEnroll confirms the user actually captured
// it correctly (proves they can generate a valid code before login starts
// requiring one, avoiding a self-lockout).
func (s *Service) EnrollMFA(ctx context.Context, principal permissions.Principal, accountName, issuer string) (EnrollMFAResult, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return EnrollMFAResult{}, fmt.Errorf("identity: generating totp secret: %w", err)
	}

	sealed, err := s.aead.Seal([]byte(key.Secret()), principal.UserID[:])
	if err != nil {
		return EnrollMFAResult{}, fmt.Errorf("identity: sealing mfa secret: %w", err)
	}

	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return s.mfa.UpsertSecret(ctx, &domain.MFASecret{
			UserID:          principal.UserID,
			OrganisationID:  principal.OrganisationID,
			SecretEncrypted: sealed,
			Enabled:         false,
		})
	})
	if err != nil {
		return EnrollMFAResult{}, err
	}

	return EnrollMFAResult{Secret: key.Secret(), ProvisioningURI: key.URL()}, nil
}

// VerifyMFAEnroll confirms enrollment and issues one-time recovery codes
// (brief §28). Recovery codes are returned in plaintext exactly once —
// only their SHA-256 hashes are persisted.
func (s *Service) VerifyMFAEnroll(ctx context.Context, principal permissions.Principal, code string) ([]string, error) {
	var recoveryCodes []string
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		secretRow, err := s.mfa.GetSecret(ctx, principal.UserID)
		if err != nil {
			return fmt.Errorf("loading mfa secret: %w", err)
		}
		secret, err := s.aead.Open(secretRow.SecretEncrypted, principal.UserID[:])
		if err != nil {
			return fmt.Errorf("decrypting mfa secret: %w", err)
		}
		if !totp.Validate(code, string(secret)) {
			return domain.ErrMFAInvalid
		}

		if err := s.mfa.SetEnabled(ctx, principal.UserID, true); err != nil {
			return err
		}
		if err := s.users.SetMFAEnabled(ctx, principal.UserID, true); err != nil {
			return err
		}

		recoveryCodes, err = generateRecoveryCodes(recoveryCodeCount)
		if err != nil {
			return err
		}
		hashes := make([]string, len(recoveryCodes))
		for i, rc := range recoveryCodes {
			hashes[i] = crypto.HashToken(rc)
		}
		if err := s.mfa.ReplaceRecoveryCodes(ctx, principal.OrganisationID, principal.UserID, hashes); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID,
			ActorUserID:    &principal.UserID,
			ActorType:      audit.ActorUser,
			Action:         "mfa.enabled",
			EntityType:     "user",
			EntityID:       &principal.UserID,
			At:             s.now(),
		})
	})
	if err != nil {
		return nil, err
	}
	return recoveryCodes, nil
}

// DisableMFA requires the caller's current password as step-up
// confirmation (brief §28 "sensitive operations should support step-up
// authentication").
func (s *Service) DisableMFA(ctx context.Context, principal permissions.Principal, currentPassword string) error {
	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		user, err := s.users.GetByID(ctx, principal.UserID)
		if err != nil {
			return err
		}
		ok, err := crypto.Verify(currentPassword, user.PasswordHash)
		if err != nil || !ok {
			return domain.ErrInvalidCredentials
		}

		if err := s.mfa.SetEnabled(ctx, principal.UserID, false); err != nil {
			return err
		}
		if err := s.users.SetMFAEnabled(ctx, principal.UserID, false); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID,
			ActorUserID:    &principal.UserID,
			ActorType:      audit.ActorUser,
			Action:         "mfa.disabled",
			EntityType:     "user",
			EntityID:       &principal.UserID,
			At:             s.now(),
		})
	})
}

func generateRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, n)
	for i := range codes {
		buf := make([]byte, 5) // 8 base32 chars, ~40 bits — recovery codes are typed by hand, so shorter than a session token by design
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("identity: generating recovery code: %w", err)
		}
		codes[i] = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	}
	return codes, nil
}

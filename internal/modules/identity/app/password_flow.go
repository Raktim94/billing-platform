package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"rechvix/internal/modules/identity/domain"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/crypto"
	"rechvix/internal/platform/permissions"
)

// ChangePassword requires the caller's current password (brief §27) and,
// on success, revokes every session for the user — including the one
// making this request, forcing a fresh login. That's a deliberate,
// simpler-is-safer reading of "rotate/revoke appropriate sessions...
// optionally revoke all other sessions": revoking everything means there
// is never a stale session left holding old-password-era trust.
func (s *Service) ChangePassword(ctx context.Context, principal permissions.Principal, currentPassword, newPassword, confirmPassword string) error {
	if newPassword != confirmPassword {
		return domain.ErrPasswordConfirmMismatch
	}
	if err := validatePassword(newPassword); err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		user, err := s.users.GetByID(ctx, principal.UserID)
		if err != nil {
			return err
		}
		ok, err := crypto.Verify(currentPassword, user.PasswordHash)
		if err != nil || !ok {
			return domain.ErrInvalidCredentials
		}

		newHash, err := s.hasher.Hash(newPassword)
		if err != nil {
			return fmt.Errorf("hashing new password: %w", err)
		}
		now := s.now()
		if err := s.users.UpdatePasswordHash(ctx, principal.UserID, newHash, now); err != nil {
			return err
		}
		if err := s.sessions.RevokeAllForUser(ctx, principal.UserID, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID,
			ActorUserID:    &principal.UserID,
			ActorType:      audit.ActorUser,
			Action:         "user.password_changed",
			EntityType:     "user",
			EntityID:       &principal.UserID,
			At:             now,
		})
	})
}

const passwordResetTokenTTL = 30 * time.Minute

// RequestPasswordReset issues a reset token if, and only if, the email
// belongs to an active account — but ALWAYS returns (nil, nil) rather
// than an error either way, and the caller (HTTP handler) must respond
// identically regardless of the result, per brief §27's user-enumeration
// protection: "Forgot-password response must not reveal whether
// email/user exists." The issued token (if any) is returned so the
// caller can hand it to a notification provider — this module does not
// send email itself (that's Stage 9's notifications module).
type PasswordResetIssued struct {
	Token  string
	UserID uuid.UUID
	Email  string
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) (*PasswordResetIssued, error) {
	email = normalizeEmail(email)
	var issued *PasswordResetIssued

	err := s.pool.Run(ctx, func(ctx context.Context) error {
		lookup, err := s.users.LookupForAuth(ctx, email)
		if err != nil || lookup.Status != domain.UserStatusActive {
			return nil // silently do nothing — see enumeration-protection note above
		}

		if err := s.pool.SetOrganisationScope(ctx, lookup.OrganisationID); err != nil {
			return err
		}

		_, token, err := newSessionToken() // same primitive: 256-bit random, URL-safe
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generating password_reset_token id: %w", err)
		}
		now := s.now()
		if err := s.passwordResets.Create(ctx, &domain.PasswordResetToken{
			ID:             id,
			OrganisationID: lookup.OrganisationID,
			UserID:         lookup.ID,
			TokenHash:      crypto.HashToken(token),
			ExpiresAt:      now.Add(passwordResetTokenTTL),
			CreatedAt:      now,
		}); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			OrganisationID: lookup.OrganisationID,
			ActorUserID:    &lookup.ID,
			ActorType:      audit.ActorUser,
			Action:         "password_reset.requested",
			EntityType:     "user",
			EntityID:       &lookup.ID,
			At:             now,
		}); err != nil {
			return err
		}

		issued = &PasswordResetIssued{Token: token, UserID: lookup.ID, Email: email}
		return nil
	})
	return issued, err
}

// ResetPassword consumes a reset token and sets a new password. Like
// ChangePassword, it revokes every existing session for the user.
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword, confirmPassword string) error {
	if newPassword != confirmPassword {
		return domain.ErrPasswordConfirmMismatch
	}
	if err := validatePassword(newPassword); err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	return s.pool.Run(ctx, func(ctx context.Context) error {
		tokenRow, err := s.passwordResets.GetByTokenHash(ctx, crypto.HashToken(rawToken))
		if err != nil {
			return domain.ErrTokenInvalid
		}
		now := s.now()
		if tokenRow.UsedAt != nil || now.After(tokenRow.ExpiresAt) {
			return domain.ErrTokenInvalid
		}

		if err := s.pool.SetOrganisationScope(ctx, tokenRow.OrganisationID); err != nil {
			return err
		}

		newHash, err := s.hasher.Hash(newPassword)
		if err != nil {
			return fmt.Errorf("hashing new password: %w", err)
		}
		if err := s.users.UpdatePasswordHash(ctx, tokenRow.UserID, newHash, now); err != nil {
			return err
		}
		if err := s.passwordResets.MarkUsed(ctx, tokenRow.ID, now); err != nil {
			return err
		}
		if err := s.sessions.RevokeAllForUser(ctx, tokenRow.UserID, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: tokenRow.OrganisationID,
			ActorUserID:    &tokenRow.UserID,
			ActorType:      audit.ActorUser,
			Action:         "password_reset.completed",
			EntityType:     "user",
			EntityID:       &tokenRow.UserID,
			At:             now,
		})
	})
}

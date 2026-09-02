// Package domain holds the identity module's entity types, repository
// interfaces, and sentinel errors. No I/O, no framework imports
// (docs/architecture.md §2).
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "ACTIVE"
	UserStatusDisabled UserStatus = "DISABLED"
)

type User struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	Email                string
	FullName             string
	PasswordHash         string
	Status               UserStatus
	MFAEnabled           bool
	LastLoginAt          *time.Time
	LastPasswordChangeAt time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AuthLookup is the minimal projection returned by the SECURITY DEFINER
// auth_lookup_user_by_email() function (see
// migrations/0003_users.up.sql) — deliberately narrower than User,
// since this is the one query that runs before organisation scope is
// established.
type AuthLookup struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	PasswordHash   string
	Status         UserStatus
	MFAEnabled     bool
}

type Session struct {
	ID                uuid.UUID
	OrganisationID    uuid.UUID
	UserID            uuid.UUID
	TokenHash         string
	CreatedAt         time.Time
	LastSeenAt        time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
	IP                string
	UserAgent         string
}

type PasswordResetToken struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	UserID         uuid.UUID
	TokenHash      string
	ExpiresAt      time.Time
	UsedAt         *time.Time
	CreatedAt      time.Time
}

type MFASecret struct {
	UserID          uuid.UUID
	OrganisationID  uuid.UUID
	SecretEncrypted []byte
	Enabled         bool
}

// --- Repository interfaces (implemented in internal/modules/identity/pg) ---

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	// LookupForAuth calls auth_lookup_user_by_email — the one
	// intentionally organisation-unscoped read in this module. See that
	// function's comment in migrations/0003_users.up.sql.
	LookupForAuth(ctx context.Context, email string) (*AuthLookup, error)
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string, at time.Time) error
	UpdateLastLogin(ctx context.Context, userID uuid.UUID, at time.Time) error
	SetMFAEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error
}

type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Touch(ctx context.Context, id uuid.UUID, lastSeenAt time.Time) error
	Revoke(ctx context.Context, id uuid.UUID, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) error
	ListActiveForUser(ctx context.Context, userID uuid.UUID) ([]*Session, error)
}

type PasswordResetRepository interface {
	Create(ctx context.Context, t *PasswordResetToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

type MFARepository interface {
	UpsertSecret(ctx context.Context, secret *MFASecret) error
	GetSecret(ctx context.Context, userID uuid.UUID) (*MFASecret, error)
	SetEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error
	ReplaceRecoveryCodes(ctx context.Context, orgID, userID uuid.UUID, codeHashes []string) error
	// ConsumeRecoveryCode atomically marks one unused, matching code as
	// used and reports whether it found one — must be atomic (a
	// SELECT-then-UPDATE race would let the same recovery code be used
	// twice by concurrent requests).
	ConsumeRecoveryCode(ctx context.Context, userID uuid.UUID, codeHash string, at time.Time) (bool, error)
}

// RoleRepository is the minimal slice of the RBAC catalog identity's
// bootstrap use case needs: creating the organisation's starter system
// roles and granting the Owner role every permission that exists.
type RoleRepository interface {
	CreateRole(ctx context.Context, id, organisationID uuid.UUID, code, name string, isSystem bool, at time.Time) error
	GrantAllPermissions(ctx context.Context, roleID uuid.UUID) error
	AssignUserRole(ctx context.Context, id, organisationID, userID, roleID uuid.UUID, at time.Time) error
}

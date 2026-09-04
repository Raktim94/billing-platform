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

// apiKeyPrefixLen is how many characters of the raw key are stored in the
// clear (key_prefix) so a UI can show "bp_live_a1b2c3...", enough for a
// user to recognize which key is which without ever displaying the whole
// secret again — the same "shown once" principle as brief §36.
const apiKeyPrefixLen = 12

type CreateAPIKeyParams struct {
	Name      string
	Scopes    []domain.APIScope
	ExpiresAt *time.Time
	AllowedIP *string
}

// CreatedAPIKey carries the one-time-visible raw key alongside the
// persisted record. The caller must show RawKey to the user immediately
// and never retain it — APIKey.KeyHash is the only thing stored.
type CreatedAPIKey struct {
	APIKey domain.APIKey
	RawKey string
}

// CreateAPIKey requires apikeys.manage and rejects an empty or unknown
// scope list outright — an API key is never issued with a wildcard/
// all-permissions scope by default (brief §36); the caller must name
// exactly what this key can do.
func (s *Service) CreateAPIKey(ctx context.Context, principal permissions.Principal, p CreateAPIKeyParams) (CreatedAPIKey, error) {
	if err := s.permissions.Require(ctx, principal, "apikeys.manage", permissions.Scope{}); err != nil {
		return CreatedAPIKey{}, err
	}
	if len(p.Scopes) == 0 {
		return CreatedAPIKey{}, domain.ErrEmptyScopeList
	}
	for _, sc := range p.Scopes {
		if !domain.ValidScopes[sc] {
			return CreatedAPIKey{}, fmt.Errorf("%w: %q", domain.ErrUnknownScope, sc)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("identity: generating api key id: %w", err)
	}
	_, raw, err := crypto.RandomToken(32)
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("identity: generating api key: %w", err)
	}
	rawKey := "bp_live_" + raw
	prefix := rawKey
	if len(prefix) > apiKeyPrefixLen {
		prefix = prefix[:apiKeyPrefixLen]
	}

	key := domain.APIKey{
		ID:             id,
		OrganisationID: principal.OrganisationID,
		UserID:         principal.UserID,
		Name:           p.Name,
		KeyPrefix:      prefix,
		KeyHash:        crypto.HashToken(rawKey),
		Scopes:         p.Scopes,
		ExpiresAt:      p.ExpiresAt,
		AllowedIP:      p.AllowedIP,
		CreatedAt:      s.now(),
		CreatedBy:      principal.UserID,
	}

	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		if err := s.apiKeys.Create(ctx, &key); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID,
			ActorUserID:    &principal.UserID,
			ActorType:      audit.ActorUser,
			Action:         "apikey.create",
			EntityType:     "api_key",
			EntityID:       &key.ID,
			// Scopes and name only — never the key itself, hash included
			// (brief §60's "never log secrets" applies to this table's
			// hash column exactly as it does to a password hash).
			AfterState: map[string]any{"name": key.Name, "scopes": p.Scopes, "key_prefix": prefix},
			At:         key.CreatedAt,
		})
	})
	if err != nil {
		return CreatedAPIKey{}, err
	}
	return CreatedAPIKey{APIKey: key, RawKey: rawKey}, nil
}

// ValidateAPIKey resolves a raw bearer key to a Principal plus its scopes,
// or ErrAPIKeyInvalid for anything wrong (missing, malformed, unknown,
// expired, revoked) — deliberately one generic error, mirroring
// ValidateSession's "no detail about *why*" reasoning (brief §27 applies
// to API keys too: an attacker probing a stolen/guessed key should not be
// able to distinguish "wrong key" from "right key, expired" from "right
// key, wrong IP"). Called unscoped, like ValidateSession — see
// migrations/0025_api_keys.up.sql.
func (s *Service) ValidateAPIKey(ctx context.Context, rawKey, remoteIP string) (permissions.Principal, []domain.APIScope, error) {
	key, err := s.apiKeys.GetByHash(ctx, crypto.HashToken(rawKey))
	if err != nil {
		return permissions.Principal{}, nil, domain.ErrAPIKeyInvalid
	}
	now := s.now()
	if key.RevokedAt != nil {
		return permissions.Principal{}, nil, domain.ErrAPIKeyInvalid
	}
	if key.ExpiresAt != nil && now.After(*key.ExpiresAt) {
		return permissions.Principal{}, nil, domain.ErrAPIKeyInvalid
	}
	if key.AllowedIP != nil && *key.AllowedIP != "" && remoteIP != "" && *key.AllowedIP != remoteIP {
		return permissions.Principal{}, nil, domain.ErrAPIKeyInvalid
	}

	_ = s.apiKeys.Touch(ctx, key.ID, now) // best-effort, same as session Touch

	return permissions.Principal{UserID: key.UserID, OrganisationID: key.OrganisationID}, key.Scopes, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, principal permissions.Principal) ([]*domain.APIKey, error) {
	if err := s.permissions.Require(ctx, principal, "apikeys.manage", permissions.Scope{}); err != nil {
		return nil, err
	}
	var out []*domain.APIKey
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		out, err = s.apiKeys.ListActiveForOrganisation(ctx, principal.OrganisationID)
		return err
	})
	return out, err
}

func (s *Service) RevokeAPIKey(ctx context.Context, principal permissions.Principal, keyID uuid.UUID) error {
	if err := s.permissions.Require(ctx, principal, "apikeys.manage", permissions.Scope{}); err != nil {
		return err
	}
	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		if err := s.apiKeys.Revoke(ctx, keyID, s.now()); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID,
			ActorUserID:    &principal.UserID,
			ActorType:      audit.ActorUser,
			Action:         "apikey.revoke",
			EntityType:     "api_key",
			EntityID:       &keyID,
			At:             s.now(),
		})
	})
}

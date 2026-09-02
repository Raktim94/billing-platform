-- Queried only after login has already resolved organisation_id and set
-- app.current_organisation_id (see 0003_users.up.sql's
-- auth_lookup_user_by_email), so normal RLS is sufficient here — no
-- bootstrap problem like sessions/password_reset_tokens.
CREATE TABLE mfa_secrets (
    user_id           uuid PRIMARY KEY REFERENCES users(id),
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    -- AES-256-GCM sealed TOTP secret (internal/platform/crypto.AEAD),
    -- never stored or logged in plaintext.
    secret_encrypted  bytea NOT NULL,
    enabled           boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE mfa_secrets ENABLE ROW LEVEL SECURITY;
CREATE POLICY mfa_secrets_tenant_isolation ON mfa_secrets
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE mfa_recovery_codes (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    user_id           uuid NOT NULL REFERENCES users(id),
    -- SHA-256 of the recovery code (internal/platform/crypto.HashToken) —
    -- codes are shown to the user exactly once at generation time.
    code_hash         text NOT NULL,
    used_at           timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, code_hash)
);

CREATE INDEX idx_mfa_recovery_codes_user_id ON mfa_recovery_codes(user_id);

ALTER TABLE mfa_recovery_codes ENABLE ROW LEVEL SECURITY;
CREATE POLICY mfa_recovery_codes_tenant_isolation ON mfa_recovery_codes
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

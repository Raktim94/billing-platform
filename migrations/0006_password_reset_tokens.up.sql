-- Same bootstrap reasoning as sessions (0004_sessions.up.sql): the
-- "consume a reset link" request carries only the token, not an
-- organisation_id, so this table is deliberately not RLS-protected —
-- token_hash is a 256-bit random secret (unique) and IS the access
-- control, not organisation_id. The password-reset *request* step (before
-- a token exists) goes through auth_lookup_user_by_email instead.
CREATE TABLE password_reset_tokens (
    id              uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id),
    user_id         uuid NOT NULL REFERENCES users(id),
    token_hash      text NOT NULL UNIQUE,
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

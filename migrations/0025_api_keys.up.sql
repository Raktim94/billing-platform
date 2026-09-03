-- API keys (brief §36, docs/architecture.md §11). Same non-RLS shape as
-- sessions (migrations/0004) and the same reasoning: this table is looked
-- up BEFORE the request's organisation scope is known — that lookup is
-- what establishes it. key_hash equality (a 256-bit random value, via
-- internal/platform/crypto.RandomToken/HashToken) is the security boundary
-- here, exactly as it is for sessions.token_hash.
CREATE TABLE api_keys (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    -- The user this key acts as. An API key can never exercise a
    -- permission its owning user doesn't already hold — permissions.Checker
    -- intersects the key's declared scopes with the owning user's real RBAC
    -- grants (internal/platform/permissions/apikeyscope.go) — so a
    -- compromised key is bounded twice over, not just by its own scope
    -- list.
    user_id           uuid NOT NULL REFERENCES users(id),
    name              text NOT NULL,
    key_prefix        text NOT NULL,
    key_hash          text NOT NULL UNIQUE,
    -- Coarse scopes per brief §36's example list (products:read,
    -- inventory:read, customers:read, customers:write, invoices:read,
    -- invoices:write, reports:read), never defaulted to "everything" — the
    -- create-key flow (identity/app/apikey.go) requires an explicit,
    -- validated, non-empty list. Stored as text[] rather than a join table:
    -- this is a small, fixed vocabulary (unlike the RBAC permissions
    -- catalog, which genuinely grows), so a normalized table would add
    -- join overhead for no real benefit.
    scopes            text[] NOT NULL,
    expires_at        timestamptz,
    allowed_ip        text,
    last_used_at      timestamptz,
    revoked_at        timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_api_keys_organisation_id ON api_keys(organisation_id);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
-- Supports "list this user's active keys" without a full table scan,
-- mirroring idx_sessions_user_active (migrations/0004).
CREATE INDEX idx_api_keys_user_active ON api_keys(user_id) WHERE revoked_at IS NULL;

INSERT INTO permissions (code, module, description) VALUES
    ('apikeys.manage', 'identity', 'Create, view, and revoke API keys');

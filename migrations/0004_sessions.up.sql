-- The bearer value handed to the browser is never stored — only its
-- SHA-256 hash (token_hash), via internal/platform/crypto.HashToken. A
-- stolen database dump does not yield a reusable session token.
CREATE TABLE sessions (
    id                  uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    user_id             uuid NOT NULL REFERENCES users(id),
    token_hash          text NOT NULL UNIQUE,
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    idle_expires_at     timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz,
    -- Stored as text, not inet: nothing here does subnet-aware querying,
    -- and pgx v5 does not scan the inet binary wire format into a plain
    -- Go string (it wants netip.Addr) — using text avoids that friction
    -- for a column that's purely informational/audit metadata.
    ip                  text,
    user_agent          text
);

CREATE INDEX idx_sessions_organisation_id ON sessions(organisation_id);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
-- Supports the "session list per user" / "logout all devices" UI (brief
-- §29) without a full table scan.
CREATE INDEX idx_sessions_user_active ON sessions(user_id) WHERE revoked_at IS NULL;

-- Deliberately NOT organisation-scoped RLS, unlike every other tenant
-- table. Session resolution is the one query in the system that runs
-- *before* the app knows which organisation the caller belongs to — that
-- fact is what this table lookup exists to establish. Requiring
-- app.current_organisation_id to already be set here would make it
-- impossible to ever set it (a bootstrap deadlock), and the query already
-- has an equivalent security boundary of its own: `token_hash` is a
-- 256-bit random value (internal/platform/crypto.RandomToken), globally
-- unique, so `WHERE token_hash = $1` cannot return another tenant's
-- session no matter what the caller supplies. The application resolves
-- organisation_id/user_id from the matched row and only THEN issues
-- `SET LOCAL app.current_organisation_id` for every subsequent query in
-- that request — which is what makes RLS on branches/warehouses/users/etc.
-- meaningful in the first place.

CREATE TABLE users (
    id                      uuid PRIMARY KEY,
    organisation_id         uuid NOT NULL REFERENCES organisations(id),
    -- Globally unique, NOT per-organisation. v1 models one login identity
    -- per person, belonging to exactly one organisation (a person needing
    -- access to two businesses uses two email addresses for now; proper
    -- multi-organisation membership is a deliberately deferred feature,
    -- not an oversight). This is also what makes login possible at all:
    -- see auth_lookup_user_by_email() below — the login request carries
    -- only an email, not an organisation_id, so the very first lookup of
    -- the request necessarily happens before tenant context exists.
    email                   text NOT NULL UNIQUE,
    full_name               text NOT NULL,
    -- Self-describing Argon2id string produced by
    -- internal/platform/crypto.PasswordHasher.Hash — never plaintext,
    -- never a reversible encryption, never SHA/MD5 (brief §27).
    password_hash           text NOT NULL,
    status                  text NOT NULL DEFAULT 'ACTIVE'
                                CHECK (status IN ('ACTIVE', 'DISABLED')),
    mfa_enabled             boolean NOT NULL DEFAULT false,
    last_login_at           timestamptz,
    last_password_change_at timestamptz NOT NULL DEFAULT now(),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_organisation_id ON users(organisation_id);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY users_tenant_isolation ON users
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- The one deliberate, narrow bypass of users' RLS: login (and
-- forgot-password) only have an email address to work with, not an
-- organisation_id — that's exactly the fact this lookup exists to
-- establish, so requiring app.current_organisation_id to already be set
-- would make it impossible to ever discover. SECURITY DEFINER makes this
-- function execute with the privileges of its owner (whichever role runs
-- this migration); because that owner is the `users` table's owner too,
-- and the table does NOT have FORCE ROW LEVEL SECURITY, the owner
-- exemption applies and this one query sees all organisations' rows.
-- Every column it returns is exactly what internal/modules/identity needs
-- to complete a login or issue a password-reset token and nothing more
-- (no full row, no other users' data beyond the single matched email).
-- Immediately after calling this, the application must
-- `SET LOCAL app.current_organisation_id` from the returned
-- organisation_id before running any other query in the request — see
-- the deployment-requirement comment at the top of
-- 0001_organisation_hierarchy.up.sql.
CREATE FUNCTION auth_lookup_user_by_email(p_email text)
RETURNS TABLE (
    id               uuid,
    organisation_id  uuid,
    password_hash    text,
    status           text,
    mfa_enabled      boolean
)
LANGUAGE sql
SECURITY DEFINER
STABLE
AS $$
    SELECT u.id, u.organisation_id, u.password_hash, u.status, u.mfa_enabled
    FROM users u
    WHERE u.email = p_email;
$$;

-- Left at Postgres's default (EXECUTE granted to PUBLIC on a new
-- function) deliberately: migrations must not assume a specific runtime
-- role name already exists (that role is created as a deployment step,
-- documented in 0001_organisation_hierarchy.up.sql, not by these
-- migrations). This is safe under brief §37's model — no untrusted
-- client ever holds a direct PostgreSQL connection, only apps/server and
-- apps/worker do. An operator who provisions per-role grants more
-- strictly than that default should add
-- `REVOKE EXECUTE ... FROM PUBLIC; GRANT EXECUTE ... TO billing_app;`
-- as a deployment-specific follow-up, not by editing this migration.

-- A user's role assignment can be scoped down to a legal entity, branch,
-- or warehouse (brief §26: "Permissions may be scoped to: organisation,
-- legal entity, branch, warehouse"). NULL in a scoping column means
-- "not restricted at this level" (e.g. legal_entity_id NULL + branch_id
-- set means the grant applies to that branch regardless of which legal
-- entity currently owns it).
CREATE TABLE user_roles (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    user_id           uuid NOT NULL REFERENCES users(id),
    role_id           uuid NOT NULL REFERENCES roles(id),
    legal_entity_id   uuid REFERENCES legal_entities(id),
    branch_id         uuid REFERENCES branches(id),
    warehouse_id      uuid REFERENCES warehouses(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (user_id, role_id, legal_entity_id, branch_id, warehouse_id)
);

CREATE INDEX idx_user_roles_organisation_id ON user_roles(organisation_id);
CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_roles_tenant_isolation ON user_roles
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

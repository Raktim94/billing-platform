-- Immutable audit trail (brief §30). Application code only ever INSERTs
-- here (internal/platform/audit.Recorder) — there is no UPDATE/DELETE
-- path in the repository layer, and this migration grants no application
-- role UPDATE/DELETE privileges on it either (left at Postgres defaults
-- for the table owner; see the role-separation note in
-- 0001_organisation_hierarchy.up.sql — an operator wanting to harden this
-- further can REVOKE UPDATE, DELETE explicitly for their runtime role).
CREATE TABLE audit_log (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    actor_user_id    uuid REFERENCES users(id),
    actor_type       text NOT NULL CHECK (actor_type IN ('USER', 'SYSTEM', 'API_KEY')),
    action           text NOT NULL,
    entity_type      text NOT NULL,
    entity_id        uuid,
    -- Snapshots of the affected row before/after the action, for
    -- change-diff display. Never populated with password hashes, MFA
    -- secrets, or other raw secrets (brief §30) — enforced by convention
    -- in internal/platform/audit callers, not by a database constraint,
    -- since the set of sensitive columns varies per entity_type.
    before_state     jsonb,
    after_state      jsonb,
    -- text, not inet — see the same note in
    -- migrations/0004_sessions.up.sql.
    ip               text,
    user_agent       text,
    request_id       text,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_organisation_id ON audit_log(organisation_id, created_at DESC);
CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_actor_user_id ON audit_log(actor_user_id);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_tenant_isolation ON audit_log
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

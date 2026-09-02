-- Tenancy hierarchy per docs/architecture.md §4:
--   organisations -> legal_entities -> branches -> warehouses
--
-- Row-Level Security pattern (repeated on every tenant-owned table added
-- from here on): the application sets a per-transaction session variable
--
--     SET LOCAL app.current_organisation_id = '<uuid>';
--
-- once per request/transaction, sourced from the authenticated session —
-- never from client-supplied input (brief Rule 5). Every tenant table gets
--
--     ALTER TABLE t ENABLE ROW LEVEL SECURITY;
--     CREATE POLICY t_tenant_isolation ON t
--       USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
--       WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);
--
-- current_setting(..., true) (the `true` = "missing_ok") returns NULL the
-- FIRST time a session ever asks for this GUC without having set it. But
-- once ANY transaction on a given connection has called set_config on it
-- even once (RunScoped does, every time), Postgres keeps a placeholder
-- for the rest of that connection's life — a later transaction that never
-- itself sets the value gets '' (empty string), not NULL, because pgxpool
-- reuses connections across transactions. Casting '' directly to ::uuid
-- raises an error (not "no match"), which aborts the whole transaction —
-- the opposite of fail-closed, it's fail-loud in a way that looks like a
-- database bug rather than "no permission." NULLIF(..., '') normalizes
-- both cases (truly-never-set, and reverted-to-empty-on-a-reused-
-- connection) to NULL before the cast, so organisation_id = NULL is
-- reliably false either way and the fail mode stays "see nothing" (fail
-- closed), not "see everything" and not "500 error." Confirmed by
-- TestRLS_UnsetScopeSeesNothing in tests/integration, which reproduces
-- the reused-connection case directly.
--
-- DEPLOYMENT REQUIREMENT — read before provisioning the database role:
-- RLS policies do not apply to a table's owning role unless the table has
-- FORCE ROW LEVEL SECURITY set (and even then, `SET row_security = off`
-- inside a same-owner SECURITY DEFINER function re-bypasses it — see the
-- auth_lookup_user_by_email() function in 0003_users.up.sql, which
-- deliberately relies on exactly that owner-exemption to solve the
-- "look up a user by email before we know their organisation" bootstrap
-- problem). That escape hatch is only safe if it is narrow and
-- deliberate, which means these tables must NOT set FORCE, and — this is
-- the actual requirement — the runtime application (apps/server,
-- apps/worker) MUST connect to PostgreSQL as a role that is NOT the
-- owner of these tables. Run migrations as a dedicated owning role (e.g.
-- `billing_migrator`); grant the runtime role (e.g. `billing_app`) only
-- SELECT/INSERT/UPDATE/DELETE plus EXECUTE on the specific SECURITY
-- DEFINER functions it needs. If the runtime role IS the table owner,
-- every RLS policy in this schema is silently a no-op for it and tenant
-- isolation reduces to "hope the application layer never has a bug" —
-- exactly the single point of failure RLS exists to protect against
-- (docs/architecture.md §10). internal/platform/database logs a
-- startup warning if it detects this misconfiguration (best-effort check,
-- not a substitute for correct provisioning) — see docs/operations/.
--
-- This is defense-in-depth alongside the application/repository-layer
-- scoping (docs/architecture.md §10) — either layer alone should already
-- prevent cross-tenant access; RLS means a bug in the other layer isn't
-- enough by itself to leak data (Scenario G).

CREATE TABLE organisations (
    id                     uuid PRIMARY KEY,
    name                   text NOT NULL,
    default_currency_code  varchar(3) NOT NULL,
    default_timezone       text NOT NULL DEFAULT 'UTC',
    status                 text NOT NULL DEFAULT 'ACTIVE'
                               CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE organisations ENABLE ROW LEVEL SECURITY;
CREATE POLICY organisations_tenant_isolation ON organisations
    USING (id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE legal_entities (
    id                   uuid PRIMARY KEY,
    organisation_id      uuid NOT NULL REFERENCES organisations(id),
    legal_name           text NOT NULL,
    -- ISO 3166-1 alpha-2. Country-specific tax registration (GSTIN etc.)
    -- deliberately does NOT live on this table — that's gstindia-module
    -- (or a future country pack's) territory, per docs/architecture.md §8:
    -- "Implement India's GST as a country tax plugin ... NOT as
    -- assumptions inside generic invoice code."
    country_code         char(2) NOT NULL,
    base_currency_code   varchar(3) NOT NULL,
    status               text NOT NULL DEFAULT 'ACTIVE'
                             CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_legal_entities_organisation_id ON legal_entities(organisation_id);

ALTER TABLE legal_entities ENABLE ROW LEVEL SECURITY;
CREATE POLICY legal_entities_tenant_isolation ON legal_entities
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE branches (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    legal_entity_id   uuid NOT NULL REFERENCES legal_entities(id),
    -- Short business code used in document-numbering series
    -- (docs/architecture.md §51 scope), e.g. "CUT", "HQ".
    code              text NOT NULL,
    name              text NOT NULL,
    timezone          text NOT NULL DEFAULT 'UTC',
    status            text NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, code)
);

CREATE INDEX idx_branches_organisation_id ON branches(organisation_id);
CREATE INDEX idx_branches_legal_entity_id ON branches(legal_entity_id);

ALTER TABLE branches ENABLE ROW LEVEL SECURITY;
CREATE POLICY branches_tenant_isolation ON branches
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE warehouses (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    branch_id         uuid NOT NULL REFERENCES branches(id),
    code              text NOT NULL,
    name              text NOT NULL,
    status            text NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, code)
);

CREATE INDEX idx_warehouses_organisation_id ON warehouses(organisation_id);
CREATE INDEX idx_warehouses_branch_id ON warehouses(branch_id);

ALTER TABLE warehouses ENABLE ROW LEVEL SECURITY;
CREATE POLICY warehouses_tenant_isolation ON warehouses
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

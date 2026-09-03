-- Grants ordinary read/write privileges (never ownership) to the runtime
-- role apps/server and apps/worker should connect as in production, per
-- the DEPLOYMENT REQUIREMENT documented in
-- migrations/0001_organisation_hierarchy.up.sql: the runtime role must
-- NOT be the owner of these tables, or every RLS policy in this schema
-- becomes a silent no-op for it.
--
-- The role itself is created separately (deploy/compose/postgres-init/ —
-- a Postgres docker-entrypoint-initdb.d script, since role creation has
-- to happen before this migration runs, using a name/password the
-- deployment controls via its own secrets, not a literal in this file).
-- This migration only grants privileges on objects that exist by the time
-- it runs — it is intentionally the LAST migration, so ALTER DEFAULT
-- PRIVILEGES here also covers every table/sequence/function created by
-- every migration before it, and needs updating only if a future
-- migration adds a schema OTHER than `public`.
--
-- If the `billing_app` role doesn't exist (e.g. a deployment that
-- deliberately runs everything as one role — not recommended, but not
-- this migration's job to forbid), these GRANTs simply target a
-- nonexistent role. Postgres errors on that, so this migration uses
-- DO-block guards to skip gracefully instead of failing the whole
-- migration run over an optional security hardening step.
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'billing_app') THEN
        EXECUTE 'GRANT USAGE ON SCHEMA public TO billing_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO billing_app';
        EXECUTE 'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO billing_app';
        EXECUTE 'GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO billing_app';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO billing_app';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO billing_app';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO billing_app';
    END IF;
END $$;

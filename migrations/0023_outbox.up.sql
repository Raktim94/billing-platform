-- Generic transactional outbox (docs/architecture.md §34, brief §12/§33).
-- Stage 8 needs this first for e-Invoice/e-Way Bill submission (a
-- government API call must never be in the same transaction as
-- sales.FinalizeDocument, and must never be lost if the process crashes
-- between finalize and submission), but the table is deliberately generic
-- (event_type + jsonb payload) rather than a special-purpose
-- "einvoice_queue" — Stage 9's WhatsApp/email/webhook delivery reuses this
-- same table instead of building a second outbox.

CREATE TABLE outbox_events (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    event_type        text NOT NULL,
    payload           jsonb NOT NULL,
    -- Enqueue-time idempotency: FinalizeDocument writes this row inside its
    -- own transaction, so a normal retry of the whole FinalizeDocument call
    -- can't happen (the document is no longer DRAFT the second time) — this
    -- key is a second, cheap backstop against any future caller that enqueues
    -- the same logical event twice (e.g. a manual "resend to IRP" action).
    idempotency_key   text NOT NULL,
    status            text NOT NULL DEFAULT 'PENDING'
                       CHECK (status IN ('PENDING', 'PROCESSING', 'DONE', 'FAILED')),
    attempts          integer NOT NULL DEFAULT 0,
    next_attempt_at   timestamptz NOT NULL DEFAULT now(),
    last_error        text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, idempotency_key)
);

CREATE INDEX idx_outbox_events_organisation_id ON outbox_events(organisation_id);
-- The worker's poll query: "next PENDING or retry-due FAILED row, oldest
-- first" — a partial index so DONE rows (the overwhelming majority over
-- time) never bloat the index the poller actually scans.
CREATE INDEX idx_outbox_events_poll ON outbox_events(next_attempt_at)
    WHERE status IN ('PENDING', 'FAILED');

ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY outbox_events_tenant_isolation ON outbox_events
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- The one deliberate, narrow bypass of outbox_events' RLS, same pattern and
-- same reasoning as migrations/0003_users.up.sql's auth_lookup_user_by_email:
-- apps/worker is a background process serving every organisation in this
-- deployment, so its poll loop must discover "what's next across ALL
-- tenants" before it has any single organisation_id to scope a normal
-- RunScoped transaction to — that's exactly the fact this function exists
-- to establish. SECURITY DEFINER makes it run with its owner's (the
-- migration-running role's) privileges, which — because that owner is also
-- outbox_events' table owner, and the table has no FORCE ROW LEVEL
-- SECURITY — bypasses RLS for this one, fixed-shape, non-parameterized
-- query. It claims (UPDATEs to PROCESSING, atomically, skipping rows another
-- concurrent worker already locked) and returns at most one row — never an
-- arbitrary caller-chosen slice of another tenant's data. Immediately after
-- calling this, the worker must RunScoped(ctx, claimed.OrganisationID, ...)
-- before doing anything else with the claimed event, so every subsequent
-- read/write (loading the sales document, writing an einvoice_records row,
-- marking this same outbox row DONE/FAILED) goes through ordinary
-- tenant-scoped RLS like everything else in this codebase.
-- RETURNS SETOF (not a single outbox_events row) so "nothing to claim" is
-- unambiguously zero result rows — a caller checks pgx.ErrNoRows, not a
-- fabricated all-NULL composite (which a plain RETURNS outbox_events with
-- an UPDATE...RETURNING INTO on zero matched rows would otherwise produce,
-- and which is awkward/ambiguous to distinguish from a real error when
-- scanning a non-nullable id column in Go).
CREATE FUNCTION outbox_claim_next() RETURNS SETOF outbox_events
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    UPDATE outbox_events
    SET status = 'PROCESSING', attempts = attempts + 1, updated_at = now()
    WHERE id = (
        SELECT id FROM outbox_events
        WHERE status IN ('PENDING', 'FAILED') AND next_attempt_at <= now()
        ORDER BY created_at
        FOR UPDATE SKIP LOCKED
        LIMIT 1
    )
    RETURNING *;
END;
$$;

-- Left at Postgres's default (EXECUTE granted to PUBLIC on a new function)
-- for the same reason migrations/0003_users.up.sql's lookup function is:
-- migrations must not assume a specific runtime role name already exists.
-- No untrusted client ever holds a direct PostgreSQL connection (brief
-- §37) — only apps/server and apps/worker do, and apps/worker is the only
-- one that should ever call this. An operator provisioning per-role grants
-- more strictly than the default should
-- `REVOKE EXECUTE ... FROM PUBLIC; GRANT EXECUTE ... TO billing_app;`
-- as a deployment-specific follow-up.

-- Webhooks (brief §38, docs/architecture.md). Delivery is queued through
-- the Stage 8 transactional outbox (internal/platform/outbox) — this
-- migration adds no second queue table, only the subscription registry
-- (webhook_endpoints) and a delivery-attempt log for operator visibility
-- (webhook_deliveries; the outbox row itself already carries the
-- authoritative status/attempts/next-retry state, this table is a
-- human-readable history of what was actually sent/received per attempt).
CREATE TABLE webhook_endpoints (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    url               text NOT NULL,
    -- HMAC-SHA256 signing secret, generated once at registration, shown
    -- once (brief §38) — stored as its raw bytes are never displayed
    -- again, so unlike api_keys.key_hash this IS the value used to sign
    -- every delivery, not just an equality check; it must be readable by
    -- the webhook handler, so it is NOT a one-way hash. It is not
    -- credential-grade secret (it doesn't grant access to anything, only
    -- lets the far end verify authenticity), so plain storage (RLS-scoped
    -- to the owning organisation) is the right tradeoff, not
    -- crypto.AEAD-at-rest encryption — that's reserved for genuine
    -- third-party API credentials (brief §60), which this is not.
    signing_secret    text NOT NULL,
    -- brief §38's event catalog. A generated CHECK (not a foreign-keyed
    -- lookup table) since this is a small, code-defined vocabulary that
    -- changes only when a developer wires a new event source, not runtime
    -- configuration data.
    subscribed_events text[] NOT NULL CHECK (
        subscribed_events <@ ARRAY[
            'invoice.created','invoice.finalized','invoice.cancelled',
            'payment.created','payment.received',
            'stock.changed','stock.low',
            'customer.created',
            'einvoice.generated','einvoice.failed',
            'ewaybill.generated','ewaybill.failed'
        ]::text[]
    ),
    is_active         boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_webhook_endpoints_organisation_id ON webhook_endpoints(organisation_id);

ALTER TABLE webhook_endpoints ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_endpoints_tenant_isolation ON webhook_endpoints
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE webhook_deliveries (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    webhook_endpoint_id uuid NOT NULL REFERENCES webhook_endpoints(id),
    event_type       text NOT NULL,
    event_id         uuid NOT NULL,
    http_status      integer,
    succeeded        boolean NOT NULL,
    error_message    text,
    attempted_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_deliveries_endpoint_id ON webhook_deliveries(webhook_endpoint_id, attempted_at DESC);

ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_deliveries_tenant_isolation ON webhook_deliveries
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

INSERT INTO permissions (code, module, description) VALUES
    ('webhooks.manage', 'integrations', 'Register, view, and revoke webhook endpoints');

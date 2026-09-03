-- Notifications / document sharing (brief §20-21). Actual send/deliver
-- work is queued through the Stage 8 outbox (event_type
-- 'notification.send'), same as e-Invoice and webhook delivery — a slow or
-- down WhatsApp/email/SMS provider can never block whatever triggered the
-- share.
--
-- Same bootstrap reasoning as sessions (0004) and password_reset_tokens
-- (0006): redeeming a share link carries only the bearer token, not an
-- organisation_id — this table is deliberately not RLS-protected,
-- token_hash (a 256-bit random secret, via
-- internal/platform/crypto.RandomToken/HashToken) IS the access control.
-- Authenticated operations (list/revoke a document's share links) filter
-- by organisation_id explicitly in the application query, the same way
-- sessions.ListActiveForUser filters by user_id without relying on RLS.
CREATE TABLE share_links (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    -- What's being shared (e.g. 'sales_document') and its id — generic
    -- rather than a sales_document_id FK, since this is meant to cover any
    -- printable document type (brief §19/§21), not just invoices.
    document_type    text NOT NULL,
    document_id      uuid NOT NULL,
    token_hash       text NOT NULL UNIQUE,
    expires_at       timestamptz NOT NULL,
    revoked_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_share_links_organisation_id ON share_links(organisation_id);
CREATE INDEX idx_share_links_document ON share_links(organisation_id, document_type, document_id);

-- Who shared what document to whom via which provider with what status
-- (brief §20's privacy/audit requirement) is recorded through
-- internal/platform/audit (Stage 2), not a second bespoke log table here —
-- audit_log already carries organisation_id/actor/action/entity/before-
-- after/timestamp, which is exactly this shape (action =
-- 'notification.share', entity_type = document_type). Recipient phone
-- numbers/emails go in audit_log.after as opaque JSON, protected by the
-- same audit.view permission as every other sensitive action (brief §20's
-- "access must be permission controlled").

INSERT INTO permissions (code, module, description) VALUES
    ('notifications.share', 'notifications', 'Share a document via email, SMS, or WhatsApp, and create/revoke share links');

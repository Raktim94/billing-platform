-- e-Invoice / e-Way Bill persistence (docs/architecture.md §9, brief §9-10).
-- Sandbox-only in Stage 8 (docs/research.md) — status machine and field
-- list are the government-facing shape regardless of which environment
-- (sandbox vs. eventual production) a provider adapter targets.

CREATE TABLE einvoice_records (
    id                    uuid PRIMARY KEY,
    organisation_id       uuid NOT NULL REFERENCES organisations(id),
    -- One e-invoice per sales document, ever, in v1 — this UNIQUE
    -- constraint IS the idempotency backstop at the database level: two
    -- concurrent/duplicate attempts to create the initial record for the
    -- same document can only ever result in one row (the second INSERT
    -- fails / is a no-op via ON CONFLICT), so a re-processed outbox event
    -- can never produce two IRNs for one invoice. Real-world GST doesn't
    -- allow silently re-issuing an IRN for the same document either — a
    -- cancelled e-invoice needs a fresh document, not a second record here.
    sales_document_id     uuid NOT NULL UNIQUE REFERENCES sales_documents(id),
    provider              text NOT NULL,
    status                text NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
        'DRAFT', 'QUEUED', 'SUBMITTING', 'GENERATED',
        'FAILED_RETRYABLE', 'FAILED_FINAL', 'CANCEL_PENDING', 'CANCELLED'
    )),
    request_version       text NOT NULL DEFAULT 'v1',
    request_payload_hash  text,
    response_payload      jsonb,
    irn                   text,
    ack_number            text,
    ack_date              timestamptz,
    signed_invoice        text,
    signed_qr_payload     text,
    error_code            text,
    error_message         text,
    correlation_id        text,
    cancelled_at          timestamptz,
    cancel_reason         text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_einvoice_records_organisation_id ON einvoice_records(organisation_id);
CREATE INDEX idx_einvoice_records_irn ON einvoice_records(irn) WHERE irn IS NOT NULL;

ALTER TABLE einvoice_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY einvoice_records_tenant_isolation ON einvoice_records
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE ewaybill_records (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    sales_document_id  uuid NOT NULL REFERENCES sales_documents(id),
    einvoice_record_id uuid REFERENCES einvoice_records(id),
    irn                text,
    ewb_number         text,
    status             text NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
        'DRAFT', 'QUEUED', 'SUBMITTING', 'GENERATED',
        'FAILED_RETRYABLE', 'FAILED_FINAL', 'CANCEL_PENDING', 'CANCELLED',
        -- CLOSED is distinct from CANCELLED: the 2026-08-01 GSTN change
        -- (docs/research.md) lets the supplier/recipient/transporter/driver
        -- voluntarily close an EWB after delivery — the shipment happened,
        -- the EWB just stops being "in transit" — whereas CANCELLED means
        -- the underlying movement never happened at all.
        'CLOSED'
    )),
    valid_from         timestamptz,
    valid_until        timestamptz,
    -- Ship-to GSTIN: mandatory wherever ship-to details are present and an
    -- EWB is required, per the 2026-08-01 GSTN advisory. NULL means "not
    -- yet resolved"; the literal string 'URP' (Unregistered Person) is a
    -- valid, real value here, not a placeholder — so this is unconstrained
    -- text, not a GSTIN-format CHECK, deliberately.
    ship_to_gstin      text,
    transporter_id     text,
    transporter_name   text,
    vehicle_number     text,
    distance_km        numeric(10,2),
    -- Each Part-B update (vehicle/transporter change en route) appended as
    -- one element — history, not just the latest value, per brief §10's
    -- "Part-B history" field.
    part_b_history     jsonb NOT NULL DEFAULT '[]'::jsonb,
    closed_at          timestamptz,
    closed_by_role     text CHECK (closed_by_role IN ('SUPPLIER', 'RECIPIENT', 'TRANSPORTER', 'DRIVER')),
    response_payload   jsonb,
    error_code         text,
    error_message      text,
    correlation_id     text,
    cancelled_at       timestamptz,
    cancel_reason      text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ewaybill_records_organisation_id ON ewaybill_records(organisation_id);
CREATE INDEX idx_ewaybill_records_sales_document_id ON ewaybill_records(sales_document_id);
CREATE INDEX idx_ewaybill_records_ewb_number ON ewaybill_records(ewb_number) WHERE ewb_number IS NOT NULL;

ALTER TABLE ewaybill_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY ewaybill_records_tenant_isolation ON ewaybill_records
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Provider credentials (client id/secret, GSP token, etc.) — encrypted at
-- rest via internal/platform/crypto's AEAD helper (brief §9/§60: never
-- plaintext, never logged, never returned through any API response). One
-- set of credentials per (organisation, legal entity, provider) since a
-- GSTIN's e-invoicing credentials are typically issued per registration.
CREATE TABLE einvoice_provider_credentials (
    id                     uuid PRIMARY KEY,
    organisation_id        uuid NOT NULL REFERENCES organisations(id),
    legal_entity_id        uuid NOT NULL REFERENCES legal_entities(id),
    provider               text NOT NULL,
    encrypted_credentials  bytea NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, legal_entity_id, provider)
);

ALTER TABLE einvoice_provider_credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY einvoice_provider_credentials_tenant_isolation ON einvoice_provider_credentials
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

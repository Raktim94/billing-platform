-- Free-first e-Way Bill portal workflow (docs/architecture.md §9b, Stage
-- 8c). Extends ewaybill_records — one table, one canonical model, for both
-- the FREE_PORTAL and AUTOMATIC_API modes, per §9b's explicit "not two
-- parallel schemas" instruction. Does not touch einvoice_records at all.

ALTER TABLE ewaybill_records DROP CONSTRAINT ewaybill_records_status_check;
ALTER TABLE ewaybill_records ADD CONSTRAINT ewaybill_records_status_check CHECK (status IN (
    -- Existing AUTOMATIC_API states (Stage 8, unchanged):
    'DRAFT', 'QUEUED', 'SUBMITTING', 'GENERATED',
    'FAILED_RETRYABLE', 'FAILED_FINAL', 'CANCEL_PENDING', 'CANCELLED', 'CLOSED',
    -- New FREE_PORTAL states (docs/architecture.md §9b):
    'NOT_REQUIRED', 'NEEDS_INFORMATION', 'READY', 'PREPARING',
    'PORTAL_FILE_READY', 'AWAITING_PORTAL_COMPLETION', 'EXPIRED'
));

ALTER TABLE ewaybill_records ADD COLUMN mode text NOT NULL DEFAULT 'AUTOMATIC_API'
    CHECK (mode IN ('FREE_PORTAL', 'AUTOMATIC_API'));

-- source distinguishes how a GENERATED record's EWB number was actually
-- obtained — the free-portal path has three real paths (manual entry is
-- the universal fallback; imported-file/PDF are conveniences on top of it),
-- the API path always uses 'API'.
ALTER TABLE ewaybill_records ADD COLUMN source text
    CHECK (source IN ('API', 'MANUAL_PORTAL', 'IMPORTED_FILE'));

-- canonical_snapshot is captured ONCE, the first time a FREE_PORTAL record
-- is prepared for a sales document, from that document's own immutable
-- tax/inventory snapshot (sales_documents + tax_documents/tax_lines, all
-- already finalized-and-frozen per brief §55) plus a point-in-time copy of
-- supplier/customer identity fields (name, GSTIN, address) as they were at
-- that moment. Every subsequent regeneration (file lost, user re-opens the
-- workflow) re-serializes THIS stored jsonb — never re-derives from live
-- catalogue/contacts data — which is what makes "edit the customer's
-- address after finalization, confirm the prepared file still shows the
-- old address" hold true (docs/architecture.md §9b).
ALTER TABLE ewaybill_records ADD COLUMN canonical_snapshot jsonb;
ALTER TABLE ewaybill_records ADD COLUMN prepared_file_name text;
ALTER TABLE ewaybill_records ADD COLUMN prepared_at timestamptz;

CREATE INDEX idx_ewaybill_records_mode ON ewaybill_records(mode);

-- Versioned e-Way Bill eligibility rules — global/government-set data, same
-- reasoning Stage 5a applied to gst_state_codes: this is a national/state
-- legal threshold, not something an individual organisation should be able
-- to silently override in a way that could mask a genuine legal
-- requirement. valid_from/valid_until mirror tax_rate_master's pattern
-- (docs/architecture.md §5) so a threshold change is a new dated row, and
-- a past invoice's eligibility evaluation (if ever re-run) uses the rule
-- that was in force on ITS date, not today's.
CREATE TABLE ewaybill_eligibility_rules (
    id                     uuid PRIMARY KEY,
    -- NULL state_code = the default/national rule; a non-NULL value
    -- overrides it for movements with that place-of-supply state (several
    -- states set a higher intra-state-only threshold in practice — this
    -- column exists so that can be modeled as data later without a schema
    -- change, even though only the national default is seeded today).
    state_code             text,
    min_consignment_value  numeric(20,6) NOT NULL,
    valid_from             date NOT NULL,
    valid_until            date,
    description            text NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ewaybill_eligibility_rules_validity ON ewaybill_eligibility_rules(valid_from, valid_until);

-- Seed the widely-cited national default: e-Way Bill required for
-- inter-state movement of goods with consignment value exceeding ₹50,000
-- (national e-Way Bill system effective 2018-04-01). This is a starting
-- default, NOT a substitute for the operator verifying the current rule —
-- see the doc comment on eligibility.Rule in Go and docs/TODO.md's Stage 8c
-- entry, which flags this explicitly as unverified-against-live-government-
-- documentation, consistent with brief Rule 2 ("never invent tax rules").
INSERT INTO ewaybill_eligibility_rules (id, state_code, min_consignment_value, valid_from, valid_until, description)
VALUES (
    '01996e6a-0000-7000-8000-000000000001',
    NULL,
    50000,
    '2018-04-01',
    NULL,
    'National default e-Way Bill consignment-value threshold — VERIFY against current CBIC/GST notifications before relying on this in production; several states set different intra-state thresholds not yet modeled here.'
);

-- Portal bulk-upload schema versioning (docs/architecture.md §9b) — a
-- format change is a new row + a new mapper version in Go, never a rewrite
-- of invoice logic. active marks which version PortalExportProvider uses
-- by default; only one row should be active at a time (enforced at the
-- application layer, not a DB constraint, since "exactly one active row"
-- as a partial unique index adds complexity this single-operator-seeded
-- table doesn't yet need).
CREATE TABLE ewaybill_portal_schema_versions (
    id               uuid PRIMARY KEY,
    version          text NOT NULL UNIQUE,
    effective_from   date NOT NULL,
    effective_until  date,
    active           boolean NOT NULL DEFAULT false,
    notes            text,
    created_at       timestamptz NOT NULL DEFAULT now()
);

INSERT INTO ewaybill_portal_schema_versions (id, version, effective_from, active, notes)
VALUES (
    '01996e6a-0000-7000-8000-000000000002',
    'v1-placeholder',
    '2026-09-03',
    true,
    'Structurally reasonable placeholder for the official government bulk-upload EWB JSON shape (top-level version tag + itemized array), NOT verified against a live official sample or current NIC documentation — see docs/TODO.md Stage 8c and internal/modules/ewaybill/portal/v1 for the explicit caveat. Replace with a verified schema version before relying on this against a real government portal upload.'
);

-- Vehicle and transporter master data (docs/architecture.md §9b) — plain
-- org-scoped reference data, same RLS pattern as every other tenant table.
CREATE TABLE vehicles (
    id                      uuid PRIMARY KEY,
    organisation_id         uuid NOT NULL REFERENCES organisations(id),
    registration_number     text NOT NULL,
    nickname                text,
    vehicle_type            text,
    default_transport_mode  text,
    active                  boolean NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, registration_number)
);

CREATE INDEX idx_vehicles_organisation_id ON vehicles(organisation_id);

ALTER TABLE vehicles ENABLE ROW LEVEL SECURITY;
CREATE POLICY vehicles_tenant_isolation ON vehicles
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE transporters (
    id                      uuid PRIMARY KEY,
    organisation_id         uuid NOT NULL REFERENCES organisations(id),
    name                    text NOT NULL,
    transporter_id          text,
    gstin                   text,
    phone                   text,
    address                 text,
    default_transport_mode  text,
    active                  boolean NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_transporters_organisation_id ON transporters(organisation_id);

ALTER TABLE transporters ENABLE ROW LEVEL SECURITY;
CREATE POLICY transporters_tenant_isolation ON transporters
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Tracks which (vehicle, transporter) a customer's invoices most recently/
-- frequently used — backs the "smart defaults" resolver (§9b) with a plain
-- recency/frequency query rather than a new architectural mechanism.
CREATE TABLE customer_transport_preferences (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    customer_party_id uuid NOT NULL REFERENCES parties(id),
    vehicle_id        uuid REFERENCES vehicles(id),
    transporter_id    uuid REFERENCES transporters(id),
    used_count        integer NOT NULL DEFAULT 1,
    last_used_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, customer_party_id, vehicle_id, transporter_id)
);

CREATE INDEX idx_customer_transport_preferences_lookup
    ON customer_transport_preferences(organisation_id, customer_party_id, last_used_at DESC);

ALTER TABLE customer_transport_preferences ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_transport_preferences_tenant_isolation ON customer_transport_preferences
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- ewaybill.generate/ewaybill.cancel already exist (migrations/0002) and
-- are reused for the free-portal prepare/manual-entry/import actions —
-- deliberately not a new ewaybill_portal.* namespace, same lesson as
-- Stage 4's purchase.* vs purchases.* duplication mistake.
INSERT INTO permissions (code, module, description) VALUES
    ('logistics.view',   'logistics', 'View vehicles and transporters'),
    ('logistics.manage', 'logistics', 'Create, edit, and deactivate vehicles and transporters');

-- Generic tax calculation snapshot (docs/architecture.md §5, brief §18/§55).
-- This is the OUTPUT side of tax calculation — country-agnostic by design.
-- No column here is named cgst/sgst/igst; tax_components.component_type is
-- a free-form string ('CGST', 'SGST', 'IGST', 'UTGST', 'CESS' today; a
-- future VAT/sales-tax engine adds new values here, not new columns).
--
-- A tax_document is written once, at the point a business document (a
-- sales invoice, once Stage 5b exists) is finalized, and never
-- recalculated later even if tax_rate_master changes afterward (brief §7)
-- — reference_type/reference_id link back to whatever finalized the
-- calculation, same generic-reference pattern as stock_movements.

CREATE TABLE tax_documents (
    id                     uuid PRIMARY KEY,
    organisation_id        uuid NOT NULL REFERENCES organisations(id),
    -- Generic link to the business document this calculation belongs to.
    -- Nullable/untyped on purpose: internal/modules/sales doesn't exist
    -- yet (Stage 5b), and this module's own tests exercise the engine
    -- through a standalone reference_type until then.
    reference_type         text NOT NULL,
    reference_id           uuid,
    document_date          date NOT NULL,
    currency_code          text NOT NULL,
    supplier_state_code    text NOT NULL REFERENCES gst_state_codes(code),
    place_of_supply_code   text NOT NULL REFERENCES gst_state_codes(code),
    supply_type            text NOT NULL DEFAULT 'B2C' CHECK (supply_type IN ('B2B', 'B2C', 'EXPORT', 'SEZ')),
    reverse_charge         boolean NOT NULL DEFAULT false,
    total_taxable_amount   numeric(20,6) NOT NULL,
    total_tax_amount       numeric(20,6) NOT NULL,
    grand_total            numeric(20,6) NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_tax_documents_organisation_id ON tax_documents(organisation_id);
CREATE INDEX idx_tax_documents_reference ON tax_documents(reference_type, reference_id) WHERE reference_id IS NOT NULL;

ALTER TABLE tax_documents ENABLE ROW LEVEL SECURITY;
CREATE POLICY tax_documents_tenant_isolation ON tax_documents
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE tax_lines (
    id                   uuid PRIMARY KEY,
    organisation_id      uuid NOT NULL REFERENCES organisations(id),
    tax_document_id      uuid NOT NULL REFERENCES tax_documents(id) ON DELETE CASCADE,
    line_ref             text NOT NULL,  -- caller's own line identifier (e.g. a future sales_document_lines.id)
    hsn_sac_code         text NOT NULL,
    pricing_mode         text NOT NULL CHECK (pricing_mode IN ('INCLUSIVE', 'EXCLUSIVE')),
    gross_amount         numeric(20,6) NOT NULL,
    taxable_amount        numeric(20,6) NOT NULL,
    total_tax_amount      numeric(20,6) NOT NULL,
    classification        text NOT NULL,
    -- Snapshot of which tax_rate_master row this line resolved to, at
    -- calculation time — the row itself may later be superseded by a new
    -- validity window, but this line keeps pointing at the exact row that
    -- was used, so "what rate did this invoice actually use" is always
    -- answerable even if tax_rate_master.valid_to later closes that
    -- window (brief §7: never recalculate a finalized document).
    tax_rate_master_id    uuid NOT NULL REFERENCES tax_rate_master(id),
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_tax_lines_organisation_id ON tax_lines(organisation_id);
CREATE INDEX idx_tax_lines_document_id ON tax_lines(tax_document_id);

ALTER TABLE tax_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY tax_lines_tenant_isolation ON tax_lines
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE tax_components (
    id             uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id),
    tax_line_id     uuid NOT NULL REFERENCES tax_lines(id) ON DELETE CASCADE,
    component_type  text NOT NULL,  -- 'CGST' | 'SGST' | 'IGST' | 'UTGST' | 'CESS' today; open string for future regimes
    rate            numeric(24,12) NOT NULL,
    amount          numeric(20,6) NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_tax_components_organisation_id ON tax_components(organisation_id);
CREATE INDEX idx_tax_components_line_id ON tax_components(tax_line_id);

ALTER TABLE tax_components ENABLE ROW LEVEL SECURITY;
CREATE POLICY tax_components_tenant_isolation ON tax_components
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Sales document family (docs/architecture.md §4, brief §5) — one table
-- family parameterized by document_type, same pattern as purchases
-- (migrations/0013). Covers quotation, proforma invoice, sales order,
-- delivery challan, tax invoice, cash invoice, credit note, debit note,
-- sales return, recurring invoice, and POS invoice.
--
-- Judgment call: brief §5 lists "credit invoice"/"credit sale" alongside
-- "tax invoice" as if distinct document types, but a credit sale IS a tax
-- invoice (payment collected later rather than at point of sale) — there
-- is no separate legal document type for it, only a payment-terms
-- difference. Modeled as TAX_INVOICE with payment_terms_days > 0, not a
-- separate document_type, to avoid an ambiguous duplicate; a POS_INVOICE
-- with payment_terms_days = 0 is the "cash invoice" case.
CREATE TABLE sales_documents (
    id                        uuid PRIMARY KEY,
    organisation_id           uuid NOT NULL REFERENCES organisations(id),
    legal_entity_id           uuid NOT NULL REFERENCES legal_entities(id),
    branch_id                 uuid NOT NULL REFERENCES branches(id),
    warehouse_id              uuid NOT NULL REFERENCES warehouses(id),
    customer_party_id         uuid NOT NULL REFERENCES parties(id),
    document_type             text NOT NULL CHECK (document_type IN (
        'QUOTATION', 'PROFORMA_INVOICE', 'SALES_ORDER', 'DELIVERY_CHALLAN',
        'TAX_INVOICE', 'POS_INVOICE', 'CREDIT_NOTE', 'DEBIT_NOTE',
        'SALES_RETURN', 'RECURRING_INVOICE'
    )),
    document_number           text NOT NULL,
    -- Self-reference for conversion chains (quotation -> invoice, sales
    -- order -> invoice, delivery challan -> invoice) and for a
    -- credit/debit note or sales return raised against a specific
    -- original invoice.
    reference_document_id     uuid REFERENCES sales_documents(id),
    -- Business-document immutability (brief §31), same rationale as
    -- migrations/0013's purchase_documents: enforced at the application
    -- layer, not a DB trigger.
    status                    text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'FINALIZED', 'CANCELLED')),
    issue_date                date NOT NULL DEFAULT CURRENT_DATE,
    due_date                  date,
    supply_date               date,
    billing_address_id        uuid REFERENCES party_addresses(id),
    shipping_address_id       uuid REFERENCES party_addresses(id),
    customer_tax_registration_id uuid REFERENCES party_tax_registrations(id),
    place_of_supply_state_code text REFERENCES gst_state_codes(code),
    salesperson_user_id       uuid REFERENCES users(id),
    price_list_id             uuid REFERENCES price_lists(id),
    currency_code             text NOT NULL,
    base_currency_code        text NOT NULL,
    exchange_rate             numeric(24,12) NOT NULL DEFAULT 1,
    -- Whether line unit_price_amount already includes tax (brief §6) —
    -- set once at document creation, applied uniformly to every line at
    -- finalize; a document does not mix inclusive and exclusive lines.
    pricing_mode              text NOT NULL DEFAULT 'EXCLUSIVE' CHECK (pricing_mode IN ('INCLUSIVE', 'EXCLUSIVE')),
    customer_reference        text,
    transporter               text,
    vehicle_number            text,
    shipping_terms            text,
    notes                     text,
    terms_and_conditions      text,
    payment_terms_days        integer NOT NULL DEFAULT 0,
    -- Populated at finalize (brief §55 snapshot) by joining to the
    -- taxation module's tax_documents row (reference_type='sales_document')
    -- — kept here too as a denormalized quick-access pointer/total so a
    -- document listing doesn't need a join for its headline total. The
    -- authoritative per-line tax breakdown lives in tax_documents/
    -- tax_lines/tax_components, not duplicated here.
    tax_document_id           uuid,
    grand_total_amount        numeric(20,6),
    created_by                uuid NOT NULL REFERENCES users(id),
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    finalized_at              timestamptz,
    UNIQUE (organisation_id, document_type, document_number)
);

CREATE INDEX idx_sales_documents_organisation_id ON sales_documents(organisation_id);
CREATE INDEX idx_sales_documents_customer ON sales_documents(customer_party_id);
CREATE INDEX idx_sales_documents_reference ON sales_documents(reference_document_id) WHERE reference_document_id IS NOT NULL;
CREATE INDEX idx_sales_documents_issue_date ON sales_documents(issue_date);
CREATE INDEX idx_sales_documents_status ON sales_documents(status);

ALTER TABLE sales_documents ENABLE ROW LEVEL SECURITY;
CREATE POLICY sales_documents_tenant_isolation ON sales_documents
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE sales_document_lines (
    id                  uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    sales_document_id   uuid NOT NULL REFERENCES sales_documents(id),
    line_number         integer NOT NULL,
    product_variant_id  uuid NOT NULL REFERENCES product_variants(id),
    unit_id             uuid NOT NULL REFERENCES units_of_measure(id),
    quantity             numeric(20,6) NOT NULL CHECK (quantity > 0),
    unit_price_amount    numeric(20,6) NOT NULL DEFAULT 0,
    line_discount_amount numeric(20,6) NOT NULL DEFAULT 0 CHECK (line_discount_amount >= 0),
    -- Snapshotted at line-add time (brief §55: a finalized document must
    -- not silently change if the catalogue's HSN/SAC is edited later),
    -- same rationale as purchase_document_lines.batch_code.
    hsn_sac_code         text NOT NULL,
    -- (quantity * unit_price) - line_discount_amount — the taxable base
    -- fed to the tax engine at finalize time (brief §6: server is
    -- authoritative, never trusts a client-computed total).
    line_total_amount    numeric(20,6) NOT NULL DEFAULT 0,
    batch_code           text,
    serial_code          text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (sales_document_id, line_number)
);

CREATE INDEX idx_sales_document_lines_organisation_id ON sales_document_lines(organisation_id);
CREATE INDEX idx_sales_document_lines_document_id ON sales_document_lines(sales_document_id);

ALTER TABLE sales_document_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY sales_document_lines_tenant_isolation ON sales_document_lines
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

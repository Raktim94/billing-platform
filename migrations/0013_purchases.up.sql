-- Purchases module (docs/architecture.md §4, brief §13). One table family
-- parameterized by document_type, same pattern the architecture doc
-- specifies for sales (Stage 5) — a purchase order, goods receipt (GRN),
-- purchase invoice, purchase return, and debit note share one header/line
-- shape rather than five near-duplicate table pairs.
--
-- Document numbering here is a purchases-only, minimal, concurrency-safe
-- counter (purchase_document_counters) — NOT the full configurable-series
-- document_number_sequences feature the brief describes (§51), which is
-- explicitly Stage 5 scope. This keeps purchases usable now without
-- building numbering twice; Stage 5 can migrate purchases onto the shared
-- sequence mechanism later without touching stock/movement logic.
--
-- Supplier payment/credit (also listed under brief §13) is deliberately
-- NOT built in this migration: recording a real payment against a
-- supplier's outstanding balance needs the ledger/payment infrastructure
-- that does not exist until Stage 6 (accounting). Building a payment
-- table now, with nothing to post it against, would just be a stub.

CREATE TABLE purchase_document_counters (
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    document_type    text NOT NULL,
    next_number      bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (organisation_id, document_type)
);

ALTER TABLE purchase_document_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY purchase_document_counters_tenant_isolation ON purchase_document_counters
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE purchase_documents (
    id                       uuid PRIMARY KEY,
    organisation_id          uuid NOT NULL REFERENCES organisations(id),
    branch_id                uuid NOT NULL REFERENCES branches(id),
    warehouse_id             uuid NOT NULL REFERENCES warehouses(id),
    supplier_party_id        uuid NOT NULL REFERENCES parties(id),
    document_type            text NOT NULL CHECK (document_type IN (
        'PURCHASE_ORDER', 'GOODS_RECEIPT', 'PURCHASE_INVOICE', 'PURCHASE_RETURN', 'DEBIT_NOTE'
    )),
    document_number          text NOT NULL,
    -- Self-reference for the natural chain PO -> GRN -> invoice, or an
    -- invoice/GRN -> the return/debit-note raised against it. Nullable:
    -- a standalone document (e.g. a purchase invoice with no prior PO)
    -- is legitimate.
    reference_document_id    uuid REFERENCES purchase_documents(id),
    -- Business-document immutability (brief §31): FINALIZED fields are
    -- not editable in place; correction is a new document (return/debit
    -- note), not an UPDATE. Enforced at the application layer
    -- (app.Service.AddLine/FinalizeDocument reject mutation once
    -- FINALIZED/CANCELLED) — see docs/architecture.md §2 for why this is
    -- an app-layer rule rather than a DB trigger: the same "don't
    -- silently alter a finalized document" invariant sales (Stage 5) will
    -- need too, and is more naturally expressed once in the application
    -- layer's use-case code than duplicated as a trigger per document
    -- family.
    status                   text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'FINALIZED', 'CANCELLED')),
    supplier_invoice_number  text,
    supplier_invoice_date    date,
    document_date            date NOT NULL DEFAULT CURRENT_DATE,
    currency_code            text NOT NULL,
    notes                    text,
    created_by               uuid NOT NULL REFERENCES users(id),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    finalized_at             timestamptz,
    UNIQUE (organisation_id, document_type, document_number)
);

CREATE INDEX idx_purchase_documents_organisation_id ON purchase_documents(organisation_id);
CREATE INDEX idx_purchase_documents_supplier ON purchase_documents(supplier_party_id);
CREATE INDEX idx_purchase_documents_reference ON purchase_documents(reference_document_id) WHERE reference_document_id IS NOT NULL;
CREATE INDEX idx_purchase_documents_document_date ON purchase_documents(document_date);

ALTER TABLE purchase_documents ENABLE ROW LEVEL SECURITY;
CREATE POLICY purchase_documents_tenant_isolation ON purchase_documents
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE purchase_document_lines (
    id                    uuid PRIMARY KEY,
    organisation_id       uuid NOT NULL REFERENCES organisations(id),
    purchase_document_id  uuid NOT NULL REFERENCES purchase_documents(id),
    line_number           integer NOT NULL,
    product_variant_id    uuid NOT NULL REFERENCES product_variants(id),
    unit_id               uuid NOT NULL REFERENCES units_of_measure(id),
    quantity              numeric(20,6) NOT NULL CHECK (quantity > 0),
    -- Snapshotted at line-add time, not looked up fresh at finalize time
    -- (brief §55: a finalized document's calculation must not silently
    -- change if the price list changes later).
    unit_price_amount     numeric(20,6) NOT NULL DEFAULT 0,
    line_total_amount     numeric(20,6) NOT NULL DEFAULT 0,
    -- A batch/lot code as written on the supplier's paperwork, not yet a
    -- FK to stock_batches: the real stock_batches row (with its
    -- expiry/manufacture dates) is get-or-created at GOODS_RECEIPT
    -- finalize time, when inventory.RecordMovementForOtherModule actually
    -- posts the receipt — a PURCHASE_ORDER line, for instance, has no
    -- stock effect yet and shouldn't force a batch row to exist before
    -- goods are ever received.
    batch_code            text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (purchase_document_id, line_number)
);

CREATE INDEX idx_purchase_document_lines_organisation_id ON purchase_document_lines(organisation_id);
CREATE INDEX idx_purchase_document_lines_document_id ON purchase_document_lines(purchase_document_id);

ALTER TABLE purchase_document_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY purchase_document_lines_tenant_isolation ON purchase_document_lines
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

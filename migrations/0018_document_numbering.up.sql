-- The REAL configurable document-numbering system (brief §51) —
-- internal/platform/numbering. purchases' Stage 4 purchase_document_counters
-- was an explicit minimal placeholder (see migrations/0013's header
-- comment); this is what it was meant to be migrated onto eventually.
-- Not migrating purchases onto this in Stage 5b to avoid destabilizing a
-- verified Stage 4 — tracked as a follow-up.
--
-- Scope is (organisation, branch, document_type, financial_year): the
-- brief allows organisation/legal_entity/branch/FY/document-type scoping
-- to be configurable, but branch+doctype+FY is the concrete instantiation
-- actually needed by every document family this project has today (a
-- sales invoice numbering series is conventionally per-branch and resets
-- each financial year, e.g. INV/2026-27/000133) — a coarser org-wide
-- series is modeled by every branch's rows simply sharing one prefix and
-- being allocated from, which this schema already permits without a
-- special case.
CREATE TABLE document_number_counters (
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    branch_id        uuid NOT NULL REFERENCES branches(id),
    document_type    text NOT NULL,
    financial_year   text NOT NULL,
    prefix           text NOT NULL,
    next_number      bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (organisation_id, branch_id, document_type, financial_year)
);

ALTER TABLE document_number_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY document_number_counters_tenant_isolation ON document_number_counters
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

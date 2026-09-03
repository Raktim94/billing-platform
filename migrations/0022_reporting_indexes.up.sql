-- Reporting module (Stage 7, docs/adr/0004-dashboard-query-design.md).
-- Composite indexes for the dashboard summary's hot "today's finalized
-- documents for this organisation" queries — the existing single-column
-- indexes on organisation_id and issue_date/document_date separately are
-- not enough for an efficient (organisation, status, date-range) lookup.
-- No new tables: this stage is read-mostly, querying data other modules
-- already own (docs/architecture.md §2).

CREATE INDEX idx_sales_documents_org_status_issuedate
    ON sales_documents(organisation_id, status, issue_date);

CREATE INDEX idx_purchase_documents_org_status_docdate
    ON purchase_documents(organisation_id, status, document_date);

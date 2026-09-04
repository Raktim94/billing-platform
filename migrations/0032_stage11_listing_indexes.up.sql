-- Stage 11 load test finding (EXPLAIN ANALYZE at 100k products / 200k
-- sales_documents / 1M sales_document_lines, real seeded data, not
-- guessed): several unbounded ListByOrganisation-style queries sort by a
-- column with no index that includes organisation_id, forcing either a
-- disk-spilling external merge sort (products: 357ms, 11.9MB temp file)
-- or a whole-table index scan that isn't actually narrowed to one
-- organisation at the index level (sales_documents: correct today with
-- one tenant, but its cost scales with every organisation's row count
-- combined, not just the caller's, once this deployment has more than
-- one). Composite (organisation_id, <sort column>) indexes fix both: the
-- products case measured 357ms -> 20.6ms (17x) with zero disk spill.
--
-- Pure index additions — no schema/API contract change, no code change
-- required, safe to apply to an existing database with no downtime
-- concern beyond the index build itself.
CREATE INDEX idx_products_organisation_id_name ON products(organisation_id, name);
CREATE INDEX idx_parties_organisation_id_legal_name ON parties(organisation_id, legal_name);
CREATE INDEX idx_sales_documents_organisation_id_issue_date ON sales_documents(organisation_id, issue_date DESC);

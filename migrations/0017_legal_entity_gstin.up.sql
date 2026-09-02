-- Additive extension to legal_entities (migrations/0001), not a Stage 2
-- bug fix: Stage 2 predates the tax module, so it had no reason yet to
-- carry the legal entity's own GST registration. Stage 5b (sales) needs
-- the SUPPLIER side of a tax calculation's state code (docs/architecture.md
-- §8 hierarchy: "Legal Entity / Tax Registration" is one level) — a sales
-- document's legal_entity IS the seller's GST registration. Nullable and
-- additive: existing rows are unaffected, no backfill needed.
ALTER TABLE legal_entities ADD COLUMN gstin text;
ALTER TABLE legal_entities ADD COLUMN gst_state_code text REFERENCES gst_state_codes(code);

-- Extensions used across the platform.
--
-- pgcrypto: gen_random_uuid() as a DB-side safety net. Application code is
-- the primary source of primary-key UUIDs (uuid.NewV7() in Go, so keys are
-- time-ordered per brief §4) — tables intentionally do NOT set this as a
-- column DEFAULT, so a row inserted without an explicit application-chosen
-- id is a bug to catch, not a case to paper over. The extension stays
-- available for ad hoc/manual SQL work (support, migrations, backfills).
--
-- pg_trgm: trigram indexes for fast fuzzy/partial search (brief §24 —
-- product/customer/SKU/barcode search must feel instantaneous). Not used
-- by any Stage 2 table yet; enabled now because enabling an extension is a
-- cheap, low-risk foundation step and every later catalogue/contacts
-- migration will want it.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

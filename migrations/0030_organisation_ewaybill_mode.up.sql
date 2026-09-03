-- Adds the per-organisation e-Way Bill production-mode setting docs/
-- architecture.md §9b calls for ("a per-organisation setting, FREE_PORTAL
-- is the default"). FREE_PORTAL needs no paid API/credentials at all;
-- AUTOMATIC_API is the optional Stage 8 EWayBillProvider path. Modeled as
-- a plain column on organisations (not a new key/value settings table —
-- there is exactly one such setting today, and a generic settings table
-- for one row would be premature).
ALTER TABLE organisations
    ADD COLUMN ewaybill_mode text NOT NULL DEFAULT 'FREE_PORTAL'
        CHECK (ewaybill_mode IN ('FREE_PORTAL', 'AUTOMATIC_API'));

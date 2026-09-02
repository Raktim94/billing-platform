-- India GST plugin master data (docs/architecture.md §8, brief §7/§18).
-- Two kinds of table here, deliberately different RLS treatment:
--
-- 1. gst_state_codes: true structural constants of Indian geography (which
--    two-digit code maps to which state/UT, and whether it's a Union
--    Territory — needed to choose CGST+SGST vs CGST+UTGST for an
--    intra-state supply). This does NOT vary per tenant, so it is a global
--    reference table with NO organisation_id and NO RLS — every
--    organisation on a self-hosted instance reads the same rows. Seeded
--    below with the standard CBIC GST state-code list; an operator can
--    update this table directly if a new UT is carved out in the future
--    (this project deliberately does not hardcode the list as a Go enum —
--    docs/research.md notes GST_STATE_CODES has changed before, e.g.
--    J&K/Ladakh in 2019).
--
-- 2. tax_rate_master: which GST/cess rate applies to a given HSN/SAC code
--    on a given date. This DOES vary per tenant — different businesses may
--    adopt a rate change on different effective dates, classify a
--    borderline HSN differently, or run a composition scheme. RLS-scoped
--    like every other tenant table since migration 0001, deliberately NOT
--    made a shared global table even though the underlying government rate
--    is nominally uniform, to keep the RLS pattern uncompromised and let
--    each organisation manage its own master via the API (brief §26
--    gst.manage, already seeded in migration 0002).

CREATE TABLE gst_state_codes (
    code               text PRIMARY KEY,  -- two-digit GST state code, e.g. '27'
    name               text NOT NULL,
    is_union_territory boolean NOT NULL DEFAULT false
);

INSERT INTO gst_state_codes (code, name, is_union_territory) VALUES
    ('01', 'Jammu and Kashmir', true),
    ('02', 'Himachal Pradesh', false),
    ('03', 'Punjab', false),
    ('04', 'Chandigarh', true),
    ('05', 'Uttarakhand', false),
    ('06', 'Haryana', false),
    ('07', 'Delhi', true),
    ('08', 'Rajasthan', false),
    ('09', 'Uttar Pradesh', false),
    ('10', 'Bihar', false),
    ('11', 'Sikkim', false),
    ('12', 'Arunachal Pradesh', false),
    ('13', 'Nagaland', false),
    ('14', 'Manipur', false),
    ('15', 'Mizoram', false),
    ('16', 'Tripura', false),
    ('17', 'Meghalaya', false),
    ('18', 'Assam', false),
    ('19', 'West Bengal', false),
    ('20', 'Jharkhand', false),
    ('21', 'Odisha', false),
    ('22', 'Chhattisgarh', false),
    ('23', 'Madhya Pradesh', false),
    ('24', 'Gujarat', false),
    ('25', 'Daman and Diu', true),
    ('26', 'Dadra and Nagar Haveli and Daman and Diu', true),
    ('27', 'Maharashtra', false),
    ('28', 'Andhra Pradesh (Before Division)', false),
    ('29', 'Karnataka', false),
    ('30', 'Goa', false),
    ('31', 'Lakshadweep', true),
    ('32', 'Kerala', false),
    ('33', 'Tamil Nadu', false),
    ('34', 'Puducherry', true),
    ('35', 'Andaman and Nicobar Islands', true),
    ('36', 'Telangana', false),
    ('37', 'Andhra Pradesh', false),
    ('38', 'Ladakh', true),
    ('97', 'Other Territory', true),
    ('99', 'Centre Jurisdiction', true)
ON CONFLICT (code) DO NOTHING;

-- One rate "set" per (organisation, hsn_sac_code, validity window). gst_rate
-- is the COMBINED rate (what would appear on a price tag) — the engine
-- splits it into CGST+SGST/UTGST or IGST at calculation time depending on
-- place of supply, per docs/architecture.md §8; this table never stores a
-- pre-split cgst/sgst pair (brief §18: no hardcoded cgst/sgst columns).
-- cess_rate is a separate, optional, additive rate (Compensation Cess,
-- e.g. on tobacco/luxury vehicles) computed on the same taxable value as
-- GST, not on the GST-inclusive amount.
CREATE TABLE tax_rate_master (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    country_code     text NOT NULL DEFAULT 'IN',
    hsn_sac_code     text NOT NULL,
    classification   text NOT NULL DEFAULT 'TAXABLE'
                     CHECK (classification IN ('TAXABLE', 'EXEMPT', 'NIL_RATED', 'ZERO_RATED')),
    gst_rate         numeric(24,12) NOT NULL DEFAULT 0 CHECK (gst_rate >= 0),
    cess_rate        numeric(24,12) NOT NULL DEFAULT 0 CHECK (cess_rate >= 0),
    valid_from       date NOT NULL,
    valid_to         date,  -- NULL = still open-ended/current
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX idx_tax_rate_master_organisation_id ON tax_rate_master(organisation_id);
-- The lookup this table exists to serve: "the row for this HSN valid on
-- this date." Not a unique constraint (overlapping windows are an
-- application-layer validation concern, not enforced here — see
-- internal/modules/gstindia/app's CreateRate for why an exclusion
-- constraint was judged not worth the complexity for v1) but this index
-- makes the resolution query an index range scan, not a tenant table scan.
CREATE INDEX idx_tax_rate_master_lookup ON tax_rate_master(organisation_id, country_code, hsn_sac_code, valid_from);

ALTER TABLE tax_rate_master ENABLE ROW LEVEL SECURITY;
CREATE POLICY tax_rate_master_tenant_isolation ON tax_rate_master
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

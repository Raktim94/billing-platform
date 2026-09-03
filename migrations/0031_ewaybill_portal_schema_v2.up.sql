-- Records the portal/v1 mapper's field-name rewrite (2026-09-04): the
-- original 'v1-placeholder' shape used entirely invented snake_case/
-- nested field names never checked against a real sample. This version
-- was rewritten against the real NIC "Generate e-Way Bill" schema,
-- cross-checked against two independent sources describing the actual
-- field list (developer.sandbox.co.in, gsthelp.charteredinfo.com) — see
-- internal/modules/ewaybill/portal/v1/mapper.go's own caveat comment for
-- exactly what is and is not independently verified. Deactivating the
-- old row rather than deleting it — this table is a historical record of
-- what shape was in production when, not just a current-config table.
UPDATE ewaybill_portal_schema_versions
    SET active = false, effective_until = '2026-09-04'
    WHERE version = 'v1-placeholder';

INSERT INTO ewaybill_portal_schema_versions (id, version, effective_from, active, notes)
VALUES (
    '01996e6a-0000-7000-8000-000000000003',
    'v2-verified-field-names',
    '2026-09-04',
    true,
    'Flat, camelCase, root-level fields matching the real NIC e-Way Bill generation schema (fromGstin/toGstin/docType/docNo/transDistance/totInvValue/itemList with per-line cgstRate/sgstRate/igstRate, etc.) — a real improvement over v1-placeholder''s invented nested/snake_case shape. NOT yet independently verified byte-for-byte against a live official sample or the current NIC bulk-upload tool PDF (docs.ewaybillgst.gov.in blocked automated fetch with a 403); subSupplyType''s numeric code mapping, vehicleType default, and the bulk-tool''s own array-wrapper key are still provisional — see internal/modules/ewaybill/portal/v1/mapper.go''s doc comment for the exact list. Verify against a live sample before this handles real production filings.'
);

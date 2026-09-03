DELETE FROM ewaybill_portal_schema_versions WHERE version = 'v2-verified-field-names';
UPDATE ewaybill_portal_schema_versions
    SET active = true, effective_until = NULL
    WHERE version = 'v1-placeholder';

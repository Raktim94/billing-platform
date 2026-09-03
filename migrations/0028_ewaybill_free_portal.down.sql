DELETE FROM permissions WHERE code IN ('logistics.view', 'logistics.manage');

DROP TABLE IF EXISTS customer_transport_preferences;
DROP TABLE IF EXISTS transporters;
DROP TABLE IF EXISTS vehicles;
DROP TABLE IF EXISTS ewaybill_portal_schema_versions;
DROP TABLE IF EXISTS ewaybill_eligibility_rules;

ALTER TABLE ewaybill_records DROP COLUMN IF EXISTS prepared_at;
ALTER TABLE ewaybill_records DROP COLUMN IF EXISTS prepared_file_name;
ALTER TABLE ewaybill_records DROP COLUMN IF EXISTS canonical_snapshot;
ALTER TABLE ewaybill_records DROP COLUMN IF EXISTS source;
ALTER TABLE ewaybill_records DROP COLUMN IF EXISTS mode;

ALTER TABLE ewaybill_records DROP CONSTRAINT ewaybill_records_status_check;
ALTER TABLE ewaybill_records ADD CONSTRAINT ewaybill_records_status_check CHECK (status IN (
    'DRAFT', 'QUEUED', 'SUBMITTING', 'GENERATED',
    'FAILED_RETRYABLE', 'FAILED_FINAL', 'CANCEL_PENDING', 'CANCELLED', 'CLOSED'
));

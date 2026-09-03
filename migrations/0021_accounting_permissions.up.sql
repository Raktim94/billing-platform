-- migrations/0002_rbac_catalog.up.sql already seeded accounting.view,
-- accounting.post, and accounting.reconcile (its brief §26 example list).
-- Fiscal period locking (brief §52) needs one more: the ability to post or
-- modify a transaction INSIDE a locked period, which is deliberately a
-- separate, narrower grant from ordinary accounting.post — most accountants
-- who can post journals should NOT be able to silently override a closed
-- period.
INSERT INTO permissions (code, module, description) VALUES
    ('accounting.override_locked_period', 'accounting', 'Post or modify a transaction inside a locked fiscal period');

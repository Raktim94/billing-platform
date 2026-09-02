-- Stage 4 permission additions. migrations/0002_rbac_catalog.up.sql
-- already pre-seeded the full future permission surface per brief §26,
-- including 'inventory.view'/'inventory.adjust'/'inventory.transfer' and
-- 'purchase.view'/'purchase.create'/'purchase.finalize'/'purchase.cancel'
-- (singular "purchase", matching the brief's own example list) — the
-- inventory and purchases modules built in Stage 4 use those existing
-- codes directly rather than duplicating or inventing a parallel
-- "purchases.*" namespace. The one genuinely new permission this stage
-- needs is 'inventory.manage', for operations that are neither a
-- correction (inventory.adjust) nor a warehouse-to-warehouse move
-- (inventory.transfer) — recording opening stock and reserving/releasing
-- stock for an in-progress sale.
INSERT INTO permissions (code, module, description) VALUES
    ('inventory.manage', 'inventory', 'Record opening stock and manage stock reservations');

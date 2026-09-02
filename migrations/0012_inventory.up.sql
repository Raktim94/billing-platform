-- Inventory module (docs/architecture.md §4/§6, brief §11). Same RLS
-- pattern as every tenant table since migration 0001.
--
-- stock_movements is the append-only source of truth (brief §11: "NEVER
-- store stock merely as a manually editable number"). stock_balances is a
-- materialized projection kept in sync by the application layer, in the
-- same RunScoped transaction as the movement insert — see
-- docs/adr/0002-stock-balance-maintenance.md for why this is an
-- application-layer update rather than a database trigger.

-- One batch/lot per (product_variant, batch_code). This project models
-- "batch" and "lot" as the same underlying concept (a group of stock
-- received/produced together, sharing an expiry/manufacture date) rather
-- than two near-duplicate tables — most ERPs treat these as synonyms for
-- the same tracking mechanism, and the brief's own wording ("batch/lot
-- tracking") supports treating them as one feature with two common names.
CREATE TABLE stock_batches (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    product_variant_id uuid NOT NULL REFERENCES product_variants(id),
    batch_code         text NOT NULL,
    manufacture_date   date,
    expiry_date        date,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, product_variant_id, batch_code)
);

CREATE INDEX idx_stock_batches_organisation_id ON stock_batches(organisation_id);
CREATE INDEX idx_stock_batches_variant_id ON stock_batches(product_variant_id);
CREATE INDEX idx_stock_batches_expiry_date ON stock_batches(expiry_date) WHERE expiry_date IS NOT NULL;

ALTER TABLE stock_batches ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_batches_tenant_isolation ON stock_batches
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE stock_serial_numbers (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    product_variant_id uuid NOT NULL REFERENCES product_variants(id),
    serial_code        text NOT NULL,
    -- Current location: NULL means not currently in any warehouse (sold,
    -- or never received). Updated as movements referencing this serial
    -- are recorded — a serial number's history is still fully derivable
    -- from stock_movements, this column is a convenience projection.
    warehouse_id       uuid REFERENCES warehouses(id),
    status             text NOT NULL DEFAULT 'IN_STOCK' CHECK (status IN ('IN_STOCK', 'RESERVED', 'SOLD')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, product_variant_id, serial_code)
);

CREATE INDEX idx_stock_serial_numbers_organisation_id ON stock_serial_numbers(organisation_id);
CREATE INDEX idx_stock_serial_numbers_variant_id ON stock_serial_numbers(product_variant_id);

ALTER TABLE stock_serial_numbers ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_serial_numbers_tenant_isolation ON stock_serial_numbers
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Transfer header: the two stock_movements legs (TRANSFER_OUT at the
-- source, TRANSFER_IN at the destination) both reference this row via
-- stock_movements.reference_type='STOCK_TRANSFER'/reference_id, so a
-- transfer's two legs stay traceable as one operation without a bespoke
-- "transfer_group_id" column duplicating what reference_id already does.
CREATE TABLE stock_transfers (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    from_warehouse_id  uuid NOT NULL REFERENCES warehouses(id),
    to_warehouse_id    uuid NOT NULL REFERENCES warehouses(id),
    status             text NOT NULL DEFAULT 'COMPLETED' CHECK (status IN ('COMPLETED', 'CANCELLED')),
    notes              text,
    created_by         uuid NOT NULL REFERENCES users(id),
    created_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (from_warehouse_id <> to_warehouse_id)
);

CREATE INDEX idx_stock_transfers_organisation_id ON stock_transfers(organisation_id);

ALTER TABLE stock_transfers ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_transfers_tenant_isolation ON stock_transfers
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE stock_adjustments (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    warehouse_id       uuid NOT NULL REFERENCES warehouses(id),
    reason             text NOT NULL,
    notes              text,
    created_by         uuid NOT NULL REFERENCES users(id),
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_stock_adjustments_organisation_id ON stock_adjustments(organisation_id);

ALTER TABLE stock_adjustments ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_adjustments_tenant_isolation ON stock_adjustments
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- The append-only ledger. quantity/base_quantity are always POSITIVE — a
-- movement is a magnitude plus a semantic movement_type, never a signed
-- delta, so a row reads naturally ("SALE, quantity 5") instead of
-- requiring the reader to already know the sign convention. Which
-- direction (increases or decreases stock_balances.quantity_on_hand) a
-- movement_type implies is a fixed mapping duplicated deliberately in two
-- places: internal/modules/inventory/domain.MovementDirection (Go, what
-- the application layer uses to update stock_balances) and this
-- migration's CHECK constraint (the exhaustive list of valid types, so an
-- invalid type can never reach the ledger even via a future direct-SQL
-- bug). Both must be updated together if a new movement_type is ever
-- added — internal/modules/inventory/domain/movement_test.go asserts
-- every constant in the CHECK list has a direction mapping, so a
-- forgotten update fails a unit test, not a silent stock-balance drift.
CREATE TABLE stock_movements (
    id                  uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    warehouse_id        uuid NOT NULL REFERENCES warehouses(id),
    product_variant_id  uuid NOT NULL REFERENCES product_variants(id),
    movement_type       text NOT NULL CHECK (movement_type IN (
        'OPENING', 'PURCHASE_RECEIPT', 'PURCHASE_RETURN', 'SALE', 'SALE_RETURN',
        'TRANSFER_OUT', 'TRANSFER_IN', 'ADJUSTMENT_IN', 'ADJUSTMENT_OUT',
        'ASSEMBLY_IN', 'ASSEMBLY_OUT', 'DAMAGE', 'EXPIRY'
    )),
    -- The unit the movement was transacted in (e.g. a purchase received
    -- in BOX) and the equivalent quantity in the product's base stocking
    -- unit, with the conversion factor already applied at the time of
    -- this movement (brief §11 — auditable, a later change to the
    -- conversion ratio must not reinterpret historical movements).
    unit_id             uuid NOT NULL REFERENCES units_of_measure(id),
    quantity            numeric(20,6) NOT NULL CHECK (quantity > 0),
    base_quantity       numeric(20,6) NOT NULL CHECK (base_quantity > 0),
    -- Cost per base unit at the time of this movement. Populated for
    -- inward movements that establish a cost basis (OPENING,
    -- PURCHASE_RECEIPT); NULL for movements that consume existing stock
    -- at whatever the current weighted-average cost already is.
    unit_cost           numeric(20,6),
    batch_id            uuid REFERENCES stock_batches(id),
    serial_number_id    uuid REFERENCES stock_serial_numbers(id),
    -- Generic link to whatever business document caused this movement
    -- (a purchases GOODS_RECEIPT, a stock_adjustments row, a
    -- stock_transfers row, and — from Stage 5 onward — a sales document).
    -- Deliberately not a foreign key to any single table, since the
    -- referenced table varies by reference_type and Stage 5's sales
    -- documents don't exist yet.
    reference_type      text,
    reference_id        uuid,
    notes                text,
    created_by          uuid NOT NULL REFERENCES users(id),
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_stock_movements_organisation_id ON stock_movements(organisation_id);
CREATE INDEX idx_stock_movements_warehouse_variant ON stock_movements(warehouse_id, product_variant_id);
CREATE INDEX idx_stock_movements_created_at ON stock_movements(created_at);
CREATE INDEX idx_stock_movements_reference ON stock_movements(reference_type, reference_id) WHERE reference_type IS NOT NULL;

ALTER TABLE stock_movements ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_movements_tenant_isolation ON stock_movements
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Materialized projection of stock_movements, maintained by the
-- application layer inside the same transaction as each movement insert
-- (docs/adr/0002-stock-balance-maintenance.md). Always re-derivable from
-- scratch by replaying stock_movements — that reconciliation is exactly
-- what Scenario N (backup/restore) checks.
CREATE TABLE stock_balances (
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    warehouse_id         uuid NOT NULL REFERENCES warehouses(id),
    product_variant_id   uuid NOT NULL REFERENCES product_variants(id),
    quantity_on_hand     numeric(20,6) NOT NULL DEFAULT 0,
    quantity_reserved    numeric(20,6) NOT NULL DEFAULT 0,
    -- Weighted-average cost per base unit (docs/architecture.md §6). The
    -- CostingStrategy interface exists so FIFO can be added later without
    -- touching this schema — see internal/modules/inventory/domain/costing.go.
    average_cost         numeric(20,6) NOT NULL DEFAULT 0,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, warehouse_id, product_variant_id)
);

CREATE INDEX idx_stock_balances_organisation_id ON stock_balances(organisation_id);
-- Low-stock/reorder queries scan for balances below a configured
-- threshold; this index makes that a warehouse-scoped range scan instead
-- of a full tenant table scan.
CREATE INDEX idx_stock_balances_warehouse_id ON stock_balances(warehouse_id);

ALTER TABLE stock_balances ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_balances_tenant_isolation ON stock_balances
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- A reservation earmarks quantity without moving it — e.g. a POS cart or
-- a draft sales order holding stock so a second concurrent sale can't
-- also claim it (Scenario D's building block). Consumption/release is
-- Stage 5's job (the sales module); this stage builds the primitive.
CREATE TABLE stock_reservations (
    id                  uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    warehouse_id        uuid NOT NULL REFERENCES warehouses(id),
    product_variant_id  uuid NOT NULL REFERENCES product_variants(id),
    quantity             numeric(20,6) NOT NULL CHECK (quantity > 0),
    reference_type       text NOT NULL,
    reference_id         uuid NOT NULL,
    status                text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'RELEASED', 'CONSUMED')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    released_at           timestamptz
);

CREATE INDEX idx_stock_reservations_organisation_id ON stock_reservations(organisation_id);
CREATE INDEX idx_stock_reservations_warehouse_variant_active
    ON stock_reservations(warehouse_id, product_variant_id) WHERE status = 'ACTIVE';

ALTER TABLE stock_reservations ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_reservations_tenant_isolation ON stock_reservations
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Per-product-per-warehouse stocking policy (brief §11: reorder level,
-- safety stock, negative-stock policy). Absence of a row means "use the
-- system default" (reorder_level=0, negative stock forbidden) rather than
-- every product needing an explicit row from day one.
CREATE TABLE stock_policies (
    organisation_id       uuid NOT NULL REFERENCES organisations(id),
    warehouse_id          uuid NOT NULL REFERENCES warehouses(id),
    product_variant_id    uuid NOT NULL REFERENCES product_variants(id),
    reorder_level         numeric(20,6) NOT NULL DEFAULT 0,
    safety_stock          numeric(20,6) NOT NULL DEFAULT 0,
    allow_negative_stock  boolean NOT NULL DEFAULT false,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, warehouse_id, product_variant_id)
);

CREATE INDEX idx_stock_policies_organisation_id ON stock_policies(organisation_id);

ALTER TABLE stock_policies ENABLE ROW LEVEL SECURITY;
CREATE POLICY stock_policies_tenant_isolation ON stock_policies
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

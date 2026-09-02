-- Catalogue module (docs/architecture.md §4, brief §11 unit-conversion
-- requirement, brief §19 HSN/SAC). Same RLS pattern as every tenant table
-- since migration 0001 — see that file's header comment for the full
-- rationale, not repeated here.
--
-- Entity shape: Product -> ProductVariant (carries the SKU) ->
-- ProductBarcode (one variant can have more than one barcode, one per
-- transacted unit — e.g. a BOX barcode and a single-PCS barcode for the
-- same variant, per brief §11 "products sold in one unit but stocked in
-- another").

CREATE TABLE units_of_measure (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    code             text NOT NULL,
    name             text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, code)
);

CREATE INDEX idx_units_of_measure_organisation_id ON units_of_measure(organisation_id);

ALTER TABLE units_of_measure ENABLE ROW LEVEL SECURITY;
CREATE POLICY units_of_measure_tenant_isolation ON units_of_measure
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- 1 unit of from_unit_id = factor * unit of to_unit_id, e.g.
-- from=BOX, to=PCS, factor=25 means "1 BOX = 25 PCS". Explicit and
-- auditable per brief §11 — never an implicit/hardcoded conversion.
CREATE TABLE unit_conversions (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    from_unit_id     uuid NOT NULL REFERENCES units_of_measure(id),
    to_unit_id       uuid NOT NULL REFERENCES units_of_measure(id),
    factor           numeric(20,6) NOT NULL CHECK (factor > 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (from_unit_id <> to_unit_id),
    UNIQUE (organisation_id, from_unit_id, to_unit_id)
);

CREATE INDEX idx_unit_conversions_organisation_id ON unit_conversions(organisation_id);

ALTER TABLE unit_conversions ENABLE ROW LEVEL SECURITY;
CREATE POLICY unit_conversions_tenant_isolation ON unit_conversions
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE categories (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    parent_id        uuid REFERENCES categories(id),
    name             text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_categories_organisation_id ON categories(organisation_id);
CREATE INDEX idx_categories_parent_id ON categories(parent_id);

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
CREATE POLICY categories_tenant_isolation ON categories
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE brands (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    name             text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_brands_organisation_id ON brands(organisation_id);

ALTER TABLE brands ENABLE ROW LEVEL SECURITY;
CREATE POLICY brands_tenant_isolation ON brands
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE products (
    id                 uuid PRIMARY KEY,
    organisation_id    uuid NOT NULL REFERENCES organisations(id),
    category_id        uuid REFERENCES categories(id),
    brand_id           uuid REFERENCES brands(id),
    base_uom_id        uuid NOT NULL REFERENCES units_of_measure(id),
    name               text NOT NULL,
    description        text,
    -- HSN (goods) or SAC (services), brief §7/§19. Nullable here — this
    -- table is country-agnostic data modeling, not tax logic (that's
    -- gstindia, Stage 5). Stage 5's tax engine is what actually enforces
    -- "must have an HSN/SAC to finalize a GST sale line."
    hsn_sac_code       text,
    status             text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_organisation_id ON products(organisation_id);
CREATE INDEX idx_products_category_id ON products(category_id);
CREATE INDEX idx_products_hsn_sac_code ON products(hsn_sac_code);
-- Trigram index: fast fuzzy/partial product-name search (brief §24/§25 —
-- the billing-screen search must feel instantaneous). The API endpoint
-- that actually uses this is later-stage UX work; the index is schema and
-- belongs with the table.
CREATE INDEX idx_products_name_trgm ON products USING gin (name gin_trgm_ops);

ALTER TABLE products ENABLE ROW LEVEL SECURITY;
CREATE POLICY products_tenant_isolation ON products
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE product_variants (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    product_id       uuid NOT NULL REFERENCES products(id),
    sku_code         text NOT NULL,
    -- Free-form variant attributes (size, colour, ...). The brief doesn't
    -- mandate a fixed attribute schema and businesses' variant dimensions
    -- vary too much to hardcode columns for; a jsonb bag avoids a schema
    -- migration every time a new dimension shows up. Anything that needs
    -- to be queried/filtered structurally (price, stock, HSN) stays a real
    -- column elsewhere, never inside this bag.
    attributes       jsonb NOT NULL DEFAULT '{}'::jsonb,
    status           text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, sku_code)
);

CREATE INDEX idx_product_variants_organisation_id ON product_variants(organisation_id);
CREATE INDEX idx_product_variants_product_id ON product_variants(product_id);
CREATE INDEX idx_product_variants_sku_trgm ON product_variants USING gin (sku_code gin_trgm_ops);

ALTER TABLE product_variants ENABLE ROW LEVEL SECURITY;
CREATE POLICY product_variants_tenant_isolation ON product_variants
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE product_barcodes (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    variant_id       uuid NOT NULL REFERENCES product_variants(id),
    -- Which transacted unit this specific barcode represents — a variant
    -- stocked in PCS but also sold by the BOX can have one barcode per
    -- unit (brief §11).
    unit_id          uuid NOT NULL REFERENCES units_of_measure(id),
    barcode          text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, barcode)
);

CREATE INDEX idx_product_barcodes_organisation_id ON product_barcodes(organisation_id);
CREATE INDEX idx_product_barcodes_variant_id ON product_barcodes(variant_id);
-- Barcode-scan lookup is exact-match, not fuzzy — the UNIQUE constraint's
-- backing btree index already serves that scan at full speed, so no
-- separate trigram index here (unlike name/SKU search, which is
-- deliberately fuzzy).

ALTER TABLE product_barcodes ENABLE ROW LEVEL SECURITY;
CREATE POLICY product_barcodes_tenant_isolation ON product_barcodes
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

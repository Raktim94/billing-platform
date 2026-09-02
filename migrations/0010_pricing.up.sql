-- Pricing module. Money amounts are NUMERIC(20,6) per brief §1, scanned
-- directly into decimal.Decimal via the pgx-shopspring-decimal integration
-- registered in internal/platform/database, and wrapped as money.Money at
-- the application layer — this table never sees a float.

CREATE TABLE price_lists (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    name             text NOT NULL,
    currency_code    varchar(3) NOT NULL,
    is_default       boolean NOT NULL DEFAULT false,
    status           text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_price_lists_organisation_id ON price_lists(organisation_id);

ALTER TABLE price_lists ENABLE ROW LEVEL SECURITY;
CREATE POLICY price_lists_tenant_isolation ON price_lists
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE price_list_items (
    id                   uuid PRIMARY KEY,
    organisation_id      uuid NOT NULL REFERENCES organisations(id),
    price_list_id        uuid NOT NULL REFERENCES price_lists(id),
    product_variant_id   uuid NOT NULL REFERENCES product_variants(id),
    unit_id              uuid NOT NULL REFERENCES units_of_measure(id),
    price_amount         numeric(20,6) NOT NULL CHECK (price_amount >= 0),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, price_list_id, product_variant_id, unit_id)
);

CREATE INDEX idx_price_list_items_organisation_id ON price_list_items(organisation_id);
CREATE INDEX idx_price_list_items_price_list_id ON price_list_items(price_list_id);
CREATE INDEX idx_price_list_items_variant_id ON price_list_items(product_variant_id);

ALTER TABLE price_list_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY price_list_items_tenant_isolation ON price_list_items
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

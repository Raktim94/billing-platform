-- Contacts module (brief §16). A party may be CUSTOMER, SUPPLIER, or BOTH
-- — one row, not two, so a business that both buys from and sells to the
-- same company doesn't end up with duplicate, drifting records.

CREATE TABLE parties (
    id                    uuid PRIMARY KEY,
    organisation_id       uuid NOT NULL REFERENCES organisations(id),
    party_type            text NOT NULL CHECK (party_type IN ('CUSTOMER', 'SUPPLIER', 'BOTH')),
    legal_name            text NOT NULL,
    trade_name            text,
    phone                 text,
    email                 text,
    currency_code         varchar(3) NOT NULL,
    -- NUMERIC, never float, per brief §1/§6 — even though credit limit
    -- isn't a transaction amount, it's compared against real invoice
    -- totals during Scenario F-style discount/limit checks later.
    credit_limit_amount   numeric(20,6),
    payment_terms_days    integer,
    notes                 text,
    status                text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_parties_organisation_id ON parties(organisation_id);
CREATE INDEX idx_parties_phone ON parties(phone);
CREATE INDEX idx_parties_name_trgm ON parties USING gin (legal_name gin_trgm_ops);
CREATE INDEX idx_parties_trade_name_trgm ON parties USING gin (trade_name gin_trgm_ops);

ALTER TABLE parties ENABLE ROW LEVEL SECURITY;
CREATE POLICY parties_tenant_isolation ON parties
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE party_addresses (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id),
    party_id         uuid NOT NULL REFERENCES parties(id),
    address_type     text NOT NULL CHECK (address_type IN ('BILLING', 'SHIPPING', 'WAREHOUSE', 'REGISTERED_OFFICE')),
    line1            text NOT NULL,
    line2            text,
    city             text,
    state            text,
    postal_code      text,
    country_code     char(2) NOT NULL,
    is_default       boolean NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_party_addresses_organisation_id ON party_addresses(organisation_id);
CREATE INDEX idx_party_addresses_party_id ON party_addresses(party_id);

ALTER TABLE party_addresses ENABLE ROW LEVEL SECURITY;
CREATE POLICY party_addresses_tenant_isolation ON party_addresses
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Generic tax registration per party, deliberately country-agnostic
-- (registration_number, not gstin) — docs/architecture.md §8: country tax
-- specifics live in the country's own module (gstindia validates GSTIN
-- format/derives state_code from it in Stage 5), not baked in here. This
-- table just records that a party holds a tax registration in a country.
CREATE TABLE party_tax_registrations (
    id                    uuid PRIMARY KEY,
    organisation_id       uuid NOT NULL REFERENCES organisations(id),
    party_id              uuid NOT NULL REFERENCES parties(id),
    country_code          char(2) NOT NULL,
    registration_number   text NOT NULL,
    -- India: 2-digit GST state code. Nullable/generic because it's only
    -- meaningful for GSTIN-shaped registrations; gstindia (Stage 5) is
    -- what actually derives/validates this from the GSTIN, this column
    -- just stores it.
    state_code            text,
    is_primary            boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, party_id, registration_number)
);

CREATE INDEX idx_party_tax_registrations_organisation_id ON party_tax_registrations(organisation_id);
CREATE INDEX idx_party_tax_registrations_party_id ON party_tax_registrations(party_id);
-- Exact-match GSTIN lookup (brief §24's "search by GSTIN") — btree via the
-- unique constraint already covers this.

ALTER TABLE party_tax_registrations ENABLE ROW LEVEL SECURITY;
CREATE POLICY party_tax_registrations_tenant_isolation ON party_tax_registrations
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Double-entry accounting core (docs/architecture.md §7, brief §14).
-- accounting is the ONLY module allowed to write journal_lines; every
-- other module with a financial effect calls accounting.Post/PostTx
-- instead of constructing journal rows itself.
--
-- The double-entry invariant is enforced at three layers (docs/architecture.md
-- §7): (1) application-layer sum check before insert, (2) a DEFERRED
-- constraint trigger below that re-verifies SUM(debit)=SUM(credit) per
-- journal at COMMIT — this catches even a hypothetical future direct-SQL
-- bug, and unlike a REVOKE-based defense it does not depend on knowing the
-- runtime role's name (this schema has no FORCE ROW LEVEL SECURITY and no
-- migration-managed role, per internal/platform/database's
-- WarnIfRuntimeRoleOwnsTenantTables comment — a table owner bypasses both
-- RLS and GRANT/REVOKE, so an unconditional trigger is the only mechanism
-- guaranteed to hold regardless of which role connects), (3) a BEFORE
-- UPDATE/DELETE trigger below that unconditionally rejects any mutation of
-- an existing journal_lines row — every journal in this schema is written
-- once, fully formed, and is POSTED from the moment it exists (there is no
-- DRAFT journal state to edit); correction is always a new reversing
-- journal referencing the original (brief §14).

CREATE TABLE accounts (
    id               uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    code             text NOT NULL,
    name             text NOT NULL,
    account_type      text NOT NULL CHECK (account_type IN ('ASSET', 'LIABILITY', 'EQUITY', 'INCOME', 'EXPENSE')),
    -- normal_balance says which side an increase to this account posts to
    -- (ASSET/EXPENSE accounts normally increase on DEBIT; LIABILITY/EQUITY/
    -- INCOME normally increase on CREDIT) — stored explicitly rather than
    -- derived purely from account_type so a contra account (e.g. "Sales
    -- Returns", an INCOME-type account that normally carries a DEBIT
    -- balance) can be modeled without a special case in application code.
    normal_balance    text NOT NULL CHECK (normal_balance IN ('DEBIT', 'CREDIT')),
    parent_account_id uuid REFERENCES accounts(id),
    is_system         boolean NOT NULL DEFAULT false,
    is_active         boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, code)
);

CREATE INDEX idx_accounts_organisation_id ON accounts(organisation_id);

ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY accounts_tenant_isolation ON accounts
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE journals (
    id                 uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    -- Generic link to whatever finalized this journal (a sales/purchase
    -- document, a receipt, a payment, a manual adjustment, a reversal) —
    -- same reference_type/reference_id pattern as tax_documents and
    -- stock_movements, not a typed FK per source.
    source_type         text NOT NULL,
    source_id           uuid,
    journal_date         date NOT NULL,
    description         text,
    -- Set only on a reversing journal, pointing back at the journal it
    -- reverses — the sole sanctioned "correction" mechanism (brief §14).
    reversed_journal_id  uuid REFERENCES journals(id),
    -- Every journal in this schema is created already POSTED — there is no
    -- application code path that inserts a non-POSTED journal, and this
    -- CHECK makes that a DB-enforced invariant too, not just a convention
    -- (brief Rule 19: prefer DB constraints for invariants).
    status              text NOT NULL DEFAULT 'POSTED' CHECK (status = 'POSTED'),
    created_by           uuid NOT NULL REFERENCES users(id),
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_journals_organisation_id ON journals(organisation_id);
CREATE INDEX idx_journals_source ON journals(source_type, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_journals_journal_date ON journals(organisation_id, journal_date);

ALTER TABLE journals ENABLE ROW LEVEL SECURITY;
CREATE POLICY journals_tenant_isolation ON journals
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE journal_lines (
    id               uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    journal_id        uuid NOT NULL REFERENCES journals(id),
    account_id        uuid NOT NULL REFERENCES accounts(id),
    -- party_id is set for AR/AP-affecting lines (links a line to the
    -- customer/supplier it concerns) so the customer/supplier ledger query
    -- can filter journal_lines by party without joining back through every
    -- possible source document type — nullable because most lines (e.g.
    -- the Sales/Tax-Payable side of a sale) aren't party-specific.
    party_id          uuid REFERENCES parties(id),
    debit_amount       numeric(20,6) NOT NULL DEFAULT 0 CHECK (debit_amount >= 0),
    credit_amount      numeric(20,6) NOT NULL DEFAULT 0 CHECK (credit_amount >= 0),
    -- A line is a debit OR a credit, never both, and never neither.
    CHECK (NOT (debit_amount > 0 AND credit_amount > 0)),
    CHECK (debit_amount > 0 OR credit_amount > 0),
    description       text,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_journal_lines_organisation_id ON journal_lines(organisation_id);
CREATE INDEX idx_journal_lines_journal_id ON journal_lines(journal_id);
CREATE INDEX idx_journal_lines_account_id ON journal_lines(account_id);
CREATE INDEX idx_journal_lines_party_id ON journal_lines(party_id) WHERE party_id IS NOT NULL;

ALTER TABLE journal_lines ENABLE ROW LEVEL SECURITY;
CREATE POLICY journal_lines_tenant_isolation ON journal_lines
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

-- Layer 2: deferred constraint trigger re-verifying SUM(debit)=SUM(credit)
-- per journal at COMMIT (fires once per row change within the transaction,
-- deferred so a multi-line INSERT is fully visible before it evaluates).
CREATE FUNCTION accounting_check_journal_balanced() RETURNS trigger AS $$
DECLARE
    jid uuid;
    total_debit numeric(20,6);
    total_credit numeric(20,6);
BEGIN
    jid := COALESCE(NEW.journal_id, OLD.journal_id);
    SELECT COALESCE(SUM(debit_amount), 0), COALESCE(SUM(credit_amount), 0)
        INTO total_debit, total_credit
        FROM journal_lines WHERE journal_id = jid;
    IF total_debit <> total_credit THEN
        RAISE EXCEPTION 'accounting: journal % is unbalanced (debit=%, credit=%)', jid, total_debit, total_credit
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER journal_lines_balanced
    AFTER INSERT OR UPDATE OR DELETE ON journal_lines
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION accounting_check_journal_balanced();

-- Layer 3: journal_lines are immutable from the instant they're written.
-- No status column to check against (see journals.status's CHECK above) —
-- ANY update or delete is rejected, unconditionally, at the database level.
CREATE FUNCTION accounting_journal_lines_no_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'accounting: journal_lines are immutable once written — post a reversing journal instead (brief §14), row id %',
        COALESCE(OLD.id, NEW.id)
        USING ERRCODE = 'check_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER journal_lines_immutable
    BEFORE UPDATE OR DELETE ON journal_lines
    FOR EACH ROW
    EXECUTE FUNCTION accounting_journal_lines_no_mutation();

CREATE TABLE fiscal_periods (
    id             uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id),
    start_date     date NOT NULL,
    end_date       date NOT NULL CHECK (end_date > start_date),
    label          text NOT NULL,
    is_locked      boolean NOT NULL DEFAULT false,
    locked_at      timestamptz,
    locked_by      uuid REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_fiscal_periods_organisation_id ON fiscal_periods(organisation_id);
CREATE INDEX idx_fiscal_periods_range ON fiscal_periods(organisation_id, start_date, end_date);

ALTER TABLE fiscal_periods ENABLE ROW LEVEL SECURITY;
CREATE POLICY fiscal_periods_tenant_isolation ON fiscal_periods
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE bank_accounts (
    id               uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    legal_entity_id    uuid NOT NULL REFERENCES legal_entities(id),
    name             text NOT NULL,
    account_kind      text NOT NULL CHECK (account_kind IN ('BANK', 'CASH')),
    account_number    text,
    bank_name         text,
    ifsc_code         text,
    currency_code     text NOT NULL,
    -- Every bank/cash account has a corresponding chart-of-accounts row it
    -- posts to — a receipt/payment against this bank_account debits/credits
    -- gl_account_id, not a hardcoded generic "Bank" account, so a business
    -- with three real bank accounts sees three real GL balances.
    gl_account_id     uuid NOT NULL REFERENCES accounts(id),
    is_active         boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_bank_accounts_organisation_id ON bank_accounts(organisation_id);

ALTER TABLE bank_accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY bank_accounts_tenant_isolation ON bank_accounts
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE receipts (
    id                 uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    party_id            uuid NOT NULL REFERENCES parties(id),
    sales_document_id    uuid REFERENCES sales_documents(id),
    amount              numeric(20,6) NOT NULL CHECK (amount > 0),
    currency_code       text NOT NULL,
    bank_account_id      uuid REFERENCES bank_accounts(id),
    payment_method       text NOT NULL CHECK (payment_method IN ('CASH', 'UPI', 'CARD', 'BANK_TRANSFER', 'CHEQUE', 'OTHER')),
    reference_number     text,
    received_at          timestamptz NOT NULL,
    journal_id           uuid NOT NULL REFERENCES journals(id),
    created_by           uuid NOT NULL REFERENCES users(id),
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_receipts_organisation_id ON receipts(organisation_id);
CREATE INDEX idx_receipts_party_id ON receipts(organisation_id, party_id);
CREATE INDEX idx_receipts_sales_document_id ON receipts(sales_document_id) WHERE sales_document_id IS NOT NULL;

ALTER TABLE receipts ENABLE ROW LEVEL SECURITY;
CREATE POLICY receipts_tenant_isolation ON receipts
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE payments (
    id                 uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id),
    party_id            uuid NOT NULL REFERENCES parties(id),
    purchase_document_id uuid REFERENCES purchase_documents(id),
    amount              numeric(20,6) NOT NULL CHECK (amount > 0),
    currency_code       text NOT NULL,
    bank_account_id      uuid REFERENCES bank_accounts(id),
    payment_method       text NOT NULL CHECK (payment_method IN ('CASH', 'UPI', 'CARD', 'BANK_TRANSFER', 'CHEQUE', 'OTHER')),
    reference_number     text,
    paid_at             timestamptz NOT NULL,
    journal_id           uuid NOT NULL REFERENCES journals(id),
    created_by           uuid NOT NULL REFERENCES users(id),
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_organisation_id ON payments(organisation_id);
CREATE INDEX idx_payments_party_id ON payments(organisation_id, party_id);
CREATE INDEX idx_payments_purchase_document_id ON payments(purchase_document_id) WHERE purchase_document_id IS NOT NULL;

ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY payments_tenant_isolation ON payments
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE reconciliations (
    id               uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    bank_account_id    uuid NOT NULL REFERENCES bank_accounts(id),
    statement_date    date NOT NULL,
    opening_balance   numeric(20,6) NOT NULL,
    closing_balance   numeric(20,6) NOT NULL,
    is_reconciled     boolean NOT NULL DEFAULT false,
    reconciled_at     timestamptz,
    reconciled_by     uuid REFERENCES users(id),
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_reconciliations_organisation_id ON reconciliations(organisation_id);
CREATE INDEX idx_reconciliations_bank_account_id ON reconciliations(bank_account_id);

ALTER TABLE reconciliations ENABLE ROW LEVEL SECURITY;
CREATE POLICY reconciliations_tenant_isolation ON reconciliations
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

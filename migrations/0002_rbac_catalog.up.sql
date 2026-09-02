-- Global permission catalog (brief §26). Permission *definitions* are
-- platform-wide, not tenant-owned data — no organisation_id here. What IS
-- tenant-owned is which permissions a given organisation's roles grant
-- (role_permissions) and which users hold which roles (user_roles,
-- migration 0003) — those tables carry organisation_id.
--
-- Only modules that exist as of Stage 2 (identity, organisation/locations,
-- audit) get real permission codes here. Codes for modules that don't
-- exist yet (sales.*, purchase.*, inventory.*, accounting.*, gst.*,
-- einvoice.*, ewaybill.*, reports.*) are seeded now anyway, deliberately:
-- the RBAC catalog is Stage 2 scope per docs/architecture.md §16, and
-- seeding the full permission surface now means every later module ships
-- against permission codes that already exist and are already assignable
-- to roles, instead of each module needing its own catalog migration.
CREATE TABLE permissions (
    code         text PRIMARY KEY,
    module       text NOT NULL,
    description  text NOT NULL
);

INSERT INTO permissions (code, module, description) VALUES
    ('users.view',            'users',       'View users'),
    ('users.create',          'users',       'Create users'),
    ('users.edit',            'users',       'Edit users'),
    ('users.disable',         'users',       'Disable/re-enable users'),
    ('roles.view',            'roles',       'View roles and permission grants'),
    ('roles.manage',          'roles',       'Create/edit roles and permission grants'),
    ('settings.view',         'settings',    'View organisation/branch/warehouse settings'),
    ('settings.manage',       'settings',    'Manage organisation/branch/warehouse settings'),
    ('audit.view',            'audit',       'View the audit log'),

    ('sales.view',            'sales',       'View sales documents'),
    ('sales.create',          'sales',       'Create sales documents'),
    ('sales.edit_draft',      'sales',       'Edit a draft sales document'),
    ('sales.finalize',        'sales',       'Finalize a sales document'),
    ('sales.cancel',          'sales',       'Cancel a finalized sales document'),
    ('sales.discount',        'sales',       'Apply a discount on a sales document'),
    ('sales.change_rate',     'sales',       'Override a line''s unit price'),
    ('sales.view_cost',       'sales',       'View product cost while billing'),

    ('purchase.view',         'purchase',    'View purchase documents'),
    ('purchase.create',       'purchase',    'Create purchase documents'),
    ('purchase.finalize',     'purchase',    'Finalize a purchase document'),
    ('purchase.cancel',       'purchase',    'Cancel a finalized purchase document'),

    ('inventory.view',        'inventory',   'View stock levels and movements'),
    ('inventory.adjust',      'inventory',   'Record a stock adjustment'),
    ('inventory.transfer',    'inventory',   'Transfer stock between warehouses'),

    ('accounting.view',       'accounting',  'View ledgers and journals'),
    ('accounting.post',       'accounting',  'Post a journal entry'),
    ('accounting.reconcile',  'accounting',  'Reconcile bank/cash accounts'),

    ('reports.view',          'reports',     'View reports and dashboards'),
    ('reports.export',        'reports',     'Export report data'),

    ('gst.view',               'gst',        'View GST configuration and reports'),
    ('gst.manage',             'gst',        'Manage GST rate/HSN configuration'),
    ('einvoice.generate',      'einvoice',   'Generate an e-Invoice IRN'),
    ('einvoice.cancel',        'einvoice',   'Cancel an e-Invoice IRN'),
    ('ewaybill.generate',      'ewaybill',   'Generate an e-Way Bill'),
    ('ewaybill.cancel',        'ewaybill',   'Cancel an e-Way Bill');

-- Roles are organisation-owned: each organisation gets its own copy of
-- e.g. "Owner", created by the bootstrap use case
-- (internal/modules/identity) when the organisation is created, not
-- seeded globally here. This lets one organisation customize a role's
-- permission grants without affecting any other organisation.
CREATE TABLE roles (
    id                uuid PRIMARY KEY,
    organisation_id   uuid NOT NULL REFERENCES organisations(id),
    code              text NOT NULL,
    name              text NOT NULL,
    -- System roles (Owner, Administrator, ...) ship with a fixed starter
    -- permission set at bootstrap and exist for every organisation;
    -- is_system marks them so the UI can, e.g., prevent deleting/renaming
    -- the Owner role. Non-system rows are fully custom roles.
    is_system         boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, code)
);

CREATE INDEX idx_roles_organisation_id ON roles(organisation_id);

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
CREATE POLICY roles_tenant_isolation ON roles
    USING (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid)
    WITH CHECK (organisation_id = NULLIF(current_setting('app.current_organisation_id', true), '')::uuid);

CREATE TABLE role_permissions (
    role_id          uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_code  text NOT NULL REFERENCES permissions(code),
    PRIMARY KEY (role_id, permission_code)
);

-- No organisation_id column here by design (it would be redundant with
-- roles.organisation_id and every real query joins through roles anyway);
-- tenant isolation for this table is enforced by always querying/writing
-- it through a role_id that the application layer has already verified
-- belongs to the caller's organisation, plus the roles table's own RLS
-- making a cross-tenant role_id unresolvable in the first place.
CREATE INDEX idx_role_permissions_permission_code ON role_permissions(permission_code);

-- New permission codes for the Stage 3 modules (catalogue, contacts,
-- pricing), added the same way as the rest of the global permission
-- catalog in migrations/0002_rbac_catalog.up.sql.
INSERT INTO permissions (code, module, description) VALUES
    ('catalogue.view',   'catalogue', 'View products, categories, brands, and units of measure'),
    ('catalogue.manage', 'catalogue', 'Create/edit products, categories, brands, and units of measure'),
    ('contacts.view',    'contacts',  'View customers and suppliers'),
    ('contacts.manage',  'contacts',  'Create/edit customers and suppliers'),
    ('pricing.view',     'pricing',   'View price lists'),
    ('pricing.manage',   'pricing',  'Create/edit price lists and price list items');

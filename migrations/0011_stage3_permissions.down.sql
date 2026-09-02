DELETE FROM permissions WHERE code IN (
    'catalogue.view', 'catalogue.manage',
    'contacts.view', 'contacts.manage',
    'pricing.view', 'pricing.manage'
);

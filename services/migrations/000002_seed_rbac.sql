INSERT INTO permissions (id, code, description) VALUES
	('10000000-0000-4000-8000-000000000001', 'system:admin', 'Full system administration'),
	('10000000-0000-4000-8000-000000000002', 'settings:manage', 'Manage panel and server settings'),
	('10000000-0000-4000-8000-000000000003', 'users:manage', 'Manage panel users and access'),
	('10000000-0000-4000-8000-000000000004', 'accounts:manage', 'Manage hosting accounts and packages'),
	('10000000-0000-4000-8000-000000000005', 'websites:manage', 'Manage websites, domains, and vhosts'),
	('10000000-0000-4000-8000-000000000006', 'databases:manage', 'Manage databases and database users'),
	('10000000-0000-4000-8000-000000000007', 'dns:manage', 'Manage DNS zones and records'),
	('10000000-0000-4000-8000-000000000008', 'mail:manage', 'Manage mail domains, mailboxes, and routing'),
	('10000000-0000-4000-8000-000000000009', 'jobs:manage', 'Manage background jobs and retries'),
	('10000000-0000-4000-8000-000000000010', 'audit:read', 'Read audit events'),
	('10000000-0000-4000-8000-000000000011', 'agent:execute', 'Execute allowlisted agent actions'),
	('10000000-0000-4000-8000-000000000012', 'billing:manage', 'Manage license, billing, and commercial settings'),
	('10000000-0000-4000-8000-000000000013', 'resellers:manage', 'Manage reseller accounts and ownership')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO roles (id, name, description) VALUES
	('20000000-0000-4000-8000-000000000001', 'owner', 'System owner with all permissions'),
	('20000000-0000-4000-8000-000000000002', 'admin', 'Server administrator without commercial ownership controls'),
	('20000000-0000-4000-8000-000000000003', 'reseller', 'Reseller operator for assigned customer accounts'),
	('20000000-0000-4000-8000-000000000004', 'customer', 'Hosting customer for owned resources')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'owner'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
	'settings:manage',
	'users:manage',
	'accounts:manage',
	'websites:manage',
	'databases:manage',
	'dns:manage',
	'mail:manage',
	'jobs:manage',
	'audit:read',
	'agent:execute'
)
WHERE r.name = 'admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
	'accounts:manage',
	'websites:manage',
	'databases:manage',
	'dns:manage',
	'mail:manage'
)
WHERE r.name = 'reseller'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
	'websites:manage',
	'databases:manage',
	'dns:manage',
	'mail:manage'
)
WHERE r.name = 'customer'
ON CONFLICT (role_id, permission_id) DO NOTHING;

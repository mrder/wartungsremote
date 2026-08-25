-- Seed default permissions and roles per docs/PERMISSIONS.md.
-- "optional" permissions in the matrix are modeled as granted-by-default for
-- technician (matches the documented default operating mode) and can be
-- revoked per-deployment via the role_permissions table.

INSERT INTO permissions (name, description) VALUES
    ('device.read',              'View devices'),
    ('device.manage',            'Create/modify/delete devices'),
    ('customer.read',            'View customers'),
    ('customer.manage',          'Manage customers/groups/tags'),
    ('monitoring.read',          'View monitoring data'),
    ('remote.terminal',          'Open remote terminal sessions'),
    ('remote.tunnel.ssh',        'Open SSH tunnels'),
    ('remote.tunnel.rdp',        'Open RDP tunnels'),
    ('remote.files.read',        'Read remote files'),
    ('remote.files.write',       'Write/delete remote files'),
    ('remote.service.control',   'Start/stop/restart services'),
    ('remote.process.terminate', 'Terminate remote processes'),
    ('remote.power',             'Restart/shutdown devices'),
    ('privilege.request',        'Request temporary privilege sessions'),
    ('maintenance.read',         'View maintenance history'),
    ('maintenance.write',        'Write maintenance notes'),
    ('audit.read',                'Read audit log'),
    ('enrollment.create',        'Create enrollment tokens'),
    ('credential.revoke',        'Revoke device credentials'),
    ('agent.update',             'Trigger agent updates'),
    ('user.manage',              'Manage users'),
    ('role.manage',              'Manage roles'),
    ('system.settings',          'Manage system settings')
ON CONFLICT (name) DO NOTHING;

INSERT INTO roles (name, description) VALUES
    ('read_only',   'Read-only access to devices, monitoring and maintenance history'),
    ('technician',  'Day-to-day remote maintenance access'),
    ('admin',       'Full administrative access excluding user/role management'),
    ('super_admin', 'Full administrative access including user/role management')
ON CONFLICT (name) DO NOTHING;

-- read_only
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'read_only' AND p.name IN (
    'device.read','customer.read','monitoring.read','maintenance.read'
)
ON CONFLICT DO NOTHING;

-- technician (includes the "optional" grants as documented default)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'technician' AND p.name IN (
    'device.read','customer.read','monitoring.read',
    'remote.terminal','remote.tunnel.ssh','remote.tunnel.rdp',
    'remote.files.read','remote.files.write',
    'remote.service.control','remote.process.terminate',
    'privilege.request','maintenance.read','maintenance.write','audit.read'
)
ON CONFLICT DO NOTHING;

-- admin: everything except user/role/system management
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name NOT IN (
    'user.manage','role.manage','system.settings'
)
ON CONFLICT DO NOTHING;

-- super_admin: all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'super_admin'
ON CONFLICT DO NOTHING;

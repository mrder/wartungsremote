-- Alerting (docs/TODO.md Phase 24): manage alert rules, acknowledge/resolve
-- alerts. Viewing alerts/rules reuses the existing 'monitoring.read'
-- permission; this new permission gates the write/manage actions.
INSERT INTO permissions (name, description) VALUES
    ('alert.manage', 'Manage alert rules and acknowledge/resolve alerts')
ON CONFLICT (name) DO NOTHING;

-- technician gets it as part of day-to-day operational access.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'technician' AND p.name = 'alert.manage'
ON CONFLICT DO NOTHING;

-- admin: everything except user/role/system management already includes
-- new permissions automatically via its NOT IN clause at seed time, but
-- that seed already ran — grant explicitly here for existing installs.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name = 'alert.manage'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'super_admin' AND p.name = 'alert.manage'
ON CONFLICT DO NOTHING;

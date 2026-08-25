-- Creates a least-privilege PostgreSQL role for wr-core's RUNTIME
-- connection (docs/TODO.md Phase 35 "DB Runtime User ohne
-- Superuserrechte", docs/DEPLOYMENT.md §5).
--
-- Usage: run migrations first (as an owner/superuser role, e.g. via
-- wr-core's normal startup, which applies internal/db's embedded
-- migrations), THEN run this script once against the same database, THEN
-- switch WR_DATABASE_URL_FILE to use wartungsremote_app instead of the
-- migration/owner role for day-to-day server operation:
--
--   psql "$ADMIN_DSN" -v app_password="'change-me'" -f scripts/create-db-runtime-role.sql
--
-- wartungsremote_app deliberately CANNOT: create/drop tables, alter the
-- audit_log append-only trigger (internal/db owns that DDL), create other
-- roles, or read/write outside this schema. It CAN read and write every
-- application table, because wr-core's own code is the access-control
-- boundary (RBAC + row-level permission checks) — the DB role isn't meant
-- to reimplement that, only to contain a SQL-injection or
-- compromised-credential blast radius to "this app's data" instead of
-- "the whole Postgres instance."

CREATE ROLE wartungsremote_app LOGIN PASSWORD :app_password;

GRANT CONNECT ON DATABASE wartungsremote TO wartungsremote_app;
GRANT USAGE ON SCHEMA public TO wartungsremote_app;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO wartungsremote_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO wartungsremote_app;

-- Any table created by a FUTURE migration automatically gets the same
-- grants, so this script doesn't need re-running after every schema
-- change — only after the role itself needs to change.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO wartungsremote_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO wartungsremote_app;

-- Explicitly deny what INSERT/UPDATE/DELETE would otherwise allow on the
-- append-only audit log — defense in depth alongside the DB trigger from
-- migrations/0003_audit_append_only.sql, which already rejects UPDATE/
-- DELETE at the trigger level regardless of grants.
REVOKE UPDATE, DELETE ON audit_log FROM wartungsremote_app;

-- No DDL rights: cannot create/alter/drop tables, cannot create other
-- roles. wartungsremote_app is intentionally NOT a member of any role
-- with CREATEDB/CREATEROLE/SUPERUSER.

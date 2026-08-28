-- Idempotent version of create-db-runtime-role.sql, safe to run on every
-- stack startup (docker-compose.yml's db-init service does exactly that)
-- rather than the one-time-manual-run original. Same grants, same
-- reasoning — see create-db-runtime-role.sql's own header comment.
--
-- Usage: psql "$ADMIN_DSN" -v app_password="'change-me'" -f scripts/ensure-db-runtime-role.sql

-- psql's :variable substitution does not happen inside dollar-quoted (DO
-- $$ ... $$) blocks, so the role is only conditionally CREATEd here (with
-- a throwaway placeholder password if it doesn't exist yet); the actual
-- password is always set immediately after by a plain ALTER ROLE, which
-- IS substituted normally.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'wartungsremote_app') THEN
        CREATE ROLE wartungsremote_app LOGIN PASSWORD 'replaced-immediately-below';
    END IF;
END
$$;

ALTER ROLE wartungsremote_app WITH PASSWORD :app_password;

GRANT CONNECT ON DATABASE wartungsremote TO wartungsremote_app;
GRANT USAGE ON SCHEMA public TO wartungsremote_app;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO wartungsremote_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO wartungsremote_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO wartungsremote_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO wartungsremote_app;

REVOKE UPDATE, DELETE ON audit_log FROM wartungsremote_app;

-- Enforce append-only semantics on audit_log at the database level
-- (defense in depth in addition to not exposing any delete/update API).
-- See docs/SECURITY.md §16 and docs/DATABASE.md audit_log.

CREATE OR REPLACE FUNCTION audit_log_deny_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_deny_mutation();

CREATE TRIGGER audit_log_no_delete
    BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_deny_mutation();

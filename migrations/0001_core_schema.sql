-- WartungsRemote core schema V1
-- See docs/DATABASE.md for the normative data model.
-- Migrations are append-only after release; do not edit an applied file, add a new one instead.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Users / Auth
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username        citext UNIQUE NOT NULL,
    display_name    text,
    password_hash   text NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','locked')),
    mfa_required    boolean NOT NULL DEFAULT true,
    failed_login_count int NOT NULL DEFAULT 0,
    locked_until    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    last_login_at   timestamptz
);

CREATE TABLE user_mfa (
    user_id             uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret_ciphertext   bytea NOT NULL,
    secret_nonce        bytea NOT NULL,
    secret_key_version  int NOT NULL DEFAULT 1,
    confirmed_at        timestamptz,
    recovery_code_hashes jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash  bytea UNIQUE NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL,
    idle_expires_at     timestamptz NOT NULL,
    revoked_at          timestamptz,
    ip                  inet,
    user_agent          text
);

CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);

CREATE TABLE mfa_challenges (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    attempts        int NOT NULL DEFAULT 0,
    consumed_at     timestamptz,
    ip              inet
);

CREATE TABLE reauth_tokens (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    consumed_at     timestamptz,
    ip              inet
);

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text UNIQUE NOT NULL,
    description text
);

CREATE TABLE permissions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text UNIQUE NOT NULL,
    description text
);

CREATE TABLE role_permissions (
    role_id       uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    scope_type  text NOT NULL DEFAULT 'global' CHECK (scope_type IN ('global','customer','group')),
    scope_id    uuid
);

-- PRIMARY KEY cannot use expressions, so uniqueness (including the "one
-- global grant per role" case where scope_id is NULL) is enforced via this
-- expression index instead.
CREATE UNIQUE INDEX idx_user_roles_unique ON user_roles (user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'));

-- ---------------------------------------------------------------------------
-- Customers / Devices
-- ---------------------------------------------------------------------------

CREATE TABLE customers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    customer_number text,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    notes           text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE device_groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id uuid REFERENCES customers(id) ON DELETE SET NULL,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    install_id          uuid UNIQUE NOT NULL,
    customer_id         uuid REFERENCES customers(id) ON DELETE SET NULL,
    group_id            uuid REFERENCES device_groups(id) ON DELETE SET NULL,
    display_name        text NOT NULL,
    hostname            text,
    os_family           text,
    os_name             text,
    os_version          text,
    architecture        text,
    agent_version       text,
    status              text NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','online','connection_lost','offline','revoked')),
    health              text NOT NULL DEFAULT 'unknown' CHECK (health IN ('healthy','warning','critical','offline','unknown')),
    health_reasons      jsonb NOT NULL DEFAULT '[]'::jsonb,
    tags                jsonb NOT NULL DEFAULT '[]'::jsonb,
    policy              jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at        timestamptz,
    last_public_ip      inet,
    credential_status   text NOT NULL DEFAULT 'none' CHECK (credential_status IN ('none','active','revoked')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    revoked_at          timestamptz
);

CREATE INDEX idx_devices_customer_id ON devices(customer_id);
CREATE INDEX idx_devices_group_id ON devices(group_id);
CREATE INDEX idx_devices_status ON devices(status);

CREATE TABLE device_credentials (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    credential_type         text NOT NULL DEFAULT 'ed25519',
    public_key              bytea NOT NULL,
    certificate_fingerprint bytea,
    issued_at               timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz,
    revoked_at              timestamptz,
    key_version             int NOT NULL DEFAULT 1,
    UNIQUE(device_id, key_version)
);

CREATE TABLE enrollment_tokens (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash          bytea UNIQUE NOT NULL,
    customer_id         uuid REFERENCES customers(id) ON DELETE SET NULL,
    group_id            uuid REFERENCES device_groups(id) ON DELETE SET NULL,
    display_name        text,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    tags                jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL,
    consumed_at         timestamptz,
    consumed_device_id  uuid REFERENCES devices(id) ON DELETE SET NULL,
    revoked_at          timestamptz
);

CREATE INDEX idx_enrollment_tokens_expires_at ON enrollment_tokens(expires_at) WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE device_capabilities (
    device_id   uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    capability  text NOT NULL,
    version     int NOT NULL DEFAULT 1,
    PRIMARY KEY(device_id, capability)
);

CREATE TABLE device_network (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL DEFAULT now(),
    interfaces  jsonb NOT NULL DEFAULT '[]'::jsonb,
    public_ip   inet
);

CREATE INDEX idx_device_network_device_id ON device_network(device_id, observed_at DESC);

CREATE TABLE device_metrics (
    id                  bigserial PRIMARY KEY,
    device_id           uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    observed_at         timestamptz NOT NULL DEFAULT now(),
    cpu_percent         real,
    memory_used_bytes   bigint,
    memory_total_bytes  bigint,
    filesystems         jsonb NOT NULL DEFAULT '[]'::jsonb,
    network             jsonb NOT NULL DEFAULT '{}'::jsonb,
    uptime_seconds      bigint
);

CREATE INDEX idx_device_metrics_device_time ON device_metrics(device_id, observed_at DESC);

CREATE TABLE device_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    event_type  text NOT NULL,
    severity    text NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','critical')),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    payload     jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_device_events_device_time ON device_events(device_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Remote sessions / privilege / commands / tunnels (schema ready for later phases)
-- ---------------------------------------------------------------------------

CREATE TABLE remote_sessions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    maintenance_session_id  uuid,
    kind                    text NOT NULL CHECK (kind IN ('terminal','ssh_tunnel','rdp_tunnel','file_transfer')),
    state                   text NOT NULL DEFAULT 'requested' CHECK (state IN ('requested','opening','active','closing','closed','failed','interrupted','expired')),
    opened_at               timestamptz NOT NULL DEFAULT now(),
    closed_at               timestamptz,
    expires_at              timestamptz NOT NULL,
    close_reason            text
);

CREATE INDEX idx_remote_sessions_device_id ON remote_sessions(device_id);
CREATE INDEX idx_remote_sessions_user_id ON remote_sessions(user_id);

CREATE TABLE privilege_sessions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    remote_session_id       uuid REFERENCES remote_sessions(id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    created_at              timestamptz NOT NULL DEFAULT now(),
    valid_until             timestamptz NOT NULL,
    revoked_at              timestamptz,
    authorization_reason    text
);

CREATE INDEX idx_privilege_sessions_device_id ON privilege_sessions(device_id);

CREATE TABLE remote_commands (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    remote_session_id       uuid REFERENCES remote_sessions(id) ON DELETE SET NULL,
    command_type            text NOT NULL,
    state                   text NOT NULL DEFAULT 'created' CHECK (state IN ('created','dispatched','acknowledged','succeeded','failed','timeout','expired','cancelled')),
    created_at              timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz NOT NULL,
    dispatched_at           timestamptz,
    completed_at            timestamptz,
    result_code             text,
    request_payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    result_summary          jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_remote_commands_device_id ON remote_commands(device_id);

CREATE TABLE tunnel_sessions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    remote_session_id       uuid NOT NULL REFERENCES remote_sessions(id) ON DELETE CASCADE,
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type             text NOT NULL CHECK (target_type IN ('ssh_local','rdp_local')),
    state                   text NOT NULL DEFAULT 'requested' CHECK (state IN ('requested','prepared','ticket_issued','connecting','active','closed','expired','denied','failed','interrupted')),
    helper_ticket_hash      bytea,
    created_at              timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz NOT NULL,
    connected_at            timestamptz,
    closed_at               timestamptz,
    bytes_up                bigint NOT NULL DEFAULT 0,
    bytes_down              bigint NOT NULL DEFAULT 0
);

CREATE INDEX idx_tunnel_sessions_device_id ON tunnel_sessions(device_id);

-- ---------------------------------------------------------------------------
-- Maintenance / Audit
-- ---------------------------------------------------------------------------

CREATE TABLE maintenance_sessions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    customer_id             uuid REFERENCES customers(id) ON DELETE SET NULL,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    started_at              timestamptz NOT NULL DEFAULT now(),
    ended_at                timestamptz,
    result                  text,
    summary                 text,
    next_maintenance_at     timestamptz
);

ALTER TABLE remote_sessions
    ADD CONSTRAINT fk_remote_sessions_maintenance
    FOREIGN KEY (maintenance_session_id) REFERENCES maintenance_sessions(id) ON DELETE SET NULL;

CREATE INDEX idx_maintenance_sessions_device_id ON maintenance_sessions(device_id);

CREATE TABLE maintenance_events (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    maintenance_session_id      uuid NOT NULL REFERENCES maintenance_sessions(id) ON DELETE CASCADE,
    event_type                  text NOT NULL,
    occurred_at                 timestamptz NOT NULL DEFAULT now(),
    summary                     text,
    reference_id                uuid
);

CREATE TABLE audit_log (
    id              bigserial PRIMARY KEY,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    actor_type      text NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id        uuid,
    session_id      uuid,
    device_id       uuid,
    customer_id     uuid,
    event_type      text NOT NULL,
    result          text NOT NULL CHECK (result IN ('success','failure','denied')),
    source_ip       inet,
    request_id      uuid,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    prev_hash       bytea,
    entry_hash      bytea
);

CREATE INDEX idx_audit_log_occurred_at ON audit_log(occurred_at DESC);
CREATE INDEX idx_audit_log_device_id ON audit_log(device_id);
CREATE INDEX idx_audit_log_actor_id ON audit_log(actor_id);

-- Audit log MUST be append-only for normal application roles: no UPDATE/DELETE grants.
-- Enforced additionally at the DB role level; see migrations/0003_db_roles.sql.

-- ---------------------------------------------------------------------------
-- Alerts / Agent versions
-- ---------------------------------------------------------------------------

CREATE TABLE alert_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_type  text NOT NULL CHECK (scope_type IN ('global','customer','group','device')),
    scope_id    uuid,
    rule_type   text NOT NULL,
    config      jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled     boolean NOT NULL DEFAULT true
);

CREATE TABLE alerts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rule_id     uuid REFERENCES alert_rules(id) ON DELETE SET NULL,
    severity    text NOT NULL CHECK (severity IN ('warning','critical')),
    state       text NOT NULL DEFAULT 'open' CHECK (state IN ('open','acknowledged','resolved')),
    opened_at   timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    summary     text
);

CREATE INDEX idx_alerts_device_id ON alerts(device_id);

CREATE TABLE agent_versions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version             text NOT NULL,
    os_family           text NOT NULL,
    architecture        text NOT NULL,
    channel             text NOT NULL DEFAULT 'stable',
    artifact_url        text NOT NULL,
    artifact_sha256     bytea NOT NULL,
    signature           bytea NOT NULL,
    published_at        timestamptz NOT NULL DEFAULT now(),
    minimum_supported   boolean NOT NULL DEFAULT false,
    blocked             boolean NOT NULL DEFAULT false,
    UNIQUE(version, os_family, architecture, channel)
);

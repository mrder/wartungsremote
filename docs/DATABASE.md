# WartungsRemote – Datenbankmodell V1

Datenbank: PostgreSQL.

## 1. Grundsätze

- Primärschlüssel bevorzugt UUID.
- Zeitstempel `timestamptz` in UTC.
- Secrets nie im Klartext, wenn Vergleich per Hash möglich ist.
- Soft Delete für fachliche Objekte, wo Historie/Audit erhalten bleiben muss.
- Foreign Keys aktiv.
- Migrationsdateien unveränderlich nach Release.

## 2. Tabellen

### users

```text
id uuid PK
username citext UNIQUE NOT NULL
display_name text
password_hash text NOT NULL
status text NOT NULL
mfa_required boolean NOT NULL default true
created_at timestamptz
updated_at timestamptz
last_login_at timestamptz
```

### user_mfa

```text
user_id uuid PK/FK
secret_ciphertext bytea
secret_key_version int
confirmed_at timestamptz
recovery_code_hashes jsonb
```

TOTP Secret muss verschlüsselt gespeichert werden; Recovery Codes gehasht.

### user_sessions

```text
id uuid PK
user_id uuid FK
session_token_hash bytea UNIQUE
created_at
last_seen_at
expires_at
idle_expires_at
revoked_at
ip inet
user_agent text
```

### roles / permissions / user_roles / role_permissions

Normales RBAC; Permission-Namen eindeutig.

### customers

```text
id uuid PK
name text NOT NULL
customer_number text
status text
notes text
created_at
updated_at
```

### device_groups

```text
id uuid PK
customer_id uuid FK nullable
name text
```

### devices

```text
id uuid PK
install_id uuid UNIQUE NOT NULL
customer_id uuid FK
group_id uuid FK
display_name text
hostname text
os_family text
os_name text
os_version text
architecture text
agent_version text
status text
health text
last_seen_at timestamptz
last_public_ip inet
credential_status text
created_at
updated_at
revoked_at
```

### device_credentials

```text
id uuid PK
device_id uuid FK
credential_type text
public_key bytea
certificate_fingerprint bytea
issued_at
expires_at
revoked_at
key_version int
UNIQUE(device_id, key_version)
```

### enrollment_tokens

```text
id uuid PK
token_hash bytea UNIQUE
customer_id uuid FK
group_id uuid FK
display_name text
metadata jsonb
created_by uuid FK users
created_at
expires_at
consumed_at
consumed_device_id uuid FK
revoked_at
```

### device_capabilities

```text
device_id uuid FK
capability text
version int default 1
PRIMARY KEY(device_id, capability)
```

### device_network

```text
id uuid PK
device_id uuid FK
observed_at timestamptz
interfaces jsonb
public_ip inet
```

### device_metrics

Partitionierung nach Zeit erwägen.

```text
id bigserial PK
device_id uuid FK
observed_at timestamptz
cpu_percent real
memory_used_bytes bigint
memory_total_bytes bigint
filesystems jsonb
network jsonb
```

Index `(device_id, observed_at desc)`.

### device_events

```text
id uuid PK
device_id uuid FK
event_type text
severity text
occurred_at timestamptz
payload jsonb
```

### remote_sessions

```text
id uuid PK
device_id uuid FK
user_id uuid FK
maintenance_session_id uuid FK
kind text
state text
opened_at
closed_at
expires_at
close_reason text
```

### privilege_sessions

```text
id uuid PK
remote_session_id uuid FK
user_id uuid FK
device_id uuid FK
created_at
valid_until
revoked_at
authorization_reason text
```

### remote_commands

```text
id uuid PK
device_id uuid FK
remote_session_id uuid FK nullable
command_type text
state text
created_at
expires_at
dispatched_at
completed_at
result_code text
request_payload jsonb
result_summary jsonb
```

Keine Secrets in Payload-Feldern speichern.

### tunnel_sessions

```text
id uuid PK
remote_session_id uuid FK
device_id uuid FK
user_id uuid FK
target_type text
state text
helper_ticket_hash bytea
created_at
expires_at
connected_at
closed_at
bytes_up bigint
bytes_down bigint
```

### maintenance_sessions

```text
id uuid PK
device_id uuid FK
customer_id uuid FK
user_id uuid FK
started_at
ended_at
result text
summary text
next_maintenance_at timestamptz
```

### maintenance_events

```text
id uuid PK
maintenance_session_id uuid FK
event_type text
occurred_at
summary text
reference_id uuid nullable
```

### audit_log

Append-orientiert.

```text
id bigserial PK
occurred_at timestamptz NOT NULL
actor_type text NOT NULL
actor_id uuid
session_id uuid
device_id uuid
customer_id uuid
event_type text NOT NULL
result text NOT NULL
source_ip inet
request_id uuid
metadata jsonb
prev_hash bytea nullable
entry_hash bytea nullable
```

Optional kann eine Hashkette Manipulation erkennbarer machen. Sie ersetzt kein externes Backup/Log-Shipping.

### alert_rules

```text
id uuid PK
scope_type text
scope_id uuid
rule_type text
config jsonb
enabled boolean
```

### alerts

```text
id uuid PK
device_id uuid FK
rule_id uuid FK
severity text
state text
opened_at
resolved_at
summary text
```

### agent_versions

```text
id uuid PK
version text
os_family text
architecture text
channel text
artifact_url text
artifact_sha256 bytea
signature bytea
published_at
minimum_supported boolean
```

## 3. Retention

Default-Vorschlag:

- Audit: 365 Tage oder länger entsprechend betrieblicher Vorgabe.
- Wartungshistorie: dauerhaft, bis bewusste Retention-Policy greift.
- Rohmetriken 5-Minuten: 30 Tage.
- 1h-Aggregate: 365 Tage.
- Device Events: 180 Tage.
- abgelaufene Sessions: 90 Tage, Audit bleibt separat.

Retention muss konfigurierbar sein.

## 4. Transaktionale Anforderungen

Atomar ausführen:

- Enrollment Token konsumieren + Device erstellen.
- Credential revoken + Sessions sperren.
- Privilege Session erstellen nach erfolgreicher Reauth.
- Tunnel Ticket von `issued` auf `used` setzen.

## 5. Datenbankrollen

Mindestens getrennte DB-Benutzer erwägen:

- Runtime App.
- Migration/DDL.
- Read-only Reporting.

Produktions-App erhält keine unnötigen Superuser-Rechte.

# WartungsRemote – Technische Architektur V1

## 1. Komponenten

```text
Agent
  |
  | TLS 1.3
  v
Public Gateway / Relay
  |
  +---- Core API
  |
  +---- Session Broker
  |
  +---- PostgreSQL

Admin Browser
  |
  | internes Netz / VPN
  v
Admin Web
```

## 2. Vorgeschlagene Ports

Serverseitig:

```text
443  TCP  Agent Gateway / öffentliche API
8443 TCP  Relay optional separat
9443 TCP  Admin Web, nur intern
```

Für MVP können Gateway und Relay beide auf 443 über unterschiedliche Pfade/Protokolle laufen.

## 3. Protokollnachrichten

Grundformat:

```json
{
  "protocol": 1,
  "type": "heartbeat",
  "request_id": "uuid",
  "timestamp": "2026-08-23T12:00:00Z",
  "payload": {}
}
```

## 4. Agent Capabilities

Beispiel Linux:

```json
{
  "capabilities": [
    "metrics",
    "terminal",
    "files",
    "systemd",
    "ssh_tunnel"
  ]
}
```

Beispiel Windows:

```json
{
  "capabilities": [
    "metrics",
    "powershell",
    "files",
    "windows_services",
    "rdp_tunnel"
  ]
}
```

## 5. Enrollment Flow

```text
Admin -> Server: Enrollment Token erzeugen
Agent -> Server: Token + Public Key + Systemdaten
Server -> DB: Token prüfen
Server -> DB: Device erstellen
Server -> Agent: Device ID + Credential
Server -> DB: Token invalidieren
Agent -> Server: neuer authentifizierter Connect
```

## 6. Session Flow

```text
Admin klickt Terminal
       |
       v
Core prüft Benutzerrechte
       |
       v
Core prüft Device online
       |
       v
Broker erstellt Remote Session
       |
       v
Agent erhält Session Request
       |
       v
Agent bestätigt
       |
       v
bidirektionaler Stream
```

## 7. Privilege Flow

```text
Remote Session aktiv
      |
Admin fordert erhöhte Rechte
      |
Server verlangt Reauthentication + TOTP
      |
Privilege Session wird erstellt
      |
Gültigkeit z.B. 15 Minuten
      |
Ablauf -> Rechte automatisch entziehen
```

## 8. SSH Tunnel Flow

```text
Admin UI -> Tunnel anfordern
Core -> Session Authorization
Helper/Admin Client -> lokaler Port 10022
Relay -> Agent Stream
Agent -> TCP 127.0.0.1:22
```

## 9. RDP Tunnel Flow

```text
Admin UI -> Tunnel anfordern
Core -> Session Authorization
Helper/Admin Client -> lokaler Port 15389
Relay -> Agent Stream
Agent -> TCP 127.0.0.1:3389
```

## 10. API V1 grob

### Agent

```text
POST /api/v1/agent/enroll
GET  /api/v1/agent/config
POST /api/v1/agent/status
POST /api/v1/agent/events
GET  /api/v1/agent/update
WS   /api/v1/agent/control
```

### Admin

```text
POST   /api/v1/auth/login
POST   /api/v1/auth/totp
POST   /api/v1/auth/logout
GET    /api/v1/devices
GET    /api/v1/devices/:id
POST   /api/v1/devices/:id/status
POST   /api/v1/devices/:id/test
POST   /api/v1/devices/:id/terminal
POST   /api/v1/devices/:id/ssh-tunnel
POST   /api/v1/devices/:id/rdp-tunnel
GET    /api/v1/devices/:id/audit
GET    /api/v1/devices/:id/maintenance
```

## 11. Datenbanktabellen

Empfohlen:

```text
users
user_mfa
user_sessions
roles
permissions
customers
device_groups
devices
device_credentials
device_capabilities
device_network
device_metrics
device_events
enrollment_tokens
remote_sessions
remote_streams
remote_commands
privilege_sessions
maintenance_sessions
maintenance_notes
audit_log
agent_versions
alert_rules
alerts
```

## 12. Security Boundaries

### Public Zone

- Agent Gateway
- Relay

### Internal Zone

- Core API
- PostgreSQL
- Admin Web

Admin Web soll nicht über denselben öffentlichen Listener erreichbar sein.

## 13. Secrets

Nicht in Git:

- DB Passwort
- JWT/Session Secrets
- CA Private Key
- Release Signing Private Key
- TOTP Secrets im Klartext

Release Signing Key möglichst offline oder in separatem sicheren Build-System.


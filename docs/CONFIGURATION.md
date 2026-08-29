# WartungsRemote – Konfigurationsreferenz V1

## 1. Serverkonfiguration

Beispiel `server.yaml` – Secrets werden nicht direkt in diese Datei geschrieben, sondern über Secret-Dateien/Environment referenziert.

```yaml
mode: production

public:
  base_url: https://remote.example.de
  listen: 0.0.0.0:8080

admin:
  listen: 127.0.0.1:9443
  session_absolute_ttl: 8h
  session_idle_ttl: 30m
  privilege_ttl: 15m
  require_mfa: true

agent:
  heartbeat_interval: 45s
  connection_lost_after: 120s
  offline_after: 300s
  status_interval: 5m
  network_upload_interval: 5m
  enrollment_ttl: 30m
  reconnect_max_backoff: 5m

relay:
  ticket_ttl: 60s
  max_tunnels_per_user: 5
  max_tunnels_per_device: 3
  max_session_duration: 8h

security:
  session_cookie_name: __Host-wr_session
  csrf_enabled: true
  hsts_enabled: true
  trusted_proxies: []

metrics:
  raw_retention: 720h
  hourly_retention: 8760h
  network_raw_retention: 168h
  network_hourly_retention: 8760h

help:
  content_dir: /app/docs
```

`security.trusted_proxies` ist standardmäßig leer — dann wird `X-Forwarded-For`
grundsätzlich ignoriert und immer die rohe TCP-Peer-Adresse als Client-IP
verwendet (relevant für Device-IP-Tracking und Audit-Einträge). Ohne diese
Absicherung könnte jeder Aufrufer (auch ein böswilliger Agent) per Header
eine beliebige IP vortäuschen. Nur setzen, wenn wr-core tatsächlich hinter
einem Reverse Proxy (nginx/Traefik/Cloudflare Tunnel) für eigene
TLS-Terminierung läuft — dann die exakte IP/CIDR dieses Proxys eintragen,
sonst nichts. Ein direktes Docker-Port-Mapping ohne Reverse Proxy braucht
das nicht: Docker reicht die echte Client-IP bereits unverändert durch.

`agent.network_upload_interval` steuert, wie oft ein Agent seine lokal
gepufferten Netzwerk-Traffic-Samples hochlädt (docs/AGENT.md
"Netzwerk-Traffic-Metriken") — unabhängig von `status_interval`, da die
lokale Sammelrate agentseitig fest ist (60s) und nur das Hochladen
serverseitig gesteuert wird. `metrics.network_raw_retention`/
`network_hourly_retention` gelten nur für diese Traffic-Historie (eigene
Tabelle, andere Aufnahmerate als CPU/RAM/Disk) und sind zusätzlich über
das Dashboard (Settings) zur Laufzeit überschreibbar, genau wie
`metrics.raw_retention`/`hourly_retention`.

## 2. Umgebungsvariablen / Secrets

Beispielnamen:

```text
WR_DATABASE_URL_FILE
WR_SESSION_PEPPER_FILE
WR_TOTP_ENCRYPTION_KEY_FILE
WR_INTERNAL_SERVICE_KEY_FILE
```

Nicht unterstützen:

```text
WR_DISABLE_TLS_VERIFY=true
WR_ALLOW_ANY_TUNNEL=true
WR_DEFAULT_ADMIN_PASSWORD=...
```

## 3. Agentkonfiguration

```yaml
server_url: https://remote.example.de
update_channel: stable
log_level: info

policy:
  terminal: true
  ssh_tunnel: true
  rdp_tunnel: true
  files_read: true
  files_write: true
  service_control: true
  process_terminate: true
  power_control: true
```

OS-unpassende Optionen werden ignoriert oder als unavailable gemeldet, aber nicht als Fehler des gesamten Agents behandelt.

## 4. Lokale Agentpolicy

Serverpermission UND lokale Agentpolicy müssen beide erlauben.

Beispiel: Wenn `rdp_tunnel: false`, kann auch ein Superadmin keinen RDP-Tunnel über diesen Agent erzeugen, bis die lokale Policy geändert wurde.

## 5. Konfigurationsvalidierung

Beim Start:

- unbekannte kritische Keys -> Warnung oder Fehler entsprechend Schema.
- negative/unsinnige Timeouts -> Startfehler.
- Production ohne MFA -> Startfehler für Remote-Admin-Funktionen.
- Production mit unsicherer Cookie-/TLS-Konfiguration -> Startfehler.

## 6. Konfigurationspriorität

Empfehlung:

```text
Defaults
< config file
< environment non-secret overrides
< secret files
```

Security Defaults dürfen nicht durch leere Werte versehentlich deaktiviert werden.

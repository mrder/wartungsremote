# WartungsRemote – HTTP API V1

Basis: `/api/v1`  
Content-Type: `application/json`  
Admin API nur über internes/VPN-erreichbares Web-Gateway.

## 1. Konventionen

Erfolg:

```json
{"data": {}, "meta": {}}
```

Fehler:

```json
{
  "error": {
    "code": "permission_denied",
    "message": "Not permitted",
    "request_id": "..."
  }
}
```

Keine internen Stacktraces an Clients.

## 2. Auth

### POST `/auth/login`

Request:

```json
{"username":"admin","password":"..."}
```

Response bei aktiviertem TOTP:

```json
{"data":{"state":"mfa_required","challenge_id":"..."}}
```

### POST `/auth/totp`

```json
{"challenge_id":"...","code":"123456"}
```

Erfolg setzt serverseitige Session + Secure HttpOnly Cookie.

### POST `/auth/logout`

Invalidiert aktuelle Session serverseitig.

### POST `/auth/reauth`

Für sensible Aktionen/Privilege Session. Liefert kurzlebige `reauth_id`, nicht den Hauptsessiontoken.

## 3. Aktueller Benutzer

### GET `/me`

Liefert User, Rollen, Permissions, Sessionablauf und MFA-Status.

## 4. Enrollment

### POST `/enrollments`

Permission: `device.manage`

```json
{
  "customer_id":"...",
  "group_id":"...",
  "display_name":"Server 01",
  "expires_in_seconds":1800,
  "tags":["server"],
  "reusable":false
}
```

Response zeigt das Token **einmalig**.

`reusable: true` erlaubt beliebig viele Geräte mit demselben Token bis
Ablauf/Widerruf (Bulk-Rollout, docs/AGENT.md §5) — bewusst akzeptiertes
Risiko: wer das Token kennt, kann zusätzliche Geräte anmelden, aber nicht
mehr als das (kein Zugriff auf bestehende Geräte/Dashboard). Deutlich
längere maximale Gültigkeit als ein Einweg-Token (90 Tage statt 24h).

### GET `/enrollments`

Listet noch nutzbare (nicht abgelaufene/widerrufene/verbrauchte) Tokens.
Keine Token-Klartexte zurückgeben — nur Metadaten (Typ, Nutzungszähler,
Ablauf) zum gezielten Widerrufen eines einzelnen Tokens.

### DELETE `/enrollments/:id`

Widerruft nicht verwendetes Enrollment.

### POST `/agent/enroll`

Öffentlicher Agent-Endpunkt. Rate-limited. Details in PROTOCOL/SECURITY.

## 5. Devices

### GET `/devices`

Filter:

- `status`
- `health`
- `customer_id`
- `group_id`
- `tag`
- `q`
- `page`
- `page_size`

### GET `/devices/:id`

### PATCH `/devices/:id`

Änderbar:

- display_name
- customer/group
- tags
- policies
- notes metadata

Nicht direkt änderbar: kryptografische Identität.

### POST `/devices/:id/status-request`

Fordert aktuellen Status an.

### POST `/devices/:id/connection-test`

Startet Agent-Roundtrip-/Capability-Test.

### POST `/devices/:id/revoke`

Reauthentication erforderlich. Widerruft Device Credential und beendet Sessions.

## 6. Metrics

### GET `/devices/:id/metrics?from=...&to=...&resolution=...`

Resolution serverseitig begrenzen.

### GET `/devices/:id/health`

Aktuelle Bewertung + Gründe.

## 7. Remote Sessions

### POST `/devices/:id/sessions`

```json
{"kind":"terminal"}
```

Response:

```json
{"data":{"session_id":"...","state":"opening","expires_at":"..."}}
```

### GET `/sessions/:id`

### DELETE `/sessions/:id`

Beendet Session.

## 8. Privilege Sessions

### POST `/sessions/:id/privilege`

```json
{"reauth_id":"...","duration_seconds":900}
```

Server begrenzt Dauer per Policy.

### DELETE `/sessions/:id/privilege`

Sofort entziehen.

## 9. Tunnel

### POST `/devices/:id/tunnels`

```json
{"target_type":"ssh_local"}
```

oder

```json
{"target_type":"rdp_local"}
```

Response:

```json
{
  "data": {
    "tunnel_id":"...",
    "helper_ticket":"ONE_TIME_SECRET",
    "expires_at":"...",
    "suggested_local_port":41022
  }
}
```

`helper_ticket` wird einmalig ausgegeben und serverseitig nur gehasht gespeichert.

### DELETE `/tunnels/:id`

### GET `/devices/:id/support-credential`

Erfordert `remote.tunnel.ssh` oder `remote.tunnel.rdp`. Gibt das aktuelle
Login für den dedizierten `remotewartung`-Account entschlüsselt zurück
(docs/AGENT.md §12a). Jeder Aufruf wird auditiert
(`support_credential.revealed`). `404`, falls das Gerät noch keinen Report
gesendet hat.

```json
{"data": {"username":"remotewartung","password":"...","updated_at":"..."}}
```

### POST `/devices/:id/support-credential/rotate`

Erfordert `remote.tunnel.ssh` oder `remote.tunnel.rdp`, Gerät muss online
sein. Weist den Agenten an, ein neues Passwort zu setzen und zu melden;
Antwort kommt asynchron über den Control-Channel, nicht in dieser Response.

## 10. Dateien

### GET `/devices/:id/files?path=...`

Listet Verzeichnis.

### GET `/devices/:id/files/download?path=...`

Download über autorisierte Stream-Session.

### POST `/devices/:id/files/upload`

Multipart oder Upload-Session; Größenlimits zwingend.

### POST `/devices/:id/files/mkdir`
### POST `/devices/:id/files/rename`
### DELETE `/devices/:id/files`

Schreiboperationen benötigen passende Permission und ggf. Privilege Session.

## 11. Services

### GET `/devices/:id/services`
### POST `/devices/:id/services/:service/start`
### POST `/devices/:id/services/:service/stop`
### POST `/devices/:id/services/:service/restart`

Service-Namen werden validiert und agentseitig systemnativ behandelt; keine Shell-String-Konkatenation.

## 12. Prozesse

### GET `/devices/:id/processes`
### POST `/devices/:id/processes/:pid/terminate`

PID + Prozess-Startzeit/Identity sollen gemeinsam geprüft werden, um PID-Reuse-Risiken zu reduzieren.

## 13. Power

### POST `/devices/:id/power/restart`
### POST `/devices/:id/power/shutdown`

Privilege + explizite UI-Bestätigung + Audit.

## 14. Wartung

### GET `/devices/:id/maintenance`
### POST `/devices/:id/maintenance`
### PATCH `/maintenance/:id`
### POST `/maintenance/:id/close`

## 15. Audit

### GET `/audit`
### GET `/devices/:id/audit`

Filter nach User, Device, Eventtyp, Zeitraum. Keine Lösch-API für normale Adminrollen.

### POST `/audit/verify`

Permission: `audit.read`. Rechnet die gesamte Hash-Chain
(`prev_hash`/`entry_hash`, docs/SECURITY.md §16) neu und vergleicht sie
mit den gespeicherten Werten — erkennt jede Änderung an bestehenden
Einträgen, auch direkt in der Datenbank (unabhängig vom Append-only-
Trigger). Read-only, aber O(n) über die gesamte Tabelle, daher eine
gezielte Admin-Aktion statt automatisch bei jedem Seitenaufruf.

```json
{"data": {"Valid": true, "EntriesCheck": 4, "EntriesPreChain": 29, "BrokenAtID": null}}
```

`EntriesPreChain` zählt führende Einträge von vor Einführung dieses
Features (kein `entry_hash` vorhanden) — kein Hinweis auf Manipulation,
nur ein Bestand aus der Zeit davor.

## 16. Kunden/Gruppen

CRUD:

- `/customers`
- `/groups`
- `/tags`

Löschung bevorzugt Soft Delete bei referenzierten Objekten.

## 17. Benutzer/Rollen

- `/users`
- `/roles`
- `/permissions`

Änderungen sind Audit-pflichtig.

## 18. Agent Updates

### GET `/agent/releases`
### POST `/agent/releases`
### POST `/devices/:id/update`

Release-Paket muss bereits extern signiert worden sein; Server erzeugt nicht ad hoc eine vertrauenswürdige Signatur mit einem online verfügbaren Produktionskey.

## 19. Hilfe

### GET `/help/index`
### GET `/help/:slug`

Help-Inhalte werden aus versionierten Markdown-Dateien gerendert. HTML muss sanitisiert werden.

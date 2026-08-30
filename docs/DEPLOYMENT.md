# WartungsRemote – Deployment V1

## 1. Empfohlenes Serverlayout

Docker Compose auf einem Linux-Server.

```text
Internet
   |
Firewall
   | 443
reverse proxy / gateway
   |
+-- wr-gateway
+-- wr-relay
+-- wr-core
+-- wr-web (nur intern veröffentlicht)
+-- postgres (nur internes Netzwerk)
```

## 2. Admin-Web

Produktionsdefault:

- bindet nicht öffentlich.
- Zugriff über LAN oder VPN.
- Reverse Proxy darf Admin-Host nur aus erlaubten Netzen weiterleiten.

Beispiel separate Namen:

```text
remote.example.de       -> öffentlicher Agent Gateway 443
remote-admin.internal   -> Admin Web intern/VPN
```

Referenzimplementierung: der `wr-web`-Service in
`deployment/docker/docker-compose.yml` baut das Dashboard (`web/`) und
serviert es statisch, inklusive `/api`-Reverse-Proxy zu `wr-core` — beides
nur auf `127.0.0.1` des Hosts gebunden, per SSH-Tunnel/VPN erreichbar
(README → "Docker Compose deployment"). Wichtig: `wr-core`s
`admin.listen` steht im Docker-Setup bewusst auf `0.0.0.0` statt
`127.0.0.1` — die eigentliche Absicherung kommt dort aus dem internen,
nicht-öffentlichen Docker-Netz plus dem loopback-gebundenen
Host-Port-Mapping, nicht aus der Bind-Adresse selbst (ein auf `127.0.0.1`
gebundener Prozess *innerhalb* eines Containers wäre weder von anderen
Containern noch über published Ports überhaupt erreichbar).

## 3. Firewall

Öffentlich:

- allow TCP 443.
- allow TCP 80 (nur für die Let's-Encrypt-Zertifikatsanfrage/-Erneuerung
  und HTTP→HTTPS-Redirect durch Caddy, kein Anwendungsverkehr).
- SSH zum Wartungsserver nur aus Adminnetz/VPN.
- DB niemals öffentlich.
- Admin-Web niemals pauschal öffentlich.
- Bei Internet-Erreichbarkeit: Port 8443 extern **nicht** freigeben — der
  ist im Referenz-Compose-Setup nur für LAN-only-Tests ohne Domain gedacht
  (siehe `deployment/docker/docker-compose.yml`); Caddy auf 443 ist der
  einzige vorgesehene öffentliche Zugang zum Agent-Gateway.

## 4. Reverse Proxy

Muss WebSocket Upgrade unterstützen.

Anforderungen:

- Request Size Limits passend.
- lange Relay/Control Timeouts.
- TLS.
- Security Header für Admin-Web.
- getrennte Access Logs ohne Secrets.

Referenzimplementierung: `deployment/docker/docker-compose.yml` bringt
dafür einen `caddy`-Service mit (`deployment/docker/Caddyfile`) — holt und
erneuert automatisch ein Let's-Encrypt-Zertifikat für die in `.env`
gesetzte `WR_DOMAIN`, WebSocket-Upgrade läuft ohne Zusatzkonfiguration
durch `reverse_proxy`, Timeouts sind für die langlebigen
Control-Channel-Verbindungen bewusst deaktiviert. Nur der Agent-Gateway
(8443intern/443extern) läuft über Caddy — das Admin-Web bleibt
`127.0.0.1`-only und wird nie über Caddy geroutet.

Damit der Server dem `X-Forwarded-For`-Header von Caddy dann auch traut
(sonst würde die echte Client-IP im Audit-Log/Device-Tracking verloren
gehen), muss `security.trusted_proxies` in `server.yaml` genau das
Docker-Netz von Caddy benennen — im Referenz-Setup bereits vorkonfiguriert
(`server.example.yaml`). Ohne einen echten Reverse Proxy davor muss diese
Liste leer bleiben, siehe docs/CONFIGURATION.md.

## 5. Secrets

Per Docker Secrets/Secret Files oder vergleichbarem Secret Store:

- DB Passwort.
- Session Pepper/Keys.
- TOTP Encryption Key.
- interne Service Credentials.

Release Signing Private Key **nicht** hier speichern.

**Vor dem ersten echten Release ein eigenes Produktions-Schlüsselpaar
erzeugen** — niemals einen zu Test-/Entwicklungszwecken erzeugten
Schlüssel weiterverwenden:

```bash
wr-release-sign -genkey -out wartungsremote-release
```

Erzeugt `wartungsremote-release.key` (privat) und
`wartungsremote-release.pub` (öffentlich). Nur die `.pub`-Datei geht auf
den Server (`WR_RELEASE_PUBLIC_KEY_FILE`) — die `.key`-Datei verlässt
den Rechner, auf dem sie erzeugt wurde, im Idealfall nie wieder (siehe
docs/AGENT.md §15, cmd/wr-release-sign). Ohne konfigurierten
öffentlichen Schlüssel lehnt der Server `POST /agent/releases` ohnehin
grundsätzlich ab (`412 not_configured`) — es gibt bewusst keinen
automatisch erzeugten Schlüssel als Fallback, anders als z. B. beim
Session Pepper im reinen Dev-Modus.

## 5a. Datenbank-Runtime-User ohne Superuserrechte

Migrationen (Schema-DDL) laufen mit einer Owner-/Superuser-Rolle — das ist
der einzige Zeitpunkt, an dem DDL-Rechte gebraucht werden. Der laufende
wr-core-Prozess soll dagegen **keine** Superuserrechte besitzen. wr-core
unterstützt dafür zwei getrennte DSNs: `WR_MIGRATION_DATABASE_URL_FILE`
(Owner-Rolle, nur für die Migrationen beim Start) und
`WR_DATABASE_URL_FILE` (die eingeschränkte Rolle, für den laufenden
Betrieb) — fehlt die erste, wird die zweite für beides verwendet (bisheriges
Verhalten, unverändert für native Installationen ohne diese Trennung).

**Docker Compose: automatisch.** Der `db-init`-Service legt
`wartungsremote_app` bei jedem Start an/aktualisiert sie (idempotent,
per `scripts/ensure-db-runtime-role.sql`), `wr-core` ist bereits auf die
getrennten DSNs verdrahtet (`deployment/docker/docker-compose.yml`) —
nichts weiter zu tun außer `generate-docker-secrets.sh` einmal laufen zu
lassen.

**Native Installation: manuell**, per `scripts/create-db-runtime-role.sql`
(die nicht-idempotente Variante, für einen einmaligen manuellen Lauf):

```bash
psql "$ADMIN_DSN" -v app_password="'<starkes Passwort>'" \
  -f scripts/create-db-runtime-role.sql
```

Danach `WR_MIGRATION_DATABASE_URL_FILE` auf die bisherige Owner-DSN und
`WR_DATABASE_URL_FILE` auf die neue Rolle `wartungsremote_app` umstellen.
Diese Rolle kann lesen/schreiben, aber keine Tabellen
anlegen/ändern/löschen und keine weiteren Rollen erstellen — ein
kompromittiertes DB-Credential oder eine SQL-Injection bleibt damit auf
die Anwendungsdaten begrenzt statt die gesamte Postgres-Instanz zu
gefährden. Künftige Migrationen erweitern die Tabellen automatisch
(`ALTER DEFAULT PRIVILEGES`), ein erneuter Lauf des Skripts ist nur bei
Rollenänderungen nötig.

Live geprüft: lokal mit getrennten DSNs gestartet — Migrationen liefen
korrekt über die Owner-Rolle, der laufende Server (inkl. echtem Login)
funktionierte anschließend vollständig über die eingeschränkte Rolle
ohne DDL-Rechte.

## 6. Backups

Mindestens täglich:

- PostgreSQL.
- Server-Konfiguration.
- interne CA, falls verwendet.
- Encryption Keys für TOTP/Secrets.
- Help/Policy-Versionen.

Backup verschlüsseln und Restore testen.

Ohne Secret-Encryption-Key ist ein DB-Backup ggf. absichtlich nicht vollständig nutzbar; daher Key-Recovery separat planen.

Referenzimplementierung für das Docker-Compose-Setup:
`scripts/backup-server.sh` sichert DB-Dump + `server.yaml`/`.env` +
`secrets/` in ein Archiv, optional AES-256-verschlüsselt
(`--encrypt-passphrase-file`), mit konfigurierbarer Aufbewahrungsdauer
(`--retention-days`, löscht ältere Archive automatisch). Nichts davon ist
fest verdrahtet — Backup-Verzeichnis, Compose-Pfad, Verschlüsselung und
Aufbewahrung sind alles Flags/Env-Variablen.

```bash
sudo ./scripts/install-backup-cron.sh \
  --schedule "15 3 * * *" \
  --backup-dir /var/backups/wartungsremote \
  --retention-days 14
  # optional: --encrypt-passphrase-file /pfad/zur/passphrase.txt
```

Erneutes Ausführen ersetzt den eigenen Cron-Eintrag statt einen zweiten
anzulegen — Zeitplan/Einstellungen also einfach durch erneuten Aufruf
ändern, oder direkt die generierte `<backup-dir>/backup.env` editieren.

## 7. Monitoring des Wartungsservers

Eigene Checks:

- Gateway erreichbar.
- Control Connections.
- Relay Sessions.
- DB Health.
- Disk usage.
- Backup success.
- Zertifikatsablauf.
- Queue backlog.
- 5xx rate.

## 8. Updates Server

- Migrationen vor App-Start kontrolliert anwenden.
- DB-Backup vor größeren Upgrades.
- Rollbackplan.
- Core/Gateway Protokollkompatibilität beachten.

## 9. Entwicklungsumgebung

Lokale Compose-Umgebung darf Self-Signed/Local CA verwenden. Produktionscode darf Zertifikatsprüfung dadurch nicht generell deaktivieren.

## 10. Production Mode

Konfigurationsflag `production=true` erzwingt:

- MFA für Admin Remote-Zugriff.
- Secure Cookies.
- TLS-only.
- keine Debug-Endpoints öffentlich.
- kein Defaultpasswort.
- keine Test-Enrollment-Tokens.
- keine unsignierten Agentupdates.

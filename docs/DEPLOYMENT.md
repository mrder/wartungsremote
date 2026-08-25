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

## 3. Firewall

Öffentlich:

- allow TCP 443.
- SSH zum Wartungsserver nur aus Adminnetz/VPN.
- DB niemals öffentlich.
- Admin-Web niemals pauschal öffentlich.

## 4. Reverse Proxy

Muss WebSocket Upgrade unterstützen.

Anforderungen:

- Request Size Limits passend.
- lange Relay/Control Timeouts.
- TLS.
- Security Header für Admin-Web.
- getrennte Access Logs ohne Secrets.

## 5. Secrets

Per Docker Secrets/Secret Files oder vergleichbarem Secret Store:

- DB Passwort.
- Session Pepper/Keys.
- TOTP Encryption Key.
- interne Service Credentials.

Release Signing Private Key **nicht** hier speichern.

## 5a. Datenbank-Runtime-User ohne Superuserrechte

Migrationen (Schema-DDL) laufen mit einer Owner-/Superuser-Rolle — das ist
der einzige Zeitpunkt, an dem DDL-Rechte gebraucht werden. Der laufende
wr-core-Prozess soll dagegen **keine** Superuserrechte besitzen:

```bash
psql "$ADMIN_DSN" -v app_password="'<starkes Passwort>'" \
  -f scripts/create-db-runtime-role.sql
```

Danach `WR_DATABASE_URL_FILE` auf die neue Rolle `wartungsremote_app`
umstellen. Diese Rolle kann lesen/schreiben, aber keine Tabellen
anlegen/ändern/löschen und keine weiteren Rollen erstellen — ein
kompromittiertes DB-Credential oder eine SQL-Injection bleibt damit auf
die Anwendungsdaten begrenzt statt die gesamte Postgres-Instanz zu
gefährden. Künftige Migrationen erweitern die Tabellen automatisch
(`ALTER DEFAULT PRIVILEGES`), ein erneuter Lauf des Skripts ist nur bei
Rollenänderungen nötig.

## 6. Backups

Mindestens täglich:

- PostgreSQL.
- Server-Konfiguration.
- interne CA, falls verwendet.
- Encryption Keys für TOTP/Secrets.
- Help/Policy-Versionen.

Backup verschlüsseln und Restore testen.

Ohne Secret-Encryption-Key ist ein DB-Backup ggf. absichtlich nicht vollständig nutzbar; daher Key-Recovery separat planen.

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

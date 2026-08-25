# WartungsRemote – Test- und Abnahmeplan V1

## 1. Unit Tests

- Token Hash/Verify.
- Password Hash/Verify.
- Permission Matrix.
- Session expiry.
- Privilege expiry.
- target_type Mapping.
- Path canonicalization.
- protocol validation.
- health rule evaluation.

## 2. Integration Tests

- PostgreSQL Migrationen leer -> aktuell.
- Login + TOTP.
- Enrollment atomar.
- Agent Control Connect.
- Heartbeat/Offline transition.
- Status request.
- terminal open/close.
- privilege grant/revoke/expire.
- SSH tunnel localhost only.
- RDP tunnel localhost only.
- file upload/download.
- service action.
- process termination.
- agent update staging/rollback.

## 3. Security Tests

### Enrollment

- falsches Token.
- abgelaufen.
- revoked.
- zweimal gleichzeitig verwenden.
- Token aus DB-Hash zurückgewinnen nicht möglich.

### Auth

- brute-force rate limit.
- TOTP replay im selben Zeitfenster nach Policy behandeln.
- Session fixation.
- CSRF.
- stolen cookie logout/revoke.

### Authorization

- read_only versucht Terminal.
- technician greift Device eines verbotenen Kunden an.
- IDOR durch geänderte UUID.
- Privilege abgelaufen.

### Relay

- Ticket zweimal.
- Ticket für falsches Gerät.
- SSH-Ticket als RDP verwenden.
- Versuch `8.8.8.8:53` als Target einzuschleusen.
- Helper bindet nicht extern.

### Files

- `../../etc/shadow`.
- Symlink aus erlaubtem Root.
- Windows Junction/Reparse Point.
- übergroßer Upload.
- Dateiname mit Sonderzeichen.

### Protocol

- malformed JSON.
- riesige Frames.
- unbekannte type.
- falsche protocol version.
- duplicate command ID.

### Update

- falscher Hash.
- falsche Signatur.
- altes manipuliertes Manifest.
- Downloadabbruch.
- neuer Agent startet nicht -> Rollback.

## 4. Plattformtests Linux

Mindestens:

- Debian Stable x86_64.
- Ubuntu LTS x86_64.
- ARM64 Linux/Raspberry Pi OS.
- Unraid unterstützter Installationspfad gesondert.

Prüfen:

- Service Autostart.
- Reboot.
- Netzwerkunterbrechung.
- Shell/PTY.
- SSH Tunnel.
- systemd soweit vorhanden.

## 5. Plattformtests Windows

- Windows 10.
- Windows 11.
- Windows Server 2019/2022+.

Prüfen:

- Service Installation.
- Boot.
- PowerShell/ConPTY.
- RDP deaktiviert -> sauberer Fehler.
- RDP aktiviert -> Tunnel.
- Windows Service Control.
- Update/Rollback.

## 6. NAT/Netztests

- normales NAT.
- Double NAT.
- CGNAT/LTE sofern Testumgebung verfügbar.
- WAN-IP-Wechsel.
- DNS-Ausfall.
- Server 30 Minuten offline.
- Proxy/Firewall, die WebSockets erlaubt.

## 7. Lasttests

Stufen:

- 100 Agents.
- 1.000 Agents.
- 10.000 simulierte Heartbeats als späteres Skalierungsziel.

Messen:

- RAM pro Connection.
- CPU.
- DB writes.
- reconnect storm.
- dashboard list latency.

## 8. Soak Test

Vor V1:

- mindestens 24h Windows-Agent online.
- mindestens 24h Linux-Agent online.
- wiederholte Netzwerkunterbrechungen.
- mehrere Terminal/Tunnel-Sessions.
- keine stetig wachsenden Goroutines/Handles/RAM.

## 9. Abnahme-Matrix

Jede TODO-Phase gilt erst als abgeschlossen, wenn:

- Code implementiert.
- Tests vorhanden.
- Security-Auswirkungen geprüft.
- Dokumentation aktualisiert.
- TODO abgehakt.

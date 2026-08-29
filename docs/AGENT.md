# WartungsRemote – Agent Design V1

## 1. Prozessmodell

Binary: `wr-agent`.

Linux: systemd Service.  
Windows: Windows Service.

Agent startet beim Booten und besitzt keine GUI-Pflicht.

## 2. Verzeichnisse

Linux Vorschlag:

```text
/etc/wartungsremote/agent.yaml
/var/lib/wartungsremote/
/var/log/wartungsremote/
```

Windows Vorschlag:

```text
%ProgramData%\WartungsRemote\config\
%ProgramData%\WartungsRemote\data\
%ProgramData%\WartungsRemote\logs\
```

Secrets niemals world-readable.

## 3. Konfiguration

Nicht-sensitive Konfiguration:

```yaml
server_url: https://remote.example.de
log_level: info
update_channel: stable
```

Enrollment Token soll bevorzugt einmalig per Installerparameter/temporärer Datei übergeben und nach erfolgreichem Enrollment gelöscht werden.

## 4. Agent State Machine

```text
UNENROLLED
  -> ENROLLING
  -> ENROLLED_DISCONNECTED
  -> CONNECTING
  -> ONLINE
  -> DEGRADED
  -> CONNECTING ...

REVOKED -> keine Remote-Funktionen, nur klarer lokaler Fehlerzustand
```

## 5. Startup

1. Konfiguration laden.
2. Dateirechte prüfen.
3. Device Credential laden.
4. falls nicht enrolled: Enrollment nur bei vorhandenem Token.
5. Systeminventar sammeln.
6. Control Channel aufbauen.
7. Capability Handshake.
8. Worker starten.

## 6. Concurrency

Getrennte begrenzte Worker/Queues für:

- Control RX/TX.
- Metrics.
- Remote Sessions.
- File Transfers.
- Update.

Keine unbeschränkten Goroutine-/Thread-Spawns pro Nachricht.

## 7. Reconnect

Exponentieller Backoff mit Jitter. Kein aggressives 100-ms-Reconnect-Loopen.

Bei Netzwerkwechsel darf sofortiger neuer Versuch ausgelöst werden.

## 8. Capabilities

Capabilities werden tatsächlich getestet/ermittelt.

Beispiele:

Linux:

- `metrics`
- `inventory`
- `terminal`
- `files_read`
- `files_write`
- `systemd`
- `processes`
- `ssh_tunnel`

Windows:

- `metrics`
- `inventory`
- `terminal`
- `files_read`
- `files_write`
- `windows_services`
- `processes`
- `rdp_tunnel`

Nicht vorhandene Dienste (z. B. RDP deaktiviert) können Capability vorhanden, Runtime-Status aber unavailable sein; UI muss Unterschied darstellen.

## 8a. Netzwerk-Traffic-Metriken

Anders als CPU/RAM/Disk (`metrics_report`, live pro `status_interval`
gepusht) wird Netzwerk-Traffic lokal gepuffert und im Batch hochgeladen,
siehe `internal/netmetrics` und docs/PROTOCOL.md §7a. Grund: sinnvolle
Traffic-Charts brauchen eine deutlich feinere Abtastrate, als man alle
paar Minuten über den Control-Channel schicken möchte — und ein kurzer
Verbindungsverlust soll keine Daten kosten, nur die Zustellung verzögern.

Ablauf:

1. Ein Sampler-Goroutine läuft für die gesamte Prozesslaufzeit (unabhängig
   von der aktuellen Verbindung), alle 60s (`netmetrics.SampleInterval`,
   fest — beeinflusst nur lokale Auflösung/Plattenverbrauch, nicht die
   Serverlast).
2. Pro Tick: `platform.Provider.NetworkCounters()` liefert kumulative
   Byte-Zähler seit Systemstart (gopsutil, alle Interfaces summiert);
   die Differenz zum letzten Tick ergibt den Traffic dieses Intervalls.
   Gleichzeitig wird der eigene Control-Channel-Traffic seit dem letzten
   Tick ausgelesen (Nachrichtennutzlast-Bytes, kein Rohbyte-Zählung auf
   TCP/TLS-Ebene) und zurückgesetzt.
3. Das Sample landet in einer lokalen SQLite-Datei
   (`<DataDir>/netmetrics.db`, `modernc.org/sqlite` — bewusst reines Go
   ohne cgo, damit der dokumentierte Ein-Zeilen-Cross-Compile für Windows
   und Linux unverändert funktioniert).
4. Alle `network_upload_interval_seconds` (vom Server via `hello_ack`
   vorgegeben, Standard 5min) sowie einmal sofort nach jedem
   Verbindungsaufbau werden alle noch nicht hochgeladenen Samples als
   `network_metrics_batch` gesendet und erst danach lokal gelöscht.

Ein Fehler beim Öffnen der lokalen Datenbank ist nicht fatal — der Agent
läuft normal weiter, nur ohne Netzwerk-Traffic-Historie für diesen Lauf.

## 9. OS Adapter Interface

Gemeinsame Interfaces für:

```text
InventoryProvider
MetricsProvider
NetworkCounterProvider
TerminalProvider
ServiceProvider
ProcessProvider
FileProvider
PowerProvider
TunnelProvider
UpdateProvider
```

OS-spezifische Implementierungen unter `platform/linux` und `platform/windows`.

## 10. Terminal Linux

- PTY verwenden.
- Default-Shell explizit bestimmen.
- keine Command-String-Einbettung in `sh -c`, wenn nicht benötigt.
- Terminalprozess an Remote Session binden.
- beim Sessionende Prozessgruppe sauber schließen.

## 11. Terminal Windows

- PowerShell als Default.
- ConPTY für interaktive Session sofern verfügbar.
- Prozessbaum bei Sessionende beenden.

## 12. Tunnel

Agent akzeptiert nur semantische Targettypen.

```go
ssh_local => net.Dial("tcp", "127.0.0.1:22")
rdp_local => net.Dial("tcp", "127.0.0.1:3389")
```

Keine API, die ungeprüft `host` und `port` aus Remoteinput übernimmt.

## 12a. Remote-support account

Der Tunnel (§12) reicht nur rohen Netzwerkverkehr an den bereits
existierenden SSH-/RDP-Dienst des Geräts weiter — dessen Login ist von
unserer eigenen Ed25519-Geräteidentität komplett getrennt. Ohne einen
bekannten Account bräuchte man dafür also doch wieder die Zugangsdaten des
Kunden.

Deshalb legt der Agent bei der ersten erfolgreichen Verbindung einmalig
einen dedizierten lokalen Account `remotewartung` an (nie der bestehende
root-/Administrator-Account des Kunden — dessen Passwort wird nie
verändert):

- **Linux**: `useradd`, Passwort per `chpasswd`. Sudo-Rechte werden
  best-effort über ein eigenes `/etc/sudoers.d/wartungsremote-support`
  Drop-in vergeben (nicht über Gruppenzugehörigkeit, da `sudo`/`wheel`/`adm`
  je nach Distribution unterschiedlich heißen) — falls `sudo` auf der
  Distribution gar nicht installiert ist (z.B. minimale Appliance-Systeme),
  funktioniert der SSH-Login trotzdem, nur ohne Rechteausweitung von dieser
  Shell aus. Terminal (agent-eigenes Protokoll) hat davon unabhängig
  ohnehin immer volle root-Rechte.
- **Windows**: `net user` + Mitgliedschaft in `Administrators` (Admins
  dürfen sich per RDP anmelden, keine separate "Remote Desktop Users"
  Gruppe nötig).

Das generierte Passwort wird einmalig über den Control-Channel gemeldet
(`support_credential_report`, docs/PROTOCOL.md) und dort AES-256-GCM
verschlüsselt gespeichert (`internal/support`, gleicher Schlüssel wie
TOTP-Secrets) — der Server sieht es im Klartext nur bei einem expliziten,
auditierten Dashboard-Reveal. Ein Admin mit `remote.tunnel.ssh` oder
`remote.tunnel.rdp` kann das Passwort jederzeit über das Dashboard rotieren
(`support_credential.rotate` Command); der Agent generiert dann ein neues
Passwort, setzt es lokal und meldet es erneut.

Provisionierung passiert genau einmal pro Installation (lokale
Marker-Datei im DataDir), nicht bei jedem Reconnect — ein fehlgeschlagener
Report wird beim nächsten Reconnect automatisch wiederholt.

## 13. Dateioperationen

Agent definiert erlaubte Roots/Policy. V1 kann systemweiten Zugriff für privilegierte Adminwartung erlauben, muss aber Pfad-Escapes verhindern.

Operationen:

- list
- stat
- download
- upload
- mkdir
- rename
- delete

Jede schreibende Operation erzeugt Ergebnis + Audit-Referenz.

## 14. Prozesse und Dienste

OS-native APIs bevorzugen.

Kein Aufbau von Befehlen wie:

```text
systemctl restart <untrusted-string>
```

über Shell-Konkatenation.

## 15. Update

Agent:

1. Manifest abrufen.
2. Release-Key-Signatur prüfen.
3. kompatible OS/Arch prüfen.
4. Paket downloaden.
5. Hash + Signatur prüfen.
6. Staging.
7. alte Version sichern.
8. atomarer Wechsel soweit Plattform erlaubt.
9. Neustart.
10. Health Signal.
11. Rollback bei Fehler.

## 16. Lokales Logging

Strukturiert, rotierend.

Loggt:

- Start/Stop.
- Connect/Disconnect ohne Credentials.
- Enrollment-Status.
- Session Start/Ende.
- Fehlercodes.
- Update.

Loggt niemals:

- Token-Klartext.
- Private Keys.
- Adminpasswörter.
- TOTP.
- komplette Terminaleingaben standardmäßig.

## 17. Deinstallation

Deinstallieren:

- Service stoppen/entfernen.
- Binary entfernen.
- Konfiguration entfernen nach Userwahl.
- Credentials sicher löschen soweit OS/Dateisystem praktikabel.

Servergerät bleibt zunächst als `offline`/`uninstalled suspected`; Admin kann es revoken/archivieren.

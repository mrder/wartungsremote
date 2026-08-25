# WartungsRemote – Verbindliche Produktspezifikation V1

Status: **Normativ für V1**  
Stand: 2026-08-23

Die Begriffe **MUSS**, **DARF NICHT**, **SOLL**, **SOLL NICHT** und **KANN** sind verbindlich zu verstehen.

## 1. Ziel

WartungsRemote ist eine transparente Remote-Wartungsplattform für autorisierte Windows- und Linux-Geräte. Kundengeräte bauen ausschließlich ausgehende Verbindungen zum öffentlichen Gateway auf. Eingehende Router-Portfreigaben sind für WartungsRemote nicht erforderlich.

## 2. V1-Komponenten

1. `wr-agent` – Windows-/Linux-Systemdienst.
2. `wr-gateway` – öffentlicher TLS-Endpunkt für Enrollment und Control Channel.
3. `wr-core` – Authentifizierung, Geräteverwaltung, Sessions, Wartungshistorie, Audit, Policies.
4. `wr-relay` – autorisierter Stream-Relay für Terminal- und TCP-Tunnel.
5. `wr-web` – Admin-Dashboard; standardmäßig nur LAN/VPN.
6. PostgreSQL – persistente Metadaten.
7. Optional `wr-helper` – lokaler Admin-PC-Helper für SSH-/RDP-Tunnel.

## 3. V1-Funktionsumfang

V1 MUSS enthalten:

- Windows 10/11 und Windows Server 2019+ x86_64.
- Debian/Ubuntu/Raspberry Pi OS x86_64/arm64; Unraid soweit technisch kompatibel.
- One-Time Enrollment.
- eindeutige Geräteidentität mit lokalem privaten Schlüssel.
- TLS-gesicherter Control Channel.
- Heartbeat, Online-/Offline-Status.
- Systeminventar und Live-Metriken.
- zentrale Geräte-, Kunden- und Gruppenverwaltung.
- Admin-Login mit Passwort + TOTP.
- serverseitige Sessions und RBAC.
- temporäre Privilege Sessions.
- Remote-Terminal.
- SSH-Tunnel zu `127.0.0.1:22` bei Linux, wenn freigegeben.
- RDP-Tunnel zu `127.0.0.1:3389` bei Windows, wenn freigegeben.
- Dateiübertragung mit Policy-Prüfung.
- Dienste und Prozesse anzeigen; schreibende Aktionen nur mit Berechtigung.
- Monitoring, Zustandsbewertung, Wartungshistorie und Audit.
- signierte Agent-Updates mit Rollback.
- integrierte Admin-Hilfe.

## 4. Bewusste Nicht-Ziele V1

V1 DARF NICHT voraussetzen, dass Port 22 oder 3389 aus dem Internet erreichbar ist.

V1 implementiert **keinen eigenen grafischen Remote-Desktop-Codec**. Windows-RDP wird zunächst getunnelt. Ein eigener Desktop-Stream ist V2+.

V1 DARF KEINE Funktionen für Keylogging, Credential Dumping, versteckte Kamera-/Mikrofonnutzung, Browserdatenextraktion, Umgehung lokaler Sicherheitsprodukte oder verschleierte Persistenz enthalten.

V1 DARF KEINEN generischen, frei adressierbaren Netzwerk-Proxy bereitstellen. Tunnelziele sind capability- und policygebunden.

## 5. Port- und Netzwerkmodell

### Kundengerät

Standardmäßig erforderlich:

- TCP 443 ausgehend zum Gateway/Relay.
- DNS und normale Internetkonnektivität.

Keine eingehende Firewallregel ist für WartungsRemote notwendig.

### Server

Empfohlene Trennung:

- `443/tcp`: öffentlich – Gateway/Control/Relay.
- `9443/tcp`: Admin-Web – nur LAN/VPN.
- `5432/tcp`: PostgreSQL – nur internes Docker-/Servernetz.

Optional kann Relay intern separat betrieben werden. Öffentlich soll möglichst nur 443 sichtbar sein.

## 6. SSH/RDP-Koexistenz

WartungsRemote verändert standardmäßig weder `sshd` noch RDP-Konfiguration noch deren Ports.

- SSH bleibt lokal auf Port 22 verwendbar.
- RDP bleibt lokal auf Port 3389 verwendbar.
- vorhandene externe Freigaben bleiben Sache des Administrators.
- WartungsRemote nutzt einen eigenen Relay-Stream.

Beispiel Admin-PC:

```text
127.0.0.1:41022 -> Relay -> Agent -> 127.0.0.1:22
127.0.0.1:43389 -> Relay -> Agent -> 127.0.0.1:3389
```

Lokale Admin-PC-Ports sollen dynamisch aus einem konfigurierbaren Bereich gewählt werden; feste Beispielports sind nicht normativ.

## 7. Geräteidentität

Jeder Agent erzeugt beim Enrollment lokal:

- `device_id`: UUIDv4 oder UUIDv7 vom Server.
- asymmetrisches Geräteschlüsselpaar.
- `install_id`: zufällige ID für genau diese Installation.

Der private Geräteschlüssel MUSS lokal geschützt gespeichert werden und DARF den Agent niemals verlassen.

Server speichert mindestens:

- Device ID.
- Install ID.
- Public Key / Zertifikatsfingerprint.
- Credential-Status.
- Credential-Erstellungs- und Ablaufdatum.

Klonen eines Agents auf mehrere Systeme MUSS erkannt bzw. verhindert werden.

## 8. Enrollment

Enrollment-Token:

- mindestens 256 Bit CSPRNG-Entropie.
- nur einmal verwendbar.
- Default-TTL 30 Minuten.
- serverseitig nur als Hash speichern.
- optional Kunde, Gruppe, Tags und vorgesehener Anzeigename vorbinden.
- nach Erfolg sofort `consumed`.

Ablauf:

1. Admin erzeugt Enrollment.
2. Agent prüft TLS-Serveridentität.
3. Agent erzeugt Keypair lokal.
4. Agent sendet Token, Public Key, Install ID und Basisinventar.
5. Server prüft Token atomar und sperrt Mehrfachverwendung.
6. Server erstellt Gerät und Credential.
7. Agent speichert Credential sicher.
8. Agent baut neuen authentifizierten Control Channel auf.
9. Server markiert Enrollment abgeschlossen.

Ein Enrollment darf nicht gleichzeitig zweimal erfolgreich abgeschlossen werden.

## 9. Control Channel

MVP: WebSocket über HTTPS/TLS.

Anforderungen:

- nur authentifizierte Geräte nach Enrollment.
- eine aktive Primary-Control-Session pro Install ID; Doppelverbindungen werden kontrolliert behandelt.
- Ping/Pong und Application Heartbeat.
- Nachrichtengrößenlimit.
- Protokollversionsprüfung.
- monotone Connection Sequence pro Reconnect optional.
- Backpressure: Server und Agent müssen überfüllte Queues begrenzen.

Heartbeat Default: 45 Sekunden.  
Heartbeat Jitter: ±10 %.  
`connection_lost`: nach 120 Sekunden ohne gültigen Heartbeat.  
`offline`: nach 300 Sekunden.

## 10. Status und Inventar

### Basisinventar

Beim Connect und bei Änderung:

- Hostname.
- OS-Familie, Edition/Distribution, Version.
- Kernel/Build.
- Architektur.
- CPU-Modell, Kerne, Threads.
- RAM gesamt.
- Datenträger/Mountpoints.
- Netzwerkinterfaces und lokale IPs.
- Agent-Version.
- Bootzeit/Uptime.
- Capabilities.

### Live-Metriken

Default alle 5 Minuten:

- CPU %.
- RAM belegt/gesamt/%.
- Datenträger belegt/gesamt/%.
- Netzwerk RX/TX optional aggregiert.
- Uptime.

Public-IP wird bevorzugt serverseitig aus der authentifizierten Verbindung erfasst.

## 11. Zustandsbewertung

Jedes Gerät erhält:

- `healthy`
- `warning`
- `critical`
- `offline`
- `unknown`

V1 Default-Regeln:

- Disk >= 90 %: warning.
- Disk >= 97 %: critical.
- RAM >= 90 % für >= 10 min: warning.
- CPU >= 95 % für >= 10 min: warning.
- Agent nicht unterstützte Version: warning/critical abhängig von Policy.
- Gerät offline: offline.
- von OS erkannter notwendiger Neustart: warning.

Schwellwerte sind serverseitig konfigurierbar.

## 12. Admin-Authentifizierung

- keine offenen Registrierungen.
- erster Superadmin wird explizit per Setup/CLI erstellt.
- Passwortspeicherung mit Argon2id.
- individuelles Salt.
- optional Pepper außerhalb DB.
- TOTP für Admins verpflichtend in Produktion.
- Recovery Codes einmalig anzeigen und nur gehasht speichern.
- serverseitige Sessions, keine Auth-Tokens in `localStorage`.

Session-Cookie:

```text
__Host-wr_session=<opaque>; Secure; HttpOnly; SameSite=Strict; Path=/
```

Default:

- absolute Session-Lebensdauer: 8 h.
- Idle Timeout: 30 min.
- Privilege Session: 15 min.

## 13. Rollen und Rechte

Mindestrollen:

- `super_admin`
- `admin`
- `technician`
- `read_only`

Rechte werden granular als Permissions implementiert, nicht ausschließlich als Rollennamen.

Beispiele:

- `device.read`
- `device.manage`
- `remote.terminal`
- `remote.tunnel.ssh`
- `remote.tunnel.rdp`
- `remote.files.read`
- `remote.files.write`
- `remote.service.control`
- `remote.process.terminate`
- `remote.power`
- `privilege.request`
- `audit.read`
- `user.manage`

Default-Deny MUSS serverseitig gelten.

## 14. Temporäre Rechteerhöhung

Ein Remote-Zugriff beginnt mit den minimal vorgesehenen Rechten.

Für privilegierte Funktionen:

1. Nutzer fordert Privilege Session an.
2. Server fordert Reauthentication.
3. Passwort und TOTP werden erneut geprüft.
4. Privilege Session wird für Gerät + Admin + Wartungssession gebunden.
5. Default 15 Minuten.
6. Dashboard zeigt Countdown.
7. Ablauf entzieht erhöhte Rechte automatisch.
8. manuelles Entziehen ist jederzeit möglich.
9. Disconnect des Admins beendet bzw. invalidiert sie gemäß Policy.

Privilege Session darf nicht als dauerhaftes globales Adminrecht dienen.

## 15. Remote-Terminal

Terminal-Sessions:

- sind explizite Remote Sessions.
- besitzen Session ID, Owner, Device ID, Start, Ablauf, Privilege State.
- Linux nutzt PTY und konfigurierten Shellpfad.
- Windows nutzt PowerShell/ConPTY sofern verfügbar.
- Resize wird als eigene Control-Nachricht übertragen.
- Input/Output wird gestreamt.
- Standard-Idle-Timeout 30 Minuten.
- max. Dauer konfigurierbar.

Befehlsinhalte werden standardmäßig **nicht vollständig als Audit-Text gespeichert**, um Secrets nicht versehentlich zu protokollieren. Audit speichert Start/Ende und administrative Metadaten. Optionaler Command-Logging-Modus muss ausdrücklich aktiviert werden.

## 16. TCP-Tunnel

V1 zulässige Targets:

- Linux SSH: `tcp://127.0.0.1:22`.
- Windows RDP: `tcp://127.0.0.1:3389`.

Zusätzliche Targets dürfen später nur als explizite serverseitige Policy/Capability hinzugefügt werden.

Der Server stellt dem Admin-Browser nicht direkt Raw-TCP bereit. Für native SSH-/RDP-Clients wird `wr-helper` empfohlen:

1. Admin authentifiziert sich.
2. Dashboard beantragt Tunnel.
3. Core erzeugt kurzlebiges, single-purpose Tunnel-Ticket.
4. Helper erhält Ticket über sicheren Deep-Link/Copy-Flow.
5. Helper bindet ausschließlich `127.0.0.1:<ephemeral>`.
6. Helper verbindet zum Relay.
7. Relay prüft Ticket, Admin, Gerät, Zieltyp und Ablauf.
8. Agent öffnet Verbindung nur zum zulässigen lokalen Target.
9. Stream wird transparent übertragen.
10. Ticket wird nach Nutzung invalidiert.

## 17. Dateiübertragung

- Pfadnormalisierung vor jeder Operation.
- Schutz gegen `..`, Symlink-/Junction-Escape und unzulässige Root-Wechsel.
- serverseitige Permission + agentseitige Policy.
- max. Datei- und Transfergröße konfigurierbar.
- Upload zunächst in temporäre Datei, danach Hashprüfung und atomare Umbenennung soweit möglich.
- Schreib-/Löschoperationen erfordern erhöhte Permission; sensible Systempfade optional Privilege Session.

## 18. Services und Prozesse

Lesen kann `technician`/`read_only` je nach Policy erhalten.

Schreibende Aktionen:

- Dienst starten/stoppen/restart.
- Prozess terminieren.
- Neustart/Shutdown.

müssen serverseitig autorisiert und vollständig auditiert werden. Kritische Power-Aktionen erfordern explizite Bestätigung im UI.

## 19. Wartungssitzung

Remote-Aktionen werden einer `maintenance_session` zugeordnet.

Automatischer Start bei erster Remote-Aktion, optional manueller Start.

Speichern:

- Admin/Techniker.
- Gerät.
- Kunde.
- Start/Ende.
- verwendete Remote-Funktionen.
- Ereignisse.
- manuelle Notiz.
- Ergebnis.
- optional nächste Wartung.

## 20. Audit

Audit MUSS append-orientiert sein.

Mindestens:

- Auth-Events.
- Enrollment.
- Geräteänderungen.
- Remote-Session Start/Ende.
- Privilege Start/Ende.
- Tunnel Start/Ende + Target-Typ.
- Dateiänderungen.
- Service-/Prozess-/Power-Aktionen.
- Updates.
- Benutzer-/Rollen-/Policyänderungen.

Nicht speichern:

- Klartextpasswörter.
- TOTP Codes.
- Private Keys.
- Session-Tokens.
- komplette Datei-Inhalte.

## 21. Agent-Updates

- Update-Manifest signiert.
- Paket-Hash SHA-256 oder stärker.
- Signaturprüfung mit fest eingebettetem Release-Public-Key.
- HTTPS Download.
- Installation nur nach erfolgreicher Signaturprüfung.
- vorherige funktionsfähige Version für Rollback behalten.
- neuer Prozess muss Health-Check bestehen.
- automatischer Rollback bei fehlgeschlagenem Start.

Release-Signing-Private-Key darf nicht im normalen Produktionsserver-Container liegen.

## 22. Datenschutz und Transparenz

Agent ist sichtbar installiert, dokumentiert und deinstallierbar.

Standardmäßig nicht erfassen:

- Browserhistorie.
- gespeicherte Passwörter.
- Cookies.
- WLAN-Schlüssel.
- Dokumentinhalte.
- Kamera/Mikrofon.

Datenminimierung ist Default.

## 23. Fehler- und Recovery-Verhalten

Agent:

- darf bei Serverausfall nicht abstürzen.
- reconnect mit exponentiellem Backoff + Jitter.
- max. Backoff 5 Minuten.
- Statusqueue begrenzen; veraltete Metriken zusammenfassen/verwerfen.
- Remote-Befehle niemals nach Neustart blind erneut ausführen.

Server:

- Sessions nach Neustart als interrupted markieren.
- One-Time Tickets nicht wiederverwenden.
- DB-Transaktionen für Enrollment und sicherheitskritische Statuswechsel.

## 24. Akzeptanzkriterien V1

V1 gilt technisch erst als releasefähig, wenn:

1. Agenten hinter NAT/CGNAT ohne Portfreigabe verbinden können.
2. bestehendes SSH/RDP unbeeinflusst bleibt.
3. Enrollment-Token nicht wiederverwendbar ist.
4. gestohlene Device ID allein keine Anmeldung erlaubt.
5. Admin ohne TOTP in Production Mode keinen Remote-Zugriff erhält.
6. Privilege Sessions automatisch ablaufen.
7. SSH-/RDP-Tunnel nur zum erlaubten localhost-Ziel führen.
8. Audit jeden privilegierten Schreibvorgang abbildet.
9. manipuliertes Agent-Update abgewiesen wird.
10. Agent nach Serverausfall automatisch reconnectet.
11. Linux und Windows mindestens 24h Stabilitätstest bestehen.
12. Hilfe-/Troubleshooting-Seiten im Dashboard verfügbar sind.

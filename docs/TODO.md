# WartungsRemote – TODO / Entwicklungsplan

## Phase 0 – Projektdefinition

- [ ] Produktnamen final festlegen
- [ ] Hersteller-/Firmenname festlegen
- [ ] Copyright-Text festlegen
- [ ] Lizenzmodell festlegen
- [x] Repository erstellen
- [x] README.md erstellen
- [x] SECURITY.md erstellen
- [x] CHANGELOG.md vorbereiten
- [x] docs-Verzeichnis anlegen
- [x] Threat Model erstellen
- [x] unterstützte Betriebssysteme definieren
- [x] unterstützte Architekturen definieren
- [x] Protokollversion 1 definieren
- [x] Datenbankmodell definieren
- [x] Deploymentmodell definieren

## Phase 1 – Grundlegende Security-Architektur

- [ ] TLS 1.3 erzwingen
- [ ] Serverzertifikatsprüfung definieren
- [x] Geräte-Keypair-Konzept definieren
- [ ] mTLS-Konzept definieren
- [x] Enrollment-Token-Format definieren
- [x] Token-Ablaufzeit definieren
- [x] Single-Use-Enrollments implementieren
- [ ] Replay-Schutz definieren
- [x] Nonce-Konzept definieren
- [ ] Command-ID-Konzept definieren
- [x] Session-ID-Konzept definieren
- [ ] Secret Rotation vorbereiten

## Phase 2 – Server-Grundgerüst

- [x] Go-Modul erstellen
- [x] Konfigurationssystem
- [x] strukturiertes Logging
- [x] Health Endpoint
- [x] Graceful Shutdown
- [x] PostgreSQL anbinden
- [x] Migration Framework
- [x] Device Repository
- [x] User Repository
- [ ] Customer Repository
- [x] Audit Repository
- [x] Monitoring Repository
- [ ] Remote Session Repository

## Phase 3 – Datenbank

- [x] users
- [x] user_mfa
- [x] user_sessions
- [x] customers
- [x] device_groups
- [x] devices
- [x] device_credentials
- [x] device_network
- [x] device_capabilities
- [x] device_metrics
- [x] device_events
- [x] enrollment_tokens
- [x] remote_sessions
- [x] remote_commands
- [x] privilege_sessions
- [x] audit_log
- [ ] maintenance_notes
- [x] maintenance_sessions
- [x] agent_versions
- [x] alert_rules
- [x] alerts

## Phase 4 – Admin-Authentifizierung

- [x] Benutzeranlage
- [x] Argon2id Passwort-Hashing
- [x] sicherer Login
- [x] Logout
- [x] sichere Session-Cookies
- [x] HttpOnly
- [x] Secure
- [x] SameSite
- [x] Session Timeout
- [x] Idle Timeout
- [x] Rate Limiting
- [x] Login Lockout
- [x] Login Audit
- [x] TOTP-Einrichtung
- [x] TOTP-Prüfung
- [x] Recovery Codes
- [x] Reauthentication für kritische Aktionen
- [x] Rollenmodell
- [x] Rechteprüfung

## Phase 5 – Agent-Grundsystem

- [x] Go Agent-Projekt
- [x] zentrale Konfiguration
- [x] Logging
- [x] Linux Service Integration
- [x] Windows Service Integration
- [ ] Machine ID
- [x] eindeutige Device ID
- [x] OS-Erkennung Linux
- [x] OS-Erkennung Windows
- [x] Architektur erkennen
- [x] Agent-Version
- [x] Bootzeit
- [x] Uptime
- [x] Graceful Shutdown
- [x] Reconnect Logic
- [x] Exponential Backoff
- [x] lokale sichere Credential-Ablage

## Phase 6 – Enrollment

- [x] Dashboard „Gerät hinzufügen“
- [x] Enrollment Token erzeugen
- [x] Token Ablaufzeit
- [x] Single Use
- [ ] Kundenzuordnung
- [ ] Gruppenzuordnung
- [x] Agent Keypair erzeugen
- [x] Public Key übertragen
- [x] Server validiert Enrollment
- [x] Device ID erstellen
- [x] Device Credential ausstellen
- [x] Token invalidieren
- [x] Enrollment Audit
- [x] Fehlversuche protokollieren

## Phase 7 – Control Channel

- [ ] WebSocket über TLS
- [x] authentifizierter Agent-Connect
- [x] Protocol Version Handshake
- [x] Capability Handshake
- [x] Heartbeat
- [ ] Server Ping/Pong
- [x] Reconnect
- [x] Session IDs
- [x] Message IDs
- [x] Request IDs
- [x] Timeouts
- [x] Größenlimits
- [x] ungültige Nachrichten ablehnen
- [x] Protokollfehler auditieren

## Phase 8 – Systeminventar

- [x] Hostname
- [x] OS
- [x] OS Version
- [x] Kernel
- [x] Architektur
- [x] CPU Modell
- [x] CPU Kerne
- [x] CPU Threads
- [x] RAM gesamt
- [x] Datenträger
- [x] Mountpoints
- [x] Dateisysteme
- [x] Netzwerkinterfaces
- [x] lokale IPv4
- [x] lokale IPv6
- [ ] Default Gateway
- [ ] DNS Server
- [x] Public IP serverseitig erkennen
- [x] Agent-Version

## Phase 9 – Live Status

- [x] CPU-Auslastung
- [x] RAM-Auslastung
- [x] Datenträgerauslastung
- [x] Uptime
- [ ] Netzwerkstatus
- [x] Online
- [x] Connection Lost
- [x] Offline
- [x] Status Request Button
- [x] sofortige Statusabfrage
- [ ] Round Trip Time
- [ ] Verbindungstest

## Phase 10 – Zustandsbewertung

- [x] Statusengine
- [x] Datenträger-Warnung
- [x] CPU-Warnung
- [x] RAM-Warnung
- [x] Agent-Version-Warnung
- [x] Offline-Warnung
- [ ] Neustart-erforderlich-Erkennung
- [ ] Dienst-Warnung
- [ ] SMART-Warnung später
- [ ] Backup-Alter später
- [x] Ampelstatus pro Gerät
- [x] Warnungsdetails

## Phase 11 – Web-Dashboard

- [x] Dashboard Layout
- [x] Online Count
- [x] Offline Count
- [x] Warning Count
- [x] Critical Count
- [x] Geräteliste
- [x] Suche
- [ ] Filter
- [ ] Sortierung
- [ ] Kundenansicht
- [ ] Gruppenansicht
- [ ] Tags
- [x] Gerätedetailseite
- [ ] Hardware Tab
- [ ] Netzwerk Tab
- [x] Monitoring Tab
- [x] Remote Tab
- [x] Dateien Tab
- [x] Dienste Tab
- [x] Prozesse Tab
- [ ] Logs Tab
- [x] Audit Tab
- [x] Wartungshistorie Tab
- [ ] Hilfe / Anleitung Tab

## Phase 12 – Kundenverwaltung

- [x] Kunden anlegen
- [ ] Kunden bearbeiten
- [ ] Kunden deaktivieren
- [ ] Ansprechpartner
- [x] Kundennummer
- [ ] Standorte
- [x] Geräte zuordnen
- [ ] Gruppen
- [ ] Tags
- [ ] interne Notizen

## Phase 13 – Wartungshistorie

- [x] Wartungssession automatisch starten
- [x] Wartungssession automatisch beenden
- [x] Techniker speichern
- [x] Startzeit
- [x] Endzeit
- [ ] ausgeführte Aktionen
- [ ] manuelle Notizen
- [x] Wartungsergebnis
- [ ] nächste Wartung optional
- [ ] Export später

## Phase 14 – Temporäre Rechteerhöhung

- [x] Standard-Remote-Session unprivilegiert
- [x] „Adminrechte anfordern“
- [x] erneute Passwortprüfung
- [x] erneute TOTP-Prüfung
- [x] Privilege Session erzeugen
- [x] Standard 15 Minuten
- [x] konfigurierbare Dauer
- [x] Countdown anzeigen
- [x] automatische Entziehung
- [x] manuelle Entziehung
- [x] Audit Start
- [x] Audit Ende
- [ ] erneute Bestätigung bei Ablauf

## Phase 15 – Remote Terminal

- [x] Session Broker
- [x] Linux PTY
- [x] Bash
- [x] Windows PowerShell
- [ ] optional CMD
- [x] Web Terminal
- [x] UTF-8
- [x] Terminal Resize
- [x] Session Timeout
- [x] Session Ende
- [x] Berechtigungsprüfung
- [x] Privilege Elevation anbinden
- [x] Audit

## Phase 16 – Relay

- [x] Relay Session IDs
- [x] Stream IDs
- [x] Multiplexing
- [x] Server Authorization
- [x] Agent Authorization
- [ ] temporäre Zugriffstokens
- [x] bidirektionale Streams
- [ ] Bandbreitenlimit
- [ ] Idle Timeout
- [x] Session Timeout
- [x] Abbruch bei Agent Disconnect
- [x] Audit

## Phase 17 – SSH-Tunnel

- [x] lokale Tunnel-App oder Helper definieren (wr-helper, natives Binary)
- [x] temporären lokalen Port reservieren (loopback-only, --port 0 = ephemeral)
- [x] Relay Stream öffnen
- [x] Agent zu 127.0.0.1:22 verbinden
- [x] Daten bidirektional übertragen
- [x] Tunnel schließen
- [x] Timeout
- [x] Fehleranzeige
- [x] Audit
- [x] bestehendes SSH auf Port 22 unangetastet lassen

## Phase 18 – RDP-Tunnel

- [x] temporären lokalen Port reservieren
- [x] Relay Stream öffnen
- [x] Agent zu 127.0.0.1:3389 verbinden
- [ ] Windows mstsc testen (Mechanik mit TCP-Testserver verifiziert, kein echtes RDP-Ziel in dieser Umgebung verfügbar)
- [x] Tunnel schließen
- [x] Timeout
- [x] Fehleranzeige
- [x] Audit
- [x] bestehendes RDP auf Port 3389 unangetastet lassen

## Phase 19 – Dateiübertragung

- [x] Verzeichnisliste
- [x] Pfadnavigation
- [x] Upload
- [x] Download
- [ ] Fortschritt
- [ ] Hashprüfung
- [x] maximale Größe
- [ ] Abbrechen
- [x] Umbenennen
- [x] Ordner anlegen
- [x] Löschen
- [x] Rechteprüfung
- [x] Directory-Traversal-Schutz
- [x] Audit

## Phase 20 – Dienstverwaltung

- [x] Linux systemd Listing
- [x] Linux Start
- [x] Linux Stop
- [x] Linux Restart
- [x] Windows Service Listing
- [x] Windows Start
- [x] Windows Stop
- [x] Windows Restart
- [x] Rechteprüfung
- [ ] temporäre Adminrechte
- [x] Audit

## Phase 21 – Prozessverwaltung

- [x] Prozessliste
- [x] PID
- [x] Name
- [x] CPU
- [x] RAM
- [x] Benutzer
- [x] Startzeit
- [ ] Prozessdetails
- [x] Prozess beenden
- [ ] kritische Prozesswarnung
- [x] Rechteprüfung
- [x] Audit

## Phase 22 – Logs

- [x] Linux journalctl API
- [x] Windows Event Log API (wevtutil, Kanal "System")
- [x] Filter
- [x] Suchbegriff
- [x] Zeitraum
- [x] Level
- [x] Limit
- [ ] Export
- [ ] Audit bei sensiblen Zugriffen

## Phase 23 – Monitoring Historie

- [x] Metrics-Speicherung
- [x] CPU History
- [x] RAM History
- [ ] Disk History (nur aktueller Snapshot, keine Zeitreihe)
- [ ] Network History (nur aktueller Snapshot, keine Zeitreihe)
- [ ] Temperatur optional
- [x] Downsampling (stündliche Aggregation, `device_metrics_hourly`)
- [x] Retention Policies (konfigurierbar über `metrics.raw_retention`/`metrics.hourly_retention`)
- [x] Charts (CPU/RAM, SVG, raw + hourly)
- [x] Aggregation

## Phase 24 – Alarmierung

- [x] Regelengine (`internal/alerting`, 1-Minuten-Sweep, global/customer/group/device Scope)
- [x] Offline Regel
- [x] CPU Regel
- [x] RAM Regel
- [x] Disk Regel
- [x] Dienst Regel (nutzt denselben `services.list`-Command wie der Services-Tab; Live-Test des positiven Auslösefalls erfordert einen Agent mit SCM-Zugriff, d.h. als installierter Windows-Dienst statt als unprivilegierter Vordergrundprozess — in dieser Dev-Session nicht möglich, siehe CHANGELOG)
- [x] Agent Version Regel
- [x] Alarm Acknowledge
- [x] Alarm History (Zustände open/acknowledged/resolved, vollständig auditiert)
- [ ] E-Mail später
- [ ] ntfy später
- [ ] Telegram später
- [ ] Webhooks später
- [ ] ioBroker später

## Phase 25 – Agent Update

- [x] Update Manifest (`agent_versions`, `internal/agentrelease`, GET/POST `/agent/releases`)
- [x] Release Signing (offline, `cmd/wr-release-sign`; server/agent never hold the private key)
- [x] Public Signing Key im Agent (`-ldflags -X internal/agentcore.ReleasePublicKeyHex=...`; empty key fails closed)
- [x] Download
- [x] SHA-256 Hashprüfung
- [x] Signaturprüfung (server-side at manifest submission AND independently agent-side before staging)
- [x] Installationsablauf Windows (live-verified: real download, verify, atomic swap-while-running, restart, commit)
- [x] Installationsablauf Linux (platform-generic Go/`os.Rename`, no OS-specific code path; cross-compiled, not live-tested on real Linux hardware in this session)
- [x] Agent Restart
- [x] Health Check ("Health Signal" = first successful reconnect commits the update)
- [x] Rollback (crash-loop boot-attempt counter + binary restore; primitives live-verified, the crash-loop trigger itself only unit-tested — see CHANGELOG)
- [x] Update Status im Dashboard (Releases page, "Check for update" on device detail)
- [x] Update Audit (`agent_release.created`, `agent.update.triggered`)

## Phase 26 – Installer

### Windows

- [ ] MSI oder EXE Installer
- [ ] Produktname
- [ ] Hersteller
- [ ] Version
- [ ] Copyright
- [ ] Server URL
- [ ] Enrollment Token
- [ ] Windows Service installieren
- [ ] Uninstaller
- [ ] Code Signing vorbereiten

### Linux

- [ ] DEB Paket
- [ ] Installer Script
- [ ] systemd Unit
- [ ] Dienstbenutzer
- [ ] Config-Verzeichnis
- [ ] State-Verzeichnis
- [ ] Log-Verzeichnis
- [ ] Uninstaller

## Phase 27 – Dashboard Hilfe / Anleitung

- [ ] Hilfe-Bereich im Dashboard
- [ ] Systemübersicht
- [ ] Gerät hinzufügen
- [ ] Enrollment erklären
- [ ] Online/Offline erklären
- [ ] Statusfarben erklären
- [ ] Verbindungstest erklären
- [ ] Terminal erklären
- [ ] Privilege Elevation erklären
- [ ] SSH-Tunnel erklären
- [ ] RDP-Tunnel erklären
- [ ] Dateiübertragung erklären
- [ ] Dienste erklären
- [ ] Prozesse erklären
- [ ] Wartungshistorie erklären
- [ ] Agent Update erklären
- [ ] häufige Fehler
- [ ] Troubleshooting Flow
- [ ] Sicherheitsinformationen

## Phase 28 – Security Tests

- [ ] TLS-Konfiguration prüfen
- [ ] Zertifikatsvalidierung prüfen
- [ ] Enrollment Token Theft testen
- [ ] Replay Attack testen
- [ ] Device Spoofing testen
- [ ] Session Hijacking testen
- [ ] CSRF testen
- [ ] XSS testen
- [ ] SQL Injection testen
- [ ] Directory Traversal testen
- [ ] unsicheren Upload testen
- [ ] Rechteeskalation testen
- [ ] Rate Limit testen
- [ ] Brute Force testen
- [ ] Agent Update Manipulation testen
- [ ] Dependency Scan
- [ ] Secret Scan
- [ ] Static Analysis

## Phase 29 – Backup und Recovery

- [ ] PostgreSQL Backup
- [ ] Config Backup
- [ ] Zertifikatsbackup
- [ ] CA Backup falls vorhanden
- [ ] Audit Backup
- [ ] Restore-Test
- [ ] Disaster-Recovery-Dokumentation

## Phase 30 – MVP Release

MVP muss enthalten:

- [ ] Linux Agent
- [ ] Windows Agent
- [ ] Enrollment
- [ ] Geräteauthentifizierung
- [ ] TLS
- [ ] Heartbeat
- [ ] Online/Offline
- [ ] Systemstatus
- [ ] Geräteübersicht
- [ ] Admin Login
- [ ] TOTP
- [ ] Statusabfrage
- [ ] Zustandsbewertung
- [ ] Wartungshistorie
- [ ] temporäre Rechteerhöhung
- [ ] Remote Terminal
- [ ] SSH Tunnel
- [ ] RDP Tunnel
- [ ] Audit Log
- [ ] signierte Agent Updates
- [ ] Dashboard Hilfe

## Phase 31 – Version 1.0

Zusätzlich:

- [ ] Dateiübertragung
- [ ] Dienste
- [ ] Prozesse
- [ ] Logs
- [ ] Monitoring History
- [ ] Alarmierung
- [ ] Kundenverwaltung
- [ ] Rollen
- [ ] Tags

## Phase 32 – Version 2.0

Optional:

- [ ] eigener Remote Desktop
- [ ] Multi Monitor
- [ ] Clipboard
- [ ] Benutzer-Consent Overlay
- [ ] Patch Management
- [ ] Softwareinventar
- [ ] automatische Backup-Prüfung
- [ ] Wartungsberichte
- [ ] Wartungsplanung
- [ ] Passkeys / WebAuthn


---

# Ergänzende verbindliche Engineering-Arbeitspakete

## Phase 33 – Protocol Contracts

- [x] JSON Envelope als gemeinsame Go-Typen implementieren
- [x] Protocol-Version validieren
- [x] Message-ID-Generator
- [x] Request/Response-Korrelation
- [x] zentrale Error Codes
- [x] Größenlimits
- [x] unbekannte Message Types sauber ablehnen
- [x] Protocol Contract Tests
- [ ] Fuzz Tests für Decoder

## Phase 34 – Tunnel Helper

- [ ] `wr-helper` Go Binary
- [ ] Login/Session-Übergabe sicher definieren
- [ ] One-Time Tunnel Ticket
- [ ] ausschließlich Loopback-Bind
- [ ] dynamischen lokalen Port wählen
- [ ] SSH Native Client Flow
- [ ] RDP Native Client Flow
- [ ] Ticket-Timeout
- [ ] Ticket Replay Test
- [ ] sauberes Schließen bei Relay-/Agent-Disconnect
- [ ] Installer/Update für Helper

## Phase 35 – Production Hardening

- [x] Production Mode implementieren
- [x] Start verweigern bei unsicherer TLS-/Cookie-Konfiguration
- [x] Security Headers
- [x] CSRF Tokens
- [x] CSP
- [x] HSTS
- [x] strukturierte Rate Limits (Login/MFA/Reauth/Enrollment bereits vorhanden; ergänzt um Control-Channel-Handshake und Tunnel-Ticket-Redemption — die verbleibenden öffentlichen, unauthentifizierten Endpunkte)
- [x] Secret Files
- [x] keine Debug-Endpoints öffentlich
- [x] sichere Default-Timeouts
- [x] DB Runtime User ohne Superuserrechte (`scripts/create-db-runtime-role.sql` + docs/DEPLOYMENT.md §5a; SQL-Syntax verifiziert, vollständige Ausführung erfordert echten Postgres-Superuser, siehe CHANGELOG)

## Phase 36 – Audit Integrity & Export

- [x] Append-only Anwendungsmodell
- [x] Audit Hashchain (`internal/audit.Logger.Record`/`VerifyChain`, `POST /audit/verify`, live-verifiziert gegen echte DB mit Bestandsdaten)
- [x] Audit Export JSON/CSV (`GET /audit/export?format=json|csv`, live-verifiziert inkl. Filter)
- [x] Filter nach User/Device/Event/Zeitraum
- [x] keine normale Delete API
- [x] Secrets Redaction Tests (`internal/auth/redaction_test.go`, `internal/config/redaction_test.go`)

## Phase 37 – Recovery / Revocation

- [x] Device Credential Revocation
- [x] User All-Sessions-Revoke (`POST /users/:id/revoke-sessions`, also triggered automatically on disable/lock)
- [x] Agent-Version blockieren (enforced at the control-channel handshake, live-verified against the real running agent)
- [x] alle offenen Enrollment-Tokens widerrufen (`POST /enrollments/revoke-all`)
- [ ] Notfallmodus dokumentieren
- [x] abgebrochene Sessions nach Serverrestart markieren (`InterruptAllActive`, live-verified)
- [ ] Restore-Test aus Backup

## Phase 38 – Dashboard Help Integration

- [x] `DASHBOARD_HELP.md` als versionierte Quelle einbinden (`internal/help`, `GET /help/index`, `GET /help/:slug`)
- [x] Markdown sicher rendern (kleiner, geschlossener Parser für exakt das in DASHBOARD_HELP.md verwendete Subset)
- [x] HTML sanitizen (Allowlist-Erzeugung statt nachträglichem Strippen; jeder Textlauf wird vor jedem Tag-Wrapping escaped — unit-getestet gegen eingebettetes `<script>`/`onerror`)
- [x] Hilfe-Suche (Client-seitiger Titel-Filter im Help-Panel)
- [ ] Kontextlinks von Fehlermeldungen zu Help Slugs
- [x] Serverstatus-Hilfeseite (Abschnitt "Serverstatus prüfen")
- [x] Enrollment-Hilfeseite (Abschnitt "Enrollment schlägt fehl")
- [x] Tunnel-Hilfeseite (Abschnitte "SSH verwenden"/"RDP verwenden"/"SSH/RDP funktioniert ... nicht")
- [x] Agent-Offline-Hilfeseite (Abschnitt "Gerät ist offline")

## Phase 39 – Release Gate V1

- [ ] alle Akzeptanzkriterien aus SPECIFICATION.md erfüllt
- [ ] TEST_PLAN.md vollständig durchgeführt
- [ ] Linux 24h Soak Test bestanden
- [ ] Windows 24h Soak Test bestanden
- [ ] NAT/CGNAT Test bestanden
- [ ] Security Scan ohne bekannte High/Critical Findings
- [ ] manipuliertes Update wird abgewiesen
- [ ] Privilege Timeout verifiziert
- [ ] SSH Port 22 unverändert verifiziert
- [ ] RDP Port 3389 unverändert verifiziert
- [ ] Backup + Restore verifiziert
- [ ] ADMIN_GUIDE und Dashboard-Hilfe aktuell
- [ ] Version 1.0.0 taggen

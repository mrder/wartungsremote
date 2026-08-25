# WartungsRemote – Projektkonzept

## 1. Projektziel

WartungsRemote ist eine offizielle, transparente Remote-Wartungsplattform für Windows- und Linux-Systeme.

Die Plattform soll Administratoren ermöglichen, entfernte Kundengeräte sicher zu überwachen und bei Bedarf zu warten, ohne dass auf Kundenseite eingehende Ports im Router freigegeben werden müssen.

Das System besteht aus:

- WartungsRemote Agent auf Windows- und Linux-Geräten
- zentralem WartungsRemote Gateway/Relay
- WartungsRemote Core Server
- PostgreSQL-Datenbank
- lokal oder per VPN erreichbarem Admin-Webinterface
- Monitoring- und Wartungshistorie
- Remote-Terminal
- optionalen SSH- und RDP-Tunneln
- Datei-, Prozess- und Dienstverwaltung
- sicherem Agent-Update-System

## 2. Grundprinzip der Verbindung

Die Kundengeräte bauen selbst eine ausgehende verschlüsselte Verbindung zum WartungsRemote-Server auf.

```text
Kundengerät
    |
    | TLS 1.3 / ausgehend
    v
WartungsRemote Gateway / Relay
    |
    v
WartungsRemote Core Server
    |
    v
Admin-Webinterface
```

Dadurch sind normalerweise keine Router-Portfreigaben auf Kundenseite erforderlich.

Auch bei:

- NAT
- CGNAT
- LTE/5G-Routern
- privaten IPv4-Adressen
- wechselnden WAN-IP-Adressen

kann der Agent erreichbar bleiben, solange er selbst eine ausgehende Internetverbindung aufbauen kann.

## 3. Keine Beeinträchtigung vorhandener SSH- und RDP-Dienste

WartungsRemote ersetzt vorhandenes SSH oder RDP nicht zwingend.

Vorhandene Dienste dürfen weiterhin normal genutzt werden:

```text
SSH: 22
RDP: 3389
```

WartungsRemote verwendet dagegen einen eigenen Relay-Kanal.

Beispiel SSH-Tunnel:

```text
Admin-PC 127.0.0.1:10022
        |
        v
WartungsRemote Relay
        |
        v
Kunden-Agent
        |
        v
127.0.0.1:22
```

Beispiel RDP-Tunnel:

```text
Admin-PC 127.0.0.1:15389
        |
        v
WartungsRemote Relay
        |
        v
Kunden-Agent
        |
        v
127.0.0.1:3389
```

Der Benutzer kann somit parallel weiterhin den normalen SSH- oder RDP-Port verwenden, sofern er diesen selbst zugänglich gemacht hat.

## 4. Zielplattformen

### Linux

Erste Zielsysteme:

- Debian
- Ubuntu
- Raspberry Pi OS
- Unraid
- später Rocky Linux / AlmaLinux

Architekturen:

- x86_64
- ARM64

### Windows

- Windows 10
- Windows 11
- Windows Server 2019 oder neuer
- x86_64
- später ARM64

## 5. Agent

Der Agent läuft als regulärer Systemdienst.

Linux:

```text
systemd
```

Windows:

```text
Windows Service
```

Der Agent ist sichtbar installiert und enthält keine versteckten Funktionen.

### Aufgaben des Agents

- Betriebssystem erkennen
- Architektur erkennen
- Hostname erfassen
- Hardwarestatus erfassen
- CPU-Auslastung erfassen
- RAM-Auslastung erfassen
- Datenträgerstatus erfassen
- Netzwerkstatus erfassen
- lokale IP-Adressen melden
- öffentliche IP serverseitig erfassen lassen
- Agent-Version melden
- Heartbeat senden
- Statusabfragen beantworten
- Verbindungsqualität messen
- Remote-Terminal bereitstellen
- SSH-/RDP-Tunnel bereitstellen
- Dateiübertragung ermöglichen
- Dienste verwalten
- Prozesse anzeigen und bei Berechtigung beenden
- Logs bereitstellen
- sichere Agent-Updates durchführen

## 6. Geräteidentität

Jedes Gerät erhält eine eindeutige zufällige Geräte-ID.

Beispiel:

```text
DEVICE-7f03b62c-e3d1-4e43-b38f-xxxxxxxxxxxx
```

Anzeigenamen sind frei konfigurierbar.

Beispiele:

```text
Firma A – Server 01
Firma A – Büro-PC 03
Privatkunde Müller – Laptop
```

## 7. Enrollment / Ersteinrichtung

Ein neues Gerät wird über ein einmalig gültiges Enrollment-Token registriert.

Ablauf:

1. Admin öffnet im Dashboard „Gerät hinzufügen“.
2. Server erzeugt ein kryptografisch zufälliges Enrollment-Token.
3. Token erhält eine kurze Lebensdauer, z. B. 30 Minuten.
4. Token ist nur einmal verwendbar.
5. Agent wird mit Serveradresse und Enrollment-Token installiert.
6. Agent erzeugt lokal ein asymmetrisches Schlüsselpaar.
7. Privater Schlüssel bleibt auf dem Gerät.
8. Öffentlicher Schlüssel wird beim Enrollment übertragen.
9. Server registriert das Gerät.
10. Enrollment-Token wird dauerhaft ungültig.

## 8. Geräteauthentifizierung

Es wird kein dauerhaftes achtstelliges Wartungspasswort verwendet.

Stattdessen:

- eindeutige Geräte-ID
- lokaler privater Geräteschlüssel
- öffentlicher Geräteschlüssel auf dem Server
- TLS 1.3
- optional mTLS / Gerätezertifikate

Der private Geräteschlüssel verlässt das Endgerät nicht.

## 9. Kommunikation

### Control Channel

Der Agent hält eine dauerhafte leichte Verbindung zum Server.

Empfehlung für MVP:

```text
WebSocket over TLS
```

Später optional:

- HTTP/2 Streams
- QUIC

### Heartbeat

Empfehlung:

```text
alle 30–60 Sekunden
```

Dadurch kann der Server den Onlinezustand zeitnah erkennen.

### Statusdaten

Ausführlichere Systemdaten werden z. B. alle 5–15 Minuten gesendet oder auf Anfrage aktualisiert.

## 10. Statusmodell

Beispiel:

```text
Heartbeat < 90 Sekunden: ONLINE
Heartbeat > 90 Sekunden: CONNECTION LOST
Heartbeat > 5 Minuten: OFFLINE
```

Alle Grenzwerte sollen konfigurierbar sein.

## 11. Automatische Zustandsbewertung

Das Dashboard soll nicht nur Rohdaten anzeigen, sondern automatisch bewerten, ob Wartungsbedarf besteht.

Mögliche Meldungen:

- Datenträger über 90 % belegt
- RAM dauerhaft über 90 %
- CPU über längere Zeit über 95 %
- SMART-Warnung
- Agent veraltet
- Backup zu alt
- Systemneustart erforderlich
- wichtiger Dienst gestoppt
- Gerät länger offline
- ungewöhnlich hohe Uptime

Statusfarben:

```text
Grün   = OK
Gelb   = Warnung
Rot    = Kritisch
Grau   = Offline
```

## 12. Wartungshistorie

Jede Wartungssitzung soll nachvollziehbar gespeichert werden.

Beispiel:

```text
Gerät: Firma A / Server01
Techniker: Admin
Start: 2026-08-23 11:42
Ende: 2026-08-23 12:07

Aktionen:
- Terminal geöffnet
- nginx neu gestartet
- Konfigurationsdatei übertragen
- Statusprüfung durchgeführt
```

Zusätzlich können manuelle Wartungsnotizen angelegt werden.

## 13. Temporäre erhöhte Rechte

Remote-Zugriffe sollen standardmäßig mit minimal notwendigen Rechten ausgeführt werden.

Für administrative Aktionen wird eine temporäre Rechteerhöhung verwendet.

Beispiel:

1. Techniker öffnet Remote-Terminal.
2. Sitzung läuft zunächst ohne unnötige Adminrechte.
3. Techniker fordert Adminrechte an.
4. Dashboard verlangt erneute Authentifizierung und TOTP.
5. Server erstellt eine temporär privilegierte Session.
6. Rechte laufen automatisch nach z. B. 15 Minuten aus.
7. Bei Bedarf müssen sie erneut bestätigt werden.

Damit bleiben erhöhte Rechte nur so lange aktiv, wie sie tatsächlich benötigt werden.

Konfigurierbare Werte:

- 5 Minuten
- 10 Minuten
- 15 Minuten
- 30 Minuten

Empfohlener Standard:

```text
15 Minuten
```

## 14. Admin-Webinterface

Das Admin-Webinterface soll standardmäßig nicht öffentlich im Internet erreichbar sein.

Mögliche Varianten:

- nur localhost
- nur internes LAN
- nur internes Management-VLAN
- Zugriff ausschließlich per VPN

Beispiel:

```text
192.168.178.20:9443
```

oder:

```text
127.0.0.1:9443
```

## 15. Admin-Authentifizierung

Vorgesehen:

- Benutzername
- Passwort
- Argon2id-Passworthash
- TOTP
- Recovery Codes
- Session Timeout
- Reauthentication bei kritischen Aktionen
- Rate Limiting
- Login-Sperren
- Audit Logging

Später optional:

- WebAuthn
- Passkeys

## 16. Benutzerrollen

Bereits früh Mehrbenutzerfähigkeit vorbereiten.

Rollen:

- Super Admin
- Administrator
- Techniker
- Read Only

Beispiele für Berechtigungen:

| Funktion | Read Only | Techniker | Admin |
|---|---:|---:|---:|
| Geräte anzeigen | ja | ja | ja |
| Status aktualisieren | nein | ja | ja |
| Terminal | nein | ja | ja |
| RDP/SSH-Tunnel | nein | ja | ja |
| Dateien | nein | ja | ja |
| Dienste | nein | ja | ja |
| Prozesse beenden | nein | optional | ja |
| Geräte löschen | nein | nein | ja |
| Benutzer verwalten | nein | nein | ja |

## 17. Remote-Terminal

### Linux

PTY-basierte Sitzung, z. B.:

```text
/bin/bash
```

### Windows

Standardmäßig:

```text
PowerShell
```

Optional:

```text
cmd.exe
```

Das Webinterface stellt ein interaktives Terminal bereit.

## 18. RDP- und SSH-Tunneling

Für die erste produktive Version ist Tunneling sinnvoller als ein komplett eigenes Remote-Desktop-Protokoll.

### SSH

Der Agent verbindet lokal zu:

```text
127.0.0.1:22
```

Der Administrator erhält lokal einen temporären Port.

### RDP

Der Agent verbindet lokal zu:

```text
127.0.0.1:3389
```

Der Administrator erhält lokal einen temporären Port.

Die vorhandenen Dienste bleiben davon vollständig unabhängig.

## 19. Eigener Remote Desktop

Ein eigener Remote-Desktop-Stack wird als spätere Erweiterung vorgesehen.

Mögliche Funktionen:

- Screen Capture
- Maussteuerung
- Tastatursteuerung
- Multi Monitor
- Clipboard
- adaptive Bildqualität
- Bandbreitenlimit
- sichtbare Sitzungshinweise
- optionaler Benutzer-Consent

## 20. Kundenseitige Funktionsfreigaben

Je Gerät einstellbar:

- Monitoring erlaubt
- Terminal erlaubt
- Remote Desktop erlaubt
- SSH-Tunnel erlaubt
- RDP-Tunnel erlaubt
- Dateiübertragung erlaubt
- Prozessverwaltung erlaubt
- Dienstverwaltung erlaubt
- unbeaufsichtigter Zugriff erlaubt

## 21. Benutzer-Consent

Für Arbeitsplatz-PCs optional:

```text
Ein Techniker möchte eine Wartungsverbindung starten.

Techniker: Max Mustermann

[Zulassen]
[Ablehnen]
```

Für Server kann optional „Unattended Access“ aktiviert werden.

## 22. Dateiübertragung

Funktionen:

- Verzeichnis anzeigen
- Datei herunterladen
- Datei hochladen
- Ordner anlegen
- umbenennen
- löschen
- Hashprüfung
- Übertragungsfortschritt
- Größenlimit
- Audit Log

## 23. Prozessverwaltung

Anzeige:

- PID
- Name
- CPU
- RAM
- Benutzer
- Startzeit

Aktionen:

- Details anzeigen
- Prozess beenden

Kritische Aktionen benötigen entsprechende Berechtigungen.

## 24. Dienstverwaltung

### Linux

systemd:

- Status
- Start
- Stop
- Restart

### Windows

Windows Services:

- Status
- Start
- Stop
- Restart

## 25. Logzugriff

Linux:

- journalctl

Windows:

- Windows Event Log

Funktionen:

- Suche
- Filter
- Zeitraum
- maximaler Umfang
- Export

## 26. Monitoring

Mögliche historische Werte:

- CPU
- RAM
- Datenträger
- Netzwerk
- Temperaturen
- Uptime

Beispiel Aufbewahrung:

```text
< 24 h  : 1-Minuten-Werte
< 7 Tage: 5-Minuten-Werte
< 30 Tage: 1-Stunden-Werte
```

## 27. Alarme

Mögliche Regeln:

- Gerät offline
- CPU zu hoch
- RAM zu hoch
- Datenträger voll
- SMART-Warnung
- Dienst ausgefallen
- Agent veraltet
- Backup veraltet

Spätere Benachrichtigungen:

- E-Mail
- ntfy
- Telegram
- Webhook
- ioBroker

## 28. Kundenstruktur

```text
Kunden
├── Firma A
│   ├── Server01
│   ├── PC01
│   └── PC02
├── Firma B
│   └── Server01
└── Privatkunden
```

Tags:

- Linux
- Windows
- Server
- Workstation
- kritisch
- Produktion

## 29. Audit Logging

Zu protokollieren:

- Login
- Logout
- fehlgeschlagener Login
- Enrollment
- Gerät gelöscht
- Gerät deaktiviert
- Terminal geöffnet
- SSH/RDP-Tunnel geöffnet
- Datei übertragen
- Prozess beendet
- Dienst gestartet/gestoppt
- Rechteerhöhung gestartet
- Rechteerhöhung beendet
- Agent-Update gestartet
- Agent-Update abgeschlossen

Audit Logs sollen nicht normal über die GUI löschbar sein.

## 30. Agent-Updates

Updates müssen digital signiert werden.

Ablauf:

1. Server meldet neue Agent-Version.
2. Agent lädt Release-Paket herunter.
3. Agent prüft Hash.
4. Agent prüft digitale Signatur.
5. Agent installiert Update.
6. Agent startet neu.
7. Health Check läuft.
8. Bei Fehler wird Rollback durchgeführt.

Der private Release-Signing-Key soll nicht auf dem normalen Produktivserver liegen.

## 31. Sicherheitsanforderungen

- TLS 1.3
- keine Klartextkommunikation
- keine statischen 8-Zeichen-Gerätepasswörter
- asymmetrische Geräteidentität
- Enrollment Token nur einmal gültig
- Enrollment Token mit Ablaufzeit
- mTLS optional bzw. bevorzugt
- Rate Limiting
- Replay-Schutz
- sichere Sessions
- CSRF-Schutz
- XSS-Schutz
- SQL-Injection-Schutz
- sichere Dateipfade
- Directory-Traversal-Schutz
- signierte Updates
- minimale Agentrechte
- vollständige Auditierung
- keine versteckten Funktionen

## 32. Datenschutz

Nur technisch notwendige Daten erheben.

Erlaubte typische Daten:

- Hostname
- Betriebssystem
- Systemmetriken
- IP-Adressen
- Netzwerkstatus
- Agent-Version
- Wartungshistorie

Nicht ungefragt erfassen:

- Passwörter
- WLAN-Schlüssel
- Cookies
- Browserhistorie
- private Dokumentinhalte
- Kamera
- Mikrofon
- Tastatureingaben außerhalb ausdrücklich gestarteter Remote-Sitzungen

## 33. Technologievorschlag

### Agent

Go

### Server

Go

### Webfrontend

React + TypeScript

### Datenbank

PostgreSQL

### Reverse Proxy

Caddy oder nginx

### Deployment

Docker / Docker Compose

## 34. Server-Komponenten

```text
wartungsremote-agent-gateway
wartungsremote-core
wartungsremote-relay
wartungsremote-web
postgres
```

Für MVP können Gateway, Core und Relay zunächst in einem Go-Dienst zusammengefasst werden.

## 35. Repository-Struktur

```text
wartungsremote/
├── agent/
│   ├── cmd/
│   ├── internal/
│   │   ├── hardware/
│   │   ├── network/
│   │   ├── terminal/
│   │   ├── files/
│   │   ├── services/
│   │   ├── processes/
│   │   └── updater/
│   └── platform/
│       ├── linux/
│       └── windows/
├── server/
│   ├── api/
│   ├── auth/
│   ├── broker/
│   ├── relay/
│   ├── devices/
│   ├── monitoring/
│   ├── audit/
│   └── database/
├── web/
├── shared/
│   └── protocol/
├── deployment/
│   ├── docker/
│   ├── systemd/
│   └── windows/
├── docs/
├── scripts/
├── LICENSE
├── README.md
├── SECURITY.md
└── TODO.md
```

## 36. Produktstatus und Transparenz

Der Agent muss sichtbar installiert sein.

Linux:

```text
systemctl status wartungsremote-agent
```

Windows:

```text
WartungsRemote Agent
```

Keine:

- versteckten Prozesse
- Keylogger
- Credential Dumping
- heimliche Kamera-/Mikrofonfunktionen
- versteckte Persistenz

## 37. Branding

Beispiel:

```text
WartungsRemote Agent
Copyright © 2026 <Hersteller>
All rights reserved.
```

Zusätzlich:

- Produktversion
- Buildnummer
- Hersteller
- Supportkontakt
- Lizenz


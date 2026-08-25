# WartungsRemote – Admin-Handbuch und Dashboard-Hilfe

Dieses Dokument ist gleichzeitig als Grundlage für die integrierte Hilfe im WartungsRemote-Dashboard vorgesehen.

---

# 1. Systemüberblick

WartungsRemote verbindet entfernte Windows- und Linux-Geräte über einen zentralen Wartungsserver.

Die Kundengeräte benötigen normalerweise keine eingehenden Router-Portfreigaben.

Der Agent stellt selbst eine ausgehende TLS-Verbindung zum WartungsRemote-Server her.

```text
Kundengerät -> Internet -> WartungsRemote Server <- Admin
```

Die Verbindung kann daher auch hinter NAT oder CGNAT funktionieren.

---

# 2. Dashboard Startseite

Die Startseite zeigt den Gesamtzustand der verwalteten Geräte.

Typische Kennzahlen:

- Geräte gesamt
- Online
- Offline
- Warnungen
- kritische Systeme
- offene Alarme
- veraltete Agenten

## Statusfarben

### Grün

Gerät ist erreichbar und aktuell sind keine relevanten Probleme erkannt.

### Gelb

Gerät ist erreichbar, es wurde jedoch mindestens eine Warnung erkannt.

Beispiele:

- Datenträger wird voll
- Agent ist nicht aktuell
- erhöhte RAM-Auslastung

### Rot

Kritischer Zustand.

Beispiele:

- Datenträger nahezu voll
- wichtiger Dienst ausgefallen
- SMART-Fehler
- längere kritische CPU-Auslastung

### Grau

Gerät ist derzeit nicht mit WartungsRemote verbunden.

---

# 3. Neues Gerät hinzufügen

## Schritt 1

Im Dashboard öffnen:

```text
Geräte -> Gerät hinzufügen
```

## Schritt 2

Optional auswählen:

- Kunde
- Gruppe
- Anzeigename
- Tags

## Schritt 3

„Enrollment Token erstellen“ auswählen.

Das Token ist:

- zeitlich begrenzt
- nur einmal verwendbar
- ausschließlich für die erste Registrierung gedacht

## Schritt 4

Agent auf dem Zielgerät installieren.

Dabei werden benötigt:

- WartungsRemote Serveradresse
- Enrollment Token

## Schritt 5

Nach erfolgreicher Registrierung erscheint das Gerät automatisch im Dashboard.

---

# 4. Enrollment schlägt fehl

Mögliche Ursachen:

## Enrollment Token abgelaufen

Lösung:

Ein neues Token erstellen.

## Enrollment Token bereits verwendet

Ein Token kann nur einmal verwendet werden.

Lösung:

Neues Token erzeugen.

## Agent erreicht Server nicht

Prüfen:

- Internetzugang vorhanden?
- DNS funktioniert?
- Serveradresse korrekt?
- TCP 443 ausgehend erlaubt?
- TLS-Inspection oder Proxy vorhanden?

## Falsche Systemzeit

Eine stark falsche Systemzeit kann TLS und Tokenprüfung beeinträchtigen.

Zeitdienst prüfen.

---

# 5. Gerät ist offline

Ein grauer Status bedeutet, dass kein aktueller Heartbeat vorhanden ist.

Prüfreihenfolge:

1. Ist das Gerät eingeschaltet?
2. Hat das Gerät Internetzugang?
3. Läuft der WartungsRemote Agent?
4. Kann der Agent den Wartungsserver erreichen?
5. Gibt es Firewall- oder Proxyänderungen?
6. Ist die Agent-Konfiguration gültig?

## Linux

```bash
systemctl status wartungsremote-agent
```

Logs:

```bash
journalctl -u wartungsremote-agent
```

## Windows

Öffnen:

```text
services.msc
```

Dienst suchen:

```text
WartungsRemote Agent
```

Alternativ PowerShell:

```powershell
Get-Service WartungsRemoteAgent
```

---

# 6. Status aktualisieren

Auf der Geräteseite:

```text
Status aktualisieren
```

Der Server fordert sofort neue Gerätedaten an.

Aktualisiert werden unter anderem:

- CPU
- RAM
- Datenträger
- Netzwerk
- Uptime
- Systeminformationen

Wenn die Abfrage fehlschlägt, sollte zuerst ein Verbindungstest durchgeführt werden.

---

# 7. Verbindungstest

Der Button:

```text
Verbindung testen
```

prüft möglichst mehrere Ebenen.

Beispiele:

- Agent erreichbar
- Geräteauthentifizierung gültig
- Control Channel aktiv
- Relay verfügbar
- Round Trip Time
- Terminal-Funktion verfügbar
- SSH-Tunnel verfügbar
- RDP-Tunnel verfügbar

Ein fehlgeschlagener Ping auf die öffentliche Kunden-IP bedeutet nicht automatisch, dass WartungsRemote gestört ist.

Viele Router blockieren ICMP.

Entscheidend ist die aktive Agent-Verbindung.

---

# 8. Remote Terminal

Gerät öffnen:

```text
Remote -> Terminal
```

## Linux

Standardmäßig wird eine Shell-Sitzung geöffnet.

## Windows

Standardmäßig wird PowerShell verwendet.

Alle Terminal-Sitzungen werden im Audit Log protokolliert.

---

# 9. Temporäre Administratorrechte

Eine normale Remote-Sitzung soll nicht automatisch dauerhaft maximale Rechte erhalten.

Wenn eine administrative Aktion notwendig ist:

```text
Adminrechte anfordern
```

Danach erfolgt eine erneute Sicherheitsprüfung.

Typischerweise:

- Passwort erneut eingeben
- TOTP bestätigen

Anschließend erhält die Sitzung für einen begrenzten Zeitraum erhöhte Rechte.

Standard:

```text
15 Minuten
```

Nach Ablauf werden die erhöhten Rechte automatisch entzogen.

## Warum?

Dadurch bleibt eine offene Wartungssitzung nicht unnötig lange privilegiert.

---

# 10. SSH-Verbindung

WartungsRemote verändert den normalen SSH-Port des Zielgeräts nicht.

Der vorhandene Dienst kann weiterhin auf Port 22 laufen.

Beim Start eines WartungsRemote SSH-Tunnels wird auf dem Admin-PC ein temporärer lokaler Port erzeugt.

Beispiel:

```text
127.0.0.1:10022
```

Dieser wird über das Relay zum Zielsystem weitergeleitet:

```text
127.0.0.1:10022 -> WartungsRemote Relay -> Zielgerät 127.0.0.1:22
```

Danach kann ein normaler SSH-Client verwendet werden.

Beispiel:

```bash
ssh user@127.0.0.1 -p 10022
```

Der Zielrechner benötigt dafür keine öffentliche SSH-Portfreigabe.

---

# 11. RDP-Verbindung

Auch der normale RDP-Dienst bleibt unangetastet.

Standardport auf Windows:

```text
3389
```

WartungsRemote öffnet lokal am Admin-PC beispielsweise:

```text
127.0.0.1:15389
```

Weiterleitung:

```text
127.0.0.1:15389 -> WartungsRemote Relay -> Zielgerät 127.0.0.1:3389
```

Anschließend kann der normale Windows-RDP-Client verwendet werden.

Beispiel Ziel:

```text
127.0.0.1:15389
```

Der Benutzer kann unabhängig davon seinen RDP-Port 3389 weiterhin normal verwenden, wenn er das möchte.

---

# 12. SSH/RDP Tunnel funktioniert nicht

Prüfen:

1. Ist das Gerät online?
2. Funktioniert der Verbindungstest?
3. Läuft auf dem Zielgerät überhaupt SSH bzw. RDP?
4. Lauscht der Dienst lokal auf dem Standardport?
5. Blockiert die lokale Firewall den Zugriff des Agents auf localhost?
6. Ist die Funktion für dieses Gerät erlaubt?
7. Hat der angemeldete Techniker die notwendige Berechtigung?

## Linux SSH prüfen

```bash
ss -lntp | grep :22
```

oder:

```bash
systemctl status ssh
```

## Windows RDP prüfen

Remote Desktop muss in Windows aktiviert sein.

Zusätzlich kann geprüft werden, ob Port 3389 lokal lauscht.

---

# 13. Dateien

Unter:

```text
Gerät -> Dateien
```

können bei entsprechender Berechtigung Dateien verwaltet werden.

Mögliche Aktionen:

- Upload
- Download
- Ordner erstellen
- umbenennen
- löschen

Alle Dateiaktionen sollen im Audit Log nachvollziehbar sein.

---

# 14. Dienste

Unter:

```text
Gerät -> Dienste
```

können Systemdienste angezeigt werden.

Linux:

- systemd

Windows:

- Windows Services

Mögliche Aktionen:

- Start
- Stop
- Restart

Administrative Aktionen können temporäre erhöhte Rechte erfordern.

---

# 15. Prozesse

Unter:

```text
Gerät -> Prozesse
```

werden unter anderem angezeigt:

- PID
- Name
- CPU
- RAM
- Benutzer

Das Beenden eines Prozesses muss protokolliert werden.

Bei kritischen Systemprozessen soll das Dashboard zusätzlich warnen.

---

# 16. Monitoring

Die Monitoring-Ansicht zeigt historische Systemwerte.

Typische Zeiträume:

- letzte Stunde
- 24 Stunden
- 7 Tage
- 30 Tage

Mögliche Daten:

- CPU
- RAM
- Datenträger
- Netzwerk
- Temperatur

---

# 17. Wartungshistorie

Unter:

```text
Gerät -> Wartung
```

werden vergangene Wartungssitzungen angezeigt.

Eine Sitzung enthält beispielsweise:

- Techniker
- Startzeit
- Endzeit
- ausgeführte Aktionen
- Notizen
- Ergebnis

Zusätzlich können manuelle Wartungsnotizen gespeichert werden.

---

# 18. Audit Log

Das Audit Log ist die zentrale Nachvollziehbarkeit administrativer Aktionen.

Beispiele:

- Login
- Terminal geöffnet
- SSH-Tunnel gestartet
- RDP-Tunnel gestartet
- Datei hochgeladen
- Dienst neu gestartet
- Prozess beendet
- Rechte erhöht
- Agent aktualisiert

Audit Logs sollen nicht regulär über die normale Benutzeroberfläche löschbar sein.

---

# 19. Agent Update

Wenn ein neuer Agent verfügbar ist, kann das Dashboard einen Hinweis anzeigen.

Updateprozess:

1. signiertes Paket wird geladen
2. Hash wird geprüft
3. Signatur wird geprüft
4. Update wird installiert
5. Agent startet neu
6. Health Check
7. bei Fehler Rollback

Ein unsigniertes Update darf nicht installiert werden.

---

# 20. Agent Update schlägt fehl

Prüfen:

- genügend Speicherplatz?
- Agent hat Schreibrechte?
- Updatepaket vollständig?
- Signatur gültig?
- System unterstützt die neue Version?
- Antivirus blockiert Update?
- Linux-Dateisystem schreibgeschützt?

Das bisherige Agent-Binary soll erhalten bleiben, bis der Health Check erfolgreich war.

---

# 21. Netzwerk und Firewall

## Kundenseite

Grundsätzlich benötigt der Agent nur ausgehende Kommunikation zum WartungsRemote-Server.

Empfehlung:

```text
TCP 443 ausgehend
```

Keine eingehenden Routerports sind für WartungsRemote notwendig.

## Admin-Webinterface

Das Webinterface soll nur intern oder per VPN erreichbar sein.

Nicht unnötig öffentlich freigeben.

---

# 22. Öffentliche IP

Die öffentliche IP wird bevorzugt serverseitig aus der Agent-Verbindung erkannt.

Dadurch ist kein externer „What is my IP“-Dienst erforderlich.

Bei CGNAT kann die angezeigte Adresse eine Provider-NAT-Adresse sein.

Das ist für den WartungsRemote-Verbindungsaufbau kein Problem, da die Verbindung vom Agent ausgeht.

---

# 23. Häufige Fehler

## Gerät meldet sich nicht mehr

- Agentdienst prüfen
- Internet prüfen
- DNS prüfen
- Firewall prüfen
- Server erreichbar?
- Zertifikatsproblem?
- Uhrzeit korrekt?

## Gerät online, Terminal funktioniert nicht

- Capability vorhanden?
- Berechtigung vorhanden?
- Agent-Version kompatibel?
- Shell vorhanden?
- Privilege Session nötig?

## RDP-Tunnel aktiv, mstsc verbindet nicht

- RDP auf Zielsystem aktiviert?
- Dienst läuft?
- Benutzer darf RDP?
- lokaler Zielport 3389 erreichbar?

## SSH-Tunnel aktiv, SSH schlägt fehl

- SSH Server installiert?
- Dienst läuft?
- Port 22 lokal offen?
- Zugangsdaten korrekt?

## Statusdaten veraltet

- Heartbeat aktuell?
- Status Request senden
- Agentlast prüfen
- Control Channel prüfen

---

# 24. Empfohlener Troubleshooting-Ablauf

Bei einem Problem immer in dieser Reihenfolge prüfen:

```text
1. Gerät online?
   |
   +-- nein -> Agent / Internet / DNS / Firewall prüfen
   |
   +-- ja
       |
2. Verbindungstest erfolgreich?
       |
       +-- nein -> Control Channel / Relay prüfen
       |
       +-- ja
           |
3. Gewünschte Capability vorhanden?
           |
           +-- nein -> Agent-Version / OS / Konfiguration prüfen
           |
           +-- ja
               |
4. Benutzerberechtigung vorhanden?
               |
               +-- nein -> Rolle / Geräterecht prüfen
               |
               +-- ja
                   |
5. Zieldienst lokal verfügbar?
                   |
                   +-- nein -> SSH/RDP/Dienst auf Zielsystem prüfen
                   |
                   +-- ja -> Sitzungs-/Relay-Logs prüfen
```

---

# 25. Sicherheitsregeln für Administratoren

- Admin-Webinterface nicht öffentlich freigeben
- TOTP niemals deaktivieren, wenn nicht zwingend nötig
- Adminpasswort nicht wiederverwenden
- Recovery Codes sicher verwahren
- privilegierte Sitzungen nur bei Bedarf starten
- Wartungssitzungen nach Abschluss schließen
- verdächtige Audit-Einträge prüfen
- Agenten aktuell halten
- Enrollment Tokens nicht dauerhaft speichern
- Tokens niemals per öffentlich zugänglichem Chat oder Ticket veröffentlichen

---

# 26. Geräte deaktivieren

Ein kompromittiertes oder ausgemustertes Gerät sollte im Dashboard deaktiviert werden.

Folgen:

- neue Agent-Sessions werden abgelehnt
- Remotezugriff wird blockiert
- Gerät bleibt für Audit/Historie sichtbar

Erst später vollständig löschen, wenn die Historie nicht mehr benötigt wird.

---

# 27. Gerät ersetzen

Bei Hardwaretausch nicht einfach Geräteschlüssel kopieren.

Empfohlener Ablauf:

1. altes Gerät deaktivieren
2. neues Enrollment Token erstellen
3. neuen Agent installieren
4. Gerät neu registrieren
5. bei Bedarf Anzeigename und Kundenbezug übernehmen

---

# 28. Dashboard-Hilfe technisch integrieren

Die integrierte Hilfe sollte aus Markdown-Inhalten generiert werden können.

Empfohlene Struktur:

```text
/docs/help/
  overview.md
  enrollment.md
  devices.md
  status.md
  terminal.md
  privileges.md
  ssh.md
  rdp.md
  files.md
  services.md
  processes.md
  monitoring.md
  updates.md
  troubleshooting.md
  security.md
```

Das Dashboard kann diese Dokumente rendern und eine Suche bereitstellen.

Vorteile:

- dieselbe Dokumentation im Repository und Dashboard
- Änderungen versionierbar
- KI kann die Dokumentation einfach lesen
- keine doppelte Pflege nötig

---

# 29. Empfohlene Hilfe-Navigation im Dashboard

```text
Hilfe
├── Erste Schritte
├── Geräte hinzufügen
├── Geräteübersicht
├── Status und Warnungen
├── Remote Terminal
├── Administratorrechte
├── SSH
├── RDP
├── Dateien
├── Dienste
├── Prozesse
├── Monitoring
├── Updates
├── Sicherheit
└── Fehlerbehebung
```

---

# 30. Support-Diagnosepaket

Später sinnvoll:

```text
Diagnosepaket erstellen
```

Das Paket sollte ausschließlich technische WartungsRemote-Daten enthalten, beispielsweise:

- Agent-Version
- OS-Version
- letzte Agent-Logs
- Verbindungstest
- Serveradresse
- Capability-Liste
- anonymisierte Netzwerkdiagnose

Nicht enthalten:

- Passwörter
- private Schlüssel
- Enrollment Tokens
- TOTP Secrets
- private Benutzerdateien


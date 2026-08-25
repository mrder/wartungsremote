# WartungsRemote – Integrierte Dashboard-Hilfe

Diese Datei ist als direkt renderbare Quelle für **Dashboard → Hilfe** vorgesehen.

## Schnellstart

### 1. Neues Gerät hinzufügen

1. Öffne **Geräte → Gerät hinzufügen**.
2. Wähle Kunde und optional Gruppe.
3. Erzeuge ein Enrollment-Token.
4. Nutze das Token innerhalb der angezeigten Gültigkeitsdauer auf dem Zielgerät.
5. Nach erfolgreichem Enrollment erscheint das Gerät automatisch in der Übersicht.

Das Enrollment-Token ist nur einmal gültig. Erzeuge bei Ablauf ein neues Token.

## Statusanzeigen

### Grün – OK

Agent ist online und aktuell liegen keine kritischen Health-Regeln vor.

### Gelb – Warnung

Beispiele:

- Speicherplatz knapp.
- hohe CPU/RAM-Last über längere Zeit.
- Agent-Version veraltet.
- Neustart erforderlich.

### Rot – Kritisch

Ein kritischer Grenzwert oder sicherheitsrelevanter Zustand wurde erkannt.

### Grau/Offline

Seit mehr als dem konfigurierten Offline-Zeitraum wurde kein Heartbeat empfangen.

## Gerät zeigt falsche/alte öffentliche IP

Die WAN-IP wird vom Server anhand der Agentverbindung erkannt. Klicke **Status aktualisieren** bzw. prüfe, ob der Agent eine neue Control-Verbindung aufgebaut hat.

Bei CGNAT kann die angezeigte IP nicht direkt zum Kundengerät geroutet werden. Das ist normal und verhindert WartungsRemote nicht.

## Verbindungstest

**Gerät → Verbindung testen** prüft:

- Control Channel.
- Roundtrip-Zeit.
- Capability-Status.
- Relay-Bereitschaft.
- SSH-/RDP-Ziel lokal erreichbar, wenn unterstützt.

Ein fehlgeschlagener normaler Internet-Ping zur WAN-IP ist kein Beweis dafür, dass WartungsRemote nicht funktioniert.

## Terminal öffnen

1. Gerät öffnen.
2. **Remote → Terminal**.
3. Session wird zunächst mit den vorgesehenen Standardrechten geöffnet.
4. Für administrative Aktionen bei Bedarf **Adminrechte anfordern**.

## Temporäre Adminrechte

Erhöhte Rechte gelten nur für die aktuelle Wartungssitzung und nur bis zum angezeigten Ablaufzeitpunkt.

Ablauf:

1. **Adminrechte anfordern**.
2. Passwort erneut eingeben.
3. TOTP bestätigen.
4. Countdown beginnt.
5. Nach Ablauf werden erhöhte Rechte automatisch entzogen.

Du kannst die Rechte jederzeit vorher manuell entziehen.

## SSH verwenden

WartungsRemote verändert SSH-Port 22 nicht.

Wenn SSH auf dem Zielgerät lokal läuft:

1. **Remote → SSH-Tunnel**.
2. Tunnel im WartungsRemote Helper öffnen.
3. Helper zeigt z. B. `127.0.0.1:41022`.
4. Verbinde den gewünschten SSH-Client auf diesen lokalen Port.

Der Datenweg ist:

```text
SSH Client -> localhost Helper -> WartungsRemote Relay -> Agent -> localhost:22 Zielgerät
```

Eine Router-Portfreigabe für 22 ist dafür nicht nötig.

## RDP verwenden

WartungsRemote verändert RDP-Port 3389 nicht.

1. **Remote → RDP-Tunnel**.
2. Helper starten.
3. angezeigten lokalen Endpoint verwenden, z. B. `127.0.0.1:43389`.
4. Windows Remotedesktop mit diesem Endpoint verbinden.

RDP muss auf dem Ziel-Windows selbst aktiviert und lokal erreichbar sein.

## SSH/RDP funktioniert über WartungsRemote nicht

Prüfe:

1. Gerät in WartungsRemote online?
2. Capability `ssh_tunnel` bzw. `rdp_tunnel` vorhanden?
3. lokaler Dienst aktiv?
4. Agent darf lokal `127.0.0.1:22/3389` erreichen?
5. Tunnel-Ticket nicht abgelaufen?
6. Relay im Serverstatus gesund?
7. lokale Security Software blockiert Agent oder Helper?

Die Standardports müssen **nicht** extern freigegeben sein.

## Gerät ist offline

Auf dem Kundengerät prüfen:

### Linux

```text
systemctl status wartungsremote-agent
journalctl -u wartungsremote-agent
```

### Windows

- Dienste öffnen.
- `WartungsRemote Agent` prüfen.
- WartungsRemote Agent Log ansehen.

Zusätzlich:

- DNS erreichbar?
- HTTPS/TCP 443 zum Server erlaubt?
- Systemzeit plausibel?
- Serverzertifikat gültig?

## Enrollment schlägt fehl

Häufige Gründe:

- Token abgelaufen.
- Token bereits benutzt.
- falsche Serveradresse.
- TLS/Zertifikatsproblem.
- Firewall blockiert ausgehendes 443.
- Systemzeit stark falsch.

Erzeuge bei unbekanntem Tokenstatus lieber ein neues Enrollment und widerrufe das alte.

## Dateien

Dateizugriff ist permissionsabhängig. Schreib-/Löschoperationen können erhöhte Rechte erfordern.

Bei Fehler `path_not_allowed` oder `permission_denied` nicht versuchen, die Prüfung zu umgehen; prüfe die Gerätepolicy bzw. Privilege Session.

## Dienste

**Dienste** zeigt systemd Services unter Linux bzw. Windows Services.

Start/Stop/Restart werden protokolliert.

## Prozesse

**Prozesse** zeigt laufende Prozesse. Beenden ist eine privilegierte Aktion und wird auditiert.

## Neustart / Herunterfahren

Power-Aktionen benötigen erhöhte Berechtigung und eine zusätzliche Bestätigung. Nach Neustart sollte der Agent automatisch reconnecten.

## Agent aktualisieren

1. Gerät → **Agent**.
2. verfügbare signierte Version prüfen.
3. **Aktualisieren**.
4. Agent lädt Paket und prüft Hash + Signatur.
5. Agent startet neu.
6. Dashboard zeigt Updateergebnis.

Schlägt der neue Agent beim Health-Check fehl, soll automatisch die vorherige Version wiederhergestellt werden.

## Wartungshistorie

Unter **Wartung** findest du:

- Zeitpunkt.
- Techniker.
- verwendete Remote-Funktionen.
- wichtige Änderungen.
- Notizen.
- Ergebnis.
- optional nächsten Wartungstermin.

## Audit

Audit zeigt sicherheitsrelevante Aktionen. Normale Admins können Audit-Einträge nicht über die Oberfläche löschen.

## Serverstatus prüfen

Dashboard → **System → Serverstatus** sollte anzeigen:

- Core.
- Gateway.
- Relay.
- Datenbank.
- aktive Agentverbindungen.
- aktive Remote-Sessions.
- Queue/Fehlerzustände.
- Zertifikatsstatus.
- Backupstatus, sofern integriert.

## Sicherheitsgrundsätze

- Keine Enrollment-Tokens oder Tunnel-Tickets weitergeben.
- TOTP Recovery Codes sicher offline verwahren.
- Keine Adminaccounts gemeinsam verwenden.
- Gerät bei Verlust/Diebstahl sofort revoken.
- Nur signierte Agentupdates installieren.
- Admin-Webinterface nur aus LAN/VPN zugänglich machen.

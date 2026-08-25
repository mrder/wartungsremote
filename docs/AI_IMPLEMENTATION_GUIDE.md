# WartungsRemote – Implementierungsanweisung für Coding-KI

## 1. Verbindliche Quellenreihenfolge

Bei Widersprüchen gilt:

1. `SECURITY.md`
2. `SPECIFICATION.md`
3. `PROTOCOL.md`
4. `API.md`
5. `DATABASE.md`
6. `AGENT.md` / `RELAY.md` / `DEPLOYMENT.md`
7. `PROJECT_CONCEPT.md`
8. `TODO.md`

Keine Sicherheitsanforderung stillschweigend abschwächen.

## 2. Technologie

Default:

```text
Go: aktuelle unterstützte stabile Version
Frontend: React + TypeScript
DB: PostgreSQL
Server: Docker Compose
```

Libraries sollen gut gepflegt, etabliert und möglichst klein sein.

## 3. Repository

```text
wartungsremote/
├── cmd/
│   ├── wr-agent/
│   ├── wr-core/
│   ├── wr-gateway/
│   ├── wr-relay/
│   └── wr-helper/
├── internal/
│   ├── auth/
│   ├── audit/
│   ├── device/
│   ├── enrollment/
│   ├── protocol/
│   ├── relay/
│   ├── remote/
│   ├── monitoring/
│   ├── update/
│   └── platform/
│       ├── linux/
│       └── windows/
├── web/
├── migrations/
├── docs/
├── deployment/
├── tests/
├── README.md
├── SECURITY.md
├── CHANGELOG.md
└── TODO.md
```

Eine Monorepo-Struktur ist für V1 bevorzugt.

## 4. Vorgehensweise

Die KI MUSS TODO in Reihenfolge bearbeiten, sofern keine technische Abhängigkeit eine begründete Änderung verlangt.

Pro Arbeitspaket:

1. relevante Spezifikationsdateien lesen.
2. Schnittstellen definieren.
3. Tests zuerst oder parallel anlegen.
4. implementieren.
5. Lint/Test ausführen.
6. Sicherheitschecks durchführen.
7. Dokumentation aktualisieren.
8. nur tatsächlich abgeschlossene TODOs abhaken.

## 5. Nicht improvisieren

Bei Unklarheit keine neue Sicherheitssemantik erfinden.

Insbesondere niemals spontan:

- TLS Verification deaktivieren.
- Hardcoded Secrets einbauen.
- Default Adminpasswort erstellen.
- beliebiges TCP Forwarding erlauben.
- Credentials loggen.
- Adminrechte dauerhaft machen.
- Auth nur im Frontend prüfen.
- signierte Updates umgehen.

## 6. Codequalität

- kleine Pakete mit klaren Verantwortlichkeiten.
- Interfaces nur dort, wo Abstraktion tatsächlich gebraucht wird.
- `context.Context` für abbrechbare Go-I/O-Operationen.
- Timeouts für alle Netzwerkoperationen.
- strukturierte Fehlercodes.
- keine `panic` für normale Remote-Fehler.
- graceful shutdown.
- Race Detector in CI soweit möglich.

## 7. Datenbank

- SQL Migrationen versionieren.
- parametrisierte Queries.
- Transaktionen für Security-State.
- keine automatische destructive Schema-Synchronisation in Produktion.

## 8. Frontend

Das Frontend ist niemals Autoritätsquelle für Berechtigungen. Buttons dürfen versteckt/deaktiviert werden, Server prüft trotzdem jede Aktion.

Hilfe-Bereich rendert versionierte Markdown-Dateien aus `docs/help/`.

## 9. Definition of Done

Ein Feature ist nur fertig, wenn:

- happy path.
- definierte Fehlerfälle.
- Permission Checks.
- Audit falls relevant.
- Tests.
- Doku.
- keine offenen Compiler/Lintfehler.
- keine bekannten High/Critical Security Findings.

## 10. Erstes Implementierungsziel

Der erste vertikale Slice soll sein:

```text
Server startet
-> Admin Login + TOTP
-> Enrollment Token erzeugen
-> Linux Testagent enrollen
-> Agent Control Channel
-> Heartbeat
-> Dashboard zeigt Online/Offline + Basisinventar
```

Noch kein Terminal, bevor diese Kette stabil und getestet ist.

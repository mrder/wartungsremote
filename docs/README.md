# WartungsRemote Dokumentationspaket

Dieses Paket ist die möglichst vollständige Planungs- und Implementierungsgrundlage für WartungsRemote V1.

## Verbindliche Dokumente

| Datei | Zweck |
|---|---|
| `SECURITY.md` | Security Architecture, Threat Model und unverhandelbare Sicherheitsregeln |
| `SPECIFICATION.md` | normative Produktspezifikation V1 und Akzeptanzkriterien |
| `PROTOCOL.md` | Agent/Server-Wire-Protocol, Nachrichten, Streams, Replay-Schutz |
| `API.md` | Admin-/Agent-HTTP-API V1 |
| `DATABASE.md` | PostgreSQL-Datenmodell, Retention und Transaktionen |
| `AGENT.md` | Agent-Lifecycle, OS-Abstraktion, Terminal, Dateien, Update |
| `RELAY.md` | Relay und nativer SSH/RDP-Tunnel über `wr-helper` |
| `PERMISSIONS.md` | RBAC-, Scope- und Privilege-Matrix |
| `STATE_MACHINES.md` | verbindliche Zustandsmodelle für Enrollment, Geräte, Sessions, Tunnel und Updates |
| `CONFIGURATION.md` | Server-/Agentkonfiguration und sichere Defaults |
| `DEPLOYMENT.md` | Produktionsdeployment, Netzwerk, Backup, Production Mode |
| `TEST_PLAN.md` | Unit-, Integration-, Security-, Plattform- und Lasttests |
| `AI_IMPLEMENTATION_GUIDE.md` | verbindliche Arbeitsanweisung für eine Coding-KI |
| `PROJECT_CONCEPT.md` | fachliches Gesamtkonzept und Produktvision |
| `TODO.md` | Entwicklungsreihenfolge und Arbeitspakete |
| `ADMIN_GUIDE.md` | ausführliches Administratorhandbuch |
| `DASHBOARD_HELP.md` | direkt renderbare Hilfe für das Web-Dashboard |
| `ARCHITECTURE.md` | kompakter Architekturüberblick |
| `REFERENCES.md` | Standards und Security-Referenzen |

## Priorität bei Widersprüchen

```text
SECURITY.md
  > SPECIFICATION.md
  > PROTOCOL.md
  > API.md
  > DATABASE.md
  > AGENT.md / RELAY.md / CONFIGURATION.md / DEPLOYMENT.md
  > PROJECT_CONCEPT.md
  > TODO.md
```

## Prompt für eine Coding-KI

```text
Lies zuerst README.md und AI_IMPLEMENTATION_GUIDE.md.
Behandle SECURITY.md und SPECIFICATION.md als verbindlich.
Implementiere das System schrittweise gemäß TODO.md.
Ändere keine Security-Invarianten stillschweigend.
Keine versteckten Funktionen, keine Klartext-Credentials, kein beliebiges TCP-Forwarding.
SSH Port 22 und RDP Port 3389 des Zielsystems dürfen durch WartungsRemote nicht verändert oder belegt werden.
Agenten müssen ohne eingehende Router-Portfreigabe funktionieren.
Für jede abgeschlossene Funktion: Tests, Permission Checks, Audit und Dokumentation ergänzen.
Hake TODO.md nur ab, wenn die Definition of Done erfüllt ist.
```

## Empfohlener Stack

```text
Agent/Core/Gateway/Relay/Helper: Go
Web: React + TypeScript
DB: PostgreSQL
Transport V1: HTTPS/TLS + WebSocket/Relay Streams
Deployment: Docker Compose
```

## Erster vertikaler Meilenstein

```text
Admin Login + TOTP
-> Enrollment Token
-> Linux Testagent Enrollment
-> authentifizierter Control Channel
-> Heartbeat
-> Basisinventar
-> Dashboard Online/Offline
```

Erst danach werden Terminal, Privilege Sessions und Tunnel implementiert.

# WartungsRemote – Rollen- und Berechtigungsmatrix V1

## 1. Grundsatz

Rollen sind nur Sammlungen granularer Permissions. Die eigentliche Prüfung erfolgt serverseitig auf Permission + Scope + Device Policy + ggf. Privilege Session.

## 2. Standardrollen

| Permission | Read Only | Technician | Admin | Super Admin |
|---|:---:|:---:|:---:|:---:|
| `device.read` | ✓ | ✓ | ✓ | ✓ |
| `device.manage` | – | – | ✓ | ✓ |
| `customer.read` | ✓ | ✓ | ✓ | ✓ |
| `customer.manage` | – | – | ✓ | ✓ |
| `monitoring.read` | ✓ | ✓ | ✓ | ✓ |
| `remote.terminal` | – | ✓ | ✓ | ✓ |
| `remote.tunnel.ssh` | – | ✓ | ✓ | ✓ |
| `remote.tunnel.rdp` | – | ✓ | ✓ | ✓ |
| `remote.files.read` | – | ✓ | ✓ | ✓ |
| `remote.files.write` | – | optional | ✓ | ✓ |
| `remote.service.control` | – | optional | ✓ | ✓ |
| `remote.process.terminate` | – | optional | ✓ | ✓ |
| `remote.power` | – | – | ✓ | ✓ |
| `privilege.request` | – | ✓ | ✓ | ✓ |
| `maintenance.read` | ✓ | ✓ | ✓ | ✓ |
| `maintenance.write` | – | ✓ | ✓ | ✓ |
| `audit.read` | – | ✓ | ✓ | ✓ |
| `enrollment.create` | – | – | ✓ | ✓ |
| `credential.revoke` | – | – | ✓ | ✓ |
| `agent.update` | – | optional | ✓ | ✓ |
| `user.manage` | – | – | – | ✓ |
| `role.manage` | – | – | – | ✓ |
| `system.settings` | – | – | – | ✓ |

`optional` bedeutet: Default-Role kann dies je nach gewünschtem Betriebsmodell erhalten; die Permission existiert unabhängig davon.

## 3. Scopes

Eine Permission allein gewährt noch keinen Zugriff auf jedes Gerät.

Unterstützte Scopes V1:

- global.
- customer.
- group.
- explicit device optional später.

Beispiel:

```text
Techniker Max:
  role = technician
  scope = customer: Firma A
```

Darf trotz `device.read` keine Geräte von Firma B lesen.

## 4. Privilege-required Actions

Zusätzlich zu Permission standardmäßig Privilege Session erforderlich:

- Systemdateien schreiben/löschen.
- Service Start/Stop/Restart, sofern als administrativ markiert.
- Prozess eines fremden/privilegierten Users terminieren.
- Power Restart/Shutdown.
- Agent Update erzwingen.
- privilegiertes Terminal.

SSH/RDP-Tunnel selbst benötigen nicht zwingend eine OS-Privilege-Session, da die eigentliche OS-Anmeldung weiterhin im SSH/RDP-Protokoll erfolgt. Die Tunnel-Erstellung ist trotzdem permission- und auditpflichtig.

## 5. Device Policy

Ein Serveradmin kann eine Funktion grundsätzlich erlauben; die lokale Agentpolicy kann sie zusätzlich blockieren.

Effektive Berechtigung:

```text
User Permission
AND User Scope
AND Device Capability
AND Device Policy
AND Session State
AND Privilege State when required
```

Wenn eine Bedingung false ist: deny.

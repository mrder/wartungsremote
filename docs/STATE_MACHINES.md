# WartungsRemote – Zustandsmaschinen V1

## 1. Device Connectivity

```text
UNKNOWN
  |
  | enrolled/connect
  v
ONLINE
  |
  | heartbeat timeout > 120s
  v
CONNECTION_LOST
  |
  | timeout > 300s
  v
OFFLINE

CONNECTION_LOST -- valid heartbeat --> ONLINE
OFFLINE ----------- reconnect ------> ONLINE
ANY --------------- credential revoke --> REVOKED
```

`health` und `connectivity` sind getrennte Zustände. Ein ONLINE-Gerät kann `health=critical` sein.

## 2. Enrollment

```text
ISSUED
  | success
  v
CONSUMED

ISSUED -- ttl --> EXPIRED
ISSUED -- admin revoke --> REVOKED
```

Nur `ISSUED` darf atomar zu `CONSUMED` wechseln.

## 3. Remote Session

```text
REQUESTED
  -> OPENING
  -> ACTIVE
  -> CLOSING
  -> CLOSED

REQUESTED/OPENING -> FAILED
ACTIVE -> INTERRUPTED (agent/server disconnect)
ACTIVE -> EXPIRED
```

Keine `INTERRUPTED` Session wird stillschweigend wieder `ACTIVE`; neue Session erforderlich.

## 4. Privilege Session

```text
NONE
 -> REAUTH_REQUIRED
 -> GRANTED
 -> EXPIRED

GRANTED -> REVOKED
GRANTED -> EXPIRED automatically
```

`GRANTED` ist an Remote Session + User + Device gebunden.

## 5. Tunnel

```text
REQUESTED
 -> PREPARED
 -> TICKET_ISSUED
 -> CONNECTING
 -> ACTIVE
 -> CLOSED
```

Fehlerzustände:

```text
EXPIRED
DENIED
FAILED
INTERRUPTED
```

Ticket wechselt bei erfolgreichem Relay-Handshake atomar von `ISSUED` zu `USED`.

## 6. Remote Command

```text
CREATED
 -> DISPATCHED
 -> ACKNOWLEDGED
 -> SUCCEEDED
```

oder:

```text
CREATED -> EXPIRED
DISPATCHED -> TIMEOUT
ACKNOWLEDGED -> FAILED
ANY non-final -> CANCELLED where supported
```

Finalzustände werden für dieselbe `command_id` nicht erneut ausgeführt.

## 7. Agent Update

```text
AVAILABLE
 -> REQUESTED
 -> DOWNLOADING
 -> VERIFIED
 -> STAGED
 -> INSTALLING
 -> HEALTH_CHECK
 -> SUCCESS
```

Fehler:

```text
DOWNLOADING -> FAILED
VERIFIED cannot be entered if signature/hash invalid
INSTALLING/HEALTH_CHECK -> ROLLBACK -> ROLLED_BACK
```

## 8. Wartungssitzung

```text
OPEN
 -> CLOSED
```

Remote Sessions und Maintenance Events referenzieren dieselbe Wartungssitzung. Ein abgeschlossener Wartungseintrag wird nicht nachträglich still überschrieben; Änderungen an Notizen werden versioniert/auditiert.

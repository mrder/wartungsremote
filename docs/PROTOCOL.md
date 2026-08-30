# WartungsRemote – Wire Protocol V1

## 1. Allgemein

Protokollbezeichner: `wrp/1`  
Transport V1: WebSocket over TLS  
Payload V1: UTF-8 JSON für Control-Nachrichten.  
Bulk-/Streamdaten können als binäre WebSocket Frames bzw. dedizierte Relay-Streams übertragen werden.

Alle unbekannten Pflichtfelder oder Nachrichtentypen sind kontrolliert abzulehnen; zusätzliche optionale Felder dürfen für Vorwärtskompatibilität ignoriert werden.

## 2. Envelope

```json
{
  "protocol": 1,
  "type": "heartbeat",
  "message_id": "0191...",
  "request_id": null,
  "timestamp": "2026-08-23T10:00:00.000Z",
  "payload": {}
}
```

Pflichtfelder:

- `protocol`: Integer.
- `type`: String.
- `message_id`: UUID.
- `timestamp`: UTC RFC3339 mit Millisekunden.
- `payload`: Objekt.

`request_id` referenziert bei Responses die ursprüngliche `message_id` bzw. Request-ID.

## 3. Größenlimits

Empfohlene Defaults:

- Control JSON: 256 KiB maximal.
- Inventory Response: 1 MiB maximal.
- Event Batch: 1 MiB maximal.
- Terminal Control Message: 64 KiB.
- Binäre Daten nicht über unlimitierte In-Memory-Puffer sammeln.

Überschreitung -> `protocol_error: message_too_large` und ggf. Verbindung schließen.

## 4. Connect Handshake

Nach TLS/WebSocket-Upgrade sendet Agent:

```json
{
  "protocol": 1,
  "type": "hello",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "device_id": "...",
    "install_id": "...",
    "agent_version": "1.0.0",
    "os": "linux",
    "arch": "amd64",
    "capabilities": ["metrics", "terminal", "ssh_tunnel"],
    "boot_id": "...",
    "secure": true
  }
}
```

`secure` ist `true`, wenn der Agent per `wss://` verbunden hat (d.h.
sein konfigurierter `server_url` ist `https://`) — vom Agenten ehrlich
selbst gemeldet, da wr-core hinter dem TLS-terminierenden Reverse Proxy
sonst keine Möglichkeit hat, das pro Gerät zu wissen. Landet in
`devices.transport_secure` (docs/API.md §5).

Server:

```json
{
  "protocol": 1,
  "type": "hello_ack",
  "message_id": "...",
  "request_id": "...",
  "timestamp": "...",
  "payload": {
    "connection_id": "...",
    "server_time": "...",
    "heartbeat_interval_seconds": 45,
    "status_interval_seconds": 300,
    "max_message_bytes": 262144,
    "minimum_agent_version": "1.0.0"
  }
}
```

## 5. Heartbeat

Agent -> Server:

```json
{
  "protocol": 1,
  "type": "heartbeat",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "uptime_seconds": 123456,
    "sequence": 42
  }
}
```

Server kann `heartbeat_ack` senden; dies ist optional konfigurierbar.

## 6. Inventory

Server -> Agent:

```json
{
  "protocol": 1,
  "type": "inventory_request",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "full": true
  }
}
```

Agent -> Server `inventory_response`:

```json
{
  "protocol": 1,
  "type": "inventory_response",
  "message_id": "...",
  "request_id": "...",
  "timestamp": "...",
  "payload": {
    "hostname": "srv01",
    "os": {
      "family": "linux",
      "distribution": "debian",
      "version": "13",
      "kernel": "6.x"
    },
    "cpu": {
      "model": "Example CPU",
      "cores": 8,
      "threads": 16
    },
    "memory_bytes": 34359738368,
    "disks": [],
    "interfaces": [],
    "agent_version": "1.0.0"
  }
}
```

## 7. Metrics

`metrics_report` enthält Punkt-in-Zeit-Daten. Ein Agent darf mehrere Filesysteme übertragen.

```json
{
  "protocol": 1,
  "type": "metrics_report",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "cpu_percent": 12.4,
    "memory": {"used_bytes": 1, "total_bytes": 2},
    "filesystems": [
      {"path": "/", "used_bytes": 1, "total_bytes": 2}
    ]
  }
}
```

## 7a. Netzwerk-Traffic-Metriken

Anders als `metrics_report` wird Netzwerk-Traffic agentseitig lokal
gepuffert (siehe internal/netmetrics, ca. alle 60s ein Sample) und dann
gebündelt als `network_metrics_batch` hochgeladen — Intervall dafür
kommt vom Server via `hello_ack.network_upload_interval_seconds`
(Standard 5min), unabhängig vom `status_interval` für §7. Ein Agent, der
diesen Nachrichtentyp nicht kennt, sendet ihn einfach nie — kein
Protokoll-Versionssprung nötig, siehe §3.

```json
{
  "protocol": 1,
  "type": "network_metrics_batch",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "samples": [
      {
        "occurred_at": "...",
        "interval_seconds": 60.2,
        "bytes_sent_total": 61440,
        "bytes_recv_total": 512000,
        "bytes_sent_control": 890,
        "bytes_recv_control": 2100
      }
    ]
  }
}
```

`*_total` = gesamter Netzwerk-Traffic des Geräts (alle Interfaces,
kumulative OS-Zähler, agentseitig zu einem Delta pro Intervall
umgerechnet). `*_control` = nur der Traffic dieses Agents auf dem
Control-Channel zu diesem Server (Nachrichtennutzlast, keine
TCP/TLS-Rohbytes — eine bewusste Näherung). Der Agent löscht ein
lokal gepuffertes Sample erst, nachdem es erfolgreich in diese
Nachricht verpackt und geschrieben wurde; bei Verbindungsverlust bleibt
es liegen und wird beim nächsten Verbindungsaufbau sofort nachgeliefert
(kein Warten auf das volle Upload-Intervall).

## 8. Generic Request Result

Erfolgreich:

```json
{
  "protocol": 1,
  "type": "command_result",
  "message_id": "...",
  "request_id": "...",
  "timestamp": "...",
  "payload": {
    "status": "success",
    "code": "ok",
    "data": {}
  }
}
```

Fehler:

```json
{
  "protocol": 1,
  "type": "command_result",
  "message_id": "...",
  "request_id": "...",
  "timestamp": "...",
  "payload": {
    "status": "error",
    "code": "permission_denied",
    "message": "Operation is not permitted by agent policy"
  }
}
```

## 9. Fehlercodes

Mindestens:

- `ok`
- `invalid_request`
- `unsupported_protocol`
- `unsupported_capability`
- `unauthenticated`
- `permission_denied`
- `privilege_required`
- `device_busy`
- `resource_not_found`
- `session_expired`
- `ticket_expired`
- `ticket_used`
- `target_not_allowed`
- `message_too_large`
- `rate_limited`
- `timeout`
- `internal_error`

Fehlermeldungen dürfen keine Secrets oder Stacktraces an untrusted Clients leaken.

## 10. Remote Session Create

Server -> Agent:

```json
{
  "protocol": 1,
  "type": "session_open",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "session_id": "...",
    "kind": "terminal",
    "expires_at": "...",
    "privileged": false,
    "options": {"shell": "default"}
  }
}
```

Agent prüft Capability und lokale Policy und antwortet `session_open_result`.

## 11. Terminal

Control:

- `terminal_open`
- `terminal_resize`
- `terminal_signal`
- `terminal_close`

Terminal-I/O wird vorzugsweise binär mit Stream Header übertragen.

Binärframe-Header V1:

```text
byte 0      frame_version = 1
byte 1      stream_kind   = 1 terminal, 2 tunnel, 3 file
bytes 2-17  stream_id UUID binary
bytes 18..  payload
```

Alternative Implementierung darf für MVP separate Relay-WebSockets verwenden, wenn dieselben Auth-/Ticketregeln gelten.

## 12. Privilege Update

Server -> Agent:

```json
{
  "protocol": 1,
  "type": "session_privilege_update",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "session_id": "...",
    "privileged": true,
    "valid_until": "...",
    "authorization_id": "..."
  }
}
```

Agent darf die Information nur akzeptieren, wenn sie aus der authentifizierten Serververbindung stammt und die Session lokal existiert.

## 13. Tunnel

Server -> Agent:

```json
{
  "protocol": 1,
  "type": "tunnel_prepare",
  "message_id": "...",
  "timestamp": "...",
  "payload": {
    "session_id": "...",
    "tunnel_id": "...",
    "target_type": "ssh_local",
    "target_host": "127.0.0.1",
    "target_port": 22,
    "expires_at": "..."
  }
}
```

Agent MUSS `target_type` gegen lokale Capability/Policy validieren. Er darf nicht blind Host/Port aus dem Payload verwenden.

Normative Abbildung:

```text
ssh_local -> 127.0.0.1:22
rdp_local -> 127.0.0.1:3389
```

## 14. Replay-Schutz

- `message_id` muss pro Sender einzigartig sein.
- sicherheitskritische Commands erhalten serverseitig persistierte `command_id`.
- bereits final ausgeführte `command_id` darf nicht erneut ausgeführt werden.
- Tunnel-Tickets sind single-use.
- Enrollment-Tokens sind single-use.
- Ablaufzeiten werden geprüft.
- starke Uhrzeitabweichung wird protokolliert; reine Clock-Skew darf Heartbeats nicht unnötig zerstören.

## 15. Reconnect

Agent Backoff Default:

```text
1s, 2s, 4s, 8s, 16s, 30s, 60s ... max 300s
```

mit ±20 % Jitter.

Bei erfolgreicher Verbindung wird Backoff zurückgesetzt.

## 16. Protokollversionierung

- Major `protocol` inkompatibel -> Verbindung ablehnen.
- optionale Features über `capabilities`.
- keine stillen Semantikänderungen bestehender Nachrichtentypen.
- neue optionale Felder sind erlaubt.
- neue Pflichtfelder erfordern neue Protokollmajorversion oder kompatiblen Feature-Negotiation-Pfad.

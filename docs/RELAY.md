# WartungsRemote – Relay & Native Tunnel Design V1

## 1. Zweck

Relay vermittelt autorisierte Streams zwischen Adminseite und Agent, ohne eingehende Verbindungen zum Kundengerät.

## 2. Kein generischer Proxy

Relay V1 unterstützt nur bekannte Session-/Streamtypen:

- terminal
- ssh_local
- rdp_local
- file_transfer

Kein SOCKS5. Kein beliebiges `host:port` Forwarding.

## 3. Native SSH/RDP Nutzung

Für native Clients wird `wr-helper` verwendet.

Beispiel:

```text
ssh -p 41022 user@127.0.0.1
mstsc -> 127.0.0.1:43389
```

Helper lauscht nur auf Loopback.

## 4. Tunnel Ticket

Eigenschaften:

- 256 Bit random.
- single-use.
- Default TTL vor Erstverbindung 60 Sekunden.
- gebunden an tunnel_id, user_id, device_id, target_type.
- nur Hash in DB.
- nach erfolgreicher Auth sofort verbraucht.

Eine aktive Relay-Verbindung besitzt danach ihre eigene Stream-Session und Ablaufzeit.

## 5. Helper Workflow

1. Dashboard `Tunnel erstellen`.
2. Core autorisiert.
3. Core bereitet Agent vor.
4. Core erzeugt Ticket.
5. Dashboard übergibt Ticket an Helper.
6. Helper wählt freien Loopbackport.
7. Helper authentifiziert Ticket am Relay.
8. Relay bestätigt und wartet auf lokalen Client.
9. Agent öffnet Zielsocket.
10. Byte-Stream läuft.
11. bei Client Close wird Tunnel geschlossen.

Alternative: Helper verbindet erst zum Relay, bindet danach Port. Sicherheitssemantik bleibt gleich.

## 6. Relay Limits

Default:

- max. 5 parallele Tunnel pro User.
- max. 3 pro Device.
- Connection Idle Timeout 30 Minuten für Tunnel ohne Traffic, konfigurierbar.
- maximale Sessiondauer 8 Stunden, kürzer per Policy.
- Bandbreitenlimit optional pro Tunnel/Kunde.

## 7. Backpressure

Streaming muss echtes Backpressure unterstützen. Kein unbegrenztes Pufferwachstum bei langsamer Gegenseite.

## 8. Disconnect

Bei Agent Disconnect:

- alle zugehörigen Streams sofort schließen.
- Status `interrupted`.
- keine automatische Wiederaufnahme eines alten SSH/RDP-Tunnels.

Bei Relay-Neustart:

- bestehende Streams brechen ab.
- Tickets bleiben nur gültig, wenn noch unbenutzt und TTL nicht abgelaufen; für maximale Einfachheit kann V1 alle outstanding Tickets beim Relay/Core-Neustart invalidieren.

## 9. Audit

Speichern:

- who.
- device.
- target_type.
- started/connected/ended.
- reason.
- byte counts optional.

Nicht speichern: Nutzdaten des SSH/RDP-Streams.

# WartungsRemote – Security Architecture & Threat Model

## 1. Sicherheitsziel

WartungsRemote ist ein hochprivilegiertes Administrationssystem. Ein Fehler kann Zugriff auf viele Kundensysteme ermöglichen. Sicherheitsentscheidungen haben daher Vorrang vor Komfort.

## 2. Trust Boundaries

```text
[Untrusted Internet]
        |
    TLS Gateway/Relay
        |
[Server Internal Network]
  Core / DB / Admin Web
        |
[Admin LAN or VPN]

[Kundengerät]
  Agent <-> lokales OS
```

Der öffentliche Gateway-Prozess ist als potenziell stärker exponiert zu behandeln als Core/DB.

## 3. Angreifermodelle

Berücksichtigen:

1. Internet-Angreifer ohne Credentials.
2. gestohlenes Enrollment-Token.
3. kompromittierter einzelner Agent.
4. kompromittierter Techniker-Account.
5. gestohlene DB-Kopie.
6. Man-in-the-Middle.
7. Replay alter Commands/Tickets.
8. bösartige/fehlerhafte Eingaben eines Agents.
9. Supply-Chain-Angriff auf Agent-Update.
10. Insider mit normaler Technikerrolle.

## 4. Sicherheitsinvarianten

- Device ID allein ist niemals Credential.
- private Geräteschlüssel verlassen das Gerät nicht.
- Enrollment-Token einmalig und kurzlebig.
- Admin Remote Access benötigt MFA im Production Mode.
- alle Rechte serverseitig prüfen.
- Agent prüft zusätzlich lokale Capability/Policy.
- Tunnel sind zielbeschränkt.
- Privilege Sessions laufen automatisch ab.
- signierte Updates sind zwingend.
- keine Secrets in Logs.

## 5. TLS

- TLS 1.3 bevorzugt/erzwungen, soweit Plattformkompatibilität dies zulässt.
- Zertifikatsvalidierung niemals deaktivieren.
- keine `InsecureSkipVerify`-Produktionsoption.
- HSTS für Admin Web.
- sichere moderne Cipher-Auswahl der verwendeten TLS-Bibliothek; keine selbst entwickelte Kryptographie.

wr-core terminiert TLS bewusst nicht selbst (docs/DEPLOYMENT.md §4) —
das übernimmt ein vorgeschalteter Reverse Proxy. Da der Server das also
nicht direkt beobachten kann, gibt es zwei ergänzende, nicht
blockierende Hinweise statt einer harten Prüfung:

- **Serverseitig**: `internal/config.Advisory` (`insecure_base_url`,
  wenn `public.base_url` nicht `https://` ist) — beim Start geloggt und
  im Dashboard als Banner für `system.settings`-Berechtigte angezeigt.
- **Clientseitig**: jeder Agent meldet beim Handshake ehrlich, ob er per
  `wss://` verbunden hat (`protocol.HelloPayload.Secure`,
  docs/PROTOCOL.md §4) — landet als `devices.transport_secure` und wird
  pro Gerät im Dashboard angezeigt, falls `false`. So kann auch ein
  einzelner, falsch konfigurierter Agent auffallen, selbst wenn der
  Server selbst korrekt hinter HTTPS läuft.

## 6. Device Credentials

Empfohlene Implementierung:

- Ed25519 für Gerätesignatur oder Plattformstandard mit vergleichbarer Sicherheit.
- alternativ mTLS mit per Enrollment ausgestelltem Client-Zertifikat.
- private Keys in OS-sicherer Ablage:
  - Windows: DPAPI/Machine Scope oder Zertifikatsspeicher mit ACL.
  - Linux: root-owned Datei `0600`, eigener Service-Account; später TPM/PKCS#11 optional.

Credential Rotation muss vorgesehen werden.

## 7. Admin Passwörter

Argon2id. Parameter müssen benchmarkbar und konfigurierbar sein.

Als Mindestuntergrenze für die erste Implementierung kann der aktuelle OWASP-Minimalwert verwendet werden; produktiv sollen Parameter auf Serverhardware so gewählt werden, dass legitime Logins akzeptabel bleiben, Offline-Bruteforce jedoch teuer ist.

Passwortregeln:

- Mindestlänge 12 Zeichen für normale Admins.
- längere Passphrases zulassen.
- keine künstlich kleine Maximallänge.
- keine erzwungenen periodischen Änderungen ohne Anlass.
- kompromittierte/geleakte Passwörter optional prüfen.

## 8. MFA/TOTP

- TOTP Secret verschlüsselt in DB.
- Setup QR nur während Enrollment/Reset anzeigen.
- Recovery Codes einmalig anzeigen, gehasht speichern.
- Rate-limit MFA-Versuche.
- MFA-Reset ist hochkritische Audit-Aktion.

Später WebAuthn/Passkeys ergänzen.

## 9. Web Sessions

- opaque random session IDs >= 128 Bit Entropie.
- nur Hash der Session-ID serverseitig speichern, wenn praktikabel.
- Cookie `__Host-`, Secure, HttpOnly, SameSite=Strict, Path=/.
- Session-ID nach Login und Privilege-Änderungen rotieren.
- CSRF-Tokens für zustandsändernde Browserrequests zusätzlich zu SameSite.
- keine Authentifizierungstokens in localStorage/sessionStorage.

## 10. Rate Limiting

Mindestens:

- Login pro IP + Account.
- MFA pro Challenge/User.
- Enrollment pro IP + Token.
- Control Connect pro Device/IP.
- API sensible Aktionen.

Lockout darf kein triviales dauerhaftes DoS gegen Adminaccounts ermöglichen. Backoff und temporäre Sperren bevorzugen.

## 11. Command Authorization

Jeder Command enthält semantischen Typ. Keine generische `exec arbitrary server command` API außerhalb der bewusst bereitgestellten Terminalfunktion.

Server prüft:

- User Session.
- Permission.
- Device Scope.
- Device Status.
- Capability.
- Privilege Session falls nötig.
- Rate/Concurrency Limits.

Agent prüft:

- authentifizierter Serverkanal.
- Session/Ticket gültig.
- lokale Policy.
- Capability.
- Zielparameter gegen Allowlist.

## 12. Tunnel Security

V1 kein SOCKS-Proxy, kein beliebiges Port Forwarding.

Targets werden durch `target_type` bestimmt, nicht durch frei vertrauten Host/Port:

```text
ssh_local -> 127.0.0.1:22
rdp_local -> 127.0.0.1:3389
```

Helper bindet nur Loopback. Kein `0.0.0.0`.

Tunnel Tickets:

- >= 256 Bit zufällig.
- TTL z. B. 60 s bis Erstverbindung.
- single-use.
- an User, Device, Target und Tunnel-ID gebunden.
- serverseitig gehasht.

## 13. File Security

- Canonical Path Checks.
- Symlink/Junction-Angriffe berücksichtigen.
- keine Shell-Kommandos zur Dateiverwaltung zusammensetzen.
- temporäre Uploaddateien mit restriktiven Rechten.
- optional Malware-Scan Hook, aber nicht Security-Garantie.

## 14. Input Validation

Alle Daten vom Agenten und Browser sind untrusted.

- JSON Schema/strukturierte Typvalidierung.
- Stringlängen.
- Enum-Validierung.
- Pfadvalidierung.
- Integer-Ranges.
- UUID-Format.
- Pagination Limits.
- keine SQL-String-Konkatenation; parametrisierte Queries.

## 15. Web Security

- CSP.
- HSTS.
- `X-Content-Type-Options: nosniff`.
- Frame-Schutz via CSP `frame-ancestors`.
- sichere Referrer Policy.
- HTML aus Markdown-Hilfe sanitizen.
- keine Secrets in Frontend-Bundle.

## 16. Audit Integrity

Audit-Benutzeroberfläche darf keine normale Delete-Funktion haben.
Zusätzlich per DB-Trigger als append-only erzwungen (kein UPDATE/DELETE
für die normale Anwendungsrolle, migrations/0003_db_roles.sql).

Implementiert: Hashchain pro Audit-Entry. Jeder Eintrag speichert
`entry_hash = SHA256(prev_hash || Felder dieses Eintrags)` — berechnet
in Go beim Schreiben (`internal/audit.Logger.Record`), nicht per
DB-Trigger, damit Schreib- und Prüfpfad nie unterschiedliche
Serialisierungen verwenden können. `POST /audit/verify` (docs/API.md
§15) rechnet die Kette komplett neu und erkennt so auch eine Änderung,
die den Append-only-Trigger umgeht (direkter DB-Zugriff mit
ausreichenden Rechten) — das ist die verbleibende Bedrohung, gegen die
der Trigger allein nicht schützt. Einträge von vor Einführung dieses
Features haben keinen Hash und werden als "pre-chain" ausgewiesen, nicht
als Manipulation.

Noch offen (optional):

- täglicher Signatur-/Checkpoint-Hash (z.B. extern notarisiert).
- zusätzliches externes Write-Only Log-Ziel.

## 17. Update Supply Chain

- Build reproduzierbar soweit möglich.
- Release Signing Key getrennt.
- Signatur auf Manifest/Artefakt.
- Hash prüfen.
- kein Auto-Update von einer nicht signierten URL.
- Abhängigkeiten pinnen.
- SCA/Dependency Scan.
- Secret Scan.
- SBOM pro Release erwägen.

## 18. Agent Service Rechte

Der Agent benötigt für manche Funktionen hohe OS-Rechte. Trotzdem:

- Netzwerkparser und Remote-Protokollcode so klein wie möglich halten.
- OS-spezifische privilegierte Aktionen kapseln.
- wo möglich least privilege.
- keine unnötigen Shell-Aufrufe.
- Argumentarrays statt String-Shells.

Bei Linux kann langfristig ein unprivilegierter Hauptagent + kleiner privilegierter Helper sinnvoll sein. Für MVP ist ein sauber gehärteter Root-Service möglich, sofern Scope strikt bleibt.

## 19. Security Test Gates

Vor Release:

- Auth Bypass Tests.
- IDOR/Scope Tests.
- CSRF/XSS.
- SQL Injection.
- Path Traversal/Symlink Escape.
- Ticket Replay.
- Enrollment Race.
- Device Spoofing.
- Privilege Expiry.
- Tunnel Target Escape.
- Update Signature Tampering.
- malformed/fuzzed protocol frames.
- dependency/secret scans.

## 20. Incident Response

Muss möglich sein:

- einzelnen Device Credential sofort revoken.
- alle Sessions eines Users invalidieren.
- User sperren.
- Enrollment Tokens global widerrufen.
- Agent-Version als blockiert markieren.
- Security Banner/Notfallhinweis im Dashboard.
- Audit exportieren.

## 21. Responsible Disclosure

Für spätere öffentliche Nutzung `SECURITY.md` mit Kontaktweg, unterstützten Versionen und Meldeprozess pflegen.

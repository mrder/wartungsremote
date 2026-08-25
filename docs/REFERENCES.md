# WartungsRemote – Sicherheits- und Standardreferenzen

Diese Quellen dienen der Implementierung als technische Referenz. Bei Änderungen der Empfehlungen sollen Security-Parameter vor einem Release erneut geprüft werden.

## OWASP

- Password Storage Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
- Session Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- Authentication Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html
- CSRF Prevention Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html
- File Upload Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
- WebSocket Security Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html

## IETF / RFC

- TLS 1.3, RFC 8446 und nachfolgende Aktualisierungen: https://www.rfc-editor.org/rfc/rfc8446
- Ed25519/EdDSA, RFC 8032: https://www.rfc-editor.org/rfc/rfc8032
- TOTP, RFC 6238: https://www.rfc-editor.org/rfc/rfc6238

## Grundregel

Keine eigene Kryptographie entwickeln. Kryptografische Primitive ausschließlich über etablierte Standardbibliotheken verwenden.

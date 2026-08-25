# Security Policy

WartungsRemote is a high-privilege remote administration system. A flaw here
can expose many customer systems at once, so security issues take priority
over convenience — see `docs/SECURITY.md` for the full architecture, threat
model, and non-negotiable security invariants.

## Supported versions

Pre-1.0: only the `main` branch is supported. There are no tagged releases
yet; see `docs/TODO.md` Phase 39 for the V1 release gate.

## Reporting a vulnerability

Do not open a public GitHub issue for security vulnerabilities.

Instead, report it privately to: `<security-contact@example.invalid>`
*(replace with the maintaining organization's real contact before this
project is made public — see `docs/TODO.md` Phase 0)*.

Please include:

- A description of the vulnerability and its impact.
- Steps to reproduce (proof-of-concept code welcome).
- The affected component (`wr-agent`, `wr-core`, `wr-relay`, `wr-helper`,
  `web`) and, if known, the affected commit.

We aim to acknowledge reports within 5 business days.

## Non-negotiable invariants

The following must never be silently weakened (`docs/AI_IMPLEMENTATION_GUIDE.md`
§5). Any change touching these requires explicit review against
`docs/SECURITY.md`:

- No disabling of TLS certificate verification in production code paths.
- No hardcoded secrets or default admin passwords.
- No arbitrary/unbounded TCP forwarding through the relay.
- No credential logging.
- No permanently elevated agent privileges (privilege sessions always expire).
- No enforcement of authorization only in the frontend — the server checks
  every action.
- No bypassing of signed-update verification.

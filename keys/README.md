# Release signing key

`release.pub` is the **public** Ed25519 key used to verify signed
WartungsRemote release artifacts (agent self-updates and manual server
downloads) — see `docs/AGENT.md` §15 and `README.md` → "Signing agent
releases".

The corresponding **private** key never touches this repository, any
server, or any agent. It is held offline and used only with
`cmd/wr-release-sign` to sign new releases before publishing them.

## Verifying a downloaded release yourself

```bash
go build -o wr-release-sign ./cmd/wr-release-sign
sha256sum wr-agent-windows-amd64.exe   # compare against the release notes
# then confirm the signature in the release notes was produced by release.pub
# (this is exactly what the agent does automatically before self-updating)
```

## Configuring a server to trust this key

```bash
export WR_RELEASE_PUBLIC_KEY_FILE=/path/to/keys/release.pub
```

## Configuring an agent build to trust this key

```bash
go build -ldflags "-X wartungsremote/internal/agentcore.ReleasePublicKeyHex=$(xxd -p keys/release.pub | tr -d '\n')" -o wr-agent ./cmd/wr-agent
```

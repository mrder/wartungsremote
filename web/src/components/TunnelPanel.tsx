import { useState } from 'react'
import { TunnelApi, SessionApi, ApiError, type TunnelCreated } from '../api'
import { useAuth } from '../AuthContext'

interface Props {
  deviceId: string
  osFamily: string
}

export default function TunnelPanel({ deviceId, osFamily }: Props) {
  const { user } = useAuth()
  const [tunnel, setTunnel] = useState<TunnelCreated | null>(null)
  const [kind, setKind] = useState<'ssh_local' | 'rdp_local' | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const canSSH = user?.permissions.includes('remote.tunnel.ssh')
  const canRDP = user?.permissions.includes('remote.tunnel.rdp')
  if (!canSSH && !canRDP) return null

  async function open(target: 'ssh_local' | 'rdp_local') {
    setBusy(true)
    setError('')
    try {
      const created = await TunnelApi.create(deviceId, target)
      setTunnel(created)
      setKind(target)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to open tunnel')
    } finally {
      setBusy(false)
    }
  }

  async function close() {
    if (!tunnel) return
    try {
      await SessionApi.close(tunnel.session_id)
    } catch {
      // best effort
    }
    setTunnel(null)
    setKind(null)
  }

  const helperCmd = tunnel ? `wr-helper --server ${user?.public_base_url ?? '<server-url>'} --ticket ${tunnel.helper_ticket}` : ''

  return (
    <div className="tunnel-panel">
      <h3>Native SSH / RDP access</h3>
      {!tunnel && (
        <div className="toolbar">
          {canSSH && <button disabled={busy} onClick={() => open('ssh_local')}>Open SSH Tunnel</button>}
          {canRDP && osFamily === 'windows' && <button disabled={busy} onClick={() => open('rdp_local')}>Open RDP Tunnel</button>}
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {tunnel && (
        <div className="enrollment-panel">
          <p>
            {kind === 'ssh_local' ? 'SSH' : 'RDP'} tunnel ready (ticket is single-use, expires{' '}
            {new Date(tunnel.expires_at).toLocaleTimeString()}). Run this on your own machine:
          </p>
          <code>{helperCmd}</code>
          <p>
            Then point your {kind === 'ssh_local' ? 'ssh client' : 'RDP client (mstsc)'} at the loopback
            port wr-helper prints — the existing {kind === 'ssh_local' ? 'SSH (22)' : 'RDP (3389)'} service
            on the target is untouched.
          </p>
          <button onClick={close}>Close Tunnel</button>
        </div>
      )}
    </div>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TunnelApi, SessionApi, ApiError, type TunnelCreated } from '../api'
import { useAuth } from '../AuthContext'

interface Props {
  deviceId: string
  osFamily: string
}

export default function TunnelPanel({ deviceId, osFamily }: Props) {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('tunnelPanel.openFailed'))
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
      <h3>{t('tunnelPanel.title')}</h3>
      <p>{t('tunnelPanel.hint')}</p>
      {!tunnel && (
        <div className="toolbar">
          {canSSH && <button disabled={busy} onClick={() => open('ssh_local')}>{t('tunnelPanel.openSsh')}</button>}
          {canRDP && osFamily === 'windows' && <button disabled={busy} onClick={() => open('rdp_local')}>{t('tunnelPanel.openRdp')}</button>}
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {tunnel && (
        <div className="enrollment-panel">
          <p>
            {t('tunnelPanel.tunnelReady', {
              kind: kind === 'ssh_local' ? 'SSH' : 'RDP',
              time: new Date(tunnel.expires_at).toLocaleTimeString(),
            })}
          </p>
          <code>{helperCmd}</code>
          <p>
            {t('tunnelPanel.pointClient', {
              client: kind === 'ssh_local' ? t('tunnelPanel.sshClient') : t('tunnelPanel.rdpClient'),
              service: kind === 'ssh_local' ? 'SSH (22)' : 'RDP (3389)',
            })}
          </p>
          <button onClick={close}>{t('tunnelPanel.closeTunnel')}</button>
        </div>
      )}
    </div>
  )
}

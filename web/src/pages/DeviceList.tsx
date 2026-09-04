import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { DeviceApi, EnrollmentApi, ApiError, type Device, type OutstandingEnrollment } from '../api'
import { useAuth } from '../AuthContext'
import StatusBadge from '../components/StatusBadge'

export default function DeviceList() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [enrollment, setEnrollment] = useState<{ token: string; expires_at: string } | null>(null)
  const [enrollBusy, setEnrollBusy] = useState(false)
  const [revokeMsg, setRevokeMsg] = useState('')
  const [installOS, setInstallOS] = useState<'linux' | 'windows'>('linux')
  const [installChannel, setInstallChannel] = useState<'stable' | 'beta'>('stable')
  const [copied, setCopied] = useState(false)
  const [reusable, setReusable] = useState(false)
  const [outstanding, setOutstanding] = useState<OutstandingEnrollment[]>([])
  const [showOutstanding, setShowOutstanding] = useState(false)
  const [showRevoked, setShowRevoked] = useState(false)

  async function load() {
    setLoading(true)
    setError('')
    try {
      const params: Record<string, string> = {}
      if (query) params.q = query
      setDevices((await DeviceApi.list(params)) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceList.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function createEnrollment() {
    setEnrollBusy(true)
    try {
      // Reusable tokens are meant for a staged bulk rollout, so they get
      // a much longer default validity (30 days) than a single-use one
      // (30 minutes) — matches the server-side ceiling per kind.
      const expiresInSeconds = reusable ? 30 * 24 * 3600 : 1800
      const created = await EnrollmentApi.create('', expiresInSeconds, reusable)
      setEnrollment(created)
      loadOutstanding()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceList.createEnrollmentFailed'))
    } finally {
      setEnrollBusy(false)
    }
  }

  async function loadOutstanding() {
    try {
      setOutstanding((await EnrollmentApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceList.loadTokensFailed'))
    }
  }

  async function revokeOne(id: string) {
    try {
      await EnrollmentApi.revoke(id)
      loadOutstanding()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceList.revokeTokenFailed'))
    }
  }

  function installCommand(os: 'linux' | 'windows', channel: 'stable' | 'beta', token: string): string {
    const serverUrl = user?.public_base_url || window.location.origin
    const repo = 'mrder/wartungsremote'
    if (os === 'linux') {
      return `curl -fsSL https://raw.githubusercontent.com/${repo}/main/scripts/quickinstall-agent-linux.sh | sudo bash -s -- --server-url ${serverUrl} --token ${token} --channel ${channel}`
    }
    return [
      `$s = New-TemporaryFile | Rename-Item -NewName { $_.Name + ".ps1" } -PassThru`,
      `Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/${repo}/main/scripts/quickinstall-agent-windows.ps1" -OutFile $s`,
      `powershell -ExecutionPolicy Bypass -File $s -ServerUrl "${serverUrl}" -Token "${token}" -Channel "${channel}"`,
    ].join('\n')
  }

  async function copyInstallCommand() {
    if (!enrollment) return
    try {
      await navigator.clipboard.writeText(installCommand(installOS, installChannel, enrollment.token))
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard API unavailable (e.g. non-HTTPS context) — command is still visible to copy manually
    }
  }

  async function revokeAllEnrollments() {
    try {
      const res = await EnrollmentApi.revokeAll()
      setRevokeMsg(t('deviceList.revokedAll', { count: res.revoked_count }))
      loadOutstanding()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceList.revokeAllFailed'))
    }
  }

  const counts = {
    online: devices.filter((d) => d.status === 'online').length,
    offline: devices.filter((d) => d.status === 'offline' || d.status === 'connection_lost').length,
    warning: devices.filter((d) => d.health === 'warning').length,
    critical: devices.filter((d) => d.health === 'critical').length,
  }
  const visibleDevices = showRevoked ? devices : devices.filter((d) => d.status !== 'revoked')
  const revokedCount = devices.filter((d) => d.status === 'revoked').length

  return (
    <>
      <h1>{t('deviceList.title')}</h1>
      <p>{t('deviceList.intro')}</p>
      <div className="stat-row">
        <div className="stat">{t('deviceList.online')}<strong>{counts.online}</strong></div>
        <div className="stat">{t('deviceList.offline')}<strong>{counts.offline}</strong></div>
        <div className="stat warn">{t('deviceList.warning')}<strong>{counts.warning}</strong></div>
        <div className="stat crit">{t('deviceList.critical')}<strong>{counts.critical}</strong></div>
      </div>

      <div className="toolbar">
        <input placeholder={t('deviceList.searchPlaceholder')} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load()} />
        <button onClick={load}>{t('common.search')}</button>
        {user?.permissions.includes('enrollment.create') && (
          <>
            <label>
              <input type="checkbox" checked={reusable} onChange={(e) => setReusable(e.target.checked)} />
              {' '}{t('deviceList.reusableToken')}
            </label>
            <button onClick={createEnrollment} disabled={enrollBusy}>{t('deviceList.addDevice')}</button>
            <button onClick={() => { setShowOutstanding(!showOutstanding); if (!showOutstanding) loadOutstanding() }}>
              {showOutstanding ? t('deviceList.hideOutstanding') : t('deviceList.showOutstanding')}
            </button>
          </>
        )}
        {user?.permissions.includes('credential.revoke') && (
          <button onClick={revokeAllEnrollments}>{t('deviceList.revokeAllTokens')}</button>
        )}
      </div>
      {revokeMsg && <p className="success">{revokeMsg}</p>}

      {showOutstanding && (
        <table className="device-table">
          <thead><tr><th>{t('deviceList.type')}</th><th>{t('deviceList.uses')}</th><th>{t('deviceList.lastUsed')}</th><th>{t('deviceList.expires')}</th><th></th></tr></thead>
          <tbody>
            {outstanding.map((tok) => (
              <tr key={tok.ID}>
                <td>{tok.IsReusable ? t('deviceList.reusable') : t('deviceList.singleUse')}</td>
                <td>{tok.UseCount}</td>
                <td>{tok.LastUsedAt ? new Date(tok.LastUsedAt).toLocaleString() : '-'}</td>
                <td>{new Date(tok.ExpiresAt).toLocaleString()}</td>
                <td><button onClick={() => revokeOne(tok.ID)}>{t('common.revoke')}</button></td>
              </tr>
            ))}
            {outstanding.length === 0 && <tr><td colSpan={5}>{t('deviceList.noOutstandingTokens')}</td></tr>}
          </tbody>
        </table>
      )}

      {enrollment && (
        <div className="enrollment-panel">
          <p>
            {reusable ? t('deviceList.newReusableToken') : t('deviceList.newToken')}{' '}
            {new Date(enrollment.expires_at).toLocaleString()}
            {reusable ? ` — ${t('deviceList.reusableUntilThen')}` : ''}. {t('deviceList.runOnTarget')}
          </p>
          <div className="toolbar" style={{ marginBottom: '0.5rem' }}>
            <button onClick={() => setInstallOS('linux')} disabled={installOS === 'linux'}>Linux</button>
            <button onClick={() => setInstallOS('windows')} disabled={installOS === 'windows'}>Windows</button>
            <button onClick={() => setInstallChannel('stable')} disabled={installChannel === 'stable'}>{t('deviceList.stable')}</button>
            <button onClick={() => setInstallChannel('beta')} disabled={installChannel === 'beta'}>{t('deviceList.beta')}</button>
            <button onClick={copyInstallCommand}>{copied ? t('deviceList.copied') : t('deviceList.copyCommand')}</button>
          </div>
          <code style={{ whiteSpace: 'pre-wrap', display: 'block' }}>{installCommand(installOS, installChannel, enrollment.token)}</code>
          {installOS === 'windows' && (
            <p style={{ marginTop: '0.5rem' }}>{t('deviceList.windowsInstallerHint')}</p>
          )}
          <p style={{ marginTop: '0.75rem' }}>
            {t('deviceList.rawToken')}: <code>{enrollment.token}</code>
          </p>
          <button onClick={() => setEnrollment(null)}>{t('common.close')}</button>
        </div>
      )}

      {error && <p className="error">{error}</p>}
      {revokedCount > 0 && (
        <label style={{ display: 'block', marginBottom: '0.5rem' }}>
          <input type="checkbox" checked={showRevoked} onChange={(e) => setShowRevoked(e.target.checked)} />
          {' '}{t('deviceList.showRevoked', { count: revokedCount })}
        </label>
      )}
      {loading ? (
        <p>{t('common.loading')}</p>
      ) : (
        <table className="device-table">
          <thead>
            <tr>
              <th>{t('deviceList.name')}</th><th>{t('deviceList.hostname')}</th><th>{t('deviceList.os')}</th><th>{t('deviceList.status')}</th><th>{t('deviceList.health')}</th><th>{t('deviceList.lastSeen')}</th>
            </tr>
          </thead>
          <tbody>
            {visibleDevices.map((d) => (
              <tr key={d.id}>
                <td><Link to={`/devices/${d.id}`}>{d.display_name}</Link></td>
                <td>{d.hostname}</td>
                <td>{d.os_family} {d.os_version}</td>
                <td><StatusBadge kind="status" value={d.status} /></td>
                <td><StatusBadge kind="health" value={d.health} /></td>
                <td>{d.last_seen_at ? new Date(d.last_seen_at).toLocaleString() : '-'}</td>
              </tr>
            ))}
            {visibleDevices.length === 0 && (
              <tr><td colSpan={6}>{t('deviceList.noDevices')}</td></tr>
            )}
          </tbody>
        </table>
      )}
    </>
  )
}

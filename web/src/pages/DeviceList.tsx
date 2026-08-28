import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { DeviceApi, EnrollmentApi, ApiError, type Device, type OutstandingEnrollment } from '../api'
import { useAuth } from '../AuthContext'
import StatusBadge from '../components/StatusBadge'
import AlertsBadge from '../components/AlertsBadge'

export default function DeviceList() {
  const { user, logout } = useAuth()
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

  async function load() {
    setLoading(true)
    setError('')
    try {
      const params: Record<string, string> = {}
      if (query) params.q = query
      setDevices((await DeviceApi.list(params)) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load devices')
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
      setError(err instanceof ApiError ? err.message : 'Failed to create enrollment token')
    } finally {
      setEnrollBusy(false)
    }
  }

  async function loadOutstanding() {
    try {
      setOutstanding((await EnrollmentApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load enrollment tokens')
    }
  }

  async function revokeOne(id: string) {
    try {
      await EnrollmentApi.revoke(id)
      loadOutstanding()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to revoke enrollment token')
    }
  }

  function installCommand(os: 'linux' | 'windows', channel: 'stable' | 'beta', token: string): string {
    const serverUrl = user?.public_base_url || window.location.origin
    const repo = 'mrder/wartungsremote'
    if (os === 'linux') {
      return `curl -fsSL https://raw.githubusercontent.com/${repo}/main/scripts/quickinstall-agent-linux.sh | sudo bash -s -- --server-url ${serverUrl} --token ${token} --channel ${channel}`
    }
    return [
      `$s = New-TemporaryFile`,
      `Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/${repo}/main/scripts/quickinstall-agent-windows.ps1" -OutFile $s`,
      `& $s -ServerUrl "${serverUrl}" -Token "${token}" -Channel "${channel}"`,
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
      setRevokeMsg(`Revoked ${res.revoked_count} outstanding enrollment token(s).`)
      loadOutstanding()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to revoke enrollment tokens')
    }
  }

  const counts = {
    online: devices.filter((d) => d.status === 'online').length,
    offline: devices.filter((d) => d.status === 'offline' || d.status === 'connection_lost').length,
    warning: devices.filter((d) => d.health === 'warning').length,
    critical: devices.filter((d) => d.health === 'critical').length,
  }

  return (
    <div className="page">
      <header className="topbar">
        <h1>WartungsRemote</h1>
        <div>
          <Link to="/customers">Customers</Link>
          {user?.permissions.includes('agent.update') && <Link to="/releases">Releases</Link>}
          {user?.permissions.includes('user.manage') && <Link to="/users">Users</Link>}
          {user?.permissions.includes('system.settings') && <Link to="/settings">Settings</Link>}
          <Link to="/help">Help</Link>
          <AlertsBadge />
          <Link to="/account">{user?.username}</Link>
          <button onClick={logout}>Logout</button>
        </div>
      </header>

      <div className="stat-row">
        <div className="stat">Online<strong>{counts.online}</strong></div>
        <div className="stat">Offline<strong>{counts.offline}</strong></div>
        <div className="stat warn">Warning<strong>{counts.warning}</strong></div>
        <div className="stat crit">Critical<strong>{counts.critical}</strong></div>
      </div>

      <div className="toolbar">
        <input placeholder="Search devices..." value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load()} />
        <button onClick={load}>Search</button>
        {user?.permissions.includes('enrollment.create') && (
          <>
            <label>
              <input type="checkbox" checked={reusable} onChange={(e) => setReusable(e.target.checked)} />
              {' '}Reusable (site) token
            </label>
            <button onClick={createEnrollment} disabled={enrollBusy}>+ Add Device</button>
            <button onClick={() => { setShowOutstanding(!showOutstanding); if (!showOutstanding) loadOutstanding() }}>
              {showOutstanding ? 'Hide' : 'Show'} outstanding tokens
            </button>
          </>
        )}
        {user?.permissions.includes('credential.revoke') && (
          <button onClick={revokeAllEnrollments}>Revoke all enrollment tokens</button>
        )}
      </div>
      {revokeMsg && <p>{revokeMsg}</p>}

      {showOutstanding && (
        <table className="device-table">
          <thead><tr><th>Type</th><th>Uses</th><th>Last used</th><th>Expires</th><th></th></tr></thead>
          <tbody>
            {outstanding.map((t) => (
              <tr key={t.ID}>
                <td>{t.IsReusable ? 'Reusable' : 'Single-use'}</td>
                <td>{t.UseCount}</td>
                <td>{t.LastUsedAt ? new Date(t.LastUsedAt).toLocaleString() : '-'}</td>
                <td>{new Date(t.ExpiresAt).toLocaleString()}</td>
                <td><button onClick={() => revokeOne(t.ID)}>Revoke</button></td>
              </tr>
            ))}
            {outstanding.length === 0 && <tr><td colSpan={5}>No outstanding enrollment tokens.</td></tr>}
          </tbody>
        </table>
      )}

      {enrollment && (
        <div className="enrollment-panel">
          <p>
            New {reusable ? 'reusable (site) ' : ''}enrollment token — shown once, expires{' '}
            {new Date(enrollment.expires_at).toLocaleString()}
            {reusable ? ' — can install any number of devices until then' : ''}. Run this on the target device
            (as Administrator/root) to install and enroll it in one step:
          </p>
          <div className="toolbar" style={{ marginBottom: '0.5rem' }}>
            <button onClick={() => setInstallOS('linux')} disabled={installOS === 'linux'}>Linux</button>
            <button onClick={() => setInstallOS('windows')} disabled={installOS === 'windows'}>Windows</button>
            <button onClick={() => setInstallChannel('stable')} disabled={installChannel === 'stable'}>Stable</button>
            <button onClick={() => setInstallChannel('beta')} disabled={installChannel === 'beta'}>Beta</button>
            <button onClick={copyInstallCommand}>{copied ? 'Copied!' : 'Copy command'}</button>
          </div>
          <code style={{ whiteSpace: 'pre-wrap', display: 'block' }}>{installCommand(installOS, installChannel, enrollment.token)}</code>
          <p style={{ marginTop: '0.75rem' }}>
            Raw token, if you'd rather install manually: <code>{enrollment.token}</code>
          </p>
          <button onClick={() => setEnrollment(null)}>Close</button>
        </div>
      )}

      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>Loading...</p>
      ) : (
        <table className="device-table">
          <thead>
            <tr>
              <th>Name</th><th>Hostname</th><th>OS</th><th>Status</th><th>Health</th><th>Last seen</th>
            </tr>
          </thead>
          <tbody>
            {devices.map((d) => (
              <tr key={d.id}>
                <td><Link to={`/devices/${d.id}`}>{d.display_name}</Link></td>
                <td>{d.hostname}</td>
                <td>{d.os_family} {d.os_version}</td>
                <td><StatusBadge kind="status" value={d.status} /></td>
                <td><StatusBadge kind="health" value={d.health} /></td>
                <td>{d.last_seen_at ? new Date(d.last_seen_at).toLocaleString() : '-'}</td>
              </tr>
            ))}
            {devices.length === 0 && (
              <tr><td colSpan={6}>No devices yet.</td></tr>
            )}
          </tbody>
        </table>
      )}
    </div>
  )
}

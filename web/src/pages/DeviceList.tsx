import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { DeviceApi, EnrollmentApi, ApiError, type Device } from '../api'
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
      const created = await EnrollmentApi.create('', 1800)
      setEnrollment(created)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create enrollment token')
    } finally {
      setEnrollBusy(false)
    }
  }

  async function revokeAllEnrollments() {
    try {
      const res = await EnrollmentApi.revokeAll()
      setRevokeMsg(`Revoked ${res.revoked_count} outstanding enrollment token(s).`)
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
          <Link to="/help">Help</Link>
          <AlertsBadge />
          <span>{user?.username}</span>
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
          <button onClick={createEnrollment} disabled={enrollBusy}>+ Add Device</button>
        )}
        {user?.permissions.includes('credential.revoke') && (
          <button onClick={revokeAllEnrollments}>Revoke all enrollment tokens</button>
        )}
      </div>
      {revokeMsg && <p>{revokeMsg}</p>}

      {enrollment && (
        <div className="enrollment-panel">
          <p>New enrollment token (shown once, expires {new Date(enrollment.expires_at).toLocaleString()}):</p>
          <code>{enrollment.token}</code>
          <p>Install the agent on the target device and provide this token as its one-time enrollment token (see docs/AGENT.md).</p>
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

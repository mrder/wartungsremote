import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { UserApi, ApiError, type AdminUser } from '../api'
import { useAuth } from '../AuthContext'

export default function Users() {
  const { user: me } = useAuth()
  const [users, setUsers] = useState<AdminUser[]>([])
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    try {
      setUsers((await UserApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load users')
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function setStatus(id: string, status: 'active' | 'disabled' | 'locked') {
    try {
      await UserApi.setStatus(id, status)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update user')
    }
  }

  async function revokeSessions(id: string) {
    try {
      await UserApi.revokeSessions(id)
      setMsg('All sessions for this user were revoked.')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to revoke sessions')
    }
  }

  async function toggleMfa(id: string, mfaRequired: boolean) {
    try {
      await UserApi.setMfaRequired(id, mfaRequired)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update MFA requirement')
    }
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Users</h1>
      {error && <p className="error">{error}</p>}
      {msg && <p>{msg}</p>}

      <table className="device-table">
        <thead><tr><th>Username</th><th>Display name</th><th>Status</th><th>MFA required</th><th>Last login</th><th></th></tr></thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id}>
              <td>{u.username}</td>
              <td>{u.display_name || '-'}</td>
              <td>{u.status}</td>
              <td>
                <label>
                  <input type="checkbox" checked={u.mfa_required} onChange={(e) => toggleMfa(u.id, e.target.checked)} />
                </label>
              </td>
              <td>{u.last_login_at ? new Date(u.last_login_at).toLocaleString() : '-'}</td>
              <td>
                {u.id !== me?.id && (
                  <>
                    {u.status !== 'active' && <button onClick={() => setStatus(u.id, 'active')}>Reactivate</button>}
                    {u.status !== 'disabled' && <button onClick={() => setStatus(u.id, 'disabled')}>Disable</button>}
                    {u.status !== 'locked' && <button onClick={() => setStatus(u.id, 'locked')}>Lock</button>}
                  </>
                )}
                <button onClick={() => revokeSessions(u.id)}>Revoke all sessions</button>
              </td>
            </tr>
          ))}
          {users.length === 0 && <tr><td colSpan={6}>No users.</td></tr>}
        </tbody>
      </table>
      <p style={{ marginTop: '0.75rem', color: 'var(--muted)' }}>
        MFA required is per-user. The server also has a global default (<code>admin.require_mfa</code> in
        server.yaml) — if that is <code>true</code>, MFA is enforced regardless of this checkbox; both must be off
        for a user to skip MFA entirely.
      </p>
    </div>
  )
}

import { useEffect, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { UserApi, ApiError, type AdminUser } from '../api'
import { useAuth } from '../AuthContext'

export default function Users() {
  const { t } = useTranslation()
  const { user: me } = useAuth()
  const [users, setUsers] = useState<AdminUser[]>([])
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')

  const [newUsername, setNewUsername] = useState('')
  const [newDisplayName, setNewDisplayName] = useState('')
  const [newRole, setNewRole] = useState('technician')
  const [createBusy, setCreateBusy] = useState(false)
  const [createdUser, setCreatedUser] = useState<{ username: string; password: string } | null>(null)

  async function load() {
    try {
      setUsers((await UserApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('users.loadFailed'))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function setStatus(id: string, status: 'active' | 'disabled' | 'locked') {
    try {
      await UserApi.setStatus(id, status)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('users.updateFailed'))
    }
  }

  async function revokeSessions(id: string) {
    try {
      await UserApi.revokeSessions(id)
      setMsg(t('users.sessionsRevoked'))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('users.revokeSessionsFailed'))
    }
  }

  async function createUser(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setCreateBusy(true)
    try {
      const result = await UserApi.create(newUsername, newDisplayName, newRole)
      setCreatedUser({ username: result.username, password: result.password })
      setNewUsername('')
      setNewDisplayName('')
      setNewRole('technician')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('users.createFailed'))
    } finally {
      setCreateBusy(false)
    }
  }

  async function toggleMfa(id: string, mfaRequired: boolean) {
    try {
      await UserApi.setMfaRequired(id, mfaRequired)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('users.updateMfaFailed'))
    }
  }

  return (
    <>
      <h1>{t('users.title')}</h1>
      <p>{t('users.intro')}</p>
      {error && <p className="error">{error}</p>}
      {msg && <p className="success">{msg}</p>}

      {createdUser && (
        <div className="enrollment-panel">
          <p>{t('users.createdHint', { username: createdUser.username })}</p>
          <code>{createdUser.password}</code>
          <p>{t('users.createdMfaHint')}</p>
          <button onClick={() => setCreatedUser(null)}>{t('common.close')}</button>
        </div>
      )}

      <h3>{t('users.addUser')}</h3>
      <p>
        <strong>{t('users.roleReadOnly')}:</strong> {t('users.roleReadOnlyHint')}<br />
        <strong>{t('users.roleTechnician')}:</strong> {t('users.roleTechnicianHint')}<br />
        <strong>{t('users.roleAdmin')}:</strong> {t('users.roleAdminHint')}<br />
        <strong>{t('users.roleSuperAdmin')}:</strong> {t('users.roleSuperAdminHint')}
      </p>
      <form onSubmit={createUser} className="field-form">
        <label>
          {t('users.username')}
          <input value={newUsername} onChange={(e) => setNewUsername(e.target.value)} required />
        </label>
        <label>
          {t('users.displayName')}
          <input value={newDisplayName} onChange={(e) => setNewDisplayName(e.target.value)} />
        </label>
        <label>
          {t('users.role')}
          <select value={newRole} onChange={(e) => setNewRole(e.target.value)}>
            <option value="read_only">{t('users.roleReadOnly')}</option>
            <option value="technician">{t('users.roleTechnician')}</option>
            <option value="admin">{t('users.roleAdmin')}</option>
            <option value="super_admin">{t('users.roleSuperAdmin')}</option>
          </select>
        </label>
        <button type="submit" disabled={createBusy}>{createBusy ? t('common.loading') : t('users.addUser')}</button>
      </form>

      <table className="device-table">
        <thead><tr><th>{t('users.username')}</th><th>{t('users.displayName')}</th><th>{t('deviceList.status')}</th><th>{t('users.mfaRequired')}</th><th>{t('users.lastLogin')}</th><th></th></tr></thead>
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
                    {u.status !== 'active' && <button onClick={() => setStatus(u.id, 'active')}>{t('users.reactivate')}</button>}
                    {u.status !== 'disabled' && <button onClick={() => setStatus(u.id, 'disabled')}>{t('users.disable')}</button>}
                    {u.status !== 'locked' && <button onClick={() => setStatus(u.id, 'locked')}>{t('users.lock')}</button>}
                  </>
                )}
                <button onClick={() => revokeSessions(u.id)}>{t('users.revokeAllSessions')}</button>
              </td>
            </tr>
          ))}
          {users.length === 0 && <tr><td colSpan={6}>{t('users.noUsers')}</td></tr>}
        </tbody>
      </table>
      <p style={{ marginTop: '0.75rem', color: 'var(--muted)' }}>
        <Trans i18nKey="users.mfaHint"><code>admin.require_mfa</code><code>true</code></Trans>
      </p>
    </>
  )
}

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ReauthApi, AccountApi, ApiError } from '../api'
import { useAuth } from '../AuthContext'

export default function Account() {
  const { user } = useAuth()
  const [currentPassword, setCurrentPassword] = useState('')
  const [code, setCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setDone(false)
    if (newPassword.length < 12) {
      setError('New password must be at least 12 characters.')
      return
    }
    if (newPassword !== confirmPassword) {
      setError('New password and confirmation do not match.')
      return
    }
    setBusy(true)
    try {
      const reauth = await ReauthApi.reauth(currentPassword, code)
      await AccountApi.changePassword(reauth.reauth_id, newPassword)
      setCurrentPassword('')
      setCode('')
      setNewPassword('')
      setConfirmPassword('')
      setDone(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to change password')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>My account</h1>
      <p>Signed in as <code>{user?.username}</code>.</p>

      <h3>Change password</h3>
      <p>
        Requires your current password and a valid MFA code, same as any other sensitive action.
        Other sessions are left alone — this doesn't log you out anywhere else.
      </p>
      {error && <p className="error">{error}</p>}
      {done && <p>Password changed.</p>}
      <form onSubmit={submit} className="toolbar" style={{ flexWrap: 'wrap' }}>
        <input
          type="password"
          placeholder="Current password"
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
          required
        />
        <input
          placeholder="MFA code"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          required
          style={{ width: '6rem' }}
        />
        <input
          type="password"
          placeholder="New password (12+ characters)"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          required
        />
        <input
          type="password"
          placeholder="Confirm new password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          required
        />
        <button type="submit" disabled={busy}>{busy ? 'Changing...' : 'Change password'}</button>
      </form>
    </div>
  )
}

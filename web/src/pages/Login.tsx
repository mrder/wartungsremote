import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import QRCode from 'qrcode'
import { AuthApi, ApiError } from '../api'
import { useAuth } from '../AuthContext'

type Stage = 'credentials' | 'mfa' | 'mfa_setup'

export default function Login() {
  const { refresh } = useAuth()
  const navigate = useNavigate()

  const [stage, setStage] = useState<Stage>('credentials')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [challengeId, setChallengeId] = useState('')
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function handleCredentials(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const res = await AuthApi.login(username, password)
      if (res.state === 'mfa_required' && res.challenge_id) {
        setChallengeId(res.challenge_id)
        setStage('mfa')
      } else if (res.state === 'mfa_setup_required' && res.setup_uri) {
        setQrDataUrl(await QRCode.toDataURL(res.setup_uri))
        setStage('mfa_setup')
      } else if (res.state === 'authenticated') {
        await refresh()
        navigate('/')
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Login failed')
    } finally {
      setBusy(false)
    }
  }

  async function handleMfa(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const res = await AuthApi.totp(challengeId, code)
      if (res.state === 'authenticated') {
        await refresh()
        navigate('/')
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Invalid code')
    } finally {
      setBusy(false)
    }
  }

  async function handleMfaSetup(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const res = await AuthApi.confirmMfaSetup(username, password, code)
      setRecoveryCodes(res.recovery_codes)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Invalid code')
    } finally {
      setBusy(false)
    }
  }

  if (recoveryCodes) {
    return (
      <div className="auth-card">
        <h1>Save your recovery codes</h1>
        <p>These are shown only once. Store them somewhere safe.</p>
        <pre className="recovery-codes">{recoveryCodes.join('\n')}</pre>
        <button onClick={async () => { await refresh(); navigate('/') }}>Continue</button>
      </div>
    )
  }

  return (
    <div className="auth-card">
      <h1>WartungsRemote</h1>
      {stage === 'credentials' && (
        <form onSubmit={handleCredentials}>
          <label>Username<input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus required /></label>
          <label>Password<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">Login</button>
        </form>
      )}
      {stage === 'mfa' && (
        <form onSubmit={handleMfa}>
          <p>Enter your 6-digit authenticator code.</p>
          <label>Code<input value={code} onChange={(e) => setCode(e.target.value)} autoFocus required maxLength={6} /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">Verify</button>
        </form>
      )}
      {stage === 'mfa_setup' && (
        <form onSubmit={handleMfaSetup}>
          <p>Two-factor authentication setup is required. Scan this QR code with your authenticator app, then enter a code to confirm.</p>
          {qrDataUrl && <img src={qrDataUrl} alt="TOTP setup QR code" width={200} height={200} />}
          <label>Code<input value={code} onChange={(e) => setCode(e.target.value)} required maxLength={6} /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">Confirm</button>
        </form>
      )}
    </div>
  )
}

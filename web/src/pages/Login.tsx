import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import QRCode from 'qrcode'
import { AuthApi, ApiError } from '../api'
import { useAuth } from '../AuthContext'

type Stage = 'credentials' | 'mfa' | 'mfa_setup'

export default function Login() {
  const { t } = useTranslation()
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const loggedOutForInactivity = (location.state as { reason?: string } | null)?.reason === 'inactivity'

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
      setError(err instanceof ApiError ? err.message : t('login.loginFailed'))
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
      setError(err instanceof ApiError ? err.message : t('login.invalidCode'))
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
      setError(err instanceof ApiError ? err.message : t('login.invalidCode'))
    } finally {
      setBusy(false)
    }
  }

  if (recoveryCodes) {
    return (
      <div className="auth-card">
        <h1>{t('login.saveRecoveryCodes')}</h1>
        <p>{t('login.recoveryCodesHint')}</p>
        <pre className="recovery-codes">{recoveryCodes.join('\n')}</pre>
        <button onClick={async () => { await refresh(); navigate('/') }}>{t('common.continue')}</button>
      </div>
    )
  }

  return (
    <div className="auth-card">
      <h1>WartungsRemote</h1>
      {loggedOutForInactivity && <p>{t('login.loggedOutForInactivity')}</p>}
      {stage === 'credentials' && (
        <form onSubmit={handleCredentials}>
          <label>{t('login.username')}<input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus required /></label>
          <label>{t('login.password')}<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">{t('login.login')}</button>
        </form>
      )}
      {stage === 'mfa' && (
        <form onSubmit={handleMfa}>
          <p>{t('login.enterCode')}</p>
          <label>{t('login.code')}<input value={code} onChange={(e) => setCode(e.target.value)} autoFocus required maxLength={6} /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">{t('login.verify')}</button>
        </form>
      )}
      {stage === 'mfa_setup' && (
        <form onSubmit={handleMfaSetup}>
          <p>{t('login.mfaSetupHint')}</p>
          {qrDataUrl && <img src={qrDataUrl} alt={t('login.qrAlt')} width={200} height={200} />}
          <label>{t('login.code')}<input value={code} onChange={(e) => setCode(e.target.value)} required maxLength={6} /></label>
          {error && <p className="error">{error}</p>}
          <button disabled={busy} type="submit">{t('login.confirm')}</button>
        </form>
      )}
    </div>
  )
}

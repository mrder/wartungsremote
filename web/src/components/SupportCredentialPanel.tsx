import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SupportCredentialApi, ApiError } from '../api'
import { useAuth } from '../AuthContext'

interface Props {
  deviceId: string
}

export default function SupportCredentialPanel({ deviceId }: Props) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [credential, setCredential] = useState<{ username: string; password: string } | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [rotateMsg, setRotateMsg] = useState('')

  const canSSH = user?.permissions.includes('remote.tunnel.ssh')
  const canRDP = user?.permissions.includes('remote.tunnel.rdp')
  if (!canSSH && !canRDP) return null

  async function reveal() {
    setBusy(true)
    setError('')
    try {
      const cred = await SupportCredentialApi.get(deviceId)
      setCredential(cred)
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.status === 404
            ? t('supportCredential.notReported')
            : err.message
          : t('supportCredential.loadFailed')
      )
    } finally {
      setBusy(false)
    }
  }

  async function rotate() {
    setBusy(true)
    setError('')
    setRotateMsg('')
    try {
      await SupportCredentialApi.rotate(deviceId)
      setCredential(null)
      setRotateMsg(t('supportCredential.rotationRequested'))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('supportCredential.rotateFailed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="tunnel-panel">
      <h3>{t('supportCredential.title')}</h3>
      <p>{t('supportCredential.hint')}</p>
      {error && <p className="error">{error}</p>}
      {rotateMsg && <p>{rotateMsg}</p>}
      {!credential ? (
        <div className="toolbar">
          <button disabled={busy} onClick={reveal}>{t('supportCredential.revealPassword')}</button>
          <button disabled={busy} onClick={rotate}>{t('supportCredential.rotatePassword')}</button>
        </div>
      ) : (
        <div className="enrollment-panel">
          <p>
            {t('supportCredential.username')}: <code>{credential.username}</code><br />
            {t('supportCredential.password')}: <code>{credential.password}</code>
          </p>
          <div className="toolbar">
            <button onClick={() => setCredential(null)}>{t('supportCredential.hide')}</button>
            <button disabled={busy} onClick={rotate}>{t('supportCredential.rotatePassword')}</button>
          </div>
        </div>
      )}
    </div>
  )
}

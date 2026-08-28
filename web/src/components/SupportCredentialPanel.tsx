import { useState } from 'react'
import { SupportCredentialApi, ApiError } from '../api'
import { useAuth } from '../AuthContext'

interface Props {
  deviceId: string
}

export default function SupportCredentialPanel({ deviceId }: Props) {
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
            ? 'No remote-support account has been reported by this device yet (it reports one shortly after first connecting).'
            : err.message
          : 'Failed to load credential'
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
      setRotateMsg('Rotation requested — the device will report its new password shortly. Reveal again in a moment.')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to request rotation')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="tunnel-panel">
      <h3>Remote-support account</h3>
      <p>
        A dedicated local account ("remotewartung") for logging into the SSH/RDP tunnel above — separate
        from any of the customer's own accounts. Every reveal is audited.
      </p>
      {error && <p className="error">{error}</p>}
      {rotateMsg && <p>{rotateMsg}</p>}
      {!credential ? (
        <div className="toolbar">
          <button disabled={busy} onClick={reveal}>Reveal password</button>
          <button disabled={busy} onClick={rotate}>Rotate password</button>
        </div>
      ) : (
        <div className="enrollment-panel">
          <p>
            Username: <code>{credential.username}</code><br />
            Password: <code>{credential.password}</code>
          </p>
          <div className="toolbar">
            <button onClick={() => setCredential(null)}>Hide</button>
            <button disabled={busy} onClick={rotate}>Rotate password</button>
          </div>
        </div>
      )}
    </div>
  )
}

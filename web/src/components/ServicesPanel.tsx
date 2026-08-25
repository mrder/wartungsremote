import { useEffect, useState } from 'react'
import { ServiceApi, ApiError, type ServiceInfo } from '../api'

export default function ServicesPanel({ deviceId }: { deviceId: string }) {
  const [services, setServices] = useState<ServiceInfo[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError('')
    try {
      setServices((await ServiceApi.list(deviceId)) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to list services')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function act(name: string, action: 'start' | 'stop' | 'restart') {
    setBusy(name)
    try {
      await ServiceApi.action(deviceId, name, action)
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `Failed to ${action} ${name}`)
    } finally {
      setBusy(null)
    }
  }

  if (loading) return <p>Loading...</p>

  return (
    <div>
      {error && <p className="error">{error}</p>}
      <table className="device-table">
        <thead><tr><th>Name</th><th>Status</th><th></th></tr></thead>
        <tbody>
          {services.map((s) => (
            <tr key={s.name}>
              <td>{s.display_name || s.name}</td>
              <td>{s.status}</td>
              <td>
                <button disabled={busy === s.name} onClick={() => act(s.name, 'start')}>Start</button>{' '}
                <button disabled={busy === s.name} onClick={() => act(s.name, 'stop')}>Stop</button>{' '}
                <button disabled={busy === s.name} onClick={() => act(s.name, 'restart')}>Restart</button>
              </td>
            </tr>
          ))}
          {services.length === 0 && <tr><td colSpan={3}>No services found.</td></tr>}
        </tbody>
      </table>
    </div>
  )
}

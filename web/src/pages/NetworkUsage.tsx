import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { NetworkUsageApi, ApiError, type DeviceNetworkTotal } from '../api'

function formatBytes(n: number): string {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + ' GB'
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
  return n + ' B'
}

export default function NetworkUsage() {
  const [totals, setTotals] = useState<DeviceNetworkTotal[]>([])
  const [hours, setHours] = useState(24)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  async function load() {
    setLoading(true)
    setError('')
    try {
      setTotals((await NetworkUsageApi.summary(hours)) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load network usage')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hours])

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Network usage</h1>
      <p>
        Which devices are using the most bandwidth, ranked by total traffic. "To server" is just
        this agent's own control-channel overhead — everything else on this device's network is
        "total". Based on raw samples, so an overly long window may miss devices' older history
        once it's past the configured raw retention (Settings → Network traffic history retention).
      </p>
      <div className="toolbar">
        <label>
          Window:{' '}
          <select value={hours} onChange={(e) => setHours(Number(e.target.value))}>
            <option value={1}>Last hour</option>
            <option value={24}>Last 24 hours</option>
            <option value={168}>Last 7 days</option>
          </select>
        </label>
      </div>
      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>Loading...</p>
      ) : (
        <table className="device-table">
          <thead>
            <tr>
              <th>Device</th>
              <th>Total sent</th>
              <th>Total received</th>
              <th>Sent to server</th>
              <th>Received from server</th>
            </tr>
          </thead>
          <tbody>
            {totals.map((t) => (
              <tr key={t.device_id}>
                <td><Link to={`/devices/${t.device_id}`}>{t.display_name}</Link></td>
                <td>{formatBytes(t.bytes_sent_total)}</td>
                <td>{formatBytes(t.bytes_recv_total)}</td>
                <td>{formatBytes(t.bytes_sent_control)}</td>
                <td>{formatBytes(t.bytes_recv_control)}</td>
              </tr>
            ))}
            {totals.length === 0 && <tr><td colSpan={5}>No network traffic data in this window yet.</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

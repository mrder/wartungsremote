import { useState } from 'react'
import { LogApi, ApiError, type LogEntry } from '../api'

export default function LogsPanel({ deviceId }: { deviceId: string }) {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [query, setQuery] = useState('')
  const [level, setLevel] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)

  async function load() {
    setLoading(true)
    setError('')
    try {
      const list = await LogApi.query(deviceId, { query, level: level || undefined, limit: 300 })
      setEntries(list ?? [])
      setLoaded(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load logs')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <div className="toolbar">
        <input placeholder="Search message text..." value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load()} />
        <select value={level} onChange={(e) => setLevel(e.target.value)}>
          <option value="">All levels</option>
          <option value="error">Error</option>
          <option value="warning">Warning</option>
          <option value="info">Info</option>
        </select>
        <button onClick={load} disabled={loading}>{loading ? 'Loading...' : 'Query'}</button>
      </div>
      {error && <p className="error">{error}</p>}
      {!loaded && !loading && <p>Click "Query" to fetch recent system logs (journalctl / Event Log).</p>}
      {loaded && (
        <table className="device-table">
          <thead><tr><th>Time</th><th>Level</th><th>Source</th><th>Message</th></tr></thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={i}>
                <td>{new Date(e.time).toLocaleString()}</td>
                <td><span className={`badge badge-${e.level === 'error' ? 'red' : e.level === 'warning' ? 'yellow' : 'gray'}`}>{e.level}</span></td>
                <td>{e.source}</td>
                <td style={{ maxWidth: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={e.message}>{e.message}</td>
              </tr>
            ))}
            {entries.length === 0 && <tr><td colSpan={4}>No log entries found.</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

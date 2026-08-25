import { useEffect, useState } from 'react'
import { ProcessApi, ApiError, type ProcessInfo } from '../api'

export default function ProcessesPanel({ deviceId }: { deviceId: string }) {
  const [processes, setProcesses] = useState<ProcessInfo[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<number | null>(null)

  async function load() {
    setLoading(true)
    setError('')
    try {
      const list = (await ProcessApi.list(deviceId)) ?? []
      list.sort((a, b) => b.cpu_percent - a.cpu_percent)
      setProcesses(list)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to list processes')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function terminate(p: ProcessInfo) {
    if (!confirm(`Terminate process "${p.name}" (PID ${p.pid})?`)) return
    setBusy(p.pid)
    try {
      await ProcessApi.terminate(deviceId, p.pid, p.start_time_unix_ms)
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to terminate process')
    } finally {
      setBusy(null)
    }
  }

  if (loading) return <p>Loading...</p>

  return (
    <div>
      {error && <p className="error">{error}</p>}
      <table className="device-table">
        <thead><tr><th>PID</th><th>Name</th><th>CPU %</th><th>RAM</th><th>User</th><th></th></tr></thead>
        <tbody>
          {processes.slice(0, 200).map((p) => (
            <tr key={p.pid}>
              <td>{p.pid}</td>
              <td>{p.name}</td>
              <td>{p.cpu_percent.toFixed(1)}</td>
              <td>{(p.memory_rss_bytes / 1e6).toFixed(1)} MB</td>
              <td>{p.username || '-'}</td>
              <td><button disabled={busy === p.pid} onClick={() => terminate(p)}>Terminate</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

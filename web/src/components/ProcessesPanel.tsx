import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ProcessApi, ApiError, type ProcessInfo } from '../api'

export default function ProcessesPanel({ deviceId }: { deviceId: string }) {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('processesPanel.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function terminate(p: ProcessInfo) {
    if (!confirm(t('processesPanel.confirmTerminate', { name: p.name, pid: p.pid }))) return
    setBusy(p.pid)
    try {
      await ProcessApi.terminate(deviceId, p.pid, p.start_time_unix_ms)
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('processesPanel.terminateFailed'))
    } finally {
      setBusy(null)
    }
  }

  if (loading) return <p>{t('common.loading')}</p>

  return (
    <div>
      {error && <p className="error">{error}</p>}
      <table className="device-table">
        <thead><tr><th>PID</th><th>{t('deviceList.name')}</th><th>CPU %</th><th>RAM</th><th>{t('processesPanel.user')}</th><th></th></tr></thead>
        <tbody>
          {processes.slice(0, 200).map((p) => (
            <tr key={p.pid}>
              <td>{p.pid}</td>
              <td>{p.name}</td>
              <td>{p.cpu_percent.toFixed(1)}</td>
              <td>{(p.memory_rss_bytes / 1e6).toFixed(1)} MB</td>
              <td>{p.username || '-'}</td>
              <td><button disabled={busy === p.pid} onClick={() => terminate(p)}>{t('processesPanel.terminate')}</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

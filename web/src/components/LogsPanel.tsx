import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LogApi, ApiError, type LogEntry } from '../api'

export default function LogsPanel({ deviceId }: { deviceId: string }) {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('logsPanel.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <div className="toolbar">
        <input placeholder={t('logsPanel.searchPlaceholder')} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load()} />
        <select value={level} onChange={(e) => setLevel(e.target.value)}>
          <option value="">{t('logsPanel.allLevels')}</option>
          <option value="error">{t('logsPanel.error')}</option>
          <option value="warning">{t('logsPanel.warning')}</option>
          <option value="info">{t('logsPanel.info')}</option>
        </select>
        <button onClick={load} disabled={loading}>{loading ? t('common.loading') : t('logsPanel.query')}</button>
      </div>
      {error && <p className="error">{error}</p>}
      {!loaded && !loading && <p>{t('logsPanel.clickToQuery')}</p>}
      {loaded && (
        <table className="device-table">
          <thead><tr><th>{t('deviceDetail.time')}</th><th>{t('logsPanel.level')}</th><th>{t('logsPanel.source')}</th><th>{t('logsPanel.message')}</th></tr></thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={i}>
                <td>{new Date(e.time).toLocaleString()}</td>
                <td><span className={`badge badge-${e.level === 'error' ? 'red' : e.level === 'warning' ? 'yellow' : 'gray'}`}>{e.level}</span></td>
                <td>{e.source}</td>
                <td style={{ maxWidth: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={e.message}>{e.message}</td>
              </tr>
            ))}
            {entries.length === 0 && <tr><td colSpan={4}>{t('logsPanel.noEntries')}</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

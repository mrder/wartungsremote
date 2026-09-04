import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { NetworkUsageApi, ApiError, type DeviceNetworkTotal } from '../api'

function formatBytes(n: number): string {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + ' GB'
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
  return n + ' B'
}

export default function NetworkUsage() {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('networkUsage.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hours])

  return (
    <>
      <h1>{t('networkUsage.title')}</h1>
      <p>{t('networkUsage.hint')}</p>
      <div className="toolbar">
        <label>
          {t('networkUsage.window')}:{' '}
          <select value={hours} onChange={(e) => setHours(Number(e.target.value))}>
            <option value={1}>{t('networkUsage.lastHour')}</option>
            <option value={24}>{t('networkUsage.last24Hours')}</option>
            <option value={168}>{t('networkUsage.last7Days')}</option>
          </select>
        </label>
      </div>
      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>{t('common.loading')}</p>
      ) : (
        <table className="device-table">
          <thead>
            <tr>
              <th>{t('networkUsage.device')}</th>
              <th>{t('networkUsage.totalSent')}</th>
              <th>{t('networkUsage.totalReceived')}</th>
              <th>{t('networkUsage.sentToServer')}</th>
              <th>{t('networkUsage.receivedFromServer')}</th>
            </tr>
          </thead>
          <tbody>
            {totals.map((row) => (
              <tr key={row.device_id}>
                <td><Link to={`/devices/${row.device_id}`}>{row.display_name}</Link></td>
                <td>{formatBytes(row.bytes_sent_total)}</td>
                <td>{formatBytes(row.bytes_recv_total)}</td>
                <td>{formatBytes(row.bytes_sent_control)}</td>
                <td>{formatBytes(row.bytes_recv_control)}</td>
              </tr>
            ))}
            {totals.length === 0 && <tr><td colSpan={5}>{t('networkUsage.noData')}</td></tr>}
          </tbody>
        </table>
      )}
    </>
  )
}

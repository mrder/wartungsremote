import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ServiceApi, ApiError, type ServiceInfo } from '../api'

export default function ServicesPanel({ deviceId }: { deviceId: string }) {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('servicesPanel.loadFailed'))
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
      setError(err instanceof ApiError ? err.message : t('servicesPanel.actionFailed', { action, name }))
    } finally {
      setBusy(null)
    }
  }

  if (loading) return <p>{t('common.loading')}</p>

  return (
    <div>
      {error && <p className="error">{error}</p>}
      <table className="device-table">
        <thead><tr><th>{t('deviceList.name')}</th><th>{t('deviceList.status')}</th><th></th></tr></thead>
        <tbody>
          {services.map((s) => (
            <tr key={s.name}>
              <td>{s.display_name || s.name}</td>
              <td>{s.status}</td>
              <td>
                <button disabled={busy === s.name} onClick={() => act(s.name, 'start')}>{t('servicesPanel.start')}</button>{' '}
                <button disabled={busy === s.name} onClick={() => act(s.name, 'stop')}>{t('servicesPanel.stop')}</button>{' '}
                <button disabled={busy === s.name} onClick={() => act(s.name, 'restart')}>{t('servicesPanel.restart')}</button>
              </td>
            </tr>
          ))}
          {services.length === 0 && <tr><td colSpan={3}>{t('servicesPanel.noServices')}</td></tr>}
        </tbody>
      </table>
    </div>
  )
}

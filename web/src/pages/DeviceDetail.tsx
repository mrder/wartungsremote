import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { DeviceApi, CustomerApi, ReleaseApi, AuditApi, ReauthApi, ApiError, type Device, type AuditEntry, type Customer, type Group, type MaintenanceSession, type IPHistoryEntry, type NetworkMetricsPoint } from '../api'
import StatusBadge from '../components/StatusBadge'
import TerminalView from '../components/TerminalView'
import TunnelPanel from '../components/TunnelPanel'
import SupportCredentialPanel from '../components/SupportCredentialPanel'
import FilesBrowser from '../components/FilesBrowser'
import ServicesPanel from '../components/ServicesPanel'
import ProcessesPanel from '../components/ProcessesPanel'
import LogsPanel from '../components/LogsPanel'
import MetricsChart from '../components/MetricsChart'
import { useAuth } from '../AuthContext'

type Tab = 'overview' | 'monitoring' | 'remote' | 'files' | 'services' | 'processes' | 'logs' | 'maintenance' | 'audit'

export default function DeviceDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [device, setDevice] = useState<Device | null>(null)
  const [tab, setTab] = useState<Tab>('overview')
  const [audit, setAudit] = useState<AuditEntry[]>([])
  const [metrics, setMetrics] = useState<Array<{ observed_at: string; cpu_percent: number; memory_used_bytes: number; memory_total_bytes: number; disk_used_bytes: number; disk_total_bytes: number }>>([])
  const [networkMetrics, setNetworkMetrics] = useState<NetworkMetricsPoint[]>([])
  const [resolution, setResolution] = useState<'raw' | 'hourly'>('raw')
  const [maintenanceHistory, setMaintenanceHistory] = useState<MaintenanceSession[]>([])
  const [customers, setCustomers] = useState<Customer[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [ipHistory, setIpHistory] = useState<IPHistoryEntry[]>([])
  const [error, setError] = useState('')
  const [requesting, setRequesting] = useState(false)
  const [updateBusy, setUpdateBusy] = useState(false)
  const [updateMsg, setUpdateMsg] = useState('')
  const [updateOk, setUpdateOk] = useState(false)
  const [updateChannel, setUpdateChannel] = useState<'stable' | 'beta'>('stable')
  const [dangerAction, setDangerAction] = useState<'revoke' | 'delete' | null>(null)
  const [dangerPassword, setDangerPassword] = useState('')
  const [dangerCode, setDangerCode] = useState('')
  const [dangerBusy, setDangerBusy] = useState(false)
  const [dangerError, setDangerError] = useState('')

  async function load() {
    if (!id) return
    try {
      setDevice(await DeviceApi.get(id))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceDetail.loadFailed'))
    }
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 15000)
    return () => clearInterval(interval)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  useEffect(() => {
    if (!id) return
    if (tab === 'audit') DeviceApi.audit(id).then((v) => setAudit(v ?? [])).catch(() => setAudit([]))
    if (tab === 'monitoring') {
      DeviceApi.metrics(id, resolution).then((v) => setMetrics(v ?? [])).catch(() => setMetrics([]))
      DeviceApi.networkMetrics(id, resolution).then((v) => setNetworkMetrics(v ?? [])).catch(() => setNetworkMetrics([]))
    }
    if (tab === 'maintenance') DeviceApi.maintenance(id).then((v) => setMaintenanceHistory(v ?? [])).catch(() => setMaintenanceHistory([]))
    if (tab === 'overview') {
      CustomerApi.list().then((v) => setCustomers(v ?? [])).catch(() => setCustomers([]))
      CustomerApi.groups().then((v) => setGroups(v ?? [])).catch(() => setGroups([]))
      DeviceApi.ipHistory(id, 24).then((v) => setIpHistory(v ?? [])).catch(() => setIpHistory([]))
    }
  }, [tab, id, resolution])

  async function assignCustomer(customerId: string) {
    if (!id) return
    try {
      await DeviceApi.patch(id, { customer_id: customerId || null })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceDetail.assignCustomerFailed'))
    }
  }

  async function assignGroup(groupId: string) {
    if (!id) return
    try {
      await DeviceApi.patch(id, { group_id: groupId || null })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceDetail.assignGroupFailed'))
    }
  }

  async function requestStatus() {
    if (!id) return
    setRequesting(true)
    try {
      await DeviceApi.statusRequest(id)
      setTimeout(load, 2000)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('deviceDetail.statusRequestFailed'))
    } finally {
      setRequesting(false)
    }
  }

  async function triggerUpdate() {
    if (!id) return
    setUpdateBusy(true)
    setUpdateMsg('')
    setUpdateOk(false)
    try {
      const res = await ReleaseApi.triggerUpdate(id, updateChannel)
      setUpdateMsg(t('deviceDetail.updateTriggered', { version: res.target_version }))
      setUpdateOk(true)
    } catch (err) {
      setUpdateMsg(err instanceof ApiError ? err.message : t('deviceDetail.updateFailed'))
    } finally {
      setUpdateBusy(false)
    }
  }

  async function submitDangerAction(e: React.FormEvent) {
    e.preventDefault()
    if (!id || !dangerAction) return
    setDangerError('')
    setDangerBusy(true)
    try {
      const reauth = await ReauthApi.reauth(dangerPassword, dangerCode)
      if (dangerAction === 'revoke') {
        await DeviceApi.revoke(id, reauth.reauth_id)
        setDangerAction(null)
        setDangerPassword('')
        setDangerCode('')
        load()
      } else {
        await DeviceApi.delete(id, reauth.reauth_id)
        navigate('/')
      }
    } catch (err) {
      setDangerError(err instanceof ApiError ? err.message : t('deviceDetail.dangerActionFailed', { action: dangerAction }))
    } finally {
      setDangerBusy(false)
    }
  }

  if (!device) return <><Link to="/">&larr; {t('common.back')}</Link>{error && <p className="error">{error}</p>}</>

  const canTerminal = user?.permissions.includes('remote.terminal')
  const canFiles = user?.permissions.includes('remote.files.read')
  const canUpdate = user?.permissions.includes('agent.update')
  const canTunnel = user?.permissions.includes('remote.tunnel.ssh') || user?.permissions.includes('remote.tunnel.rdp')
  const canRevoke = user?.permissions.includes('credential.revoke')
  const isWindows = device.os_family === 'windows'
  const defaultFilesPath = isWindows ? 'C:\\' : '/'

  return (
    <>
      <Link to="/">&larr; {t('common.backToDevices')}</Link>
      <header className="device-header">
        <h1>{device.display_name}</h1>
        <StatusBadge kind="status" value={device.status} />
        <StatusBadge kind="health" value={device.health} />
        <button onClick={requestStatus} disabled={requesting}>{t('deviceDetail.refreshStatus')}</button>
      </header>

      {device.health_reasons?.length > 0 && (
        <ul className="health-reasons">
          {device.health_reasons.map((r, i) => <li key={i}>{r}</li>)}
        </ul>
      )}

      <nav className="tabs">
        <button className={tab === 'overview' ? 'active' : ''} onClick={() => setTab('overview')}>{t('deviceDetail.tabs.overview')}</button>
        <button className={tab === 'monitoring' ? 'active' : ''} onClick={() => setTab('monitoring')}>{t('deviceDetail.tabs.monitoring')}</button>
        {canTerminal && <button className={tab === 'remote' ? 'active' : ''} onClick={() => setTab('remote')}>{t('deviceDetail.tabs.remote')}</button>}
        {canFiles && <button className={tab === 'files' ? 'active' : ''} onClick={() => setTab('files')}>{t('deviceDetail.tabs.files')}</button>}
        <button className={tab === 'services' ? 'active' : ''} onClick={() => setTab('services')}>{t('deviceDetail.tabs.services')}</button>
        <button className={tab === 'processes' ? 'active' : ''} onClick={() => setTab('processes')}>{t('deviceDetail.tabs.processes')}</button>
        {canFiles && <button className={tab === 'logs' ? 'active' : ''} onClick={() => setTab('logs')}>{t('deviceDetail.tabs.logs')}</button>}
        <button className={tab === 'maintenance' ? 'active' : ''} onClick={() => setTab('maintenance')}>{t('deviceDetail.tabs.maintenance')}</button>
        <button className={tab === 'audit' ? 'active' : ''} onClick={() => setTab('audit')}>{t('deviceDetail.tabs.audit')}</button>
      </nav>

      {tab === 'overview' && (
        <table className="kv-table">
          <tbody>
            <tr><td>{t('deviceDetail.hostname')}</td><td>{device.hostname || '-'}</td></tr>
            <tr><td>{t('deviceList.os')}</td><td>{device.os_family} {device.os_name} {device.os_version}</td></tr>
            <tr><td>{t('deviceDetail.architecture')}</td><td>{device.architecture || '-'}</td></tr>
            <tr>
              <td>{t('deviceDetail.agentVersion')}</td>
              <td>
                {device.agent_version || '-'}
                {canUpdate && device.status === 'online' && (
                  <>
                    <select
                      value={updateChannel}
                      onChange={(e) => setUpdateChannel(e.target.value as 'stable' | 'beta')}
                      style={{ marginLeft: '0.75rem' }}
                    >
                      <option value="stable">stable</option>
                      <option value="beta">beta</option>
                    </select>
                    <button onClick={triggerUpdate} disabled={updateBusy} style={{ marginLeft: '0.5rem' }}>
                      {updateBusy ? t('deviceDetail.triggering') : t('deviceDetail.checkForUpdate')}
                    </button>
                  </>
                )}
                {updateMsg && <span style={{ marginLeft: '0.5rem', color: updateOk ? 'var(--green)' : 'var(--red)' }}>{updateMsg}</span>}
              </td>
            </tr>
            <tr><td>{t('deviceDetail.installId')}</td><td><code>{device.install_id}</code></td></tr>
            <tr><td>{t('deviceList.lastSeen')}</td><td>{device.last_seen_at ? new Date(device.last_seen_at).toLocaleString() : '-'}</td></tr>
            <tr>
              <td>{t('deviceDetail.publicIp')}</td>
              <td>
                <span
                  title={
                    ipHistory.length > 0
                      ? t('deviceDetail.ipHistoryTitle', { count: ipHistory.length }) + '\n' +
                        ipHistory.map((h) => `${h.IP} (${t('deviceDetail.lastSeenAt', { time: new Date(h.LastSeen).toLocaleString() })})`).join('\n')
                      : t('deviceDetail.noIpHistory')
                  }
                  style={{ cursor: 'help', borderBottom: '1px dotted var(--muted)' }}
                >
                  {device.last_public_ip || '-'}
                  {ipHistory.length > 1 && <span style={{ color: 'var(--yellow)', marginLeft: '0.4rem' }}>({t('deviceDetail.inLast24h', { count: ipHistory.length })})</span>}
                </span>
              </td>
            </tr>
            {device.transport_secure === false && (
              <tr>
                <td>{t('deviceDetail.connection')}</td>
                <td>
                  <span className="badge badge-yellow" title={t('deviceDetail.unencryptedTitle')}>
                    {t('deviceDetail.unencrypted')}
                  </span>
                </td>
              </tr>
            )}
            {canTunnel && (
              <tr>
                <td>{t('deviceDetail.supportAccount')}</td>
                <td>
                  {device.support_credential_available ? (
                    <span className="badge badge-green">
                      {t('deviceDetail.ready')}{device.support_credential_updated_at ? ` (${t('deviceDetail.setAt', { time: new Date(device.support_credential_updated_at).toLocaleString() })})` : ''}
                    </span>
                  ) : (
                    <span className="badge badge-yellow" title={t('deviceDetail.notProvisionedTitle')}>
                      {t('deviceDetail.notProvisioned')}
                    </span>
                  )}
                  {' — ' + t('deviceDetail.seeRemoteTab')}
                </td>
              </tr>
            )}
            <tr><td>{t('deviceDetail.tags')}</td><td>{device.tags?.join(', ') || '-'}</td></tr>
            <tr>
              <td>{t('customers.title')}</td>
              <td>
                <select value={device.customer_id || ''} onChange={(e) => assignCustomer(e.target.value)}>
                  <option value="">{t('common.none')}</option>
                  {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
                </select>
              </td>
            </tr>
            <tr>
              <td>{t('deviceDetail.group')}</td>
              <td>
                <select value={device.group_id || ''} onChange={(e) => assignGroup(e.target.value)}>
                  <option value="">{t('common.none')}</option>
                  {groups
                    .filter((g) => !g.CustomerID || g.CustomerID === device.customer_id)
                    .map((g) => <option key={g.ID} value={g.ID}>{g.Name}</option>)}
                </select>
              </td>
            </tr>
          </tbody>
        </table>
      )}

      {tab === 'overview' && canRevoke && device.status !== 'revoked' && (
        <div className="danger-zone" style={{ marginTop: '2rem', paddingTop: '1rem', borderTop: '1px solid var(--border, #444)' }}>
          <h3>{t('deviceDetail.dangerZone')}</h3>
          {!dangerAction ? (
            <div className="toolbar">
              <button onClick={() => setDangerAction('revoke')}>{t('deviceDetail.revokeDevice')}</button>
              {!device.last_seen_at && <button onClick={() => setDangerAction('delete')}>{t('deviceDetail.deleteDevice')}</button>}
            </div>
          ) : (
            <form onSubmit={submitDangerAction} className="field-form">
              <p style={{ width: '100%' }}>
                {dangerAction === 'revoke'
                  ? t('deviceDetail.confirmRevokeText', { name: device.display_name })
                  : t('deviceDetail.confirmDeleteText', { name: device.display_name })}
              </p>
              <label>
                {t('deviceDetail.yourPassword')}
                <input
                  type="password"
                  value={dangerPassword}
                  onChange={(e) => setDangerPassword(e.target.value)}
                  required
                />
              </label>
              <label>
                {t('account.mfaCode')}
                <input
                  value={dangerCode}
                  onChange={(e) => setDangerCode(e.target.value)}
                  required
                  style={{ width: '6rem' }}
                />
              </label>
              <button type="submit" disabled={dangerBusy}>
                {dangerBusy ? t('deviceDetail.working') : dangerAction === 'revoke' ? t('deviceDetail.confirmRevoke') : t('deviceDetail.confirmDelete')}
              </button>
              <button type="button" onClick={() => { setDangerAction(null); setDangerPassword(''); setDangerCode(''); setDangerError('') }}>
                {t('common.cancel')}
              </button>
            </form>
          )}
          {dangerError && <p className="error">{dangerError}</p>}
        </div>
      )}

      {tab === 'monitoring' && (
        <div>
          <div className="toolbar">
            <label>
              {t('deviceDetail.resolution')}:{' '}
              <select value={resolution} onChange={(e) => setResolution(e.target.value as 'raw' | 'hourly')}>
                <option value="raw">{t('deviceDetail.rawLast24h')}</option>
                <option value="hourly">{t('deviceDetail.hourlyLast30d')}</option>
              </select>
            </label>
          </div>
          <MetricsChart
            title="CPU"
            unit="%"
            max={100}
            points={metrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.cpu_percent }))}
          />
          <MetricsChart
            title={t('deviceDetail.ramUsed')}
            unit=" GB"
            points={metrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.memory_used_bytes / 1e9 }))}
          />
          <MetricsChart
            title={t('deviceDetail.diskUsed')}
            unit="%"
            max={100}
            points={metrics
              .filter((m) => m.disk_total_bytes > 0)
              .map((m) => ({ t: new Date(m.observed_at).getTime(), v: (m.disk_used_bytes / m.disk_total_bytes) * 100 }))}
          />
          <h4 style={{ marginTop: '1.5rem', marginBottom: 0 }}>{t('deviceDetail.network')}</h4>
          <p style={{ color: 'var(--muted, #888)', fontSize: '0.85rem' }}>{t('deviceDetail.networkHint')}</p>
          <MetricsChart
            title={t('deviceDetail.sentTotal')}
            unit=" KB/s"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_sent_total_per_sec / 1024 }))}
          />
          <MetricsChart
            title={t('deviceDetail.receivedTotal')}
            unit=" KB/s"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_recv_total_per_sec / 1024 }))}
          />
          <MetricsChart
            title={t('deviceDetail.sentToServer')}
            unit=" KB/s"
            color="var(--accent-2, #ff9e4a)"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_sent_control_per_sec / 1024 }))}
          />
          <MetricsChart
            title={t('deviceDetail.receivedToServer')}
            unit=" KB/s"
            color="var(--accent-2, #ff9e4a)"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_recv_control_per_sec / 1024 }))}
          />
          <table className="device-table">
            <thead><tr><th>{t('deviceDetail.time')}</th><th>CPU %</th><th>{t('deviceDetail.ramUsedTotal')}</th><th>{t('deviceDetail.diskUsedTotal')}</th></tr></thead>
            <tbody>
              {metrics.map((m, i) => (
                <tr key={i}>
                  <td>{new Date(m.observed_at).toLocaleString()}</td>
                  <td>{m.cpu_percent.toFixed(1)}%</td>
                  <td>{(m.memory_used_bytes / 1e9).toFixed(1)} / {(m.memory_total_bytes / 1e9).toFixed(1)} GB</td>
                  <td>{(m.disk_used_bytes / 1e9).toFixed(1)} / {(m.disk_total_bytes / 1e9).toFixed(1)} GB</td>
                </tr>
              ))}
              {metrics.length === 0 && <tr><td colSpan={4}>{t('deviceDetail.noMetrics')}</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'remote' && (
        device.status === 'online' ? (
          <>
            {canTerminal && <TerminalView deviceId={device.id} />}
            <TunnelPanel deviceId={device.id} osFamily={device.os_family} />
            <SupportCredentialPanel deviceId={device.id} />
          </>
        ) : <p>{t('deviceDetail.mustBeOnlineRemote')}</p>
      )}

      {tab === 'files' && canFiles && (
        device.status === 'online' ? <FilesBrowser deviceId={device.id} defaultPath={defaultFilesPath} /> : <p>{t('deviceDetail.mustBeOnlineFiles')}</p>
      )}

      {tab === 'services' && (
        device.status === 'online' ? <ServicesPanel deviceId={device.id} /> : <p>{t('deviceDetail.mustBeOnlineServices')}</p>
      )}

      {tab === 'processes' && (
        device.status === 'online' ? <ProcessesPanel deviceId={device.id} /> : <p>{t('deviceDetail.mustBeOnlineProcesses')}</p>
      )}

      {tab === 'logs' && canFiles && (
        device.status === 'online' ? <LogsPanel deviceId={device.id} /> : <p>{t('deviceDetail.mustBeOnlineLogs')}</p>
      )}

      {tab === 'maintenance' && (
        <table className="device-table">
          <thead><tr><th>{t('deviceDetail.started')}</th><th>{t('deviceDetail.ended')}</th><th>{t('deviceDetail.result')}</th><th>{t('alerts.summary')}</th></tr></thead>
          <tbody>
            {maintenanceHistory.map((m) => (
              <tr key={m.ID}>
                <td>{new Date(m.StartedAt).toLocaleString()}</td>
                <td>{m.EndedAt ? new Date(m.EndedAt).toLocaleString() : t('deviceDetail.inProgress')}</td>
                <td>{m.Result || '-'}</td>
                <td>{m.Summary || '-'}</td>
              </tr>
            ))}
            {maintenanceHistory.length === 0 && <tr><td colSpan={4}>{t('deviceDetail.noMaintenance')}</td></tr>}
          </tbody>
        </table>
      )}

      {tab === 'audit' && (
        <table className="device-table">
          <caption style={{ textAlign: 'left', marginBottom: '0.5rem' }}>
            {t('deviceDetail.export')}: <a href={AuditApi.exportUrl('json', device.id)}>JSON</a>{' '}
            <a href={AuditApi.exportUrl('csv', device.id)}>CSV</a>
          </caption>
          <thead><tr><th>{t('deviceDetail.time')}</th><th>{t('deviceDetail.event')}</th><th>{t('deviceDetail.result')}</th><th>{t('deviceDetail.actor')}</th><th>{t('deviceDetail.ip')}</th></tr></thead>
          <tbody>
            {audit.map((a) => (
              <tr key={a.ID}>
                <td>{new Date(a.OccurredAt).toLocaleString()}</td>
                <td>{a.EventType}</td>
                <td>{a.Result}</td>
                <td>{a.ActorUsername ? `${a.ActorUsername} (${a.ActorType})` : a.ActorType}</td>
                <td>{a.SourceIP || '-'}</td>
              </tr>
            ))}
            {audit.length === 0 && <tr><td colSpan={5}>{t('deviceDetail.noAudit')}</td></tr>}
          </tbody>
        </table>
      )}
    </>
  )
}

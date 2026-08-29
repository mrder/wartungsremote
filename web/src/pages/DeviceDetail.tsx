import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { DeviceApi, CustomerApi, ReleaseApi, AuditApi, ApiError, type Device, type AuditEntry, type Customer, type Group, type MaintenanceSession, type IPHistoryEntry, type NetworkMetricsPoint } from '../api'
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
  const { id } = useParams<{ id: string }>()
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
  const [updateChannel, setUpdateChannel] = useState<'stable' | 'beta'>('stable')

  async function load() {
    if (!id) return
    try {
      setDevice(await DeviceApi.get(id))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load device')
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
      setError(err instanceof ApiError ? err.message : 'Failed to assign customer')
    }
  }

  async function assignGroup(groupId: string) {
    if (!id) return
    try {
      await DeviceApi.patch(id, { group_id: groupId || null })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to assign group')
    }
  }

  async function requestStatus() {
    if (!id) return
    setRequesting(true)
    try {
      await DeviceApi.statusRequest(id)
      setTimeout(load, 2000)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Status request failed')
    } finally {
      setRequesting(false)
    }
  }

  async function triggerUpdate() {
    if (!id) return
    setUpdateBusy(true)
    setUpdateMsg('')
    try {
      const res = await ReleaseApi.triggerUpdate(id, updateChannel)
      setUpdateMsg(`Update to ${res.target_version} triggered.`)
    } catch (err) {
      setUpdateMsg(err instanceof ApiError ? err.message : 'Failed to trigger update')
    } finally {
      setUpdateBusy(false)
    }
  }

  if (!device) return <div className="page"><Link to="/">&larr; Back</Link>{error && <p className="error">{error}</p>}</div>

  const canTerminal = user?.permissions.includes('remote.terminal')
  const canFiles = user?.permissions.includes('remote.files.read')
  const canUpdate = user?.permissions.includes('agent.update')
  const canTunnel = user?.permissions.includes('remote.tunnel.ssh') || user?.permissions.includes('remote.tunnel.rdp')
  const isWindows = device.os_family === 'windows'
  const defaultFilesPath = isWindows ? 'C:\\' : '/'

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <header className="device-header">
        <h1>{device.display_name}</h1>
        <StatusBadge kind="status" value={device.status} />
        <StatusBadge kind="health" value={device.health} />
        <button onClick={requestStatus} disabled={requesting}>Refresh status</button>
      </header>

      {device.health_reasons?.length > 0 && (
        <ul className="health-reasons">
          {device.health_reasons.map((r, i) => <li key={i}>{r}</li>)}
        </ul>
      )}

      <nav className="tabs">
        <button className={tab === 'overview' ? 'active' : ''} onClick={() => setTab('overview')}>Overview</button>
        <button className={tab === 'monitoring' ? 'active' : ''} onClick={() => setTab('monitoring')}>Monitoring</button>
        {canTerminal && <button className={tab === 'remote' ? 'active' : ''} onClick={() => setTab('remote')}>Remote</button>}
        {canFiles && <button className={tab === 'files' ? 'active' : ''} onClick={() => setTab('files')}>Files</button>}
        <button className={tab === 'services' ? 'active' : ''} onClick={() => setTab('services')}>Services</button>
        <button className={tab === 'processes' ? 'active' : ''} onClick={() => setTab('processes')}>Processes</button>
        {canFiles && <button className={tab === 'logs' ? 'active' : ''} onClick={() => setTab('logs')}>Logs</button>}
        <button className={tab === 'maintenance' ? 'active' : ''} onClick={() => setTab('maintenance')}>Maintenance</button>
        <button className={tab === 'audit' ? 'active' : ''} onClick={() => setTab('audit')}>Audit</button>
      </nav>

      {tab === 'overview' && (
        <table className="kv-table">
          <tbody>
            <tr><td>Hostname</td><td>{device.hostname || '-'}</td></tr>
            <tr><td>OS</td><td>{device.os_family} {device.os_name} {device.os_version}</td></tr>
            <tr><td>Architecture</td><td>{device.architecture || '-'}</td></tr>
            <tr>
              <td>Agent version</td>
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
                      {updateBusy ? 'Triggering...' : 'Check for update'}
                    </button>
                  </>
                )}
                {updateMsg && <span style={{ marginLeft: '0.5rem' }}>{updateMsg}</span>}
              </td>
            </tr>
            <tr><td>Install ID</td><td><code>{device.install_id}</code></td></tr>
            <tr><td>Last seen</td><td>{device.last_seen_at ? new Date(device.last_seen_at).toLocaleString() : '-'}</td></tr>
            <tr>
              <td>Public IP</td>
              <td>
                <span
                  title={
                    ipHistory.length > 0
                      ? `${ipHistory.length} distinct IP(s) in last 24h:\n` +
                        ipHistory.map((h) => `${h.IP} (last seen ${new Date(h.LastSeen).toLocaleString()})`).join('\n')
                      : 'No IP history recorded yet in the last 24h'
                  }
                  style={{ cursor: 'help', borderBottom: '1px dotted var(--muted)' }}
                >
                  {device.last_public_ip || '-'}
                  {ipHistory.length > 1 && <span style={{ color: 'var(--yellow)', marginLeft: '0.4rem' }}>({ipHistory.length} in 24h)</span>}
                </span>
              </td>
            </tr>
            {canTunnel && (
              <tr>
                <td>Remote-support account</td>
                <td>
                  {device.support_credential_available ? (
                    <span className="badge badge-green">
                      Ready{device.support_credential_updated_at ? ` (set ${new Date(device.support_credential_updated_at).toLocaleString()})` : ''}
                    </span>
                  ) : (
                    <span className="badge badge-yellow" title="The device reports this shortly after first connecting, or after policy changes enable it">
                      Not yet provisioned
                    </span>
                  )}
                  {' — see Remote tab to reveal/rotate'}
                </td>
              </tr>
            )}
            <tr><td>Tags</td><td>{device.tags?.join(', ') || '-'}</td></tr>
            <tr>
              <td>Customer</td>
              <td>
                <select value={device.customer_id || ''} onChange={(e) => assignCustomer(e.target.value)}>
                  <option value="">- none -</option>
                  {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
                </select>
              </td>
            </tr>
            <tr>
              <td>Group</td>
              <td>
                <select value={device.group_id || ''} onChange={(e) => assignGroup(e.target.value)}>
                  <option value="">- none -</option>
                  {groups
                    .filter((g) => !g.CustomerID || g.CustomerID === device.customer_id)
                    .map((g) => <option key={g.ID} value={g.ID}>{g.Name}</option>)}
                </select>
              </td>
            </tr>
          </tbody>
        </table>
      )}

      {tab === 'monitoring' && (
        <div>
          <div className="toolbar">
            <label>
              Resolution:{' '}
              <select value={resolution} onChange={(e) => setResolution(e.target.value as 'raw' | 'hourly')}>
                <option value="raw">Raw (last 24h)</option>
                <option value="hourly">Hourly average (last 30d)</option>
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
            title="RAM used"
            unit=" GB"
            points={metrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.memory_used_bytes / 1e9 }))}
          />
          <MetricsChart
            title="Disk used"
            unit="%"
            max={100}
            points={metrics
              .filter((m) => m.disk_total_bytes > 0)
              .map((m) => ({ t: new Date(m.observed_at).getTime(), v: (m.disk_used_bytes / m.disk_total_bytes) * 100 }))}
          />
          <h4 style={{ marginTop: '1.5rem', marginBottom: 0 }}>Network</h4>
          <p style={{ color: 'var(--muted, #888)', fontSize: '0.85rem' }}>
            "Total" is all network traffic on the device; "to server" is just this agent's own
            control-channel overhead — useful for telling general bandwidth use apart from what this
            tool itself adds.
          </p>
          <MetricsChart
            title="Sent (total)"
            unit=" KB/s"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_sent_total_per_sec / 1024 }))}
          />
          <MetricsChart
            title="Received (total)"
            unit=" KB/s"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_recv_total_per_sec / 1024 }))}
          />
          <MetricsChart
            title="Sent (to server)"
            unit=" KB/s"
            color="var(--accent-2, #ff9e4a)"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_sent_control_per_sec / 1024 }))}
          />
          <MetricsChart
            title="Received (to server)"
            unit=" KB/s"
            color="var(--accent-2, #ff9e4a)"
            points={networkMetrics.map((m) => ({ t: new Date(m.observed_at).getTime(), v: m.bytes_recv_control_per_sec / 1024 }))}
          />
          <table className="device-table">
            <thead><tr><th>Time</th><th>CPU %</th><th>RAM used/total</th><th>Disk used/total</th></tr></thead>
            <tbody>
              {metrics.map((m, i) => (
                <tr key={i}>
                  <td>{new Date(m.observed_at).toLocaleString()}</td>
                  <td>{m.cpu_percent.toFixed(1)}%</td>
                  <td>{(m.memory_used_bytes / 1e9).toFixed(1)} / {(m.memory_total_bytes / 1e9).toFixed(1)} GB</td>
                  <td>{(m.disk_used_bytes / 1e9).toFixed(1)} / {(m.disk_total_bytes / 1e9).toFixed(1)} GB</td>
                </tr>
              ))}
              {metrics.length === 0 && <tr><td colSpan={4}>No metrics yet.</td></tr>}
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
        ) : <p>Device must be online for remote access.</p>
      )}

      {tab === 'files' && canFiles && (
        device.status === 'online' ? <FilesBrowser deviceId={device.id} defaultPath={defaultFilesPath} /> : <p>Device must be online to browse files.</p>
      )}

      {tab === 'services' && (
        device.status === 'online' ? <ServicesPanel deviceId={device.id} /> : <p>Device must be online to manage services.</p>
      )}

      {tab === 'processes' && (
        device.status === 'online' ? <ProcessesPanel deviceId={device.id} /> : <p>Device must be online to manage processes.</p>
      )}

      {tab === 'logs' && canFiles && (
        device.status === 'online' ? <LogsPanel deviceId={device.id} /> : <p>Device must be online to query logs.</p>
      )}

      {tab === 'maintenance' && (
        <table className="device-table">
          <thead><tr><th>Started</th><th>Ended</th><th>Result</th><th>Summary</th></tr></thead>
          <tbody>
            {maintenanceHistory.map((m) => (
              <tr key={m.ID}>
                <td>{new Date(m.StartedAt).toLocaleString()}</td>
                <td>{m.EndedAt ? new Date(m.EndedAt).toLocaleString() : 'in progress'}</td>
                <td>{m.Result || '-'}</td>
                <td>{m.Summary || '-'}</td>
              </tr>
            ))}
            {maintenanceHistory.length === 0 && <tr><td colSpan={4}>No maintenance history yet.</td></tr>}
          </tbody>
        </table>
      )}

      {tab === 'audit' && (
        <table className="device-table">
          <caption style={{ textAlign: 'left', marginBottom: '0.5rem' }}>
            Export: <a href={AuditApi.exportUrl('json', device.id)}>JSON</a>{' '}
            <a href={AuditApi.exportUrl('csv', device.id)}>CSV</a>
          </caption>
          <thead><tr><th>Time</th><th>Event</th><th>Result</th><th>Actor</th><th>IP</th></tr></thead>
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
            {audit.length === 0 && <tr><td colSpan={5}>No audit entries yet.</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

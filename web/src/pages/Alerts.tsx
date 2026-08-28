import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertApi, DeviceApi, CustomerApi, ApiError, type Alert, type AlertRule, type Device, type Customer, type Group } from '../api'
import { useAuth } from '../AuthContext'

const RULE_TYPES = ['offline', 'cpu', 'ram', 'disk', 'service', 'agent_version'] as const

export default function Alerts() {
  const { user } = useAuth()
  const canManage = user?.permissions.includes('alert.manage')

  const [alerts, setAlerts] = useState<Alert[]>([])
  const [rules, setRules] = useState<AlertRule[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [customers, setCustomers] = useState<Customer[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [stateFilter, setStateFilter] = useState('')
  const [error, setError] = useState('')

  const [scopeType, setScopeType] = useState('global')
  const [scopeID, setScopeID] = useState('')
  const [ruleType, setRuleType] = useState<(typeof RULE_TYPES)[number]>('cpu')
  const [threshold, setThreshold] = useState('90')
  const [serviceName, setServiceName] = useState('')
  const [minVersion, setMinVersion] = useState('')

  async function loadAlerts() {
    try {
      setAlerts((await AlertApi.list(stateFilter ? { state: stateFilter } : {})) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load alerts')
    }
  }

  async function loadRules() {
    try {
      setRules((await AlertApi.listRules()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load alert rules')
    }
  }

  useEffect(() => {
    loadAlerts()
    loadRules()
    DeviceApi.list().then((v) => setDevices(v ?? [])).catch(() => setDevices([]))
    CustomerApi.list().then((v) => setCustomers(v ?? [])).catch(() => setCustomers([]))
    CustomerApi.groups().then((v) => setGroups(v ?? [])).catch(() => setGroups([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stateFilter])

  function deviceName(id: string) {
    return devices.find((d) => d.id === id)?.display_name ?? id
  }

  async function acknowledge(id: string) {
    try {
      await AlertApi.acknowledge(id)
      loadAlerts()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to acknowledge alert')
    }
  }

  async function resolve(id: string) {
    try {
      await AlertApi.resolve(id)
      loadAlerts()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to resolve alert')
    }
  }

  async function deleteAlert(id: string) {
    try {
      await AlertApi.delete(id)
      loadAlerts()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete alert')
    }
  }

  async function toggleRule(id: string, enabled: boolean) {
    try {
      await AlertApi.setRuleEnabled(id, enabled)
      loadRules()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update rule')
    }
  }

  async function deleteRule(id: string) {
    try {
      await AlertApi.deleteRule(id)
      loadRules()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete rule')
    }
  }

  async function createRule(e: React.FormEvent) {
    e.preventDefault()
    let config: Record<string, unknown> = {}
    if (ruleType === 'cpu' || ruleType === 'ram' || ruleType === 'disk') {
      config = { threshold_percent: Number(threshold) }
    } else if (ruleType === 'service') {
      config = { service_name: serviceName }
    } else if (ruleType === 'agent_version') {
      config = { minimum_version: minVersion }
    }
    try {
      await AlertApi.createRule({
        scope_type: scopeType,
        scope_id: scopeType === 'global' ? undefined : scopeID || undefined,
        rule_type: ruleType,
        config,
      })
      loadRules()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create rule')
    }
  }

  function ruleSummary(r: AlertRule) {
    const cfg = r.Config as Record<string, unknown>
    switch (r.RuleType) {
      case 'cpu': return `CPU >= ${cfg.threshold_percent}%`
      case 'ram': return `RAM >= ${cfg.threshold_percent}%`
      case 'disk': return `Disk >= ${cfg.threshold_percent}%`
      case 'service': return `Service "${cfg.service_name}" not running`
      case 'agent_version': return `Agent older than ${cfg.minimum_version}`
      case 'offline': return 'Device offline'
      default: return r.RuleType
    }
  }

  function scopeLabel(r: AlertRule) {
    if (r.ScopeType === 'global') return 'All devices'
    if (r.ScopeType === 'device') return deviceName(r.ScopeID ?? '')
    if (r.ScopeType === 'customer') return 'Customer: ' + (customers.find((c) => c.ID === r.ScopeID)?.Name ?? r.ScopeID)
    if (r.ScopeType === 'group') return 'Group: ' + (groups.find((g) => g.ID === r.ScopeID)?.Name ?? r.ScopeID)
    return r.ScopeType
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Alerts</h1>
      {error && <p className="error">{error}</p>}

      <h3>Open / recent alerts</h3>
      <div className="toolbar">
        <select value={stateFilter} onChange={(e) => setStateFilter(e.target.value)}>
          <option value="">All states</option>
          <option value="open">Open</option>
          <option value="acknowledged">Acknowledged</option>
          <option value="resolved">Resolved</option>
        </select>
      </div>
      <table className="device-table">
        <thead><tr><th>Device</th><th>Severity</th><th>State</th><th>Summary</th><th>Opened</th><th></th></tr></thead>
        <tbody>
          {alerts.map((a) => (
            <tr key={a.ID}>
              <td><Link to={`/devices/${a.DeviceID}`}>{deviceName(a.DeviceID)}</Link></td>
              <td><span className={`badge badge-${a.Severity === 'critical' ? 'red' : 'yellow'}`}>{a.Severity}</span></td>
              <td>{a.State}</td>
              <td>{a.Summary}</td>
              <td>{new Date(a.OpenedAt).toLocaleString()}</td>
              <td>
                {canManage && a.State === 'open' && <button onClick={() => acknowledge(a.ID)}>Acknowledge</button>}
                {canManage && a.State !== 'resolved' && <button onClick={() => resolve(a.ID)}>Resolve</button>}
                {canManage && <button onClick={() => deleteAlert(a.ID)}>Delete</button>}
              </td>
            </tr>
          ))}
          {alerts.length === 0 && <tr><td colSpan={6}>No alerts.</td></tr>}
        </tbody>
      </table>

      <h3>Alert rules</h3>
      <table className="device-table">
        <thead><tr><th>Type</th><th>Condition</th><th>Scope</th><th>Enabled</th><th></th></tr></thead>
        <tbody>
          {rules.map((r) => (
            <tr key={r.ID}>
              <td>{r.RuleType}</td>
              <td>{ruleSummary(r)}</td>
              <td>{scopeLabel(r)}</td>
              <td>{r.Enabled ? 'yes' : 'no'}</td>
              <td>
                {canManage && (
                  <>
                    <button onClick={() => toggleRule(r.ID, !r.Enabled)}>{r.Enabled ? 'Disable' : 'Enable'}</button>
                    <button onClick={() => deleteRule(r.ID)}>Delete</button>
                  </>
                )}
              </td>
            </tr>
          ))}
          {rules.length === 0 && <tr><td colSpan={5}>No alert rules configured yet.</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <form onSubmit={createRule} className="toolbar" style={{ flexWrap: 'wrap' }}>
          <select value={ruleType} onChange={(e) => setRuleType(e.target.value as typeof ruleType)}>
            {RULE_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
          {(ruleType === 'cpu' || ruleType === 'ram' || ruleType === 'disk') && (
            <input type="number" placeholder="Threshold %" value={threshold} onChange={(e) => setThreshold(e.target.value)} required />
          )}
          {ruleType === 'service' && (
            <input placeholder="Service name" value={serviceName} onChange={(e) => setServiceName(e.target.value)} required />
          )}
          {ruleType === 'agent_version' && (
            <input placeholder="Minimum version (e.g. 0.2.0)" value={minVersion} onChange={(e) => setMinVersion(e.target.value)} required />
          )}
          <select value={scopeType} onChange={(e) => { setScopeType(e.target.value); setScopeID('') }}>
            <option value="global">All devices</option>
            <option value="customer">Customer</option>
            <option value="group">Group</option>
            <option value="device">Device</option>
          </select>
          {scopeType === 'customer' && (
            <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
              <option value="">- select customer -</option>
              {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
            </select>
          )}
          {scopeType === 'group' && (
            <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
              <option value="">- select group -</option>
              {groups.map((g) => <option key={g.ID} value={g.ID}>{g.Name}</option>)}
            </select>
          )}
          {scopeType === 'device' && (
            <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
              <option value="">- select device -</option>
              {devices.map((d) => <option key={d.id} value={d.id}>{d.display_name}</option>)}
            </select>
          )}
          <button type="submit">+ Add rule</button>
        </form>
      )}
    </div>
  )
}

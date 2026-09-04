import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AlertApi, DeviceApi, CustomerApi, ApiError, type Alert, type AlertRule, type Device, type Customer, type Group } from '../api'
import { useAuth } from '../AuthContext'

const RULE_TYPES = ['offline', 'cpu', 'ram', 'disk', 'service', 'agent_version'] as const

export default function Alerts() {
  const { t } = useTranslation()
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
      setError(err instanceof ApiError ? err.message : t('alerts.loadFailed'))
    }
  }

  async function loadRules() {
    try {
      setRules((await AlertApi.listRules()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('alerts.loadRulesFailed'))
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
      setError(err instanceof ApiError ? err.message : t('alerts.acknowledgeFailed'))
    }
  }

  async function resolve(id: string) {
    try {
      await AlertApi.resolve(id)
      loadAlerts()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('alerts.resolveFailed'))
    }
  }

  async function deleteAlert(id: string) {
    try {
      await AlertApi.delete(id)
      loadAlerts()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('alerts.deleteFailed'))
    }
  }

  async function toggleRule(id: string, enabled: boolean) {
    try {
      await AlertApi.setRuleEnabled(id, enabled)
      loadRules()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('alerts.updateRuleFailed'))
    }
  }

  async function deleteRule(id: string) {
    try {
      await AlertApi.deleteRule(id)
      loadRules()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('alerts.deleteRuleFailed'))
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
      setError(err instanceof ApiError ? err.message : t('alerts.createRuleFailed'))
    }
  }

  function ruleTypeLabel(rt: string) {
    return t(`alerts.ruleType.${rt}`, { defaultValue: rt })
  }

  function ruleSummary(r: AlertRule) {
    const cfg = r.Config as Record<string, unknown>
    switch (r.RuleType) {
      case 'cpu': return `CPU >= ${cfg.threshold_percent}%`
      case 'ram': return `RAM >= ${cfg.threshold_percent}%`
      case 'disk': return `Disk >= ${cfg.threshold_percent}%`
      case 'service': return t('alerts.serviceNotRunning', { name: cfg.service_name })
      case 'agent_version': return t('alerts.agentOlderThan', { version: cfg.minimum_version })
      case 'offline': return t('alerts.deviceOffline')
      default: return r.RuleType
    }
  }

  function scopeLabel(r: AlertRule) {
    if (r.ScopeType === 'global') return t('alerts.allDevices')
    if (r.ScopeType === 'device') return deviceName(r.ScopeID ?? '')
    if (r.ScopeType === 'customer') return t('customers.title') + ': ' + (customers.find((c) => c.ID === r.ScopeID)?.Name ?? r.ScopeID)
    if (r.ScopeType === 'group') return t('customers.groups') + ': ' + (groups.find((g) => g.ID === r.ScopeID)?.Name ?? r.ScopeID)
    return r.ScopeType
  }

  return (
    <>
      <h1>{t('alerts.title')}</h1>
      <p>{t('alerts.intro')}</p>
      {error && <p className="error">{error}</p>}

      <h3>{t('alerts.openRecent')}</h3>
      <div className="toolbar">
        <select value={stateFilter} onChange={(e) => setStateFilter(e.target.value)}>
          <option value="">{t('alerts.allStates')}</option>
          <option value="open">{t('alerts.open')}</option>
          <option value="acknowledged">{t('alerts.acknowledged')}</option>
          <option value="resolved">{t('alerts.resolved')}</option>
        </select>
      </div>
      <table className="device-table">
        <thead><tr><th>{t('alerts.device')}</th><th>{t('alerts.severity')}</th><th>{t('alerts.state')}</th><th>{t('alerts.summary')}</th><th>{t('alerts.opened')}</th><th></th></tr></thead>
        <tbody>
          {alerts.map((a) => (
            <tr key={a.ID}>
              <td><Link to={`/devices/${a.DeviceID}`}>{deviceName(a.DeviceID)}</Link></td>
              <td><span className={`badge badge-${a.Severity === 'critical' ? 'red' : 'yellow'}`}>{a.Severity}</span></td>
              <td>{a.State}</td>
              <td>{a.Summary}</td>
              <td>{new Date(a.OpenedAt).toLocaleString()}</td>
              <td>
                {canManage && a.State === 'open' && <button onClick={() => acknowledge(a.ID)}>{t('alerts.acknowledge')}</button>}
                {canManage && a.State !== 'resolved' && <button onClick={() => resolve(a.ID)}>{t('alerts.resolve')}</button>}
                {canManage && <button onClick={() => deleteAlert(a.ID)}>{t('common.delete')}</button>}
              </td>
            </tr>
          ))}
          {alerts.length === 0 && <tr><td colSpan={6}>{t('alerts.noAlerts')}</td></tr>}
        </tbody>
      </table>

      <h3>{t('alerts.alertRules')}</h3>
      <p>{t('alerts.rulesHint')}</p>
      <table className="device-table">
        <thead><tr><th>{t('deviceList.type')}</th><th>{t('alerts.condition')}</th><th>{t('alerts.scope')}</th><th>{t('alerts.enabled')}</th><th></th></tr></thead>
        <tbody>
          {rules.map((r) => (
            <tr key={r.ID}>
              <td>{ruleTypeLabel(r.RuleType)}</td>
              <td>{ruleSummary(r)}</td>
              <td>{scopeLabel(r)}</td>
              <td>{r.Enabled ? t('common.yes') : t('common.no')}</td>
              <td>
                {canManage && (
                  <>
                    <button onClick={() => toggleRule(r.ID, !r.Enabled)}>{r.Enabled ? t('alerts.disable') : t('alerts.enable')}</button>
                    <button onClick={() => deleteRule(r.ID)}>{t('common.delete')}</button>
                  </>
                )}
              </td>
            </tr>
          ))}
          {rules.length === 0 && <tr><td colSpan={5}>{t('alerts.noRules')}</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <form onSubmit={createRule} className="field-form">
          <label>
            {t('alerts.condition')}
            <select value={ruleType} onChange={(e) => setRuleType(e.target.value as typeof ruleType)}>
              {RULE_TYPES.map((rt) => <option key={rt} value={rt}>{ruleTypeLabel(rt)}</option>)}
            </select>
          </label>
          {(ruleType === 'cpu' || ruleType === 'ram' || ruleType === 'disk') && (
            <label>
              {t('alerts.thresholdPercent')}
              <input type="number" value={threshold} onChange={(e) => setThreshold(e.target.value)} required style={{ width: '6rem' }} />
            </label>
          )}
          {ruleType === 'service' && (
            <label>
              {t('alerts.serviceName')}
              <input value={serviceName} onChange={(e) => setServiceName(e.target.value)} required />
            </label>
          )}
          {ruleType === 'agent_version' && (
            <label>
              {t('alerts.minimumVersion')}
              <input value={minVersion} onChange={(e) => setMinVersion(e.target.value)} required />
            </label>
          )}
          <label>
            {t('alerts.scope')}
            <select value={scopeType} onChange={(e) => { setScopeType(e.target.value); setScopeID('') }}>
              <option value="global">{t('alerts.allDevices')}</option>
              <option value="customer">{t('customers.title')}</option>
              <option value="group">{t('customers.groups')}</option>
              <option value="device">{t('alerts.device')}</option>
            </select>
          </label>
          {scopeType === 'customer' && (
            <label>
              {t('customers.title')}
              <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
                <option value="">{t('alerts.selectCustomer')}</option>
                {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
              </select>
            </label>
          )}
          {scopeType === 'group' && (
            <label>
              {t('customers.groups')}
              <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
                <option value="">{t('alerts.selectGroup')}</option>
                {groups.map((g) => <option key={g.ID} value={g.ID}>{g.Name}</option>)}
              </select>
            </label>
          )}
          {scopeType === 'device' && (
            <label>
              {t('alerts.device')}
              <select value={scopeID} onChange={(e) => setScopeID(e.target.value)} required>
                <option value="">{t('alerts.selectDevice')}</option>
                {devices.map((d) => <option key={d.id} value={d.id}>{d.display_name}</option>)}
              </select>
            </label>
          )}
          <button type="submit">{t('alerts.addRule')}</button>
        </form>
      )}
    </>
  )
}

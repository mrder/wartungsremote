import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CustomerApi, ApiError, type Customer, type Group } from '../api'
import { useAuth } from '../AuthContext'

export default function Customers() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [customers, setCustomers] = useState<Customer[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [number, setNumber] = useState('')
  const [groupName, setGroupName] = useState('')
  const [groupCustomerId, setGroupCustomerId] = useState('')
  const [renamingId, setRenamingId] = useState('')
  const [renameValue, setRenameValue] = useState('')
  const canManage = user?.permissions.includes('customer.manage')

  async function load() {
    try {
      setCustomers((await CustomerApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.loadFailed'))
    }
  }

  async function loadGroups() {
    try {
      setGroups((await CustomerApi.groups()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.loadGroupsFailed'))
    }
  }

  useEffect(() => {
    load()
    loadGroups()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function create(e: React.FormEvent) {
    e.preventDefault()
    if (!name) return
    try {
      await CustomerApi.create(name, number, '')
      setName('')
      setNumber('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.createFailed'))
    }
  }

  async function createGroup(e: React.FormEvent) {
    e.preventDefault()
    if (!groupName) return
    try {
      await CustomerApi.createGroup(groupName, groupCustomerId || undefined)
      setGroupName('')
      setGroupCustomerId('')
      loadGroups()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.createGroupFailed'))
    }
  }

  async function saveRename(id: string) {
    if (!renameValue) return
    try {
      await CustomerApi.renameGroup(id, renameValue)
      setRenamingId('')
      loadGroups()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.renameGroupFailed'))
    }
  }

  async function deleteGroup(id: string) {
    try {
      await CustomerApi.deleteGroup(id)
      loadGroups()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('customers.deleteGroupFailed'))
    }
  }

  function customerName(id: string | null) {
    if (!id) return t('customers.global')
    return customers.find((c) => c.ID === id)?.Name ?? id
  }

  return (
    <>
      <h1>{t('customers.title')}</h1>
      <p>{t('customers.intro')}</p>
      {error && <p className="error">{error}</p>}

      {canManage && (
        <form onSubmit={create} className="toolbar">
          <input placeholder={t('customers.customerName')} value={name} onChange={(e) => setName(e.target.value)} required />
          <input placeholder={t('customers.customerNumber')} value={number} onChange={(e) => setNumber(e.target.value)} />
          <button type="submit">{t('customers.addCustomer')}</button>
        </form>
      )}

      <table className="device-table">
        <thead><tr><th>{t('deviceList.name')}</th><th>{t('customers.number')}</th><th>{t('deviceList.status')}</th></tr></thead>
        <tbody>
          {customers.map((c) => (
            <tr key={c.ID}>
              <td>{c.Name}</td>
              <td>{c.CustomerNumber || '-'}</td>
              <td>{c.Status}</td>
            </tr>
          ))}
          {customers.length === 0 && <tr><td colSpan={3}>{t('customers.noCustomers')}</td></tr>}
        </tbody>
      </table>

      <h3>{t('customers.groups')}</h3>
      <p>{t('customers.groupsHint')}</p>
      <table className="device-table">
        <thead><tr><th>{t('deviceList.name')}</th><th>{t('customers.title')}</th><th></th></tr></thead>
        <tbody>
          {groups.map((g) => (
            <tr key={g.ID}>
              <td>
                {renamingId === g.ID ? (
                  <input value={renameValue} onChange={(e) => setRenameValue(e.target.value)} autoFocus />
                ) : (
                  g.Name
                )}
              </td>
              <td>{customerName(g.CustomerID)}</td>
              <td>
                {canManage && (
                  renamingId === g.ID ? (
                    <>
                      <button onClick={() => saveRename(g.ID)}>{t('common.save')}</button>
                      <button onClick={() => setRenamingId('')}>{t('common.cancel')}</button>
                    </>
                  ) : (
                    <>
                      <button onClick={() => { setRenamingId(g.ID); setRenameValue(g.Name) }}>{t('customers.rename')}</button>
                      <button onClick={() => deleteGroup(g.ID)}>{t('common.delete')}</button>
                    </>
                  )
                )}
              </td>
            </tr>
          ))}
          {groups.length === 0 && <tr><td colSpan={3}>{t('customers.noGroups')}</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <form onSubmit={createGroup} className="toolbar">
          <input placeholder={t('customers.groupName')} value={groupName} onChange={(e) => setGroupName(e.target.value)} required />
          <select value={groupCustomerId} onChange={(e) => setGroupCustomerId(e.target.value)}>
            <option value="">{t('customers.globalAllCustomers')}</option>
            {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
          </select>
          <button type="submit">{t('customers.addGroup')}</button>
        </form>
      )}
    </>
  )
}

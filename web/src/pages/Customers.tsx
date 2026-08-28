import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { CustomerApi, ApiError, type Customer, type Group } from '../api'
import { useAuth } from '../AuthContext'

export default function Customers() {
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
      setError(err instanceof ApiError ? err.message : 'Failed to load customers')
    }
  }

  async function loadGroups() {
    try {
      setGroups((await CustomerApi.groups()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load groups')
    }
  }

  useEffect(() => {
    load()
    loadGroups()
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
      setError(err instanceof ApiError ? err.message : 'Failed to create customer')
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
      setError(err instanceof ApiError ? err.message : 'Failed to create group')
    }
  }

  async function saveRename(id: string) {
    if (!renameValue) return
    try {
      await CustomerApi.renameGroup(id, renameValue)
      setRenamingId('')
      loadGroups()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to rename group')
    }
  }

  async function deleteGroup(id: string) {
    try {
      await CustomerApi.deleteGroup(id)
      loadGroups()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete group')
    }
  }

  function customerName(id: string | null) {
    if (!id) return '- global -'
    return customers.find((c) => c.ID === id)?.Name ?? id
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Customers</h1>
      {error && <p className="error">{error}</p>}

      {canManage && (
        <form onSubmit={create} className="toolbar">
          <input placeholder="Customer name" value={name} onChange={(e) => setName(e.target.value)} required />
          <input placeholder="Customer number" value={number} onChange={(e) => setNumber(e.target.value)} />
          <button type="submit">+ Add Customer</button>
        </form>
      )}

      <table className="device-table">
        <thead><tr><th>Name</th><th>Number</th><th>Status</th></tr></thead>
        <tbody>
          {customers.map((c) => (
            <tr key={c.ID}>
              <td>{c.Name}</td>
              <td>{c.CustomerNumber || '-'}</td>
              <td>{c.Status}</td>
            </tr>
          ))}
          {customers.length === 0 && <tr><td colSpan={3}>No customers yet.</td></tr>}
        </tbody>
      </table>

      <h3>Groups</h3>
      <p>
        Devices and alert rules can be scoped to a group instead of (or within) a customer — assign a device
        to one from its Overview tab. A group left without a customer applies across all customers.
      </p>
      <table className="device-table">
        <thead><tr><th>Name</th><th>Customer</th><th></th></tr></thead>
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
                      <button onClick={() => saveRename(g.ID)}>Save</button>
                      <button onClick={() => setRenamingId('')}>Cancel</button>
                    </>
                  ) : (
                    <>
                      <button onClick={() => { setRenamingId(g.ID); setRenameValue(g.Name) }}>Rename</button>
                      <button onClick={() => deleteGroup(g.ID)}>Delete</button>
                    </>
                  )
                )}
              </td>
            </tr>
          ))}
          {groups.length === 0 && <tr><td colSpan={3}>No groups yet.</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <form onSubmit={createGroup} className="toolbar">
          <input placeholder="Group name" value={groupName} onChange={(e) => setGroupName(e.target.value)} required />
          <select value={groupCustomerId} onChange={(e) => setGroupCustomerId(e.target.value)}>
            <option value="">- global (all customers) -</option>
            {customers.map((c) => <option key={c.ID} value={c.ID}>{c.Name}</option>)}
          </select>
          <button type="submit">+ Add Group</button>
        </form>
      )}
    </div>
  )
}

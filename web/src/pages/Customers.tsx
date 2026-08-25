import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { CustomerApi, ApiError, type Customer } from '../api'
import { useAuth } from '../AuthContext'

export default function Customers() {
  const { user } = useAuth()
  const [customers, setCustomers] = useState<Customer[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [number, setNumber] = useState('')
  const canManage = user?.permissions.includes('customer.manage')

  async function load() {
    try {
      setCustomers((await CustomerApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load customers')
    }
  }

  useEffect(() => {
    load()
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
    </div>
  )
}

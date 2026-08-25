import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertApi } from '../api'

export default function AlertsBadge() {
  const [count, setCount] = useState(0)

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const res = await AlertApi.openCount()
        if (!cancelled) setCount(res.open_count)
      } catch {
        // permission-gated or transient failure; badge just stays at last known count
      }
    }
    load()
    const interval = setInterval(load, 20000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [])

  return (
    <Link to="/alerts" className="alerts-badge-link">
      Alerts{count > 0 && <span className="alerts-badge-count">{count}</span>}
    </Link>
  )
}

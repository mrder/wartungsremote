import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { SettingsApi, ApiError } from '../api'

export default function Settings() {
  const [rawHours, setRawHours] = useState('')
  const [hourlyHours, setHourlyHours] = useState('')
  const [rotationDays, setRotationDays] = useState('')
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const [rotationSaved, setRotationSaved] = useState(false)

  async function load() {
    try {
      const s = await SettingsApi.getRetention()
      setRawHours(String(s.raw_retention_hours))
      setHourlyHours(String(s.hourly_retention_hours))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load settings')
    }
    try {
      const r = await SettingsApi.getSupportCredentialRotation()
      setRotationDays(String(r.rotation_days))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load settings')
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setSaved(false)
    try {
      await SettingsApi.setRetention(Number(rawHours), Number(hourlyHours))
      setSaved(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save settings')
    }
  }

  async function saveRotation(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setRotationSaved(false)
    try {
      await SettingsApi.setSupportCredentialRotation(Number(rotationDays))
      setRotationSaved(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save settings')
    }
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Settings</h1>

      <h3>Monitoring history retention</h3>
      <p>
        How long raw (high-resolution) and hourly-averaged CPU/RAM/disk history is kept before
        being deleted. Alerts are not affected by this — they're kept indefinitely until manually
        acknowledged/resolved/deleted.
      </p>
      {error && <p className="error">{error}</p>}
      {saved && <p>Saved.</p>}
      <form onSubmit={save} className="toolbar" style={{ flexWrap: 'wrap' }}>
        <label>
          Raw retention (hours):{' '}
          <input
            type="number"
            min="1"
            value={rawHours}
            onChange={(e) => setRawHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {rawHours && !isNaN(Number(rawHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({(Number(rawHours) / 24).toFixed(1)} days)</span>
          )}
        </label>
        <label>
          Hourly-average retention (hours):{' '}
          <input
            type="number"
            min="1"
            value={hourlyHours}
            onChange={(e) => setHourlyHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {hourlyHours && !isNaN(Number(hourlyHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({(Number(hourlyHours) / 24).toFixed(1)} days)</span>
          )}
        </label>
        <button type="submit">Save</button>
      </form>

      <h3>Remote-support account rotation</h3>
      <p>
        Automatically generates and applies a new password for the "remotewartung" account on every
        device on a schedule — it's machine-generated and never seen by anyone, so unlike a human
        login password there's no downside to rotating it, only upside: it limits how long a copy
        would stay useful if the database were ever compromised. Set to 0 to disable (the default —
        rotation still works on demand from a device's Remote tab either way).
      </p>
      {rotationSaved && <p>Saved.</p>}
      <form onSubmit={saveRotation} className="toolbar" style={{ flexWrap: 'wrap' }}>
        <label>
          Rotate every:{' '}
          <input
            type="number"
            min="0"
            value={rotationDays}
            onChange={(e) => setRotationDays(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {' '}days (0 = disabled)
        </label>
        <button type="submit">Save</button>
      </form>
    </div>
  )
}

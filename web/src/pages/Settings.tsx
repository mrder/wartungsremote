import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { SettingsApi, AuditApi, ApiError, type ChainVerification } from '../api'
import { useAuth } from '../AuthContext'

export default function Settings() {
  const { user } = useAuth()
  const [rawHours, setRawHours] = useState('')
  const [hourlyHours, setHourlyHours] = useState('')
  const [networkRawHours, setNetworkRawHours] = useState('')
  const [networkHourlyHours, setNetworkHourlyHours] = useState('')
  const [rotationDays, setRotationDays] = useState('')
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const [networkSaved, setNetworkSaved] = useState(false)
  const [rotationSaved, setRotationSaved] = useState(false)

  const [telegramConfigured, setTelegramConfigured] = useState(false)
  const [telegramUpdatedAt, setTelegramUpdatedAt] = useState('')
  const [botToken, setBotToken] = useState('')
  const [chatId, setChatId] = useState('')
  const [telegramSaved, setTelegramSaved] = useState(false)
  const [telegramTestMsg, setTelegramTestMsg] = useState('')
  const [telegramBusy, setTelegramBusy] = useState(false)

  const [chainResult, setChainResult] = useState<ChainVerification | null>(null)
  const [chainBusy, setChainBusy] = useState(false)

  async function load() {
    try {
      const s = await SettingsApi.getRetention()
      setRawHours(String(s.raw_retention_hours))
      setHourlyHours(String(s.hourly_retention_hours))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load settings')
    }
    try {
      const n = await SettingsApi.getNetworkRetention()
      setNetworkRawHours(String(n.raw_retention_hours))
      setNetworkHourlyHours(String(n.hourly_retention_hours))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load settings')
    }
    try {
      const r = await SettingsApi.getSupportCredentialRotation()
      setRotationDays(String(r.rotation_days))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load settings')
    }
    try {
      const t = await SettingsApi.getTelegram()
      setTelegramConfigured(t.configured)
      setTelegramUpdatedAt(t.updated_at)
      setChatId(t.chat_id ?? '')
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

  async function saveNetworkRetention(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setNetworkSaved(false)
    try {
      await SettingsApi.setNetworkRetention(Number(networkRawHours), Number(networkHourlyHours))
      setNetworkSaved(true)
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

  async function saveTelegram(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setTelegramSaved(false)
    setTelegramTestMsg('')
    try {
      await SettingsApi.setTelegram(botToken, chatId)
      setBotToken('')
      setTelegramSaved(true)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save settings')
    }
  }

  async function verifyChain() {
    setError('')
    setChainResult(null)
    setChainBusy(true)
    try {
      setChainResult(await AuditApi.verifyChain())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to verify audit chain')
    } finally {
      setChainBusy(false)
    }
  }

  async function testTelegram() {
    setError('')
    setTelegramTestMsg('')
    setTelegramBusy(true)
    try {
      await SettingsApi.testTelegram()
      setTelegramTestMsg('Test message sent — check your Telegram chat.')
    } catch (err) {
      setTelegramTestMsg(err instanceof ApiError ? err.message : 'Failed to send test message')
    } finally {
      setTelegramBusy(false)
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

      <h3>Network traffic history retention</h3>
      <p>
        Same idea, but for network traffic history specifically — it's collected far more often
        (roughly once a minute per device, buffered locally on the agent and uploaded in batches)
        than CPU/RAM/disk, so it gets its own, shorter default raw retention to keep the row volume
        reasonable; the hourly rollup still covers the long term.
      </p>
      {networkSaved && <p>Saved.</p>}
      <form onSubmit={saveNetworkRetention} className="toolbar" style={{ flexWrap: 'wrap' }}>
        <label>
          Raw retention (hours):{' '}
          <input
            type="number"
            min="1"
            value={networkRawHours}
            onChange={(e) => setNetworkRawHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {networkRawHours && !isNaN(Number(networkRawHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({(Number(networkRawHours) / 24).toFixed(1)} days)</span>
          )}
        </label>
        <label>
          Hourly-total retention (hours):{' '}
          <input
            type="number"
            min="1"
            value={networkHourlyHours}
            onChange={(e) => setNetworkHourlyHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {networkHourlyHours && !isNaN(Number(networkHourlyHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({(Number(networkHourlyHours) / 24).toFixed(1)} days)</span>
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

      <h3>Telegram alert notifications</h3>
      <p>
        Sends a Telegram message every time a new alert opens (docs/TODO.md lists email/ntfy/webhooks
        as possible future channels — Telegram is the first one built). Setup:
      </p>
      <ol>
        <li>In Telegram, message <strong>@BotFather</strong>, send <code>/newbot</code>, and follow the
          prompts. It gives you a bot token like <code>123456:ABC-DEF1234...</code>.</li>
        <li>Message your new bot directly (or add it to a group and mention it once) so it can see the chat.</li>
        <li>Open <code>https://api.telegram.org/bot&lt;YOUR_TOKEN&gt;/getUpdates</code> in a browser
          (replace <code>&lt;YOUR_TOKEN&gt;</code>) and find <code>"chat":{'{'}"id":...{'}'}</code> in the
          response — that number is your chat ID.</li>
        <li>Paste both below, save, then send a test message to confirm it works.</li>
      </ol>
      {telegramConfigured && (
        <p>
          Currently configured (chat ID <code>{chatId}</code>, last set{' '}
          {telegramUpdatedAt ? new Date(telegramUpdatedAt).toLocaleString() : '-'}). Enter a new bot
          token below only if you want to replace it.
        </p>
      )}
      {telegramSaved && <p>Saved.</p>}
      {telegramTestMsg && <p>{telegramTestMsg}</p>}
      <form onSubmit={saveTelegram} className="toolbar" style={{ flexWrap: 'wrap' }}>
        <input
          type="password"
          placeholder="Bot token"
          value={botToken}
          onChange={(e) => setBotToken(e.target.value)}
          required
          style={{ minWidth: 260 }}
        />
        <input
          placeholder="Chat ID"
          value={chatId}
          onChange={(e) => setChatId(e.target.value)}
          required
          style={{ minWidth: 160 }}
        />
        <button type="submit">Save</button>
        {telegramConfigured && (
          <button type="button" onClick={testTelegram} disabled={telegramBusy}>
            {telegramBusy ? 'Sending...' : 'Send test message'}
          </button>
        )}
      </form>

      {user?.permissions.includes('audit.read') && (
        <>
          <h3>Audit log integrity</h3>
          <p>
            Every audit entry is cryptographically chained to the one before it, so an entry can't be
            edited or deleted afterwards without breaking the chain from that point on — this
            recomputes the whole chain from scratch and checks it against what's stored. Read-only, but
            scans the entire audit log, so it's a manual action rather than something run on every page
            load.
          </p>
          <div className="toolbar">
            <button type="button" onClick={verifyChain} disabled={chainBusy}>
              {chainBusy ? 'Verifying...' : 'Verify chain'}
            </button>
          </div>
          {chainResult && (
            chainResult.Valid ? (
              <p>
                Chain intact — {chainResult.EntriesCheck} entries checked, no tampering detected.
                {chainResult.EntriesPreChain > 0 &&
                  ` (${chainResult.EntriesPreChain} older entries predate this feature and aren't covered by the chain.)`}
              </p>
            ) : (
              <p className="error">
                Chain broken at entry #{chainResult.BrokenAtID} ({chainResult.EntriesCheck} entries
                checked before the break was found) — this entry or one before it no longer matches what
                was originally recorded.
              </p>
            )
          )}
        </>
      )}
    </div>
  )
}

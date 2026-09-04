import { useEffect, useState } from 'react'
import { useTranslation, Trans } from 'react-i18next'
import { SettingsApi, AuditApi, ReauthApi, AccountApi, ApiError, type ChainVerification } from '../api'
import { useAuth } from '../AuthContext'
import { useAppearance, THEMES, LAYOUTS, type Theme, type LayoutMode } from '../AppearanceContext'

export default function Settings() {
  const { t, i18n } = useTranslation()
  const { user } = useAuth()
  const { theme, layout: appearanceLayout, setTheme, setLayout } = useAppearance()

  const [currentPassword, setCurrentPassword] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [accountError, setAccountError] = useState('')
  const [accountBusy, setAccountBusy] = useState(false)
  const [passwordChanged, setPasswordChanged] = useState(false)

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
  const [telegramTestOk, setTelegramTestOk] = useState(false)
  const [telegramBusy, setTelegramBusy] = useState(false)

  const [chainResult, setChainResult] = useState<ChainVerification | null>(null)
  const [chainBusy, setChainBusy] = useState(false)

  async function load() {
    try {
      const s = await SettingsApi.getRetention()
      setRawHours(String(s.raw_retention_hours))
      setHourlyHours(String(s.hourly_retention_hours))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.loadFailed'))
    }
    try {
      const n = await SettingsApi.getNetworkRetention()
      setNetworkRawHours(String(n.raw_retention_hours))
      setNetworkHourlyHours(String(n.hourly_retention_hours))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.loadFailed'))
    }
    try {
      const r = await SettingsApi.getSupportCredentialRotation()
      setRotationDays(String(r.rotation_days))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.loadFailed'))
    }
    try {
      const tg = await SettingsApi.getTelegram()
      setTelegramConfigured(tg.configured)
      setTelegramUpdatedAt(tg.updated_at)
      setChatId(tg.chat_id ?? '')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.loadFailed'))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setSaved(false)
    try {
      await SettingsApi.setRetention(Number(rawHours), Number(hourlyHours))
      setSaved(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.saveFailed'))
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
      setError(err instanceof ApiError ? err.message : t('settings.saveFailed'))
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
      setError(err instanceof ApiError ? err.message : t('settings.saveFailed'))
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
      setError(err instanceof ApiError ? err.message : t('settings.saveFailed'))
    }
  }

  async function verifyChain() {
    setError('')
    setChainResult(null)
    setChainBusy(true)
    try {
      setChainResult(await AuditApi.verifyChain())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('settings.audit.verifyFailed'))
    } finally {
      setChainBusy(false)
    }
  }

  async function changePassword(e: React.FormEvent) {
    e.preventDefault()
    setAccountError('')
    setPasswordChanged(false)
    if (newPassword.length < 12) {
      setAccountError(t('account.passwordTooShort'))
      return
    }
    if (newPassword !== confirmPassword) {
      setAccountError(t('account.passwordMismatch'))
      return
    }
    setAccountBusy(true)
    try {
      const reauth = await ReauthApi.reauth(currentPassword, mfaCode)
      await AccountApi.changePassword(reauth.reauth_id, newPassword)
      setCurrentPassword('')
      setMfaCode('')
      setNewPassword('')
      setConfirmPassword('')
      setPasswordChanged(true)
    } catch (err) {
      setAccountError(err instanceof ApiError ? err.message : t('account.changeFailed'))
    } finally {
      setAccountBusy(false)
    }
  }

  async function testTelegram() {
    setError('')
    setTelegramTestMsg('')
    setTelegramTestOk(false)
    setTelegramBusy(true)
    try {
      await SettingsApi.testTelegram()
      setTelegramTestMsg(t('settings.telegram.testSent'))
      setTelegramTestOk(true)
    } catch (err) {
      setTelegramTestMsg(err instanceof ApiError ? err.message : t('settings.telegram.testFailed'))
    } finally {
      setTelegramBusy(false)
    }
  }

  return (
    <>
      <h1>{t('settings.title')}</h1>
      <p>{t('account.signedInAs')} <code>{user?.username}</code>.</p>

      <section className="settings-group">
        <h2>{t('settings.group.account')}</h2>
        <h3>{t('account.changePassword')}</h3>
        <p>{t('account.changePasswordHint')}</p>
        {accountError && <p className="error">{accountError}</p>}
        {passwordChanged && <p className="success">{t('account.passwordChanged')}</p>}
        <form onSubmit={changePassword} className="field-form">
          <label>
            {t('account.currentPassword')}
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
            />
          </label>
          <label>
            {t('account.mfaCode')}
            <input
              value={mfaCode}
              onChange={(e) => setMfaCode(e.target.value)}
              required
              style={{ width: '6rem' }}
            />
          </label>
          <label>
            {t('account.newPassword')}
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
            />
          </label>
          <label>
            {t('account.confirmNewPassword')}
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
            />
          </label>
          <button type="submit" disabled={accountBusy}>{accountBusy ? t('account.changing') : t('account.changePassword')}</button>
        </form>
      </section>

      <section className="settings-group">
        <h2>{t('settings.group.display')}</h2>
        <h3>{t('settings.language.title')}</h3>
        <p>{t('settings.language.hint')}</p>
        <div className="toolbar">
          <select value={i18n.language} onChange={(e) => i18n.changeLanguage(e.target.value)}>
            <option value="en">English</option>
            <option value="de">Deutsch</option>
          </select>
        </div>

        <h3>{t('settings.appearance.title')}</h3>
        <p>{t('settings.appearance.hint')}</p>
        <div className="field-form">
          <label>
            {t('settings.appearance.theme')}
            <select value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
              {THEMES.map((th) => <option key={th} value={th}>{t(`settings.appearance.themeNames.${th}`)}</option>)}
            </select>
          </label>
          <label>
            {t('settings.appearance.layout')}
            <select value={appearanceLayout} onChange={(e) => setLayout(e.target.value as LayoutMode)}>
              {LAYOUTS.map((l) => <option key={l} value={l}>{t(`settings.appearance.layoutNames.${l}`)}</option>)}
            </select>
          </label>
        </div>
      </section>

      <section className="settings-group">
        <h2>{t('settings.group.monitoring')}</h2>
        <h3>{t('settings.retention.title')}</h3>
        <p>{t('settings.retention.hint')}</p>
        {error && <p className="error">{error}</p>}
        {saved && <p className="success">{t('common.saved')}</p>}
        <form onSubmit={save} className="field-form">
        <label>
          {t('settings.retention.rawHours')}:{' '}
          <input
            type="number"
            min="1"
            value={rawHours}
            onChange={(e) => setRawHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {rawHours && !isNaN(Number(rawHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({t('settings.retention.days', { count: Number((Number(rawHours) / 24).toFixed(1)) })})</span>
          )}
        </label>
        <label>
          {t('settings.retention.hourlyHours')}:{' '}
          <input
            type="number"
            min="1"
            value={hourlyHours}
            onChange={(e) => setHourlyHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {hourlyHours && !isNaN(Number(hourlyHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({t('settings.retention.days', { count: Number((Number(hourlyHours) / 24).toFixed(1)) })})</span>
          )}
        </label>
        <button type="submit">{t('common.save')}</button>
      </form>

        <h3>{t('settings.networkRetention.title')}</h3>
        <p>{t('settings.networkRetention.hint')}</p>
      {networkSaved && <p className="success">{t('common.saved')}</p>}
      <form onSubmit={saveNetworkRetention} className="field-form">
        <label>
          {t('settings.retention.rawHours')}:{' '}
          <input
            type="number"
            min="1"
            value={networkRawHours}
            onChange={(e) => setNetworkRawHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {networkRawHours && !isNaN(Number(networkRawHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({t('settings.retention.days', { count: Number((Number(networkRawHours) / 24).toFixed(1)) })})</span>
          )}
        </label>
        <label>
          {t('settings.networkRetention.hourlyHours')}:{' '}
          <input
            type="number"
            min="1"
            value={networkHourlyHours}
            onChange={(e) => setNetworkHourlyHours(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {networkHourlyHours && !isNaN(Number(networkHourlyHours)) && (
            <span style={{ marginLeft: '0.5rem' }}>({t('settings.retention.days', { count: Number((Number(networkHourlyHours) / 24).toFixed(1)) })})</span>
          )}
        </label>
        <button type="submit">{t('common.save')}</button>
      </form>
      </section>

      <section className="settings-group">
      <h2>{t('settings.group.security')}</h2>
      <h3>{t('settings.rotation.title')}</h3>
      <p>{t('settings.rotation.hint')}</p>
      {rotationSaved && <p className="success">{t('common.saved')}</p>}
      <form onSubmit={saveRotation} className="field-form">
        <label>
          {t('settings.rotation.rotateEvery')}:{' '}
          <input
            type="number"
            min="0"
            value={rotationDays}
            onChange={(e) => setRotationDays(e.target.value)}
            required
            style={{ width: '6rem' }}
          />
          {' '}{t('settings.rotation.daysDisabled')}
        </label>
        <button type="submit">{t('common.save')}</button>
      </form>

      {user?.permissions.includes('audit.read') && (
        <>
          <h3>{t('settings.audit.title')}</h3>
          <p>{t('settings.audit.hint')}</p>
          <div className="toolbar">
            <button type="button" onClick={verifyChain} disabled={chainBusy}>
              {chainBusy ? t('settings.audit.verifying') : t('settings.audit.verifyChain')}
            </button>
          </div>
          {chainResult && (
            chainResult.Valid ? (
              <p className="success">
                {t('settings.audit.chainIntact', { count: chainResult.EntriesCheck })}
                {chainResult.EntriesPreChain > 0 &&
                  ' ' + t('settings.audit.preChainNote', { count: chainResult.EntriesPreChain })}
              </p>
            ) : (
              <p className="error">
                {t('settings.audit.chainBroken', { id: chainResult.BrokenAtID, count: chainResult.EntriesCheck })}
              </p>
            )
          )}
        </>
      )}
      </section>

      <section className="settings-group">
      <h2>{t('settings.group.notifications')}</h2>
      <h3>{t('settings.telegram.title')}</h3>
      <p>{t('settings.telegram.hint')}</p>
      <ol>
        <li><Trans i18nKey="settings.telegram.step1"><strong>@BotFather</strong><code>/newbot</code></Trans></li>
        <li>{t('settings.telegram.step2')}</li>
        <li><Trans i18nKey="settings.telegram.step3"><code>https://api.telegram.org/bot&lt;YOUR_TOKEN&gt;/getUpdates</code><code>&lt;YOUR_TOKEN&gt;</code><code>"chat":{'{'}"id":...{'}'}</code></Trans></li>
        <li>{t('settings.telegram.step4')}</li>
      </ol>
      {telegramConfigured && (
        <p>
          {t('settings.telegram.currentlyConfigured', { chatId, updatedAt: telegramUpdatedAt ? new Date(telegramUpdatedAt).toLocaleString() : '-' })}
        </p>
      )}
      {telegramSaved && <p className="success">{t('common.saved')}</p>}
      {telegramTestMsg && <p className={telegramTestOk ? 'success' : 'error'}>{telegramTestMsg}</p>}
      <form onSubmit={saveTelegram} className="field-form">
        <input
          type="password"
          placeholder={t('settings.telegram.botToken')}
          value={botToken}
          onChange={(e) => setBotToken(e.target.value)}
          required
          style={{ minWidth: 260 }}
        />
        <input
          placeholder={t('settings.telegram.chatId')}
          value={chatId}
          onChange={(e) => setChatId(e.target.value)}
          required
          style={{ minWidth: 160 }}
        />
        <button type="submit">{t('common.save')}</button>
        {telegramConfigured && (
          <button type="button" onClick={testTelegram} disabled={telegramBusy}>
            {telegramBusy ? t('settings.telegram.sending') : t('settings.telegram.sendTest')}
          </button>
        )}
      </form>
      </section>
    </>
  )
}

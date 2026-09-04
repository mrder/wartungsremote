import { useEffect, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { ReleaseApi, ApiError, type AgentRelease } from '../api'
import { useAuth } from '../AuthContext'

export default function Releases() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const canManage = user?.permissions.includes('agent.update')
  const [releases, setReleases] = useState<AgentRelease[]>([])
  const [error, setError] = useState('')

  const [version, setVersion] = useState('')
  const [osFamily, setOsFamily] = useState('windows')
  const [architecture, setArchitecture] = useState('amd64')
  const [channel, setChannel] = useState('stable')
  const [artifactURL, setArtifactURL] = useState('')
  const [sha256, setSha256] = useState('')
  const [signature, setSignature] = useState('')

  const [syncBusy, setSyncBusy] = useState(false)
  const [syncResult, setSyncResult] = useState<{ imported: number; skipped: number; errors: string[] } | null>(null)

  async function load() {
    try {
      setReleases((await ReleaseApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('releases.loadFailed'))
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function create(e: React.FormEvent) {
    e.preventDefault()
    try {
      await ReleaseApi.create({
        version, os_family: osFamily, architecture, channel,
        artifact_url: artifactURL, artifact_sha256_hex: sha256, signature_base64: signature,
      })
      setVersion(''); setArtifactURL(''); setSha256(''); setSignature('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('releases.createFailed'))
    }
  }

  async function syncFromGitHub() {
    setError('')
    setSyncResult(null)
    setSyncBusy(true)
    try {
      setSyncResult(await ReleaseApi.syncFromGitHub())
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('releases.syncFailed'))
    } finally {
      setSyncBusy(false)
    }
  }

  async function toggleBlocked(id: string, blocked: boolean) {
    try {
      await ReleaseApi.setBlocked(id, blocked)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('releases.updateFailed'))
    }
  }

  return (
    <>
      <h1>{t('releases.title')}</h1>
      <p>
        <Trans i18nKey="releases.hint"><code>wr-release-sign</code><code>WR_RELEASE_PUBLIC_KEY_FILE</code></Trans>
      </p>
      {error && <p className="error">{error}</p>}

      {canManage && (
        <>
          <div className="toolbar">
            <button onClick={syncFromGitHub} disabled={syncBusy}>{syncBusy ? t('common.loading') : t('releases.syncFromGitHub')}</button>
          </div>
          {syncResult && (
            <p>
              {t('releases.syncResult', { imported: syncResult.imported, skipped: syncResult.skipped })}
              {syncResult.errors.length > 0 && (
                <>
                  <br />
                  {syncResult.errors.join('; ')}
                </>
              )}
            </p>
          )}
        </>
      )}

      <table className="device-table">
        <thead><tr><th>{t('releases.version')}</th><th>{t('deviceList.os')}</th><th>{t('releases.arch')}</th><th>{t('releases.channel')}</th><th>{t('releases.published')}</th><th>{t('releases.minSupported')}</th><th>{t('releases.blocked')}</th><th></th></tr></thead>
        <tbody>
          {releases.map((r) => (
            <tr key={r.ID}>
              <td>{r.Version}</td>
              <td>{r.OSFamily}</td>
              <td>{r.Architecture}</td>
              <td>{r.Channel}</td>
              <td>{new Date(r.PublishedAt).toLocaleString()}</td>
              <td>{r.MinimumSupported ? t('common.yes') : t('common.no')}</td>
              <td>{r.Blocked ? t('common.yes') : t('common.no')}</td>
              <td>
                {canManage && (
                  <button onClick={() => toggleBlocked(r.ID, !r.Blocked)}>{r.Blocked ? t('releases.unblock') : t('releases.block')}</button>
                )}
              </td>
            </tr>
          ))}
          {releases.length === 0 && <tr><td colSpan={8}>{t('releases.noReleases')}</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <>
          <h3>{t('releases.manualTitle')}</h3>
          <p>{t('releases.manualHint')}</p>
          <form onSubmit={create} className="field-form">
            <label>
              {t('releases.version')}
              <input placeholder={t('releases.versionPlaceholder')} value={version} onChange={(e) => setVersion(e.target.value)} required style={{ width: '8rem' }} />
            </label>
            <label>
              {t('deviceList.os')}
              <select value={osFamily} onChange={(e) => setOsFamily(e.target.value)}>
                <option value="windows">windows</option>
                <option value="linux">linux</option>
              </select>
            </label>
            <label>
              {t('releases.arch')}
              <select value={architecture} onChange={(e) => setArchitecture(e.target.value)}>
                <option value="amd64">amd64</option>
                <option value="arm64">arm64</option>
              </select>
            </label>
            <label>
              {t('releases.channel')}
              <select value={channel} onChange={(e) => setChannel(e.target.value)}>
                <option value="stable">stable</option>
                <option value="beta">beta</option>
              </select>
            </label>
            <label style={{ flex: '1 1 260px' }}>
              {t('releases.artifactUrl')}
              <input value={artifactURL} onChange={(e) => setArtifactURL(e.target.value)} required />
            </label>
            <label style={{ flex: '1 1 260px' }}>
              {t('releases.sha256')}
              <input value={sha256} onChange={(e) => setSha256(e.target.value)} required />
            </label>
            <label style={{ flex: '1 1 260px' }}>
              {t('releases.signature')}
              <input value={signature} onChange={(e) => setSignature(e.target.value)} required />
            </label>
            <button type="submit">{t('releases.publish')}</button>
          </form>
        </>
      )}
    </>
  )
}

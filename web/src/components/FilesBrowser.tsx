import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileApi, ApiError, type FileEntry } from '../api'

function joinPath(base: string, name: string): string {
  if (base.endsWith('/') || base.endsWith('\\')) return base + name
  const sep = base.includes('\\') && !base.includes('/') ? '\\' : '/'
  return base + sep + name
}

export default function FilesBrowser({ deviceId, defaultPath }: { deviceId: string; defaultPath: string }) {
  const { t } = useTranslation()
  const [path, setPath] = useState(defaultPath)
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  async function load(p: string) {
    setLoading(true)
    setError('')
    try {
      const list = await FileApi.list(deviceId, p)
      setEntries(list ?? [])
      setPath(p)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('filesBrowser.listFailed'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(defaultPath)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleMkdir() {
    const name = prompt(t('filesBrowser.newFolderName'))
    if (!name) return
    try {
      await FileApi.mkdir(deviceId, joinPath(path, name))
      load(path)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('filesBrowser.createFolderFailed'))
    }
  }

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      await FileApi.upload(deviceId, joinPath(path, file.name), file)
      load(path)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('filesBrowser.uploadFailed'))
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  return (
    <div>
      <div className="toolbar">
        <input value={path} onChange={(e) => setPath(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load(path)} />
        <button onClick={() => load(path)}>{t('filesBrowser.go')}</button>
        <button onClick={handleMkdir}>{t('filesBrowser.newFolder')}</button>
        <input ref={fileInputRef} type="file" onChange={handleUpload} style={{ maxWidth: 180 }} />
      </div>
      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>{t('common.loading')}</p>
      ) : (
        <table className="device-table">
          <thead><tr><th>{t('deviceList.name')}</th><th>{t('filesBrowser.size')}</th><th>{t('filesBrowser.modified')}</th></tr></thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.name}>
                <td>
                  {e.is_dir ? (
                    <a href="#" onClick={(ev) => { ev.preventDefault(); load(joinPath(path, e.name)) }}>{e.name}/</a>
                  ) : (
                    <a href={FileApi.downloadUrl(deviceId, joinPath(path, e.name))}>{e.name}</a>
                  )}
                </td>
                <td>{e.is_dir ? '-' : `${(e.size / 1024).toFixed(1)} KB`}</td>
                <td>{new Date(e.mod_time_unix_ms).toLocaleString()}</td>
              </tr>
            ))}
            {entries.length === 0 && <tr><td colSpan={3}>{t('filesBrowser.emptyDirectory')}</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

import { useEffect, useRef, useState } from 'react'
import { FileApi, ApiError, type FileEntry } from '../api'

function joinPath(base: string, name: string): string {
  if (base.endsWith('/') || base.endsWith('\\')) return base + name
  const sep = base.includes('\\') && !base.includes('/') ? '\\' : '/'
  return base + sep + name
}

export default function FilesBrowser({ deviceId, defaultPath }: { deviceId: string; defaultPath: string }) {
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
      setError(err instanceof ApiError ? err.message : 'Failed to list directory')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(defaultPath)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleMkdir() {
    const name = prompt('New folder name:')
    if (!name) return
    try {
      await FileApi.mkdir(deviceId, joinPath(path, name))
      load(path)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Create folder failed')
    }
  }

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      await FileApi.upload(deviceId, joinPath(path, file.name), file)
      load(path)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Upload failed')
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  return (
    <div>
      <div className="toolbar">
        <input value={path} onChange={(e) => setPath(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && load(path)} />
        <button onClick={() => load(path)}>Go</button>
        <button onClick={handleMkdir}>+ Folder</button>
        <input ref={fileInputRef} type="file" onChange={handleUpload} style={{ maxWidth: 180 }} />
      </div>
      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>Loading...</p>
      ) : (
        <table className="device-table">
          <thead><tr><th>Name</th><th>Size</th><th>Modified</th></tr></thead>
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
            {entries.length === 0 && <tr><td colSpan={3}>Empty directory.</td></tr>}
          </tbody>
        </table>
      )}
    </div>
  )
}

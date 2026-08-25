import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ReleaseApi, ApiError, type AgentRelease } from '../api'
import { useAuth } from '../AuthContext'

export default function Releases() {
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

  async function load() {
    try {
      setReleases((await ReleaseApi.list()) ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load releases')
    }
  }

  useEffect(() => {
    load()
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
      setError(err instanceof ApiError ? err.message : 'Failed to create release')
    }
  }

  async function toggleBlocked(id: string, blocked: boolean) {
    try {
      await ReleaseApi.setBlocked(id, blocked)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update release')
    }
  }

  return (
    <div className="page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Agent Releases</h1>
      <p>
        Release artifacts are signed offline with <code>wr-release-sign</code> — this server never holds a
        production signing key. Submissions are verified against the configured trusted public key
        (<code>WR_RELEASE_PUBLIC_KEY_FILE</code>) before being accepted.
      </p>
      {error && <p className="error">{error}</p>}

      <table className="device-table">
        <thead><tr><th>Version</th><th>OS</th><th>Arch</th><th>Channel</th><th>Published</th><th>Min. supported</th><th>Blocked</th><th></th></tr></thead>
        <tbody>
          {releases.map((r) => (
            <tr key={r.ID}>
              <td>{r.Version}</td>
              <td>{r.OSFamily}</td>
              <td>{r.Architecture}</td>
              <td>{r.Channel}</td>
              <td>{new Date(r.PublishedAt).toLocaleString()}</td>
              <td>{r.MinimumSupported ? 'yes' : 'no'}</td>
              <td>{r.Blocked ? 'yes' : 'no'}</td>
              <td>
                {canManage && (
                  <button onClick={() => toggleBlocked(r.ID, !r.Blocked)}>{r.Blocked ? 'Unblock' : 'Block'}</button>
                )}
              </td>
            </tr>
          ))}
          {releases.length === 0 && <tr><td colSpan={8}>No releases published yet.</td></tr>}
        </tbody>
      </table>

      {canManage && (
        <form onSubmit={create} className="toolbar" style={{ flexWrap: 'wrap' }}>
          <input placeholder="Version (e.g. 0.2.0)" value={version} onChange={(e) => setVersion(e.target.value)} required />
          <select value={osFamily} onChange={(e) => setOsFamily(e.target.value)}>
            <option value="windows">windows</option>
            <option value="linux">linux</option>
          </select>
          <select value={architecture} onChange={(e) => setArchitecture(e.target.value)}>
            <option value="amd64">amd64</option>
            <option value="arm64">arm64</option>
          </select>
          <input placeholder="Channel" value={channel} onChange={(e) => setChannel(e.target.value)} />
          <input placeholder="Artifact URL" value={artifactURL} onChange={(e) => setArtifactURL(e.target.value)} required style={{ minWidth: 260 }} />
          <input placeholder="SHA-256 (hex)" value={sha256} onChange={(e) => setSha256(e.target.value)} required style={{ minWidth: 260 }} />
          <input placeholder="Signature (base64)" value={signature} onChange={(e) => setSignature(e.target.value)} required style={{ minWidth: 260 }} />
          <button type="submit">+ Publish release</button>
        </form>
      )}
    </div>
  )
}

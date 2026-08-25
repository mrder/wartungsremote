import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { HelpApi, ApiError, type HelpIndexEntry, type HelpPage } from '../api'

export default function Help() {
  const { slug } = useParams<{ slug?: string }>()
  const [index, setIndex] = useState<HelpIndexEntry[]>([])
  const [page, setPage] = useState<HelpPage | null>(null)
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    HelpApi.index().then((v) => setIndex(v ?? [])).catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load help index'))
  }, [])

  useEffect(() => {
    if (!slug) {
      setPage(null)
      return
    }
    HelpApi.page(slug).then(setPage).catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load help page'))
  }, [slug])

  const filtered = query
    ? index.filter((e) => e.title.toLowerCase().includes(query.toLowerCase()))
    : index

  return (
    <div className="page help-page">
      <Link to="/">&larr; Back to devices</Link>
      <h1>Help</h1>
      {error && <p className="error">{error}</p>}

      <div className="help-layout">
        <nav className="help-nav">
          <input placeholder="Search help..." value={query} onChange={(e) => setQuery(e.target.value)} />
          <ul>
            {filtered.map((e) => (
              <li key={e.slug}>
                <Link to={`/help/${e.slug}`} className={slug === e.slug ? 'active' : ''}>{e.title}</Link>
              </li>
            ))}
            {filtered.length === 0 && <li>No matching topics.</li>}
          </ul>
        </nav>
        <article className="help-content">
          {!slug && <p>Select a topic on the left.</p>}
          {page && (
            <>
              <h2>{page.title}</h2>
              <div dangerouslySetInnerHTML={{ __html: page.html }} />
            </>
          )}
        </article>
      </div>
    </div>
  )
}

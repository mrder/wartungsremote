import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { HelpApi, ApiError, type HelpIndexEntry, type HelpPage } from '../api'

export default function Help() {
  const { t } = useTranslation()
  const { slug } = useParams<{ slug?: string }>()
  const [index, setIndex] = useState<HelpIndexEntry[]>([])
  const [page, setPage] = useState<HelpPage | null>(null)
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    HelpApi.index().then((v) => setIndex(v ?? [])).catch((err) => setError(err instanceof ApiError ? err.message : t('help.loadIndexFailed')))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!slug) {
      setPage(null)
      return
    }
    HelpApi.page(slug).then(setPage).catch((err) => setError(err instanceof ApiError ? err.message : t('help.loadPageFailed')))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slug])

  const filtered = query
    ? index.filter((e) => e.title.toLowerCase().includes(query.toLowerCase()))
    : index

  return (
    <div className="help-page">
      <h1>{t('nav.help')}</h1>
      {error && <p className="error">{error}</p>}

      <div className="help-layout">
        <nav className="help-nav">
          <input placeholder={t('help.searchPlaceholder')} value={query} onChange={(e) => setQuery(e.target.value)} />
          <ul>
            {filtered.map((e) => (
              <li key={e.slug}>
                <Link to={`/help/${e.slug}`} className={slug === e.slug ? 'active' : ''}>{e.title}</Link>
              </li>
            ))}
            {filtered.length === 0 && <li>{t('help.noMatches')}</li>}
          </ul>
        </nav>
        <article className="help-content">
          {!slug && <p>{t('help.selectTopic')}</p>}
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

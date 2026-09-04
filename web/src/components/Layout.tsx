import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../AuthContext'
import { useAppearance } from '../AppearanceContext'
import AlertsBadge from './AlertsBadge'
import InactivityLogout from './InactivityLogout'

export default function Layout({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  const { layout } = useAppearance()
  const location = useLocation()

  function navClass(path: string) {
    return location.pathname === path || (path !== '/' && location.pathname.startsWith(path)) ? 'active' : ''
  }

  const navLinks = (
    <>
      <Link to="/customers" className={navClass('/customers')}>{t('nav.customers')}</Link>
      {user?.permissions.includes('monitoring.read') && <Link to="/network-usage" className={navClass('/network-usage')}>{t('nav.networkUsage')}</Link>}
      {user?.permissions.includes('agent.update') && <Link to="/releases" className={navClass('/releases')}>{t('nav.releases')}</Link>}
      {user?.permissions.includes('user.manage') && <Link to="/users" className={navClass('/users')}>{t('nav.users')}</Link>}
      {user?.permissions.includes('system.settings') && <Link to="/settings" className={navClass('/settings')}>{t('nav.settings')}</Link>}
      <Link to="/help" className={navClass('/help')}>{t('nav.help')}</Link>
      <AlertsBadge />
    </>
  )

  const account = (
    <div className="topbar-account">
      <span className="topbar-username">{user?.username}</span>
      <InactivityLogout />
    </div>
  )

  if (layout === 'sidebar') {
    return (
      <div className="app-with-sidebar">
        <aside className="sidebar">
          <Link to="/" className="sidebar-logo"><h1>WartungsRemote</h1></Link>
          <nav className="sidebar-nav">{navLinks}</nav>
          <div className="sidebar-footer">
            {account}
            <button onClick={logout}>{t('nav.logout')}</button>
          </div>
        </aside>
        <div className="page sidebar-main">{children}</div>
      </div>
    )
  }

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/"><h1>WartungsRemote</h1></Link>
        <div>
          {navLinks}
          {account}
          <button onClick={logout}>{t('nav.logout')}</button>
        </div>
      </header>
      {children}
    </div>
  )
}

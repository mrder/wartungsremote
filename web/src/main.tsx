import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import './i18n'
import App from './App.tsx'
import { AppearanceProvider } from './AppearanceContext'

// Applied synchronously, before React mounts, to avoid a flash of the
// default theme — this is a same-origin module script, so it's unaffected
// by the strict CSP that blocks inline <script>/<style> elsewhere.
try {
  document.documentElement.setAttribute('data-theme', localStorage.getItem('wr_theme') || 'dark')
  document.documentElement.setAttribute('data-layout', localStorage.getItem('wr_layout') || 'topnav')
} catch {
  // ignore — private browsing / storage disabled just keeps the CSS defaults
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AppearanceProvider>
        <App />
      </AppearanceProvider>
    </BrowserRouter>
  </StrictMode>,
)

import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

export type Theme = 'dark' | 'light' | 'scifi' | 'modern' | 'classic' | 'retro'
export type LayoutMode = 'topnav' | 'sidebar'

export const THEMES: Theme[] = ['dark', 'light', 'scifi', 'modern', 'classic', 'retro']
export const LAYOUTS: LayoutMode[] = ['topnav', 'sidebar']

interface AppearanceState {
  theme: Theme
  layout: LayoutMode
  setTheme: (t: Theme) => void
  setLayout: (l: LayoutMode) => void
}

const AppearanceContext = createContext<AppearanceState | null>(null)

function readStored<T extends string>(key: string, fallback: T, valid: readonly T[]): T {
  try {
    const v = localStorage.getItem(key)
    if (v && (valid as readonly string[]).includes(v)) return v as T
  } catch {
    // ignore — private browsing / storage disabled falls back to default
  }
  return fallback
}

export function AppearanceProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => readStored('wr_theme', 'dark', THEMES))
  const [layout, setLayoutState] = useState<LayoutMode>(() => readStored('wr_layout', 'topnav', LAYOUTS))

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  useEffect(() => {
    document.documentElement.setAttribute('data-layout', layout)
  }, [layout])

  function setTheme(t: Theme) {
    setThemeState(t)
    try {
      localStorage.setItem('wr_theme', t)
    } catch {
      // ignore
    }
  }

  function setLayout(l: LayoutMode) {
    setLayoutState(l)
    try {
      localStorage.setItem('wr_layout', l)
    } catch {
      // ignore
    }
  }

  return <AppearanceContext.Provider value={{ theme, layout, setTheme, setLayout }}>{children}</AppearanceContext.Provider>
}

export function useAppearance() {
  const ctx = useContext(AppearanceContext)
  if (!ctx) throw new Error('useAppearance must be used within AppearanceProvider')
  return ctx
}

import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { AuthApi, type Me } from './api'

interface AuthState {
  user: Me | null
  loading: boolean
  refresh: () => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    try {
      const me = await AuthApi.me()
      setUser(me)
    } catch {
      setUser(null)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  const logout = async () => {
    await AuthApi.logout()
    setUser(null)
  }

  return <AuthContext.Provider value={{ user, loading, refresh, logout }}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  clearAuthToken,
  getAuthToken,
  getCurrentUser,
  login as loginRequest,
  setAuthToken,
} from '../api/client'
import type { AuthUser } from '../types'

interface AuthContextValue {
  user: AuthUser | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false

    async function validateStoredToken() {
      const token = getAuthToken()
      if (!token) {
        setLoading(false)
        return
      }
      try {
        const currentUser = await getCurrentUser()
        if (!cancelled) setUser(currentUser)
      } catch {
        clearAuthToken()
        if (!cancelled) setUser(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    validateStoredToken()
    return () => {
      cancelled = true
    }
  }, [])

  async function login(username: string, password: string) {
    const res = await loginRequest(username, password)
    setAuthToken(res.token)
    setUser(res.user)
  }

  function logout() {
    clearAuthToken()
    setUser(null)
  }

  const value = useMemo<AuthContextValue>(
    () => ({ user, loading, login, logout }),
    [user, loading],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) {
    throw new Error('useAuth must be used inside AuthProvider')
  }
  return value
}

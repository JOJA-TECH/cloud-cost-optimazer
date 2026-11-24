'use client'

import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react'
import { login as loginApi, logout as logoutApi, getToken, getUserName, getUserEmail, isTokenExpiringSoon, refreshToken } from '@/lib/auth'

interface AuthContextType {
  token: string | null
  userName: string | null
  userEmail: string | null
  isAuthenticated: boolean
  isClient: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(null)
  const [userName, setUserName] = useState<string | null>(null)
  const [userEmail, setUserEmail] = useState<string | null>(null)
  const [isClient, setIsClient] = useState(false)

  // Solo leer de sessionStorage después de la hidratación
  useEffect(() => {
    setToken(getToken())
    setUserName(getUserName())
    setUserEmail(getUserEmail())
    setIsClient(true)
  }, [])

  const doLogin = useCallback(
    async (email: string, password: string) => {
      const data = await loginApi({ email, password })
      setToken(data.access_token)
      setUserName(data.display_name || data.email.split('@')[0] || 'Usuario')
      setUserEmail(data.email)
    },
    [],
  )

  const doLogout = useCallback(() => {
    logoutApi()
    setToken(null)
    setUserName(null)
    setUserEmail(null)
  }, [])

  // Auto-refresh del token antes de que expire (cada 30 segundos verificar)
  useEffect(() => {
    if (!token || !isClient) return

    const interval = setInterval(async () => {
      if (isTokenExpiringSoon(2 * 60 * 1000)) {
        const success = await refreshToken()
        if (success) {
          setToken(getToken())
        }
      }
    }, 30 * 1000)

    return () => clearInterval(interval)
  }, [token, isClient])

  // Durante la hidratación inicial, no renderizar nada para evitar mismatch
  if (!isClient) {
    return null
  }

  return (
    <AuthContext.Provider
      value={{
        token,
        userName,
        userEmail,
        isAuthenticated: !!token,
        isClient,
        login: doLogin,
        logout: doLogout,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}

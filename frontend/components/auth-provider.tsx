'use client'

import { createContext, useContext, useState, useCallback, ReactNode } from 'react'
import { login as loginApi, logout as logoutApi, getToken, getUserName, getUserEmail } from '@/lib/auth'

interface AuthContextType {
  token: string | null
  userName: string | null
  userEmail: string | null
  isAuthenticated: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(getToken())
  const [userName, setUserName] = useState<string | null>(getUserName())
  const [userEmail, setUserEmail] = useState<string | null>(getUserEmail())

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

  return (
    <AuthContext.Provider
      value={{
        token,
        userName,
        userEmail,
        isAuthenticated: !!token,
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

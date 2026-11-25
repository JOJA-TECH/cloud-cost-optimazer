'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { apiJson } from '@/lib/api'
import { LoginResponse } from '@/lib/auth'
import { useAuth } from '@/components/auth-provider'

export default function OIDCCallbackPage() {
  const router = useRouter()
  const { restoreSession } = useAuth()

  useEffect(() => {
    let active = true
    apiJson<LoginResponse>('/auth/oidc/session')
      .then((data) => {
        if (!active) return
        restoreSession(data)
        router.replace('/dashboard')
      })
      .catch(() => {
        if (active) router.replace('/login')
      })
    return () => {
      active = false
    }
  }, [restoreSession, router])

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <p className="text-sm text-muted-foreground">Finishing Microsoft sign-in…</p>
    </div>
  )
}

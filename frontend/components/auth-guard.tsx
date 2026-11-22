'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useAuth } from '@/components/auth-provider'

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth()
  const [checked, setChecked] = useState(false)
  const router = useRouter()

  useEffect(() => {
    // Verificar en montaje inicial
    checkAuth()

    // Verificar en cada cambio de ruta (incluye botón atrás/adelante del navegador)
    const unsubscribe = router.events.on('routeChangeComplete', () => {
      checkAuth()
    })

    // También interceptar popstate (botón atrás/adelante)
    const handlePopState = () => {
      checkAuth()
    }
    window.addEventListener('popstate', handlePopState)

    return () => {
      unsubscribe()
      window.removeEventListener('popstate', handlePopState)
    }
  }, [isAuthenticated, router])

  function checkAuth() {
    if (!isAuthenticated) {
      router.push('/login')
    }
    setChecked(true)
  }

  if (!checked) return null
  return <>{children}</>
}

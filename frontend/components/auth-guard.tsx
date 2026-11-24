'use client'

import { useEffect, useState } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { useAuth } from '@/components/auth-provider'

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth()
  const [checked, setChecked] = useState(false)
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    // usePathname se actualiza en las navegaciones del App Router, incluso atrás/adelante.
    if (!isAuthenticated) {
      router.replace('/login')
      return
    }
    setChecked(true)
  }, [isAuthenticated, pathname, router])

  if (!checked || !isAuthenticated) return null
  return <>{children}</>
}

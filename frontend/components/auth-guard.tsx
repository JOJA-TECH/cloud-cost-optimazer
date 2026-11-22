'use client'

import { useEffect, useState } from 'react'
import { isAuthenticated } from '@/lib/auth'
import { useRouter } from 'next/navigation'

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const [checked, setChecked] = useState(false)
  const router = useRouter()

  useEffect(() => {
    if (!isAuthenticated()) {
      router.push('/login')
    }
    setChecked(true)
  }, [router])

  if (!checked) return null
  return <>{children}</>
}

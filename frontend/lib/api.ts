import { getToken, refreshToken, isTokenExpiringSoon, getProjectId } from './auth'
import { isDemoMode } from './demo-mode'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

let isRefreshing = false
let refreshPromise: Promise<string | null> | null = null

async function getValidToken(): Promise<string | null> {
  const token = getToken()
  if (!token) return null

  // Si el token está próximo a expirar, renovar antes de usar
  if (isTokenExpiringSoon()) {
    // Evitar refreshs concurrentes
    if (isRefreshing && refreshPromise) {
      return refreshPromise
    }
    isRefreshing = true
    refreshPromise = (async () => {
      const success = await refreshToken()
      isRefreshing = false
      refreshPromise = null
      return success ? getToken() : null
    })()
    return refreshPromise
  }

  return token
}

export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  if (isDemoMode() && !String(input).startsWith('/auth/')) {
    throw new Error('El modo demostración está aislado del backend. Sal de la demo para consultar datos reales.')
  }
  const token = await getValidToken()
  const projectId = getProjectId()

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> || {}),
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  if (projectId) {
    headers['X-Project-ID'] = projectId
  }

  const res = await fetch(`${API_URL}${input}`, {
    ...init,
    headers,
  })

  if (res.status === 401) {
    // Intentar renovar el token una vez
    const refreshed = await refreshToken()
    if (refreshed) {
      // Reintentar la solicitud con el nuevo token
      const newToken = getToken()
      const newHeaders: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(init?.headers as Record<string, string> || {}),
      }
      if (newToken) {
        newHeaders['Authorization'] = `Bearer ${newToken}`
      }
      const retryRes = await fetch(`${API_URL}${input}`, {
        ...init,
        headers: newHeaders,
      })
      return retryRes
    }

    // Si el refresh falló, redirigir a login
    if (typeof window !== 'undefined') {
      window.location.href = '/login'
    }
  }

  return res
}

export async function apiJson<T>(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<T> {
  const res = await apiFetch(input, init)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.message || `Error ${res.status}`)
  }
  return (await res.json()) as T
}

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

async function getToken(): Promise<string | null> {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('cco_access_token')
}

export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const token = await getToken()

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> || {}),
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${API_URL}${input}`, {
    ...init,
    headers,
  })

  if (res.status === 401) {
    localStorage.removeItem('cco_access_token')
    localStorage.removeItem('cco_refresh_token')
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

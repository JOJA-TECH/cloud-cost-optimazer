import { apiJson } from './api'

// ─────────────────────────────────────────────────────────────────────────────
// Estado de autenticación
// - access_token y refresh_token: sessionStorage (persiste entre recargas)
// - Se renueva automáticamente antes de expirar (TTL 15 min del backend)
// ─────────────────────────────────────────────────────────────────────────────

const ACCESS_TOKEN_KEY = 'cco_access_token'
const REFRESH_TOKEN_KEY = 'cco_refresh_token'
const EXPIRES_AT_KEY = 'cco_expires_at'
const PROJECT_ID_KEY = 'cco_project_id'

let currentToken: string | null = null

export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  access_token: string
  refresh_token: string
  expires_at: number
  user_id: string
  email: string
  display_name: string
  org_id: string
  org_role: string
}

export interface RegisterRequest {
  email: string
  display_name: string
  password: string
}

export async function login(req: LoginRequest): Promise<LoginResponse> {
  const data = await apiJson<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(req),
  })
  // Persistir en sessionStorage para que sobreviva a recargas de página
  if (typeof window !== 'undefined') {
    sessionStorage.setItem(ACCESS_TOKEN_KEY, data.access_token)
    sessionStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token)
    sessionStorage.setItem(EXPIRES_AT_KEY, String(data.expires_at))
    sessionStorage.setItem('cco_user_name', data.display_name || data.email.split('@')[0] || 'User')
    sessionStorage.setItem('cco_user_email', data.email)
  }
  currentToken = data.access_token
  return data
}

export async function register(req: RegisterRequest) {
  return apiJson('/auth/register', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export function getToken(): string | null {
  if (typeof window === 'undefined') return currentToken
  // Leer de sessionStorage si no está en memoria (ej. después de recarga)
  if (!currentToken) {
    currentToken = sessionStorage.getItem(ACCESS_TOKEN_KEY)
  }
  return currentToken
}

export function getRefreshToken(): string | null {
  if (typeof window === 'undefined') return null
  return sessionStorage.getItem(REFRESH_TOKEN_KEY)
}

export function getExpiresAt(): number | null {
  if (typeof window === 'undefined') return null
  const v = sessionStorage.getItem(EXPIRES_AT_KEY)
  return v ? Number(v) : null
}

export function isTokenExpiringSoon(thresholdMs: number = 2 * 60 * 1000): boolean {
  const expiresAt = getExpiresAt()
  if (!expiresAt) return true
  return Date.now() + thresholdMs >= expiresAt
}

export async function refreshToken(): Promise<boolean> {
  const refreshTokenVal = getRefreshToken()
  if (!refreshTokenVal) return false

  try {
    const data = await apiJson<LoginResponse>('/auth/token/refresh', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshTokenVal }),
    })
    if (typeof window !== 'undefined') {
      sessionStorage.setItem(ACCESS_TOKEN_KEY, data.access_token)
      sessionStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token)
      sessionStorage.setItem(EXPIRES_AT_KEY, String(data.expires_at))
    }
    currentToken = data.access_token
    return true
  } catch {
    return false
  }
}

export function getUserName(): string | null {
  if (typeof window !== 'undefined') {
    return sessionStorage.getItem('cco_user_name') ?? null
  }
  return null
}

export function getUserEmail(): string | null {
  if (typeof window !== 'undefined') {
    return sessionStorage.getItem('cco_user_email') ?? null
  }
  return null
}

export function getProjectId(): string | null {
  if (typeof window === 'undefined') return null
  return sessionStorage.getItem(PROJECT_ID_KEY) ?? null
}

export function setProjectId(projectId: string | null): void {
  if (typeof window === 'undefined') return
  if (projectId) {
    sessionStorage.setItem(PROJECT_ID_KEY, projectId)
  } else {
    sessionStorage.removeItem(PROJECT_ID_KEY)
  }
}

export function isAuthenticated(): boolean {
  return !!getToken()
}

export function logout() {
  currentToken = null
  if (typeof window !== 'undefined') {
    sessionStorage.removeItem(ACCESS_TOKEN_KEY)
    sessionStorage.removeItem(REFRESH_TOKEN_KEY)
    sessionStorage.removeItem(EXPIRES_AT_KEY)
    sessionStorage.removeItem(PROJECT_ID_KEY)
    sessionStorage.removeItem('cco_user_name')
    sessionStorage.removeItem('cco_user_email')
    window.location.href = '/login'
  }
}

import { apiJson } from './api'

// ─────────────────────────────────────────────────────────────────────────────
// Estado de autenticación
// - Token: solo en memoria (se pierde al recargar → se exige login)
// - Nombre/email: sessionStorage (persiste entre navegaciones hacia atrás/adelante
//   en la misma pestaña, pero se limpia al cerrar la pestaña)
// ─────────────────────────────────────────────────────────────────────────────

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
  // Token solo en memoria (se pierde al recargar la página)
  currentToken = data.access_token
  // Nombre y email en sessionStorage para que persistan entre navegaciones
  if (typeof window !== 'undefined') {
    sessionStorage.setItem('cco_user_name', data.display_name || data.email.split('@')[0] || 'Usuario')
    sessionStorage.setItem('cco_user_email', data.email)
  }
  return data
}

export async function register(req: RegisterRequest) {
  return apiJson('/auth/register', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export function getToken(): string | null {
  return currentToken
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

export function isAuthenticated(): boolean {
  return !!currentToken
}

export function logout() {
  currentToken = null
  if (typeof window !== 'undefined') {
    sessionStorage.removeItem('cco_user_name')
    sessionStorage.removeItem('cco_user_email')
    window.location.href = '/login'
  }
}

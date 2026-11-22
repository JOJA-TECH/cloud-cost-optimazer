import { apiJson } from './api'

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
  localStorage.setItem('cco_access_token', data.access_token)
  localStorage.setItem('cco_refresh_token', data.refresh_token)
  return data
}

export async function register(req: RegisterRequest) {
  return apiJson('/auth/register', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export function getToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('cco_access_token')
}

export function logout() {
  localStorage.removeItem('cco_access_token')
  localStorage.removeItem('cco_refresh_token')
  if (typeof window !== 'undefined') {
    window.location.href = '/login'
  }
}

export function isAuthenticated(): boolean {
  return !!getToken()
}

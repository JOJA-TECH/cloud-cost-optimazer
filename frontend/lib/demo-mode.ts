const KEY = 'cco_demo_mode'

// Opt-in and scoped to this browser tab. Production data never falls back to demo.
export function isDemoMode(): boolean {
  return typeof window !== 'undefined' && sessionStorage.getItem(KEY) === 'true'
}

export function setDemoMode(enabled: boolean): void {
  if (enabled) sessionStorage.setItem(KEY, 'true')
  else sessionStorage.removeItem(KEY)
}

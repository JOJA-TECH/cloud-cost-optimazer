'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import {
  PanelLeftClose,
  PanelLeft,
  HelpCircle,
  ChevronDown,
  Database,
  MapPin,
  Calendar,
  RefreshCw,
  Menu,
  X,
  ChevronDown as ChevronDownIcon,
} from 'lucide-react'
import { navItems } from './nav'
import { LogoMark } from '@/components/logo'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import {
  getDashboardResource,
  getCloudAccounts,
  getCloudResources,
  setSelectedResourceUid,
  getSelectedResourceUid,
  getTimeRange,
  setTimeRange,
  type DashboardResource,
  type CloudAccount,
  type CloudResource,
} from '@/lib/api-services'
import { useAuth } from '@/components/auth-provider'
import { isDemoMode, setDemoMode } from '@/lib/demo-mode'
import { resource as demoResource } from '@/lib/mock-data'

const DEFAULT_TIME_RANGE: { from: string; to: string } = {
  from: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
  to: new Date().toISOString(),
}

export function DashboardShell({ children }: { children: React.ReactNode }) {
  const { token, logout } = useAuth()
  const router = useRouter()
  const pathname = usePathname()
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const [resource, setResource] = useState<DashboardResource | null>(null)
  const [userName, setUserName] = useState<string | null>(null)
  const [userEmail, setUserEmail] = useState<string | null>(null)
  const [accounts, setAccounts] = useState<CloudAccount[]>([])
  const [cloudResources, setCloudResourcesState] = useState<CloudResource[]>([])
  const [resourceDropdownOpen, setResourceDropdownOpen] = useState(false)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string }>(DEFAULT_TIME_RANGE)
  const [demo, setDemo] = useState(false)

  useEffect(() => {
    const enabled = isDemoMode()
    setDemo(enabled)
    const savedRange = getTimeRange()
    if (savedRange) setTimeRangeState(savedRange)
    else {
      setTimeRange(DEFAULT_TIME_RANGE.from, DEFAULT_TIME_RANGE.to)
      window.location.reload()
    }
    if (enabled) {
      setResource({ ...demoResource, resource_uid: 'demo-resource' })
    }
  }, [])

  // Verificar autenticación: si no hay token, redirigir al login
  useEffect(() => {
    if (!token) {
      if (typeof window !== 'undefined') {
        window.location.href = '/login'
      }
      return
    }
  }, [token])

  // Cargar cuentas y recursos al montar y en cada cambio de ruta
  useEffect(() => {
    if (!token) {
      router.push('/login')
      return
    }
    if (!isDemoMode()) loadAccountsAndResources()
    if (typeof window !== 'undefined') {
      setUserName(sessionStorage.getItem('cco_user_name') ?? null)
      setUserEmail(sessionStorage.getItem('cco_user_email') ?? null)
    }
  }, [token, router, pathname])

  // Escuchar popstate (botón atrás/adelante del navegador)
  useEffect(() => {
    const handlePopState = () => {
      if (!token) {
        router.push('/login')
        return
      }
      if (typeof window !== 'undefined') {
        setUserName(sessionStorage.getItem('cco_user_name') ?? null)
        setUserEmail(sessionStorage.getItem('cco_user_email') ?? null)
      }
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [token, router])

  async function loadAccountsAndResources() {
    try {
      const accountsResult = await getCloudAccounts()
      const accts = accountsResult.data ?? []
      setAccounts(accts)
      if (accts.length > 0) {
        const resourcesResult = await getCloudResources(accts[0].cloud_account_id)
        const res = resourcesResult.data ?? []
        setCloudResourcesState(res)
        if (res.length > 0) {
          const currentUid = getSelectedResourceUid()
          const selected = res.find((r) => r.resource_uid === currentUid) ?? res[0]
          setSelectedResourceUid(selected.resource_uid)
          const dashboardRes = await getDashboardResource(accts[0].cloud_account_id)
          setResource(dashboardRes.data)
          if (currentUid !== selected.resource_uid) window.location.reload()
        }
      }
    } catch {
      setResource(null)
      setCloudResourcesState([])
      setAccounts([])
    }
  }

  function handleResourceChange(uid: string) {
    setSelectedResourceUid(uid)
    const selected = cloudResources.find((r) => r.resource_uid === uid)
    if (selected) {
      setResource(null)
      void getDashboardResource(accounts[0]?.cloud_account_id).then((result) => setResource(result.data))
    }
    setResourceDropdownOpen(false)
    window.location.reload()
  }

  function handleTimeRangeChange(from: string, to: string) {
    setTimeRange(from, to)
    setTimeRangeState({ from, to })
    window.location.reload()
  }

  if (!token) return null

  return (
    <div className="flex min-h-screen bg-background">
      {/* Desktop sidebar */}
      <aside
        className={cn(
          'sticky top-0 hidden h-screen shrink-0 flex-col bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-16' : 'w-60',
        )}
      >
        <SidebarContent collapsed={collapsed} pathname={pathname} onNavigate={() => {}} userName={userName ?? 'Usuario'} userEmail={userEmail ?? 'usuario@empresa.com'} />
        <button
          onClick={() => setCollapsed((v) => !v)}
          className="m-3 flex items-center justify-center gap-2 rounded-md border border-sidebar-border py-2 text-xs font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-white"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <PanelLeft className="size-4" /> : <><PanelLeftClose className="size-4" /> Collapse</>}
        </button>
      </aside>

      {/* Mobile drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div className="absolute inset-0 bg-black/50" onClick={() => setMobileOpen(false)} />
          <aside className="absolute left-0 top-0 flex h-full w-64 flex-col bg-sidebar">
            <div className="flex items-center justify-between px-4 py-3">
              <span className="text-sm font-semibold text-white">Menu</span>
              <button onClick={() => setMobileOpen(false)} aria-label="Close menu">
                <X className="size-5 text-sidebar-foreground" />
              </button>
            </div>
            <SidebarContent collapsed={false} pathname={pathname} onNavigate={() => setMobileOpen(false)} userName={userName ?? 'Usuario'} userEmail={userEmail ?? 'usuario@empresa.com'} />
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar
          onMenu={() => setMobileOpen(true)}
          resource={resource}
          userName={userName ?? 'Usuario'}
          accounts={accounts}
          cloudResources={cloudResources}
          selectedResourceUid={getSelectedResourceUid() ?? ''}
          timeRange={timeRange}
          onResourceChange={handleResourceChange}
          onTimeRangeChange={handleTimeRangeChange}
          resourceDropdownOpen={resourceDropdownOpen}
          setResourceDropdownOpen={setResourceDropdownOpen}
        />
        <main className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-6 pb-24 sm:px-6 md:pb-6">
          <div className="mb-4 flex items-center justify-between rounded-lg border border-border bg-card px-4 py-2 text-xs">
            <span>{demo ? 'Modo demostración: datos ilustrativos, no representan recursos reales.' : 'Modo real: sin sustitución automática por datos demostrativos.'}</span>
            <button type="button" className="rounded border border-border px-2 py-1 font-medium" onClick={() => {
              if (!demo) {
                sessionStorage.setItem('cco_real_resource_uid', getSelectedResourceUid() ?? '')
                setSelectedResourceUid('demo-resource')
              } else {
                setSelectedResourceUid(sessionStorage.getItem('cco_real_resource_uid') || null)
                sessionStorage.removeItem('cco_real_resource_uid')
              }
              setDemoMode(!demo)
              window.location.reload()
            }}>
              {demo ? 'Salir de la demo' : 'Activar demo'}
            </button>
          </div>
          {children}
        </main>
        <MobileTabBar pathname={pathname} />
      </div>
    </div>
  )
}

function SidebarContent({
  collapsed,
  pathname,
  onNavigate,
  userName,
  userEmail,
}: {
  collapsed: boolean
  pathname: string
  onNavigate: () => void
  userName: string
  userEmail: string
}) {
  return (
    <>
      <Link
        href="/"
        className="flex items-center gap-2 px-4 py-4"
        aria-label="Cloud Database Cost Optimizer home"
      >
        <LogoMark className="size-7 shrink-0" />
        {!collapsed && (
          <span className="text-sm font-semibold leading-tight text-white">
            Cost Optimizer
          </span>
        )}
      </Link>

      <nav className="flex-1 overflow-y-auto px-3">
        <ul className="flex flex-col gap-0.5">
          {navItems.map((item) => {
            const active =
              item.href === '/dashboard'
                ? pathname === '/dashboard'
                : pathname.startsWith(item.href)
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  onClick={onNavigate}
                  title={collapsed ? item.label : undefined}
                  className={cn(
                    'flex items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium transition-colors',
                    active
                      ? 'bg-sidebar-primary/20 text-white'
                      : 'text-sidebar-foreground hover:bg-sidebar-accent hover:text-white',
                    collapsed && 'justify-center px-0',
                  )}
                >
                  <item.icon className="size-4 shrink-0" />
                  {!collapsed && item.label}
                  {active && !collapsed && (
                    <span className="ml-auto size-1.5 rounded-full bg-sidebar-primary" />
                  )}
                </Link>
              </li>
            )
          })}
        </ul>
      </nav>

      <div className="mt-2 border-t border-sidebar-border px-3 py-3">
        <Link
          href="#"
          className={cn(
            'flex items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-white',
            collapsed && 'justify-center px-0',
          )}
          title={collapsed ? 'Help' : undefined}
        >
          <HelpCircle className="size-4 shrink-0" />
          {!collapsed && 'Help'}
        </Link>
        <div
          className={cn(
            'mt-1 flex items-center gap-2.5 rounded-md px-2.5 py-2',
            collapsed && 'justify-center px-0',
          )}
        >
          <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-sidebar-primary text-xs font-semibold text-white">
            {getInitials(userName)}
          </span>
          {!collapsed && (
            <div className="min-w-0">
              <p className="truncate text-xs font-medium text-white">{userName}</p>
              <p className="truncate text-[11px] text-sidebar-foreground">{userEmail}</p>
            </div>
          )}
        </div>
      </div>
    </>
  )
}

function TopBar({
  onMenu,
  resource,
  userName,
  accounts,
  cloudResources,
  selectedResourceUid,
  timeRange,
  onResourceChange,
  onTimeRangeChange,
  resourceDropdownOpen,
  setResourceDropdownOpen,
}: {
  onMenu: () => void
  resource: DashboardResource | null
  userName: string
  accounts: CloudAccount[]
  cloudResources: CloudResource[]
  selectedResourceUid: string
  timeRange: { from: string; to: string }
  onResourceChange: (uid: string) => void
  onTimeRangeChange: (from: string, to: string) => void
  resourceDropdownOpen: boolean
  setResourceDropdownOpen: (open: boolean) => void
}) {
  const selectedResource = cloudResources.find((r) => r.resource_uid === selectedResourceUid)

  return (
    <header className="sticky top-0 z-40 flex h-14 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6">
      <button
        onClick={onMenu}
        className="inline-flex size-9 items-center justify-center rounded-md text-foreground md:hidden"
        aria-label="Open menu"
      >
        <Menu className="size-5" />
      </button>

      {/* Selector de recurso */}
      <div className="flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-muted">
        <Database className="size-4 text-primary" />
        <div className="flex flex-col items-start">
          <span className="max-w-[12rem] truncate sm:max-w-none font-semibold">
            {resource?.name ?? 'Cargando...'}
          </span>
          {cloudResources.length > 1 && (
            <span className="text-[10px] text-muted-foreground truncate max-w-[12rem]">
              {selectedResource?.region_code ?? resource?.region ?? ''}
            </span>
          )}
        </div>
        <ChevronDownIcon className="size-3.5 text-muted-foreground" />
        {cloudResources.length > 1 && (
          <div
            className={cn(
              'absolute right-0 mt-2 w-64 bg-card border border-border rounded-md shadow-lg z-50',
              resourceDropdownOpen ? '' : 'hidden',
            )}
          >
            <div className="px-3 py-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Recurso
            </div>
            <div className="py-1 overflow-y-auto max-h-48">
              {cloudResources.map((r) => (
                <button
                  key={r.resource_uid}
                  onClick={() => onResourceChange(r.resource_uid)}
                  className={cn(
                    'flex w-full items-center gap-3 px-3 py-2 text-sm transition-colors',
                    r.resource_uid === selectedResourceUid
                      ? 'bg-primary/10 text-primary'
                      : 'text-foreground hover:bg-muted',
                  )}
                >
                  <span className="truncate max-w-[10rem]">{r.database_name}</span>
                  <span className="ml-auto text-[10px] text-muted-foreground">
                    {r.region_code}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      <Badge variant="neutral" className="hidden lg:inline-flex">
        Azure SQL
      </Badge>

      {/* Selector de ventana temporal */}
      <div className="flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5">
        <Calendar className="size-4 text-primary" />
        <span className="ml-1 text-xs font-medium text-foreground hidden sm:inline">
          Desde:
        </span>
        <input
          type="datetime-local"
          className="text-xs bg-transparent border-none focus:ring-0 w-[130px]"
          value={timeRange.from.slice(0, 16)}
          onChange={(e) => {
            const from = new Date(e.target.value).toISOString()
            onTimeRangeChange(from, timeRange.to)
          }}
        />
        <span className="mx-1 text-xs text-muted-foreground hidden sm:inline">Hasta:</span>
        <input
          type="datetime-local"
          className="text-xs bg-transparent border-none focus:ring-0 w-[130px]"
          value={timeRange.to.slice(0, 16)}
          onChange={(e) => {
            const to = new Date(e.target.value).toISOString()
            onTimeRangeChange(timeRange.from, to)
          }}
        />
      </div>

      <div className="ml-auto flex items-center gap-2">
        <button className="hidden items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted sm:flex">
          <RefreshCw className="size-3" />
          8 min ago
        </button>
        <span className="flex size-8 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
          {getInitials(userName)}
        </span>
      </div>
    </header>
  )
}

function MobileTabBar({ pathname }: { pathname: string }) {
  const primary = navItems.slice(0, 5)
  return (
    <nav className="fixed inset-x-0 bottom-0 z-40 flex border-t border-border bg-card md:hidden">
      {primary.map((item) => {
        const active =
          item.href === '/dashboard'
            ? pathname === '/dashboard'
            : pathname.startsWith(item.href)
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              'flex flex-1 flex-col items-center gap-0.5 py-2 text-[10px] font-medium',
              active ? 'text-primary' : 'text-muted-foreground',
            )}
          >
            <item.icon className="size-5" />
            {item.label}
          </Link>
        )
      })}
    </nav>
  )
}

function getInitials(name: string | null | undefined): string {
  if (!name) return 'U'
  return name
    .split(' ')
    .map((part) => part.charAt(0).toUpperCase())
    .slice(0, 2)
    .join('')
}

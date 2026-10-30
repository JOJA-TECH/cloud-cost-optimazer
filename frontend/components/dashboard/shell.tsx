'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
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
} from 'lucide-react'
import { navItems } from './nav'
import { LogoMark } from '@/components/logo'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { resource } from '@/lib/mock-data'

export function DashboardShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <div className="flex min-h-screen bg-background">
      {/* Desktop sidebar */}
      <aside
        className={cn(
          'sticky top-0 hidden h-screen shrink-0 flex-col bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-16' : 'w-60',
        )}
      >
        <SidebarContent collapsed={collapsed} pathname={pathname} onNavigate={() => {}} />
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
            <SidebarContent collapsed={false} pathname={pathname} onNavigate={() => setMobileOpen(false)} />
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar onMenu={() => setMobileOpen(true)} />
        <main className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-6 pb-24 sm:px-6 md:pb-6">
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
}: {
  collapsed: boolean
  pathname: string
  onNavigate: () => void
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
            AR
          </span>
          {!collapsed && (
            <div className="min-w-0">
              <p className="truncate text-xs font-medium text-white">Alex Rivera</p>
              <p className="truncate text-[11px] text-sidebar-foreground">FinOps Engineer</p>
            </div>
          )}
        </div>
      </div>
    </>
  )
}

function TopBar({ onMenu }: { onMenu: () => void }) {
  return (
    <header className="sticky top-0 z-40 flex h-14 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6">
      <button
        onClick={onMenu}
        className="inline-flex size-9 items-center justify-center rounded-md text-foreground md:hidden"
        aria-label="Open menu"
      >
        <Menu className="size-5" />
      </button>

      <button className="flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-muted">
        <Database className="size-4 text-primary" />
        <span className="max-w-[9rem] truncate sm:max-w-none">{resource.name}</span>
        <ChevronDown className="size-3.5 text-muted-foreground" />
      </button>

      <Badge variant="neutral" className="hidden lg:inline-flex">
        Azure SQL
      </Badge>
      <span className="hidden items-center gap-1 text-xs text-muted-foreground lg:flex">
        <MapPin className="size-3.5" />
        {resource.region}
      </span>

      <div className="ml-auto flex items-center gap-2">
        <button className="hidden items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted sm:flex">
          <Calendar className="size-3.5 text-muted-foreground" />
          Last 30 days
          <ChevronDown className="size-3 text-muted-foreground" />
        </button>
        <span className="hidden items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground sm:flex">
          <RefreshCw className="size-3" />
          8 min ago
        </span>
        <span className="flex size-8 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
          AR
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

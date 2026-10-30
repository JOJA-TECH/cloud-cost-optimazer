import {
  LayoutDashboard,
  DollarSign,
  Gauge,
  Activity,
  GitCompareArrows,
  CircleCheck,
  TrendingUp,
  Stethoscope,
  History,
  Settings,
  type LucideIcon,
} from 'lucide-react'

export type NavItem = {
  label: string
  href: string
  icon: LucideIcon
}

export const navItems: NavItem[] = [
  { label: 'Overview', href: '/dashboard', icon: LayoutDashboard },
  { label: 'Costs', href: '/dashboard/costs', icon: DollarSign },
  { label: 'Performance', href: '/dashboard/performance', icon: Gauge },
  { label: 'Workload', href: '/dashboard/workload', icon: Activity },
  { label: 'Scenarios', href: '/dashboard/scenarios', icon: GitCompareArrows },
  { label: 'Recommendations', href: '/dashboard/recommendations', icon: CircleCheck },
  { label: 'Forecasting', href: '/dashboard/forecasting', icon: TrendingUp },
  { label: 'Diagnostics', href: '/dashboard/diagnostics', icon: Stethoscope },
  { label: 'History', href: '/dashboard/history', icon: History },
  { label: 'Configuration', href: '/dashboard/configuration', icon: Settings },
]

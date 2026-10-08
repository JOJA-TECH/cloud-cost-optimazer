import type { Metadata } from 'next'
import { DashboardShell } from '@/components/dashboard/shell'

export const metadata: Metadata = {
  title: 'Dashboard · Cloud Database Cost Optimizer',
  description: 'Workload-aware optimization for Azure SQL Database.',
}

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return <DashboardShell>{children}</DashboardShell>
}

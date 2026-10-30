import Link from 'next/link'
import { Wordmark } from '@/components/logo'

const columns = [
  {
    heading: 'Product',
    links: [
      { label: 'Product', href: '#product' },
      { label: 'How it works', href: '#how-it-works' },
      { label: 'Dashboard', href: '/dashboard' },
    ],
  },
  {
    heading: 'Resources',
    links: [
      { label: 'Documentation', href: '#' },
      { label: 'Insights', href: '#insights' },
      { label: 'About', href: '#about' },
    ],
  },
]

export function SiteFooter() {
  return (
    <footer id="about" className="border-t border-border bg-card">
      <div className="mx-auto max-w-6xl px-4 py-12 sm:px-6">
        <div className="grid gap-10 md:grid-cols-[1.5fr_1fr_1fr]">
          <div>
            <Wordmark />
            <p className="mt-3 max-w-xs text-sm text-muted-foreground">
              Workload-aware cost and capacity optimization for Azure SQL Database.
            </p>
            <p className="mt-4 inline-flex items-center gap-2 rounded-md border border-border bg-background px-2.5 py-1 text-xs text-muted-foreground">
              <span className="size-1.5 rounded-full bg-primary" />
              Release 1.0 · Azure SQL Database
            </p>
          </div>
          {columns.map((col) => (
            <div key={col.heading}>
              <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
                {col.heading}
              </p>
              <ul className="mt-3 flex flex-col gap-2">
                {col.links.map((l) => (
                  <li key={l.label}>
                    <Link
                      href={l.href}
                      className="text-sm text-muted-foreground transition-colors hover:text-foreground"
                    >
                      {l.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="mt-10 flex flex-col gap-2 border-t border-border pt-6 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <p>© 2026 Cloud Database Cost Optimizer. Demonstration prototype with sample data.</p>
          <p>Does not modify production resources. Multi-cloud support is roadmap-only.</p>
        </div>
      </div>
    </footer>
  )
}

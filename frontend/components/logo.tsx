import { cn } from '@/lib/utils'

export function LogoMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      className={cn('size-7', className)}
      fill="none"
      aria-hidden="true"
    >
      <rect width="32" height="32" rx="8" fill="var(--primary)" />
      {/* database cylinder */}
      <ellipse cx="16" cy="10" rx="7" ry="2.8" fill="white" fillOpacity="0.95" />
      <path
        d="M9 10v12c0 1.55 3.13 2.8 7 2.8s7-1.25 7-2.8V10"
        stroke="white"
        strokeOpacity="0.5"
        strokeWidth="1.6"
      />
      <path d="M9 16c0 1.55 3.13 2.8 7 2.8s7-1.25 7-2.8" stroke="white" strokeOpacity="0.5" strokeWidth="1.6" />
      {/* down optimization arrow */}
      <path
        d="M16 12.5v7m0 0l-2.4-2.6M16 19.5l2.4-2.6"
        stroke="white"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <LogoMark />
      <span className="text-[15px] font-semibold tracking-tight text-foreground">
        Cloud Database <span className="text-primary">Cost Optimizer</span>
      </span>
    </div>
  )
}

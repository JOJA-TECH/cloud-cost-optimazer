import { SiteHeader } from '@/components/landing/site-header'
import { Hero } from '@/components/landing/hero'
import { Problem } from '@/components/landing/problem'
import { Features } from '@/components/landing/features'
import { HowItWorks } from '@/components/landing/how-it-works'
import { Showcase } from '@/components/landing/showcase'
import { SiteFooter } from '@/components/landing/site-footer'

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background">
      <SiteHeader />
      <main>
        <Hero />
        <Problem />
        <Features />
        <HowItWorks />
        <Showcase />
      </main>
      <SiteFooter />
    </div>
  )
}

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  CircleDollarSign,
  Gauge,
  Layers3,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'

import { Hero3DScene } from '../hero-3d-scene'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const docsUrl =
    (status?.docs_link as string | undefined)?.trim() ||
    'https://docs.newapi.pro'

  const renderDocsButton = () => {
    const isExternal = /^https?:\/\//i.test(docsUrl)
    if (isExternal) {
      return (
        <Button
          variant='outline'
          className='benefit-liquid-glass-clear group hover:bg-background/80 inline-flex h-12 items-center gap-1.5 rounded-full px-5 text-sm font-medium'
          render={
            <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
          }
        >
          <BookOpen className='text-muted-foreground/80 group-hover:text-foreground size-4 transition-colors duration-200' />
          <span>{t('Docs')}</span>
        </Button>
      )
    }
    return (
      <Button
        variant='outline'
        className='benefit-liquid-glass-clear group hover:bg-background/80 inline-flex h-12 items-center gap-1.5 rounded-full px-5 text-sm font-medium'
        render={<Link to={docsUrl} />}
      >
        <BookOpen className='text-muted-foreground/80 group-hover:text-foreground size-4 transition-colors duration-200' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  const serviceSignals = [
    {
      icon: Layers3,
      label: t('One endpoint, many models'),
      className: 'text-teal-600 dark:text-teal-400',
    },
    {
      icon: CircleDollarSign,
      label: t('Pay only for usage'),
      className: 'text-amber-600 dark:text-amber-400',
    },
    {
      icon: Gauge,
      label: t('Usage and balance stay visible'),
      className: 'text-sky-600 dark:text-sky-400',
    },
  ]

  return (
    <section className='benefit-apple-shell benefit-hero-3d relative z-10 overflow-hidden border-b px-5 pt-28 pb-16 sm:pt-32 md:pt-36 md:pb-20'>
      <Hero3DScene />
      <div
        aria-hidden='true'
        className='border-border/25 pointer-events-none absolute inset-y-0 left-1/2 z-[1] w-full max-w-6xl -translate-x-1/2 border-x'
      />
      <div
        aria-hidden='true'
        className='border-border/20 pointer-events-none absolute inset-x-0 top-[58%] z-[1] border-t'
      />
      <div className='relative z-[2] mx-auto max-w-6xl'>
        <div className='benefit-hero-3d-content mx-auto flex max-w-4xl flex-col items-center text-center'>
          <div
            className='benefit-liquid-glass-clear landing-animate-fade-up inline-flex min-h-8 items-center gap-2 rounded-full px-3 py-1.5 text-xs font-medium text-teal-700 dark:text-teal-300'
            style={{ animationDelay: '0ms' }}
          >
            <span className='size-1.5 rounded-full bg-teal-500' />
            <span>{t('Unified AI access, ready for production')}</span>
          </div>

          <h1
            className='landing-animate-fade-up mt-5 text-5xl leading-none font-semibold sm:text-6xl md:text-7xl'
            style={{ animationDelay: '60ms' }}
          >
            Benefit API
          </h1>
          <p
            className='landing-animate-fade-up mt-5 max-w-3xl text-2xl leading-tight font-semibold md:text-4xl'
            style={{ animationDelay: '90ms' }}
          >
            {t('More models. Lower access cost.')}
          </p>
          <p
            className='landing-animate-fade-up text-muted-foreground mt-5 max-w-2xl text-base leading-relaxed md:text-lg'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'One OpenAI-compatible address connects leading models. Pay by actual usage, with balance and billing always visible.'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-7 flex w-full flex-col items-center justify-center gap-3 sm:w-auto sm:flex-row'
            style={{ animationDelay: '180ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  className='benefit-liquid-primary group h-12 w-full rounded-full px-6 text-sm font-semibold sm:w-auto'
                  render={<Link to='/dashboard' />}
                >
                  {t('Go to Dashboard')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                {renderDocsButton()}
              </>
            ) : (
              <>
                <Button
                  className='benefit-liquid-primary group h-12 w-full rounded-full px-6 text-sm font-semibold sm:w-auto'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                <Button
                  variant='outline'
                  className='benefit-liquid-glass-clear hover:bg-background/80 h-12 w-full rounded-full px-6 text-sm font-medium sm:w-auto'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
                {renderDocsButton()}
              </>
            )}
          </div>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center justify-center gap-x-6 gap-y-3'
            style={{ animationDelay: '240ms' }}
          >
            {serviceSignals.map((signal) => {
              const Icon = signal.icon
              return (
                <div key={signal.label} className='flex items-center gap-2'>
                  <Icon className={`size-4 shrink-0 ${signal.className}`} />
                  <span className='text-muted-foreground text-xs leading-none'>
                    {signal.label}
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </section>
  )
}

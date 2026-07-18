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
import { CherryStudio } from '@lobehub/icons'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  CircleDollarSign,
  Route,
  ShieldCheck,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'

import { HeroTerminalDemo } from '../hero-terminal-demo'

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
          className='group border-border/50 hover:border-border hover:bg-muted/50 inline-flex h-11 items-center gap-1.5 rounded-lg px-5 text-sm font-medium'
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
        className='group border-border/50 hover:border-border hover:bg-muted/50 inline-flex h-11 items-center gap-1.5 rounded-lg px-5 text-sm font-medium'
        render={<Link to={docsUrl} />}
      >
        <BookOpen className='text-muted-foreground/80 group-hover:text-foreground size-4 transition-colors duration-200' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  const serviceSignals = [
    {
      icon: Route,
      label: t('OpenAI-compatible access'),
      className: 'text-teal-600 dark:text-teal-400',
    },
    {
      icon: ShieldCheck,
      label: t('Stable request routing'),
      className: 'text-sky-600 dark:text-sky-400',
    },
    {
      icon: CircleDollarSign,
      label: t('Pay only for usage'),
      className: 'text-amber-600 dark:text-amber-400',
    },
  ]

  return (
    <section className='relative z-10 overflow-hidden border-b px-6 pt-24 pb-12 md:pt-28 md:pb-16'>
      <div className='mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 lg:grid-cols-12 lg:gap-10'>
        <div className='flex flex-col items-start text-left lg:col-span-6'>
          <div
            className='landing-animate-fade-up mb-5 inline-flex items-center gap-2 text-xs font-semibold text-teal-700 opacity-0 dark:text-teal-300'
            style={{ animationDelay: '0ms' }}
          >
            <span className='size-2 rounded-full bg-teal-500 shadow-[0_0_0_4px_rgba(20,184,166,0.12)]' />
            <span>{t('Unified AI access, ready for production')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-4xl leading-[1.08] font-bold opacity-0 md:text-5xl'
            style={{ animationDelay: '60ms' }}
          >
            Benefit API
          </h1>
          <p
            className='landing-animate-fade-up mt-4 max-w-xl text-xl leading-snug font-semibold opacity-0 md:text-2xl'
            style={{ animationDelay: '90ms' }}
          >
            {t('One endpoint for every AI workflow')}
          </p>
          <p
            className='landing-animate-fade-up text-muted-foreground/80 mt-5 max-w-xl text-base leading-relaxed opacity-0 md:text-[15px]'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'Connect OpenAI-compatible clients to one clear base URL, manage keys and quota in one place, and keep every request visible.'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '180ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
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
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                <Button
                  variant='outline'
                  className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
                {renderDocsButton()}
              </>
            )}
          </div>

          <div
            className='landing-animate-fade-up mt-9 grid w-full max-w-xl gap-3 opacity-0 sm:grid-cols-3'
            style={{ animationDelay: '240ms' }}
          >
            {serviceSignals.map((signal) => {
              const Icon = signal.icon
              return (
                <div key={signal.label} className='flex items-center gap-2'>
                  <Icon className={`size-4 shrink-0 ${signal.className}`} />
                  <span className='text-muted-foreground text-xs leading-snug'>
                    {signal.label}
                  </span>
                </div>
              )
            })}
          </div>

          <div
            className='landing-animate-fade-up mt-8 flex items-center gap-3 opacity-0'
            style={{ animationDelay: '280ms' }}
          >
            <span className='text-muted-foreground text-xs'>
              {t('Works with')}
            </span>
            <a
              href='https://cherry-ai.com'
              target='_blank'
              rel='noopener noreferrer'
              className='text-foreground/80 hover:text-foreground flex items-center gap-1.5 text-xs font-medium transition-colors'
            >
              <CherryStudio.Color size={18} className='shrink-0' />
              <span>Cherry Studio</span>
            </a>
            <span aria-hidden className='bg-border h-4 w-px' />
            <span className='text-foreground/70 text-xs font-medium'>
              OpenAI SDK
            </span>
          </div>
        </div>

        <div
          className='landing-animate-fade-up flex w-full justify-center opacity-0 lg:col-span-6'
          style={{ animationDelay: '320ms' }}
        >
          <HeroTerminalDemo />
        </div>
      </div>
    </section>
  )
}

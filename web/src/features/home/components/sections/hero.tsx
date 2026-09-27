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
import { ArrowRight, BookOpen } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'

import { observeHeroMotion } from '../../lib/hero-motion'
import { HeroCosmicBackdrop } from '../hero-cosmic-backdrop'
import { HeroRoutingVisual } from '../hero-routing-visual'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const heroRef = useRef<HTMLElement>(null)
  useEffect(() => {
    if (heroRef.current) return observeHeroMotion(heroRef.current)
  }, [])
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
          role='link'
          variant='outline'
          className='benefit-cinema-secondary group inline-flex h-12 items-center gap-2 rounded-full px-6 text-sm font-medium'
          render={
            <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
          }
        >
          <BookOpen aria-hidden='true' className='size-4' />
          <span>{t('Docs')}</span>
        </Button>
      )
    }
    return (
      <Button
        role='link'
        variant='outline'
        className='benefit-cinema-secondary group inline-flex h-12 items-center gap-2 rounded-full px-6 text-sm font-medium'
        render={<Link to={docsUrl} />}
      >
        <BookOpen aria-hidden='true' className='size-4' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  return (
    <section
      ref={heroRef}
      data-decorative-motion='paused'
      className={cn('benefit-cinema-hero', props.className)}
      aria-labelledby='benefit-home-title'
    >
      <HeroCosmicBackdrop />
      <div className='benefit-cinema-hero-grid'>
        <div className='benefit-cinema-copy'>
          <div className='benefit-cinema-eyebrow'>
            <span aria-hidden='true' className='benefit-cinema-rule' />
            <span>{t('Unified AI access, ready for production')}</span>
          </div>

          <h1 id='benefit-home-title' className='benefit-cinema-title'>
            Benefit <span>API</span>
          </h1>
          <p className='benefit-cinema-headline'>
            {t('More models. Lower access cost.')}
          </p>
          <p className='benefit-cinema-description'>
            {t(
              'One OpenAI-compatible address connects leading models. Pay by actual usage, with balance and billing always visible.'
            )}
          </p>

          <div className='benefit-cinema-actions'>
            {props.isAuthenticated ? (
              <>
                <Button
                  role='link'
                  className='benefit-cinema-primary group h-12 rounded-full px-6 text-sm font-semibold'
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
                  role='link'
                  className='benefit-cinema-primary group h-12 rounded-full px-6 text-sm font-semibold'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                <Button
                  role='link'
                  variant='outline'
                  className='benefit-cinema-secondary h-12 rounded-full px-6 text-sm font-medium'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
              </>
            )}
          </div>
        </div>
        <HeroRoutingVisual />
      </div>
      <div className='benefit-cinema-caption'>
        <span>{t('One endpoint, many models')}</span>
        <span className='font-mono'>/v1</span>
      </div>
    </section>
  )
}

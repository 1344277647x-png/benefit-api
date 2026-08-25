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
import { Braces, Radio } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

import { HeroTerminalDemo } from '../hero-terminal-demo'

export function ProductPreview() {
  const { t } = useTranslation()

  return (
    <section className='benefit-apple-shell bg-muted/15 relative z-10 border-b px-5 py-16 md:py-20'>
      <div className='mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-[minmax(260px,0.7fr)_minmax(0,1.3fr)] lg:gap-14'>
        <AnimateInView className='max-w-lg' animation='fade-up'>
          <div className='text-primary flex items-center gap-2 text-xs font-semibold'>
            <Radio className='size-4' aria-hidden='true' />
            <span>{t('OpenAI-compatible access')}</span>
          </div>
          <h2 className='mt-4 text-3xl leading-tight font-semibold md:text-4xl'>
            {t('One endpoint, many models')}
          </h2>
          <p className='text-muted-foreground mt-4 text-sm leading-relaxed md:text-base'>
            {t(
              'Use a single base URL across OpenAI-compatible tools and supported native routes.'
            )}
          </p>

          <div className='benefit-liquid-glass-clear mt-7 flex flex-wrap items-center gap-3 rounded-[8px] p-3'>
            <span className='text-muted-foreground text-xs'>
              {t('Works with')}
            </span>
            <a
              href='https://cherry-ai.com'
              target='_blank'
              rel='noopener noreferrer'
              className='hover:bg-foreground/[0.06] flex min-h-9 items-center gap-1.5 rounded-[8px] px-2 text-xs font-medium transition-colors'
            >
              <CherryStudio.Color size={18} className='shrink-0' />
              <span>Cherry Studio</span>
            </a>
            <span aria-hidden='true' className='bg-border h-5 w-px' />
            <span className='flex min-h-9 items-center gap-1.5 px-2 text-xs font-medium'>
              <Braces
                className='text-muted-foreground size-4'
                aria-hidden='true'
              />
              OpenAI SDK
            </span>
          </div>
        </AnimateInView>

        <AnimateInView animation='fade-up' delay={120}>
          <HeroTerminalDemo className='max-w-none' />
        </AnimateInView>
      </div>
    </section>
  )
}

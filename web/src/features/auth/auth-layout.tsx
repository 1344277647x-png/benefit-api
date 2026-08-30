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
import { CircleDollarSign, Gauge, Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='benefit-apple-shell benefit-auth-stage relative min-h-svh overflow-hidden'>
      <div
        aria-hidden='true'
        className='border-border/35 pointer-events-none absolute inset-y-0 left-1/2 w-full max-w-6xl -translate-x-1/2 border-x'
      />
      <div
        aria-hidden='true'
        className='border-border/25 pointer-events-none absolute inset-x-0 top-[28%] border-t'
      />
      <Link
        to='/'
        className='benefit-liquid-glass absolute top-4 left-4 z-10 flex min-h-11 items-center gap-2.5 rounded-full px-3 transition-opacity hover:opacity-85 sm:top-6 sm:left-6'
      >
        <div className='relative size-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-lg' />
          ) : (
            <img
              src={logo}
              alt={systemName || t('Logo')}
              className='size-8 rounded-[8px] object-contain'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-base font-semibold'>{systemName}</h1>
        )}
      </Link>
      <div className='relative flex min-h-svh items-center px-4 pt-24 pb-8 sm:px-6 sm:pt-20 lg:px-10'>
        <div className='mx-auto grid w-full max-w-6xl items-center gap-12 lg:grid-cols-[minmax(0,1fr)_460px] xl:gap-20'>
          <aside className='hidden min-w-0 flex-col justify-center lg:flex'>
            <p className='text-primary text-sm font-semibold'>
              {t('Unified AI access, ready for production')}
            </p>
            <h2 className='mt-4 max-w-xl text-4xl leading-tight font-semibold xl:text-5xl'>
              {t('More models. Lower access cost.')}
            </h2>
            <p className='text-muted-foreground mt-5 max-w-xl text-base leading-7'>
              {t(
                'One OpenAI-compatible address connects leading models. Pay by actual usage, with balance and billing always visible.'
              )}
            </p>
            <ul className='mt-8 grid max-w-xl gap-3'>
              {[
                { icon: Layers3, label: t('One endpoint, many models') },
                { icon: CircleDollarSign, label: t('Pay only for usage') },
                { icon: Gauge, label: t('Usage and balance stay visible') },
              ].map((item) => {
                const Icon = item.icon
                return (
                  <li
                    key={item.label}
                    className='border-border/60 flex min-h-12 items-center gap-3 border-b py-3 last:border-b-0'
                  >
                    <span className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-[8px]'>
                      <Icon className='size-4' aria-hidden='true' />
                    </span>
                    <span className='text-sm font-medium'>{item.label}</span>
                  </li>
                )
              })}
            </ul>
          </aside>
          <div className='benefit-auth-panel benefit-solid-surface mx-auto flex w-full max-w-[460px] flex-col justify-center space-y-2 rounded-[8px] px-5 py-8 sm:px-9 sm:py-10 lg:mx-0'>
            {children}
          </div>
        </div>
      </div>
    </div>
  )
}

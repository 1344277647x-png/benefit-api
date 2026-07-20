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
      <div className='relative flex min-h-svh items-center px-4 pt-24 pb-8 sm:px-6 sm:pt-20'>
        <div className='benefit-auth-panel benefit-solid-surface mx-auto flex w-full max-w-[460px] flex-col justify-center space-y-2 rounded-[8px] px-5 py-8 sm:px-9 sm:py-10'>
          {children}
        </div>
      </div>
    </div>
  )
}

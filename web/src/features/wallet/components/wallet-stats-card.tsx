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
import { Activity, BarChart3, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <div className='benefit-metric-band grid grid-cols-2 rounded-lg border sm:grid-cols-3 sm:divide-x'>
        {(['balance', 'usage', 'requests'] as const).map((key, index) => (
          <div
            key={key}
            className={cn(
              'min-w-0 px-3.5 py-3 sm:px-5 sm:py-4',
              index === 0 && 'col-span-2 border-b sm:col-span-1 sm:border-b-0',
              index === 1 && 'border-r sm:border-r-0'
            )}
          >
            <Skeleton className='h-3.5 w-24 max-w-full' />
            <Skeleton className='mt-2 h-6 w-32 max-w-full sm:h-7' />
            <Skeleton className='mt-1.5 hidden h-3.5 w-24 md:block' />
          </div>
        ))}
      </div>
    )
  }

  const stats: {
    id: 'balance' | 'usage' | 'requests'
    label: string
    value: string
    description: string
    icon: typeof WalletCards
    tone: IconBadgeTone
  }[] = [
    {
      id: 'balance',
      label: t('Current Balance'),
      value: formatQuota(props.user?.quota ?? 0),
      description: t('Remaining quota'),
      icon: WalletCards,
      tone: 'success',
    },
    {
      id: 'usage',
      label: t('Total Usage'),
      value: formatQuota(props.user?.used_quota ?? 0),
      description: t('Total consumed quota'),
      icon: BarChart3,
      tone: 'info',
    },
    {
      id: 'requests',
      label: t('API Requests'),
      value: (props.user?.request_count ?? 0).toLocaleString(),
      description: t('Total requests made'),
      icon: Activity,
      tone: 'chart-4',
    },
  ]

  return (
    <div className='benefit-metric-band grid grid-cols-2 rounded-lg border sm:grid-cols-3 sm:divide-x'>
      {stats.map((item, index) => (
        <div
          key={item.id}
          className={cn(
            'min-w-0 px-3.5 py-3 sm:px-5 sm:py-4',
            index === 0 && 'col-span-2 border-b sm:col-span-1 sm:border-b-0',
            index === 1 && 'border-r sm:border-r-0'
          )}
        >
          <div className='flex items-center gap-2 sm:gap-2.5'>
            <IconBadge tone={item.tone} size='stat'>
              <item.icon />
            </IconBadge>
            <div className='text-muted-foreground truncate text-xs font-medium tracking-wider uppercase'>
              {item.label}
            </div>
          </div>

          <div
            className={cn(
              'text-foreground mt-2 font-mono font-bold tracking-tight tabular-nums sm:mt-2.5 sm:text-2xl',
              index === 0 ? 'text-2xl' : 'text-base sm:text-2xl'
            )}
          >
            {item.value}
          </div>
          <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
            {item.description}
          </div>
        </div>
      ))}
    </div>
  )
}

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
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  CheckCircle2,
  Clock3,
  RefreshCw,
  XCircle,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampRelative } from '@/lib/format'

import {
  getPublicChannelHealth,
  type PublicHealthStatus,
  type PublicModelHealth,
} from './api'

const statusStyles: Record<PublicHealthStatus, string> = {
  normal: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600',
  delayed: 'border-amber-500/30 bg-amber-500/10 text-amber-600',
  unavailable: 'border-rose-500/30 bg-rose-500/10 text-rose-600',
  unknown: 'border-muted-foreground/20 bg-muted text-muted-foreground',
}

function StatusIcon({ status }: { status: PublicHealthStatus }) {
  if (status === 'normal') return <CheckCircle2 className='size-4' />
  if (status === 'delayed') return <Clock3 className='size-4' />
  if (status === 'unavailable') return <XCircle className='size-4' />
  return <Activity className='size-4' />
}

function statusLabel(status: PublicHealthStatus, t: (key: string) => string) {
  return {
    normal: t('Operational'),
    delayed: t('Delayed'),
    unavailable: t('Unavailable'),
    unknown: t('Not detected'),
  }[status]
}

function HealthCard({
  item,
  t,
}: {
  item: PublicModelHealth
  t: (key: string) => string
}) {
  return (
    <Card className='bg-card/70 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md'>
      <CardContent className='flex items-center justify-between gap-3 p-4'>
        <div className='min-w-0'>
          <p className='truncate text-sm font-medium'>{item.model}</p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {item.last_sample_at
              ? `${t('Updated')} ${formatTimestampRelative(item.last_sample_at)}`
              : t('Waiting for traffic')}
          </p>
        </div>
        <Badge variant='outline' className={statusStyles[item.status]}>
          <StatusIcon status={item.status} />
          {statusLabel(item.status, t)}
        </Badge>
      </CardContent>
    </Card>
  )
}

export function PublicChannelHealth() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['public-channel-health'],
    queryFn: getPublicChannelHealth,
    refetchInterval: (currentQuery) => {
      const data = currentQuery.state.data?.data
      if (data?.enabled === false) return false
      const seconds = data?.refresh_interval_seconds
      return (typeof seconds === 'number' && seconds >= 5 ? seconds : 30) * 1000
    },
    staleTime: 15_000,
    retry: 1,
  })
  const enabled = query.data?.data?.enabled !== false
  const refreshSeconds =
    typeof query.data?.data?.refresh_interval_seconds === 'number'
      ? query.data.data.refresh_interval_seconds
      : 30
  const items = query.data?.data?.items ?? []
  const refreshedAt = query.data?.data?.refreshed_at
  let statusContent: ReactNode
  if (query.isLoading) {
    statusContent = (
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
        <Skeleton className='h-20 rounded-xl' />
        <Skeleton className='h-20 rounded-xl' />
        <Skeleton className='h-20 rounded-xl' />
      </div>
    )
  } else if (query.isError || query.data?.success === false) {
    statusContent = (
      <Card>
        <CardContent className='text-muted-foreground py-12 text-center text-sm'>
          {t('Status data is temporarily unavailable.')}
        </CardContent>
      </Card>
    )
  } else if (!enabled) {
    statusContent = (
      <Card>
        <CardContent className='text-muted-foreground flex items-center justify-center gap-2 py-12 text-center text-sm'>
          <Activity className='size-4' />
          {t('Channel health tracking is disabled.')}
        </CardContent>
      </Card>
    )
  } else if (items.length === 0) {
    statusContent = (
      <Card>
        <CardContent className='text-muted-foreground py-12 text-center text-sm'>
          {t('No model health samples are available yet.')}
        </CardContent>
      </Card>
    )
  } else {
    statusContent = (
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
        {items.map((item) => (
          <HealthCard key={item.model} item={item} t={t} />
        ))}
      </div>
    )
  }

  return (
    <PublicLayout>
      <div className='mx-auto w-full max-w-6xl space-y-6 py-4 sm:py-8'>
        <header className='flex flex-wrap items-end justify-between gap-4'>
          <div>
            <div className='text-primary mb-2 flex items-center gap-2 text-xs font-medium tracking-[0.18em] uppercase'>
              <Activity className='size-3.5' />
              Benefit API
            </div>
            <h1 className='text-3xl font-semibold tracking-tight'>
              {t('Service status')}
            </h1>
            <p className='text-muted-foreground mt-2 max-w-2xl text-sm'>
              {t(
                'A public view of model availability based on recent real traffic. Channel names and private diagnostics are hidden.'
              )}
            </p>
          </div>
          {enabled && (
            <Button
              variant='outline'
              size='sm'
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              <RefreshCw className={query.isFetching ? 'animate-spin' : ''} />
              {t('Refresh')}
            </Button>
          )}
        </header>

        {statusContent}

        {refreshedAt && (
          <p className='text-muted-foreground text-center text-xs'>
            {t('Last checked')} {formatTimestampRelative(refreshedAt)} ·{' '}
            {t('Updates every {{seconds}} seconds', {
              seconds: refreshSeconds,
            })}
          </p>
        )}
      </div>
    </PublicLayout>
  )
}

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
  AlertTriangle,
  CheckCircle2,
  Clock3,
  RefreshCw,
  XCircle,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import {
  getChannelHealth,
  type ChannelHealthView,
  type PublicHealthStatus,
} from './api'

const statusStyles: Record<PublicHealthStatus, string> = {
  normal: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600',
  delayed: 'border-amber-500/30 bg-amber-500/10 text-amber-600',
  unavailable: 'border-rose-500/30 bg-rose-500/10 text-rose-600',
  unknown: 'border-muted-foreground/20 bg-muted text-muted-foreground',
}

function statusLabel(status: PublicHealthStatus, t: (key: string) => string) {
  return {
    normal: t('Normal'),
    delayed: t('Delayed'),
    unavailable: t('Unavailable'),
    unknown: t('Not detected'),
  }[status]
}

function StatusIcon({ status }: { status: PublicHealthStatus }) {
  if (status === 'normal') return <CheckCircle2 className='size-3.5' />
  if (status === 'delayed') return <Clock3 className='size-3.5' />
  if (status === 'unavailable') return <XCircle className='size-3.5' />
  return <Activity className='size-3.5' />
}

function HealthRow({
  item,
  t,
}: {
  item: ChannelHealthView
  t: (key: string) => string
}) {
  return (
    <div className='grid min-w-[700px] grid-cols-[72px_minmax(180px,1fr)_110px_100px_100px_90px] items-center gap-3 border-b px-3 py-2.5 text-sm last:border-b-0'>
      <span className='text-muted-foreground tabular-nums'>
        #{item.channel_id}
      </span>
      <div className='min-w-0'>
        <p className='truncate font-medium'>{item.model}</p>
        <p className='text-muted-foreground truncate text-[11px]'>
          {item.endpoint_type || t('Unknown endpoint')}
        </p>
        {(item.last_error_code ||
          item.last_error_class ||
          item.last_http_status) && (
          <p
            className='text-destructive/80 truncate text-[11px]'
            title={item.last_error_code || item.last_error_class}
          >
            {item.last_error_code ||
              item.last_error_class ||
              `HTTP ${item.last_http_status}`}
          </p>
        )}
      </div>
      <Badge variant='outline' className={statusStyles[item.status]}>
        <StatusIcon status={item.status} />
        {statusLabel(item.status, t)}
      </Badge>
      <span className='tabular-nums'>
        {item.average_latency_ms ? `${item.average_latency_ms} ms` : '-'}
      </span>
      <span className='tabular-nums'>
        {item.average_ttft_ms ? `${item.average_ttft_ms} ms` : '-'}
      </span>
      <span className='text-muted-foreground tabular-nums'>
        {item.request_count > 0 ? `${item.success_rate.toFixed(1)}%` : '-'}
      </span>
    </div>
  )
}

export function ChannelHealthPanel() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['channel-health'],
    queryFn: getChannelHealth,
    refetchInterval: 15_000,
    staleTime: 10_000,
    retry: false,
  })
  const items = query.data?.data?.items ?? []
  let content: ReactNode
  if (query.isLoading) {
    content = (
      <CardContent className='space-y-2 pt-4'>
        <Skeleton className='h-8' />
        <Skeleton className='h-8' />
        <Skeleton className='h-8' />
      </CardContent>
    )
  } else if (query.isError || query.data?.success === false) {
    content = (
      <CardContent className='text-muted-foreground flex items-center gap-2 py-6 text-sm'>
        <AlertTriangle className='size-4' />
        {query.data?.message || t('Unable to load channel health.')}
      </CardContent>
    )
  } else if (items.length === 0) {
    content = (
      <CardContent className='text-muted-foreground py-6 text-center text-sm'>
        {t('No channel health samples yet.')}
      </CardContent>
    )
  } else {
    content = (
      <CardContent className='overflow-x-auto p-0'>
        <div className='bg-muted/30 text-muted-foreground grid min-w-[700px] grid-cols-[72px_minmax(180px,1fr)_110px_100px_100px_90px] gap-3 border-b px-3 py-2 text-[11px] font-medium tracking-wide uppercase'>
          <span>{t('Channel')}</span>
          <span>{t('Model')}</span>
          <span>{t('Status')}</span>
          <span>{t('Latency')}</span>
          <span>{t('First byte')}</span>
          <span>{t('Success')}</span>
        </div>
        {items.map((item) => (
          <HealthRow
            key={`${item.channel_id}:${item.model}`}
            item={item}
            t={t}
          />
        ))}
      </CardContent>
    )
  }
  return (
    <Card className='bg-card/80 mb-4 overflow-hidden shadow-sm'>
      <CardHeader className='flex flex-row items-center justify-between gap-3 border-b'>
        <div>
          <CardTitle className='flex items-center gap-2 text-base'>
            <Activity className='text-primary size-4' />
            {t('Channel health')}
          </CardTitle>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Rolling five-minute view; refreshes every 15 seconds')}
          </p>
        </div>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
          aria-label={t('Refresh')}
        >
          <RefreshCw className={query.isFetching ? 'animate-spin' : ''} />
        </Button>
      </CardHeader>
      {content}
    </Card>
  )
}

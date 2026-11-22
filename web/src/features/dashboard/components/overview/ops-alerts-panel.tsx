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
import { Link } from '@tanstack/react-router'
import {
  AlertTriangle,
  ArrowRight,
  Inbox,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { getOpsAlerts } from '../../api'
import type { OpsAlertTLSCertStatus } from '../../types'

function formatExpiryDate(unixSeconds?: number): string {
  if (!unixSeconds || unixSeconds <= 0) return '—'
  try {
    return new Date(unixSeconds * 1000).toISOString().slice(0, 10)
  } catch {
    return '—'
  }
}

function getTlsStatusLabel(
  tls: OpsAlertTLSCertStatus,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  switch (tls.status) {
    case 'ok':
      return t('{{days}} days left', { days: tls.days_remaining })
    case 'warning':
      return t('Expiring in {{days}} days', { days: tls.days_remaining })
    case 'critical':
      return tls.days_remaining <= 0
        ? t('Expired')
        : t('Expiring in {{days}} days', { days: tls.days_remaining })
    case 'error':
      return t('Check failed')
    default:
      return t('HTTPS not configured')
  }
}

function getTlsBadgeClass(status: OpsAlertTLSCertStatus['status']): string {
  switch (status) {
    case 'ok':
      return 'border-emerald-500/40 text-emerald-600 dark:text-emerald-400'
    case 'warning':
    case 'error':
      return 'border-amber-500/40 text-amber-600 dark:text-amber-400'
    case 'critical':
      return 'border-destructive/50 text-destructive'
    default:
      return 'text-muted-foreground'
  }
}

export function OpsAlertsPanel() {
  const { t } = useTranslation()
  const [refreshing, setRefreshing] = useState(false)

  const alertsQuery = useQuery({
    queryKey: ['ops-alerts'],
    queryFn: () => getOpsAlerts(false),
    staleTime: 60 * 1000,
    retry: false,
  })

  const handleForceRefresh = async () => {
    setRefreshing(true)
    try {
      await getOpsAlerts(true)
      await alertsQuery.refetch()
    } finally {
      setRefreshing(false)
    }
  }

  const snapshot = alertsQuery.data?.data
  const loading = alertsQuery.isLoading
  const alertLevel = snapshot?.alert_level ?? 'ok'
  let headerTone: 'destructive' | 'warning' | 'success' = 'success'
  if (alertLevel === 'critical') {
    headerTone = 'destructive'
  } else if (alertLevel === 'warning') {
    headerTone = 'warning'
  }
  let alertLevelLabel = t('All systems healthy')
  if (alertLevel === 'critical') {
    alertLevelLabel = t('Action required')
  } else if (alertLevel === 'warning') {
    alertLevelLabel = t('Attention needed')
  }
  const anomalyTotal =
    (snapshot?.queues.billing_exceptions_count ?? 0) +
    (snapshot?.queues.unsettled_pre_consume_count ?? 0)
  let outboxBadgeClass =
    'border-emerald-500/40 text-emerald-600 dark:text-emerald-400'
  if ((snapshot?.queues.business_dead_letter ?? 0) > 0) {
    outboxBadgeClass = 'border-destructive/50 text-destructive'
  } else if (anomalyTotal > 0) {
    outboxBadgeClass = 'border-amber-500/40 text-amber-600 dark:text-amber-400'
  }

  return (
    <section className='bg-card benefit-cinema-panel overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center gap-2 border-b px-4 py-3 sm:px-5'>
        <IconBadge tone={headerTone} size='sm'>
          {alertLevel === 'ok' ? <ShieldCheck /> : <ShieldAlert />}
        </IconBadge>
        <div className='min-w-0'>
          <h3 className='text-sm font-semibold'>
            {t('Operations health & alerts')}
          </h3>
        </div>
        <div className='ml-auto flex items-center gap-2'>
          {snapshot && (
            <Badge
              variant='outline'
              className={cn(
                alertLevel === 'ok' &&
                  'border-emerald-500/40 text-emerald-600 dark:text-emerald-400',
                alertLevel === 'warning' &&
                  'border-amber-500/40 text-amber-600 dark:text-amber-400',
                alertLevel === 'critical' &&
                  'border-destructive/50 text-destructive'
              )}
            >
              {alertLevelLabel}
            </Badge>
          )}
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-7 gap-1.5 text-xs'
            onClick={() => void handleForceRefresh()}
            disabled={loading || refreshing}
          >
            <RefreshCw
              className={cn(
                'size-3.5',
                (loading || refreshing) && 'animate-spin'
              )}
            />
            {t('Refresh')}
          </Button>
        </div>
      </div>

      <div className='space-y-3 p-4 sm:p-5'>
        {alertsQuery.isError && (
          <Alert variant='destructive'>
            <AlertTriangle className='size-4' />
            <AlertTitle>{t('Unable to load operations alerts')}</AlertTitle>
            <AlertDescription>
              {alertsQuery.error instanceof Error
                ? alertsQuery.error.message
                : t('Please try refreshing later.')}
            </AlertDescription>
          </Alert>
        )}

        {loading ? (
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
            {['tls', 'ops-queue', 'biz-queue'].map((key) => (
              <Skeleton key={key} className='h-24 w-full rounded-xl' />
            ))}
          </div>
        ) : (
          snapshot && (
            <>
              <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                <div className='bg-muted/30 flex flex-col justify-between gap-2 rounded-xl border p-3'>
                  <div className='flex items-center justify-between gap-2'>
                    <span className='text-muted-foreground text-xs font-medium'>
                      {t('TLS certificate')}
                    </span>
                    <Badge
                      variant='outline'
                      className={cn(
                        'text-[11px]',
                        getTlsBadgeClass(snapshot.tls.status)
                      )}
                    >
                      {getTlsStatusLabel(snapshot.tls, t)}
                    </Badge>
                  </div>
                  <div className='min-w-0'>
                    <p className='truncate font-mono text-xs font-medium'>
                      {snapshot.tls.domain || t('Not configured')}
                    </p>
                    <p className='text-muted-foreground mt-0.5 text-[11px]'>
                      {snapshot.tls.not_after
                        ? t('Expires {{date}}', {
                            date: formatExpiryDate(snapshot.tls.not_after),
                          })
                        : snapshot.tls.error ||
                          t('Alert threshold: {{days}} days', {
                            days: snapshot.tls.warning_threshold_days,
                          })}
                    </p>
                  </div>
                </div>

                <div className='bg-muted/30 flex flex-col justify-between gap-2 rounded-xl border p-3'>
                  <div className='flex items-center justify-between gap-2'>
                    <span className='text-muted-foreground text-xs font-medium'>
                      {t('Accounting queue')}
                    </span>
                    <Badge
                      variant='outline'
                      className={cn(
                        'text-[11px]',
                        snapshot.queues.operations_pending > 0
                          ? 'border-amber-500/40 text-amber-600 dark:text-amber-400'
                          : 'border-emerald-500/40 text-emerald-600 dark:text-emerald-400'
                      )}
                    >
                      {snapshot.queues.operations_pending > 0
                        ? t('{{count}} pending', {
                            count: snapshot.queues.operations_pending,
                          })
                        : t('Clear')}
                    </Badge>
                  </div>
                  <div className='flex items-baseline justify-between gap-2'>
                    <span className='text-lg font-semibold tabular-nums'>
                      {snapshot.queues.operations_pending}
                    </span>
                    <span className='text-muted-foreground text-[11px]'>
                      {t('Retrying: {{count}}', {
                        count: snapshot.queues.operations_retrying,
                      })}
                    </span>
                  </div>
                </div>

                <div className='bg-muted/30 flex flex-col justify-between gap-2 rounded-xl border p-3'>
                  <div className='flex items-center justify-between gap-2'>
                    <span className='text-muted-foreground text-xs font-medium'>
                      {t('Outbox & dead letters')}
                    </span>
                    <Badge
                      variant='outline'
                      className={cn('text-[11px]', outboxBadgeClass)}
                    >
                      {t('Dead letters: {{count}}', {
                        count: snapshot.queues.business_dead_letter,
                      })}
                    </Badge>
                  </div>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <span className='text-muted-foreground flex items-center gap-1 text-[11px]'>
                      <Inbox className='size-3.5' aria-hidden='true' />
                      {t('Pending: {{pending}} · Anomalies: {{anomalies}}', {
                        pending: snapshot.queues.business_pending,
                        anomalies:
                          snapshot.queues.billing_exceptions_count +
                          snapshot.queues.unsettled_pre_consume_count,
                      })}
                    </span>
                    <Link
                      to='/dashboard/$section'
                      params={{ section: 'reconciliation' }}
                      className='text-primary inline-flex items-center gap-1 text-xs font-medium hover:underline'
                    >
                      {t('Reconcile')}
                      <ArrowRight className='size-3' aria-hidden='true' />
                    </Link>
                  </div>
                </div>
              </div>

              {snapshot.alerts.length > 0 && (
                <div className='space-y-2'>
                  {snapshot.alerts.map((alert) => (
                    <Alert
                      key={alert.code}
                      variant={
                        alert.severity === 'critical'
                          ? 'destructive'
                          : 'default'
                      }
                      className={cn(
                        alert.severity === 'warning' &&
                          'border-amber-500/40 bg-amber-500/5'
                      )}
                    >
                      <AlertTriangle className='size-4' />
                      <AlertTitle className='flex flex-wrap items-center justify-between gap-2 text-xs font-semibold'>
                        <span>{t(alert.title)}</span>
                        {alert.action_path === '/dashboard/reconciliation' && (
                          <Link
                            to='/dashboard/$section'
                            params={{ section: 'reconciliation' }}
                            className='text-primary inline-flex items-center gap-1 text-xs font-medium hover:underline'
                          >
                            {t('Open reconciliation')}
                            <ArrowRight className='size-3' />
                          </Link>
                        )}
                        {alert.action_path === '/dashboard/operations' && (
                          <Link
                            to='/dashboard/$section'
                            params={{ section: 'operations' }}
                            className='text-primary inline-flex items-center gap-1 text-xs font-medium hover:underline'
                          >
                            {t('Open operations report')}
                            <ArrowRight className='size-3' />
                          </Link>
                        )}
                      </AlertTitle>
                      <AlertDescription className='text-xs'>
                        {alert.detail}
                      </AlertDescription>
                    </Alert>
                  ))}
                </div>
              )}
            </>
          )
        )}
      </div>
    </section>
  )
}

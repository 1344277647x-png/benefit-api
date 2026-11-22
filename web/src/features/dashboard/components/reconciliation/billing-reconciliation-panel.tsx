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
  AlertTriangle,
  Download,
  ExternalLink,
  FileJson,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useIsMobile } from '@/hooks/use-mobile'
import { formatCurrencyUSD } from '@/lib/format'
import { getApiErrorMessage } from '@/lib/http-client'
import { cn } from '@/lib/utils'

import {
  downloadBillingReconciliation,
  getBillingReconciliation,
} from '../../api'
import type {
  BillingReconciliationItem,
  BillingReconciliationKind,
} from '../../types'

function formatDateInput(date: Date): string {
  return new Date(date.getTime() + 8 * 60 * 60 * 1000)
    .toISOString()
    .slice(0, 10)
}

function parseDateInput(value: string, endOfDay = false): number {
  const parsed = new Date(
    `${value}T${endOfDay ? '23:59:59' : '00:00:00'}+08:00`
  )
  return Number.isNaN(parsed.getTime())
    ? 0
    : Math.floor(parsed.getTime() / 1000)
}

function formatDateTimeUTC8(unixSeconds: number): string {
  if (!unixSeconds || unixSeconds <= 0) return '—'
  return new Date((unixSeconds + 8 * 3600) * 1000)
    .toISOString()
    .replace('T', ' ')
    .slice(0, 19)
}

function kindLabel(
  kind: BillingReconciliationItem['kind'],
  t: (key: string) => string
): string {
  switch (kind) {
    case 'settlement_failed':
      return t('Settlement failed')
    case 'partial_settlement_failed':
      return t('Partial stream settlement failed')
    case 'accounting_projection_failed':
      return t('Accounting projection failed')
    case 'unsettled_pre_consume':
      return t('Unsettled pre-consume')
  }
}

function formatSignedQuota(value?: number): string {
  if (value === undefined) return '—'
  if (value > 0) return `+${value.toLocaleString()}`
  return value.toLocaleString()
}

function formatSignedUSD(value?: number): string {
  if (value === undefined) return '—'
  if (value > 0) return `+${formatCurrencyUSD(value)}`
  return formatCurrencyUSD(value)
}

export function BillingReconciliationPanel() {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const now = useMemo(() => new Date(), [])
  const defaultStart = useMemo(() => {
    const start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
    return formatDateInput(start)
  }, [now])
  const defaultEnd = useMemo(() => formatDateInput(now), [now])

  const [startDate, setStartDate] = useState(defaultStart)
  const [endDate, setEndDate] = useState(defaultEnd)
  const [kind, setKind] = useState<BillingReconciliationKind>('all')
  const [userIdInput, setUserIdInput] = useState('')
  const [requestIdInput, setRequestIdInput] = useState('')
  const [exporting, setExporting] = useState<'csv' | 'json' | null>(null)

  const queryParams = useMemo(() => {
    const startTimestamp = parseDateInput(startDate, false)
    const endTimestamp = parseDateInput(endDate, true)
    const parsedUserId = Number.parseInt(userIdInput.trim(), 10)
    return {
      start_timestamp: startTimestamp,
      end_timestamp: endTimestamp,
      kind,
      ...(Number.isFinite(parsedUserId) && parsedUserId > 0
        ? { user_id: parsedUserId }
        : {}),
      ...(requestIdInput.trim() ? { request_id: requestIdInput.trim() } : {}),
    }
  }, [startDate, endDate, kind, userIdInput, requestIdInput])

  const validRange =
    queryParams.start_timestamp > 0 &&
    queryParams.end_timestamp >= queryParams.start_timestamp

  const reconciliationQuery = useQuery({
    queryKey: ['billing-reconciliation', queryParams],
    queryFn: () => getBillingReconciliation(queryParams),
    enabled: validRange,
    retry: false,
  })

  const report = reconciliationQuery.data?.data

  const handleReset = () => {
    setStartDate(defaultStart)
    setEndDate(defaultEnd)
    setKind('all')
    setUserIdInput('')
    setRequestIdInput('')
  }

  const handleExport = async (format: 'csv' | 'json') => {
    if (!validRange) {
      toast.error(t('Please select a valid date range'))
      return
    }
    setExporting(format)
    try {
      const blob = await downloadBillingReconciliation(queryParams, format)
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `benefit-api-billing-reconciliation-${startDate}_${endDate}.${format}`
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
      toast.success(
        format === 'csv'
          ? t('CSV export downloaded')
          : t('JSON export downloaded')
      )
    } catch (error) {
      toast.error(
        getApiErrorMessage(
          error,
          t('Unable to export billing reconciliation report')
        )
      )
    } finally {
      setExporting(null)
    }
  }

  const kindOptions: Array<{
    value: BillingReconciliationKind
    label: string
  }> = [
    { value: 'all', label: t('All anomaly types') },
    { value: 'settlement_failed', label: t('Settlement failed') },
    {
      value: 'partial_settlement_failed',
      label: t('Partial stream settlement failed'),
    },
    {
      value: 'unsettled_pre_consume',
      label: t('Unsettled pre-consume'),
    },
    {
      value: 'accounting_projection_failed',
      label: t('Accounting projection failed'),
    },
  ]

  return (
    <div className='space-y-4'>
      <Alert className='border-emerald-500/30 bg-emerald-500/5'>
        <ShieldCheck className='size-4 text-emerald-600 dark:text-emerald-400' />
        <AlertTitle>{t('Read-only settlement reconciliation')}</AlertTitle>
        <AlertDescription>
          {t(
            'This panel aggregates unsettled pre-consume records and settlement failure events for manual audit and export. It never modifies balances, subscriptions, or billing records.'
          )}
        </AlertDescription>
      </Alert>

      <Card className='benefit-cinema-panel'>
        <CardHeader className='pb-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div>
              <CardTitle className='text-base'>
                {t('Reconciliation filters')}
              </CardTitle>
              <CardDescription>
                {t(
                  'Filter by date range, anomaly type, user ID, or exact request ID.'
                )}
              </CardDescription>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={handleReset}
              >
                <RotateCcw className='mr-1.5 size-3.5' />
                {t('Reset')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => void reconciliationQuery.refetch()}
                disabled={reconciliationQuery.isFetching || !validRange}
              >
                <RefreshCw
                  className={cn(
                    'mr-1.5 size-3.5',
                    reconciliationQuery.isFetching && 'animate-spin'
                  )}
                />
                {t('Refresh')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => void handleExport('csv')}
                disabled={exporting !== null || !validRange}
              >
                <Download className='mr-1.5 size-3.5' />
                {t('Export CSV')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => void handleExport('json')}
                disabled={exporting !== null || !validRange}
              >
                <FileJson className='mr-1.5 size-3.5' />
                {t('Export JSON')}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5'>
            <div className='space-y-1'>
              <label className='text-muted-foreground text-xs font-medium'>
                {t('Start date')}
              </label>
              <Input
                type='date'
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
              />
            </div>
            <div className='space-y-1'>
              <label className='text-muted-foreground text-xs font-medium'>
                {t('End date')}
              </label>
              <Input
                type='date'
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
              />
            </div>
            <div className='space-y-1'>
              <label className='text-muted-foreground text-xs font-medium'>
                {t('Anomaly type')}
              </label>
              <Select
                value={kind}
                onValueChange={(value) => {
                  if (value) setKind(value as BillingReconciliationKind)
                }}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {kindOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className='space-y-1'>
              <label className='text-muted-foreground text-xs font-medium'>
                {t('User ID')}
              </label>
              <Input
                inputMode='numeric'
                placeholder={t('Optional user ID')}
                value={userIdInput}
                onChange={(e) => setUserIdInput(e.target.value)}
              />
            </div>
            <div className='space-y-1'>
              <label className='text-muted-foreground text-xs font-medium'>
                {t('Request ID')}
              </label>
              <Input
                placeholder={t('Exact request ID')}
                value={requestIdInput}
                onChange={(e) => setRequestIdInput(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {reconciliationQuery.isError && (
        <Alert variant='destructive'>
          <AlertTriangle className='size-4' />
          <AlertTitle>
            {t('Unable to load billing reconciliation report')}
          </AlertTitle>
          <AlertDescription>
            {getApiErrorMessage(
              reconciliationQuery.error,
              t('Please check your filters and try again.')
            )}
          </AlertDescription>
        </Alert>
      )}

      {report && (
        <>
          {report.truncated && (
            <Alert className='border-amber-500/40 bg-amber-500/5'>
              <AlertTriangle className='size-4 text-amber-600' />
              <AlertTitle>{t('Result truncated')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Results reached the query limit. Narrow the date range or filter by user ID for complete reconciliation.'
                )}
              </AlertDescription>
            </Alert>
          )}

          <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            <Card className='benefit-cinema-panel'>
              <CardHeader className='pb-1'>
                <CardDescription>{t('Total anomalies')}</CardDescription>
                <CardTitle className='text-2xl tabular-nums'>
                  {report.summary.total_items}
                </CardTitle>
              </CardHeader>
              <CardContent className='text-muted-foreground text-xs'>
                {t(
                  'Settlement: {{settlement}} · Pre-consume: {{pre}} · Projection: {{proj}}',
                  {
                    settlement: report.summary.settlement_exception_count,
                    pre: report.summary.unsettled_pre_consume_count,
                    proj: report.summary.projection_failure_count,
                  }
                )}
              </CardContent>
            </Card>

            <Card className='benefit-cinema-panel'>
              <CardHeader className='pb-1'>
                <CardDescription>{t('Known pre-consumed')}</CardDescription>
                <CardTitle className='text-2xl tabular-nums'>
                  {formatCurrencyUSD(report.summary.known_pre_consumed_usd)}
                </CardTitle>
              </CardHeader>
              <CardContent className='text-muted-foreground text-xs tabular-nums'>
                {t('{{quota}} quota units', {
                  quota:
                    report.summary.known_pre_consumed_quota.toLocaleString(),
                })}
              </CardContent>
            </Card>

            <Card className='benefit-cinema-panel'>
              <CardHeader className='pb-1'>
                <CardDescription>
                  {t('Known funding difference')}
                </CardDescription>
                <CardTitle
                  className={cn(
                    'text-2xl tabular-nums',
                    report.summary.known_net_diff_quota > 0 && 'text-amber-600',
                    report.summary.known_net_diff_quota < 0 && 'text-sky-600'
                  )}
                >
                  {formatSignedUSD(report.summary.known_net_diff_usd)}
                </CardTitle>
              </CardHeader>
              <CardContent className='text-muted-foreground text-xs tabular-nums'>
                {t('{{quota}} quota units', {
                  quota: formatSignedQuota(report.summary.known_net_diff_quota),
                })}
                <div>
                  {t('Token differences are excluded from the funding total.')}
                </div>
              </CardContent>
            </Card>

            <Card className='benefit-cinema-panel'>
              <CardHeader className='pb-1'>
                <CardDescription>
                  {t('Unknown-actual pre-consumed')}
                </CardDescription>
                <CardTitle className='text-2xl tabular-nums'>
                  {formatCurrencyUSD(report.summary.unknown_pre_consumed_usd)}
                </CardTitle>
              </CardHeader>
              <CardContent className='text-muted-foreground text-xs'>
                {t(
                  '{{count}} record(s) without consume log (excluded from net diff)',
                  {
                    count: report.summary.unknown_actual_count,
                  }
                )}
              </CardContent>
            </Card>
          </div>

          <Card className='benefit-cinema-panel'>
            <CardHeader className='pb-3'>
              <CardTitle className='text-base'>
                {t('Anomaly records')}
              </CardTitle>
              <CardDescription>
                {t(
                  'Positive diff means actual consumption exceeded pre-consumed quota; negative diff means over-reserved candidate. Records without a consume log stay marked Unknown.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='p-0'>
              {report.items.length === 0 && (
                <div className='text-muted-foreground px-6 py-10 text-center text-sm'>
                  {t('No settlement anomalies found in the selected range.')}
                </div>
              )}
              {report.items.length > 0 && isMobile && (
                <div className='divide-y'>
                  {report.items.map((item) => {
                    const rowKey = `${item.source}:${item.event_key || item.request_id}:${item.created_at}`
                    let kindBadgeClass =
                      'border-amber-500/40 text-amber-600 dark:text-amber-400'
                    if (item.kind === 'accounting_projection_failed') {
                      kindBadgeClass =
                        'border-sky-500/40 text-sky-600 dark:text-sky-400'
                    } else if (item.dead_letter_at) {
                      kindBadgeClass = 'border-destructive/50 text-destructive'
                    }
                    let actualMobileNode = (
                      <span className='text-muted-foreground'>
                        {t('Unknown')}
                      </span>
                    )
                    if (
                      item.actual_quota !== undefined &&
                      item.actual_usd !== undefined
                    ) {
                      actualMobileNode = (
                        <div>
                          <div className='font-medium'>
                            {formatCurrencyUSD(item.actual_usd)}
                          </div>
                          <div className='text-muted-foreground text-[11px]'>
                            {item.actual_quota.toLocaleString()}
                          </div>
                        </div>
                      )
                    } else if (item.projected_quota !== undefined) {
                      actualMobileNode = (
                        <span className='text-muted-foreground text-[11px]'>
                          {t('Projected: {{quota}}', {
                            quota: item.projected_quota.toLocaleString(),
                          })}
                        </span>
                      )
                    }

                    let fundingDiffMobileNode = (
                      <span className='text-muted-foreground'>
                        {t('Unknown')}
                      </span>
                    )
                    if (
                      item.diff_quota !== undefined &&
                      item.diff_usd !== undefined
                    ) {
                      fundingDiffMobileNode = (
                        <div
                          className={cn(
                            'font-medium',
                            item.diff_quota > 0 &&
                              'text-amber-600 dark:text-amber-400',
                            item.diff_quota < 0 &&
                              'text-sky-600 dark:text-sky-400'
                          )}
                        >
                          <div>{formatSignedUSD(item.diff_usd)}</div>
                          <div className='text-[11px] opacity-80'>
                            {formatSignedQuota(item.diff_quota)}
                          </div>
                        </div>
                      )
                    }

                    const timeRangeQuery =
                      item.created_at > 0
                        ? `&startTime=${Math.max(1, item.created_at - 3600) * 1000}&endTime=${(item.created_at + 3600) * 1000}`
                        : ''

                    return (
                      <div key={rowKey} className='space-y-2.5 p-4 text-xs'>
                        <div className='flex flex-wrap items-center justify-between gap-2'>
                          <Badge variant='outline' className={kindBadgeClass}>
                            {kindLabel(item.kind, t)}
                          </Badge>
                          <span className='text-muted-foreground font-mono text-[11px]'>
                            {formatDateTimeUTC8(item.created_at)}
                          </span>
                        </div>

                        <div className='flex flex-wrap items-baseline justify-between gap-2'>
                          <div>
                            <span className='font-medium'>
                              {item.username || `#${item.user_id}`}
                            </span>
                            <span className='text-muted-foreground ml-1.5 text-[11px]'>
                              ID {item.user_id}
                              {item.subscription_id
                                ? ` · Sub #${item.subscription_id}`
                                : ''}
                            </span>
                          </div>
                          {item.model_name && (
                            <span className='text-muted-foreground truncate font-mono text-[11px]'>
                              {item.model_name}
                            </span>
                          )}
                        </div>

                        <div className='font-mono'>
                          <span className='text-muted-foreground mr-1.5 font-sans'>
                            {t('Request ID')}:
                          </span>
                          {item.request_id ? (
                            <a
                              href={`/usage-logs/common?requestId=${encodeURIComponent(item.request_id)}${timeRangeQuery}`}
                              className='text-primary inline-flex max-w-full items-center gap-1 truncate align-bottom hover:underline'
                            >
                              <span className='truncate'>
                                {item.request_id}
                              </span>
                              <ExternalLink className='size-3 shrink-0' />
                            </a>
                          ) : (
                            '—'
                          )}
                        </div>

                        <div className='bg-muted/40 grid grid-cols-2 gap-2 rounded-lg p-2.5 font-mono tabular-nums'>
                          <div>
                            <div className='text-muted-foreground font-sans text-[11px]'>
                              {t('Pre-consumed')}
                            </div>
                            <div className='font-medium'>
                              {formatCurrencyUSD(item.pre_consumed_usd)}
                            </div>
                            <div className='text-muted-foreground text-[11px]'>
                              {item.pre_consumed_quota.toLocaleString()}
                            </div>
                          </div>
                          <div>
                            <div className='text-muted-foreground font-sans text-[11px]'>
                              {t('Actual')}
                            </div>
                            {actualMobileNode}
                          </div>
                          <div>
                            <div className='text-muted-foreground font-sans text-[11px]'>
                              {t('Funding difference')}
                            </div>
                            {fundingDiffMobileNode}
                          </div>
                          <div>
                            <div className='text-muted-foreground font-sans text-[11px]'>
                              {t('Token difference (quota)')}
                            </div>
                            <div>
                              {item.token_diff_quota == null
                                ? t('Unknown')
                                : formatSignedQuota(item.token_diff_quota)}
                            </div>
                          </div>
                        </div>

                        <div>
                          <div className='font-medium'>
                            {item.failure_stage || item.status}
                          </div>
                          {item.summary && (
                            <div className='text-muted-foreground line-clamp-2 text-[11px]'>
                              {item.summary}
                            </div>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
              {report.items.length > 0 && !isMobile && (
                <div className='overflow-x-auto'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Time (UTC+8)')}</TableHead>
                        <TableHead>{t('Type')}</TableHead>
                        <TableHead>{t('User')}</TableHead>
                        <TableHead>{t('Request ID')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Pre-consumed')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Actual')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Funding difference')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Token difference (quota)')}
                        </TableHead>
                        <TableHead>{t('Stage / Status')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {report.items.map((item) => {
                        const rowKey = `${item.source}:${item.event_key || item.request_id}:${item.created_at}`
                        let kindBadgeClass =
                          'border-amber-500/40 text-amber-600'
                        if (item.kind === 'accounting_projection_failed') {
                          kindBadgeClass = 'border-sky-500/40 text-sky-600'
                        } else if (item.dead_letter_at) {
                          kindBadgeClass =
                            'border-destructive/50 text-destructive'
                        }
                        let actualCell = (
                          <Badge
                            variant='outline'
                            className='text-muted-foreground text-[11px]'
                          >
                            {t('Unknown')}
                          </Badge>
                        )
                        if (
                          item.actual_quota !== undefined &&
                          item.actual_usd !== undefined
                        ) {
                          actualCell = (
                            <>
                              <div>{formatCurrencyUSD(item.actual_usd)}</div>
                              <div className='text-muted-foreground text-[11px]'>
                                {item.actual_quota.toLocaleString()}
                              </div>
                            </>
                          )
                        } else if (item.projected_quota !== undefined) {
                          actualCell = (
                            <span className='text-muted-foreground text-[11px]'>
                              {t('Projected: {{quota}}', {
                                quota: item.projected_quota.toLocaleString(),
                              })}
                            </span>
                          )
                        }
                        return (
                          <TableRow key={rowKey}>
                            <TableCell className='font-mono text-xs whitespace-nowrap'>
                              {formatDateTimeUTC8(item.created_at)}
                            </TableCell>
                            <TableCell className='whitespace-nowrap'>
                              <Badge
                                variant='outline'
                                className={kindBadgeClass}
                              >
                                {kindLabel(item.kind, t)}
                              </Badge>
                            </TableCell>
                            <TableCell className='text-xs whitespace-nowrap'>
                              <div className='font-medium'>
                                {item.username || `#${item.user_id}`}
                              </div>
                              <div className='text-muted-foreground text-[11px]'>
                                ID {item.user_id}
                                {item.subscription_id
                                  ? ` · Sub #${item.subscription_id}`
                                  : ''}
                              </div>
                            </TableCell>
                            <TableCell className='max-w-[220px] font-mono text-xs'>
                              {item.request_id ? (
                                <a
                                  href={`/usage-logs/common?requestId=${encodeURIComponent(item.request_id)}${item.created_at > 0 ? `&startTime=${Math.max(1, item.created_at - 3600) * 1000}&endTime=${(item.created_at + 3600) * 1000}` : ''}`}
                                  className='text-primary inline-flex items-center gap-1 truncate hover:underline'
                                >
                                  <span className='truncate'>
                                    {item.request_id}
                                  </span>
                                  <ExternalLink className='size-3 shrink-0' />
                                </a>
                              ) : (
                                '—'
                              )}
                              {item.model_name && (
                                <div className='text-muted-foreground truncate text-[11px]'>
                                  {item.model_name}
                                </div>
                              )}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs whitespace-nowrap tabular-nums'>
                              <div>
                                {formatCurrencyUSD(item.pre_consumed_usd)}
                              </div>
                              <div className='text-muted-foreground text-[11px]'>
                                {item.pre_consumed_quota.toLocaleString()}
                              </div>
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs whitespace-nowrap tabular-nums'>
                              {actualCell}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs whitespace-nowrap tabular-nums'>
                              {item.diff_quota !== undefined &&
                              item.diff_usd !== undefined ? (
                                <div
                                  className={cn(
                                    'font-medium',
                                    item.diff_quota > 0 && 'text-amber-600',
                                    item.diff_quota < 0 && 'text-sky-600'
                                  )}
                                >
                                  <div>{formatSignedUSD(item.diff_usd)}</div>
                                  <div className='text-[11px] opacity-80'>
                                    {formatSignedQuota(item.diff_quota)}
                                  </div>
                                </div>
                              ) : (
                                t('Unknown')
                              )}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs whitespace-nowrap tabular-nums'>
                              {item.token_diff_quota == null
                                ? t('Unknown')
                                : formatSignedQuota(item.token_diff_quota)}
                            </TableCell>
                            <TableCell className='max-w-[260px] text-xs'>
                              <div className='font-medium'>
                                {item.failure_stage || item.status}
                              </div>
                              <div className='text-muted-foreground line-clamp-2 text-[11px]'>
                                {item.summary}
                              </div>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

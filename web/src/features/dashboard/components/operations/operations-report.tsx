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
import { AlertTriangle, Download, RefreshCw, RotateCcw } from 'lucide-react'
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
import { formatCompactNumber, formatCurrencyUSD } from '@/lib/format'
import { getApiErrorMessage } from '@/lib/http-client'
import { cn } from '@/lib/utils'

import { downloadOperationsReport, getOperationsReport } from '../../api'
import type {
  OperationsReport,
  OperationsReportGroupBy,
  OperationsReportHealth,
} from '../../types'
import { AccountingCoverage } from './accounting-coverage'
import { ChannelCostSettings } from './channel-cost-settings'

function dateInputValue(date: Date) {
  return new Date(date.getTime() + 8 * 60 * 60 * 1000)
    .toISOString()
    .slice(0, 10)
}

function dateInputTimestamp(value: string, endOfDay = false) {
  const date = new Date(`${value}T${endOfDay ? '23:59:59' : '00:00:00'}+08:00`)
  return Number.isNaN(date.getTime()) ? 0 : Math.floor(date.getTime() / 1000)
}

function costLabel(
  status: OperationsReportRowCostStatus,
  t: (key: string) => string
) {
  if (status === 'known') return t('Known')
  if (status === 'partial') return t('Partial')
  return t('Unable to calculate')
}

type OperationsReportRowCostStatus = 'known' | 'partial' | 'unknown'

function CostBadge({ status }: { status: OperationsReportRowCostStatus }) {
  const { t } = useTranslation()
  return (
    <Badge
      variant={status === 'known' ? 'secondary' : 'outline'}
      className={cn(
        status === 'partial' && 'border-amber-500/50 text-amber-600',
        status === 'unknown' && 'border-destructive/40 text-destructive'
      )}
    >
      {costLabel(status, t)}
    </Badge>
  )
}

function HealthBadge({ status }: { status: OperationsReportHealth['status'] }) {
  const { t } = useTranslation()
  const labels: Record<OperationsReportHealth['status'], string> = {
    normal: t('Normal'),
    delayed: t('Delayed'),
    unavailable: t('Unavailable'),
    unknown: t('Not detected'),
  }
  return (
    <Badge
      variant='outline'
      className={cn(
        status === 'normal' && 'border-emerald-500/50 text-emerald-600',
        status === 'delayed' && 'border-amber-500/50 text-amber-600',
        status === 'unavailable' && 'border-destructive/40 text-destructive'
      )}
    >
      {labels[status]}
    </Badge>
  )
}

export function OperationsReport() {
  const { t } = useTranslation()
  const today = new Date()
  const [startDate, setStartDate] = useState(dateInputValue(today))
  const [endDate, setEndDate] = useState(dateInputValue(today))
  const [groupBy, setGroupBy] = useState<OperationsReportGroupBy>('day')
  const [modelName, setModelName] = useState('')
  const [username, setUsername] = useState('')
  const [group, setGroup] = useState('')
  const [channelId, setChannelId] = useState('')
  const [costChannelId, setCostChannelId] = useState<number | undefined>()
  const [afterCostEffective, setAfterCostEffective] = useState(false)

  const handleSelectChannel = (nextChannelId: number) => {
    if (nextChannelId > 0) {
      setCostChannelId(nextChannelId)
    }
  }

  const params = useMemo(
    () => ({
      start_timestamp: dateInputTimestamp(startDate),
      end_timestamp: dateInputTimestamp(endDate, true),
      group_by: groupBy,
      after_cost_effective: afterCostEffective,
      ...(modelName.trim() ? { model_name: modelName.trim() } : {}),
      ...(username.trim() ? { username: username.trim() } : {}),
      ...(group.trim() ? { group: group.trim() } : {}),
      ...(channelId.trim() ? { channel_id: Number(channelId) } : {}),
    }),
    [
      afterCostEffective,
      channelId,
      endDate,
      group,
      groupBy,
      modelName,
      startDate,
      username,
    ]
  )

  const reportQuery = useQuery({
    queryKey: ['operations-report', params],
    queryFn: () => getOperationsReport(params),
    enabled:
      params.start_timestamp > 0 &&
      params.end_timestamp >= params.start_timestamp,
    staleTime: 30 * 1000,
  })
  const report = reportQuery.data?.data

  const clearFilters = () => {
    const now = new Date()
    setStartDate(dateInputValue(now))
    setEndDate(dateInputValue(now))
    setGroupBy('day')
    setModelName('')
    setUsername('')
    setGroup('')
    setChannelId('')
    setAfterCostEffective(false)
  }

  const download = async () => {
    try {
      const blob = await downloadOperationsReport(params)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = 'benefit-api-operations-report.csv'
      anchor.click()
      URL.revokeObjectURL(url)
    } catch (error) {
      toast.error(getApiErrorMessage(error, t('Request failed')))
    }
  }

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader className='gap-3 border-b'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div>
              <div className='flex flex-wrap items-center gap-2'>
                <CardTitle>{t('Operations report')}</CardTitle>
                {report?.pre_aggregated && (
                  <Badge
                    variant='outline'
                    className='border-emerald-500/40 text-emerald-600 dark:text-emerald-400'
                  >
                    {t('Hourly rollups ({{hours}}h)', {
                      hours: report.rollup_hours_used ?? 0,
                    })}
                  </Badge>
                )}
              </div>
              <CardDescription>
                {t('Revenue and usage by model, channel and user.')}
              </CardDescription>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void reportQuery.refetch()}
                disabled={reportQuery.isFetching}
              >
                <RefreshCw
                  className={cn(reportQuery.isFetching && 'animate-spin')}
                />
                {t('Refresh')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void download()}
                disabled={!report || reportQuery.isFetching}
              >
                <Download />
                {t('Export CSV')}
              </Button>
            </div>
          </div>
          <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
            <label className='text-muted-foreground text-xs'>
              {t('Start date')}
              <Input
                type='date'
                value={startDate}
                onChange={(event) => setStartDate(event.target.value)}
                className='mt-1'
              />
            </label>
            <label className='text-muted-foreground text-xs'>
              {t('End date')}
              <Input
                type='date'
                value={endDate}
                onChange={(event) => setEndDate(event.target.value)}
                className='mt-1'
              />
            </label>
            <label className='text-muted-foreground text-xs'>
              {t('Group by')}
              <Select
                value={groupBy}
                onValueChange={(value) =>
                  value && setGroupBy(value as OperationsReportGroupBy)
                }
              >
                <SelectTrigger className='mt-1 w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='day'>{t('Day')}</SelectItem>
                  <SelectItem value='model'>{t('Model')}</SelectItem>
                  <SelectItem value='channel'>{t('Channel')}</SelectItem>
                  <SelectItem value='user'>{t('User')}</SelectItem>
                  <SelectItem value='group'>{t('Group')}</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <div className='flex items-end'>
              <Button
                variant='ghost'
                size='sm'
                onClick={clearFilters}
                className='w-full sm:w-auto'
              >
                <RotateCcw />
                {t('Reset filters')}
              </Button>
            </div>
          </div>
          <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
            <Input
              value={modelName}
              onChange={(event) => setModelName(event.target.value)}
              placeholder={t('Filter by model')}
              aria-label={t('Filter by model')}
            />
            <Input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder={t('Filter by user')}
              aria-label={t('Filter by user')}
            />
            <Input
              value={group}
              onChange={(event) => setGroup(event.target.value)}
              placeholder={t('Filter by group')}
              aria-label={t('Filter by group')}
            />
            <Input
              value={channelId}
              onChange={(event) =>
                setChannelId(event.target.value.replaceAll(/\D/g, ''))
              }
              placeholder={t('Channel ID')}
              aria-label={t('Channel ID')}
              inputMode='numeric'
            />
          </div>
          <label className='flex items-center gap-2 text-sm'>
            <input
              type='checkbox'
              checked={afterCostEffective}
              onChange={(event) => setAfterCostEffective(event.target.checked)}
            />
            {t('Only requests after each channel first cost version')}
          </label>
        </CardHeader>
        <CardContent className='space-y-4 pt-4'>
          {report?.truncated && (
            <Alert variant='default'>
              <AlertTriangle />
              <AlertTitle>{t('Report was capped')}</AlertTitle>
              <AlertDescription>
                {t(
                  'The selected range contains more records than the report limit. Narrow the date range for a complete result.'
                )}
              </AlertDescription>
            </Alert>
          )}
          {report?.warnings?.map((warning) => (
            <Alert key={warning} variant='default'>
              <AlertTriangle />
              <AlertDescription>{t(warning)}</AlertDescription>
            </Alert>
          ))}
          {reportQuery.isError && (
            <Alert variant='destructive'>
              <AlertTriangle />
              <AlertTitle>{t('Unable to load operations report')}</AlertTitle>
              <AlertDescription>
                {reportQuery.error instanceof Error
                  ? reportQuery.error.message
                  : t('Try a shorter date range or refresh the page.')}
              </AlertDescription>
            </Alert>
          )}
          <SummaryGrid report={report} />
          {report && (
            <AccountingCoverage
              data={report.summary}
              onSelectChannel={handleSelectChannel}
            />
          )}
          <p className='text-muted-foreground text-sm'>
            {t(
              'Partial profit uses only requests with both verified income and cost; it is not an estimate of total profit.'
            )}
          </p>
          {report?.after_cost_effective && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Filtered by each channel first effective time; requests without a matching version or start time are excluded.'
              )}{' '}
              {t('Excluded requests')}: {report.excluded_requests ?? 0}
            </p>
          )}
          {!!report?.accounting_started_at && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Accounting starts at the first deployment; earlier usage is excluded.'
              )}{' '}
              {new Date(report.accounting_started_at * 1000).toLocaleString(
                'zh-CN',
                { timeZone: 'Asia/Shanghai' }
              )}
            </p>
          )}
          {report?.funding && (
            <div className='grid gap-3 text-sm sm:grid-cols-2'>
              <div>
                {t('Confirmed cash receipts (not consumption revenue)')}:{' '}
                {formatCurrencyUSD(report.funding.receipts_known_usd)}
              </div>
              <div>
                {t('Expiry remainder income (not allocated to channels)')}:{' '}
                {formatCurrencyUSD(report.funding.expiry_known_usd)}
              </div>
              {(report.funding.unknown_entries > 0 ||
                report.funding.truncated) && (
                <p className='text-amber-600 sm:col-span-2'>
                  {t(
                    'Funding totals are incomplete; unverified entries are excluded.'
                  )}
                </p>
              )}
            </div>
          )}
          {!!report?.summary.unknown_revenue_requests && (
            <Alert>
              <AlertDescription>
                {t(
                  'Some income sources are unverified. Revenue shows only the confirmed portion; gross profit is unavailable.'
                )}
              </AlertDescription>
            </Alert>
          )}
          <ReportTable report={report} onSelectChannel={handleSelectChannel} />
        </CardContent>
      </Card>
      <ChannelOperationsCard
        report={report}
        onSelectChannel={handleSelectChannel}
      />
      <ChannelCostSettings selectedChannelId={costChannelId} />
    </div>
  )
}

function SummaryGrid({ report }: { report?: OperationsReport }) {
  const { t } = useTranslation()
  const summary = report?.summary
  let costValue = '-'
  if (summary) {
    costValue =
      summary.cost_status === 'unknown'
        ? t('Unable to calculate')
        : formatCurrencyUSD(summary.cost_usd)
  }
  const cards = [
    [t('Requests'), summary ? formatCompactNumber(summary.request_count) : '-'],
    [t('Error rate'), summary ? `${summary.error_rate.toFixed(2)}%` : '-'],
    [t('Tokens'), summary ? formatCompactNumber(summary.token_count) : '-'],
    [
      t('Confirmed income'),
      summary
        ? formatCurrencyUSD(summary.revenue_usd - summary.refund_usd)
        : '-',
    ],
    [t('Known cost'), costValue],
    [
      t('Gross profit'),
      summary?.profit_usd == null
        ? t('Pending verification')
        : formatCurrencyUSD(summary.profit_usd),
    ],
  ]
  return (
    <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-6'>
      {cards.map(([label, value]) => (
        <div key={label} className='bg-muted/30 rounded-lg border px-3 py-3'>
          <div className='text-muted-foreground text-xs'>{label}</div>
          <div className='mt-1 text-lg font-semibold tabular-nums'>{value}</div>
        </div>
      ))}
    </div>
  )
}

function ReportTable({
  report,
  onSelectChannel,
}: {
  report?: OperationsReport
  onSelectChannel?: (channelId: number) => void
}) {
  const { t } = useTranslation()
  if (!report) return null
  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{t('Breakdown')}</h3>
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} rows', { count: report.rows.length })}
        </span>
      </div>
      <div className='overflow-x-auto'>
        <Table className='min-w-[760px]'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Dimension')}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead className='text-right'>{t('Error rate')}</TableHead>
              <TableHead className='text-right'>{t('Tokens')}</TableHead>
              <TableHead className='text-right'>
                {t('Consumed quota value')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Confirmed income')}
              </TableHead>
              <TableHead className='text-right'>{t('Refunds')}</TableHead>
              <TableHead>{t('Cost status')}</TableHead>
              <TableHead>{t('Known cost')}</TableHead>
              <TableHead>{t('Accounting coverage')}</TableHead>
              <TableHead className='text-right'>{t('Profit')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {report.rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={11}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No report data for this range')}
                </TableCell>
              </TableRow>
            ) : (
              report.rows.map((row) => {
                const rowChannelId = row.channel_id
                return (
                  <TableRow key={row.key}>
                    <TableCell>
                      {rowChannelId && rowChannelId > 0 && onSelectChannel ? (
                        <button
                          type='button'
                          onClick={() => onSelectChannel(rowChannelId)}
                          className='text-left font-medium underline underline-offset-2'
                        >
                          {row.label}
                        </button>
                      ) : (
                        <div className='font-medium'>{row.label}</div>
                      )}
                      {row.model_name && report.group_by !== 'model' && (
                        <div className='text-muted-foreground text-xs'>
                          {row.model_name}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatCompactNumber(row.request_count)}
                    </TableCell>
                    <TableCell className='text-right'>
                      {row.error_rate.toFixed(2)}%
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatCompactNumber(row.token_count)}
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatCurrencyUSD(
                        row.revenue_quota / report.quota_per_unit
                      )}
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatCurrencyUSD(row.revenue_usd)}
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatCurrencyUSD(row.refund_usd)}
                    </TableCell>
                    <TableCell>
                      <CostBadge status={row.cost_status} />
                    </TableCell>
                    <TableCell>
                      {row.cost_status === 'unknown'
                        ? t('Pending verification')
                        : formatCurrencyUSD(row.cost_usd)}
                    </TableCell>
                    <TableCell className='min-w-64'>
                      <AccountingCoverage
                        data={row}
                        onSelectChannel={onSelectChannel}
                      />
                    </TableCell>
                    <TableCell className='text-right'>
                      {row.profit_usd == null
                        ? t('Pending verification')
                        : formatCurrencyUSD(row.profit_usd)}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

function ChannelOperationsCard({
  report,
  onSelectChannel,
}: {
  report?: OperationsReport
  onSelectChannel?: (channelId: number) => void
}) {
  const { t } = useTranslation()
  if (!report || (report.channels.length === 0 && report.health.length === 0)) {
    return null
  }
  const healthByChannel = new Map<number, OperationsReportHealth[]>()
  report.health.forEach((item) => {
    const list = healthByChannel.get(item.channel_id) ?? []
    list.push(item)
    healthByChannel.set(item.channel_id, list)
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Channel operations')}</CardTitle>
        <CardDescription>
          {t('Balance and live quality for channels in this report.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='overflow-x-auto'>
          <Table className='min-w-[560px]'>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Balance')}</TableHead>
                <TableHead>{t('Response time')}</TableHead>
                <TableHead>{t('Health')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {report.channels.map((channel) => {
                const health = healthByChannel.get(channel.channel_id) ?? []
                return (
                  <TableRow key={channel.channel_id}>
                    <TableCell>
                      <div className='font-medium'>
                        {onSelectChannel ? (
                          <button
                            type='button'
                            onClick={() => onSelectChannel(channel.channel_id)}
                            className='text-left underline underline-offset-2'
                          >
                            {channel.name || `#${channel.channel_id}`}
                          </button>
                        ) : (
                          channel.name || `#${channel.channel_id}`
                        )}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        #{channel.channel_id}
                      </div>
                    </TableCell>
                    <TableCell>{formatCurrencyUSD(channel.balance)}</TableCell>
                    <TableCell>
                      {channel.response_time > 0
                        ? `${channel.response_time} ms`
                        : '-'}
                    </TableCell>
                    <TableCell>
                      <div className='flex flex-wrap gap-1'>
                        {health.length === 0 ? (
                          <HealthBadge status='unknown' />
                        ) : (
                          health
                            .slice(0, 3)
                            .map((item) => (
                              <HealthBadge
                                key={`${item.channel_id}:${item.model}`}
                                status={item.status}
                              />
                            ))
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}

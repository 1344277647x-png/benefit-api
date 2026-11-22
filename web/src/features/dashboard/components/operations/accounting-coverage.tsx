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
import { useTranslation } from 'react-i18next'

import { formatCurrencyUSD } from '@/lib/format'

import type { OperationsReportSummary } from '../../types'

export function AccountingCoverage(props: {
  data: OperationsReportSummary
  onSelectChannel?: (channelId: number) => void
}) {
  const { t } = useTranslation()
  const data = props.data
  const reasons: Record<string, string> = {
    failed_request_unverified: t('Failed request requires verification'),
    failed_cost_unknown: t('Failed request cost is unknown'),
    retry_cost_unknown: t('Retry cost is unknown'),
    income_record_missing: t('Income record is missing or inconsistent'),
    funding_source_unknown: t('Funding source is unverified'),
    request_start_missing: t('Request start time is missing'),
    cost_version_missing: t('No effective cost version in this range'),
    before_cost_effective: t('Request predates the cost multiplier'),
    billing_metadata_incomplete: t(
      'Billing details are incomplete or unsupported'
    ),
  }
  const matched = data.matched_requests ?? 0
  const requestCoverage =
    data.request_count > 0
      ? `${((matched / data.request_count) * 100).toFixed(2)}%`
      : '—'
  const quotaCoverage =
    data.revenue_quota > 0
      ? `${(((data.matched_quota ?? 0) / data.revenue_quota) * 100).toFixed(2)}%`
      : '—'
  return (
    <div className='space-y-2 text-xs'>
      <p>
        {t('Calculable partial profit')}:{' '}
        {data.partial_profit_usd == null
          ? t('Pending verification')
          : formatCurrencyUSD(data.partial_profit_usd)}
      </p>
      <p>
        {t('Calculable requests')}: {matched} / {data.request_count} (
        {requestCoverage})
      </p>
      <p>
        {t('Calculable consumption coverage')}: {quotaCoverage}
      </p>
      <p>
        {t('Pending requests')}: {data.pending_requests ?? 0}
      </p>
      <p>
        {t('Failed records without consumption quota')}:{' '}
        {data.errors_without_quota ?? 0}
      </p>
      {!!data.pending_requests && (
        <details className='max-w-lg break-words whitespace-normal'>
          <summary className='cursor-pointer'>
            {t('Verification reasons and request samples')}
          </summary>
          <p className='text-muted-foreground my-2'>
            {t(
              'Reasons may overlap. Up to 20 request samples are shown per group; narrow filters to inspect more.'
            )}
          </p>
          <ul className='space-y-1'>
            {Object.entries(data.reasons ?? {}).map(([reason, count]) => (
              <li key={reason}>
                {reasons[reason] ?? t('Pending verification')}: {count}
              </li>
            ))}
          </ul>
          <ul className='mt-2 space-y-2'>
            {(data.pending_samples ?? []).map((sample, index) => (
              <li
                key={
                  sample.log_id !== undefined && sample.log_id > 0
                    ? sample.log_id
                    : `sample:${sample.request_id}:${sample.channel_id}:${sample.created_at}:${index}`
                }
              >
                {sample.request_id ? (
                  <a
                    href={`/usage-logs/common?requestId=${encodeURIComponent(sample.request_id)}${sample.created_at > 0 ? `&startTime=${Math.max(1, sample.created_at - 60) * 1000}&endTime=${(sample.created_at + 60) * 1000}` : ''}`}
                    className='break-all underline underline-offset-2'
                  >
                    {sample.request_id}
                  </a>
                ) : (
                  <span className='break-all'>
                    {t('Request ID unavailable')}
                  </span>
                )}{' '}
                ·{' '}
                {sample.channel_id > 0 && props.onSelectChannel ? (
                  <button
                    type='button'
                    onClick={() => props.onSelectChannel?.(sample.channel_id)}
                    className='underline underline-offset-2'
                  >
                    #{sample.channel_id}
                  </button>
                ) : (
                  `#${sample.channel_id}`
                )}{' '}
                · {new Date(sample.created_at * 1000).toLocaleString()}
                <br />
                {sample.reasons
                  .map((reason) => reasons[reason] ?? t('Pending verification'))
                  .join(' / ')}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}

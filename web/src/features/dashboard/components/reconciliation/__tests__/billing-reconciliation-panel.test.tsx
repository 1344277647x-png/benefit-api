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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { BillingReconciliationPanel } from '../billing-reconciliation-panel'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (!opts) return key
      return Object.entries(opts).reduce(
        (acc, [k, v]) => acc.replaceAll(`{{${k}}}`, String(v)),
        key
      )
    },
  }),
}))

afterEach(() => vi.restoreAllMocks())

it('renders read-only reconciliation totals, separates unknown-actual pre-consumes, and links request IDs to logs', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        read_only: true,
        generated_at: 1760000000,
        start_timestamp: 1759000000,
        end_timestamp: 1760000000,
        kind: 'all',
        quota_per_unit: 500000,
        truncated: false,
        summary: {
          total_items: 2,
          settlement_exception_count: 1,
          projection_failure_count: 0,
          unsettled_pre_consume_count: 1,
          unknown_actual_count: 1,
          known_pre_consumed_quota: 30,
          known_actual_quota: 100,
          known_net_diff_quota: 0,
          unknown_pre_consumed_quota: 500000,
          known_pre_consumed_usd: 0.00006,
          known_actual_usd: 0.0002,
          known_net_diff_usd: 0,
          unknown_pre_consumed_usd: 1.0,
        },
        items: [
          {
            source: 'business_event',
            kind: 'settlement_failed',
            event_key: 'billing-exception:req-settle-1',
            request_id: 'req-settle-1',
            user_id: 1188,
            username: 'viky',
            subscription_id: 42,
            funding_source: 'subscription',
            model_name: 'gpt-5',
            failure_stage: 'token_adjustment_failed',
            status: 'pending_manual_review',
            pre_consumed_quota: 30,
            actual_quota: 100,
            diff_quota: 0,
            token_diff_quota: 70,
            pre_consumed_usd: 0.00006,
            actual_usd: 0.0002,
            diff_usd: 0,
            has_consume_log: false,
            created_at: 1759999000,
            summary: 'settlement failed',
          },
          {
            source: 'subscription_pre_consume',
            kind: 'unsettled_pre_consume',
            request_id: 'req-unknown-1',
            user_id: 1188,
            username: 'viky',
            subscription_id: 42,
            funding_source: 'subscription',
            status: 'unsettled_pre_consume',
            pre_consumed_quota: 500000,
            pre_consumed_usd: 1.0,
            has_consume_log: false,
            created_at: 1759998000,
            summary: 'actual consumption unknown',
          },
        ],
      },
    },
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <BillingReconciliationPanel />
    </QueryClientProvider>
  )

  await waitFor(() => {
    expect(screen.getByText('req-settle-1')).toBeTruthy()
  })
  expect(screen.getByText('Read-only settlement reconciliation')).toBeTruthy()
  expect(
    screen.getByText('1 record(s) without consume log (excluded from net diff)')
  ).toBeTruthy()
  expect(
    screen.getByRole('columnheader', { name: 'Funding difference' })
  ).toBeTruthy()
  expect(
    screen.getByRole('columnheader', { name: 'Token difference (quota)' })
  ).toBeTruthy()
  const tokenFailure = within(screen.getByRole('row', { name: /req-settle-1/ }))
  expect(tokenFailure.getByText('0')).toBeTruthy()
  expect(tokenFailure.getByText('+70')).toBeTruthy()
  expect(
    within(screen.getByRole('row', { name: /req-unknown-1/ })).getAllByText(
      'Unknown'
    )
  ).toHaveLength(3)

  const reqLink = screen.getByRole('link', { name: 'req-settle-1' })
  expect(reqLink.getAttribute('href')).toContain(
    '/usage-logs/common?requestId=req-settle-1&startTime='
  )
})

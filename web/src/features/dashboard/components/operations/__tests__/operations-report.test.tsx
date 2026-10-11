import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getOperationsReport } from '../../../api'
import { OperationsReport } from '../operations-report'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

afterEach(() => vi.restoreAllMocks())

it('renders an empty report when the API serializes channel and health lists as null', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        generated_at: 1,
        start_timestamp: 1,
        end_timestamp: 2,
        group_by: 'day',
        truncated: false,
        quota_per_unit: 500000,
        summary: {
          request_count: 0,
          error_rate: 0,
          token_count: 0,
          revenue_quota: 0,
          refund_quota: 0,
          revenue_usd: 0,
          refund_usd: 0,
          cost_usd: 0,
          cost_status: 'unknown',
          unknown_cost_requests: 0,
        },
        rows: null,
        channel_ids: null,
        channels: null,
        health: null,
      },
    },
  })
  const result = await getOperationsReport({
    start_timestamp: 1,
    end_timestamp: 2,
    group_by: 'day',
  })
  expect(result.data?.channels).toEqual([])
  expect(result.data?.health).toEqual([])
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OperationsReport />
    </QueryClientProvider>
  )
  expect(await screen.findByText('No report data for this range')).toBeVisible()
  expect(screen.getByText('Upstream cost multipliers')).toBeVisible()
})

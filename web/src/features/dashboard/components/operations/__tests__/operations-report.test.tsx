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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { formatCurrencyUSD } from '@/lib/format'

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

it('uses confirmed income rather than nominal consumed quota in the breakdown', async () => {
  const errors = vi.spyOn(console, 'error')
  const amounts = {
    request_count: 1,
    success_count: 1,
    error_count: 0,
    error_rate: 0,
    token_count: 100,
    revenue_quota: 10965000,
    refund_quota: 0,
    revenue_usd: 9.21875,
    refund_usd: 0,
    cost_usd: 1,
    cost_status: 'known',
    unknown_cost_requests: 0,
    matched_requests: 1,
    pending_requests: 2,
    matched_quota: 1096500,
    partial_profit_usd: 8.21875,
    reasons: { before_cost_effective: 2 },
    pending_samples: [
      {
        log_id: 0,
        request_id: 'request-before-version',
        channel_id: 111,
        created_at: 1,
        reasons: ['before_cost_effective'],
      },
      {
        log_id: 0,
        request_id: 'another-request',
        channel_id: 111,
        created_at: 1,
        reasons: ['before_cost_effective'],
      },
    ],
  }
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
        summary: amounts,
        rows: [{ ...amounts, key: 'day:2026-10-11', label: '2026-10-11' }],
        channel_ids: [],
        channels: [],
        health: [],
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OperationsReport />
    </QueryClientProvider>
  )
  await screen.findByText('2026-10-11')
  expect(screen.getAllByText(formatCurrencyUSD(9.21875))).toHaveLength(2)
  const row = screen.getByRole('row', { name: /2026-10-11/ })
  expect(within(row).getByText(formatCurrencyUSD(21.93))).toBeVisible()
  expect(
    within(row).getByText(/Calculable consumption coverage.*10.00%/)
  ).toBeVisible()
  const details = within(row).getByText(
    'Verification reasons and request samples'
  )
  fireEvent.click(details)
  const sampleLink = within(row).getByRole('link', {
    name: 'request-before-version',
  })
  expect(sampleLink).toBeVisible()
  expect(sampleLink).toHaveAttribute(
    'href',
    '/usage-logs/common?requestId=request-before-version&startTime=1000&endTime=61000'
  )
  expect(within(row).getByText('another-request')).toBeVisible()
  expect(errors.mock.calls.flat().join(' ')).not.toMatch(
    /same key|unique.*key/i
  )
  fireEvent.click(within(row).getAllByRole('button', { name: '#111' })[0])
  expect(screen.getByRole('spinbutton', { name: 'Channel ID' })).toHaveValue(
    111
  )
  fireEvent.click(
    screen.getByLabelText('Only requests after each channel first cost version')
  )
  await waitFor(() =>
    expect(api.get).toHaveBeenLastCalledWith(
      '/api/log/operations-report',
      expect.objectContaining({
        params: expect.objectContaining({ after_cost_effective: true }),
      })
    )
  )
  fireEvent.click(screen.getByRole('button', { name: 'Reset filters' }))
  expect(
    screen.getByLabelText('Only requests after each channel first cost version')
  ).not.toBeChecked()
})

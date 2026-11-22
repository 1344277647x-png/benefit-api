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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { OpsAlertsPanel } from '../ops-alerts-panel'

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

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: ReactNode; className?: string }) => (
    <a href='#mock' className={props.className}>
      {props.children}
    </a>
  ),
}))

afterEach(() => vi.restoreAllMocks())

it('renders TLS expiry warning, queue backlogs, and dead-letter alerts with manual refresh', async () => {
  const getSpy = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        generated_at: 1760000000,
        alert_level: 'critical',
        tls: {
          status: 'warning',
          domain: 'xbenefitapi.xyz',
          issuer: "Let's Encrypt",
          not_after: 1760864000,
          days_remaining: 10,
          warning_threshold_days: 14,
          checked_at: 1760000000,
        },
        queues: {
          operations_pending: 4,
          operations_retrying: 1,
          operations_oldest_pending_at: 1759999000,
          business_pending: 2,
          business_retrying: 0,
          business_dead_letter: 1,
          billing_exceptions_count: 3,
          unsettled_pre_consume_count: 2,
        },
        alerts: [
          {
            code: 'tls_cert_expiring',
            severity: 'warning',
            title: 'TLS certificate expiring soon',
            detail: 'xbenefitapi.xyz expires in 10 day(s).',
          },
          {
            code: 'business_event_dead_letter',
            severity: 'critical',
            title: 'Business outbox dead letters detected',
            detail: '1 dead-letter event(s) require inspection.',
            count: 1,
            action_path: '/dashboard/reconciliation',
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
      <OpsAlertsPanel />
    </QueryClientProvider>
  )

  await waitFor(() => {
    expect(screen.getByText('xbenefitapi.xyz')).toBeTruthy()
  })
  expect(screen.getByText('Expiring in 10 days')).toBeTruthy()
  expect(screen.getByText('4 pending')).toBeTruthy()
  expect(screen.getByText('Dead letters: 1')).toBeTruthy()
  expect(screen.getByText('Business outbox dead letters detected')).toBeTruthy()

  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => {
    expect(getSpy).toHaveBeenCalledWith('/api/log/ops-alerts', {
      params: { refresh: 'true' },
    })
  })
})

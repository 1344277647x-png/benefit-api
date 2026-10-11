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
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type React from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { getBusinessLogDetails } from '../../lib/format'
import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

function normalizedText(value: string | null): string {
  return (value ?? '').replaceAll(/\s/g, '')
}

describe('log cost display', () => {
  test('shows the stored lottery yuan prize instead of the zero consumption column', () => {
    renderCost({
      type: 9,
      quota: 0,
      other: { reward_cents: 10000, reward_quota: 7000000 },
    })
    expect(screen.getByText('¥100.00')).toBeInTheDocument()
    expect(screen.getByText('Lottery prize amount')).toBeInTheDocument()
  })

  test.each(['referral.reward_created', 'referral.reward_settled'])(
    'shows referral amount for %s',
    (action) => {
      const rendered = renderCost({
        type: 8,
        quota: 0,
        other: { business_event: { action }, reward_quota: 25000 },
      })
      expect(normalizedText(rendered.container.textContent)).toContain(
        normalizedText(formatLogQuota(25000))
      )
    }
  )

  test('shows a transfer and all four stored balances without treating it as another reward', () => {
    const other = {
      business_event: { action: 'referral.transfer_to_balance' },
      quota: 25000,
      referral_balance_before: 25000,
      referral_balance_after: 0,
      site_balance_before: 10000,
      site_balance_after: 35000,
    }
    expect(getBusinessLogDetails(8, other, (key) => key)).toEqual([
      { label: 'Transferred to site balance', value: formatLogQuota(25000) },
      { label: 'Referral balance before', value: formatLogQuota(25000) },
      { label: 'Referral balance after', value: formatLogQuota(0) },
      { label: 'Site balance before', value: formatLogQuota(10000) },
      { label: 'Site balance after', value: formatLogQuota(35000) },
    ])
  })

  test('preserves draw pool, number and immutable string version in details', () => {
    expect(
      getBusinessLogDetails(
        9,
        {
          reward_cents: 50,
          tier: 'jackpot',
          draw_number: 50,
          rule_version: 'rule-123',
        },
        (key) => key
      )
    ).toEqual([
      { label: 'Lottery prize amount', value: '¥0.50' },
      { label: 'Draw number', value: '50' },
      { label: 'Prize pool', value: 'Jackpot pool' },
      { label: 'Rule version', value: 'rule-123' },
    ])
  })

  test('does not invent a zero amount when historical business fields are missing', () => {
    renderCost({ type: 8, quota: 0, other: null })
    expect(screen.getByText('Amount not recorded')).toBeInTheDocument()
  })

  test('keeps explicit zero rewards visible but ignores malformed amounts', () => {
    expect(
      getBusinessLogDetails(8, { reward_quota: 0 }, (key) => key)[0].value
    ).toBe(formatLogQuota(0))
    expect(
      getBusinessLogDetails(9, { reward_cents: -1 }, (key) => key)[0].value
    ).toBe('Amount not recorded')
    expect(
      getBusinessLogDetails(2, { reward_cents: 100 }, (key) => key)
    ).toEqual([])
  })
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Subscription: 'Subscription',
      'Deducted by subscription': 'Deducted by subscription',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
    })
  })

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formatLogQuota(12500))
      )
    ).toBe(true)
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('preserves the subscription badge and adds the same legacy surcharge marker', () => {
    renderCost({
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
    })

    expect(screen.getByText('Subscription')).toBeInTheDocument()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })
})

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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import type React from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
} from 'vitest'

import { formatLogQuota } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { getBusinessLogDetails } from '../../lib/format'
import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
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
      renderCost({
        type: 8,
        quota: 0,
        other: { business_event: { action }, reward_quota: 25000 },
      })
      expect(screen.getByText(formatLogQuota(25000))).toBeInTheDocument()
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
      Wallet: 'Wallet',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
    })
  })

  beforeEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  afterEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
    localStorage.clear()
  })

  test.each([
    { consumed: 12500, expected: '$0.025' },
    { consumed: 0, expected: '$0' },
    { consumed: 1, expected: '$0.000002' },
    { consumed: undefined, expected: '$0.01' },
  ])(
    'shows subscription deduction $consumed without hover, falling back only when absent',
    ({ consumed, expected }) => {
      renderCost({
        quota: 5000,
        other: {
          billing_source: 'subscription',
          subscription_consumed: consumed,
        },
        showBillingSource: true,
      })

      expect(screen.getByText(expected)).toBeVisible()
      expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
      expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    }
  )

  test('shows wallet cost and source icon without using subscription metadata', () => {
    renderCost({
      quota: 5000,
      other: { billing_source: 'wallet', subscription_consumed: 12500 },
      showBillingSource: true,
    })

    expect(screen.getByText('$0.01')).toBeVisible()
    expect(screen.getByRole('img', { name: 'Wallet' })).toBeVisible()
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
  })

  test('hides the wallet icon when subscriptions are unavailable', () => {
    renderCost({
      quota: 5000,
      other: { billing_source: 'wallet' },
      showBillingSource: false,
    })

    expect(screen.getByText('$0.01')).toBeVisible()
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('keeps the subscription icon on a subscription-billed log when billing sources are hidden', () => {
    renderCost({
      quota: 5000,
      other: { billing_source: 'subscription', subscription_consumed: 12500 },
      showBillingSource: false,
    })

    expect(screen.getByText('$0.025')).toBeVisible()
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
    expect(
      screen.queryByRole('img', { name: 'Wallet' })
    ).not.toBeInTheDocument()
  })

  test('keeps legacy cost visible without inventing a funding source', () => {
    renderCost({ quota: 5000, other: null })

    expect(screen.getByText('$0.01')).toBeVisible()
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('keeps a large amount unabridged in a single-line cost bubble', () => {
    const rendered = renderCost({
      quota: 2147483647,
      other: { billing_source: 'subscription' },
      showBillingSource: true,
    })

    const amount = screen.getByText('$4,294.9673')
    expect(amount).toBeVisible()
    expect(amount).toHaveClass('whitespace-nowrap')
    expect(amount.closest('[data-slot="status-badge"]')).toHaveClass(
      'border',
      'rounded-md',
      'tabular-nums'
    )
    expect(rendered.container.firstElementChild).toHaveClass('inline-flex')
    expect(rendered.container.firstElementChild).not.toHaveClass('flex-col')
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
  })

  test.each([
    { source: 'subscription', label: 'Subscription' },
    { source: 'wallet', label: 'Wallet' },
  ])(
    'vertically centers the $source icon within its container',
    ({ source, label }) => {
      renderCost({
        quota: 5000,
        other: { billing_source: source },
        showBillingSource: true,
      })

      // Mobile cards give inline-flex descendants a minimum height, so the
      // source container must center its smaller icon independently of the badge.
      expect(screen.getByRole('img', { name: label })).toHaveClass(
        'inline-flex',
        'items-center'
      )
    }
  )

  test.each([
    { source: 'subscription', label: 'Subscription' },
    { source: 'wallet', label: 'Wallet' },
  ])(
    'reveals the $source label on hover and keyboard focus',
    async ({ source, label }) => {
      const user = userEvent.setup()
      renderCost({
        quota: 5000,
        other: { billing_source: source },
        showBillingSource: true,
      })

      const marker = screen.getByRole('img', { name: label })
      expect(screen.queryByText(label)).not.toBeInTheDocument()

      await user.hover(marker)
      expect(await screen.findByText(label)).toBeVisible()
      await user.unhover(marker)
      await waitFor(() =>
        expect(screen.queryByText(label)).not.toBeInTheDocument()
      )
      await user.tab()
      expect(marker).toHaveFocus()
      expect(await screen.findByText(label)).toBeVisible()
    }
  )

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    renderCost({
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(screen.getByText('$0.025')).toBeVisible()
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('shows subscription cost and source alongside the legacy surcharge marker', () => {
    renderCost({
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
      showBillingSource: true,
    })

    expect(screen.getByText('$0.01')).toBeVisible()
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })
})

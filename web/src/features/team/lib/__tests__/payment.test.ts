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
import { describe, expect, test } from 'vitest'

import type { TopupInfo } from '@/features/wallet/types'

import { availableTeamEpayMethods } from '../payment'

describe('team online payment methods', () => {
  test('excludes non-Epay wallet gateways when online Epay is configured', () => {
    const payment = {
      enable_online_topup: true,
      pay_methods: [
        { type: 'alipay', name: 'Epay Alipay' },
        { type: 'alipay_native', name: 'Native Alipay' },
        { type: 'waffo', name: 'Waffo' },
      ],
      epay_pay_methods: [{ type: 'alipay', name: 'Epay Alipay' }],
    } as TopupInfo

    expect(availableTeamEpayMethods(payment)).toEqual([
      { type: 'alipay', name: 'Epay Alipay' },
    ])
  })

  test('does not infer Epay methods from wallet methods when gateway is disabled', () => {
    const payment = {
      enable_online_topup: false,
      pay_methods: [{ type: 'alipay_native', name: 'Native Alipay' }],
    } as TopupInfo

    expect(availableTeamEpayMethods(payment)).toEqual([])
  })
})

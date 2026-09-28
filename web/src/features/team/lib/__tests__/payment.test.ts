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

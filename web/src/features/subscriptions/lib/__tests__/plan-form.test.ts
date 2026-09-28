import { describe, expect, it } from 'vitest'

import { formValuesToPlanPayload, PLAN_FORM_DEFAULTS } from '../plan-form'

describe('subscription plan form', () => {
  it('keeps a new plan personal with no reserved seats', () => {
    const payload = formValuesToPlanPayload(PLAN_FORM_DEFAULTS)

    expect(payload.plan.scope).toBe('personal')
    expect(payload.plan.seat_limit).toBe(0)
  })

  it('clears personal-only group and payment fields for team plans', () => {
    const payload = formValuesToPlanPayload({
      ...PLAN_FORM_DEFAULTS,
      scope: 'team',
      seat_limit: 5,
      upgrade_group: 'vip',
      downgrade_group: 'default',
      allow_wallet_overflow: true,
      stripe_price_id: 'price_personal',
    })

    expect(payload.plan).toMatchObject({
      scope: 'team',
      seat_limit: 5,
      upgrade_group: '',
      downgrade_group: '',
      allow_wallet_overflow: false,
      stripe_price_id: '',
    })
  })
})

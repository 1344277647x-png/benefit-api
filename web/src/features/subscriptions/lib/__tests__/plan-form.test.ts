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

import { describe, expect, it } from 'vitest'

import {
  LOTTERY_WHEEL_REWARDS_CENTS,
  nextWheelRotation,
  rewardWheelIndex,
} from '../wheel-config'

describe('lottery wheel configuration', () => {
  it('shows every approved reward amount', () => {
    expect(LOTTERY_WHEEL_REWARDS_CENTS).toEqual([
      50, 100, 200, 500, 1000, 5000, 10000,
    ])
  })

  it('lands on the reward returned by the backend', () => {
    const next = nextWheelRotation(0, 0, 5000)
    expect(next.index).toBe(rewardWheelIndex(5000))
    expect(next.rotation).toBeGreaterThanOrEqual(4 * 360)
  })
})

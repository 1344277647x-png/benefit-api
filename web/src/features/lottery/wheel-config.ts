export const LOTTERY_WHEEL_REWARDS_CENTS = [
  50, 100, 200, 500, 1000, 5000, 10000,
] as const

export const LOTTERY_WHEEL_STEP_DEGREES =
  360 / LOTTERY_WHEEL_REWARDS_CENTS.length

export function rewardWheelIndex(rewardCents: number): number {
  const index = LOTTERY_WHEEL_REWARDS_CENTS.indexOf(
    rewardCents as (typeof LOTTERY_WHEEL_REWARDS_CENTS)[number]
  )
  return index < 0 ? 0 : index
}

export function nextWheelRotation(
  currentRotation: number,
  currentIndex: number,
  rewardCents: number
): { rotation: number; index: number } {
  const index = rewardWheelIndex(rewardCents)
  const forwardSteps =
    (currentIndex - index + LOTTERY_WHEEL_REWARDS_CENTS.length) %
    LOTTERY_WHEEL_REWARDS_CENTS.length
  return {
    rotation:
      currentRotation + 4 * 360 + forwardSteps * LOTTERY_WHEEL_STEP_DEGREES,
    index,
  }
}

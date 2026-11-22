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
